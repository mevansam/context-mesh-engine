# petstore demo

End-to-end Arazzo demo: MCP/REST engine plus an AsyncAPI-shaped order adapter in front of [Petstore 3](https://petstore3.swagger.io/).

| Directory | Role |
| --- | --- |
| [`mcp-server/`](mcp-server/) | `context-mesh-engine` with the three-workflow Arazzo plan |
| [`async-order-server/`](async-order-server/) | HTTP adapter for [pet-asyncapi.yaml](https://github.com/OAI/Arazzo-Specification/blob/main/examples/1.1.0/pet-asyncapi.yaml); calls `POST /store/order` |

How to run both, curl the workflows, change order status, and use an MCP agent: **[examples/README.md](../README.md#petstore-demo)**.
