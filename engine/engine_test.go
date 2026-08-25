// Copyright 2026 Fidelity Investments. All rights reserved.
// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package engine_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mevansam/context-mesh-engine/api"
	"github.com/mevansam/context-mesh-engine/engine"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newTestEngine(t *testing.T) *engine.Engine {
	t.Helper()
	e, err := engine.New(engine.Options{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		DualMCPandREST: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	mcp.AddTool(e.MCP(), &mcp.Tool{
		Name:        "ping",
		Description: "liveness probe",
	}, ping)
	return e
}

func ping(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "pong"}},
	}, nil, nil
}

func TestHandler_HealthJSON(t *testing.T) {
	ts := httptest.NewServer(newTestEngine(t).Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var body api.HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" {
		t.Fatalf("status = %q, want ok", body.Status)
	}
}

func TestHandler_ToolsListJSON(t *testing.T) {
	ts := httptest.NewServer(newTestEngine(t).Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/api/tools")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body mcp.ListToolsResult
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Tools == nil {
		t.Fatal("tools is null, want []")
	}
	names := map[string]bool{}
	for _, tl := range body.Tools {
		names[tl.Name] = true
	}
	if !names["ping"] {
		t.Fatalf("tools = %v, want ping", names)
	}
}

func TestHandler_ToolsListEmpty(t *testing.T) {
	e, err := engine.New(engine.Options{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(e.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/api/tools")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body mcp.ListToolsResult
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Tools) != 0 {
		t.Fatalf("tools = %v, want empty", body.Tools)
	}
}

func TestHandler_MCPInitializeAndPing(t *testing.T) {
	ts := httptest.NewServer(newTestEngine(t).Handler())
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

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "ping"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatal("ping returned tool error")
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok || text.Text != "pong" {
		t.Fatalf("content = %#v, want pong", res.Content)
	}
}

func TestHandler_MCPGETRequiresSession(t *testing.T) {
	ts := httptest.NewServer(newTestEngine(t).Handler())
	t.Cleanup(ts.Close)

	req, err := http.NewRequest(http.MethodGet, ts.URL+engine.MCPPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (SSE path mounted, session required)", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestHandler_RESTNotMCP(t *testing.T) {
	ts := httptest.NewServer(newTestEngine(t).Handler())
	t.Cleanup(ts.Close)

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d (REST mux, not MCP)", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestHandler_CustomAPIPrefix(t *testing.T) {
	e, err := engine.New(engine.Options{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		APIPrefix: "service/v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.APIPrefix(); got != "/service/v2" {
		t.Fatalf("APIPrefix() = %q, want /service/v2", got)
	}

	ts := httptest.NewServer(e.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/service/v2/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("custom prefix status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	old, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer old.Body.Close()
	if old.StatusCode != http.StatusNotFound {
		t.Fatalf("default prefix status = %d, want %d", old.StatusCode, http.StatusNotFound)
	}
}

func TestNew_APIPrefixRejected(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, p := range []string{"/", "/mcp", "mcp"} {
		if _, err := engine.New(engine.Options{Logger: log, APIPrefix: p}); err == nil {
			t.Fatalf("APIPrefix %q: expected error", p)
		}
	}
}

func TestNew_ServeModesMutuallyExclusive(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	combos := []engine.Options{
		{Logger: log, DualMCPandREST: true, MCPOnly: true},
		{Logger: log, DualMCPandREST: true, RESTOnly: true},
		{Logger: log, MCPOnly: true, RESTOnly: true},
		{Logger: log, DualMCPandREST: true, MCPOnly: true, RESTOnly: true},
	}
	for i, opts := range combos {
		if _, err := engine.New(opts); err == nil {
			t.Fatalf("combo %d: expected mutually exclusive error", i)
		}
	}
}

func TestHandler_DefaultRESTOnly(t *testing.T) {
	e, err := engine.New(engine.Options{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(e.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("REST status = %d, want 200", resp.StatusCode)
	}

	mcpResp, err := http.Get(ts.URL + engine.MCPPath)
	if err != nil {
		t.Fatal(err)
	}
	defer mcpResp.Body.Close()
	if mcpResp.StatusCode != http.StatusNotFound {
		t.Fatalf("MCP status = %d, want 404", mcpResp.StatusCode)
	}
}

func TestHandler_DualMCPandREST(t *testing.T) {
	e := newTestEngine(t)
	ts := httptest.NewServer(e.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("REST status = %d, want 200", resp.StatusCode)
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
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "ping"})
	if err != nil || res.IsError {
		t.Fatalf("ping: %v %#v", err, res)
	}
}

func TestHandler_MCPOnly(t *testing.T) {
	e, err := engine.New(engine.Options{
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		MCPOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	mcp.AddTool(e.MCP(), &mcp.Tool{Name: "ping", Description: "liveness probe"}, ping)
	ts := httptest.NewServer(e.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("REST status = %d, want 404", resp.StatusCode)
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
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "ping"})
	if err != nil || res.IsError {
		t.Fatalf("ping: %v %#v", err, res)
	}
}

func TestHandler_RESTOnly(t *testing.T) {
	e, err := engine.New(engine.Options{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		RESTOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	mcp.AddTool(e.MCP(), &mcp.Tool{Name: "ping", Description: "liveness probe"}, ping)
	ts := httptest.NewServer(e.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("REST status = %d, want 200", resp.StatusCode)
	}

	mcpResp, err := http.Get(ts.URL + engine.MCPPath)
	if err != nil {
		t.Fatal(err)
	}
	defer mcpResp.Body.Close()
	if mcpResp.StatusCode != http.StatusNotFound {
		t.Fatalf("MCP status = %d, want 404", mcpResp.StatusCode)
	}

	tools, err := http.Get(ts.URL + "/api/tools")
	if err != nil {
		t.Fatal(err)
	}
	defer tools.Body.Close()
	var body mcp.ListToolsResult
	if err := json.NewDecoder(tools.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tl := range body.Tools {
		if tl.Name == "ping" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("REST /tools should still list MCP ping")
	}
}

type extraController struct{}

func (extraController) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /extra", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
}

func TestEngine_AddController(t *testing.T) {
	e := newTestEngine(t)
	e.AddController(extraController{})
	ts := httptest.NewServer(e.Handler())
	t.Cleanup(ts.Close)
	resp, err := http.Get(ts.URL + "/api/extra")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}

func TestEngine_ListenAndServeCancel(t *testing.T) {
	e, err := engine.New(engine.Options{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Addr:   "127.0.0.1:0",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.ListenAndServe(ctx); err != nil {
		t.Fatal(err)
	}
}
