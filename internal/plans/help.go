// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/mevansam/context-mesh-engine/arazzo"
	"github.com/mevansam/context-mesh-engine/internal/ttlcache"
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

func helpRequest(key string) arazzo.ToolHelpRequest {
	if key == "query" {
		return arazzo.ToolHelpRequest{Kind: arazzo.ToolHelpKindQuery}
	}
	planID, version, _ := strings.Cut(key, "\x00")
	return arazzo.ToolHelpRequest{Kind: arazzo.ToolHelpKindPlan, PlanID: planID, Version: version}
}

// HelpCache looks up title/description templates on tools/list and GET /tools.
type HelpCache struct {
	tmpls  arazzo.ToolDocTemplates
	logger *slog.Logger

	mu      sync.Mutex
	targets map[string]helpTarget
	cache   *ttlcache.Cache[string, arazzo.ToolHelp]
}

func newHelpCache(tmpls arazzo.ToolDocTemplates, lookup arazzo.ToolHelpLookup, ttl time.Duration, logger *slog.Logger) *HelpCache {
	if lookup == nil {
		lookup = arazzo.DefaultToolHelpLookup()
	}
	if logger == nil {
		logger = slog.Default()
	}
	c := &HelpCache{
		tmpls:   arazzo.MergeTemplates(tmpls),
		logger:  logger,
		targets: map[string]helpTarget{},
	}
	c.cache = ttlcache.New(ttl, func(ctx context.Context, key string) (arazzo.ToolHelp, error) {
		got, err := lookup.Lookup(ctx, helpRequest(key))
		if err != nil {
			return arazzo.ToolHelp{}, err
		}
		if got == nil {
			return arazzo.ToolHelp{}, nil
		}
		return *got, nil
	})
	return c
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

func (c *HelpCache) get(ctx context.Context, tgt helpTarget) arazzo.ToolHelp {
	help, err := c.cache.Get(ctx, tgt.key())
	if err != nil {
		c.logger.Warn("tool help lookup failed",
			"kind", tgt.kind, "planId", tgt.planID, "version", tgt.version, "err", err)
	}
	return help
}
