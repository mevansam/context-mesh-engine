# async-order-server

HTTP adapter for the official AsyncAPI 3 example [pet-asyncapi.yaml](https://github.com/OAI/Arazzo-Specification/blob/main/examples/1.1.0/pet-asyncapi.yaml) (`place-order` send, `confirm-order` receive). Listens on `localhost:8091` by default.

| Method | Path | AsyncAPI |
| --- | --- | --- |
| POST | `/place-order` | `placeOrder` (header `orderCorrelationId` / `orderRequestId`, JSON `{ "petId": N }`) |
| GET | `/confirm-order` | `confirmOrder` (same correlation header) |
| GET | `/health` | liveness |

On place, the server `POST`s Petstore 3 `/store/order`. `-petstore local` (default, Docker `http://localhost:8090/api/v3`) or `-petstore hosted` (`https://petstore3.swagger.io/api/v3`). Override with `-petstore-url`. Local Docker: [../petstore-openapi-server/README.md](../petstore-openapi-server/README.md).

How to run this with `mcp-server` and curl the workflows: **[../README.md](../README.md)**.
