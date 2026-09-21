// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mevansam/context-mesh-engine/arazzo"
	high "github.com/pb33f/libopenapi/datamodel/high/arazzo"
	"github.com/pb33f/libopenapi/orderedmap"
	"go.yaml.in/yaml/v4"
)

func plansDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", "arazzo", "plans")
}

func inlinePlanSource(t *testing.T, data []byte) arazzo.Source {
	t.Helper()
	dir, err := filepath.Abs(plansDir(t))
	if err != nil {
		t.Fatal(err)
	}
	base := (&url.URL{Scheme: "file", Path: filepath.ToSlash(dir) + "/"}).String()
	return arazzo.Source{
		URI:     filepath.Join(dir, "inline.yaml"),
		Data:    data,
		BaseURL: base,
	}
}

const inlinePlanPrefix = `arazzo: 1.0.1
info:
  title: t
  version: "1.0.0"
  x-planId: p
sourceDescriptions:
  - name: petstoreApi
    url: ../sources/openapi.yaml
    type: openapi
`

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func loadPetstore(t *testing.T) *Catalog {
	t.Helper()
	c, err := Load(context.Background(), []arazzo.Loader{arazzo.NewFileLoader(plansDir(t))}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLoad_SkipsMissingPlanIDAndPicksLatest(t *testing.T) {
	c := loadPetstore(t)
	if n := len(c.Entries()); n != 2 {
		t.Fatalf("entries = %d, want 2", n)
	}
	e, ok := c.Latest("petstore")
	if !ok {
		t.Fatal("missing latest petstore")
	}
	if e.Version != "1.1.0" {
		t.Fatalf("latest version = %q, want 1.1.0", e.Version)
	}
	if _, ok := c.Get("petstore", "1.0.0"); !ok {
		t.Fatal("missing 1.0.0")
	}
	if _, ok := c.GetBySegment("petstore", "v1.0.0"); !ok {
		t.Fatal("GetBySegment v1.0.0")
	}
	ids := e.WorkflowIDs()
	if len(ids) != 2 {
		t.Fatalf("latest workflows = %v", ids)
	}
}

func TestLoad_DuplicatePlanVersion(t *testing.T) {
	loader := arazzo.NewFileLoader(plansDir(t))
	_, err := Load(context.Background(), []arazzo.Loader{loader, loader}, discardLogger())
	if err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestLoad_InvalidInfoVersion(t *testing.T) {
	load := func(version string) error {
		_, err := Load(context.Background(), []arazzo.Loader{errLoader{srcs: []arazzo.Source{{
			URI:  "plan.yaml",
			Data: []byte("arazzo: 1.0.1\ninfo:\n  title: t\n  version: \"" + version + "\"\n  x-planId: p\nworkflows:\n  - workflowId: ping\n    steps:\n      - stepId: s\n        operationId: op\n        successCriteria:\n          - condition: $statusCode == 200\n"),
		}}}}, discardLogger())
		return err
	}
	if err := load("v1.0.0"); err == nil || !strings.Contains(err.Error(), `info.version "v1.0.0"`) {
		t.Fatalf("prefixed: %v", err)
	}
	if err := load("beta"); err == nil || !strings.Contains(err.Error(), `info.version "beta"`) {
		t.Fatalf("non-semver: %v", err)
	}
	if err := checkPlanVersion("1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := checkPlanVersion("1.0.0-beta.1"); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_NestedXSourceFails(t *testing.T) {
	src := []byte(inlinePlanPrefix + `
workflows:
  - workflowId: ping
    inputs:
      type: object
      properties:
        meta:
          type: object
          properties:
            requestId:
              type: string
              x-source:
                interface: rest
                in: header
    steps:
      - stepId: s
        operationId: getHealth
        successCriteria:
          - condition: $statusCode == 200
`)
	_, err := Load(context.Background(), []arazzo.Loader{errLoader{srcs: []arazzo.Source{inlinePlanSource(t, src)}}}, discardLogger())
	if err == nil || !strings.Contains(err.Error(), "x-source is only allowed on top-level properties") {
		t.Fatalf("nested x-source: %v", err)
	}
}

func TestLoad_OutputSourceInMustBeHeader(t *testing.T) {
	doc := func(inLine string) []byte {
		return []byte(inlinePlanPrefix + `
workflows:
  - workflowId: ping
    steps:
      - stepId: s
        operationId: getHealth
        successCriteria:
          - condition: $statusCode == 200
        outputs:
          orderId: $statusCode
    outputs:
      orderId: $steps.s.outputs.orderId
    x-outputs:
      type: object
      properties:
        orderId:
          type: string
          x-source:
            interface: rest
` + inLine)
	}
	for _, tc := range []struct {
		name string
		src  []byte
	}{
		{name: "query", src: doc("            in: query\n")},
		{name: "cookie", src: doc("            in: cookie\n")},
		{name: "path", src: doc("            in: path\n")},
		{name: "missing", src: doc("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(context.Background(), []arazzo.Loader{errLoader{srcs: []arazzo.Source{inlinePlanSource(t, tc.src)}}}, discardLogger())
			if err == nil || !strings.Contains(err.Error(), "x-source.in must be header") {
				t.Fatalf("output in %s: %v", tc.name, err)
			}
		})
	}
}

func TestLoad_InvalidXSecurity(t *testing.T) {
	src := []byte(inlinePlanPrefix + `
workflows:
  - workflowId: ping
    x-security:
      planOAuth: [pets:read]
    steps:
      - stepId: s
        operationId: getHealth
        successCriteria:
          - condition: $statusCode == 200
`)
	_, err := Load(context.Background(), []arazzo.Loader{errLoader{srcs: []arazzo.Source{inlinePlanSource(t, src)}}}, discardLogger())
	if err == nil || !strings.Contains(err.Error(), "x-security must be a sequence") {
		t.Fatalf("x-security object: %v", err)
	}
}

func TestPickLatest(t *testing.T) {
	if got := pickLatest([]string{"1.0.0", "1.1.0", "1.0.1"}); got != "1.1.0" {
		t.Fatalf("semver latest = %q", got)
	}
	if got := pickLatest([]string{"beta", "alpha"}); got != "beta" {
		t.Fatalf("lex latest = %q", got)
	}
	if got := pickLatest(nil); got != "" {
		t.Fatalf("empty = %q", got)
	}
}

type errLoader struct {
	err  error
	srcs []arazzo.Source
}

func (e errLoader) Load(context.Context) ([]arazzo.Source, error) {
	return e.srcs, e.err
}

func TestLoad_LoaderAndParseErrors(t *testing.T) {
	_, err := Load(context.Background(), []arazzo.Loader{errLoader{err: errors.New("boom")}}, discardLogger())
	if err == nil || err.Error() != "boom" {
		t.Fatalf("loader err = %v", err)
	}
	_, err = Load(context.Background(), []arazzo.Loader{errLoader{srcs: []arazzo.Source{{
		URI:  "bad.yaml",
		Data: []byte("not: [valid"),
	}}}}, discardLogger())
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestLoad_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Load(ctx, []arazzo.Loader{arazzo.NewFileLoader(plansDir(t))}, discardLogger())
	if err == nil {
		t.Fatal("expected cancelled context error")
	}
}

func TestCatalog_MissesAndNilView(t *testing.T) {
	c := loadPetstore(t)
	if _, ok := c.GetBySegment("petstore", "1.0.0"); ok {
		t.Fatal("segment without v prefix should miss")
	}
	if _, ok := c.GetBySegment("petstore", "v"); ok {
		t.Fatal("empty raw version should miss")
	}
	if _, ok := c.Latest("missing"); ok {
		t.Fatal("missing plan")
	}
	e, ok := c.Get("petstore", "1.1.0")
	if !ok {
		t.Fatal("1.1.0")
	}
	if e.VersionSegment() != "v1.1.0" {
		t.Fatalf("segment = %q", e.VersionSegment())
	}
	var nilCat *Catalog
	view := nilCat.View()
	if _, ok := view.Get("petstore", "1.0.0"); ok {
		t.Fatal("nil catalog Get")
	}
	if _, ok := view.Latest("petstore"); ok {
		t.Fatal("nil catalog Latest")
	}
	for range view.Plans() {
		t.Fatal("nil catalog Plans")
	}
}

type stubExec struct{ n int }

func (s *stubExec) Execute(_ context.Context, _ *arazzo.ExecutionRequest) (*arazzo.ExecutionResponse, error) {
	s.n++
	return &arazzo.ExecutionResponse{StatusCode: 200, Body: map[string]any{"ok": true}}, nil
}

type statusExec struct{ code int }

func (s statusExec) Execute(_ context.Context, _ *arazzo.ExecutionRequest) (*arazzo.ExecutionResponse, error) {
	return &arazzo.ExecutionResponse{StatusCode: s.code}, nil
}

func TestRunner_RunAndNoExecutor(t *testing.T) {
	c := loadPetstore(t)
	r := NewRunner(c, nil, nil)
	_, err := r.Run(context.Background(), "petstore", "1.1.0", "pingHealth", map[string]any{})
	if err != ErrNoExecutor {
		t.Fatalf("err = %v, want ErrNoExecutor", err)
	}

	exec := &stubExec{}
	r = NewRunner(c, exec, nil)
	res, err := r.Run(context.Background(), "petstore", "1.1.0", "pingHealth", map[string]any{"name": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("expected outputs map")
	}
	if exec.n != 1 {
		t.Fatalf("executor calls = %d", exec.n)
	}

	_, err = r.Run(context.Background(), "nope", "1.0.0", "pingHealth", nil)
	if err == nil {
		t.Fatal("expected not found")
	}

	_, err = r.Run(context.Background(), "petstore", "1.1.0", "noSuchWorkflow", nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown workflow err = %v", err)
	}

	fail := NewRunner(c, statusExec{code: 500}, nil)
	_, err = fail.Run(context.Background(), "petstore", "1.1.0", "pingHealth", map[string]any{"name": "x"})
	if err == nil {
		t.Fatal("expected workflow failure")
	}
}

func TestRunner_RejectsUnexpectedInputs(t *testing.T) {
	c := loadPetstore(t)
	r := NewRunner(c, &stubExec{}, nil)
	_, err := r.Run(context.Background(), "petstore", "1.1.0", "pingHealth", map[string]any{
		"name":                "x",
		arazzo.PolicyHintsKey: map[string]any{"mode": "forged"},
	})
	if !errors.Is(err, ErrUnexpectedInputs) {
		t.Fatalf("policyHints err = %v", err)
	}
	_, err = r.Run(context.Background(), "petstore", "1.1.0", "pingHealth", map[string]any{
		"name":  "x",
		"extra": 1,
	})
	if !errors.Is(err, ErrUnexpectedInputs) {
		t.Fatalf("extra err = %v", err)
	}
}

func TestQueryEnabled_NilRunner(t *testing.T) {
	var r *Runner
	if r.QueryEnabled() {
		t.Fatal("nil runner should not be query-enabled")
	}
}

func TestRunner_QueryStub(t *testing.T) {
	c := loadPetstore(t)
	r := NewRunner(c, &stubExec{}, nil)
	if r.Catalog() != c {
		t.Fatal("Catalog mismatch")
	}
	if r.QueryEnabled() {
		t.Fatal("QueryEnabled with nil matcher")
	}
	_, err := r.Query(context.Background(), "check health", map[string]any{"name": "x"})
	if err != ErrQueryNotImplemented {
		t.Fatalf("err = %v, want ErrQueryNotImplemented", err)
	}
}

type staticMatcher struct {
	match *arazzo.QueryMatch
	err   error
}

func (s staticMatcher) Match(_ context.Context, _ arazzo.QueryRequest) (*arazzo.QueryMatch, error) {
	return s.match, s.err
}

func TestRunner_QueryMatcher(t *testing.T) {
	c := loadPetstore(t)
	exec := &stubExec{}

	r := NewRunner(c, exec, staticMatcher{match: &arazzo.QueryMatch{
		PlanID:     "petstore",
		WorkflowID: "pingHealth",
		Inputs:     map[string]any{"name": "from-match"},
	}})
	out, err := r.Query(context.Background(), "is the api up", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		t.Fatal("expected outputs")
	}
	if exec.n != 1 {
		t.Fatalf("executor calls = %d", exec.n)
	}

	r = NewRunner(c, exec, staticMatcher{match: &arazzo.QueryMatch{
		PlanID:     "petstore",
		Version:    "1.0.0",
		WorkflowID: "pingHealth",
	}})
	if _, err := r.Query(context.Background(), "v1", map[string]any{"name": "x"}); err != nil {
		t.Fatal(err)
	}

	r = NewRunner(c, exec, staticMatcher{match: &arazzo.QueryMatch{
		PlanID:     "unloaded",
		Version:    "9.9.9",
		WorkflowID: "pingHealth",
	}})
	_, err = r.Query(context.Background(), "global hit", nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("unloaded plan err = %v, want ErrNotFound", err)
	}

	r = NewRunner(c, exec, staticMatcher{match: &arazzo.QueryMatch{
		PlanID:     "petstore",
		WorkflowID: "noSuchWorkflow",
	}})
	_, err = r.Query(context.Background(), "bad workflow", nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("bad workflow err = %v, want ErrNotFound", err)
	}

	r = NewRunner(c, exec, staticMatcher{})
	_, err = r.Query(context.Background(), "nothing", nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("nil match err = %v, want ErrNotFound", err)
	}

	r = NewRunner(c, exec, staticMatcher{match: &arazzo.QueryMatch{PlanID: "petstore", WorkflowID: "pingHealth"}})
	_, err = r.Query(context.Background(), "   ", nil)
	if !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("empty query err = %v, want ErrEmptyQuery", err)
	}

	r = NewRunner(c, exec, staticMatcher{err: errors.New("index down")})
	_, err = r.Query(context.Background(), "anything", nil)
	if err == nil || err.Error() != "index down" {
		t.Fatalf("matcher err = %v, want index down", err)
	}
}

func TestCatalog_View(t *testing.T) {
	c := loadPetstore(t)
	view := c.View()
	s, ok := view.Latest("petstore")
	if !ok || s.PlanID != "petstore" || s.Version != "1.1.0" {
		t.Fatalf("latest = %+v ok=%v", s, ok)
	}
	s, ok = view.Get("petstore", "1.0.0")
	if !ok || s.Version != "1.0.0" {
		t.Fatalf("get 1.0.0 = %+v ok=%v", s, ok)
	}
	n := 0
	for range view.Plans() {
		n++
	}
	if n != 2 {
		t.Fatalf("Plans count = %d, want 2", n)
	}
}

func TestInputSchema_OneOf(t *testing.T) {
	c := loadPetstore(t)
	e, ok := c.Get("petstore", "1.1.0")
	if !ok {
		t.Fatal("missing 1.1.0")
	}
	s, err := InputSchema(e.Doc)
	if err != nil {
		t.Fatal(err)
	}
	if s.Type != "object" {
		t.Fatalf("type = %q", s.Type)
	}
	if len(s.OneOf) != 2 {
		t.Fatalf("oneOf = %d, want 2", len(s.OneOf))
	}
}

func TestInputSchema_EmptyAndNilNode(t *testing.T) {
	s, err := InputSchema(&high.Arazzo{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.OneOf) != 1 || s.OneOf[0].Type != "object" {
		t.Fatalf("empty workflows schema = %#v", s)
	}
	ns, err := nodeToSchema(nil)
	if err != nil || ns.Type != "object" {
		t.Fatalf("nil node schema = %#v err=%v", ns, err)
	}
	v, err := nodeToJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := v.(map[string]any)
	if m["type"] != "object" {
		t.Fatalf("nil node json = %#v", v)
	}
	if m["additionalProperties"] != false {
		t.Fatalf("nil node should be closed: %#v", v)
	}
}

func yamlMapping(t *testing.T, src string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return &doc
}

func TestNodeToJSON_StripsReservedInputs(t *testing.T) {
	n := yamlMapping(t, `
type: object
required: [status, policyHints, secrets]
properties:
  status:
    type: string
  policyHints:
    type: object
    additionalProperties: true
  secrets:
    type: object
  policyHints.petStatus:
    type: string
  vault:
    type: object
    properties:
      secrets:
        type: array
      token:
        type: string
oneOf:
  - properties:
      policyHints:
        type: object
      name:
        type: string
`)
	v, err := nodeToJSON(n)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := v.(map[string]any)
	props, _ := m["properties"].(map[string]any)
	if _, ok := props["status"]; !ok {
		t.Fatalf("status missing: %#v", props)
	}
	for _, k := range []string{"policyHints", "secrets", "policyHints.petStatus"} {
		if _, ok := props[k]; ok {
			t.Fatalf("leaked %s: %#v", k, props)
		}
	}
	vault, _ := props["vault"].(map[string]any)
	vprops, _ := vault["properties"].(map[string]any)
	if _, ok := vprops["secrets"]; !ok {
		t.Fatalf("nested secrets should remain: %#v", vprops)
	}
	req, _ := m["required"].([]any)
	if len(req) != 1 || req[0] != "status" {
		t.Fatalf("required = %#v", req)
	}
	oneOf, _ := m["oneOf"].([]any)
	branch, _ := oneOf[0].(map[string]any)
	bprops, _ := branch["properties"].(map[string]any)
	if _, ok := bprops["policyHints"]; ok {
		t.Fatalf("oneOf leaked policyHints: %#v", bprops)
	}
	if _, ok := bprops["name"]; !ok {
		t.Fatalf("oneOf name missing: %#v", bprops)
	}
	if m["additionalProperties"] != false {
		t.Fatalf("root should be closed: %#v", m)
	}
	if branch["additionalProperties"] != false {
		t.Fatalf("oneOf branch should be closed: %#v", branch)
	}
	if _, ok := vault["additionalProperties"]; ok {
		t.Fatalf("nested vault should not be force-closed: %#v", vault)
	}
}

func TestInputSchema_StripsReservedInputs(t *testing.T) {
	n := yamlMapping(t, `
type: object
properties:
  status:
    type: string
  policyHints:
    type: object
`)
	s, err := InputSchema(&high.Arazzo{
		Workflows: []*high.Workflow{{
			WorkflowId: "retrievePet",
			Inputs:     n,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	in := s.OneOf[0].Properties["inputs"]
	if _, ok := in.Properties["status"]; !ok {
		t.Fatalf("status missing: %#v", in.Properties)
	}
	if _, ok := in.Properties["policyHints"]; ok {
		t.Fatalf("leaked policyHints: %#v", in.Properties)
	}
	if in.AdditionalProperties == nil || in.AdditionalProperties.Not == nil {
		t.Fatal("inputs schema should be closed")
	}
	if s.OneOf[0].AdditionalProperties == nil || s.OneOf[0].AdditionalProperties.Not == nil {
		t.Fatal("oneOf branch should be closed")
	}
}

func TestOpenAPIJSON_OmitsReservedInputsAndText(t *testing.T) {
	n := yamlMapping(t, `
type: object
properties:
  status:
    type: string
  policyHints:
    type: object
    additionalProperties: true
`)
	e := &Entry{
		PlanID:  "petstore",
		Version: "0.0.1",
		Doc: &high.Arazzo{
			Workflows: []*high.Workflow{{
				WorkflowId:  "retrievePet",
				Summary:     "Find a pet by status",
				Description: "Availability comes from $inputs.policyHints.petStatus",
				Inputs:      n,
			}},
		},
	}
	b, err := OpenAPIJSON(e, true, OpenAPIMeta{})
	if err != nil {
		t.Fatal(err)
	}
	raw := string(b)
	if strings.Contains(raw, "policyHints") {
		t.Fatalf("OpenAPI leaked policyHints: %s", raw)
	}
	if !strings.Contains(raw, `"status"`) {
		t.Fatalf("missing status: %s", raw)
	}
	if !strings.Contains(raw, `"Find a pet by status"`) {
		t.Fatalf("missing summary: %s", raw)
	}
	if !strings.Contains(raw, `"additionalProperties":false`) {
		t.Fatalf("request schema should be closed: %s", raw)
	}
}

func TestNodeToJSON_StripsXSource(t *testing.T) {
	n := yamlMapping(t, `
type: object
properties:
  requestId:
    type: string
    x-source:
      interface: rest
      protocol: http
      in: header
      name: x-request-id
  status:
    type: string
`)
	v, err := nodeToJSON(n)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := v.(map[string]any)
	props, _ := m["properties"].(map[string]any)
	reqID, _ := props["requestId"].(map[string]any)
	if _, ok := reqID["x-source"]; ok {
		t.Fatalf("MCP schema leaked x-source: %#v", reqID)
	}
	if reqID["type"] != "string" {
		t.Fatalf("requestId = %#v", reqID)
	}
	if _, ok := props["status"]; !ok {
		t.Fatalf("status missing: %#v", props)
	}
}

func TestInputSchema_KeepsRESTSourcedProperties(t *testing.T) {
	n := yamlMapping(t, `
type: object
properties:
  requestId:
    type: string
    x-source:
      interface: rest
      protocol: http
      in: header
      name: x-request-id
`)
	s, err := InputSchema(&high.Arazzo{
		Workflows: []*high.Workflow{{
			WorkflowId: "retrievePet",
			Inputs:     n,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	in := s.OneOf[0].Properties["inputs"]
	if _, ok := in.Properties["requestId"]; !ok {
		t.Fatalf("requestId missing from MCP schema: %#v", in.Properties)
	}
}

func TestOpenAPIJSON_MapsRESTSource(t *testing.T) {
	n := yamlMapping(t, `
type: object
required: [status, note]
properties:
  requestId:
    type: string
    x-source:
      interface: rest
      protocol: http
      in: header
      name: x-request-id
  session:
    type: string
    x-source:
      interface: rest
      in: cookie
      name: sid
  status:
    type: string
    enum: [available, pending, sold]
    x-source:
      interface: rest
      protocol: http
      in: query
  note:
    type: string
  mcpOnly:
    type: string
    x-source:
      interface: mcp
      protocol: http
      in: header
      name: x-mcp
`)
	e := &Entry{
		PlanID:  "petstore",
		Version: "0.0.1",
		Doc: &high.Arazzo{
			Workflows: []*high.Workflow{{
				WorkflowId: "retrievePet",
				Inputs:     n,
			}},
		},
	}
	b, err := OpenAPIJSON(e, true, OpenAPIMeta{})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	paths, _ := doc["paths"].(map[string]any)
	item, ok := paths["/tools/petstore/retrievePet"].(map[string]any)
	if !ok {
		t.Fatalf("missing retrievePet path: %s", b)
	}
	post, _ := item["post"].(map[string]any)
	params, _ := post["parameters"].([]any)
	if len(params) != 3 {
		t.Fatalf("parameters = %#v", params)
	}
	got := map[string]map[string]any{}
	for _, p := range params {
		pm, _ := p.(map[string]any)
		name, _ := pm["name"].(string)
		got[name] = pm
	}
	if got["x-request-id"]["in"] != "header" || got["x-request-id"]["required"] != false {
		t.Fatalf("header = %#v", got["x-request-id"])
	}
	if got["sid"]["in"] != "cookie" {
		t.Fatalf("cookie = %#v", got["sid"])
	}
	if got["status"]["in"] != "query" || got["status"]["required"] != true {
		t.Fatalf("query = %#v", got["status"])
	}
	schema, _ := got["status"]["schema"].(map[string]any)
	if _, ok := schema["x-source"]; ok {
		t.Fatalf("parameter schema leaked x-source: %#v", schema)
	}
	rb, _ := post["requestBody"].(map[string]any)
	content, _ := rb["content"].(map[string]any)
	app, _ := content["application/json"].(map[string]any)
	rschema, _ := app["schema"].(map[string]any)
	rprops, _ := rschema["properties"].(map[string]any)
	if _, ok := rprops["note"]; !ok {
		t.Fatalf("body missing note: %#v", rschema)
	}
	if _, ok := rprops["mcpOnly"]; !ok {
		t.Fatalf("mcp source should stay in body: %#v", rschema)
	}
	mcpOnly, _ := rprops["mcpOnly"].(map[string]any)
	if _, ok := mcpOnly["x-source"]; ok {
		t.Fatalf("body leaked x-source: %#v", mcpOnly)
	}
	for _, k := range []string{"requestId", "session", "status"} {
		if _, ok := rprops[k]; ok {
			t.Fatalf("lifted %s still in body: %#v", k, rschema)
		}
	}
	req, _ := rschema["required"].([]any)
	if len(req) != 1 || req[0] != "note" {
		t.Fatalf("body required = %#v", req)
	}
	c := &Catalog{
		byKey:  map[string]*Entry{key(e.PlanID, e.Version): e},
		latest: map[string]string{e.PlanID: e.Version},
	}
	cb, err := CatalogOpenAPIJSON(c, false, OpenAPIMeta{})
	if err != nil {
		t.Fatal(err)
	}
	var cdoc map[string]any
	if err := json.Unmarshal(cb, &cdoc); err != nil {
		t.Fatal(err)
	}
	cpaths, _ := cdoc["paths"].(map[string]any)
	refItem, _ := cpaths["/tools/petstore/retrievePet"].(map[string]any)
	want := "/api/openapi/petstore#/paths/~1tools~1petstore~1retrievePet"
	if refItem["$ref"] != want {
		t.Fatalf("catalog $ref = %v, want %q", refItem["$ref"], want)
	}
}

func TestOpenAPIJSON_OmitsRequestBodyWhenAllLifted(t *testing.T) {
	n := yamlMapping(t, `
type: object
properties:
  status:
    type: string
    x-source:
      interface: rest
      protocol: http
      in: query
`)
	e := &Entry{
		PlanID:  "petstore",
		Version: "0.0.1",
		Doc: &high.Arazzo{
			Workflows: []*high.Workflow{{
				WorkflowId: "getPet",
				Inputs:     n,
			}},
		},
	}
	b, err := OpenAPIJSON(e, true, OpenAPIMeta{})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	paths, _ := doc["paths"].(map[string]any)
	item, _ := paths["/tools/petstore/getPet"].(map[string]any)
	post, _ := item["post"].(map[string]any)
	if _, ok := post["requestBody"]; ok {
		t.Fatalf("requestBody should be omitted: %s", b)
	}
}

func TestOpenAPIJSON_InvalidSourceStaysInBody(t *testing.T) {
	n := yamlMapping(t, `
type: object
properties:
  badIn:
    type: string
    x-source:
      interface: rest
      protocol: http
      in: body
  pathIn:
    type: string
    x-source:
      interface: rest
      protocol: http
      in: path
  badProto:
    type: string
    x-source:
      interface: rest
      protocol: grpc
      in: header
`)
	e := &Entry{
		PlanID:  "petstore",
		Version: "0.0.1",
		Doc: &high.Arazzo{
			Workflows: []*high.Workflow{{
				WorkflowId: "retrievePet",
				Inputs:     n,
			}},
		},
	}
	b, err := OpenAPIJSON(e, true, OpenAPIMeta{})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	paths, _ := doc["paths"].(map[string]any)
	item, _ := paths["/tools/petstore/retrievePet"].(map[string]any)
	post, _ := item["post"].(map[string]any)
	if _, ok := post["parameters"]; ok {
		t.Fatalf("invalid sources should not lift: %s", b)
	}
	rb, _ := post["requestBody"].(map[string]any)
	content, _ := rb["content"].(map[string]any)
	app, _ := content["application/json"].(map[string]any)
	schema, _ := app["schema"].(map[string]any)
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["badIn"]; !ok {
		t.Fatalf("badIn missing: %#v", schema)
	}
	if _, ok := props["pathIn"]; !ok {
		t.Fatalf("pathIn missing: %#v", schema)
	}
	if _, ok := props["badProto"]; !ok {
		t.Fatalf("badProto missing: %#v", schema)
	}
}

func TestOutputsToJSONSchema(t *testing.T) {
	empty := outputsToJSONSchema(nil)
	if empty["type"] != "object" {
		t.Fatalf("empty = %#v", empty)
	}
	props := orderedmap.New[string, string]()
	props.Set("petId", "$steps.x.outputs.id")
	got := outputsToJSONSchema(props)
	p, _ := got["properties"].(map[string]any)
	if _, ok := p["petId"]; !ok {
		t.Fatalf("properties = %#v", got)
	}
}

func TestOpenAPIJSON_MapsOutputHeaders(t *testing.T) {
	outs := orderedmap.New[string, string]()
	outs.Set("orderId", "$steps.s.outputs.id")
	outs.Set("pet", "$steps.s.outputs.pet")
	ext := orderedmap.New[string, *yaml.Node]()
	ext.Set(outputSchemaExt, yamlMapping(t, `
type: object
required: [orderId]
properties:
  orderId:
    type: string
    x-source:
      interface: rest
      in: header
      name: x-order-id
  pet:
    type: object
`))
	e := &Entry{
		PlanID:  "petstore",
		Version: "0.0.1",
		Doc: &high.Arazzo{
			Workflows: []*high.Workflow{{
				WorkflowId: "purchasePet",
				Outputs:    outs,
				Extensions: ext,
			}},
		},
	}
	b, err := OpenAPIJSON(e, true, OpenAPIMeta{})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	paths, _ := doc["paths"].(map[string]any)
	item, _ := paths["/tools/petstore/purchasePet"].(map[string]any)
	post, _ := item["post"].(map[string]any)
	resp, _ := post["responses"].(map[string]any)
	okResp, _ := resp["200"].(map[string]any)
	headers, _ := okResp["headers"].(map[string]any)
	h, _ := headers["x-order-id"].(map[string]any)
	if h["required"] != true {
		t.Fatalf("header = %#v", h)
	}
	hs, _ := h["schema"].(map[string]any)
	if _, ok := hs["x-source"]; ok {
		t.Fatalf("header schema leaked x-source: %#v", h)
	}
	content, _ := okResp["content"].(map[string]any)
	app, _ := content["application/json"].(map[string]any)
	schema, _ := app["schema"].(map[string]any)
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["orderId"]; ok {
		t.Fatalf("lifted orderId still in body: %#v", schema)
	}
	if _, ok := props["pet"]; !ok {
		t.Fatalf("pet missing from body: %#v", schema)
	}
}

func TestWalkXSource_RootAndNested(t *testing.T) {
	n := yamlMapping(t, `
type: object
x-source:
  interface: rest
  in: header
`)
	if err := validateXSourcePlacement(n, "inputs"); err == nil {
		t.Fatal("expected root x-source error")
	}
	ok := yamlMapping(t, `
type: object
properties:
  requestId:
    type: string
    x-source:
      interface: rest
      in: header
`)
	if err := validateXSourcePlacement(ok, "inputs"); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeRESTCommonParams(t *testing.T) {
	_, err := NormalizeRESTCommonParams([]RESTCommonParam{{In: "header"}})
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("empty name: %v", err)
	}
	_, err = NormalizeRESTCommonParams([]RESTCommonParam{{Name: "X-End-User-Token", In: "query"}})
	if err == nil || !strings.Contains(err.Error(), "in must be header or cookie") {
		t.Fatalf("query: %v", err)
	}
	_, err = NormalizeRESTCommonParams([]RESTCommonParam{
		{Name: "X-End-User-Token", In: "header"},
		{Name: "x-end-user-token", In: "HEADER"},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("dup header: %v", err)
	}
	got, err := NormalizeRESTCommonParams([]RESTCommonParam{
		{Name: "X-End-User-Token", In: "header", Required: true, Description: "end user JWT"},
		{Name: "sid", In: "cookie"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].In != "header" || got[1].In != "cookie" {
		t.Fatalf("got = %#v", got)
	}
	if got[0].Schema["type"] != "string" || got[1].Schema["type"] != "string" {
		t.Fatalf("default schema = %#v", got)
	}
}

func TestCheckCommonParamCollisions(t *testing.T) {
	c := &Catalog{
		entries: []*Entry{{
			PlanID: "p",
			Doc: &high.Arazzo{
				Workflows: []*high.Workflow{{
					WorkflowId: "ping",
					Inputs: yamlMapping(t, `
type: object
properties:
  requestId:
    type: string
    x-source:
      interface: rest
      in: header
      name: x-request-id
`),
				}},
			},
		}},
	}
	err := CheckCommonParamCollisions(c, []RESTCommonParam{{Name: "X-Request-Id", In: "header"}})
	if err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("header collision: %v", err)
	}
	if err := CheckCommonParamCollisions(c, []RESTCommonParam{{Name: "x-request-id", In: "cookie"}}); err != nil {
		t.Fatalf("cookie vs header should not collide: %v", err)
	}
}

func TestOpenAPIJSON_CommonRESTParams(t *testing.T) {
	c := loadPetstore(t)
	latest, ok := c.Latest("petstore")
	if !ok {
		t.Fatal("latest")
	}
	meta := OpenAPIMeta{CommonParams: []RESTCommonParam{
		{Name: "X-End-User-Token", In: "header", Required: true, Description: "end-user JWT"},
		{Name: "sid", In: "cookie"},
	}}
	b, err := OpenAPIJSON(latest, true, meta)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	paths, _ := doc["paths"].(map[string]any)
	item, _ := paths["/tools/petstore/pingHealth"].(map[string]any)
	post, _ := item["post"].(map[string]any)
	params, _ := post["parameters"].([]any)
	h := findOpenAPIParam(params, "header", "X-End-User-Token")
	if h == nil || h["required"] != true || h["description"] != "end-user JWT" {
		t.Fatalf("header param = %#v", params)
	}
	if findOpenAPIParam(params, "cookie", "sid") == nil {
		t.Fatalf("cookie param = %#v", params)
	}

	cb, err := CatalogOpenAPIJSON(c, true, meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(cb, &doc); err != nil {
		t.Fatal(err)
	}
	paths, _ = doc["paths"].(map[string]any)
	tools, _ := paths["/tools"].(map[string]any)
	get, _ := tools["get"].(map[string]any)
	params, _ = get["parameters"].([]any)
	if findOpenAPIParam(params, "header", "X-End-User-Token") == nil {
		t.Fatalf("/tools missing common header: %#v", params)
	}
	if findOpenAPIParam(params, "query", "cursor") == nil {
		t.Fatalf("/tools missing cursor: %#v", params)
	}
	query, _ := paths["/plans/query"].(map[string]any)
	qpost, _ := query["post"].(map[string]any)
	params, _ = qpost["parameters"].([]any)
	if findOpenAPIParam(params, "cookie", "sid") == nil {
		t.Fatalf("query missing cookie: %#v", params)
	}

	s, err := InputSchema(latest.Doc)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "X-End-User-Token") || strings.Contains(string(raw), "sid") {
		t.Fatalf("MCP schema leaked common params: %s", raw)
	}
}

func TestOpenAPIJSON_Security(t *testing.T) {
	schemes := []SecurityScheme{{
		Name:        "planOAuth",
		Type:        "oauth2",
		Description: "Calling-application OAuth",
		Flows: &OAuthFlows{ClientCredentials: &OAuthFlow{
			TokenURL: "http://localhost:8092/oauth/token",
			Scopes: map[string]string{
				"pets:read":  "Find pets",
				"pets:write": "Purchase pets",
				"tools:list": "List tools",
			},
		}},
	}}
	fallback := []SecurityRequirement{{Schemes: []SchemeRef{{Name: "planOAuth", Scopes: []string{"tools:list"}}}}}
	ext := orderedmap.New[string, *yaml.Node]()
	ext.Set(workflowSecurityExt, yamlMapping(t, `
- planOAuth: [pets:read]
`))
	e := &Entry{
		PlanID:  "petstore",
		Version: "0.0.1",
		Doc: &high.Arazzo{
			Workflows: []*high.Workflow{
				{WorkflowId: "retrievePet", Extensions: ext},
				{WorkflowId: "pingHealth"},
			},
		},
	}
	b, err := OpenAPIJSON(e, true, OpenAPIMeta{SecuritySchemes: schemes, Security: fallback})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	comps, _ := doc["components"].(map[string]any)
	ss, _ := comps["securitySchemes"].(map[string]any)
	plan, _ := ss["planOAuth"].(map[string]any)
	if plan["type"] != "oauth2" {
		t.Fatalf("scheme = %#v", plan)
	}
	flows, _ := plan["flows"].(map[string]any)
	cc, _ := flows["clientCredentials"].(map[string]any)
	if cc["tokenUrl"] != "http://localhost:8092/oauth/token" {
		t.Fatalf("tokenUrl = %#v", cc)
	}
	if _, ok := doc["security"]; ok {
		t.Fatalf("child spec should not set document security: %#v", doc["security"])
	}
	paths, _ := doc["paths"].(map[string]any)
	retrieve, _ := paths["/tools/petstore/retrievePet"].(map[string]any)
	rpost, _ := retrieve["post"].(map[string]any)
	rsec, _ := rpost["security"].([]any)
	rm, _ := rsec[0].(map[string]any)
	if fmt.Sprint(rm["planOAuth"]) != "[pets:read]" {
		t.Fatalf("retrievePet security = %#v", rsec)
	}
	ping, _ := paths["/tools/petstore/pingHealth"].(map[string]any)
	ppost, _ := ping["post"].(map[string]any)
	psec, _ := ppost["security"].([]any)
	pm, _ := psec[0].(map[string]any)
	if fmt.Sprint(pm["planOAuth"]) != "[tools:list]" {
		t.Fatalf("pingHealth fallback security = %#v", psec)
	}

	cb, err := CatalogOpenAPIJSON(nil, true, OpenAPIMeta{SecuritySchemes: schemes, Security: fallback})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(cb, &doc); err != nil {
		t.Fatal(err)
	}
	sec, _ := doc["security"].([]any)
	sm, _ := sec[0].(map[string]any)
	if fmt.Sprint(sm["planOAuth"]) != "[tools:list]" {
		t.Fatalf("catalog security = %#v", sec)
	}
}

func findOpenAPIParam(params []any, in, name string) map[string]any {
	for _, p := range params {
		m, _ := p.(map[string]any)
		if m["in"] == in && m["name"] == name {
			return m
		}
	}
	return nil
}

func TestOpenAPIJSON_LatestAndVersioned(t *testing.T) {
	c := loadPetstore(t)
	latest, ok := c.Latest("petstore")
	if !ok {
		t.Fatal("latest")
	}
	b, err := OpenAPIJSON(latest, true, OpenAPIMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `/tools/petstore/pingHealth`) {
		t.Fatalf("latest paths: %s", b)
	}
	if strings.Contains(string(b), `"durationMs"`) || strings.Contains(string(b), `"workflowId"`) {
		t.Fatalf("200 schema should be workflow outputs, not the execution trace: %s", b)
	}
	if strings.Contains(string(b), `/tools/petstore/v1.1.0/`) {
		t.Fatalf("latest should not include version segment: %s", b)
	}
	if !strings.Contains(string(b), `"url":"/api"`) {
		t.Fatalf("latest missing default servers url: %s", b)
	}
	if !strings.Contains(string(b), `"additionalProperties":false`) {
		t.Fatalf("latest request schema should be closed: %s", b)
	}
	b, err = OpenAPIJSON(latest, false, OpenAPIMeta{ServerURL: "http://example.test/api"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `/tools/petstore/v1.1.0/echoName`) {
		t.Fatalf("versioned paths: %s", b)
	}
	if !strings.Contains(string(b), `"url":"http://example.test/api"`) {
		t.Fatalf("versioned missing servers url: %s", b)
	}
}

func TestCatalogOpenAPIJSON_ToolsAndPlanRefs(t *testing.T) {
	c := loadPetstore(t)
	b, err := CatalogOpenAPIJSON(c, false, OpenAPIMeta{})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["openapi"] != "3.1.0" {
		t.Fatalf("openapi = %v", doc["openapi"])
	}
	info, _ := doc["info"].(map[string]any)
	if info["title"] != "Arazzo plan catalog" {
		t.Fatalf("default catalog title = %v", info["title"])
	}
	if info["version"] != "1.0.0" {
		t.Fatalf("default catalog version = %v", info["version"])
	}
	named, err := CatalogOpenAPIJSON(c, false, OpenAPIMeta{
		CatalogTitle:   "API Execution Plan Catalog Demo",
		CatalogVersion: "0.0.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(named), `"title":"API Execution Plan Catalog Demo"`) {
		t.Fatalf("custom catalog title: %s", named)
	}
	if !strings.Contains(string(named), `"version":"0.0.1"`) {
		t.Fatalf("custom catalog version: %s", named)
	}
	paths, _ := doc["paths"].(map[string]any)
	if _, ok := paths["/tools"]; !ok {
		t.Fatalf("missing /tools: %s", b)
	}
	if _, ok := paths["/plans/query"]; ok {
		t.Fatalf("query path without queryEnabled: %s", b)
	}
	ping, _ := paths["/tools/petstore/pingHealth"].(map[string]any)
	ref, _ := ping["$ref"].(string)
	want := "/api/openapi/petstore#/paths/~1tools~1petstore~1pingHealth"
	if ref != want {
		t.Fatalf("pingHealth $ref = %q, want %q", ref, want)
	}
	echo, _ := paths["/tools/petstore/echoName"].(map[string]any)
	if echo["$ref"] != "/api/openapi/petstore#/paths/~1tools~1petstore~1echoName" {
		t.Fatalf("echoName $ref = %v", echo["$ref"])
	}
	if _, ok := paths["/tools/petstore/v1.1.0/echoName"]; ok {
		t.Fatalf("catalog must $ref latest paths only: %s", b)
	}
	comps, _ := doc["components"].(map[string]any)
	schemas, _ := comps["schemas"].(map[string]any)
	ltr, _ := schemas["ListToolsResult"].(map[string]any)
	props, _ := ltr["properties"].(map[string]any)
	for _, name := range []string{"ttlMs", "cacheScope", "tools"} {
		if _, ok := props[name]; !ok {
			t.Fatalf("ListToolsResult missing %s: %#v", name, props)
		}
	}
	if _, ok := props["_meta"]; ok {
		t.Fatalf("ListToolsResult schema must not include _meta: %#v", props)
	}
	if _, ok := props["resultType"]; ok {
		t.Fatalf("ListToolsResult schema must not include resultType: %#v", props)
	}
	tools, _ := props["tools"].(map[string]any)
	items, _ := tools["items"].(map[string]any)
	itemProps, _ := items["properties"].(map[string]any)
	if _, ok := itemProps["inputSchema"]; !ok {
		t.Fatalf("Tool missing inputSchema: %#v", itemProps)
	}
	if _, ok := itemProps["name"]; !ok {
		t.Fatalf("Tool missing name: %#v", itemProps)
	}
	if _, ok := itemProps["_meta"]; ok {
		t.Fatalf("Tool schema must not include _meta: %#v", itemProps)
	}

	b, err = CatalogOpenAPIJSON(c, true, OpenAPIMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"/plans/query"`) {
		t.Fatalf("queryEnabled missing /plans/query: %s", b)
	}

	b, err = CatalogOpenAPIJSON(nil, false, OpenAPIMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	paths, _ = doc["paths"].(map[string]any)
	if _, ok := paths["/tools"]; !ok {
		t.Fatal("nil catalog must still describe /tools")
	}
	if _, ok := paths["/tools/petstore/pingHealth"]; ok {
		t.Fatal("nil catalog must not $ref plans")
	}
}

func TestOpenAPIServerURL(t *testing.T) {
	if got := OpenAPIServerURL("", ""); got != "/api" {
		t.Fatalf("empty = %q", got)
	}
	if got := OpenAPIServerURL("http://localhost:8080", "/api"); got != "http://localhost:8080/api" {
		t.Fatalf("joined = %q", got)
	}
	if got := OpenAPIServerURL("http://example.test/", "/service/v2"); got != "http://example.test/service/v2" {
		t.Fatalf("custom = %q", got)
	}
}

func TestNativeOutput_YAMLMapping(t *testing.T) {
	n := &yaml.Node{}
	if err := n.Encode(map[string]any{"id": 1, "name": "Dog", "status": "available"}); err != nil {
		t.Fatal(err)
	}
	got, ok := nativeOutput(n).(map[string]any)
	if !ok {
		t.Fatalf("got %T", nativeOutput(n))
	}
	if got["name"] != "Dog" || got["status"] != "available" {
		t.Fatalf("pet = %#v", got)
	}
	out := nativeOutputs(nil)
	if len(out) != 0 {
		t.Fatalf("nil outputs = %#v", out)
	}
	var node yaml.Node
	if err := node.Encode("hello"); err != nil {
		t.Fatal(err)
	}
	if got := nativeOutput(node); got != "hello" {
		t.Fatalf("value node = %#v", got)
	}
}
