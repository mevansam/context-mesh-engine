# petstore demo

End-to-end Arazzo demo: MCP/REST engine plus an AsyncAPI-shaped order adapter in front of [Petstore 3](https://petstore3.swagger.io/).

| Directory | Role |
| --- | --- |
| [`mcp-server/`](mcp-server/) | `context-mesh-engine` with the three-workflow Arazzo plan |
| [`async-order-server/`](async-order-server/) | HTTP adapter for [pet-asyncapi.yaml](https://github.com/OAI/Arazzo-Specification/blob/main/examples/1.1.0/pet-asyncapi.yaml); calls `POST /store/order` |

Two processes:

1. **`async-order-server`** (`localhost:8091`) — HTTP adapter for the official AsyncAPI 3 [pet-asyncapi.yaml](https://github.com/OAI/Arazzo-Specification/blob/main/examples/1.1.0/pet-asyncapi.yaml). `POST /place-order` calls hosted Petstore `POST /store/order`.
2. **`mcp-server`** (`localhost:8080`) — `context-mesh-engine` with an Arazzo plan based on the [1.1 spec example](https://spec.openapis.org/arazzo/latest.html), plus `x-planId: petstore`. Workflows: `retrievePet`, `purchasePet`, `checkOrderStatus`.

Hosted API: [https://petstore3.swagger.io/](https://petstore3.swagger.io/). If v3 returns 5xx (the public demo is often flaky on `/pet/findByStatus` and `/store/order`), the MCP executor and the order adapter retry `https://petstore.swagger.io/v2`.

Do not run `mcp-server` at the same time as another example (or `cmd/engine`) on `localhost:8080`.

## Run both servers

Use two terminals, both from the **repository root**:

```bash
go run ./examples/petstore/async-order-server
```

```bash
go run ./examples/petstore/mcp-server
```

Default for `mcp-server` is REST only. `-dual` also mounts MCP Streamable HTTP at `/mcp`.

Optional flags:

```bash
go run ./examples/petstore/async-order-server -addr localhost:8091
go run ./examples/petstore/mcp-server -addr localhost:8080 -async-order-url http://localhost:8091
go run ./examples/petstore/mcp-server -dual
```

Checks:

```bash
curl -s http://localhost:8091/health
curl -s http://localhost:8080/api/health
curl -s http://localhost:8080/api/tools
curl -s http://localhost:8080/api/openapi/petstore
```

Demo login: `username=user1`, `password=abc123`.

## REST: retrieve a pet

```bash
curl -s -X POST http://localhost:8080/api/plans/petstore/retrievePet \
  -H 'Content-Type: application/json' \
  -d '{"username":"user1","password":"abc123","status":"available"}'
```

Versioned URL: `POST /api/plans/petstore/v1.0.1/retrievePet`.

Pick a `petId` from the response (or `pet.id`). If find-by-status is down on the host, `GET https://petstore3.swagger.io/api/v3/pet/1` still works; use `petId: 1` for purchase.

## REST: purchase that pet (async order)

Requires the async adapter. `orderCorrelationId` is any unique string (AsyncAPI `orderRequestId`).

```bash
curl -s -X POST http://localhost:8080/api/plans/petstore/purchasePet \
  -H 'Content-Type: application/json' \
  -d '{"username":"user1","password":"abc123","petId":1,"orderCorrelationId":"demo-order-1"}'
```

The engine: login → `POST http://localhost:8091/place-order` → poll `GET /confirm-order`. The adapter: `POST https://petstore3.swagger.io/api/v3/store/order` (then v2 on 5xx). Save `orderId` from the raw JSON (do not pipe through `jq`: hosted ids are often larger than JSON float precision).

`checkOrderStatus` tries v3 then v2. A direct `GET` on petstore3 will 404 for orders that were created on v2.

## REST: check order status

```bash
curl -s -X POST http://localhost:8080/api/plans/petstore/checkOrderStatus \
  -H 'Content-Type: application/json' \
  -d '{"username":"user1","password":"abc123","orderId":1}'
```

Replace `orderId` with the id from `purchasePet`. `status` is `placed`, `approved`, or `delivered`. If you GET the hosted Petstore yourself, use the same host the adapter logged (`v3` or `v2`); the engine tries both.

## Change order status on the hosted Petstore

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

## MCP with an agent

Start `mcp-server` with `-dual` so Streamable HTTP is mounted at **`http://localhost:8080/mcp`**. After both servers are up, point an MCP client at that URL (not `/mcp/`).

```bash
go run ./examples/petstore/mcp-server -dual
```

Cursor: Settings → MCP → add a **URL** server `http://localhost:8080/mcp`. Restart the agent chat so it reloads tools.

The plan registers:

| Tool | Use |
| --- | --- |
| `run_petstore_v1.0.1` | Run one workflow; `workflowId` is `retrievePet`, `purchasePet`, or `checkOrderStatus` |

This example does not set `QueryMatcher`, so `query` is not registered.

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

Go client: [docs/users/usage.md](../../docs/users/usage.md#mcp-client) (`mcp.StreamableClientTransport{Endpoint: "http://localhost:8080/mcp"}`).

### MCP handshake with curl

REST equivalent of `tools/list` (no session):

```bash
curl -s http://localhost:8080/api/tools
```

To list tools over MCP itself, POST JSON-RPC `tools/list`. The engine answers POST with SSE (`event: message` / `data: …`), so copy `Mcp-Session-Id` from the initialize headers, then send that header on `tools/list`.

```bash
curl -sS -D - -X POST http://localhost:8080/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"curl","version":"0"}}}'
```

```bash
curl -sS -X POST http://localhost:8080/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'Mcp-Session-Id: <session>' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
```

Replace `<session>` with the `Mcp-Session-Id` value. The `data:` line lists `run_petstore_v1.0.1`. To print names only:

```bash
curl -sS -X POST http://localhost:8080/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'Mcp-Session-Id: <session>' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  | awk '/^data: /{print substr($0,7)}' | jq -r '.result.tools[].name'
```

## What this shows about the engine

- One catalog (`x-planId` + `info.version`) → MCP `run_*` and REST `POST /plans/{planId}/{workflowId}`.
- You supply loaders + an `Executor`. This demo’s executor is HTTP: OpenAPI operations on Petstore, AsyncAPI operations on the local adapter.
- Generated `GET /api/openapi/petstore` describes the same execute routes (paths without the `/api` prefix).
- `GET /api/tools` is the REST form of MCP `tools/list`.
- `query` is published only when `Options.QueryMatcher` is set. Petstore leaves it unset; [arazzo-fs](../arazzo-fs/README.md) ships a dummy matcher.

Arazzo file: `mcp-server/plans/petstore.arazzo.yaml`. AsyncAPI file: `async-order-server/pet-asyncapi.yaml`.
