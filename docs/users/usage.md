# SDK usage

Public import paths (the SDK contract):

| Package | Role |
| --- | --- |
| `github.com/mevansam/context-mesh-engine/engine` | Construct the listener, mux, and shared `mcp.Server` |
| `github.com/mevansam/context-mesh-engine/api` | `Controller`, JSON helpers, `HealthResponse` |
| `github.com/mevansam/context-mesh-engine/arazzo` | Arazzo `Loader`, `FileLoader`, `Executor`, `QueryMatcher`, tool-doc templates, `ToolHelpLookup` |
| `github.com/modelcontextprotocol/go-sdk/mcp` | Tools, prompts, resources, Streamable HTTP client |

`internal/` is not part of the SDK contract. Do not import it from an application.

Canonical types live in the Go source (godoc). This page is the usage contract for humans and coding agents.

## Public API (engine)

```go
const (
    DefaultAddr = "localhost:8080"
    MCPPath     = "/mcp"
    DefaultAPIPrefix = "/api"
)

func New(opts Options) (*Engine, error)
func (e *Engine) MCP() *mcp.Server
func (e *Engine) AddController(c api.Controller)
func (e *Engine) APIPrefix() string
func (e *Engine) Handler() http.Handler
func (e *Engine) ListenAndServe(ctx context.Context) error
```

`DefaultAPIPrefix` is the **default** REST prefix (`/api`). Override it with `Options.APIPrefix` (or `cmd/engine -api-prefix`). `e.APIPrefix()` is the prefix after defaults (leading `/` added, trailing `/` stripped).

Register extra MCP tools and REST controllers **before** serving. `Handler()` builds the root mux once (`sync.Once`). `AddController` still works after `Handler()` because it mutates the live REST mux; prefer registering before listen.

Do **not** call `mcp.Server.Run`. Streamable HTTP creates sessions per HTTP connection. `Run` is for stdio / single-session transports.

## Options

Zero-value `engine.Options` after `New` (see `engine/engine.go`):

| Field | Zero / empty means | Default |
| --- | --- | --- |
| `Addr` | empty | `localhost:8080` (`DefaultAddr`) |
| `Implementation` | nil | `Name: "context-mesh-engine"`, `Version: "0.1.0"` |
| `Logger` | nil | `slog.Default()` |
| `SessionTimeout` | 0 | idle MCP sessions never closed (go-sdk default) |
| `APITimeout` | 0 | 15s on the REST prefix only. Set a **negative** duration to disable. |
| `ReadHeaderTimeout` | 0 | 10s on `http.Server` |
| `APIPrefix` | empty | `/api` (`DefaultAPIPrefix`). Must not be `/` or `/mcp`. |
| `MCPOnly` | false | if true, mount only `/mcp` |
| `RESTOnly` | false | if true, mount only REST (same HTTP surface as the default) |
| `DualMCPandREST` | false | if true, mount `/mcp` and REST. Default (all three false) is REST only |
| `ArazzoLoaders` | nil/empty | no plan tools or plan REST routes |
| `ArazzoExecutor` | nil | catalog + OpenAPI still load; execute is 501 |
| `QueryMatcher` | nil | MCP `query` and `POST /plans/query` are not registered |
| `PublicBaseURL` | empty | REST URLs in `GET /tools` descriptions are path-only (`{APIPrefix}/...`) |
| `ToolDoc` | zero struct | `arazzo.DefaultToolDocTemplates()` |
| `ToolHelpLookup` | nil | `arazzo.DefaultToolHelpLookup()` (built-in templates; no I/O at `New`) |
| `ToolHelpCacheTTL` | 0 | 5m (`arazzo.DefaultToolHelpCacheTTL`). Set a **negative** duration to always refresh. |

`WriteTimeout` is **never** set on the engine-owned `http.Server`. A short write timeout kills GET SSE.

`PublicBaseURL` is **not** derived from `Addr` except in `cmd/engine` (`-public-base-url` or `http://` + `-addr`).

```go
e, err := engine.New(engine.Options{
    Addr: "localhost:8080",
    Implementation: &mcp.Implementation{
        Name:    "my-app",
        Version: "1.0.0",
    },
})
if err != nil {
    log.Fatal(err)
}
```

`New` returns an error when `APIPrefix` is `/` or `/mcp` (after normalize), when more than one of `DualMCPandREST`, `MCPOnly`, and `RESTOnly` is true, or when `ArazzoLoaders` is non-empty and templates fail to parse, specs fail to load, `(planId, version)` is duplicated, or rendered MCP tool names collide.

## Routes

Paths below use the **default** REST prefix `/api`. Replace that prefix with `Options.APIPrefix` when you set one. Controller patterns are always relative to the prefix (`GET /health` → `GET {APIPrefix}/health`).

| Method | Path | Result |
| --- | --- | --- |
| POST | `/mcp` | JSON-RPC into the MCP server (default response `text/event-stream`) |
| GET | `/mcp` | Standalone SSE (requires `Mcp-Session-Id` and `Accept: text/event-stream`) |
| DELETE | `/mcp` | End the MCP session |
| GET | `/api/health` | `{"status":"ok"}` (`api.HealthResponse`) |
| GET | `/api/tools` | MCP `tools/list` envelope (`ttlMs`, `cacheScope`, `tools`). Arazzo plan/query descriptions are REST-specific (`ToolHelpLookup`, cached). Optional `?cursor=` |
| POST | `/api/plans/query` | Natural-language match + execute (same contract as MCP `query`; loaders **and** `QueryMatcher` required) |
| POST | `/api/plans/{planId}/{workflowId}` | Execute **latest** plan version (loaders required) |
| POST | `/api/plans/{planId}/{version}/{workflowId}` | Execute that version (`{version}` is `v` + `info.version`) |
| GET | `/api/openapi/{planId}` | OAS 3.1 for latest execute paths |
| GET | `/api/openapi/{planId}/{version}` | OAS 3.1 for that version |
| * | `/api/...` | Your controllers |

Advertise MCP at **`/mcp`** (no trailing slash) when `DualMCPandREST` or `MCPOnly` is set. `/mcp/` is mounted so extra path segments still reach the same handler. Do not `http.StripPrefix("/mcp", ...)` yourself. Default (all serve-mode flags false) is REST only. `RESTOnly` is the same HTTP surface as the default. The three flags are mutually exclusive.

Plan routes exist only after `New` with non-empty `ArazzoLoaders`. Details: [arazzo.md](arazzo.md). `GET {APIPrefix}/tools` is always registered and lists every tool on the shared MCP server (including `run_*` when loaders are set, `query` when `QueryMatcher` is set, and any tools you add with `mcp.AddTool`). Names and schemas match Streamable HTTP `tools/list`; Arazzo plan/query `description` fields use REST templates. MCP `query` and `POST {APIPrefix}/plans/query` are omitted unless `Options.QueryMatcher` is set.

REST errors from this SDK use `{"error":"<message>"}` (`api.ErrorBody`) except `http.TimeoutHandler` on the REST prefix, which writes plain text `request timeout\n`.

## Public API (api)

```go
type Controller interface {
    Register(mux *http.ServeMux)
}

type HealthResponse struct {
    Status string `json:"status"`
}

type ErrorBody struct {
    Error string `json:"error"`
}

func WriteJSON(w http.ResponseWriter, status int, v any)
func WriteError(w http.ResponseWriter, status int, msg string)
func ReadJSON(r *http.Request, v any) error
```

`ReadJSON`: rejects unknown JSON fields; caps the body at **1 MiB**. Plan execute POST does **not** use `ReadJSON` (unknown fields allowed; same 1 MiB cap).

The mux passed to `Controller.Register` is already stripped of `Options.APIPrefix`. Pattern `GET /items` is `GET {APIPrefix}/items` (default `GET /api/items`).

## Add MCP tools

Register on the shared server returned by `e.MCP()`:

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

Schema inference and argument validation are go-sdk’s job (`mcp.AddTool`). Do not wrap the MCP handler in buffering middleware.

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

Use method-aware patterns (`GET /items`). The root mux is stdlib `ServeMux` (Go 1.22+).

## Run vs embed

**Engine-owned listener** (signals, shutdown). On context cancel, `ListenAndServe` calls `Shutdown` with a **10s** timeout:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
if err := e.ListenAndServe(ctx); err != nil {
    log.Fatal(err)
}
```

See [`examples/minimal`](../../examples/minimal/README.md).

**Your `http.Server`:**

```go
srv := &http.Server{
    Addr:              "localhost:8080",
    Handler:           e.Handler(),
    ReadHeaderTimeout: 10 * time.Second,
    // Do not set WriteTimeout.
}
log.Fatal(srv.ListenAndServe())
```

See [`examples/embed-handler`](../../examples/embed-handler/README.md).

Do **not** wrap `e.Handler()` in Gin, a buffering logger, gzip that buffers, or `http.TimeoutHandler`. Those hide `http.Flusher` and break GET SSE. REST timeouts are already applied under `Options.APIPrefix` inside the engine.

## MCP client

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

## Auth (not implemented here)

To require a bearer token later, wrap **only** the MCP handler before mount, as in the [go-sdk auth example](https://github.com/modelcontextprotocol/go-sdk/blob/main/examples/auth/server/main.go). Do not put that middleware on the REST mux unless you intend it.

## Coding-agent checklist (users)

- Import `engine`, `api`, `arazzo`, and `mcp` only — never `internal/`.
- Always handle `engine.New`’s error.
- Advertise MCP at `/mcp` (not `/mcp/`).
- Leave `WriteTimeout` unset; do not wrap the root handler in Gin/`TimeoutHandler`.
- Do not call `mcp.Server.Run`.
- Arazzo execute needs an `arazzo.Executor`; OpenAPI GET does not.
- `PublicBaseURL` is a separate option from `Addr`.

## Next

- [Arazzo plans](arazzo.md)
- [Examples](examples.md)
- Mux internals (contributors only): [code design](../contributors/design.md)
