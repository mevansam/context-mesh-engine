# api/v1

Internal REST mux mounted at `Options.APIPrefix` (paths below are relative after `StripPrefix`).

**Not** part of the public SDK. Applications import `engine` and `api` only.

| Route | When | Body |
| --- | --- | --- |
| `GET /health` | always | `{"status":"ok"}` |
| `GET /tools` | always | MCP `tools/list` result (`ttlMs`, `cacheScope`, `tools`) |
| `POST /plans/...`, `GET /openapi/...` | `ArazzoLoaders` set | plan execute / generated OAS |

`GET /tools` asks the live `mcp.Server` (in-memory session) so the JSON matches Streamable HTTP `tools/list`, including tools registered after `engine.New`. Optional `?cursor=` is MCP pagination.

Contributor contract: [docs/contributors/layout.md](../../../docs/contributors/layout.md).
