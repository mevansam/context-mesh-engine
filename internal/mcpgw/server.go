// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

// Package mcpgw constructs the shared MCP server and its Streamable HTTP handler.
package mcpgw

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Options configures the MCP gateway.
type Options struct {
	Implementation *mcp.Implementation
	Logger         *slog.Logger
	SessionTimeout time.Duration
}

// Gateway holds one mcp.Server reused by every Streamable HTTP session.
type Gateway struct {
	server  *mcp.Server
	handler http.Handler
}

// New creates a shared MCP server and a stateful Streamable HTTP handler.
// Tools are registered by the caller on [Gateway.Server].
func New(opts Options) *Gateway {
	server := mcp.NewServer(opts.Implementation, &mcp.ServerOptions{
		Logger: opts.Logger,
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		Logger:         opts.Logger,
		SessionTimeout: opts.SessionTimeout,
	})
	return &Gateway{server: server, handler: handler}
}

// Server is the shared MCP server. Register tools, prompts, and resources on it.
func (g *Gateway) Server() *mcp.Server {
	return g.server
}

// Handler is the Streamable HTTP handler (POST, GET SSE, DELETE).
func (g *Gateway) Handler() http.Handler {
	return g.handler
}
