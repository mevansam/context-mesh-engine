# petstore-openapi-server

Runs the official [Swagger Petstore 3](https://github.com/swagger-api/swagger-petstore) OpenAPI server in Docker so the petstore example does not depend on the hosted [petstore3.swagger.io](https://petstore3.swagger.io/) demo.

The image is [swaggerapi/petstore3:unstable](https://hub.docker.com/r/swaggerapi/petstore3) (same as the upstream README). Scripts tag it locally as `context-mesh-petstore3:local` and **build only when that tag is missing**.

## Requirements

- [Docker](https://docs.docker.com/get-docker/) (Desktop or engine) on `PATH`

## Launch

From the **repository root**, or from this directory:

```bash
./examples/petstore/petstore-openapi-server/run.sh
```

Windows (PowerShell):

```powershell
./examples/petstore/petstore-openapi-server/run.ps1
```

Rebuild the image (pulls `swaggerapi/petstore3:unstable` again):

```bash
./examples/petstore/petstore-openapi-server/run.sh --rebuild
```

```powershell
./examples/petstore/petstore-openapi-server/run.ps1 -Rebuild
```

Host **8090** is used so it does not collide with `mcp-server` on `8080`. The container still listens on 8080 inside Docker.

| URL | Role |
| --- | --- |
| `http://localhost:8090/` | Swagger UI |
| `http://localhost:8090/api/v3` | OpenAPI 3 base (same path as hosted `/api/v3`) |
| `http://localhost:8090/api/v3/openapi.json` | Spec |

```bash
curl -s 'http://localhost:8090/api/v3/pet/findByStatus?status=available'
curl -s -H 'Accept: application/json' http://localhost:8090/api/v3/pet/1
```

Stop:

```bash
docker stop petstore-openapi-server
```

## Env

| Variable | Default | Meaning |
| --- | --- | --- |
| `PETSTORE_IMAGE` | `context-mesh-petstore3:local` | Local image tag |
| `PETSTORE_CONTAINER` | `petstore-openapi-server` | Container name |
| `PETSTORE_PORT` | `8090` | Host port |

If you change `PETSTORE_PORT`, pass the same origin to the Go processes:

```bash
go run ./examples/petstore/async-order-server -petstore-url http://localhost:PORT/api/v3
go run ./examples/petstore/mcp-server -petstore-url http://localhost:PORT/api/v3
```

Full demo: [../README.md](../README.md).
