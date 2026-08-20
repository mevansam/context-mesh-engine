# arazzo-fs

Run from the **repository root** so `testdata/arazzo/plans` resolves (path in `main.go` is cwd-relative):

```bash
go run ./examples/arazzo-fs
```

Loads sample Pet Store Arazzo plans. Registers MCP `query`, `run_petstore_v1.0.0`, `run_petstore_v1.1.0`. Serves:

- `GET /api/tools` (MCP `tools/list` result)
- `POST /api/plans/query` (dummy matcher → latest petstore `pingHealth`)
- `POST /api/plans/petstore/{workflowId}` (latest = 1.1.0)
- `POST /api/plans/petstore/v1.0.0/{workflowId}`
- `GET /api/openapi/petstore`
- `GET /api/openapi/petstore/v1.1.0`

Stub `Executor` always returns HTTP 200. Dummy `QueryMatcher` always selects `petstore` / `pingHealth` (not semantic search). Replace both in your app.

```bash
curl -s -X POST http://localhost:8080/api/plans/query \
  -H 'Content-Type: application/json' -d '{"query":"is the api up","data":{"name":"demo"}}'
```

User guide: [docs/users/arazzo.md](../../docs/users/arazzo.md). Run notes: [docs/users/examples.md](../../docs/users/examples.md#arazzo-fs).
