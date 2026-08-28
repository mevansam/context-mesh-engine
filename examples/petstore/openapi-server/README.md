# openapi-server

Runs the official [Swagger Petstore 3](https://github.com/swagger-api/swagger-petstore) OpenAPI server in Docker so the petstore example does not depend on the hosted [petstore3.swagger.io](https://petstore3.swagger.io/) demo.

The [Dockerfile](Dockerfile) starts from [swaggerapi/petstore3:latest](https://hub.docker.com/r/swaggerapi/petstore3) and adds an [SLF4J](http://www.slf4j.org/) 1.7 binding so Jetty does not print `StaticLoggerBinder` errors. Both the base image and the binding jar are overridable; see [Build](#build). Scripts **build** `context-mesh-petstore3:local` when that tag is missing (they pull the upstream base only if it is not already local). The published image is `linux/amd64`; Docker Desktop on Apple Silicon runs it under emulation.

Jetty still prints `jetty-runner is deprecated` on startup; that warning comes from upstream and is harmless.

The container runs in the **foreground** (`docker run --rm`). Ctrl+C stops it and **removes** the container, which deletes in-memory users and orders. Each run is a clean store.

## Requirements

- [Docker](https://docs.docker.com/get-docker/) (Desktop or engine) on `PATH`
- `curl` on Unix (`run.sh` downloads the SLF4J jar on the host)

## Launch

From the **repository root**, or from this directory:

```bash
./examples/petstore/openapi-server/run.sh
```

Windows (PowerShell):

```powershell
./examples/petstore/openapi-server/run.ps1
```

That uses the defaults: upstream `swaggerapi/petstore3:latest` and Maven Central [slf4j-simple 1.7.36](https://repo1.maven.org/maven2/org/slf4j/slf4j-simple/1.7.36/slf4j-simple-1.7.36.jar). If `context-mesh-petstore3:local` already exists, the scripts **reuse it** and ignore `--upstream` / `--slf4j` until you pass `--rebuild` / `-Rebuild`.

Host **8090** is used so it does not collide with `mcp-server` on `8080`. The container still listens on 8080 inside Docker.

| URL | Role |
| --- | --- |
| `http://localhost:8090/` | Swagger UI |
| `http://localhost:8090/api/v3` | OpenAPI 3 base (same path as hosted `/api/v3`) |
| `http://localhost:8090/api/v3/openapi.json` | Spec |

## Build

Always pass `--rebuild` / `-Rebuild` when changing the upstream image or the SLF4J jar. Otherwise the existing local tag is used as-is.

CLI flags override env (`PETSTORE_UPSTREAM`, `PETSTORE_SLF4J`). Staging (Jetty XML plus a downloaded or copied jar) is written to `.build/` next to this README and is gitignored. Do not commit it. Use `run.sh` / `run.ps1`; do not `docker build` this directory directly — the Dockerfile expects that prepared context.

The binding must be an **SLF4J 1.7** `StaticLoggerBinder` implementation (`slf4j-simple`, `slf4j-log4j12`, `slf4j-jdk14`, …) to match `slf4j-api` 1.7.36 in the upstream image. The source filename is not assumed; the image always installs it as `slf4j-impl.jar`. A checksum is verified only for the default Maven Central `slf4j-simple` URL. A log4j12 binding is only the SLF4J adapter — it does not include log4j itself.

### Rebuild with defaults

```bash
./examples/petstore/openapi-server/run.sh --rebuild
```

```powershell
./examples/petstore/openapi-server/run.ps1 -Rebuild
```

### Replace the upstream image

`--upstream` / `-Upstream` is the Docker `FROM` image (Docker Hub, a mirror, or a private registry). Default: `swaggerapi/petstore3:latest`.

```bash
# Pin a tag
./examples/petstore/openapi-server/run.sh --rebuild --upstream swaggerapi/petstore3:1.0.21

# Pull from a registry mirror / Artifactory virtual Docker repo
./examples/petstore/openapi-server/run.sh --rebuild \
  --upstream my.jfrog.io/docker-remote/swaggerapi/petstore3:latest
```

```powershell
./examples/petstore/openapi-server/run.ps1 -Rebuild -Upstream swaggerapi/petstore3:1.0.21
./examples/petstore/openapi-server/run.ps1 -Rebuild -Upstream my.jfrog.io/docker-remote/swaggerapi/petstore3:latest
```

```bash
PETSTORE_UPSTREAM=swaggerapi/petstore3:1.0.21 ./examples/petstore/openapi-server/run.sh --rebuild
```

### Replace the SLF4J binding

`--slf4j` / `-Slf4j` is either an `http(s)` URL (downloaded on the host into `.build/`) or a local `.jar` path.

```bash
# Same as default: Maven Central slf4j-simple 1.7.36
./examples/petstore/openapi-server/run.sh --rebuild \
  --slf4j https://repo1.maven.org/maven2/org/slf4j/slf4j-simple/1.7.36/slf4j-simple-1.7.36.jar

# Artifactory (or any Maven layout) — full jar URL, not the repo root
./examples/petstore/openapi-server/run.sh --rebuild \
  --slf4j https://artifactory.example.com/artifactory/maven-central/org/slf4j/slf4j-simple/1.7.36/slf4j-simple-1.7.36.jar

# Local jar (any 1.7 binding name)
./examples/petstore/openapi-server/run.sh --rebuild --slf4j ./slf4j-simple-1.7.36.jar
./examples/petstore/openapi-server/run.sh --rebuild --slf4j /opt/jars/slf4j-log4j12-1.7.36.jar

# No binding (upstream StaticLoggerBinder noise remains)
./examples/petstore/openapi-server/run.sh --rebuild --no-slf4j
```

```powershell
./examples/petstore/openapi-server/run.ps1 -Rebuild -Slf4j https://repo1.maven.org/maven2/org/slf4j/slf4j-simple/1.7.36/slf4j-simple-1.7.36.jar
./examples/petstore/openapi-server/run.ps1 -Rebuild -Slf4j https://artifactory.example.com/artifactory/maven-central/org/slf4j/slf4j-simple/1.7.36/slf4j-simple-1.7.36.jar
./examples/petstore/openapi-server/run.ps1 -Rebuild -Slf4j .\slf4j-simple-1.7.36.jar
./examples/petstore/openapi-server/run.ps1 -Rebuild -Slf4j C:\jars\slf4j-log4j12-1.7.36.jar
./examples/petstore/openapi-server/run.ps1 -Rebuild -NoSlf4j
```

```bash
PETSTORE_SLF4J=https://artifactory.example.com/artifactory/maven-central/org/slf4j/slf4j-log4j12/1.7.36/slf4j-log4j12-1.7.36.jar \
  ./examples/petstore/openapi-server/run.sh --rebuild

PETSTORE_SLF4J=skip ./examples/petstore/openapi-server/run.sh --rebuild
```

### Replace both

```bash
./examples/petstore/openapi-server/run.sh --rebuild \
  --upstream my.jfrog.io/docker-remote/swaggerapi/petstore3:latest \
  --slf4j https://artifactory.example.com/artifactory/maven-central/org/slf4j/slf4j-simple/1.7.36/slf4j-simple-1.7.36.jar
```

```powershell
./examples/petstore/openapi-server/run.ps1 -Rebuild `
  -Upstream my.jfrog.io/docker-remote/swaggerapi/petstore3:latest `
  -Slf4j https://artifactory.example.com/artifactory/maven-central/org/slf4j/slf4j-simple/1.7.36/slf4j-simple-1.7.36.jar
```

```bash
./examples/petstore/openapi-server/run.sh --rebuild \
  --upstream my.jfrog.io/docker-remote/swaggerapi/petstore3:latest \
  --slf4j ./slf4j-log4j12-1.7.36.jar
```

```powershell
./examples/petstore/openapi-server/run.ps1 -Rebuild `
  -Upstream my.jfrog.io/docker-remote/swaggerapi/petstore3:latest `
  -Slf4j .\slf4j-log4j12-1.7.36.jar
```

```bash
./examples/petstore/openapi-server/run.sh --rebuild \
  --upstream swaggerapi/petstore3:latest \
  --no-slf4j
```

```powershell
./examples/petstore/openapi-server/run.ps1 -Rebuild `
  -Upstream swaggerapi/petstore3:latest `
  -NoSlf4j
```

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
| `PETSTORE_SLF4J` | Maven Central `slf4j-simple` 1.7.36 jar URL | SLF4J 1.7 binding URL, local `.jar` path, or `skip` (`--slf4j` / `-Slf4j` / `--no-slf4j`) |
| `PETSTORE_PLATFORM` | `linux/amd64` | `docker pull` / `build` / `run` platform |
| `PETSTORE_PULL_TIMEOUT` | `120` | Seconds to wait for `docker pull` and for the host SLF4J `curl` / download |

If you change `PETSTORE_PORT`, pass the same origin to the Go processes:

```bash
go run ./examples/petstore/async-order-server -petstore-url http://localhost:PORT/api/v3
go run ./examples/petstore/mcp-server -petstore-url http://localhost:PORT/api/v3
```

To use the hosted demo instead of this container, run the Go processes with `-petstore hosted` (Docker is not required).

## Registry timeout

`DeadlineExceeded: context deadline exceeded` (or a hung `docker pull`) means the **Docker daemon** cannot reach Docker Hub. `curl` from the host can succeed while Docker Desktop’s VM cannot — a VPN is the usual cause. An SLF4J URL download uses host `curl` / `Invoke-WebRequest`, so Maven/Artifactory reachability is independent of the Docker VM.

1. Disconnect VPN
2. Restart Docker Desktop
3. Re-run `run.sh` / `run.ps1`

Or skip Docker: `-petstore hosted` on `async-order-server` and `mcp-server`.

Full demo: [../README.md](../README.md).
