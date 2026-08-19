# Documentation

Two paths. Read only the one that matches what you are doing. Mixing them is how mux rules get broken and how SDK users get sent into `internal/`.

```text
docs/
  README.md                 you are here
  users/                    application authors (import this module)
    getting-started.md      install, first request, first embed
    usage.md                public API, routes, Options, MCP, REST
    arazzo.md               loaders, Executor, run_* tools, REST plans
    examples.md             how to run examples/ and what each shows
  contributors/             people changing this repository
    getting-started.md      clone, replace directives, test, PR checks
    layout.md               package map and import rules
    design.md               mux, Streamable HTTP, timeouts, middleware
    arazzo.md               catalog, runner, templates, OpenAPI generator
```

---

## Path A — I am using this SDK

**Goal:** embed `engine.Engine` (or run `cmd/engine`) so MCP and REST share one TCP port.

Do not import `internal/`.

1. [Getting started](users/getting-started.md) — requirements, sample binary, first library snippet.
2. [SDK usage](users/usage.md) — public types, `Options` defaults, adding tools/controllers, `ListenAndServe` vs `Handler()`, MCP client headers.
3. [Arazzo plans](users/arazzo.md) — load specs, `Executor`, MCP `query`/`run_*`, REST query/execute and OpenAPI.
4. [Examples](users/examples.md) — `go run ./examples/...` and what each program demonstrates.

---

## Path B — I am contributing to this SDK

**Goal:** change mux, REST, MCP wiring, or Arazzo internals **without** breaking GET SSE on `/mcp`.

1. [Contributor getting started](contributors/getting-started.md) — Go version, sibling `replace`s, `go test ./...`.
2. [Repository layout](contributors/layout.md) — what may be imported; where code lives.
3. [Code design](contributors/design.md) — sibling mount, no `StripPrefix` on `/mcp`, REST-only timeouts.
4. [Arazzo internals](contributors/arazzo.md) — catalog skip/fail rules, per-call Engine, tool templates.

SDK users should not need Path B to ship an app.

---

## Shared facts (both paths)

| Route | Protocol | Implementation |
| --- | --- | --- |
| `/mcp`, `/mcp/` | MCP Streamable HTTP (POST, GET SSE, DELETE) | `mcp.NewStreamableHTTPHandler` |
| `/api/v1/` | JSON REST | stdlib `ServeMux` + `api.Controller` |
| `/api/v1/plans/...`, `/api/v1/openapi/...` | JSON REST | registered only when `Options.ArazzoLoaders` is non-empty |

Root mux: Go 1.22+ `net/http.ServeMux`. Do not put Gin/chi on the **root** listener.

Module: `github.com/mevansam/context-mesh-engine` (`go 1.25.7` in `go.mod`).

Upstream libraries (not vendored): [`go-sdk`](https://github.com/modelcontextprotocol/go-sdk), [`libopenapi`](https://github.com/pb33f/libopenapi) Arazzo engine.
