// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package apiv1

import (
	"context"
	"log/slog"
	"net/http"

	iapi "github.com/mevansam/context-mesh-engine/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolHelpOverlay rewrites Arazzo tool title/description on GET /tools.
type ToolHelpOverlay func(ctx context.Context, res *mcp.ListToolsResult)

// ToolsController serves GET /tools: the JSON envelope of MCP tools/list,
// with REST-specific descriptions overlaid on Arazzo plan/query tools.
type ToolsController struct {
	server  *mcp.Server
	overlay ToolHelpOverlay
	logger  *slog.Logger
}

// NewToolsController lists tools from the shared MCP server.
func NewToolsController(server *mcp.Server, logger *slog.Logger) *ToolsController {
	if logger == nil {
		logger = slog.Default()
	}
	return &ToolsController{server: server, logger: logger}
}

// SetToolHelpOverlay sets the REST description overlay applied on GET /tools.
func (c *ToolsController) SetToolHelpOverlay(fn ToolHelpOverlay) {
	c.overlay = fn
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
		c.logger.Error("tools/list failed", "err", err)
		iapi.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if c.overlay != nil {
		c.overlay(r.Context(), res)
	}
	body, err := restToolsBody(res)
	if err != nil {
		c.logger.Error("tools/list encode failed", "err", err)
		iapi.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	iapi.WriteJSON(w, http.StatusOK, body)
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
