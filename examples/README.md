# Examples

How to run each program and what it shows: **[docs/users/examples.md](../docs/users/examples.md)**.

From the repository root (sibling `go-sdk` / `libopenapi` `replace`s):

```bash
go run ./examples/minimal
go run ./examples/embed-handler
go run ./examples/arazzo-fs
```

| Directory | Demonstrates |
| --- | --- |
| `minimal/` | `engine.ListenAndServe` — the engine owns the HTTP listener |
| `embed-handler/` | `e.Handler()` on your own `http.Server` |
| `arazzo-fs/` | Arazzo `FileLoader`, MCP `run_*` tools, REST `/plans` and `/openapi` |

Do not run them on the same port at the same time. `arazzo-fs` must be started from the repo root so `testdata/arazzo/plans` resolves.
