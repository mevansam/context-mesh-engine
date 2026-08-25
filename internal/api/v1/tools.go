// Copyright 2026 Fidelity Investments. All rights reserved.
// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package apiv1

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	iapi "github.com/mevansam/context-mesh-engine/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolsController serves GET /tools: the JSON envelope of MCP tools/list,
// with REST-specific descriptions overlaid on Arazzo plan/query tools.
type ToolsController struct {
	server   *mcp.Server
	mu       sync.RWMutex
	restDesc map[string]string
}

// NewToolsController lists tools from the shared MCP server.
func NewToolsController(server *mcp.Server) *ToolsController {
	return &ToolsController{server: server}
}

// SetRESTDescriptions replaces the name → REST description overlay applied
// on GET /tools. Callers must not mutate m after this returns.
func (c *ToolsController) SetRESTDescriptions(m map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.restDesc = m
}

// Register implements api.Controller.
func (c *ToolsController) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /tools", c.Get)
}

// Get returns the same envelope as JSON-RPC method tools/list (the result
// object, not the envelope). Optional query cursor matches ListToolsParams.
// Arazzo tool descriptions use REST templates; other tools keep MCP text.
func (c *ToolsController) Get(w http.ResponseWriter, r *http.Request) {
	res, err := listServerTools(r.Context(), c.server, r.URL.Query().Get("cursor"))
	if err != nil {
		iapi.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	c.applyRESTDescriptions(res)
	iapi.WriteJSON(w, http.StatusOK, res)
}

func (c *ToolsController) applyRESTDescriptions(res *mcp.ListToolsResult) {
	if res == nil {
		return
	}
	c.mu.RLock()
	descs := c.restDesc
	c.mu.RUnlock()
	if len(descs) == 0 {
		return
	}
	for i, tl := range res.Tools {
		if tl == nil {
			continue
		}
		rest, ok := descs[tl.Name]
		if !ok {
			continue
		}
		clone := *tl
		clone.Description = rest
		res.Tools[i] = &clone
	}
}

func listServerTools(ctx context.Context, server *mcp.Server, cursor string) (*mcp.ListToolsResult, error) {
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = ss.Close() }()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "context-mesh-engine-rest",
		Version: "0.1.0",
	}, &mcp.ClientOptions{Logger: slog.New(slog.DiscardHandler)})
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cs.Close() }()

	var params *mcp.ListToolsParams
	if cursor != "" {
		params = &mcp.ListToolsParams{Cursor: cursor}
	}
	return cs.ListTools(ctx, params)
}
