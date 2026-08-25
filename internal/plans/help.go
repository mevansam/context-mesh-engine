// Copyright 2026 Fidelity Investments. All rights reserved.
// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/mevansam/context-mesh-engine/arazzo"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type helpSurface int

const (
	surfaceMCP helpSurface = iota
	surfaceREST
)

type helpTarget struct {
	kind    arazzo.ToolHelpKind
	planID  string
	version string
	docCtx  arazzo.ToolDocContext
}

func (t helpTarget) key() string {
	if t.kind == arazzo.ToolHelpKindQuery {
		return "query"
	}
	return t.planID + "\x00" + t.version
}

func (t helpTarget) req() arazzo.ToolHelpRequest {
	return arazzo.ToolHelpRequest{Kind: t.kind, PlanID: t.planID, Version: t.version}
}

type cacheEntry struct {
	help  arazzo.ToolHelp
	until time.Time
	have  bool
}

type inflight struct {
	wg sync.WaitGroup
}

// HelpCache looks up title/description templates on tools/list and GET /tools.
type HelpCache struct {
	tmpls  arazzo.ToolDocTemplates
	lookup arazzo.ToolHelpLookup
	ttl    time.Duration
	logger *slog.Logger

	mu       sync.Mutex
	targets  map[string]helpTarget
	entries  map[string]cacheEntry
	inflight map[string]*inflight
}

func newHelpCache(tmpls arazzo.ToolDocTemplates, lookup arazzo.ToolHelpLookup, ttl time.Duration, logger *slog.Logger) *HelpCache {
	if lookup == nil {
		lookup = arazzo.DefaultToolHelpLookup()
	}
	if logger == nil {
		logger = slog.Default()
	}
	if ttl < 0 {
		ttl = 0
	}
	return &HelpCache{
		tmpls:    arazzo.MergeTemplates(tmpls),
		lookup:   lookup,
		ttl:      ttl,
		logger:   logger,
		targets:  map[string]helpTarget{},
		entries:  map[string]cacheEntry{},
		inflight: map[string]*inflight{},
	}
}

func (c *HelpCache) add(name string, tgt helpTarget) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.targets[name] = tgt
}

// ReceivingMiddleware rewrites Arazzo tool title/description on tools/list (MCP).
func (c *HelpCache) ReceivingMiddleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			res, err := next(ctx, method, req)
			if err != nil || method != "tools/list" {
				return res, err
			}
			list, ok := res.(*mcp.ListToolsResult)
			if !ok || list == nil {
				return res, err
			}
			c.apply(ctx, list, surfaceMCP)
			return list, nil
		}
	}
}

// ApplyREST rewrites Arazzo tool descriptions for GET /tools.
func (c *HelpCache) ApplyREST(ctx context.Context, res *mcp.ListToolsResult) {
	c.apply(ctx, res, surfaceREST)
}

func (c *HelpCache) apply(ctx context.Context, res *mcp.ListToolsResult, surface helpSurface) {
	if res == nil {
		return
	}
	for i, tl := range res.Tools {
		if tl == nil {
			continue
		}
		c.mu.Lock()
		tgt, ok := c.targets[tl.Name]
		c.mu.Unlock()
		if !ok {
			continue
		}
		help := c.get(ctx, tgt)
		doc, err := arazzo.RenderWithHelp(c.tmpls, help, tgt.kind, tgt.docCtx)
		if err != nil {
			c.logger.Warn("tool help render failed", "tool", tl.Name, "err", err)
			continue
		}
		clone := *tl
		clone.Title = doc.Title
		if surface == surfaceREST {
			clone.Description = doc.RESTDescription
		} else {
			clone.Description = doc.Description
		}
		res.Tools[i] = &clone
	}
}

func (c *HelpCache) fresh(e cacheEntry) bool {
	if !e.have || c.ttl <= 0 {
		return false
	}
	return time.Now().Before(e.until)
}

func (c *HelpCache) get(ctx context.Context, tgt helpTarget) arazzo.ToolHelp {
	key := tgt.key()

	c.mu.Lock()
	if e, ok := c.entries[key]; ok && c.fresh(e) {
		c.mu.Unlock()
		return e.help
	}
	if f, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		f.wg.Wait()
		c.mu.Lock()
		e := c.entries[key]
		c.mu.Unlock()
		if e.have {
			return e.help
		}
		return arazzo.ToolHelp{}
	}
	f := &inflight{}
	f.wg.Add(1)
	c.inflight[key] = f
	stale, haveStale := c.entries[key]
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.inflight, key)
		c.mu.Unlock()
		f.wg.Done()
	}()

	var help arazzo.ToolHelp
	got, err := c.lookup.Lookup(ctx, tgt.req())
	if err != nil {
		c.logger.Warn("tool help lookup failed",
			"kind", tgt.kind, "planId", tgt.planID, "version", tgt.version, "err", err)
		if haveStale && stale.have {
			return stale.help
		}
		return arazzo.ToolHelp{}
	}
	if got != nil {
		help = *got
	}

	c.mu.Lock()
	e := cacheEntry{help: help, have: true}
	if c.ttl > 0 {
		e.until = time.Now().Add(c.ttl)
	}
	c.entries[key] = e
	c.mu.Unlock()
	return help
}
