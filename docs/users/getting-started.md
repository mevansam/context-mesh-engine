# Getting started

Embed `engine.Engine` in a Go process so **MCP tools** and **REST + OpenAPI** share one TCP port and the same Arazzo plan catalog.

This guide is for application authors. If you are changing this repository, use the [contributor path](../contributors/getting-started.md) instead.

## What you get

| Surface | Default URL | Purpose |
| --- | --- | --- |
| Health | `GET /api/health` | Liveness JSON `{"status":"ok"}` |
| Tools | `GET /api/tools` | Same tool names and schemas as MCP `tools/list`; Arazzo descriptions are REST-specific |
| REST API | `/api/...` | Your controllers, plus plan execute/OpenAPI when loaders are set |
| MCP | `/mcp` | Streamable HTTP. Mounted only when you opt in (see [serve modes](configuration.md#serve-modes)) |

The engine does **not** register a `ping` MCP tool. `cmd/engine` and the [minimal](examples.md#minimal) example add one as a sample.

## Requirements

- Go **1.25.7** or newer (`go.mod` in this repository).
- Module `github.com/mevansam/context-mesh-engine`.
- Transitive dependencies: [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) and [libopenapi](https://github.com/pb33f/libopenapi).

Published module versions do not require local clones of those libraries. This checkout uses `replace` directives for development; that is a [contributor](../contributors/getting-started.md) concern.

Do **not** import `github.com/mevansam/context-mesh-engine/internal/...`.

## Install

```bash
go get github.com/mevansam/context-mesh-engine
```

Public packages:

| Import | Role |
| --- | --- |
| `.../engine` | Construct, configure, and serve |
| `.../api` | REST `Controller` and JSON helpers |
| `.../arazzo` | Loaders, Executor, QueryMatcher, tool help |
| `github.com/modelcontextprotocol/go-sdk/mcp` | `AddTool`, MCP client |

## Run the sample binary

From a clone of this repository:

```bash
go run ./cmd/engine -addr localhost:8080
```

```bash
curl -s http://localhost:8080/api/health
# {"status":"ok"}
curl -s http://localhost:8080/api/tools
```

Default is **REST only**. Pass `-dual` to also mount MCP at `/mcp`. Flags map 1:1 to [`engine.Options`](configuration.md#options-reference) (`-addr`, `-api-prefix`, `-mcp-only`, `-rest-only`, `-dual`, `-specs`, `-public-base-url`).

`-specs` loads Arazzo files but does **not** set an [`Executor`](adapters.md#executor) or [`QueryMatcher`](adapters.md#querymatcher). Execute returns **501** until you embed with your own executor. `query` is unpublished until you set a matcher. Use [arazzo-fs](examples.md#arazzo-fs) for stubs, or [petstore](examples.md#petstore) for live HTTP.

## Embed the engine

```go
package main

import (
	"context"
	"log"

	"github.com/mevansam/context-mesh-engine/engine"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	e, err := engine.New(engine.Options{
		Addr:           "localhost:8080",
		DualMCPandREST: true, // omit for REST only
	})
	if err != nil {
		log.Fatal(err)
	}

	mcp.AddTool(e.MCP(), &mcp.Tool{
		Name:        "ping",
		Description: "liveness probe",
	}, ping)

	log.Fatal(e.ListenAndServe(context.Background()))
}

func ping(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "pong"}},
	}, nil, nil
}
```

`engine.New` always returns `(*Engine, error)`. Construction fails if Arazzo loaders are set and specs or templates are invalid.

Runnable copies: [minimal](examples.md#minimal) (`ListenAndServe`) and [embed-handler](examples.md#embed-handler) (your `http.Server`).

## Reading order

1. **[Configuration](configuration.md)** — every `Options` field, serve modes, `ListenAndServe` vs `Handler()`, routes.
2. **[Adapters](adapters.md)** — implement `Loader`, `Executor`, `QueryMatcher`, `PolicyLoader`, `ToolHelpLookup`, REST controllers.
3. **[Arazzo plans](arazzo.md)** — spec requirements, MCP `run_*` / `query`, REST execute, generated OpenAPI.
4. **[Examples](examples.md)** — what each program demonstrates and when to copy it.
