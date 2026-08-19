# Arazzo plugins

Public package `github.com/mevansam/context-mesh-engine/arazzo`.

| Type / func | Role |
| --- | --- |
| `Loader`, `Source` | Pluggable spec source |
| `NewFileLoader` | Recursive `.yaml` / `.yml` / `.json` |
| `Executor`, `ExecutionRequest`, `ExecutionResponse` | Backend HTTP for workflow steps (aliases of libopenapi) |
| `ToolDocTemplates`, `ToolDocContext` | `text/template` recipes for MCP tool name/title/description |

Do not import `internal/plans`. Wire loaders through `engine.Options.ArazzoLoaders`.

- Users: [docs/users/arazzo.md](../docs/users/arazzo.md)
- Contributors: [docs/contributors/arazzo.md](../docs/contributors/arazzo.md)
