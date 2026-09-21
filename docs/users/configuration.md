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
- `CommonRESTParams` has an empty name, `in` other than `header` or `cookie`, a duplicate name, or a name that collides with a workflow `x-source` REST lift.
- `OpenAPISecuritySchemes` or `OpenAPISecurity` is invalid (missing name/type, unknown scheme, oauth2 scope not declared on the scheme), a scheme collides with `CommonRESTParams` or an `x-source` REST lift, or a workflow `x-security` names an unknown scheme.
- `ArazzoLoaders` is non-empty and templates fail to parse, specs fail to load, `info.version` is not semver or starts with `v`, `(planId, version)` is duplicated, or rendered MCP tool names collide.

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

These fields are optional. Empty `ArazzoLoaders` means no `run_*` tools and no `/plans` execute routes or per-plan OpenAPI. `GET {APIPrefix}/tools` and `GET {APIPrefix}/openapi` (catalog index) are always registered.

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `ArazzoLoaders` | `[]arazzo.Loader` | none | Spec sources. Implement [`Loader`](adapters.md#loader) or use `arazzo.NewFileLoader`. |
| `ArazzoExecutor` | `arazzo.Executor` | `nil` | Backend HTTP for workflow steps. Nil: catalog and OpenAPI still load; execute is **501**. See [`Executor`](adapters.md#executor). |
| `QueryMatcher` | `arazzo.QueryMatcher` | `nil` | Plan selection for MCP `query` and `POST {APIPrefix}/plans/query`. Nil: those surfaces are **not** registered. See [`QueryMatcher`](adapters.md#querymatcher). |
| `PublicBaseURL` | `string` | empty | Origin written into REST tool descriptions (for example `http://localhost:8080`). Empty → path-only URLs (`{APIPrefix}/...`). **Not** derived from `Addr` except in `cmd/engine` (`-public-base-url` or `http://` + `-addr`). |
| `OpenAPICatalogTitle` | `string` | `Arazzo plan catalog` | `info.title` on `GET {APIPrefix}/openapi`. |
| `OpenAPICatalogVersion` | `string` | `1.0.0` | `info.version` on `GET {APIPrefix}/openapi`. Not a plan `info.version`. |
| `CommonRESTParams` | `[]engine.CommonRESTParam` | none | Header/cookie parameters documented on generated OpenAPI for `GET /tools`, `POST /plans/query`, and execute. **Not** bound to Arazzo `$inputs`. See [Request identity](#request-identity). |
| `OpenAPISecuritySchemes` | `[]engine.OpenAPISecurityScheme` | none | OAS 3.1 `components.securitySchemes` on catalog and child specs. Type `http`, `apiKey`, `oauth2`, or `openIdConnect`. The engine does **not** verify tokens from this config; wraps still do. |
| `OpenAPISecurity` | `[]engine.OpenAPISecurityRequirement` | none | Document-level OAS `security` on `GET /tools` and `POST /plans/query`. Child execute operations use workflow [`x-security`](arazzo.md#x-security) when set, otherwise this fallback. Scheme names must exist in `OpenAPISecuritySchemes`. Empty means schemes are documented only. |

How documents are loaded, executed, and exposed: [Arazzo plans](arazzo.md).

### Tool documentation

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `ToolDoc` | `arazzo.ToolDocTemplates` | [`DefaultToolDocTemplates()`](adapters.md#tooldoctemplates) | Go `text/template` recipes for tool **name** (shared) plus MCP vs REST **descriptions**. Empty fields use the built-in recipes. |
| `ToolHelpLookup` | `arazzo.ToolHelpLookup` | [`DefaultToolHelpLookup()`](adapters.md#toolhelplookup) | Per-plan and query **title/description** templates. Looked up on `tools/list` and `GET /tools`, not during `New`. Nil uses empty help (built-in / `ToolDoc` recipes). |
| `ToolHelpCacheTTL` | `time.Duration` | `5m` (`arazzo.DefaultToolHelpCacheTTL`) | How long a successful help lookup is reused. Zero in `Options` means that default. A **negative** duration disables caching (every list calls `Lookup`). |

Names stay on `ToolDoc.Name` / `QueryName`. They are never supplied by `ToolHelpLookup`.

### Policy

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `PolicyLoader` | `arazzo.PolicyLoader` | `nil` | Optional OPA inbound/outbound modules per plan version. Looked up on execute, not during `New`. Nil skips all policy checks. If the value also implements [`SharedPolicySource`](adapters.md#sharedpolicysource), org-wide modules are loaded separately. See [`PolicyLoader`](adapters.md#policyloader). Do not load `.rego` through `ArazzoLoaders`. |
| `PolicyCacheTTL` | `time.Duration` | `5m` (`arazzo.DefaultPolicyCacheTTL`) | How long a compiled bundle is reused. Zero in `Options` means that default. A **negative** duration disables caching (every `Run` loads and compiles). |
| `RequestPreprocessor` | `arazzo.RequestPreprocessor` | `nil` | Builds OPA `input.headers` / `input.auth` from HTTP or MCP headers. Nil skips. See [Request identity](#request-identity). |
| `SecretsProvider` | `arazzo.SecretsProvider` | `nil` | Named secrets for the host Executor (downstream JWT) and optional `$inputs.secrets.*`. |
| `SecretInputs` | `[]string` | `nil` | Secret names to flatten onto workflow inputs. Empty means do not inject into `$inputs`. |
| `MCPHandlerWrap` | `func(http.Handler) http.Handler` | `nil` | Wraps Streamable HTTP only. Use `auth.RequireBearerToken` for the calling-application JWT. |
| `RESTHandlerWrap` | `func(http.Handler) http.Handler` | `nil` | Wraps REST after `APIPrefix` strip, before `APITimeout`. After the strip, paths are `/health`, `/tools`, `/openapi`, `/openapi/{planId}`, `/plans/…`. The engine does not require auth; the host chooses which paths to wrap. |

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
| GET | `/api/openapi` | REST mounted | Catalog OAS 3.1: `GET /tools` plus `$ref` to each latest plan spec |
| POST | `/api/plans/query` | loaders **and** `QueryMatcher` | Natural-language match + execute |
| POST | `/api/tools/{planId}/{workflowId}` | loaders | Execute **latest** version; body is workflow inputs |
| POST | `/api/tools/{planId}/{version}/{workflowId}` | loaders | Execute that version (`{version}` is `v` + `info.version`) |
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

Calling-application OAuth (one `Authorization: Bearer` JWT) is `Options.MCPHandlerWrap` / `Options.RESTHandlerWrap` with go-sdk [`auth.RequireBearerToken`](https://github.com/modelcontextprotocol/go-sdk/blob/main/examples/server/auth-middleware/main.go). Wrap **child** handlers only, never the root mux. The engine does not require a token on REST; which paths need a bearer is a host wrap decision (see `RESTHandlerWrap` after `APIPrefix` strip).

Generated OpenAPI documents schemes via [`OpenAPISecuritySchemes`](#arazzo-plans) and requirements via [`OpenAPISecurity`](#arazzo-plans) / workflow [`x-security`](arazzo.md#x-security). That is documentation plus a **runtime scope check** in `Run` (`TokenInfo.Scopes` vs oauth2/OIDC scopes on `x-security`). The wrap still verifies the JWT. `RequireBearerTokenOptions.Scopes` is optional extra wrap-level enforcement (for example `tools:list` on `GET /tools`); the engine does not set it.

End-user JWTs on `x-*` headers, claim extraction, and remote enrichment are [`RequestPreprocessor`](adapters.md#requestpreprocessor). How those values reach each handler and OPA: [Request identity](#request-identity). Invalid preprocessor → **401**. Missing client token when `x-security` is set → **401**. Missing required scopes → **403** `insufficient scope`. Inbound deny → **403** `policy denied`.

The host `Executor` may mint a **new** downstream JWT from [`SecretsProvider`](adapters.md). Do not put signing keys in `$inputs` unless they are listed in `SecretInputs`.

Petstore walkthrough: [examples/petstore](../../examples/petstore/README.md).

## Request identity

HTTP headers and cookies are **not** copied onto Arazzo `$inputs` unless a workflow property has REST/HTTP [`x-source`](arazzo.md#x-source-rest-vs-mcp). Host-wide headers and cookies that wraps and OPA should see are declared on [`Options.CommonRESTParams`](#arazzo-plans) so generated OpenAPI documents them. `Required` on those params is OpenAPI documentation only; missing values are **not** `400 missing required input`. Enforce in the wrap (`401`) or preprocessor.

OAS **security schemes** are a separate Options field (`OpenAPISecuritySchemes`). Do not also list `Authorization` as a `CommonRESTParam` when using `http` bearer or oauth2 — that collides at `New`. `apiKey` schemes collide with the same `in`+name as a common param or `x-source` lift.

```go
e, err := engine.New(engine.Options{
    CommonRESTParams: []engine.CommonRESTParam{
        {Name: "X-End-User-Token", In: "header", Required: true, Description: "end-user JWT"},
        {Name: "sid", In: "cookie"},
    },
    RESTHandlerWrap:     wrapREST,      // sees *http.Request
    MCPHandlerWrap:      wrapMCP,       // Streamable HTTP only
    RequestPreprocessor: preprocessor,  // execute/query only
})
```

`In` must be `header` or `cookie`. Duplicate names fail `New` (headers compared case-insensitively). A common param that uses the same `in`+name as a workflow `x-source` REST lift also fails `New`. They appear on:

| OpenAPI operation | Documented | Runtime consumer |
| --- | --- | --- |
| `GET /tools` | yes | `RESTHandlerWrap` only |
| `POST /plans/query` and execute `POST /tools/{planId}/…` | yes | wrap, then preprocessor, then OPA |
| MCP `run_*` / `query` (Streamable HTTP) | not in `inputSchema` | `MCPHandlerWrap`, then preprocessor, then OPA |
| `GET /health` | no | wrap sees the request if it does not skip `/health` |

### Handler chain

REST (after `APIPrefix` strip):

```text
RESTHandlerWrap
  → GET /tools          ToolsController (cursor only; no preprocessor)
  → GET /openapi/…      OpenAPI bytes; no preprocessor
  → POST /plans/query
  → POST /tools/{planId}/…
        RequestSourceFromHTTP(r)     // clone of r.Header (Cookie header included)
        EnrichContext → RequestPreprocessor.Process
        bindRESTInputs               // x-source only
        inbound OPA → workflow → outbound OPA
        Executor                     // PolicyRequestFromContext
```

MCP Streamable HTTP (`/mcp`):

```text
MCPHandlerWrap
  → tools/list          no preprocessor
  → tools/call (run_* / query)
        RequestSourceFromMCP(req)    // req.Extra.Header from the HTTP request
        EnrichContext → RequestPreprocessor.Process
        inbound OPA → workflow → outbound OPA
        Executor
```

`GET /tools` opens an **in-memory** MCP `tools/list` session. Headers on the REST GET are **not** forwarded into that session’s `Extra.Header`. Document them so clients send them; read them in `RESTHandlerWrap`.

### What each layer can read

| Layer | Access | Typical use |
| --- | --- | --- |
| `RESTHandlerWrap` | `r.Header.Get("X-End-User-Token")`, `r.Cookie("sid")`. After strip, `r.URL.Path` is `/tools`, `/openapi`, `/plans/…`, `/health`. | Reject missing/invalid client bearer before the engine. Skip `/health` if it must stay open. |
| `MCPHandlerWrap` | Same `*http.Request` on `/mcp` (initialize, `tools/list`, `tools/call`). | Client bearer on Streamable HTTP. |
| `RequestPreprocessor.Process` | `src.Header` (`http.Header`). Cookies are the `Cookie` header (`src.Header.Get("Cookie")`) unless you parse them yourself. `src.ClientAuth` is go-sdk `TokenInfo` after `RequireBearerToken` (`userId`, `scopes`, `expiration`). | Verify extra JWTs, allowlist headers, build `Auth`. Error → **401**. Nil option skips this step. |
| OPA inbound/outbound | Only what the preprocessor returned: `input.headers` (`map[string]string`) and `input.auth` (`map[string]any`). Raw `Authorization` is not copied unless the preprocessor puts it there (do not). | Allow/deny. Plan inbound `hints` → `$inputs.policyHints`. Shared inbound cannot set hints. |
| `Executor` | `arazzo.PolicyRequestFromContext(ctx)` → same `Headers` / `Auth` maps. | Downstream credentials from processed identity, not from `$inputs`. |
| Arazzo `$inputs` | JSON body plus REST/HTTP **`x-source`** lifts (`header` / `cookie` / `query`). | Workflow expressions. Common params are never merged here. |

Preprocessor result is stored on `ctx` with `arazzo.WithPolicyRequest`. `policyEvalInput` copies it into the OPA input object:

```json
{
  "planId": "petstore",
  "version": "1.0.0",
  "workflowId": "purchasePet",
  "inputs": { },
  "headers": { "x-request-id": "demo-1" },
  "auth": { "endUser": { "username": "buyer" }, "client": { "userId": "…" } }
}
```

`headers` / `auth` are omitted when the preprocessor is nil or returned nil maps. Outbound eval uses the same maps plus `outputs`.

Do not put `Authorization` or raw user JWTs in `PolicyRequestContext.Headers`. Allowlist names you want Rego to see. Petstore: [`examples/petstore/mcp-server/auth.go`](../../examples/petstore/mcp-server/auth.go).

## Checklist

- Import `engine`, `api`, `arazzo`, and `mcp` only — never `internal/`.
- Always handle `engine.New`’s error.
- Advertise MCP at `/mcp` (not `/mcp/`).
- Leave `WriteTimeout` unset; do not wrap the root handler in Gin / `TimeoutHandler`.
- Do not call `mcp.Server.Run`.
- Arazzo execute needs an `Executor`; OpenAPI GET does not.
- `PublicBaseURL` is a separate option from `Addr`.
- Declare host-wide headers/cookies on `CommonRESTParams`; read them in wraps / `RequestPreprocessor`, not `$inputs`.
- Declare OAS schemes on `OpenAPISecuritySchemes` and catalog requirements on `OpenAPISecurity`; put per-workflow scopes on `x-security`. Copy wrap-verified `TokenInfo.Scopes` so `Run` can enforce them.

## Next

- [Adapters](adapters.md) — implement loaders, executors, matchers, and tool help.
- [Arazzo plans](arazzo.md) — spec rules, MCP/REST contracts, OpenAPI.
- [Examples](examples.md)
