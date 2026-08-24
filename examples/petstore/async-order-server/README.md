# async-order-server

HTTP adapter for the official AsyncAPI 3 example [pet-asyncapi.yaml](https://github.com/OAI/Arazzo-Specification/blob/main/examples/1.1.0/pet-asyncapi.yaml) (`place-order` send, `confirm-order` receive). Listens on `localhost:8091` by default.

| Method | Path | AsyncAPI |
| --- | --- | --- |
| POST | `/place-order` | `placeOrder` (header `orderCorrelationId` / `orderRequestId`, JSON `{ "petId": N }`) |
| GET | `/confirm-order` | `confirmOrder` (same correlation header) |
| GET | `/health` | liveness |

On place, the server `POST`s the local Petstore 3 OpenAPI server (`http://localhost:8090/api/v3` by default) `/store/order`. Override with `-petstore-url`. Start Docker first: [../petstore-openapi-server/README.md](../petstore-openapi-server/README.md).

How to run this with `mcp-server` and curl the workflows: **[../README.md](../README.md)**.
