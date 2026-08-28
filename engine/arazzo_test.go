// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package engine_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mevansam/context-mesh-engine/arazzo"
	"github.com/mevansam/context-mesh-engine/engine"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type countingExec struct{ n int }

func (c *countingExec) Execute(_ context.Context, _ *arazzo.ExecutionRequest) (*arazzo.ExecutionResponse, error) {
	c.n++
	return &arazzo.ExecutionResponse{StatusCode: 200, Body: map[string]any{"ok": true}}, nil
}

func plansDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "testdata", "arazzo", "plans")
}

func newArazzoEngine(t *testing.T, exec arazzo.Executor) *engine.Engine {
	t.Helper()
	e, err := engine.New(engine.Options{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		ArazzoLoaders:  []arazzo.Loader{arazzo.NewFileLoader(plansDir(t))},
		ArazzoExecutor: exec,
		PublicBaseURL:  "http://example.test",
		DualMCPandREST: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestArazzo_InvalidTemplatesFailNew(t *testing.T) {
	_, err := engine.New(engine.Options{
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		ArazzoLoaders: []arazzo.Loader{arazzo.NewFileLoader(plansDir(t))},
		ToolDoc:       arazzo.ToolDocTemplates{Name: "{{.Nope"},
	})
	if err == nil {
		t.Fatal("expected template error")
	}
}

func TestArazzo_QueryTemplatesFailNewOnlyWithMatcher(t *testing.T) {
	bad := arazzo.ToolDocTemplates{QueryName: "{{.Nope"}
	_, err := engine.New(engine.Options{
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		ArazzoLoaders: []arazzo.Loader{arazzo.NewFileLoader(plansDir(t))},
		ToolDoc:       bad,
	})
	if err != nil {
		t.Fatalf("nil matcher should skip query templates: %v", err)
	}
	_, err = engine.New(engine.Options{
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		ArazzoLoaders: []arazzo.Loader{arazzo.NewFileLoader(plansDir(t))},
		QueryMatcher:  pingMatcher{planID: "petstore", workflowID: "pingHealth"},
		ToolDoc:       bad,
	})
	if err == nil {
		t.Fatal("expected query template error")
	}
}

func TestArazzo_InvalidRESTDescriptionFailNew(t *testing.T) {
	_, err := engine.New(engine.Options{
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		ArazzoLoaders: []arazzo.Loader{arazzo.NewFileLoader(plansDir(t))},
		ToolDoc:       arazzo.ToolDocTemplates{RESTDescription: "{{.Nope"},
	})
	if err == nil {
		t.Fatal("expected REST description template error")
	}
}

func TestArazzo_OpenAPIWithoutExecutor(t *testing.T) {
	e := newArazzoEngine(t, nil)
	ts := httptest.NewServer(e.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/api/openapi/petstore")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	paths, _ := doc["paths"].(map[string]any)
	if _, ok := paths["/plans/petstore/pingHealth"]; !ok {
		t.Fatalf("paths = %v", paths)
	}

	resp, err = http.Get(ts.URL + "/api/openapi")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("catalog status = %d", resp.StatusCode)
	}
	var catalog map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		t.Fatal(err)
	}
	cpaths, _ := catalog["paths"].(map[string]any)
	if _, ok := cpaths["/tools"]; !ok {
		t.Fatalf("catalog missing /tools: %v", cpaths)
	}
	ping, _ := cpaths["/plans/petstore/pingHealth"].(map[string]any)
	if ping["$ref"] != "/api/openapi/petstore#/paths/~1plans~1petstore~1pingHealth" {
		t.Fatalf("catalog pingHealth $ref = %v", ping["$ref"])
	}
	servers, _ := catalog["servers"].([]any)
	if len(servers) == 0 {
		t.Fatal("catalog missing servers")
	}
	s0, _ := servers[0].(map[string]any)
	if s0["url"] != "http://example.test/api" {
		t.Fatalf("catalog servers url = %v", s0["url"])
	}
	if _, ok := cpaths["/plans/query"]; ok {
		t.Fatal("catalog must omit /plans/query without QueryMatcher")
	}

	resp, err = http.Get(ts.URL + "/api/openapi/petstore/v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("versioned status = %d", resp.StatusCode)
	}

	resp, err = http.Post(ts.URL+"/api/plans/petstore/pingHealth", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("execute without executor status = %d, want 501", resp.StatusCode)
	}
}

func TestArazzo_RESTExecuteLatestAndVersioned(t *testing.T) {
	exec := &countingExec{}
	e := newArazzoEngine(t, exec)
	ts := httptest.NewServer(e.Handler())
	t.Cleanup(ts.Close)

	post := func(path, body string) map[string]any {
		t.Helper()
		resp, err := http.Post(ts.URL+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("%s status = %d body = %s", path, resp.StatusCode, b)
		}
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	latest := post("/api/plans/petstore/pingHealth", `{"name":"a"}`)
	if _, ok := latest["success"]; ok {
		t.Fatalf("REST body should be outputs only, got %v", latest)
	}
	ver := post("/api/plans/petstore/v1.0.0/pingHealth", `{"name":"b"}`)
	if _, ok := ver["success"]; ok {
		t.Fatalf("REST body should be outputs only, got %v", ver)
	}
	if exec.n != 2 {
		t.Fatalf("executor calls = %d, want 2", exec.n)
	}
}

func TestArazzo_MCPQueryStubAndRunTools(t *testing.T) {
	exec := &countingExec{}
	e := newArazzoEngine(t, exec)
	ts := httptest.NewServer(e.Handler())
	t.Cleanup(ts.Close)

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: ts.URL + engine.MCPPath,
	}, nil)
	if err != nil {
		t.Fatalf("MCP connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, tl := range tools.Tools {
		got[tl.Name] = true
		if strings.HasPrefix(tl.Name, "run_") {
			if strings.Contains(tl.Description, "POST ") || strings.Contains(tl.Description, "/api/plans/") {
				t.Fatalf("MCP tool %s description must not mention REST:\n%s", tl.Name, tl.Description)
			}
			if !strings.Contains(tl.Description, "How to call this MCP tool") {
				t.Fatalf("MCP tool %s description missing how-to:\n%s", tl.Name, tl.Description)
			}
		}
	}
	if got["query"] || !got["run_petstore_v1.0.0"] || !got["run_petstore_v1.1.0"] {
		t.Fatalf("tools = %v", got)
	}

	rest, err := http.Get(ts.URL + "/api/tools")
	if err != nil {
		t.Fatal(err)
	}
	defer rest.Body.Close()
	if rest.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/tools status = %d", rest.StatusCode)
	}
	var restBody mcp.ListToolsResult
	if err := json.NewDecoder(rest.Body).Decode(&restBody); err != nil {
		t.Fatal(err)
	}
	if len(restBody.Tools) != len(tools.Tools) {
		t.Fatalf("REST tools = %d, MCP tools/list = %d", len(restBody.Tools), len(tools.Tools))
	}
	for i, tl := range tools.Tools {
		if restBody.Tools[i].Name != tl.Name {
			t.Fatalf("tools[%d]: REST %q MCP %q", i, restBody.Tools[i].Name, tl.Name)
		}
		if strings.HasPrefix(tl.Name, "run_") {
			if !strings.Contains(restBody.Tools[i].Description, "POST http://example.test/api/plans/") {
				t.Fatalf("REST tool %s description missing REST URL:\n%s", tl.Name, restBody.Tools[i].Description)
			}
			if strings.Contains(strings.ToLower(restBody.Tools[i].Description), "mcp") {
				t.Fatalf("REST tool %s description must not mention MCP:\n%s", tl.Name, restBody.Tools[i].Description)
			}
		}
	}

	again, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range again.Tools {
		if strings.HasPrefix(tl.Name, "run_") && (strings.Contains(tl.Description, "POST ") || strings.Contains(tl.Description, "/api/plans/")) {
			t.Fatalf("MCP tool %s description mutated after GET /tools:\n%s", tl.Name, tl.Description)
		}
	}

	qresp, err := http.Post(ts.URL+"/api/plans/query", "application/json", strings.NewReader(`{"query":"hello","data":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer qresp.Body.Close()
	if qresp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(qresp.Body)
		t.Fatalf("REST query status = %d, want 404 body = %s", qresp.StatusCode, b)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "run_petstore_v1.1.0",
		Arguments: map[string]any{
			"workflowId": "pingHealth",
			"inputs":     map[string]any{"name": "mcp"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("run error: %s", toolErrorText(res))
	}
	sc, _ := res.StructuredContent.(map[string]any)
	if _, ok := sc["success"]; ok {
		t.Fatalf("structured content should be outputs only, got %#v", res.StructuredContent)
	}

	resp, err := http.Post(ts.URL+"/api/plans/petstore/v1.1.0/echoName", "application/json", bytes.NewReader([]byte(`{"name":"rest"}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("rest status = %d %s", resp.StatusCode, b)
	}
	if exec.n != 2 {
		t.Fatalf("shared runner executor calls = %d, want 2 (mcp+rest)", exec.n)
	}
}

func TestArazzo_CustomAPIPrefix(t *testing.T) {
	e, err := engine.New(engine.Options{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		ArazzoLoaders:  []arazzo.Loader{arazzo.NewFileLoader(plansDir(t))},
		ArazzoExecutor: &countingExec{},
		QueryMatcher: pingMatcher{
			planID:     "petstore",
			workflowID: "pingHealth",
		},
		PublicBaseURL:  "http://example.test",
		APIPrefix:      "/service/v2",
		DualMCPandREST: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(e.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/service/v2/openapi/petstore")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("custom openapi status = %d", resp.StatusCode)
	}

	cat, err := http.Get(ts.URL + "/service/v2/openapi")
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Body.Close()
	if cat.StatusCode != http.StatusOK {
		t.Fatalf("custom catalog status = %d", cat.StatusCode)
	}
	var catDoc map[string]any
	if err := json.NewDecoder(cat.Body).Decode(&catDoc); err != nil {
		t.Fatal(err)
	}
	catPaths, _ := catDoc["paths"].(map[string]any)
	ping, _ := catPaths["/plans/petstore/pingHealth"].(map[string]any)
	if ping["$ref"] != "/service/v2/openapi/petstore#/paths/~1plans~1petstore~1pingHealth" {
		t.Fatalf("custom prefix $ref = %v", ping["$ref"])
	}

	old, err := http.Get(ts.URL + "/api/openapi/petstore")
	if err != nil {
		t.Fatal(err)
	}
	defer old.Body.Close()
	if old.StatusCode != http.StatusNotFound {
		t.Fatalf("default openapi status = %d, want 404", old.StatusCode)
	}

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: ts.URL + engine.MCPPath,
	}, nil)
	if err != nil {
		t.Fatalf("MCP connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range tools.Tools {
		if tl.Name == "query" && (strings.Contains(tl.Description, "POST ") || strings.Contains(tl.Description, "/plans/query")) {
			t.Fatalf("MCP query description must not mention REST:\n%s", tl.Description)
		}
		if strings.HasPrefix(tl.Name, "run_") && (strings.Contains(tl.Description, "POST ") || strings.Contains(tl.Description, "/plans/")) {
			t.Fatalf("MCP tool %s description must not mention REST:\n%s", tl.Name, tl.Description)
		}
	}

	rest, err := http.Get(ts.URL + "/service/v2/tools")
	if err != nil {
		t.Fatal(err)
	}
	defer rest.Body.Close()
	if rest.StatusCode != http.StatusOK {
		t.Fatalf("GET /service/v2/tools status = %d", rest.StatusCode)
	}
	var restBody mcp.ListToolsResult
	if err := json.NewDecoder(rest.Body).Decode(&restBody); err != nil {
		t.Fatal(err)
	}
	for _, tl := range restBody.Tools {
		if strings.Contains(strings.ToLower(tl.Description), "mcp") {
			t.Fatalf("REST tool %s description must not mention MCP:\n%s", tl.Name, tl.Description)
		}
		if tl.Name == "query" && !strings.Contains(tl.Description, "POST http://example.test/service/v2/plans/query") {
			t.Fatalf("REST query description missing custom URL:\n%s", tl.Description)
		}
		if strings.HasPrefix(tl.Name, "run_") && !strings.Contains(tl.Description, "POST http://example.test/service/v2/plans/") {
			t.Fatalf("REST tool %s description missing custom URL:\n%s", tl.Name, tl.Description)
		}
	}
}

type pingMatcher struct {
	planID, version, workflowID string
}

func (p pingMatcher) Match(_ context.Context, req arazzo.QueryRequest) (*arazzo.QueryMatch, error) {
	return &arazzo.QueryMatch{
		PlanID:     p.planID,
		Version:    p.version,
		WorkflowID: p.workflowID,
		Inputs:     req.Data,
	}, nil
}

type recordingHelp struct {
	n atomic.Int32
}

func (r *recordingHelp) Lookup(_ context.Context, req arazzo.ToolHelpRequest) (*arazzo.ToolHelp, error) {
	r.n.Add(1)
	switch req.Kind {
	case arazzo.ToolHelpKindQuery:
		return &arazzo.ToolHelp{Title: "Ask", Description: "query-help"}, nil
	case arazzo.ToolHelpKindPlan:
		if req.PlanID == "petstore" && req.Version == "1.1.0" {
			return &arazzo.ToolHelp{
				Title:       "Custom {{.PlanID}}",
				Description: "plan-help-{{.Version}}",
			}, nil
		}
	}
	return nil, nil
}

func TestArazzo_ToolHelpLookupOnDemand(t *testing.T) {
	h := &recordingHelp{}
	e, err := engine.New(engine.Options{
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		ArazzoLoaders:    []arazzo.Loader{arazzo.NewFileLoader(plansDir(t))},
		ArazzoExecutor:   &countingExec{},
		QueryMatcher:     pingMatcher{planID: "petstore", workflowID: "pingHealth"},
		PublicBaseURL:    "http://example.test",
		ToolHelpLookup:   h,
		ToolHelpCacheTTL: time.Hour,
		DualMCPandREST:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := h.n.Load(); n != 0 {
		t.Fatalf("lookup during New = %d, want 0", n)
	}

	ts := httptest.NewServer(e.Handler())
	t.Cleanup(ts.Close)

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: ts.URL + engine.MCPPath,
	}, nil)
	if err != nil {
		t.Fatalf("MCP connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n := h.n.Load(); n != 3 {
		t.Fatalf("lookups after first list = %d, want 3 (query + 2 plans)", n)
	}
	byName := map[string]*mcp.Tool{}
	for _, tl := range tools.Tools {
		byName[tl.Name] = tl
	}
	q := byName["query"]
	if q == nil || q.Title != "Ask" || q.Description != "query-help" {
		t.Fatalf("query = %#v", q)
	}
	v11 := byName["run_petstore_v1.1.0"]
	if v11 == nil || v11.Title != "Custom petstore" || v11.Description != "plan-help-1.1.0" {
		t.Fatalf("v1.1.0 = %#v", v11)
	}
	v10 := byName["run_petstore_v1.0.0"]
	if v10 == nil || !strings.Contains(v10.Description, "How to call this MCP tool") {
		t.Fatalf("v1.0.0 default MCP desc missing: %#v", v10)
	}

	rest, err := http.Get(ts.URL + "/api/tools")
	if err != nil {
		t.Fatal(err)
	}
	defer rest.Body.Close()
	var restBody mcp.ListToolsResult
	if err := json.NewDecoder(rest.Body).Decode(&restBody); err != nil {
		t.Fatal(err)
	}
	if n := h.n.Load(); n != 3 {
		t.Fatalf("lookups after GET /tools = %d, want 3 (cached)", n)
	}
	restBy := map[string]*mcp.Tool{}
	for _, tl := range restBody.Tools {
		restBy[tl.Name] = tl
	}
	if restBy["query"] == nil || restBy["query"].Description != "query-help" {
		t.Fatalf("REST query = %#v", restBy["query"])
	}
	if restBy["run_petstore_v1.1.0"] == nil || restBy["run_petstore_v1.1.0"].Description != "plan-help-1.1.0" {
		t.Fatalf("REST v1.1.0 should reuse Description, got %#v", restBy["run_petstore_v1.1.0"])
	}
	if restBy["run_petstore_v1.0.0"] == nil || !strings.Contains(restBy["run_petstore_v1.0.0"].Description, "POST http://example.test/api/plans/") {
		t.Fatalf("REST v1.0.0 default REST desc missing: %#v", restBy["run_petstore_v1.0.0"])
	}

	if _, err := session.ListTools(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if n := h.n.Load(); n != 3 {
		t.Fatalf("lookups after second list = %d, want 3", n)
	}
}

type failHelp struct{}

func (failHelp) Lookup(context.Context, arazzo.ToolHelpRequest) (*arazzo.ToolHelp, error) {
	return nil, fmt.Errorf("registry down")
}

func TestArazzo_ToolHelpLookupErrorUsesDefaults(t *testing.T) {
	e, err := engine.New(engine.Options{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		ArazzoLoaders:  []arazzo.Loader{arazzo.NewFileLoader(plansDir(t))},
		ToolHelpLookup: failHelp{},
		DualMCPandREST: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(e.Handler())
	t.Cleanup(ts.Close)
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + engine.MCPPath}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) == 0 {
		t.Fatal("expected tools")
	}
	for _, tl := range tools.Tools {
		if strings.HasPrefix(tl.Name, "run_") && !strings.Contains(tl.Description, "How to call this MCP tool") {
			t.Fatalf("%s should fall back to defaults:\n%s", tl.Name, tl.Description)
		}
	}
}

func TestArazzo_ToolHelpCacheTTLDisabled(t *testing.T) {
	h := &recordingHelp{}
	e, err := engine.New(engine.Options{
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		ArazzoLoaders:    []arazzo.Loader{arazzo.NewFileLoader(plansDir(t))},
		QueryMatcher:     pingMatcher{planID: "petstore", workflowID: "pingHealth"},
		ToolHelpLookup:   h,
		ToolHelpCacheTTL: -1,
		DualMCPandREST:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(e.Handler())
	t.Cleanup(ts.Close)
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + engine.MCPPath}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if _, err := session.ListTools(ctx, nil); err != nil {
		t.Fatal(err)
	}
	first := h.n.Load()
	if first != 3 {
		t.Fatalf("first list lookups = %d, want 3", first)
	}
	if _, err := session.ListTools(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if n := h.n.Load(); n != 6 {
		t.Fatalf("second list lookups = %d, want 6 (cache disabled)", n)
	}
}

func TestArazzo_InvalidRESTQueryDescriptionFailNew(t *testing.T) {
	_, err := engine.New(engine.Options{
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		ArazzoLoaders: []arazzo.Loader{arazzo.NewFileLoader(plansDir(t))},
		QueryMatcher:  pingMatcher{planID: "petstore", workflowID: "pingHealth"},
		ToolDoc:       arazzo.ToolDocTemplates{RESTQueryDescription: "{{.Nope"},
	})
	if err == nil {
		t.Fatal("expected REST query description template error")
	}
}

func TestArazzo_QueryMatcher(t *testing.T) {
	exec := &countingExec{}
	e, err := engine.New(engine.Options{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		ArazzoLoaders:  []arazzo.Loader{arazzo.NewFileLoader(plansDir(t))},
		ArazzoExecutor: exec,
		QueryMatcher: pingMatcher{
			planID:     "petstore",
			workflowID: "pingHealth",
		},
		PublicBaseURL:  "http://example.test",
		DualMCPandREST: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(e.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Post(ts.URL+"/api/plans/query", "application/json", strings.NewReader(`{"query":"health check","data":{"name":"q"}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("query status = %d %s", resp.StatusCode, b)
	}
	if exec.n != 1 {
		t.Fatalf("executor calls = %d", exec.n)
	}

	toolsResp, err := http.Get(ts.URL + "/api/tools")
	if err != nil {
		t.Fatal(err)
	}
	defer toolsResp.Body.Close()
	var listed mcp.ListToolsResult
	if err := json.NewDecoder(toolsResp.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	foundQuery := false
	for _, tl := range listed.Tools {
		if tl.Name == "query" {
			foundQuery = true
			break
		}
	}
	if !foundQuery {
		t.Fatal("query tool missing when QueryMatcher is set")
	}

	catResp, err := http.Get(ts.URL + "/api/openapi")
	if err != nil {
		t.Fatal(err)
	}
	defer catResp.Body.Close()
	var catDoc map[string]any
	if err := json.NewDecoder(catResp.Body).Decode(&catDoc); err != nil {
		t.Fatal(err)
	}
	catPaths, _ := catDoc["paths"].(map[string]any)
	if _, ok := catPaths["/plans/query"]; !ok {
		t.Fatalf("catalog missing /plans/query: %v", catPaths)
	}

	e2, err := engine.New(engine.Options{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		ArazzoLoaders:  []arazzo.Loader{arazzo.NewFileLoader(plansDir(t))},
		ArazzoExecutor: exec,
		QueryMatcher: pingMatcher{
			planID:     "other-mesh",
			version:    "3.0.0",
			workflowID: "pingHealth",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ts2 := httptest.NewServer(e2.Handler())
	t.Cleanup(ts2.Close)
	resp, err = http.Post(ts2.URL+"/api/plans/query", "application/json", strings.NewReader(`{"query":"from global registry"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("unloaded plan status = %d %s", resp.StatusCode, b)
	}

	resp, err = http.Post(ts.URL+"/api/plans/query", "application/json", strings.NewReader(`{"query":"   "}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("empty query status = %d %s", resp.StatusCode, b)
	}

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + engine.MCPPath}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	qtool, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "query",
		Arguments: map[string]any{"query": "health", "data": map[string]any{"name": "mcp"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if qtool.IsError {
		t.Fatalf("MCP query: %s", toolErrorText(qtool))
	}
	if exec.n != 2 {
		t.Fatalf("executor calls = %d, want 2 (REST+MCP)", exec.n)
	}
}

func TestArazzo_RESTNotFoundAndBadJSON(t *testing.T) {
	e := newArazzoEngine(t, &countingExec{})
	ts := httptest.NewServer(e.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Post(ts.URL+"/api/plans/missing/pingHealth", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing plan status = %d", resp.StatusCode)
	}

	resp, err = http.Post(ts.URL+"/api/plans/petstore/v9.9.9/pingHealth", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing version status = %d", resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "/api/openapi/missing")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing openapi status = %d", resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "/api/openapi/petstore/v9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing openapi version status = %d", resp.StatusCode)
	}

	resp, err = http.Post(ts.URL+"/api/plans/petstore/pingHealth", "application/json", strings.NewReader(`{`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad json status = %d", resp.StatusCode)
	}

	resp, err = http.Post(ts.URL+"/api/plans/query", "application/json", strings.NewReader(`{`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("query without matcher status = %d, want 404 (route not registered)", resp.StatusCode)
	}
}

type denyAllPolicy struct{}

func (denyAllPolicy) Load(context.Context, arazzo.PolicyRequest) (*arazzo.PolicyBundle, error) {
	return &arazzo.PolicyBundle{Inbound: []byte(`
package plan.inbound
import rego.v1
default allow := false
`)}, nil
}

func TestArazzo_PolicyDeniedForbidden(t *testing.T) {
	e, err := engine.New(engine.Options{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		ArazzoLoaders:  []arazzo.Loader{arazzo.NewFileLoader(plansDir(t))},
		ArazzoExecutor: &countingExec{},
		PolicyLoader:   denyAllPolicy{},
		DualMCPandREST: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(e.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Post(ts.URL+"/api/plans/petstore/pingHealth", "application/json", strings.NewReader(`{"name":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 403 body = %s", resp.StatusCode, b)
	}
}

func toolErrorText(res *mcp.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return fmt.Sprintf("%#v", res)
	}
	if t, ok := res.Content[0].(*mcp.TextContent); ok {
		return t.Text
	}
	return fmt.Sprintf("%#v", res.Content)
}
