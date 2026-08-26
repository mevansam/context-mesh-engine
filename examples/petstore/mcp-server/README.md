# mcp-server

`context-mesh-engine` host for the petstore Arazzo plan. Plans are loaded from `plans/` next to `main.go` (compile-time path via `runtime.Caller`, not the working directory). OPA modules are loaded from `policies/` the same way (`FilePolicyLoader`); they are **not** passed to `FileLoader`.

Plan: `plans/petstore.arazzo.yaml` (`x-planId: petstore`, version `0.0.1`). Workflows: `retrievePet`, `purchasePet`, `checkOrderStatus`.

Inbound policy (`policies/petstore/0.0.1/inbound.rego`) calls Petstore `GET /user/{username}` and uses `userStatus`: **1** (or missing user) may only run `retrievePet`; **2** may also `purchasePet` and `checkOrderStatus`. `findByStatus` uses `$inputs.policyHints.petStatus`.

Talks to Petstore 3: `-petstore local` (default, Docker `http://localhost:8090/api/v3`) or `-petstore hosted` (`https://petstore3.swagger.io/api/v3`). Override with `-petstore-url`. Local Docker: [../petstore-openapi-server/README.md](../petstore-openapi-server/README.md).

How to run all processes, seed users, curl the workflows, and use an MCP agent: **[../README.md](../README.md)**.
