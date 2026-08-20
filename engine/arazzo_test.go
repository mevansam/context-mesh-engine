// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
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
	"testing"

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
		if tl.Name == "query" && !strings.Contains(tl.Description, "POST http://example.test/api/plans/query") {
			t.Fatalf("query description missing REST URL:\n%s", tl.Description)
		}
		if strings.HasPrefix(tl.Name, "run_") && !strings.Contains(tl.Description, "POST http://example.test/api/plans/") {
			t.Fatalf("tool %s description missing REST URL:\n%s", tl.Name, tl.Description)
		}
	}
	if !got["query"] || !got["run_petstore_v1.0.0"] || !got["run_petstore_v1.1.0"] {
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
	}

	q, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "query",
		Arguments: map[string]any{"query": "hello"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !q.IsError {
		t.Fatal("query should be unimplemented")
	}
	if got := toolErrorText(q); got != "query is not implemented" {
		t.Fatalf("MCP query error = %q", got)
	}

	qresp, err := http.Post(ts.URL+"/api/plans/query", "application/json", strings.NewReader(`{"query":"hello","data":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer qresp.Body.Close()
	if qresp.StatusCode != http.StatusNotImplemented {
		b, _ := io.ReadAll(qresp.Body)
		t.Fatalf("REST query status = %d, want 501 body = %s", qresp.StatusCode, b)
	}
	var qerr struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(qresp.Body).Decode(&qerr); err != nil {
		t.Fatal(err)
	}
	if qerr.Error != "query is not implemented" {
		t.Fatalf("REST query error = %q", qerr.Error)
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
		PublicBaseURL:  "http://example.test",
		APIPrefix:      "/service/v2",
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
		if tl.Name == "query" && !strings.Contains(tl.Description, "POST http://example.test/service/v2/plans/query") {
			t.Fatalf("query description missing custom REST URL:\n%s", tl.Description)
		}
		if strings.HasPrefix(tl.Name, "run_") && !strings.Contains(tl.Description, "POST http://example.test/service/v2/plans/") {
			t.Fatalf("tool %s description missing custom REST URL:\n%s", tl.Name, tl.Description)
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
		PublicBaseURL: "http://example.test",
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
