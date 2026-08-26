// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package arazzo

import (
	"bytes"
	"strings"
	"text/template"
	"unicode"
)

// ToolDocTemplates are Go text/templates that produce tool name, title,
// and transport-specific descriptions. Empty fields use [DefaultToolDocTemplates].
//
// Description and QueryDescription are used on MCP tools/list.
// RESTDescription and RESTQueryDescription are used on GET /tools.
// Name and title are shared.
//
// These fields are recipes, not template variables. Do not write
// {{.ToolDoc.Title}} — write {{.Title}} to insert Arazzo info.title.
type ToolDocTemplates struct {
	Name                 string
	Title                string
	Description          string
	RESTDescription      string
	QueryName            string
	QueryTitle           string
	QueryDescription     string
	RESTQueryDescription string
}

// RenderedToolDoc is the result of [RenderToolDoc] or [RenderQueryDoc].
type RenderedToolDoc struct {
	Name            string
	Title           string
	Description     string
	RESTDescription string
}

// WorkflowDoc is one workflow in [ToolDocContext.Workflows].
type WorkflowDoc struct {
	ID                   string
	Summary              string
	Description          string
	SummaryOrDescription string
}

// ToolDocContext is the data bag passed to ToolDoc templates.
type ToolDocContext struct {
	PlanID                  string
	Version                 string
	Title                   string
	Summary                 string
	Description             string
	Workflows               []WorkflowDoc
	WorkflowIDs             string
	SafePlanID              string
	SafeVersion             string
	VersionSegment          string
	PublicBaseURL           string
	APIRoot                 string
	RESTQueryURL            string
	RESTExecuteLatestURL    string
	RESTExecuteVersionedURL string
	OpenAPILatestURL        string
	OpenAPIVersionedURL     string
}

// DefaultToolDocTemplates returns the built-in recipes.
func DefaultToolDocTemplates() ToolDocTemplates {
	return ToolDocTemplates{
		Name:                 `run_{{.SafePlanID}}_v{{.SafeVersion}}`,
		Title:                `{{.Title}} ({{.PlanID}} {{.VersionSegment}})`,
		Description:          defaultMCPDescriptionTemplate,
		RESTDescription:      defaultRESTDescriptionTemplate,
		QueryName:            "query",
		QueryTitle:           "Query plans",
		QueryDescription:     defaultMCPQueryDescription,
		RESTQueryDescription: defaultRESTQueryDescription,
	}
}

const defaultMCPQueryDescription = `Match a simple natural-language request plus inputs against Arazzo plans, then execute the selected plan.`

const defaultRESTQueryDescription = `Match a simple natural-language request plus inputs against Arazzo plans, then execute the selected plan. POST {{.RESTQueryURL}}.`

const defaultMCPDescriptionTemplate = `{{.Title}}

Plan ID: {{.PlanID}}
Version: {{.Version}}
MCP tool name: run_{{.SafePlanID}}_v{{.SafeVersion}}

{{if .Summary}}Summary:
{{.Summary}}
{{end}}{{if .Description}}Description:
{{.Description}}
{{end}}How to call this MCP tool:
- Set workflowId to one of: {{.WorkflowIDs}}
- Set inputs to a JSON object that matches that workflow's Arazzo inputs schema (see inputSchema.oneOf).

Workflows in this plan:
{{range .Workflows}}- {{.ID}}: {{.SummaryOrDescription}}
{{end}}`

const defaultRESTDescriptionTemplate = `{{.Title}}

Plan ID: {{.PlanID}}
Version: {{.Version}}

{{if .Summary}}Summary:
{{.Summary}}
{{end}}{{if .Description}}Description:
{{.Description}}
{{end}}How to call:
POST with Content-Type: application/json. The JSON body is the workflow inputs object.
- This version: POST {{.RESTExecuteVersionedURL}}
- Latest version of this plan: POST {{.RESTExecuteLatestURL}}
Replace {workflowId} with one of: {{.WorkflowIDs}}

OpenAPI:
- This version: GET {{.OpenAPIVersionedURL}}
- Latest: GET {{.OpenAPILatestURL}}

Workflows in this plan:
{{range .Workflows}}- {{.ID}}: {{.SummaryOrDescription}}
{{end}}`

// MergeTemplates fills empty fields from defaults.
func MergeTemplates(t ToolDocTemplates) ToolDocTemplates {
	d := DefaultToolDocTemplates()
	if t.Name == "" {
		t.Name = d.Name
	}
	if t.Title == "" {
		t.Title = d.Title
	}
	if t.Description == "" {
		t.Description = d.Description
	}
	if t.RESTDescription == "" {
		t.RESTDescription = d.RESTDescription
	}
	if t.QueryName == "" {
		t.QueryName = d.QueryName
	}
	if t.QueryTitle == "" {
		t.QueryTitle = d.QueryTitle
	}
	if t.QueryDescription == "" {
		t.QueryDescription = d.QueryDescription
	}
	if t.RESTQueryDescription == "" {
		t.RESTQueryDescription = d.RESTQueryDescription
	}
	return t
}

// NewToolDocContext builds template data from spec fields plus PublicBaseURL
// and the REST API prefix (for example /api). Empty apiPrefix defaults to /api.
func NewToolDocContext(planID, version, title, summary, description string, workflows []WorkflowDoc, publicBaseURL, apiPrefix string) ToolDocContext {
	ids := make([]string, 0, len(workflows))
	wfs := make([]WorkflowDoc, len(workflows))
	for i, w := range workflows {
		w.SummaryOrDescription = firstNonEmpty(w.Summary, firstLine(w.Description), w.ID)
		wfs[i] = w
		ids = append(ids, w.ID)
	}
	base := strings.TrimRight(publicBaseURL, "/")
	apiRoot := joinPublicURL(base, normalizeAPIPrefix(apiPrefix))
	seg := "v" + version
	ctx := ToolDocContext{
		PlanID:                  planID,
		Version:                 version,
		Title:                   title,
		Summary:                 summary,
		Description:             description,
		Workflows:               wfs,
		WorkflowIDs:             strings.Join(ids, ", "),
		SafePlanID:              sanitizeRunes(planID, false),
		SafeVersion:             sanitizeRunes(version, true),
		VersionSegment:          seg,
		PublicBaseURL:           base,
		APIRoot:                 apiRoot,
		RESTQueryURL:            apiRoot + "/plans/query",
		RESTExecuteLatestURL:    apiRoot + "/plans/" + planID + "/{workflowId}",
		RESTExecuteVersionedURL: apiRoot + "/plans/" + planID + "/" + seg + "/{workflowId}",
		OpenAPILatestURL:        apiRoot + "/openapi/" + planID,
		OpenAPIVersionedURL:     apiRoot + "/openapi/" + planID + "/" + seg,
	}
	return ctx
}

const defaultAPIPrefix = "/api"

func normalizeAPIPrefix(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return defaultAPIPrefix
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}

func joinPublicURL(base, prefix string) string {
	if base == "" {
		return prefix
	}
	return base + prefix
}

// RenderToolDoc executes name, title, MCP description, and REST description templates.
func RenderToolDoc(tmpls ToolDocTemplates, ctx ToolDocContext) (RenderedToolDoc, error) {
	tmpls = MergeTemplates(tmpls)
	return renderDoc(tmpls.Name, tmpls.Title, tmpls.Description, tmpls.RESTDescription,
		"name", "title", "description", "restDescription", ctx)
}

// RenderQueryDoc executes query name, title, MCP description, and REST description templates.
func RenderQueryDoc(tmpls ToolDocTemplates, ctx ToolDocContext) (RenderedToolDoc, error) {
	tmpls = MergeTemplates(tmpls)
	return renderDoc(tmpls.QueryName, tmpls.QueryTitle, tmpls.QueryDescription, tmpls.RESTQueryDescription,
		"queryName", "queryTitle", "queryDescription", "restQueryDescription", ctx)
}

func renderDoc(nameSrc, titleSrc, descSrc, restSrc, nameID, titleID, descID, restID string, ctx ToolDocContext) (RenderedToolDoc, error) {
	name, err := execTemplate(nameID, nameSrc, ctx)
	if err != nil {
		return RenderedToolDoc{}, err
	}
	name = SanitizeToolName(name)
	if name == "" {
		return RenderedToolDoc{}, errEmptyToolName
	}
	title, err := execTemplate(titleID, titleSrc, ctx)
	if err != nil {
		return RenderedToolDoc{}, err
	}
	description, err := execTemplate(descID, descSrc, ctx)
	if err != nil {
		return RenderedToolDoc{}, err
	}
	restDescription, err := execTemplate(restID, restSrc, ctx)
	if err != nil {
		return RenderedToolDoc{}, err
	}
	return RenderedToolDoc{
		Name:            name,
		Title:           strings.TrimSpace(title),
		Description:     strings.TrimSpace(description),
		RESTDescription: strings.TrimSpace(restDescription),
	}, nil
}

var errEmptyToolName = errString("tool name is empty after sanitization")

type errString string

func (e errString) Error() string { return string(e) }

func execTemplate(name, src string, data any) (string, error) {
	t, err := template.New(name).Option("missingkey=zero").Parse(src)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// SanitizeToolName keeps MCP-valid runes [A-Za-z0-9_.-] and truncates to 128.
func SanitizeToolName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if isMCPToolRune(r) {
			b.WriteRune(r)
		} else if unicode.IsSpace(r) {
			b.WriteByte('_')
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if len(out) > 128 {
		out = out[:128]
	}
	return out
}

func sanitizeRunes(s string, allowDot bool) string {
	var b strings.Builder
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if allowDot && r == '.' {
			ok = true
		}
		if ok {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func isMCPToolRune(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_' || r == '-' || r == '.'
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
