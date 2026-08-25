# Documentation

Two paths. Read only the one that matches what you are doing. Mixing them is how mux rules get broken and how SDK users get sent into `internal/`.

```text
docs/
  README.md                 you are here
  users/                    application authors (import this module)
    getting-started.md      install, sample binary, first embed
    configuration.md        Options, serve modes, ListenAndServe vs Handler, routes
    adapters.md             Loader, Executor, QueryMatcher, ToolHelpLookup, Controller
    arazzo.md               spec rules, MCP/REST contracts, OpenAPI
    examples.md             what each example demonstrates
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
2. [Configuration](users/configuration.md) — every `Options` field, serve modes, starting the server, routes.
3. [Adapters](users/adapters.md) — implement loaders, executors, query matchers, tool help, REST controllers.
4. [Arazzo plans](users/arazzo.md) — spec requirements, MCP `query` / `run_*`, REST execute, generated OpenAPI.
5. [Examples](users/examples.md) — what each program demonstrates; run details in `examples/*/README.md`.

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
| `/mcp`, `/mcp/` | MCP Streamable HTTP (POST, GET SSE, DELETE) | mounted when `DualMCPandREST` or `MCPOnly` |
| `/api` (default `Options.APIPrefix`) | JSON REST | stdlib `ServeMux` + `api.Controller` |
| `{APIPrefix}/tools` | JSON REST | always; MCP `tools/list` envelope, REST descriptions for Arazzo tools |
| `{APIPrefix}/plans/...`, `{APIPrefix}/openapi/...` | JSON REST | registered only when `Options.ArazzoLoaders` is non-empty |

Root mux: Go 1.22+ `net/http.ServeMux`. Do not put Gin/chi on the **root** listener.

Module: `github.com/mevansam/context-mesh-engine` (`go 1.25.7` in `go.mod`).

Upstream libraries (not vendored): [`go-sdk`](https://github.com/modelcontextprotocol/go-sdk), [`libopenapi`](https://github.com/pb33f/libopenapi) Arazzo engine.
