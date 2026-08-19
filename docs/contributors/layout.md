# Repository layout

Module: `github.com/mevansam/context-mesh-engine`

```text
context-mesh-engine/
  README.md                      Entry; points at docs/
  LICENSE                        MIT (novassist.ai)
  go.mod / go.sum                replace -> ../go-sdk and ../libopenapi
  docs/                          users/ vs contributors/ (start at docs/README.md)
  cmd/engine/                    Sample process: -addr, -specs, -public-base-url, SIGINT, ping
  examples/
    README.md                    Points at docs/users/examples.md
    minimal/                     ListenAndServe
    embed-handler/               Your http.Server + Handler()
    arazzo-fs/                   FileLoader + stub Executor
  engine/                        Public facade: New, Handler, ListenAndServe
  api/                           Public facade: Controller, JSON helpers, HealthResponse
  arazzo/                        Public Loader, FileLoader, Executor aliases, ToolDoc
  testdata/arazzo/
    plans/                       Arazzo fixtures (FileLoader root)
    sources/openapi.yaml         OpenAPI referenced as ../sources/openapi.yaml
  internal/
    httpserver/server.go         http.Server, root mux, Shutdown
    mcpgw/server.go              mcp.NewServer + StreamableHTTPHandler
    api/json.go                  WriteJSON / ReadJSON implementation
    api/v1/router.go             /api/v1 ServeMux
    api/v1/health.go             GET /health
    api/v1/plans.go              POST /plans, GET /openapi
    plans/                       Catalog, runner, MCP tools, OAS generator
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
| `engine/engine.go` | `Options`, `New` (`error`), `MCP`, `AddController`, `Handler`, `ListenAndServe` |
| `engine/engine_test.go` | Mux contract: health JSON, MCP handshake, SSE GET 400, REST ≠ MCP |
| `engine/arazzo_test.go` | Plan MCP tools, REST execute, OpenAPI, shared runner, 501 |
| `api/controller.go` | `Controller` |
| `api/json.go` | Re-exports of `WriteJSON`, `WriteError`, `ReadJSON`, `ErrorBody` |
| `api/health.go` | `HealthResponse` |
| `arazzo/loader.go` | `Loader`, `Source`, `Executor` / request / response aliases |
| `arazzo/fileloader.go` | Recursive filesystem loader; `BaseURL` trailing slash |
| `arazzo/tooldoc.go` | Templates + `ToolDocContext` |

## File map (`internal/`)

| File | Responsibility |
| --- | --- |
| `httpserver/server.go` | Sibling mount of `/mcp` and `/api/v1/`; `ListenAndServe` + 10s `Shutdown` |
| `mcpgw/server.go` | Shared `mcp.Server`; `Stateless`/`JSONResponse` left false |
| `api/json.go` | JSON encode/decode; `ReadJSON` unknown fields rejected; 1 MiB |
| `api/v1/router.go` | v1 `ServeMux` and `Register` |
| `api/v1/health.go` | Default `GET /health` |
| `api/v1/plans.go` | `POST /plans/query`, `POST /plans/...`, `GET /openapi/...`; error mapping |
| `plans/catalog.go` | Load, skip, duplicate, `ResolveSources`, latest |
| `plans/runner.go` | New libopenapi Engine per `Run`; `ResultJSON` |
| `plans/schema.go` | MCP `inputSchema` oneOf + workflowId const |
| `plans/openapi.go` | OAS 3.1 JSON (paths **without** `/api/v1`) |
| `plans/mcp.go` | Stub `query` + `run_*` tools |

## Tests as the contract

| Test | Do not break |
| --- | --- |
| `TestHandler_HealthJSON` | `/api/v1/health` is JSON |
| `TestHandler_MCPInitializeAndPing` | Streamable HTTP at `/mcp` |
| `TestHandler_MCPGETRequiresSession` | GET `/mcp` without session is 400 (handler is mounted) |
| `TestHandler_RESTNotMCP` | POST `/api/v1/health` is REST 405, not MCP |
| `TestArazzo_*` | plans, OpenAPI, 501, MCP `query`/`run_*`, `POST /plans/query` |
| `internal/plans` tests | skip missing `x-planId`, duplicate versions, latest `1.1.0` |

## What not to add here

- Vendored copies of go-sdk or libopenapi (they are module dependencies)
- Gin/chi as the **root** mux
- Legacy `mcp.NewSSEHandler` (2024-11-05) unless a later phase explicitly needs old clients
- New public types in `engine`/`api`/`arazzo` that belong in `internal/`
- `StripPrefix` on `/mcp`

## Next

[Code design](design.md). Arazzo internals: [arazzo.md](arazzo.md).
