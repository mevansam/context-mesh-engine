# Repository layout

Module: `github.com/mevansam/context-mesh-engine`

```text
context-mesh-engine/
  README.md                      Entry; points at docs/
  LICENSE                        Apache License 2.0
  go.mod / go.sum                replace -> ../go-sdk and ../libopenapi
  docs/                          users/ vs contributors/ (start at docs/README.md)
  cmd/engine/                    Sample process: -addr, -api-prefix, -dual, -mcp-only, -rest-only, -specs, -public-base-url, SIGINT, ping
  examples/
    README.md                    Index; each example has its own README
    minimal/                     ListenAndServe
    embed-handler/               Your http.Server + Handler()
    arazzo-fs/                   FileLoader (plans dir arg) + stub Executor + dummy QueryMatcher
    petstore/                    Petstore e2e: mcp-server + async-order-server
  engine/                        Public facade: New, Handler, ListenAndServe
  api/                           Public facade: Controller, JSON helpers, HealthResponse
  arazzo/                        Public Loader, FileLoader, PolicyLoader, Executor, QueryMatcher, ToolDoc, ToolHelpLookup
  testdata/arazzo/
    plans/                       Arazzo fixtures (FileLoader root; not testdata/arazzo/)
    sources/openapi.yaml         OpenAPI referenced as ../sources/openapi.yaml
  internal/
    httpserver/server.go         http.Server, root mux, Shutdown
    mcpgw/server.go              mcp.NewServer + StreamableHTTPHandler
    api/json.go                  WriteJSON / ReadJSON implementation
    api/v1/router.go             REST ServeMux (mounted at Options.APIPrefix)
    api/v1/health.go             GET /health
    api/v1/tools.go              GET /tools (MCP envelope; REST descriptions for Arazzo tools)
    api/v1/plans.go              GET /openapi, POST /plans, GET /openapi/{planId}
    plans/                       Catalog, runner, MCP tools, OAS generator, OPA eval
    ttlcache/                    Generic singleflight TTL cache (help + policy)
```

## Import rules

| Path | Import | Audience |
| --- | --- | --- |
| `engine` | `github.com/mevansam/context-mesh-engine/engine` | SDK users |
| `api` | `github.com/mevansam/context-mesh-engine/api` | SDK users |
| `arazzo` | `github.com/mevansam/context-mesh-engine/arazzo` | SDK users |
| `internal/...` | **never** from apps or examples that are copy/paste samples except this module | contributors |
| `cmd/engine` | not imported | sample binary |
| `examples/*` | not imported | copy/paste |

`engine` and `api` stay thin. Mux rules, Streamable HTTP construction, REST routing, JSON encoding, and Arazzo catalog live under `internal/`.

`internal/api/v1` must not import public `api` for the `Controller` interface (cycle). It uses a local `controller` interface with the same `Register(*http.ServeMux)` method. Public `Engine.AddController` still takes `api.Controller`.

## File map (public)

| File | Responsibility |
| --- | --- |
| `engine/engine.go` | `Options` (incl. `APIPrefix`, `DualMCPandREST`, `MCPOnly`, `RESTOnly`), `New` (`error`), `MCP`, `AddController`, `APIPrefix`, `Handler`, `ListenAndServe` |
| `engine/engine_test.go` | Mux contract: health JSON, custom prefix, MCP handshake, SSE GET 400, REST ≠ MCP |
| `engine/arazzo_test.go` | Plan MCP tools, REST execute, OpenAPI, query matcher, 501 |
| `api/controller.go` | `Controller` |
| `api/json.go` | Re-exports of `WriteJSON`, `WriteError`, `ReadJSON`, `ErrorBody` |
| `api/health.go` | `HealthResponse` |
| `arazzo/loader.go` | `Loader`, `Source`, `Executor` / request / response aliases |
| `arazzo/fileloader.go` | Recursive filesystem loader; `BaseURL` trailing slash |
| `arazzo/policy.go` | `PolicyLoader`, `PolicyBundle`, `PolicyHintsKey` |
| `arazzo/request.go` | `RequestPreprocessor`, `PolicyRequestContext` |
| `arazzo/secrets.go` | `SecretsProvider`, `MapSecrets` |
| `arazzo/filepolicy.go` | Filesystem policy layout `{planId}/{version}/*.rego` |
| `arazzo/tooldoc.go` | Templates + `ToolDocContext` |
| `arazzo/toolhelp.go` | `ToolHelpLookup` + overlay |
| `arazzo/matcher.go` | `QueryMatcher`, `PlanCatalog`, `QueryMatch` |

## File map (`internal/`)

| File | Responsibility |
| --- | --- |
| `httpserver/server.go` | Sibling mount of `/mcp` and `Options.APIPrefix` (optional omit either); `ListenAndServe` + 10s `Shutdown` |
| `mcpgw/server.go` | Shared `mcp.Server`; `Stateless`/`JSONResponse` left false |
| `api/json.go` | JSON encode/decode; `ReadJSON` unknown fields rejected; 1 MiB |
| `api/v1/router.go` | v1 `ServeMux` and `Register` |
| `api/v1/health.go` | Default `GET /health` |
| `api/v1/tools.go` | `GET /tools` (MCP envelope; REST descriptions for Arazzo tools) |
| `api/v1/plans.go` | `GET /openapi` (always), `POST /plans/query`, `POST /plans/...`, `GET /openapi/{planId}`; sanitized errors |
| `plans/catalog.go` | Load, skip, duplicate, `ResolveSources`, latest |
| `plans/runner.go` | New libopenapi Engine per `Run`/`Query`; inbound/outbound OPA; secrets inject; preprocessor enrich; closed inputs |
| `plans/policy.go` | Compile/eval OPA; TTL cache via `internal/ttlcache`; `input.auth` / `input.headers` |
| `plans/request.go` | HTTP/MCP → `RequestSource` |
| `plans/redact.go` | RFC 6901 redaction of workflow outputs |
| `plans/schema.go` | MCP `inputSchema` oneOf + workflowId const; strip/close consumer inputs |
| `plans/public.go` | `ClassifyError` / `LogAndPublic` |
| `plans/openapi.go` | OAS 3.1 catalog + per-plan JSON; prefix-absolute `$ref`; `servers` from PublicBaseURL+APIPrefix |
| `plans/mcp.go` | `query` + `run_*` tools |
| `plans/help.go` | Help TTL cache + `tools/list` overlay (`internal/ttlcache`) |
| `ttlcache/cache.go` | Generic singleflight TTL cache |

## Tests as the contract

| Test | Do not break |
| --- | --- |
| `TestHandler_HealthJSON` | `{APIPrefix}/health` (default `/api/health`) is JSON |
| `TestHandler_ToolsListJSON` | `{APIPrefix}/tools` lists MCP tools |
| `TestHandler_ToolsListEmpty` | empty tools + `GET {APIPrefix}/openapi` describes `/tools` without plan `$ref`s |
| `TestHandler_CustomAPIPrefix` | custom prefix serves health; default `/api` is 404 |
| `TestNew_APIPrefixRejected` | `/`, `/mcp` fail `New` |
| `TestNew_ServeModesMutuallyExclusive` | more than one of Dual/MCPOnly/RESTOnly fails `New` |
| `TestHandler_DefaultRESTOnly` | default mounts REST; `/mcp` is 404 |
| `TestHandler_DualMCPandREST` | Dual mounts `/mcp` and REST |
| `TestHandler_MCPOnly` | `/mcp` works; REST prefix is 404 |
| `TestHandler_RESTOnly` | REST works; `/mcp` is 404 |
| `TestHandler_MCPInitializeAndPing` | Streamable HTTP at `/mcp` |
| `TestHandler_MCPGETRequiresSession` | GET `/mcp` without session is 400 (handler is mounted) |
| `TestHandler_RESTNotMCP` | POST `/api/health` is REST 405, not MCP |
| `TestArazzo_*` | plans, catalog/per-plan OpenAPI, 501, MCP `query`/`run_*`, `POST /plans/query` |
| `internal/plans` tests | skip missing `x-planId`, reject `v`-prefixed / non-semver `info.version`, duplicate versions, latest `1.1.0` |

## What not to add here

- Vendored copies of go-sdk or libopenapi (they are module dependencies)
- Gin/chi as the **root** mux
- Legacy `mcp.NewSSEHandler` (2024-11-05) unless a later phase explicitly needs old clients
- New public types in `engine`/`api`/`arazzo` that belong in `internal/`
- `StripPrefix` on `/mcp`

## Next

[Code design](design.md). Arazzo internals: [arazzo.md](arazzo.md).
