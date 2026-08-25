# Getting started (contributors)

This path is for **people changing this repository**. SDK users should follow [Path A](../users/getting-started.md).

Do not copy or edit files under the sibling `go-sdk` or `libopenapi` clones except through those projects’ own process. This module depends on them via `replace`.

## Where you will work

See [layout.md](layout.md) for the full map.

| Area | Change when |
| --- | --- |
| `internal/httpserver` | root mux, `http.Server`, shutdown |
| `internal/mcpgw` | shared `mcp.Server` + Streamable HTTP handler options |
| `internal/api`, `internal/api/v1` | JSON helpers, REST router, health, plans HTTP |
| `internal/plans` | catalog, runner, MCP tool registration, OAS JSON |
| `arazzo/` | public `Loader` / `Executor` / templates |
| `engine/`, `api/` | thin public facades only |
| `docs/users/` vs `docs/contributors/` | keep the two audiences separate |

## Toolchain

- Go version in **this** `go.mod` (currently **1.25.7**). Prefer that over CI or sibling `go.mod` if they disagree.
- `gofmt`

`go.mod` contains:

```text
replace github.com/modelcontextprotocol/go-sdk => ../go-sdk
replace github.com/pb33f/libopenapi => ../libopenapi
```

Sibling layout:

```text
source/
  context-mesh-engine/
  go-sdk/
  libopenapi/
```

If your checkout differs, change `replace` locally. Do not commit a machine-specific path without discussion.

## Build and test

```bash
gofmt -w .
go test ./...
go build ./cmd/engine ./examples/...
go run ./cmd/engine -addr localhost:8080
```

Targeted tests:

| Command | Covers |
| --- | --- |
| `go test ./engine -run TestHandler_` | mux contract: health, MCP handshake, SSE GET 400, REST ≠ MCP |
| `go test ./engine -run TestArazzo_` | plan MCP tools, REST execute, OpenAPI, query matcher, 501, shared runner |
| `go test ./internal/plans` | catalog skip/duplicate/latest, runner, schema, OAS JSON |
| `go test ./arazzo` | FileLoader, template render |
| `go test ./examples/petstore/mcp-server` | petstore Arazzo plan loads |
| `go test ./examples/petstore/async-order-server` | async adapter place/confirm |

## Minimum checks before a PR

1. `go test ./...` passes.
2. `GET {APIPrefix}/health` (default `/api/health`) still returns JSON `{"status":"ok"}`.
3. `GET {APIPrefix}/tools` still returns the MCP `tools/list` envelope (REST descriptions for Arazzo tools).
4. An MCP client can still `Connect` to `/mcp` (`TestHandler_MCPInitializeAndPing`).
5. You did not wrap the **root** handler in buffering middleware or set `WriteTimeout`.
6. User-facing behavior changes are reflected in `docs/users/`; internals in `docs/contributors/`.
7. New **public** types have a godoc comment and an update to [docs/users/usage.md](../users/usage.md) (Arazzo types: [docs/users/arazzo.md](../users/arazzo.md)).

## Documentation rules

- User-facing behavior → `docs/users/`
- Internals, mux rules, package map → `docs/contributors/`
- Keep root `README.md` short; link to `docs/README.md`
- Package `README.md` files (`arazzo/`, `examples/`, `internal/plans/`, `internal/api/v1/`) point at `docs/`; do not duplicate design there
- Write for two readers: a human embedding the SDK, and a coding agent that must not import `internal/` or wrap `/mcp` in Gin

## Next

1. [Repository layout](layout.md)
2. [Code design](design.md)
3. [Arazzo internals](arazzo.md)
