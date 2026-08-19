# arazzo-fs

Run from the **repository root** so `testdata/arazzo/plans` resolves (path in `main.go` is cwd-relative):

```bash
go run ./examples/arazzo-fs
```

Loads sample Pet Store Arazzo plans. Registers MCP `query`, `run_petstore_v1.0.0`, `run_petstore_v1.1.0`. Serves:

- `POST /api/v1/plans/petstore/{workflowId}` (latest = 1.1.0)
- `POST /api/v1/plans/petstore/v1.0.0/{workflowId}`
- `GET /api/v1/openapi/petstore`
- `GET /api/v1/openapi/petstore/v1.1.0`

Stub `Executor` always returns HTTP 200. Replace it with a real backend client in your app.

User guide: [docs/users/arazzo.md](../../docs/users/arazzo.md). Run notes: [docs/users/examples.md](../../docs/users/examples.md#arazzo-fs).
