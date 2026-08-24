# mcp-server

`context-mesh-engine` host for the petstore Arazzo plan. Plans are loaded from `plans/` next to `main.go` (compile-time path via `runtime.Caller`, not the working directory).

Plan: `plans/petstore.arazzo.yaml` (`x-planId: petstore`, version `1.0.1`). Workflows: `retrievePet`, `purchasePet`, `checkOrderStatus`.

Talks to Petstore 3: `-petstore local` (default, Docker `http://localhost:8090/api/v3`) or `-petstore hosted` (`https://petstore3.swagger.io/api/v3`). Override with `-petstore-url`. Local Docker: [../petstore-openapi-server/README.md](../petstore-openapi-server/README.md).

How to run all processes, curl the workflows, and use an MCP agent: **[../README.md](../README.md)**.
