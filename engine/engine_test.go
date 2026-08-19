// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
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
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
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
