// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package apiv1

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToolsController_Get_prunesMCPMeta(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "ping",
		Description: "ping",
		InputSchema: map[string]any{"type": "object"},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, map[string]any{"ok": true}, nil
	})

	c := NewToolsController(server, slog.New(slog.NewTextHandler(io.Discard, nil)))
	mux := http.NewServeMux()
	c.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/tools", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body = %s", rec.Code, rec.Body.Bytes())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["_meta"]; ok {
		t.Fatalf("response must not include _meta: %v", body)
	}
	if _, ok := body["resultType"]; ok {
		t.Fatalf("response must not include resultType: %v", body)
	}
	tools, _ := body["tools"].([]any)
	if len(tools) == 0 {
		t.Fatal("tools is empty")
	}
	for i, raw := range tools {
		tm, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("tools[%d] type = %T", i, raw)
		}
		if _, ok := tm["_meta"]; ok {
			t.Fatalf("tools[%d] must not include _meta: %v", i, tm)
		}
	}
}

func TestRestToolsBody(t *testing.T) {
	res := &mcp.ListToolsResult{
		Tools: []*mcp.Tool{{Name: "a", InputSchema: map[string]any{"type": "object"}}},
	}
	res.SetMeta(map[string]any{"io.modelcontextprotocol/serverInfo": map[string]any{"name": "x"}})
	res.Tools[0].SetMeta(map[string]any{"k": "v"})

	body, err := restToolsBody(res)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := body["_meta"]; ok {
		t.Fatalf("_meta not pruned: %v", body)
	}
	tools := body["tools"].([]any)
	tm := tools[0].(map[string]any)
	if _, ok := tm["_meta"]; ok {
		t.Fatalf("tool _meta not pruned: %v", tm)
	}
	if tm["name"] != "a" {
		t.Fatalf("name = %v", tm["name"])
	}
}
