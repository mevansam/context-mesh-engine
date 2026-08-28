// Use of this source code is governed by the Apache 2.0 license
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

	// DefaultAPIPrefix is the default REST API prefix used when [Options.APIPrefix] is empty.
	DefaultAPIPrefix = "/api"
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
	// If empty, [DefaultAPIPrefix] ("/api") is used. A leading slash is added
	// if missing; a trailing slash is stripped. Must not be "/" or [MCPPath].
	APIPrefix string

	// DualMCPandREST serves Streamable HTTP at [MCPPath] and REST under
	// APIPrefix. DualMCPandREST, MCPOnly, and RESTOnly are mutually
	// exclusive; at most one may be true.
	//
	// If all three are false (the default), only REST is mounted.
	DualMCPandREST bool

	// MCPOnly serves only Streamable HTTP at [MCPPath]. REST under
	// APIPrefix is not mounted.
	MCPOnly bool

	// RESTOnly serves only REST under APIPrefix. [MCPPath] is not
	// mounted. The MCP server still exists for GET {APIPrefix}/tools
	// and mcp.AddTool. This is the same HTTP surface as the default
	// (all three flags false).
	RESTOnly bool

	// ArazzoLoaders supply Arazzo documents. If empty, no plan tools or
	// plan execute routes are registered. GET {APIPrefix}/openapi (catalog
	// index) is still registered.
	ArazzoLoaders []arazzo.Loader

	// ArazzoExecutor performs backend HTTP calls for workflow steps.
	// If nil, catalog and OpenAPI still load; execute returns 501.
	ArazzoExecutor arazzo.Executor

	// QueryMatcher selects a plan and workflow for MCP query and
	// POST {APIPrefix}/plans/query. Matching (for example vector search
	// against a global registry) is implemented by the application.
	// If nil, the query tool and REST route are not registered. After
	// Match, the engine checks that the plan is loaded in this process.
	QueryMatcher arazzo.QueryMatcher

	// PublicBaseURL is the origin used in REST tool descriptions on
	// GET {APIPrefix}/tools (for example http://localhost:8080). Addr
	// is not substituted. If empty, REST URLs in those descriptions
	// are path-only. Custom MCP templates may still reference URL fields.
	PublicBaseURL string

	// ToolDoc are Go text/templates for generated tool name, title,
	// MCP description, and REST description. Empty fields use
	// [arazzo.DefaultToolDocTemplates]. Per-plan and query title/description
	// templates may be supplied at list time by [Options.ToolHelpLookup].
	ToolDoc arazzo.ToolDocTemplates

	// ToolHelpLookup returns title and description templates for run_* and
	// query tools. Lookups run on MCP tools/list and GET {APIPrefix}/tools,
	// not during [New]. Nil uses [arazzo.DefaultToolHelpLookup] (built-in
	// templates).
	ToolHelpLookup arazzo.ToolHelpLookup

	// ToolHelpCacheTTL is how long a successful help lookup is reused.
	// If zero, [arazzo.DefaultToolHelpCacheTTL] (5m) is used. A negative
	// duration disables caching (every list calls Lookup).
	ToolHelpCacheTTL time.Duration

	// PolicyLoader returns optional OPA inbound/outbound modules for a
	// plan version. Lookups run on execute, not during [New]. Nil skips
	// all policy checks. Do not load .rego files through [Loader].
	PolicyLoader arazzo.PolicyLoader

	// PolicyCacheTTL is how long a compiled policy bundle is reused.
	// If zero, [arazzo.DefaultPolicyCacheTTL] (5m) is used. A negative
	// duration disables caching (every Run loads and compiles).
	PolicyCacheTTL time.Duration

	// RequestPreprocessor builds input.headers / input.auth for OPA from
	// HTTP or MCP headers. Looked up on execute. Nil skips enrichment.
	RequestPreprocessor arazzo.RequestPreprocessor

	// SecretsProvider fetches named secrets for the host Executor and for
	// optional $inputs.secrets.* injection. Nil skips injection.
	SecretsProvider arazzo.SecretsProvider

	// SecretInputs are secret names to copy onto workflow inputs (flattened
	// as secrets.<name>). Empty means do not inject secrets into $inputs.
	SecretInputs []string

	// MCPHandlerWrap wraps the Streamable HTTP handler only (not REST).
	// Use auth.RequireBearerToken here. Nil means no wrap.
	MCPHandlerWrap func(http.Handler) http.Handler

	// RESTHandlerWrap wraps the REST mux after StripPrefix of APIPrefix
	// and before APITimeout. Nil means no wrap. Paths are /health, /plans/...
	RESTHandlerWrap func(http.Handler) http.Handler
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
// health and tools controllers registered under [Options.APIPrefix].
// GET {APIPrefix}/tools returns the MCP tools/list envelope with REST
// descriptions for Arazzo plan/query tools (looked up on demand).
// GET {APIPrefix}/openapi is always registered (catalog index, including
// GET /tools). When [Options.ArazzoLoaders] is set, plans are loaded and
// MCP run_* tools plus REST plan routes and per-plan OpenAPI are
// registered. MCP query and POST {APIPrefix}/plans/query are added only
// when [Options.QueryMatcher] is set. [Options.PolicyLoader] is consulted on execute, not here.
// [Options.DualMCPandREST], [Options.MCPOnly], and
// [Options.RESTOnly] control which HTTP surfaces [Engine.Handler]
// mounts; all false serves REST only. Load or template errors fail
// construction. Help registry I/O is deferred until tools/list.
func New(opts Options) (*Engine, error) {
	opts = applyDefaults(opts)
	if err := validateServeMode(opts); err != nil {
		return nil, err
	}
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
	toolsCtrl := apiv1.NewToolsController(gw.Server())
	router.Register(toolsCtrl)

	if len(opts.ArazzoLoaders) > 0 {
		docCtx := arazzo.NewToolDocContext(
			"plan", "1.0.0", "t", "", "", nil, opts.PublicBaseURL, opts.APIPrefix,
		)
		if _, err := arazzo.RenderToolDoc(opts.ToolDoc, docCtx); err != nil {
			return nil, fmt.Errorf("tool doc templates: %w", err)
		}
		if opts.QueryMatcher != nil {
			if _, err := arazzo.RenderQueryDoc(opts.ToolDoc, docCtx); err != nil {
				return nil, fmt.Errorf("query tool doc templates: %w", err)
			}
		}
		catalog, err := plans.Load(context.Background(), opts.ArazzoLoaders, opts.Logger)
		if err != nil {
			return nil, err
		}
		runner := plans.NewRunner(catalog, opts.ArazzoExecutor, opts.QueryMatcher)
		if opts.PolicyLoader != nil {
			runner.SetPolicy(plans.NewPolicyCache(opts.PolicyLoader, opts.PolicyCacheTTL, opts.Logger))
		}
		runner.SetPreprocessor(opts.RequestPreprocessor)
		runner.SetSecrets(opts.SecretsProvider, opts.SecretInputs)
		help, err := plans.RegisterMCP(gw.Server(), catalog, runner, plans.RegisterMCPConfig{
			Templates:     opts.ToolDoc,
			HelpLookup:    opts.ToolHelpLookup,
			HelpCacheTTL:  opts.ToolHelpCacheTTL,
			Logger:        opts.Logger,
			PublicBaseURL: opts.PublicBaseURL,
			APIPrefix:     opts.APIPrefix,
		})
		if err != nil {
			return nil, err
		}
		gw.Server().AddReceivingMiddleware(help.ReceivingMiddleware())
		toolsCtrl.SetToolHelpOverlay(help.ApplyREST)
		router.Register(apiv1.NewPlansController(catalog, runner))
	} else {
		router.Register(apiv1.NewPlansController(nil, nil))
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
	if opts.ToolHelpLookup == nil {
		opts.ToolHelpLookup = arazzo.DefaultToolHelpLookup()
	}
	if opts.ToolHelpCacheTTL == 0 {
		opts.ToolHelpCacheTTL = arazzo.DefaultToolHelpCacheTTL
	}
	if opts.PolicyCacheTTL == 0 {
		opts.PolicyCacheTTL = arazzo.DefaultPolicyCacheTTL
	}
	return opts
}

func normalizeAPIPrefix(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return DefaultAPIPrefix
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}

func validateServeMode(opts Options) error {
	n := 0
	if opts.DualMCPandREST {
		n++
	}
	if opts.MCPOnly {
		n++
	}
	if opts.RESTOnly {
		n++
	}
	if n > 1 {
		return fmt.Errorf("DualMCPandREST, MCPOnly, and RESTOnly are mutually exclusive")
	}
	return nil
}

func (e *Engine) serveMCP() bool {
	return e.opts.DualMCPandREST || e.opts.MCPOnly
}

func (e *Engine) serveREST() bool {
	return !e.opts.MCPOnly
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

// Handler returns the root HTTP handler. By default only the REST prefix
// is mounted. [Options.DualMCPandREST] mounts /mcp and REST;
// [Options.MCPOnly] mounts only /mcp; [Options.RESTOnly] mounts only REST.
// Safe to call more than once; the mux is built once.
func (e *Engine) Handler() http.Handler {
	e.once.Do(func() {
		opts := httpserver.MuxOptions{
			MCPPath:    MCPPath,
			APIPrefix:  e.opts.APIPrefix,
			APITimeout: e.opts.APITimeout,
		}
		if e.serveMCP() {
			opts.MCPHandler = e.mcp.Handler()
			opts.WrapMCP = e.opts.MCPHandlerWrap
		}
		if e.serveREST() {
			opts.APIHandler = e.router.Handler()
			opts.WrapREST = e.opts.RESTHandlerWrap
		}
		e.handler = httpserver.NewMux(opts)
	})
	return e.handler
}

// ListenAndServe starts the HTTP server and blocks until ctx is
// cancelled or the listener fails. On cancel it calls Shutdown.
func (e *Engine) ListenAndServe(ctx context.Context) error {
	return httpserver.ListenAndServe(ctx, e.opts.Addr, e.Handler(), e.opts.ReadHeaderTimeout)
}
