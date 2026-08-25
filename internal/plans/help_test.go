// Copyright 2026 Fidelity Investments. All rights reserved.
// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mevansam/context-mesh-engine/arazzo"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type countLookup struct {
	n       atomic.Int32
	help    *arazzo.ToolHelp
	err     error
	mu      sync.Mutex
	fail    bool
	lastReq arazzo.ToolHelpRequest
}

func (c *countLookup) Lookup(_ context.Context, req arazzo.ToolHelpRequest) (*arazzo.ToolHelp, error) {
	c.n.Add(1)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastReq = req
	if c.fail {
		return nil, c.err
	}
	return c.help, c.err
}

type blockLookup struct {
	n       atomic.Int32
	started chan struct{}
	release chan struct{}
	help    *arazzo.ToolHelp
	err     error
}

func (b *blockLookup) Lookup(context.Context, arazzo.ToolHelpRequest) (*arazzo.ToolHelp, error) {
	b.n.Add(1)
	select {
	case <-b.started:
	default:
		close(b.started)
	}
	<-b.release
	return b.help, b.err
}

func testCache(t *testing.T, lookup arazzo.ToolHelpLookup, ttl time.Duration) *HelpCache {
	t.Helper()
	c := newHelpCache(arazzo.ToolDocTemplates{}, lookup, ttl, slog.New(slog.NewTextHandler(io.Discard, nil)))
	c.add("run_p_v1", helpTarget{
		kind:    arazzo.ToolHelpKindPlan,
		planID:  "p",
		version: "1",
		docCtx:  arazzo.NewToolDocContext("p", "1", "t", "", "", nil, "", ""),
	})
	return c
}

func TestHelpCache_TTL(t *testing.T) {
	lu := &countLookup{help: &arazzo.ToolHelp{Title: "T", Description: "D-{{.PlanID}}"}}
	c := testCache(t, lu, 80*time.Millisecond)
	res := &mcp.ListToolsResult{Tools: []*mcp.Tool{{Name: "run_p_v1"}}}
	c.apply(context.Background(), res, surfaceMCP)
	c.apply(context.Background(), res, surfaceMCP)
	if n := lu.n.Load(); n != 1 {
		t.Fatalf("lookups = %d, want 1", n)
	}
	if res.Tools[0].Title != "T" || res.Tools[0].Description != "D-p" {
		t.Fatalf("title=%q desc=%q", res.Tools[0].Title, res.Tools[0].Description)
	}
	lu.mu.Lock()
	if lu.lastReq.Kind != arazzo.ToolHelpKindPlan || lu.lastReq.PlanID != "p" || lu.lastReq.Version != "1" {
		t.Fatalf("req = %+v", lu.lastReq)
	}
	lu.mu.Unlock()
	time.Sleep(100 * time.Millisecond)
	c.apply(context.Background(), res, surfaceMCP)
	if n := lu.n.Load(); n != 2 {
		t.Fatalf("lookups after TTL = %d, want 2", n)
	}
}

func TestHelpCache_AlwaysRefresh(t *testing.T) {
	lu := &countLookup{help: &arazzo.ToolHelp{Description: "x"}}
	c := testCache(t, lu, -1)
	res := &mcp.ListToolsResult{Tools: []*mcp.Tool{{Name: "run_p_v1"}}}
	c.apply(context.Background(), res, surfaceMCP)
	c.apply(context.Background(), res, surfaceMCP)
	if n := lu.n.Load(); n != 2 {
		t.Fatalf("lookups = %d, want 2 (ttl disabled)", n)
	}
}

func TestHelpCache_ErrorKeepsStale(t *testing.T) {
	lu := &countLookup{help: &arazzo.ToolHelp{Description: "ok"}, err: nil}
	c := testCache(t, lu, time.Millisecond)
	res := &mcp.ListToolsResult{Tools: []*mcp.Tool{{Name: "run_p_v1"}}}
	c.apply(context.Background(), res, surfaceMCP)
	if res.Tools[0].Description != "ok" {
		t.Fatalf("desc = %q", res.Tools[0].Description)
	}
	time.Sleep(5 * time.Millisecond)
	lu.mu.Lock()
	lu.fail = true
	lu.err = errString("registry down")
	lu.mu.Unlock()
	c.apply(context.Background(), res, surfaceMCP)
	if res.Tools[0].Description != "ok" {
		t.Fatalf("stale desc = %q", res.Tools[0].Description)
	}
}

func TestHelpCache_ErrorNoStaleUsesDefaults(t *testing.T) {
	lu := &countLookup{fail: true, err: errors.New("down")}
	c := testCache(t, lu, time.Minute)
	res := &mcp.ListToolsResult{Tools: []*mcp.Tool{{Name: "run_p_v1", Description: "placeholder"}}}
	c.apply(context.Background(), res, surfaceMCP)
	if !strings.Contains(res.Tools[0].Description, "How to call this MCP tool") {
		t.Fatalf("want default MCP description, got %q", res.Tools[0].Description)
	}
}

func TestHelpCache_NilLookupAndLogger(t *testing.T) {
	c := newHelpCache(arazzo.ToolDocTemplates{}, nil, time.Minute, nil)
	c.add("run_p_v1", helpTarget{
		kind:   arazzo.ToolHelpKindPlan,
		planID: "p", version: "1",
		docCtx: arazzo.NewToolDocContext("p", "1", "t", "", "", nil, "", ""),
	})
	res := &mcp.ListToolsResult{Tools: []*mcp.Tool{{Name: "run_p_v1"}}}
	c.apply(context.Background(), res, surfaceMCP)
	if !strings.Contains(res.Tools[0].Description, "How to call this MCP tool") {
		t.Fatalf("default lookup desc = %q", res.Tools[0].Description)
	}
}

func TestHelpCache_RESTSurface(t *testing.T) {
	lu := &countLookup{help: &arazzo.ToolHelp{Description: "same"}}
	c := testCache(t, lu, time.Minute)
	res := &mcp.ListToolsResult{Tools: []*mcp.Tool{{Name: "run_p_v1"}}}
	c.ApplyREST(context.Background(), res)
	if res.Tools[0].Description != "same" {
		t.Fatalf("REST desc = %q", res.Tools[0].Description)
	}
}

func TestHelpCache_RESTDescriptionDistinct(t *testing.T) {
	lu := &countLookup{help: &arazzo.ToolHelp{Description: "mcp-d", RESTDescription: "rest-d"}}
	c := testCache(t, lu, time.Minute)
	mcpRes := &mcp.ListToolsResult{Tools: []*mcp.Tool{{Name: "run_p_v1"}}}
	restRes := &mcp.ListToolsResult{Tools: []*mcp.Tool{{Name: "run_p_v1"}}}
	c.apply(context.Background(), mcpRes, surfaceMCP)
	c.apply(context.Background(), restRes, surfaceREST)
	if mcpRes.Tools[0].Description != "mcp-d" {
		t.Fatalf("MCP desc = %q", mcpRes.Tools[0].Description)
	}
	if restRes.Tools[0].Description != "rest-d" {
		t.Fatalf("REST desc = %q", restRes.Tools[0].Description)
	}
}

func TestHelpCache_SkipsUnknownAndNilTools(t *testing.T) {
	lu := &countLookup{help: &arazzo.ToolHelp{Description: "hit"}}
	c := testCache(t, lu, time.Minute)
	c.apply(context.Background(), nil, surfaceMCP)
	res := &mcp.ListToolsResult{Tools: []*mcp.Tool{
		nil,
		{Name: "ping", Description: "liveness"},
		{Name: "run_p_v1"},
	}}
	c.apply(context.Background(), res, surfaceMCP)
	if res.Tools[1].Description != "liveness" {
		t.Fatalf("ping mutated: %q", res.Tools[1].Description)
	}
	if res.Tools[2].Description != "hit" {
		t.Fatalf("plan desc = %q", res.Tools[2].Description)
	}
	if n := lu.n.Load(); n != 1 {
		t.Fatalf("lookups = %d, want 1", n)
	}
}

func TestHelpCache_QueryKind(t *testing.T) {
	lu := &countLookup{help: &arazzo.ToolHelp{Title: "Ask", Description: "q"}}
	c := newHelpCache(arazzo.ToolDocTemplates{}, lu, time.Minute, slog.New(slog.NewTextHandler(io.Discard, nil)))
	c.add("query", helpTarget{
		kind:   arazzo.ToolHelpKindQuery,
		docCtx: arazzo.NewToolDocContext("plan", "1.0.0", "", "", "", nil, "", ""),
	})
	res := &mcp.ListToolsResult{Tools: []*mcp.Tool{{Name: "query"}}}
	c.apply(context.Background(), res, surfaceMCP)
	if res.Tools[0].Title != "Ask" || res.Tools[0].Description != "q" {
		t.Fatalf("query = %+v", res.Tools[0])
	}
	lu.mu.Lock()
	defer lu.mu.Unlock()
	if lu.lastReq.Kind != arazzo.ToolHelpKindQuery {
		t.Fatalf("kind = %q", lu.lastReq.Kind)
	}
}

func TestHelpCache_RenderErrorKeepsPlaceholder(t *testing.T) {
	lu := &countLookup{help: &arazzo.ToolHelp{Description: "{{.Nope"}}
	c := testCache(t, lu, time.Minute)
	res := &mcp.ListToolsResult{Tools: []*mcp.Tool{{Name: "run_p_v1", Description: "placeholder"}}}
	c.apply(context.Background(), res, surfaceMCP)
	if res.Tools[0].Description != "placeholder" {
		t.Fatalf("desc = %q, want placeholder", res.Tools[0].Description)
	}
}

func TestHelpCache_Singleflight(t *testing.T) {
	lu := &blockLookup{
		started: make(chan struct{}),
		release: make(chan struct{}),
		help:    &arazzo.ToolHelp{Description: "ok"},
	}
	c := testCache(t, lu, time.Minute)
	var wg sync.WaitGroup
	wg.Add(2)
	run := func() {
		defer wg.Done()
		res := &mcp.ListToolsResult{Tools: []*mcp.Tool{{Name: "run_p_v1"}}}
		c.apply(context.Background(), res, surfaceMCP)
		if res.Tools[0].Description != "ok" {
			t.Errorf("desc = %q", res.Tools[0].Description)
		}
	}
	go run()
	<-lu.started
	go run()
	time.Sleep(20 * time.Millisecond)
	close(lu.release)
	wg.Wait()
	if n := lu.n.Load(); n != 1 {
		t.Fatalf("lookups = %d, want 1", n)
	}
}

func TestHelpCache_ReceivingMiddleware(t *testing.T) {
	lu := &countLookup{help: &arazzo.ToolHelp{Description: "from-mw"}}
	c := testCache(t, lu, time.Minute)
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "run_p_v1", Description: "placeholder"},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
		})
	server.AddReceivingMiddleware(c.ReceivingMiddleware())
	cs := mcpSession(t, server)
	if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "run_p_v1"}); err != nil {
		t.Fatal(err)
	}
	if n := lu.n.Load(); n != 0 {
		t.Fatalf("lookup on tools/call = %d, want 0", n)
	}
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != 1 || res.Tools[0].Description != "from-mw" {
		t.Fatalf("listed = %#v", res.Tools)
	}
}

func TestHelpCache_SingleflightError(t *testing.T) {
	lu := &blockLookup{
		started: make(chan struct{}),
		release: make(chan struct{}),
		err:     errors.New("down"),
	}
	c := testCache(t, lu, time.Minute)
	var wg sync.WaitGroup
	wg.Add(2)
	run := func() {
		defer wg.Done()
		res := &mcp.ListToolsResult{Tools: []*mcp.Tool{{Name: "run_p_v1", Description: "placeholder"}}}
		c.apply(context.Background(), res, surfaceMCP)
		if !strings.Contains(res.Tools[0].Description, "How to call this MCP tool") {
			t.Errorf("desc = %q", res.Tools[0].Description)
		}
	}
	go run()
	<-lu.started
	go run()
	time.Sleep(20 * time.Millisecond)
	close(lu.release)
	wg.Wait()
	if n := lu.n.Load(); n != 1 {
		t.Fatalf("lookups = %d, want 1", n)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func listMCP(t *testing.T, server *mcp.Server) *mcp.ListToolsResult {
	t.Helper()
	res, err := mcpSession(t, server).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func mcpSession(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, &mcp.ClientOptions{
		Logger: slog.New(slog.DiscardHandler),
	})
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}
