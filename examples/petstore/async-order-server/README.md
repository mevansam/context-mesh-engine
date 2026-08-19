# async-order-server

HTTP adapter for the official AsyncAPI 3 example [pet-asyncapi.yaml](https://github.com/OAI/Arazzo-Specification/blob/main/examples/1.1.0/pet-asyncapi.yaml) (`place-order` send, `confirm-order` receive).

```bash
go run ./examples/petstore/async-order-server
```

Listens on `localhost:8091` by default.

| Method | Path | AsyncAPI |
| --- | --- | --- |
| POST | `/place-order` | `placeOrder` (header `orderCorrelationId` / `orderRequestId`, JSON `{ "petId": N }`) |
| GET | `/confirm-order` | `confirmOrder` (same correlation header) |
| GET | `/health` | liveness |

On place, the server `POST`s [Petstore 3](https://petstore3.swagger.io/) `/api/v3/store/order`. If that host returns 5xx, it retries `https://petstore.swagger.io/v2/store/order`.

Full demo: [examples/README.md](../../README.md#petstore-demo).
