// Copyright 2026 Fidelity Investments. All rights reserved.
// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mevansam/context-mesh-engine/arazzo"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRegisterMCP_RunToolsAndDuplicateName(t *testing.T) {
	catalog := loadPetstore(t)
	exec := &stubExec{}
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	cache, err := RegisterMCP(server, catalog, NewRunner(catalog, exec, nil), RegisterMCPConfig{
		Logger: discardLogger(),
	})
	if err != nil {
		t.Fatal(err)
	}
	server.AddReceivingMiddleware(cache.ReceivingMiddleware())
	cs := mcpSession(t, server)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tl := range res.Tools {
		names[tl.Name] = true
		if strings.HasPrefix(tl.Name, "run_") && !strings.Contains(tl.Description, "How to call this MCP tool") {
			t.Fatalf("%s missing MCP how-to:\n%s", tl.Name, tl.Description)
		}
	}
	if names["query"] || !names["run_petstore_v1.0.0"] || !names["run_petstore_v1.1.0"] {
		t.Fatalf("names = %v", names)
	}

	call, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "run_petstore_v1.1.0",
		Arguments: map[string]any{
			"workflowId": "pingHealth",
			"inputs":     map[string]any{"name": "mcp"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if call.IsError {
		t.Fatalf("run error: %#v", call)
	}
	if exec.n != 1 {
		t.Fatalf("executor calls = %d", exec.n)
	}

	rest := &mcp.ListToolsResult{Tools: cloneTools(res.Tools)}
	cache.ApplyREST(context.Background(), rest)
	for _, tl := range rest.Tools {
		if strings.HasPrefix(tl.Name, "run_") && !strings.Contains(tl.Description, "POST ") {
			t.Fatalf("REST %s missing POST:\n%s", tl.Name, tl.Description)
		}
	}

	_, err = RegisterMCP(mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil), catalog, nil, RegisterMCPConfig{
		Templates: arazzo.ToolDocTemplates{Name: "dup"},
		Logger:    discardLogger(),
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate MCP tool name") {
		t.Fatalf("err = %v", err)
	}
}

func TestRegisterMCP_QueryTool(t *testing.T) {
	catalog := loadPetstore(t)
	exec := &stubExec{}
	runner := NewRunner(catalog, exec, staticMatcher{match: &arazzo.QueryMatch{
		PlanID: "petstore", WorkflowID: "pingHealth",
	}})
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	lu := &countLookup{help: &arazzo.ToolHelp{Title: "Ask", Description: "q-help"}}
	cache, err := RegisterMCP(server, catalog, runner, RegisterMCPConfig{
		HelpLookup:   lu,
		HelpCacheTTL: time.Hour,
		Logger:       discardLogger(),
	})
	if err != nil {
		t.Fatal(err)
	}
	server.AddReceivingMiddleware(cache.ReceivingMiddleware())
	cs := mcpSession(t, server)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var query *mcp.Tool
	for _, tl := range res.Tools {
		if tl.Name == "query" {
			query = tl
		}
	}
	if query == nil || query.Title != "Ask" || query.Description != "q-help" {
		t.Fatalf("query = %#v", query)
	}
	call, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "query",
		Arguments: map[string]any{"query": "health", "data": map[string]any{"name": "x"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if call.IsError {
		t.Fatalf("query error: %#v", call)
	}
	if exec.n != 1 {
		t.Fatalf("executor calls = %d", exec.n)
	}
}

func TestRegisterMCP_InvalidTemplates(t *testing.T) {
	catalog := loadPetstore(t)
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	_, err := RegisterMCP(server, catalog, nil, RegisterMCPConfig{
		Templates: arazzo.ToolDocTemplates{Name: "{{.Nope"},
		Logger:    discardLogger(),
	})
	if err == nil {
		t.Fatal("expected plan template error")
	}
	runner := NewRunner(catalog, &stubExec{}, staticMatcher{match: &arazzo.QueryMatch{
		PlanID: "petstore", WorkflowID: "pingHealth",
	}})
	_, err = RegisterMCP(mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil), catalog, runner, RegisterMCPConfig{
		Templates: arazzo.ToolDocTemplates{QueryName: "{{.Nope"},
		Logger:    discardLogger(),
	})
	if err == nil {
		t.Fatal("expected query template error")
	}
}

func cloneTools(in []*mcp.Tool) []*mcp.Tool {
	out := make([]*mcp.Tool, len(in))
	for i, tl := range in {
		if tl == nil {
			continue
		}
		cp := *tl
		out[i] = &cp
	}
	return out
}
