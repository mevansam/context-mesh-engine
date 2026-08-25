# api/v1

Internal REST mux mounted at `Options.APIPrefix` (paths below are relative after `StripPrefix`).

**Not** part of the public SDK. Applications import `engine` and `api` only.

| Route | When | Body |
| --- | --- | --- |
| `GET /health` | always | `{"status":"ok"}` |
| `GET /tools` | always | MCP `tools/list` envelope; Arazzo descriptions use REST templates |
| `POST /plans/...`, `GET /openapi/...` | `ArazzoLoaders` set | plan execute / generated OAS |

`GET /tools` asks the live `mcp.Server` (in-memory session) so names, schemas, and non-Arazzo tools match Streamable HTTP `tools/list`, including tools registered after `engine.New`. Arazzo plan/query `description` fields are replaced with REST templates. Optional `?cursor=` is MCP pagination.

Contributor contract: [docs/contributors/layout.md](../../../docs/contributors/layout.md).
