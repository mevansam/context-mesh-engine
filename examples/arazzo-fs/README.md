# arazzo-fs

Arazzo `FileLoader` plus a stub `Executor` and a dummy `QueryMatcher`. Registers MCP `query` / `run_*` tools and REST `/plans` + `/openapi`.

## Run

The plans directory is a positional argument. Relative paths are cwd-relative. From the **repository root**:

```bash
go run ./examples/arazzo-fs testdata/arazzo/plans
go run ./examples/arazzo-fs -dual testdata/arazzo/plans
```

Default is REST only. `-dual` also mounts MCP Streamable HTTP at `/mcp`.

Do not run this at the same time as another example (or `cmd/engine`) on `localhost:8080`.

## What it loads

The sample fixtures (`x-planId: petstore`, versions `1.0.0` and `1.1.0`) register MCP `query`, `run_petstore_v1.0.0`, and `run_petstore_v1.1.0`. Workflows: `pingHealth` (both versions), `echoName` (1.1.0 only). OpenAPI sources live in `testdata/arazzo/sources/openapi.yaml` (not passed to `FileLoader`).

Serves:

- `GET /api/tools` (MCP `tools/list` envelope; Arazzo descriptions are REST-specific)
- `POST /api/plans/query` (dummy matcher → latest petstore `pingHealth`)
- `POST /api/plans/petstore/{workflowId}` (latest = 1.1.0)
- `POST /api/plans/petstore/v1.0.0/{workflowId}`
- `GET /api/openapi/petstore`
- `GET /api/openapi/petstore/v1.1.0`

Stub `Executor` always returns HTTP 200. Dummy `QueryMatcher` always selects `petstore` / `pingHealth` (not semantic search). Replace both in your app.

## REST

```bash
curl -s http://localhost:8080/api/openapi/petstore
curl -s -X POST http://localhost:8080/api/plans/petstore/pingHealth \
  -H 'Content-Type: application/json' -d '{"name":"demo"}'
curl -s -X POST http://localhost:8080/api/plans/petstore/v1.0.0/pingHealth \
  -H 'Content-Type: application/json' -d '{"name":"demo"}'
curl -s -X POST http://localhost:8080/api/plans/query \
  -H 'Content-Type: application/json' -d '{"query":"is the api up","data":{"name":"demo"}}'
```

Arazzo contracts: [docs/users/arazzo.md](../../docs/users/arazzo.md). How to replace the stub executor and dummy matcher: [docs/users/adapters.md](../../docs/users/adapters.md). Live Petstore HTTP: [petstore](../petstore/README.md). What this example is for: [docs/users/examples.md](../../docs/users/examples.md#arazzo-fs).
