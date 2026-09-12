// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package arazzo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// sharedPolicyDirName is the FilePolicyLoader directory for [SharedPolicySource].
// It is not a planId; [PolicyLoader.Load] rejects it.
const sharedPolicyDirName = "_shared"

var (
	_ PolicyLoader       = (*FilePolicyLoader)(nil)
	_ SharedPolicySource = (*FilePolicyLoader)(nil)
)

// FilePolicyLoader loads OPA modules from a directory tree.
// Plan modules: {Dir}/{planId}/{version}/inbound.rego and outbound.rego.
// Optional data.json in that directory is merged with [FilePolicyLoader.Data]
// (struct fields override file keys) and returned as [PolicyBundle.Data].
// Shared modules: {Dir}/_shared/inbound.rego, outbound.rego, and
// {Dir}/_shared/lib/**/*.rego (libraries).
type FilePolicyLoader struct {
	Dir  string
	Data map[string]any
}

// NewFilePolicyLoader returns a filesystem [PolicyLoader] rooted at dir.
func NewFilePolicyLoader(dir string) *FilePolicyLoader {
	return &FilePolicyLoader{Dir: dir}
}

// Load implements [PolicyLoader]. Missing directories or modules are not
// an error: a nil bundle means no policy. Invalid planId/version (path
// separators or "..") is an error. planId "_shared" is reserved for
// [FilePolicyLoader.LoadShared].
func (l *FilePolicyLoader) Load(ctx context.Context, req PolicyRequest) (*PolicyBundle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.PlanID == sharedPolicyDirName {
		return nil, fmt.Errorf("policy planId %q is reserved by FilePolicyLoader", sharedPolicyDirName)
	}
	dir, err := policyDir(l.Dir, req.PlanID, req.Version)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	inbound, err := readOptional(filepath.Join(dir, "inbound.rego"))
	if err != nil {
		return nil, err
	}
	outbound, err := readOptional(filepath.Join(dir, "outbound.rego"))
	if err != nil {
		return nil, err
	}
	if len(inbound) == 0 && len(outbound) == 0 {
		return nil, nil
	}

	data, err := mergePolicyData(filepath.Join(dir, "data.json"), l.Data)
	if err != nil {
		return nil, err
	}
	return &PolicyBundle{
		Inbound:  inbound,
		Outbound: outbound,
		Data:     data,
		Revision: contentRevision(map[string][]byte{
			"inbound.rego":  inbound,
			"outbound.rego": outbound,
			"data.json":     data,
		}),
	}, nil
}

// LoadShared implements [SharedPolicySource]. Missing _shared is not an
// error: a nil result means no shared modules.
func (l *FilePolicyLoader) LoadShared(ctx context.Context) (*SharedPolicy, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := filepath.Abs(l.Dir)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, sharedPolicyDirName)
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("policy path escapes root")
	}
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	inbound, err := readOptional(filepath.Join(dir, "inbound.rego"))
	if err != nil {
		return nil, err
	}
	outbound, err := readOptional(filepath.Join(dir, "outbound.rego"))
	if err != nil {
		return nil, err
	}
	libs, err := readSharedLibraries(dir)
	if err != nil {
		return nil, err
	}
	if len(inbound) == 0 && len(outbound) == 0 && len(libs) == 0 {
		return nil, nil
	}

	named := map[string][]byte{
		"inbound.rego":  inbound,
		"outbound.rego": outbound,
	}
	for k, v := range libs {
		named[k] = v
	}
	return &SharedPolicy{
		Inbound:   inbound,
		Outbound:  outbound,
		Libraries: libs,
		Revision:  contentRevision(named),
	}, nil
}

func readSharedLibraries(sharedDir string) (map[string][]byte, error) {
	libDir := filepath.Join(sharedDir, "lib")
	if _, err := os.Stat(libDir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := map[string][]byte{}
	err := filepath.WalkDir(libDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".rego" {
			return nil
		}
		rel, err := filepath.Rel(libDir, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("policy path escapes root")
		}
		src, err := readOptional(path)
		if err != nil {
			return err
		}
		if len(src) == 0 {
			return nil
		}
		out["lib/"+filepath.ToSlash(rel)] = src
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func contentRevision(named map[string][]byte) string {
	keys := make([]string, 0, len(named))
	for k := range named {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		_, _ = h.Write([]byte(k))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(named[k])
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func policyDir(root, planID, version string) (string, error) {
	if err := safePolicySegment(planID, "planId"); err != nil {
		return "", err
	}
	if err := safePolicySegment(version, "version"); err != nil {
		return "", err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(absRoot, planID, version)
	rel, err := filepath.Rel(absRoot, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("policy path escapes root")
	}
	return dir, nil
}

func safePolicySegment(s, name string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("policy %s is empty", name)
	}
	if strings.Contains(s, "..") || strings.ContainsAny(s, `/\`) {
		return fmt.Errorf("policy %s %q is not a safe path segment", name, s)
	}
	return nil
}

func readOptional(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return nil, nil
	}
	return b, nil
}

func mergePolicyData(path string, extra map[string]any) ([]byte, error) {
	out := map[string]any{}
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil && len(bytes.TrimSpace(b)) > 0 {
		if err := json.Unmarshal(b, &out); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if out == nil {
			out = map[string]any{}
		}
	}
	for k, v := range extra {
		out[k] = v
	}
	if len(out) == 0 {
		return nil, nil
	}
	return json.Marshal(out)
}
