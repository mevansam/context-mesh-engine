# mcp-server

`context-mesh-engine` host for the petstore Arazzo plan. Plans are loaded from `plans/` next to `main.go` (not the working directory). Run `go run` from the **repository root** so module `replace`s resolve (async adapter must already be listening):

```bash
go run ./examples/petstore/async-order-server
go run ./examples/petstore/mcp-server [-dual]
```

Default is REST only. `-dual` also mounts MCP Streamable HTTP at `/mcp`.

Plan: `plans/petstore.arazzo.yaml` (`x-planId: petstore`, version `1.0.1`). Workflows: `retrievePet`, `purchasePet`, `checkOrderStatus`.

Full demo: [examples/README.md](../../README.md#petstore-demo).
