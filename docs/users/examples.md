# Examples

Runnable programs under [`examples/`](../../examples/). They are the supported way to see how the SDK is meant to be used. Run them from the **repository root** (this checkout `replace`s sibling `../go-sdk` and `../libopenapi`).

Do not run two examples (or `cmd/engine`) on `localhost:8080` at the same time. Default HTTP surface is **REST only**. Each example accepts `-dual` to also mount MCP Streamable HTTP at `/mcp`.

Flags, curl walkthroughs, and implementation notes live in each example’s **local README**. This page describes **what each program is for** so you can pick the right starting point.

## Choose an example

| Example | Start here if you want to… | Adapters it implements |
| --- | --- | --- |
| [minimal](#minimal) | Own nothing but the engine: `New` + `ListenAndServe` | none (sample `ping` MCP tool only) |
| [embed-handler](#embed-handler) | Drop the engine mux onto an `http.Server` you already own | none (sample `ping` MCP tool only) |
| [arazzo-fs](#arazzo-fs) | Load plans from disk and exercise `run_*`, REST execute, and `query` without real backends | `FileLoader`, stub `Executor`, dummy `QueryMatcher` |
| [petstore](#petstore) | Run a real multi-step Arazzo plan against live HTTP APIs | `FileLoader`, HTTP `Executor`, `RequestPreprocessor`, `SecretsProvider`, OAuth wraps (no `QueryMatcher`) |

`cmd/engine` (`go run ./cmd/engine`) is a **product-shaped** binary (flags, SIGINT shutdown, sample `ping` tool), not an SDK usage example. It can load `-specs` but does not set an executor or matcher. See [Getting started](getting-started.md#run-the-sample-binary).

Index of the same programs: [`examples/README.md`](../../examples/README.md).

---

## minimal

**Path:** [`examples/minimal`](../../examples/minimal/README.md)

**Demonstrates:** the smallest embed. `engine.New` with a listen address, one `mcp.AddTool` (`ping`), and `e.ListenAndServe`. The engine owns `http.Server`, bind, and shutdown when the context is cancelled.

**Does not demonstrate:** Arazzo loaders, executors, query matching, tool help, or custom REST controllers.

**When to copy:** a new process whose only HTTP job is this engine.

```bash
go run ./examples/minimal
go run ./examples/minimal -dual
```

```bash
curl -s http://localhost:8080/api/health
curl -s http://localhost:8080/api/tools
```

Configuration counterpart: [Start the server — engine-owned listener](configuration.md#engine-owned-listener).

---

## embed-handler

**Path:** [`examples/embed-handler`](../../examples/embed-handler/README.md)

**Demonstrates:** mounting `e.Handler()` on an `http.Server` you construct. Sets `ReadHeaderTimeout` and leaves `WriteTimeout` unset — the required pattern when you already own listen, TLS, or process shutdown.

**Does not demonstrate:** Arazzo, wrapping the handler in another framework, or custom middleware on `/mcp`.

**When to copy:** the engine must sit beside existing routes or a TLS listener you control. Do not wrap `e.Handler()` in Gin, a buffering logger, or `http.TimeoutHandler`.

```bash
go run ./examples/embed-handler
go run ./examples/embed-handler -dual
```

Configuration counterpart: [Start the server — your own HTTP server](configuration.md#your-own-http-server).

---

## arazzo-fs

**Path:** [`examples/arazzo-fs`](../../examples/arazzo-fs/README.md)

**Demonstrates:** turning a directory of Arazzo YAML into MCP tools and REST plan routes **without** calling real backends.

| Piece | What this example does |
| --- | --- |
| [`Loader`](adapters.md#loader) | `arazzo.NewFileLoader(plansDir)` (positional argument) |
| [`Executor`](adapters.md#executor) | Stub that always returns HTTP 200 and `{"status":"ok"}` |
| [`QueryMatcher`](adapters.md#querymatcher) | Dummy that always selects `petstore` / `pingHealth` |
| [`ToolHelpLookup`](adapters.md#toolhelplookup) | unset (built-in descriptions) |

It loads the fixtures in `testdata/arazzo/plans/` (`x-planId: petstore`, versions `1.0.0` and `1.1.0`). OpenAPI sources stay in `testdata/arazzo/sources/` and are **not** passed to `FileLoader`. Registered tools: `query`, `run_petstore_v1.0.0`, `run_petstore_v1.1.0`. Workflows: `pingHealth` (both versions), `echoName` (1.1.0 only).

**When to copy:** you need the wiring for loaders + executor + matcher before you have production HTTP. Replace the stub executor and dummy matcher in your app.

```bash
go run ./examples/arazzo-fs testdata/arazzo/plans
go run ./examples/arazzo-fs -dual testdata/arazzo/plans
```

```bash
curl -s http://localhost:8080/api/openapi
curl -s http://localhost:8080/api/openapi/petstore
curl -s -X POST http://localhost:8080/api/plans/petstore/pingHealth \
  -H 'Content-Type: application/json' -d '{"name":"demo"}'
curl -s -X POST http://localhost:8080/api/plans/query \
  -H 'Content-Type: application/json' \
  -d '{"query":"is the api up","data":{"name":"demo"}}'
```

Plan contracts: [Arazzo plans](arazzo.md). Live backends: [petstore](#petstore).

---

## petstore

**Path:** [`examples/petstore`](../../examples/petstore/README.md)

**Demonstrates:** an end-to-end governed plan over **real HTTP**: login, find a pet, place an order (via an AsyncAPI-shaped adapter), and check order status. This is the example that shows what an `Executor` looks like in production, not a stub.

Three processes plus a demo IdP:

| Process | Listen | Role |
| --- | --- | --- |
| `openapi-server` | `localhost:8090` | Official Petstore 3 in Docker |
| `auth-server` | `localhost:8092` | Issues client + end-user JWTs (`loginUser` + `getUserByName`) |
| `async-order-server` | `localhost:8091` | HTTP adapter for the spec’s AsyncAPI order flow; calls Petstore `POST /store/order` |
| `mcp-server` | `localhost:8080` | `context-mesh-engine` with `x-planId: petstore` |

| Piece | What this example does |
| --- | --- |
| [`Loader`](adapters.md#loader) | `FileLoader` on `mcp-server/plans/` |
| [`Executor`](adapters.md#executor) | HTTP client; mints a downstream JWT from `SecretsProvider` |
| [`PolicyLoader`](adapters.md#policyloader) | `FilePolicyLoader`; `userStatus` from end-user JWT (`input.auth.endUser`), not `http.send` |
| [`RequestPreprocessor`](adapters.md#requestpreprocessor) | Verifies `X-End-User-Token`; copies client `TokenInfo` |
| [`SecretsProvider`](adapters.md#secretsprovider) | `MapSecrets{"downstream-hmac": ...}` |
| [`QueryMatcher`](adapters.md#querymatcher) | **unset** — `query` is not registered; callers use `run_petstore_v0.0.1` or REST execute |

Workflows: `retrievePet`, `purchasePet`, `checkOrderStatus`. `GET /api/openapi` is the catalog index (`$ref` to the petstore child spec). `GET /api/openapi/petstore` describes those execute paths. `GET /api/tools` is the REST form of MCP `tools/list` (same names/schemas; REST-specific Arazzo descriptions).

**When to copy:** you are implementing an HTTP `Executor`, wiring `sourceDescriptions` to live origins, or showing an agent a real `run_*` tool. For natural-language `query`, start from [arazzo-fs](#arazzo-fs) and keep this executor.

Default Petstore target is local Docker (`-petstore local`). `-petstore hosted` uses [petstore3.swagger.io](https://petstore3.swagger.io/) (often flaky). Seed users, mint JWTs, and MCP agent prompts: the [petstore README](../../examples/petstore/README.md).

```bash
./examples/petstore/openapi-server/run.sh   # Docker Petstore on :8090
go run ./examples/petstore/async-order-server
go run ./examples/petstore/auth-server
go run ./examples/petstore/mcp-server                 # REST only
go run ./examples/petstore/mcp-server -dual           # also /mcp
```

```bash
curl -s -X POST http://localhost:8080/api/plans/petstore/retrievePet \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLIENT" \
  -H "X-End-User-Token: $USER" \
  -d '{"status":"available"}'
```

Mint `$CLIENT` / `$USER` from `auth-server` (`POST /oauth/token`). See the [petstore README](../../examples/petstore/README.md).

---

## What the examples do *not* cover

| Topic | Where to read instead |
| --- | --- |
| Every `Options` field | [Configuration](configuration.md#options-reference) |
| Implementing `ToolHelpLookup` | [Adapters — ToolHelpLookup](adapters.md#toolhelplookup) |
| Implementing `PolicyLoader` | [Adapters — PolicyLoader](adapters.md#policyloader) |
| Custom REST `Controller` | [Adapters — REST controllers](adapters.md#rest-controllers) |
| MCP client headers | [Configuration — MCP client](configuration.md#mcp-client) |

## Next

- [Configuration](configuration.md)
- [Adapters](adapters.md)
- [Arazzo plans](arazzo.md)
