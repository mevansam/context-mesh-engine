// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package arazzo

import (
	"bytes"
	"strings"
	"text/template"
	"unicode"
)

// ToolDocTemplates are Go text/templates that produce MCP Tool.name,
// Tool.title, and Tool.description. Empty fields use [DefaultToolDocTemplates].
//
// These fields are recipes, not template variables. Do not write
// {{.ToolDoc.Title}} — write {{.Title}} to insert Arazzo info.title.
type ToolDocTemplates struct {
	Name             string
	Title            string
	Description      string
	QueryName        string
	QueryTitle       string
	QueryDescription string
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
		Name:             `run_{{.SafePlanID}}_v{{.SafeVersion}}`,
		Title:            `{{.Title}} ({{.PlanID}} {{.VersionSegment}})`,
		Description:      defaultDescriptionTemplate,
		QueryName:        "query",
		QueryTitle:       "Query plans",
		QueryDescription: "Match a simple natural-language request plus inputs against Arazzo plans, then execute the selected plan. Same contract as POST {{.RESTQueryURL}}.",
	}
}

const defaultDescriptionTemplate = `{{.Title}}

Plan ID: {{.PlanID}}
Arazzo version: {{.Version}}
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
{{end}}REST equivalent:
This MCP tool can also be invoked with an HTTP POST. The JSON body is the same object as the MCP inputs argument. Content-Type: application/json.
- This version: POST {{.RESTExecuteVersionedURL}}
- Latest version of this plan: POST {{.RESTExecuteLatestURL}}
Replace {workflowId} with the workflowId you would pass to this tool.

OpenAPI for these REST resources:
- This version: GET {{.OpenAPIVersionedURL}}
- Latest: GET {{.OpenAPILatestURL}}
`

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
	if t.QueryName == "" {
		t.QueryName = d.QueryName
	}
	if t.QueryTitle == "" {
		t.QueryTitle = d.QueryTitle
	}
	if t.QueryDescription == "" {
		t.QueryDescription = d.QueryDescription
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

// RenderToolDoc executes name, title, and description templates.
func RenderToolDoc(tmpls ToolDocTemplates, ctx ToolDocContext) (name, title, description string, err error) {
	tmpls = MergeTemplates(tmpls)
	name, err = execTemplate("name", tmpls.Name, ctx)
	if err != nil {
		return "", "", "", err
	}
	name = SanitizeToolName(name)
	if name == "" {
		return "", "", "", errEmptyToolName
	}
	title, err = execTemplate("title", tmpls.Title, ctx)
	if err != nil {
		return "", "", "", err
	}
	description, err = execTemplate("description", tmpls.Description, ctx)
	if err != nil {
		return "", "", "", err
	}
	return name, strings.TrimSpace(title), strings.TrimSpace(description), nil
}

// RenderQueryDoc executes query name, title, and description templates.
func RenderQueryDoc(tmpls ToolDocTemplates, ctx ToolDocContext) (name, title, description string, err error) {
	tmpls = MergeTemplates(tmpls)
	name, err = execTemplate("queryName", tmpls.QueryName, ctx)
	if err != nil {
		return "", "", "", err
	}
	name = SanitizeToolName(name)
	if name == "" {
		return "", "", "", errEmptyToolName
	}
	title, err = execTemplate("queryTitle", tmpls.QueryTitle, ctx)
	if err != nil {
		return "", "", "", err
	}
	description, err = execTemplate("queryDescription", tmpls.QueryDescription, ctx)
	if err != nil {
		return "", "", "", err
	}
	return name, strings.TrimSpace(title), strings.TrimSpace(description), nil
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
