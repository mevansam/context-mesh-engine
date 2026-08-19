# mcp-server

`context-mesh-engine` host for the petstore Arazzo plan. Run from the **repository root** (async adapter must already be listening):

```bash
go run ./examples/petstore/async-order-server
go run ./examples/petstore/mcp-server
```

Plan: `plans/petstore.arazzo.yaml` (`x-planId: petstore`, version `1.0.1`). Workflows: `retrievePet`, `purchasePet`, `checkOrderStatus`.

Full demo: [examples/README.md](../../README.md#petstore-demo).
