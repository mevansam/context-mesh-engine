# Examples

Runnable programs under [`examples/`](../../examples/). Run them from the **repository root**. In this checkout, `go.mod` `replace`s sibling `../go-sdk` and `../libopenapi`.

Do not run two examples (or `cmd/engine`) on `localhost:8080` at the same time. Default is REST only; each example accepts `-dual` to also mount MCP Streamable HTTP at `/mcp`.

How to run, flags, curl walkthroughs, and implementation notes live in **each example’s local README**:

| Example | Demonstrates |
| --- | --- |
| [minimal](../../examples/minimal/README.md) | Engine-owned listener via `ListenAndServe` |
| [embed-handler](../../examples/embed-handler/README.md) | Your `http.Server` using `Handler()` |
| [arazzo-fs](../../examples/arazzo-fs/README.md) | `FileLoader` + stub `Executor`, MCP `run_*` and REST plans |
| [petstore](../../examples/petstore/README.md) | Arazzo + HTTP executor over [petstore3](https://petstore3.swagger.io/) and the async order adapter |

`cmd/engine` is a fourth runnable (`go run ./cmd/engine -addr localhost:8080`). It is the sample **product** binary (flags + SIGINT shutdown), not an SDK usage example. Flags: [getting started](getting-started.md#run-the-sample-binary).

Index of the same programs: [examples/README.md](../../examples/README.md).
