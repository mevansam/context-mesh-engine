# embed-handler

Mount the engine mux on an `http.Server` you construct. Call `e.Handler()`, set `ReadHeaderTimeout`, leave `WriteTimeout` unset.

Use this when you already own listen/TLS/shutdown.

Do not wrap `e.Handler()` in Gin, a buffering logger, or `http.TimeoutHandler`. REST timeouts are already applied under `Options.APIPrefix`.

## Run

From the **repository root**:

```bash
go run ./examples/embed-handler
go run ./examples/embed-handler -dual
```

Default is REST only (`GET /api/health`, `GET /api/tools`). `-dual` also mounts MCP Streamable HTTP at `/mcp`.

```bash
curl -s http://localhost:8080/api/health
# {"status":"ok"}
curl -s http://localhost:8080/api/tools
```

Do not run this at the same time as another example (or `cmd/engine`) on `localhost:8080`.

SDK: [docs/users/usage.md](../../docs/users/usage.md#run-vs-embed).
