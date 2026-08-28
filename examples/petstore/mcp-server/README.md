# mcp-server

`context-mesh-engine` host. The engine owns catalog, MCP `run_*`, REST `/plans`, and OPA order. This directory is the **adapters** you pass to `engine.New`.

| File | SDK seam | What it changes |
| --- | --- | --- |
| [`main.go`](main.go) | `engine.Options` | Loader, executor, policy, preprocessor, secrets, handler wraps |
| [`docs.go`](docs.go) / [`docs/`](docs/) | `AddController` | Swagger UI at `GET /api/docs` |
| [`auth.go`](auth.go) | `MCPHandlerWrap`, `RESTHandlerWrap`, `RequestPreprocessor` | Client JWT on MCP + `POST /plans/`; end-user JWT → OPA `input.auth` |
| [`executor.go`](executor.go) | `ArazzoExecutor`, `SecretsProvider` | HTTP to Petstore / async adapter; **new** downstream JWT |
| `plans/` | `ArazzoLoaders` | Arazzo document (`x-planId: petstore`) |
| `policies/` | `PolicyLoader` | Inbound/outbound Rego (not passed to `FileLoader`) |

Field-by-field notes: comments on `hostOptions` in `main.go`. How to run all processes: **[../README.md](../README.md)**.

Plan: `plans/petstore.arazzo.yaml` (`x-planId: petstore`, version `0.0.1`). Workflows: `retrievePet`, `purchasePet`, `checkOrderStatus`. First step is `getUserByName` (`$inputs.policyHints.username` from the end-user JWT).

Inbound (`policies/petstore/0.0.1/inbound.rego`) reads `input.auth.endUser`. It does **not** `http.send`. `userStatus` **1** may only `retrievePet`; **2** may also purchase/check order.

Tokens: [`../petstore-auth-server`](../petstore-auth-server/). Share `-jwt-secret`. Petstore: `-petstore local` (default) or `-hosted`. Override `-petstore-url`.

Stdout logs client JWT claims (`sub`, `iss`, `aud`, `token_use`), end-user claims (`username`, `userStatus`), and each Arazzo step (method, URL, status, truncated body). Raw tokens and passwords are not logged.

Swagger UI: **[../README.md — Swagger UI](../README.md#swagger-ui)** (`http://localhost:8080/api/docs`).
