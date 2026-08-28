# openapi-server

Runs the official [Swagger Petstore 3](https://github.com/swagger-api/swagger-petstore) OpenAPI server in Docker so the petstore example does not depend on the hosted [petstore3.swagger.io](https://petstore3.swagger.io/) demo.

The [Dockerfile](Dockerfile) starts from [swaggerapi/petstore3:latest](https://hub.docker.com/r/swaggerapi/petstore3) by default and adds an [SLF4J](http://www.slf4j.org/) 1.7 simple binding so Jetty does not print `StaticLoggerBinder` errors. Override the base with `--upstream` / `-Upstream` or `PETSTORE_UPSTREAM`. Scripts **build** `context-mesh-petstore3:local` when that tag is missing (they pull the upstream base only if it is not already local). The published image is `linux/amd64`; Docker Desktop on Apple Silicon runs it under emulation.

Jetty still prints `jetty-runner is deprecated` on startup; that warning comes from upstream and is harmless.

The container runs in the **foreground** (`docker run --rm`). Ctrl+C stops it and **removes** the container, which deletes in-memory users and orders. Each run is a clean store.

## Requirements

- [Docker](https://docs.docker.com/get-docker/) (Desktop or engine) on `PATH`

## Launch

From the **repository root**, or from this directory:

```bash
./examples/petstore/openapi-server/run.sh
```

Windows (PowerShell):

```powershell
./examples/petstore/openapi-server/run.ps1
```

Force a fresh image (optionally from another base):

```bash
./examples/petstore/openapi-server/run.sh --rebuild
./examples/petstore/openapi-server/run.sh --rebuild --upstream swaggerapi/petstore3:latest
```

```powershell
./examples/petstore/openapi-server/run.ps1 -Rebuild
./examples/petstore/openapi-server/run.ps1 -Rebuild -Upstream swaggerapi/petstore3:latest
```

Host **8090** is used so it does not collide with `mcp-server` on `8080`. The container still listens on 8080 inside Docker.

| URL | Role |
| --- | --- |
| `http://localhost:8090/` | Swagger UI |
| `http://localhost:8090/api/v3` | OpenAPI 3 base (same path as hosted `/api/v3`) |
| `http://localhost:8090/api/v3/openapi.json` | Spec |

## Curl: pet and order

There is **no PUT** for orders. Status and `complete` are set on `POST /store/order`. Posting the same `id` again replaces the in-memory order (that is how this sample “updates”).

Look up a pet (seed data includes id `1`):

```bash
curl -s 'http://localhost:8090/api/v3/pet/findByStatus?status=available' | head
curl -s -H 'Accept: application/json' http://localhost:8090/api/v3/pet/1
```

Place an order for that pet:

```bash
curl -s -X POST http://localhost:8090/api/v3/store/order \
  -H 'Accept: application/json' -H 'Content-Type: application/json' \
  -d '{"id":900001,"petId":1,"quantity":1,"status":"placed","complete":false}'
```

Check order status:

```bash
curl -s -H 'Accept: application/json' http://localhost:8090/api/v3/store/order/900001
```

Mark it complete (same id, `complete: true`, `status: delivered`):

```bash
curl -s -X POST http://localhost:8090/api/v3/store/order \
  -H 'Accept: application/json' -H 'Content-Type: application/json' \
  -d '{"id":900001,"petId":1,"quantity":1,"status":"delivered","complete":true}'

curl -s -H 'Accept: application/json' http://localhost:8090/api/v3/store/order/900001
```

## Stop

Ctrl+C in the terminal running `run.sh` / `run.ps1`. The container is removed (`--rm`); seed users and orders are gone.

If a leftover container remains:

```bash
docker rm -fv petstore-openapi-server
```

Optional — also drop the local image tag (next `run.sh` will rebuild):

```bash
docker image rm context-mesh-petstore3:local
```

## Env

| Variable | Default | Meaning |
| --- | --- | --- |
| `PETSTORE_IMAGE` | `context-mesh-petstore3:local` | Local image tag |
| `PETSTORE_CONTAINER` | `petstore-openapi-server` | Container name |
| `PETSTORE_PORT` | `8090` | Host port |
| `PETSTORE_UPSTREAM` | `swaggerapi/petstore3:latest` | Base image (`--upstream` / `-Upstream` override this) |
| `PETSTORE_PLATFORM` | `linux/amd64` | `docker pull` / `build` / `run` platform |
| `PETSTORE_PULL_TIMEOUT` | `120` | Seconds to wait for `docker pull` |

If you change `PETSTORE_PORT`, pass the same origin to the Go processes:

```bash
go run ./examples/petstore/async-order-server -petstore-url http://localhost:PORT/api/v3
go run ./examples/petstore/mcp-server -petstore-url http://localhost:PORT/api/v3
```

To use the hosted demo instead of this container, run the Go processes with `-petstore hosted` (Docker is not required).

## Registry timeout

`DeadlineExceeded: context deadline exceeded` (or a hung `docker pull`) means the **Docker daemon** cannot reach Docker Hub. `curl` from the host can succeed while Docker Desktop’s VM cannot — a VPN is the usual cause.

1. Disconnect VPN
2. Restart Docker Desktop
3. Re-run `run.sh` / `run.ps1`

Or skip Docker: `-petstore hosted` on `async-order-server` and `mcp-server`.

Full demo: [../README.md](../README.md).
