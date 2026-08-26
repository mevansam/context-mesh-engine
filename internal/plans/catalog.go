// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mevansam/context-mesh-engine/arazzo"
	"github.com/pb33f/libopenapi"
	libarazzo "github.com/pb33f/libopenapi/arazzo"
	high "github.com/pb33f/libopenapi/datamodel/high/arazzo"
	v3high "github.com/pb33f/libopenapi/datamodel/high/v3"
	"go.yaml.in/yaml/v4"
	"golang.org/x/mod/semver"
)

const planIDExt = "x-planId"

// Entry is one loaded Arazzo plan version.
type Entry struct {
	PlanID    string
	Version   string
	URI       string
	Doc       *high.Arazzo
	Sources   []*libarazzo.ResolvedSource
	Workflows []arazzo.WorkflowDoc
}

// VersionSegment is the REST path token: "v" + Version.
func (e *Entry) VersionSegment() string {
	return "v" + e.Version
}

// Catalog indexes plans by (planId, version).
type Catalog struct {
	entries []*Entry
	byKey   map[string]*Entry
	latest  map[string]string // planId -> version
}

func key(planID, version string) string {
	return planID + "\x00" + version
}

// Load parses sources from loaders into a catalog.
func Load(ctx context.Context, loaders []arazzo.Loader, logger *slog.Logger) (*Catalog, error) {
	if logger == nil {
		logger = slog.Default()
	}
	c := &Catalog{
		byKey:  make(map[string]*Entry),
		latest: make(map[string]string),
	}
	for _, loader := range loaders {
		sources, err := loader.Load(ctx)
		if err != nil {
			return nil, err
		}
		for _, src := range sources {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if err := c.addSource(src, logger); err != nil {
				return nil, err
			}
		}
	}
	c.computeLatest()
	return c, nil
}

func (c *Catalog) addSource(src arazzo.Source, logger *slog.Logger) error {
	doc, err := libopenapi.NewArazzoDocument(src.Data)
	if err != nil {
		return fmt.Errorf("%s: parse arazzo: %w", src.URI, err)
	}
	if doc.Info == nil {
		logger.Warn("skipping arazzo document without info", "uri", src.URI)
		return nil
	}
	planID := extensionString(doc.Info, planIDExt)
	if planID == "" || doc.Info.Version == "" {
		logger.Warn("skipping arazzo document without x-planId or version", "uri", src.URI)
		return nil
	}
	sources, err := resolveSources(doc, src)
	if err != nil {
		return fmt.Errorf("%s: resolve sources: %w", src.URI, err)
	}
	if vr := libarazzo.Validate(doc); vr != nil && vr.HasErrors() {
		return fmt.Errorf("%s: validate: %w", src.URI, vr)
	}

	k := key(planID, doc.Info.Version)
	if _, dup := c.byKey[k]; dup {
		return fmt.Errorf("duplicate plan %s version %s (also %s)", planID, doc.Info.Version, src.URI)
	}

	e := &Entry{
		PlanID:    planID,
		Version:   doc.Info.Version,
		URI:       src.URI,
		Doc:       doc,
		Sources:   sources,
		Workflows: workflowDocs(doc),
	}
	c.entries = append(c.entries, e)
	c.byKey[k] = e
	return nil
}

func resolveSources(doc *high.Arazzo, src arazzo.Source) ([]*libarazzo.ResolvedSource, error) {
	var roots []string
	if u, err := url.Parse(src.BaseURL); err == nil && u.Scheme == "file" && u.Path != "" {
		dir := filepath.Clean(filepath.FromSlash(u.Path))
		// Include the spec directory and its parent so relative source
		// URLs like ../sources/openapi.yaml stay inside FSRoots.
		roots = []string{dir, filepath.Dir(dir)}
	}
	return libarazzo.ResolveSources(doc, &libarazzo.ResolveConfig{
		BaseURL: src.BaseURL,
		FSRoots: roots,
		OpenAPIFactory: func(_ string, b []byte) (*v3high.Document, error) {
			d, err := libopenapi.NewDocument(b)
			if err != nil {
				return nil, err
			}
			m, err := d.BuildV3Model()
			if err != nil {
				return nil, err
			}
			return &m.Model, nil
		},
		ArazzoFactory: func(_ string, b []byte) (*high.Arazzo, error) {
			return libopenapi.NewArazzoDocument(b)
		},
	})
}

func workflowDocs(doc *high.Arazzo) []arazzo.WorkflowDoc {
	var out []arazzo.WorkflowDoc
	for _, wf := range doc.Workflows {
		if wf == nil || wf.WorkflowId == "" {
			continue
		}
		out = append(out, arazzo.WorkflowDoc{
			ID:          wf.WorkflowId,
			Summary:     wf.Summary,
			Description: wf.Description,
		})
	}
	return out
}

func extensionString(info *high.Info, name string) string {
	if info == nil || info.Extensions == nil {
		return ""
	}
	node, ok := info.Extensions.Get(name)
	if !ok || node == nil {
		return ""
	}
	if node.Kind == yaml.ScalarNode {
		return node.Value
	}
	return ""
}

func (c *Catalog) computeLatest() {
	byPlan := map[string][]string{}
	for _, e := range c.entries {
		byPlan[e.PlanID] = append(byPlan[e.PlanID], e.Version)
	}
	for planID, vers := range byPlan {
		c.latest[planID] = pickLatest(vers)
	}
}

func pickLatest(versions []string) string {
	if len(versions) == 0 {
		return ""
	}
	canon := make([]string, len(versions))
	all := true
	for i, v := range versions {
		c, ok := semverCanon(v)
		if !ok {
			all = false
			break
		}
		canon[i] = c
	}
	if all {
		best := 0
		for i := 1; i < len(versions); i++ {
			if semver.Compare(canon[i], canon[best]) > 0 {
				best = i
			}
		}
		return versions[best]
	}
	sorted := append([]string(nil), versions...)
	sort.Strings(sorted)
	return sorted[len(sorted)-1]
}

func semverCanon(v string) (string, bool) {
	if semver.IsValid(v) {
		return v, true
	}
	if semver.IsValid("v" + v) {
		return "v" + v, true
	}
	return "", false
}

// Get returns the entry for planId + raw info.version.
func (c *Catalog) Get(planID, version string) (*Entry, bool) {
	e, ok := c.byKey[key(planID, version)]
	return e, ok
}

// GetBySegment looks up VersionSegment (e.g. v1.0.0).
func (c *Catalog) GetBySegment(planID, segment string) (*Entry, bool) {
	raw, ok := strings.CutPrefix(segment, "v")
	if !ok || raw == "" {
		return nil, false
	}
	return c.Get(planID, raw)
}

// LatestVersion returns the latest raw version for planID.
func (c *Catalog) LatestVersion(planID string) (string, bool) {
	v, ok := c.latest[planID]
	return v, ok
}

// Latest returns the latest entry for planID.
func (c *Catalog) Latest(planID string) (*Entry, bool) {
	v, ok := c.LatestVersion(planID)
	if !ok {
		return nil, false
	}
	return c.Get(planID, v)
}

// Entries returns all loaded plans.
func (c *Catalog) Entries() []*Entry {
	return c.entries
}

// WorkflowIDs of an entry.
func (e *Entry) WorkflowIDs() []string {
	ids := make([]string, 0, len(e.Workflows))
	for _, w := range e.Workflows {
		ids = append(ids, w.ID)
	}
	return ids
}

func (e *Entry) summary() arazzo.PlanSummary {
	title, summary, desc := "", "", ""
	if e.Doc != nil && e.Doc.Info != nil {
		title = e.Doc.Info.Title
		summary = e.Doc.Info.Summary
		desc = e.Doc.Info.Description
	}
	wfs := append([]arazzo.WorkflowDoc(nil), e.Workflows...)
	return arazzo.PlanSummary{
		PlanID:      e.PlanID,
		Version:     e.Version,
		Title:       title,
		Summary:     summary,
		Description: desc,
		Workflows:   wfs,
	}
}

// View is the [arazzo.PlanCatalog] for this catalog. It does not copy
// entries until Get, Latest, or Plans is used.
func (c *Catalog) View() arazzo.PlanCatalog {
	return catalogView{c: c}
}

type catalogView struct{ c *Catalog }

func (v catalogView) Get(planID, version string) (arazzo.PlanSummary, bool) {
	if v.c == nil {
		return arazzo.PlanSummary{}, false
	}
	e, ok := v.c.Get(planID, version)
	if !ok {
		return arazzo.PlanSummary{}, false
	}
	return e.summary(), true
}

func (v catalogView) Latest(planID string) (arazzo.PlanSummary, bool) {
	if v.c == nil {
		return arazzo.PlanSummary{}, false
	}
	e, ok := v.c.Latest(planID)
	if !ok {
		return arazzo.PlanSummary{}, false
	}
	return e.summary(), true
}

func (v catalogView) Plans() iter.Seq[arazzo.PlanSummary] {
	return func(yield func(arazzo.PlanSummary) bool) {
		if v.c == nil {
			return
		}
		for _, e := range v.c.entries {
			if !yield(e.summary()) {
				return
			}
		}
	}
}
