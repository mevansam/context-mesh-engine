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

	resp, err := http.Get(ts.URL + "/api/v1/openapi/petstore")
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

	resp, err = http.Get(ts.URL + "/api/v1/openapi/petstore/v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("versioned status = %d", resp.StatusCode)
	}

	resp, err = http.Post(ts.URL+"/api/v1/plans/petstore/pingHealth", "application/json", strings.NewReader(`{}`))
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

	latest := post("/api/v1/plans/petstore/pingHealth", `{"name":"a"}`)
	if latest["success"] != true {
		t.Fatalf("latest = %v", latest)
	}
	ver := post("/api/v1/plans/petstore/v1.0.0/pingHealth", `{"name":"b"}`)
	if ver["success"] != true {
		t.Fatalf("versioned = %v", ver)
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
		if strings.HasPrefix(tl.Name, "run_") && !strings.Contains(tl.Description, "POST http://example.test/api/v1/plans/") {
			t.Fatalf("tool %s description missing REST URL:\n%s", tl.Name, tl.Description)
		}
	}
	if !got["query"] || !got["run_petstore_v1.0.0"] || !got["run_petstore_v1.1.0"] {
		t.Fatalf("tools = %v", got)
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

	qresp, err := http.Post(ts.URL+"/api/v1/plans/query", "application/json", strings.NewReader(`{"query":"hello","data":{}}`))
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
	if sc["success"] != true {
		t.Fatalf("structured = %#v", res.StructuredContent)
	}

	resp, err := http.Post(ts.URL+"/api/v1/plans/petstore/v1.1.0/echoName", "application/json", bytes.NewReader([]byte(`{"name":"rest"}`)))
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

func toolErrorText(res *mcp.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return fmt.Sprintf("%#v", res)
	}
	if t, ok := res.Content[0].(*mcp.TextContent); ok {
		return t.Text
	}
	return fmt.Sprintf("%#v", res.Content)
}
