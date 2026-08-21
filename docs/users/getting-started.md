# Getting started (SDK users)

This path is for **application authors**. You import `github.com/mevansam/context-mesh-engine/...`.

If you intend to change mux wiring, timeouts, or package layout in **this** repository, switch to the [contributor path](../contributors/getting-started.md).

## What you get

One process, one TCP port:

| URL | Role |
| --- | --- |
| `http://<addr>/api/health` | Default liveness JSON: `{"status":"ok"}`. Always mounted unless `MCPOnly`. |
| `http://<addr>/api/tools` | MCP `tools/list` result (same `tools` array as Streamable HTTP). |
| `http://<addr>/api/...` | Your `api.Controller` routes (and, if you set loaders, Arazzo plan routes). |
| `http://<addr>/mcp` | MCP Streamable HTTP. Mounted only with `DualMCPandREST` or `MCPOnly`. |

The engine does **not** register a `ping` MCP tool by itself. `cmd/engine` and `examples/minimal` add `ping` as a sample.

## Requirements

- Go **1.25.7** or newer (`go.mod` in this repo).
- Import this module. It pulls [`github.com/modelcontextprotocol/go-sdk/mcp`](https://github.com/modelcontextprotocol/go-sdk).
- Arazzo plans also need [`github.com/pb33f/libopenapi`](https://github.com/pb33f/libopenapi) (already a dependency).

When you `go get` this module from a published version, you do not need local clones of go-sdk or libopenapi. Local development of **this** repo uses `replace` directives; that is a [contributor](../contributors/getting-started.md) concern.

## Run the sample binary

From a clone of this repository:

```bash
go run ./cmd/engine -addr localhost:8080
```

Flags (`cmd/engine/main.go`):

| Flag | Default | Meaning |
| --- | --- | --- |
| `-addr` | `localhost:8080` | Listen address (`engine.DefaultAddr`) |
| `-api-prefix` | `/api` | REST path prefix (`engine.Options.APIPrefix`) |
| `-mcp-only` | false | Serve only MCP at `/mcp` (`Options.MCPOnly`) |
| `-rest-only` | false | Serve only REST (`Options.RESTOnly`; same as the default) |
| `-dual` | false | Serve MCP and REST (`Options.DualMCPandREST`) |
| `-specs` | empty | Directory of Arazzo YAML/JSON (recursive `FileLoader`) |
| `-public-base-url` | `http://` + `-addr` when `-specs` is set | Origin written into MCP tool descriptions |

Check REST:

```bash
curl -s http://localhost:8080/api/health
# {"status":"ok"}
curl -s http://localhost:8080/api/tools
```

The sample registers an MCP `ping` tool. With the default (REST only) it appears on `GET /api/tools`. Point a Streamable HTTP client at `http://localhost:8080/mcp` only when you pass `-dual` or `-mcp-only` ([MCP client](usage.md#mcp-client)).

Default is REST only. `-dual` serves both surfaces. `-mcp-only`, `-rest-only`, and `-dual` are mutually exclusive.

`-specs` loads plans and registers `run_*` tools, but **does not** set an `Executor` or `QueryMatcher`. Execute (`POST /api/plans/...` and MCP `run_*`) returns 501 until you embed with your own executor. `query` is not published until you set `QueryMatcher`. Use [examples/arazzo-fs](examples.md#arazzo-fs) for a stub executor and dummy matcher, or [examples/petstore](examples.md#petstore) for live Petstore HTTP. Full Arazzo guide: [arazzo.md](arazzo.md).

## Use it as a library

Module path: `github.com/mevansam/context-mesh-engine`.

Public packages:

| Import | Use |
| --- | --- |
| `.../engine` | `New`, `ListenAndServe`, `Handler`, `MCP` |
| `.../api` | `Controller`, JSON helpers |
| `.../arazzo` | `Loader`, `FileLoader`, `Executor`, `QueryMatcher`, tool-doc templates |
| `github.com/modelcontextprotocol/go-sdk/mcp` | `AddTool`, client |

Do **not** import `.../internal/...`.

```go
package main

import (
	"context"
	"log"

	"github.com/mevansam/context-mesh-engine/engine"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	e, err := engine.New(engine.Options{Addr: "localhost:8080"})
	if err != nil {
		log.Fatal(err)
	}
	mcp.AddTool(e.MCP(), &mcp.Tool{Name: "ping", Description: "liveness probe"}, ping)
	log.Fatal(e.ListenAndServe(context.Background()))
}

func ping(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "pong"}},
	}, nil, nil
}
```

Default is REST only. Set `DualMCPandREST: true` (or run `go run ./examples/minimal -dual`) to also mount `/mcp`.

`engine.New` always returns `(*Engine, error)`. Construction fails if Arazzo loaders are set and specs/templates are invalid.

Runnable copies of this pattern: [examples.md](examples.md). Full API: [usage.md](usage.md).

## Next

- [SDK usage](usage.md) — Options, routes, tools, controllers, embed vs listen.
- [Arazzo plans](arazzo.md) — loaders, Executor, MCP `run_*`, REST execute.
- [Examples](examples.md)
