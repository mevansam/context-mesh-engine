# minimal

Smallest embed: `engine.New` + one `mcp.AddTool` (`ping`) + `e.ListenAndServe`. The engine owns `http.Server`, bind, and shutdown when the context is cancelled.

Use this when the process does not already have an HTTP server.

## Run

From the **repository root**:

```bash
go run ./examples/minimal
go run ./examples/minimal -dual
```

Default is REST only (`GET /api/health`, `GET /api/tools`). `-dual` also mounts MCP Streamable HTTP at `/mcp`.

```bash
curl -s http://localhost:8080/api/health
# {"status":"ok"}
curl -s http://localhost:8080/api/tools
```

Do not run this at the same time as another example (or `cmd/engine`) on `localhost:8080`.

SDK: [docs/users/configuration.md](../../docs/users/configuration.md#engine-owned-listener). What this example is for: [docs/users/examples.md](../../docs/users/examples.md#minimal).
