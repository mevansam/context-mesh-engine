# Examples

Runnable programs under [`examples/`](../../examples/). Run them from the **repository root**. In this checkout, `go.mod` `replace`s sibling `../go-sdk` and `../libopenapi`.

Do not run two examples (or `cmd/engine`) on `localhost:8080` at the same time.

| Example | Command | What it demonstrates |
| --- | --- | --- |
| [minimal](#minimal) | `go run ./examples/minimal` | Engine-owned listener via `ListenAndServe` |
| [embed-handler](#embed-handler) | `go run ./examples/embed-handler` | Your `http.Server` using `Handler()` |
| [arazzo-fs](#arazzo-fs) | `go run ./examples/arazzo-fs` | `FileLoader` + stub `Executor`, MCP `run_*` and REST plans |
| [petstore](#petstore) | `go run ./examples/petstore/mcp-server` | Arazzo + HTTP executor over [petstore3](https://petstore3.swagger.io/) and the async order adapter |

`cmd/engine` is a fourth runnable (`go run ./cmd/engine -addr localhost:8080`). It is the sample **product** binary (flags + SIGINT shutdown), not an SDK usage example. Flags: [getting started](getting-started.md#run-the-sample-binary).

After any of these are listening:

```bash
curl -s http://localhost:8080/api/health
# {"status":"ok"}
curl -s http://localhost:8080/api/tools
```

MCP clients connect to `http://localhost:8080/mcp`. See [MCP client](usage.md#mcp-client). Stop with Ctrl+C.

---

## minimal

**Source:** [`examples/minimal/main.go`](../../examples/minimal/main.go)

```bash
go run ./examples/minimal
```

Smallest embed: `engine.New` + one `mcp.AddTool` (`ping`) + `e.ListenAndServe`. The engine owns `http.Server`, bind, and shutdown when the context is cancelled.

Use this when the process does not already have an HTTP server.

---

## embed-handler

**Source:** [`examples/embed-handler/main.go`](../../examples/embed-handler/main.go)

```bash
go run ./examples/embed-handler
```

Mount the same MCP + REST mux on an `http.Server` you construct. Call `e.Handler()`, set `ReadHeaderTimeout`, leave `WriteTimeout` unset.

Use this when you already own listen/TLS/shutdown.

Do not wrap `e.Handler()` in Gin, a buffering logger, or `http.TimeoutHandler`. REST timeouts are already applied under `Options.APIPrefix`.

---

## arazzo-fs

**Source:** [`examples/arazzo-fs/main.go`](../../examples/arazzo-fs/main.go)

Must run from the repository root so `testdata/arazzo/plans` resolves (cwd-relative path in `main.go`):

```bash
go run ./examples/arazzo-fs
```

Loads `testdata/arazzo/plans` (`x-planId: petstore`, versions `1.0.0` and `1.1.0`). Registers MCP `query`, `run_petstore_v1.0.0`, `run_petstore_v1.1.0`. Stub `Executor` always returns HTTP 200. Dummy `QueryMatcher` always selects `pingHealth` on the latest petstore plan.

```bash
curl -s http://localhost:8080/api/openapi/petstore
curl -s -X POST http://localhost:8080/api/plans/petstore/pingHealth \
  -H 'Content-Type: application/json' -d '{"name":"demo"}'
curl -s -X POST http://localhost:8080/api/plans/petstore/v1.0.0/pingHealth \
  -H 'Content-Type: application/json' -d '{"name":"demo"}'
curl -s -X POST http://localhost:8080/api/plans/query \
  -H 'Content-Type: application/json' -d '{"query":"is the api up","data":{"name":"demo"}}'
```

Workflows in the fixtures: `pingHealth` (both versions), `echoName` (1.1.0 only). OpenAPI sources live in `testdata/arazzo/sources/openapi.yaml` (not passed to `FileLoader`).

Options and schemas: [arazzo.md](arazzo.md).

---

## petstore

**Source:** [`examples/petstore/`](../../examples/petstore/)

Two processes, from the repository root:

```bash
go run ./examples/petstore/async-order-server
go run ./examples/petstore/mcp-server
```

Arazzo plan (`x-planId: petstore`, version `1.0.1`) based on the [1.1 spec example](https://spec.openapis.org/arazzo/latest.html), split into `retrievePet`, `purchasePet`, and `checkOrderStatus`. Hosted API is [petstore3.swagger.io](https://petstore3.swagger.io/). `purchasePet` sends/receives through the local AsyncAPI HTTP adapter, which calls `POST /store/order`.

Curl, order-status, and MCP agent walkthrough: [examples/README.md](../../examples/README.md#petstore-demo).

```bash
curl -s -X POST http://localhost:8080/api/plans/petstore/retrievePet \
  -H 'Content-Type: application/json' \
  -d '{"username":"user1","password":"abc123","status":"available"}'
```

