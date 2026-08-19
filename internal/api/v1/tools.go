// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package apiv1

import (
	"context"
	"log/slog"
	"net/http"

	iapi "github.com/mevansam/context-mesh-engine/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolsController serves GET /tools: the JSON result of MCP tools/list.
type ToolsController struct {
	server *mcp.Server
}

// NewToolsController lists tools from the shared MCP server.
func NewToolsController(server *mcp.Server) *ToolsController {
	return &ToolsController{server: server}
}

// Register implements api.Controller.
func (c *ToolsController) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /tools", c.Get)
}

// Get returns the same payload as JSON-RPC method tools/list (the result
// object, not the envelope). Optional query cursor matches ListToolsParams.
func (c *ToolsController) Get(w http.ResponseWriter, r *http.Request) {
	res, err := listServerTools(r.Context(), c.server, r.URL.Query().Get("cursor"))
	if err != nil {
		iapi.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	iapi.WriteJSON(w, http.StatusOK, res)
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
