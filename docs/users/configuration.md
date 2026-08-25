# Configuration

Construct an [`engine.Engine`](https://pkg.go.dev/github.com/mevansam/context-mesh-engine/engine), set [`Options`](#options-reference), then either let the engine own the listener or mount [`Handler()`](#start-the-server) on your `http.Server`.

Canonical types live in Go source (godoc). This page is the usage contract for humans and coding agents.

## Public packages

| Package | Role |
| --- | --- |
| `github.com/mevansam/context-mesh-engine/engine` | Listener, mux, shared `mcp.Server` |
| `github.com/mevansam/context-mesh-engine/api` | `Controller`, JSON helpers, `HealthResponse` |
| `github.com/mevansam/context-mesh-engine/arazzo` | `Loader`, `Executor`, `QueryMatcher`, tool documentation |
| `github.com/modelcontextprotocol/go-sdk/mcp` | Tools, prompts, resources, Streamable HTTP client |

`internal/` is not part of the SDK. Do not import it from an application.

```go
const (
    DefaultAddr      = "localhost:8080"
    MCPPath          = "/mcp"
    DefaultAPIPrefix = "/api"
)

func New(opts Options) (*Engine, error)
func (e *Engine) MCP() *mcp.Server
func (e *Engine) AddController(c api.Controller)
func (e *Engine) APIPrefix() string
func (e *Engine) Handler() http.Handler
func (e *Engine) ListenAndServe(ctx context.Context) error
```

Register extra MCP tools and REST controllers **before** serving. `Handler()` builds the root mux once (`sync.Once`). `AddController` still mutates the live REST mux after `Handler()`; prefer registering first.

Do **not** call `mcp.Server.Run`. Streamable HTTP creates sessions per HTTP connection. `Run` is for stdio / single-session transports.

## Create an engine

```go
e, err := engine.New(engine.Options{
    Addr: "localhost:8080",
    Implementation: &mcp.Implementation{
        Name:    "my-app",
        Version: "1.0.0",
    },
    DualMCPandREST: true,
})
if err != nil {
    log.Fatal(err)
}
```

Zero-value `Options` is valid: REST only at `/api`, listen address `localhost:8080`, built-in health and tools routes, no Arazzo plans.

`New` returns an error when:

- More than one of `DualMCPandREST`, `MCPOnly`, and `RESTOnly` is true.
- `APIPrefix` normalizes to `/` or `/mcp`.
- `ArazzoLoaders` is non-empty and templates fail to parse, specs fail to load, `(planId, version)` is duplicated, or rendered MCP tool names collide.

## Options reference

Fields are grouped by concern. Unlisted zero values use the defaults in the tables.

### Listener

| Field | Type | Default (zero / empty) | Description |
| --- | --- | --- | --- |
| `Addr` | `string` | `localhost:8080` (`DefaultAddr`) | Bind address for [`ListenAndServe`](#engine-owned-listener). Ignored if you serve `Handler()` yourself. |
| `ReadHeaderTimeout` | `time.Duration` | `10s` | Set on the engine-owned `http.Server`. Zero means that default. |
| `Logger` | `*slog.Logger` | `slog.Default()` | HTTP and MCP handler logs. |

`WriteTimeout` is **never** set on the engine-owned server. A short write timeout kills GET SSE on `/mcp`.

### Serve modes

At most one of these may be true. If all three are false, the HTTP surface is REST only (same as `RESTOnly`).

| Field | Effect |
| --- | --- |
| `DualMCPandREST` | Mount Streamable HTTP at `/mcp` **and** REST under `APIPrefix`. |
| `MCPOnly` | Mount only `/mcp`. Health, tools, and plan REST are not served. The MCP server still exists so you can `AddTool`. |
| `RESTOnly` | Mount only REST. `/mcp` is not served. `GET {APIPrefix}/tools` still lists MCP tools over an in-memory session. |

Advertise MCP at **`/mcp`** (no trailing slash). `/mcp/` is mounted so extra path segments reach the same handler. Do not `http.StripPrefix("/mcp", ...)` yourself.

### MCP server

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `Implementation` | `*mcp.Implementation` | `Name: "context-mesh-engine"`, `Version: "0.1.0"` | Identity returned to MCP clients on initialize. |
| `SessionTimeout` | `time.Duration` | `0` (never) | Closes idle MCP sessions. Zero leaves the go-sdk default (sessions stay open). |

### REST

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `APIPrefix` | `string` | `/api` (`DefaultAPIPrefix`) | REST path prefix for health, tools, plans, OpenAPI, and your controllers. A leading slash is added if missing; a trailing slash is stripped. Must not be `/` or `/mcp`. |
| `APITimeout` | `time.Duration` | `15s` | Per-request timeout for the REST prefix only. Zero means that default. A **negative** duration disables the timeout. |

`e.APIPrefix()` returns the prefix after defaults are applied.

REST errors from this SDK use `{"error":"<message>"}` (`api.ErrorBody`), except `http.TimeoutHandler` on the REST prefix, which writes plain text `request timeout\n`.

### Arazzo plans

These fields are optional. Empty `ArazzoLoaders` means no `run_*` tools and no `/plans` or `/openapi` routes. `GET {APIPrefix}/tools` is always registered.

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `ArazzoLoaders` | `[]arazzo.Loader` | none | Spec sources. Implement [`Loader`](adapters.md#loader) or use `arazzo.NewFileLoader`. |
| `ArazzoExecutor` | `arazzo.Executor` | `nil` | Backend HTTP for workflow steps. Nil: catalog and OpenAPI still load; execute is **501**. See [`Executor`](adapters.md#executor). |
| `QueryMatcher` | `arazzo.QueryMatcher` | `nil` | Plan selection for MCP `query` and `POST {APIPrefix}/plans/query`. Nil: those surfaces are **not** registered. See [`QueryMatcher`](adapters.md#querymatcher). |
| `PublicBaseURL` | `string` | empty | Origin written into REST tool descriptions (for example `http://localhost:8080`). Empty → path-only URLs (`{APIPrefix}/...`). **Not** derived from `Addr` except in `cmd/engine` (`-public-base-url` or `http://` + `-addr`). |

How documents are loaded, executed, and exposed: [Arazzo plans](arazzo.md).

### Tool documentation

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `ToolDoc` | `arazzo.ToolDocTemplates` | [`DefaultToolDocTemplates()`](adapters.md#tooldoctemplates) | Go `text/template` recipes for tool **name** (shared) plus MCP vs REST **descriptions**. Empty fields use the built-in recipes. |
| `ToolHelpLookup` | `arazzo.ToolHelpLookup` | [`DefaultToolHelpLookup()`](adapters.md#toolhelplookup) | Per-plan and query **title/description** templates. Looked up on `tools/list` and `GET /tools`, not during `New`. Nil uses empty help (built-in / `ToolDoc` recipes). |
| `ToolHelpCacheTTL` | `time.Duration` | `5m` (`arazzo.DefaultToolHelpCacheTTL`) | How long a successful help lookup is reused. Zero in `Options` means that default. A **negative** duration disables caching (every list calls `Lookup`). |

Names stay on `ToolDoc.Name` / `QueryName`. They are never supplied by `ToolHelpLookup`.

## Start the server

### Engine-owned listener

`ListenAndServe` binds `Options.Addr`, serves `Handler()`, and on context cancel calls `Shutdown` with a **10s** timeout:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
if err := e.ListenAndServe(ctx); err != nil {
    log.Fatal(err)
}
```

Use this when the process does not already own an HTTP server. Example: [minimal](examples.md#minimal).

### Your own HTTP server

Call `e.Handler()`, set `ReadHeaderTimeout`, leave `WriteTimeout` unset:

```go
srv := &http.Server{
    Addr:              "localhost:8080",
    Handler:           e.Handler(),
    ReadHeaderTimeout: 10 * time.Second,
    // Do not set WriteTimeout.
}
log.Fatal(srv.ListenAndServe())
```

Use this when you already own listen, TLS, or shutdown. Example: [embed-handler](examples.md#embed-handler).

Do **not** wrap `e.Handler()` in Gin, a buffering logger, gzip that buffers, or `http.TimeoutHandler`. Those hide `http.Flusher` and break GET SSE. REST timeouts are already applied under `APIPrefix` inside the engine.

## HTTP routes

Paths below use the default REST prefix `/api`. Replace it with `Options.APIPrefix` when you set one. Controller patterns are always relative to the prefix (`GET /health` → `GET {APIPrefix}/health`).

| Method | Path | When | Result |
| --- | --- | --- | --- |
| POST | `/mcp` | `DualMCPandREST` or `MCPOnly` | JSON-RPC into the MCP server (default response `text/event-stream`) |
| GET | `/mcp` | same | Standalone SSE (`Mcp-Session-Id` and `Accept: text/event-stream`) |
| DELETE | `/mcp` | same | End the MCP session |
| GET | `/api/health` | REST mounted | `{"status":"ok"}` (`api.HealthResponse`) |
| GET | `/api/tools` | REST mounted | MCP `tools/list` envelope (`ttlMs`, `cacheScope`, `tools`). Optional `?cursor=` |
| POST | `/api/plans/query` | loaders **and** `QueryMatcher` | Natural-language match + execute |
| POST | `/api/plans/{planId}/{workflowId}` | loaders | Execute **latest** version; body is workflow inputs |
| POST | `/api/plans/{planId}/{version}/{workflowId}` | loaders | Execute that version (`{version}` is `v` + `info.version`) |
| GET | `/api/openapi/{planId}` | loaders | OAS 3.1 for latest execute paths |
| GET | `/api/openapi/{planId}/{version}` | loaders | OAS 3.1 for that version |
| * | `/api/...` | REST mounted | Your [`Controller`](adapters.md#rest-controllers) routes |

`GET {APIPrefix}/tools` lists every tool on the shared MCP server: `run_*` when loaders are set, `query` when `QueryMatcher` is set, and anything you add with `mcp.AddTool`. Names and schemas match Streamable HTTP; Arazzo plan/query `description` fields use REST templates.

Plan contracts: [Arazzo plans](arazzo.md).

## Add MCP tools

Register on the shared server returned by `e.MCP()` **before** listen:

```go
type EchoInput struct {
    Text string `json:"text" jsonschema:"text to echo"`
}

func echo(_ context.Context, _ *mcp.CallToolRequest, in EchoInput) (*mcp.CallToolResult, any, error) {
    return &mcp.CallToolResult{
        Content: []mcp.Content{&mcp.TextContent{Text: in.Text}},
    }, nil, nil
}

mcp.AddTool(e.MCP(), &mcp.Tool{Name: "echo", Description: "echo text"}, echo)
```

Schema inference and argument validation are the go-sdk’s job. Do not wrap the MCP handler in buffering middleware.

## Add REST controllers

```go
type ItemsController struct{}

func (c *ItemsController) Register(mux *http.ServeMux) {
    mux.HandleFunc("GET /items", c.list)
}

func (c *ItemsController) list(w http.ResponseWriter, r *http.Request) {
    api.WriteJSON(w, http.StatusOK, map[string]any{"items": []string{}})
}

e.AddController(&ItemsController{})
```

The mux passed to `Register` is already stripped of `APIPrefix`. Pattern `GET /items` is `GET {APIPrefix}/items`. Use method-aware patterns (Go 1.22+ `ServeMux`). JSON helpers and the `Controller` contract: [adapters](adapters.md#rest-controllers).

## MCP client

Requires `DualMCPandREST` or `MCPOnly`.

```go
client := mcp.NewClient(&mcp.Implementation{Name: "demo", Version: "0.0.1"}, nil)
session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
    Endpoint: "http://localhost:8080/mcp",
}, nil)
if err != nil {
    log.Fatal(err)
}
defer session.Close()

res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "ping"})
```

Custom HTTP clients talking to `/mcp` must send:

- POST: `Content-Type: application/json` and `Accept: application/json, text/event-stream`
- GET SSE: `Accept: text/event-stream` and `Mcp-Session-Id`

## Auth

This SDK does not implement authentication. To require a bearer token, wrap **only** the MCP handler before mount, as in the [go-sdk auth example](https://github.com/modelcontextprotocol/go-sdk/blob/main/examples/auth/server/main.go). Do not put that middleware on the REST mux unless you intend it.

## Checklist

- Import `engine`, `api`, `arazzo`, and `mcp` only — never `internal/`.
- Always handle `engine.New`’s error.
- Advertise MCP at `/mcp` (not `/mcp/`).
- Leave `WriteTimeout` unset; do not wrap the root handler in Gin / `TimeoutHandler`.
- Do not call `mcp.Server.Run`.
- Arazzo execute needs an `Executor`; OpenAPI GET does not.
- `PublicBaseURL` is a separate option from `Addr`.

## Next

- [Adapters](adapters.md) — implement loaders, executors, matchers, and tool help.
- [Arazzo plans](arazzo.md) — spec rules, MCP/REST contracts, OpenAPI.
- [Examples](examples.md)
