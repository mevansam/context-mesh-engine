# petstore demo

End-to-end Arazzo demo: MCP/REST engine plus an AsyncAPI-shaped order adapter in front of a **local** [Petstore 3](https://github.com/swagger-api/swagger-petstore) OpenAPI server (Docker).

| Directory | Role |
| --- | --- |
| [`petstore-openapi-server/`](petstore-openapi-server/) | Official Petstore 3 in Docker (`localhost:8090`) |
| [`mcp-server/`](mcp-server/) | `context-mesh-engine` with the three-workflow Arazzo plan |
| [`async-order-server/`](async-order-server/) | HTTP adapter for [pet-asyncapi.yaml](https://github.com/OAI/Arazzo-Specification/blob/main/examples/1.1.0/pet-asyncapi.yaml); calls `POST /store/order` |

Three processes:

1. **`petstore-openapi-server`** (`localhost:8090`) — Swagger Petstore 3 in Docker. OpenAPI base: `http://localhost:8090/api/v3`. UI: `http://localhost:8090/`.
2. **`async-order-server`** (`localhost:8091`) — HTTP adapter for the official AsyncAPI 3 [pet-asyncapi.yaml](https://github.com/OAI/Arazzo-Specification/blob/main/examples/1.1.0/pet-asyncapi.yaml). `POST /place-order` calls local Petstore `POST /store/order`.
3. **`mcp-server`** (`localhost:8080`) — `context-mesh-engine` with an Arazzo plan based on the [1.1 spec example](https://spec.openapis.org/arazzo/latest.html), plus `x-planId: petstore`. Workflows: `retrievePet`, `purchasePet`, `checkOrderStatus`.

`mcp-server` and `async-order-server` default to `http://localhost:8090/api/v3`. They do **not** call the hosted [petstore3.swagger.io](https://petstore3.swagger.io/) demo.

Do not run `mcp-server` at the same time as another example (or `cmd/engine`) on `localhost:8080`.

## Petstore 3 in Docker

Needs [Docker](https://docs.docker.com/get-docker/) on `PATH`. The scripts wrap [swaggerapi/petstore3:unstable](https://hub.docker.com/r/swaggerapi/petstore3) as described in the [upstream README](https://github.com/swagger-api/swagger-petstore/blob/master/README.md). They **build only if** the local image `context-mesh-petstore3:local` is missing, then start (or reuse) a container.

From the **repository root**:

```bash
./examples/petstore/petstore-openapi-server/run.sh
```

Windows (PowerShell):

```powershell
./examples/petstore/petstore-openapi-server/run.ps1
```

Force a fresh image:

```bash
./examples/petstore/petstore-openapi-server/run.sh --rebuild
```

```powershell
./examples/petstore/petstore-openapi-server/run.ps1 -Rebuild
```

Host port **8090** avoids clashing with `mcp-server` on 8080 (the process inside the container still uses 8080).

Checks:

```bash
curl -s http://localhost:8090/api/v3/openapi.json | head
curl -s 'http://localhost:8090/api/v3/pet/findByStatus?status=available' | head
```

Stop: `docker stop petstore-openapi-server`.

Image/container names and port: [petstore-openapi-server/README.md](petstore-openapi-server/README.md). If you change the port, pass `-petstore-url http://localhost:PORT/api/v3` to both Go processes.

## Run the Go servers

After Petstore Docker is up, two more terminals from the **repository root**:

```bash
go run ./examples/petstore/async-order-server
```

```bash
go run ./examples/petstore/mcp-server
```

Default for `mcp-server` is REST only. `-dual` also mounts MCP Streamable HTTP at `/mcp`.

Optional flags:

```bash
go run ./examples/petstore/async-order-server -addr localhost:8091 -petstore-url http://localhost:8090/api/v3
go run ./examples/petstore/mcp-server -addr localhost:8080 -async-order-url http://localhost:8091 -petstore-url http://localhost:8090/api/v3
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

Pick a `petId` from the response (or `pet.id`). Direct Petstore: `GET http://localhost:8090/api/v3/pet/1`.

## REST: purchase that pet (async order)

Requires the async adapter. `orderCorrelationId` is any unique string (AsyncAPI `orderRequestId`).

```bash
curl -s -X POST http://localhost:8080/api/plans/petstore/purchasePet \
  -H 'Content-Type: application/json' \
  -d '{"username":"user1","password":"abc123","petId":1,"orderCorrelationId":"demo-order-1"}'
```

The engine: login → `POST http://localhost:8091/place-order` → poll `GET /confirm-order`. The adapter: `POST http://localhost:8090/api/v3/store/order`. Save `orderId`.

## REST: check order status

```bash
curl -s -X POST http://localhost:8080/api/plans/petstore/checkOrderStatus \
  -H 'Content-Type: application/json' \
  -d '{"username":"user1","password":"abc123","orderId":1}'
```

Replace `orderId` with the id from `purchasePet`. `status` is `placed`, `approved`, or `delivered`.

## Change order status on local Petstore

Petstore has **no PUT** for orders. Status is set when the order is created (`POST /store/order`). After you have an `orderId` from `purchasePet` (or from a direct POST below), `checkOrderStatus` reads it back.

```bash
curl -s -X POST http://localhost:8090/api/v3/store/order \
  -H 'Accept: application/json' -H 'Content-Type: application/json' \
  -d '{"id":900001,"petId":1,"quantity":1,"status":"placed","complete":false}'
```

Then:

```bash
curl -s -X POST http://localhost:8080/api/plans/petstore/checkOrderStatus \
  -H 'Content-Type: application/json' \
  -d '{"username":"user1","password":"abc123","orderId":900001}'
```

Direct GET (no engine):

```bash
curl -s -H 'Accept: application/json' http://localhost:8090/api/v3/store/order/900001
```

## MCP with an agent

Start `mcp-server` with `-dual` so Streamable HTTP is mounted at **`http://localhost:8080/mcp`**. After Petstore Docker and both Go servers are up, point an MCP client at that URL (not `/mcp/`).

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
  "inputs": { "username": "user1", "password": "abc123", "orderId": 900001 }
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
- You supply loaders + an `Executor`. This demo’s executor is HTTP: OpenAPI operations on local Petstore, AsyncAPI operations on the local adapter.
- Generated `GET /api/openapi/petstore` describes the same execute routes (paths without the `/api` prefix).
- `GET /api/tools` is the REST form of MCP `tools/list`.
- `query` is published only when `Options.QueryMatcher` is set. Petstore leaves it unset; [arazzo-fs](../arazzo-fs/README.md) ships a dummy matcher.

Arazzo file: `mcp-server/plans/petstore.arazzo.yaml`. AsyncAPI file: `async-order-server/pet-asyncapi.yaml`.
