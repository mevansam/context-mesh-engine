# mcp-server

`context-mesh-engine` host for the petstore Arazzo plan. Plans are loaded from `plans/` next to `main.go` (compile-time path via `runtime.Caller`, not the working directory).

Plan: `plans/petstore.arazzo.yaml` (`x-planId: petstore`, version `1.0.1`). Workflows: `retrievePet`, `purchasePet`, `checkOrderStatus`.

How to run both processes, curl the workflows, and use an MCP agent: **[../README.md](../README.md)**.
