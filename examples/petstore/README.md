# petstore demo

End-to-end Arazzo demo: MCP/REST engine plus an AsyncAPI-shaped order adapter in front of [Petstore 3](https://petstore3.swagger.io/).

| Directory | Role |
| --- | --- |
| [`mcp-server/`](mcp-server/) | `context-mesh-engine` with the three-workflow Arazzo plan |
| [`async-order-server/`](async-order-server/) | HTTP adapter for [pet-asyncapi.yaml](https://github.com/OAI/Arazzo-Specification/blob/main/examples/1.1.0/pet-asyncapi.yaml); calls `POST /store/order` |

How to run both, curl the workflows, change order status, and use an MCP agent: **[examples/README.md](../README.md#petstore-demo)**.

## MCP

Streamable HTTP is **`http://localhost:8080/mcp`**. REST equivalent of `tools/list` (no session):

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

Replace `<session>` with the `Mcp-Session-Id` value. The `data:` line lists `query` and `run_petstore_v1.0.1`. To print names only:

```bash
curl -sS -X POST http://localhost:8080/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'Mcp-Session-Id: <session>' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  | awk '/^data: /{print substr($0,7)}' | jq -r '.result.tools[].name'
```
