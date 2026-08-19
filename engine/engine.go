// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// Package engine is the public SDK for hosting an MCP Streamable HTTP
// server and a JSON REST API on one HTTP listener.
//
// Implementation lives under internal/: mcpgw, httpserver, api/v1, and plans.
package engine

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mevansam/context-mesh-engine/api"
	"github.com/mevansam/context-mesh-engine/arazzo"
	apiv1 "github.com/mevansam/context-mesh-engine/internal/api/v1"
	"github.com/mevansam/context-mesh-engine/internal/httpserver"
	"github.com/mevansam/context-mesh-engine/internal/mcpgw"
	"github.com/mevansam/context-mesh-engine/internal/plans"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// DefaultAddr is the listen address used when [Options.Addr] is empty.
	DefaultAddr = "localhost:8080"

	// MCPPath is the Streamable HTTP endpoint advertised to MCP clients.
	MCPPath = "/mcp"

	// APIv1Prefix is the default REST API prefix used when [Options.APIPrefix] is empty.
	APIv1Prefix = "/api/v1"
)

const (
	defaultReadHeaderTimeout = 10 * time.Second
	defaultAPITimeout        = 15 * time.Second
)

// Options configures an [Engine].
type Options struct {
	// Addr is the TCP address for [Engine.ListenAndServe].
	// If empty, [DefaultAddr] is used.
	Addr string

	// Implementation identifies this MCP server to clients.
	// If nil, Name "context-mesh-engine" and Version "0.1.0" are used.
	Implementation *mcp.Implementation

	// Logger is used for HTTP and MCP handler logs.
	// If nil, slog.Default() is used.
	Logger *slog.Logger

	// SessionTimeout closes idle MCP sessions. If zero, idle sessions
	// are never closed (the go-sdk default).
	SessionTimeout time.Duration

	// APITimeout is the per-request timeout for REST under [Options.APIPrefix] only.
	// If zero, 15s is used. A negative value disables the timeout.
	APITimeout time.Duration

	// ReadHeaderTimeout is set on the HTTP server.
	// If zero, 10s is used. WriteTimeout is left unset so GET SSE can hang.
	ReadHeaderTimeout time.Duration

	// APIPrefix is the REST path prefix (health, plans, OpenAPI).
	// If empty, [APIv1Prefix] ("/api/v1") is used. A leading slash is added
	// if missing; a trailing slash is stripped. Must not be "/" or [MCPPath].
	APIPrefix string

	// ArazzoLoaders supply Arazzo documents. If empty, no plan tools or
	// plan REST routes are registered.
	ArazzoLoaders []arazzo.Loader

	// ArazzoExecutor performs backend HTTP calls for workflow steps.
	// If nil, catalog and OpenAPI still load; execute returns 501.
	ArazzoExecutor arazzo.Executor

	// PublicBaseURL is the origin used in MCP tool descriptions
	// (for example http://localhost:8080). Addr is not substituted.
	// If empty, REST URLs in descriptions are path-only.
	PublicBaseURL string

	// ToolDoc are Go text/templates for generated MCP tool name, title,
	// and description. Empty fields use [arazzo.DefaultToolDocTemplates].
	ToolDoc arazzo.ToolDocTemplates
}

// Engine is a thin facade over internal/mcpgw, internal/httpserver,
// and internal/api/v1.
type Engine struct {
	opts   Options
	mcp    *mcpgw.Gateway
	router *apiv1.Router

	once    sync.Once
	handler http.Handler
}

// New constructs an Engine with a shared MCP server and the default
// health controller registered under [Options.APIPrefix]. When
// [Options.ArazzoLoaders] is set, plans are loaded and MCP run_* tools
// plus REST plan routes are registered. Load or template errors fail
// construction.
func New(opts Options) (*Engine, error) {
	opts = applyDefaults(opts)
	if err := validateAPIPrefix(opts.APIPrefix); err != nil {
		return nil, err
	}

	gw := mcpgw.New(mcpgw.Options{
		Implementation: opts.Implementation,
		Logger:         opts.Logger,
		SessionTimeout: opts.SessionTimeout,
	})

	router := apiv1.New()
	router.Register(&apiv1.HealthController{})

	if len(opts.ArazzoLoaders) > 0 {
		docCtx := arazzo.NewToolDocContext(
			"plan", "1.0.0", "t", "", "", nil, opts.PublicBaseURL, opts.APIPrefix,
		)
		if _, _, _, err := arazzo.RenderToolDoc(opts.ToolDoc, docCtx); err != nil {
			return nil, fmt.Errorf("tool doc templates: %w", err)
		}
		if _, _, _, err := arazzo.RenderQueryDoc(opts.ToolDoc, docCtx); err != nil {
			return nil, fmt.Errorf("query tool doc templates: %w", err)
		}
		catalog, err := plans.Load(context.Background(), opts.ArazzoLoaders, opts.Logger)
		if err != nil {
			return nil, err
		}
		runner := plans.NewRunner(catalog, opts.ArazzoExecutor)
		if err := plans.RegisterMCP(gw.Server(), catalog, runner, opts.ToolDoc, opts.PublicBaseURL, opts.APIPrefix); err != nil {
			return nil, err
		}
		router.Register(apiv1.NewPlansController(catalog, runner))
	}

	return &Engine{
		opts:   opts,
		mcp:    gw,
		router: router,
	}, nil
}

func applyDefaults(opts Options) Options {
	if opts.Addr == "" {
		opts.Addr = DefaultAddr
	}
	if opts.Implementation == nil {
		opts.Implementation = &mcp.Implementation{
			Name:    "context-mesh-engine",
			Version: "0.1.0",
		}
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.ReadHeaderTimeout == 0 {
		opts.ReadHeaderTimeout = defaultReadHeaderTimeout
	}
	if opts.APITimeout == 0 {
		opts.APITimeout = defaultAPITimeout
	}
	opts.APIPrefix = normalizeAPIPrefix(opts.APIPrefix)
	return opts
}

func normalizeAPIPrefix(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return APIv1Prefix
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}

func validateAPIPrefix(p string) error {
	if p == "" || p == "/" {
		return fmt.Errorf("APIPrefix %q is not a valid REST path prefix", p)
	}
	if p == MCPPath {
		return fmt.Errorf("APIPrefix must not be %s", MCPPath)
	}
	return nil
}

// MCP returns the shared MCP server. Register tools, prompts, and
// resources on it before serving.
func (e *Engine) MCP() *mcp.Server {
	return e.mcp.Server()
}

// AddController registers a REST controller on the API mux (paths are
// relative to [Options.APIPrefix]).
func (e *Engine) AddController(c api.Controller) {
	e.router.Register(c)
}

// APIPrefix is the REST path prefix after defaults are applied.
func (e *Engine) APIPrefix() string {
	return e.opts.APIPrefix
}

// Handler returns the root HTTP handler with /mcp and the REST prefix
// mounted as siblings. Safe to call more than once; the mux is built once.
func (e *Engine) Handler() http.Handler {
	e.once.Do(func() {
		e.handler = httpserver.NewMux(httpserver.MuxOptions{
			MCPPath:    MCPPath,
			APIPrefix:  e.opts.APIPrefix,
			MCPHandler: e.mcp.Handler(),
			APIHandler: e.router.Handler(),
			APITimeout: e.opts.APITimeout,
		})
	})
	return e.handler
}

// ListenAndServe starts the HTTP server and blocks until ctx is
// cancelled or the listener fails. On cancel it calls Shutdown.
func (e *Engine) ListenAndServe(ctx context.Context) error {
	return httpserver.ListenAndServe(ctx, e.opts.Addr, e.Handler(), e.opts.ReadHeaderTimeout)
}
