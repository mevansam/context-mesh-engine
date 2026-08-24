# Examples

Runnable programs under this directory. Run them from the **repository root** (sibling `go-sdk` / `libopenapi` `replace`s). Do not run two examples that bind `localhost:8080` at the same time.

Each example’s **local README** has the run commands, flags, curl walkthroughs, and implementation notes:

| Directory | Demonstrates |
| --- | --- |
| [`minimal/`](minimal/README.md) | `engine.ListenAndServe` — the engine owns the HTTP listener |
| [`embed-handler/`](embed-handler/README.md) | `e.Handler()` on your own `http.Server` |
| [`arazzo-fs/`](arazzo-fs/README.md) | Arazzo `FileLoader`, stub `Executor`, MCP `run_*` and REST `/plans` |
| [`petstore/`](petstore/README.md) | End-to-end Petstore demo (local Docker Petstore 3 + MCP engine + async order adapter) |

Default is REST only (`/api/...`). Pass `-dual` to also mount MCP Streamable HTTP at `/mcp` (same flag as `cmd/engine`).
