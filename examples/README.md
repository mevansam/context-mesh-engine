# Examples

How to run each program and what it shows: **[docs/users/examples.md](../docs/users/examples.md)**.

From the repository root (sibling `go-sdk` / `libopenapi` `replace`s):

```bash
go run ./examples/minimal
go run ./examples/embed-handler
go run ./examples/arazzo-fs
go run ./examples/petstore/async-order-server
go run ./examples/petstore/mcp-server
```

| Directory | Demonstrates |
| --- | --- |
| `minimal/` | `engine.ListenAndServe` — the engine owns the HTTP listener |
| `embed-handler/` | `e.Handler()` on your own `http.Server` |
| `arazzo-fs/` | Arazzo `FileLoader`, stub `Executor`, MCP `run_*` and REST `/plans` |
| `petstore/` | End-to-end Petstore demo (MCP engine + async order adapter) |

Do not run two examples that bind `localhost:8080` at the same time. Petstore processes must be started from the repo root.

---

## Petstore demo

Two processes:

1. **`async-order-server`** (`localhost:8091`) — HTTP adapter for the official AsyncAPI 3 [pet-asyncapi.yaml](https://github.com/OAI/Arazzo-Specification/blob/main/examples/1.1.0/pet-asyncapi.yaml). `POST /place-order` calls hosted Petstore `POST /store/order`.
2. **`mcp-server`** (`localhost:8080`) — `context-mesh-engine` with an Arazzo plan based on the [1.1 spec example](https://spec.openapis.org/arazzo/latest.html), plus `x-planId: petstore`. Workflows: `retrievePet`, `purchasePet`, `checkOrderStatus`.

Hosted API: [https://petstore3.swagger.io/](https://petstore3.swagger.io/). If v3 returns 5xx (the public demo is often flaky on `/pet/findByStatus` and `/store/order`), the MCP executor and the order adapter retry `https://petstore.swagger.io/v2`.

### Run both servers

Use two terminals, both from the repository root:

```bash
go run ./examples/petstore/async-order-server
```

```bash
go run ./examples/petstore/mcp-server
```

Optional flags:

```bash
go run ./examples/petstore/async-order-server -addr localhost:8091
go run ./examples/petstore/mcp-server -addr localhost:8080 -async-order-url http://localhost:8091
```

Checks:

```bash
curl -s http://localhost:8091/health
curl -s http://localhost:8080/api/health
curl -s http://localhost:8080/api/openapi/petstore
```

Demo login: `username=user1`, `password=abc123`.

### REST: retrieve a pet

```bash
curl -s -X POST http://localhost:8080/api/plans/petstore/retrievePet \
  -H 'Content-Type: application/json' \
  -d '{"username":"user1","password":"abc123","status":"available"}'
```

Versioned URL: `POST /api/plans/petstore/v1.0.1/retrievePet`.

Pick a `petId` from `outputs.petId` (first match) or `outputs.pets`. If find-by-status is down on the host, `GET https://petstore3.swagger.io/api/v3/pet/1` still works; use `petId: 1` for purchase.

### REST: purchase that pet (async order)

Requires the async adapter. `orderCorrelationId` is any unique string (AsyncAPI `orderRequestId`).

```bash
curl -s -X POST http://localhost:8080/api/plans/petstore/purchasePet \
  -H 'Content-Type: application/json' \
  -d '{"username":"user1","password":"abc123","petId":1,"orderCorrelationId":"demo-order-1"}'
```

The engine: login → `POST http://localhost:8091/place-order` → poll `GET /confirm-order`. The adapter: `POST https://petstore3.swagger.io/api/v3/store/order` (then v2 on 5xx). Save `outputs.orderId`.

### REST: check order status

```bash
curl -s -X POST http://localhost:8080/api/plans/petstore/checkOrderStatus \
  -H 'Content-Type: application/json' \
  -d '{"username":"user1","password":"abc123","orderId":1}'
```

Replace `orderId` with the id from `purchasePet`. `outputs.status` is `placed`, `approved`, or `delivered`.

### Change order status on the hosted Petstore

Petstore has **no PUT** for orders. Status is set when the order is created (`POST /store/order`). After you have an `orderId` from `purchasePet` (or from a direct POST below), `checkOrderStatus` reads it back.

Create an order with a chosen status (try v3 first; use v2 if v3 returns 500):

```bash
# placed (same as the async adapter)
curl -s -X POST https://petstore3.swagger.io/api/v3/store/order \
  -H 'Accept: application/json' -H 'Content-Type: application/json' \
  -d '{"id":900001,"petId":1,"quantity":1,"status":"placed","complete":false}'

curl -s -X POST https://petstore.swagger.io/v2/store/order \
  -H 'Accept: application/json' -H 'Content-Type: application/json' \
  -d '{"id":900002,"petId":1,"quantity":1,"status":"approved","complete":false}'
```

Then:

```bash
curl -s -X POST http://localhost:8080/api/plans/petstore/checkOrderStatus \
  -H 'Content-Type: application/json' \
  -d '{"username":"user1","password":"abc123","orderId":900002}'
```

Direct GET (no engine):

```bash
curl -s -H 'Accept: application/json' https://petstore3.swagger.io/api/v3/store/order/900002
curl -s -H 'Accept: application/json' https://petstore.swagger.io/v2/store/order/900002
```

### MCP with an agent

The engine advertises Streamable HTTP at **`http://localhost:8080/mcp`**. After both servers are up, point an MCP client at that URL (not `/mcp/`).

Cursor: Settings → MCP → add a **URL** server `http://localhost:8080/mcp`. Restart the agent chat so it reloads tools.

The plan registers:

| Tool | Use |
| --- | --- |
| `query` | Natural-language match (501 until matching is wired) |
| `run_petstore_v1.0.1` | Run one workflow; `workflowId` is `retrievePet`, `purchasePet`, or `checkOrderStatus` |

Example prompts:

- “Using the petstore MCP tools, retrieve an available pet. Login as user1 / abc123.”
- “Purchase pet id 1 with orderCorrelationId demo-order-1, same login.”
- “Check order status for orderId \<id from purchase\>.”

The agent should call `run_petstore_v1.0.1` with:

```json
{
  "workflowId": "retrievePet",
  "inputs": { "username": "user1", "password": "abc123", "status": "available" }
}
```

```json
{
  "workflowId": "purchasePet",
  "inputs": {
    "username": "user1",
    "password": "abc123",
    "petId": 1,
    "orderCorrelationId": "demo-order-1"
  }
}
```

```json
{
  "workflowId": "checkOrderStatus",
  "inputs": { "username": "user1", "password": "abc123", "orderId": 900002 }
}
```

Go client: [docs/users/usage.md](../docs/users/usage.md#mcp-client) (`mcp.StreamableClientTransport{Endpoint: "http://localhost:8080/mcp"}`).

### What this shows about the engine

- One catalog (`x-planId` + `info.version`) → MCP `run_*` and REST `POST /plans/{planId}/{workflowId}`.
- You supply loaders + an `Executor`. This demo’s executor is HTTP: OpenAPI operations on Petstore, AsyncAPI operations on the local adapter.
- Generated `GET /api/openapi/petstore` describes the same execute routes (paths without the `/api` prefix).
- `query` is registered but not implemented yet.

Arazzo file: `examples/petstore/mcp-server/plans/petstore.arazzo.yaml`. AsyncAPI file: `examples/petstore/async-order-server/pet-asyncapi.yaml`.
