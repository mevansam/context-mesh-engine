# Arazzo plans (SDK users)

Load [Arazzo](https://spec.openapis.org/arazzo/latest.html) workflow documents, expose **one MCP tool per plan version**, and execute the same workflows over REST. MCP and REST share one runner.

Public package: `github.com/mevansam/context-mesh-engine/arazzo`.

This is not enabled until `engine.Options.ArazzoLoaders` is non-empty. Empty loaders: no `query` tool, no `run_*` tools, no `/api/v1/plans` or `/api/v1/openapi` routes.

## What `engine.New` registers

When `ArazzoLoaders` is set, `New` loads every document from every loader, then:

| Surface | Name / path |
| --- | --- |
| MCP `query` | `query` — semantically match a natural-language request to a plan and execute it (same contract as REST query) |
| MCP `run_*` | one per catalog entry; default name `run_{{.SafePlanID}}_v{{.SafeVersion}}` |
| REST `query` | `POST /api/v1/plans/query` |
| REST execute (latest) | `POST /api/v1/plans/{planId}/{workflowId}` |
| REST execute (versioned) | `POST /api/v1/plans/{planId}/{version}/{workflowId}` |
| OpenAPI (latest) | `GET /api/v1/openapi/{planId}` |
| OpenAPI (versioned) | `GET /api/v1/openapi/{planId}/{version}` |

`{version}` in URLs is **`v` + Arazzo `info.version`**. Example: `info.version: 1.0.0` → path token `v1.0.0`.

**Latest** among versions of the same `x-planId`: if every version is semver (with or without a leading `v`), pick the highest semver; otherwise lexicographic maximum (`golang.org/x/mod/semver` + `sort.Strings`).

Each execute call constructs a **new** libopenapi `arazzo.Engine`. That type is not concurrency-safe; the catalog caches parsed docs and resolved sources instead.

## Spec requirements

Each Arazzo file must have:

```yaml
info:
  title: ...
  version: 1.0.0          # required; skipped if empty
  x-planId: petstore      # required Info extension; skipped if missing
```

Skip (log a warning, do not fail `New`):

- no `info`
- empty `x-planId`
- empty `info.version`
- non-`.yaml`/`.yml`/`.json` files (`FileLoader` never opens them)

Fail `New`:

- parse error
- Arazzo structural validation errors (`libopenapi/arazzo.Validate`)
- source resolution failure (OpenAPI/Arazzo `sourceDescriptions`)
- two documents with the same `(x-planId, info.version)`
- two rendered MCP tool **names** colliding
- invalid `ToolDoc` templates (parse/execute)

Fixtures showing skip vs load: `testdata/arazzo/plans/` (`no-plan-id.yaml` is skipped; `ignore.txt` is ignored).

## Loader

```go
type Loader interface {
    Load(ctx context.Context) ([]Source, error)
}

type Source struct {
    URI     string // error messages (usually a filesystem path)
    Data    []byte // raw Arazzo bytes
    BaseURL string // file:// directory URL with a trailing slash
}
```

You may implement `Loader` (HTTP, embed, git). The built-in loader:

```go
arazzo.NewFileLoader("/path/to/plans")
```

`FileLoader` walks **recursively** for `.yaml`, `.yml`, `.json`. Point it at the **plans directory**, not a tree that also contains OpenAPI files you do not want parsed as Arazzo.

`Source.BaseURL` is `file://` + the Arazzo file’s directory **with a trailing slash**. Relative `sourceDescriptions[].url` values resolve against that directory (`../sources/openapi.yaml` is the intended layout). Without the trailing slash, `../` climbs one extra level (RFC 3986).

## Embed

```go
e, err := engine.New(engine.Options{
    Addr: "localhost:8080",
    ArazzoLoaders: []arazzo.Loader{
        arazzo.NewFileLoader("/path/to/plans"),
    },
    ArazzoExecutor: myExecutor{}, // nil: OpenAPI works; execute is 501
    PublicBaseURL:  "http://localhost:8080",
})
```

`PublicBaseURL` is the origin written into MCP tool descriptions (REST POST and OpenAPI GET URLs). It is not `Addr`. Empty → path-only URLs (`/api/v1/plans/...`).

Runnable sample: [examples/arazzo-fs](examples.md#arazzo-fs).

## Executor

The engine does not call backend APIs itself. You implement `arazzo.Executor` (alias of `libopenapi/arazzo.Executor`):

```go
type Executor interface {
    Execute(ctx context.Context, req *ExecutionRequest) (*ExecutionResponse, error)
}
```

| Type | Fields you typically use |
| --- | --- |
| `ExecutionRequest` | `OperationID`, `OperationPath`, `Method`, `Parameters`, `RequestBody`, `ContentType`, `Source` |
| `ExecutionResponse` | `StatusCode` (required for success criteria), `Headers`, `Body`, `URL`, `Method` |

```go
type myExecutor struct{}

func (myExecutor) Execute(_ context.Context, req *arazzo.ExecutionRequest) (*arazzo.ExecutionResponse, error) {
    return &arazzo.ExecutionResponse{
        StatusCode: 200,
        Body:       map[string]any{"ok": true},
    }, nil
}
```

Nil executor:

- `GET /api/v1/openapi/...` still works
- `POST /api/v1/plans/...` execute → **501** `{"error":"executor not configured"}`
- `POST /api/v1/plans/query` → **501** `query is not implemented` (matching is not wired; independent of executor)
- MCP `run_*` → tool error (`IsError: true`), not a JSON-RPC protocol error

This module does not ship an HTTP client executor. `examples/arazzo-fs` uses a stub that always returns 200.

## MCP and REST: `query`

Same job on both surfaces: the caller sends a **simple, direct** natural-language question plus a clear outline of the inputs they have. The engine semantically matches that against the plan registry, selects a plan, and executes it. The success payload is the same result object as direct execute.

| Surface | How |
| --- | --- |
| MCP | tool `query` |
| REST | `POST /api/v1/plans/query` |

Body / arguments:

```json
{ "query": "natural language", "data": { } }
```

`data` is the input outline (optional object). Override MCP display strings with `ToolDoc.QueryName`, `QueryTitle`, `QueryDescription` (literals, not templates).

This version returns MCP tool error / HTTP **501** `{"error":"query is not implemented"}` until matching is wired. Direct `run_*` and `POST /api/v1/plans/{planId}/...` are implemented.

## MCP: `run_*` arguments

```json
{
  "workflowId": "pingHealth",
  "inputs": { "name": "demo" }
}
```

`inputSchema` is JSON Schema `type: object` with `oneOf` branches:

```json
{
  "type": "object",
  "oneOf": [
    {
      "type": "object",
      "required": ["workflowId", "inputs"],
      "properties": {
        "workflowId": { "const": "pingHealth" },
        "inputs": { /* that workflow's Arazzo inputs schema */ }
      }
    }
  ]
}
```

Each branch uses `workflowId` **const** so overlapping workflow input schemas still validate uniquely.

Successful `run_*` returns structured content matching the REST result object below (go-sdk `StructuredContent`). Runner errors become MCP tool errors.

## REST execute

POST body is the workflow **inputs object** (no `workflowId` wrapper). `workflowId` is the path. Empty body is allowed (`{}` or no body).

```bash
curl -s -X POST http://localhost:8080/api/v1/plans/petstore/pingHealth \
  -H 'Content-Type: application/json' \
  -d '{"name":"demo"}'

curl -s -X POST http://localhost:8080/api/v1/plans/petstore/v1.0.0/pingHealth \
  -H 'Content-Type: application/json' \
  -d '{"name":"demo"}'
```

### Result JSON (MCP structured content and REST 200)

```json
{
  "workflowId": "pingHealth",
  "success": true,
  "inputs": { },
  "outputs": { },
  "steps": [
    {
      "stepId": "getHealth",
      "success": true,
      "statusCode": 200,
      "outputs": { },
      "error": "",
      "durationMs": 1,
      "retries": 0
    }
  ],
  "error": "",
  "durationMs": 1
}
```

`success: false` is still HTTP **200** if the runner returned a result. HTTP errors:

| Status | When |
| --- | --- |
| 400 | Invalid JSON body, or runner error that is not not-found / no-executor |
| 404 | Unknown `planId`, version, or `workflowId` |
| 501 | `ArazzoExecutor` is nil |

Body: `{"error":"<message>"}`.

## OpenAPI

`GET /api/v1/openapi/{planId}` and `GET /api/v1/openapi/{planId}/{version}` return OAS **3.1.0** JSON (`Content-Type: application/json`).

Paths **inside** that document are relative to `/api/v1` (the REST mux is `StripPrefix`’d). Example:

- HTTP: `GET /api/v1/openapi/petstore`
- Document path: `/plans/petstore/pingHealth` → real URL `POST /api/v1/plans/petstore/pingHealth`

Latest document: `/plans/{planId}/{workflowId}`. Versioned document: `/plans/{planId}/v{version}/{workflowId}`. Request body schema is that workflow’s Arazzo `inputs`.

404 if the plan or version is missing. OpenAPI does **not** require an executor.

## Tool documentation templates

`Options.ToolDoc` (`arazzo.ToolDocTemplates`) fields `Name`, `Title`, and `Description` are Go `text/template` **recipes** executed against `ToolDocContext`. They are not nested under `.ToolDoc`.

Write `{{.Title}}` for Arazzo `info.title`. Do **not** write `{{.ToolDoc.Title}}`.

`QueryName` / `QueryTitle` / `QueryDescription` are copied as-is (not parsed as templates). Empty fields fall back to `DefaultToolDocTemplates()`.

Default name recipe: `run_{{.SafePlanID}}_v{{.SafeVersion}}`. After render, the name is sanitized to MCP runes `[A-Za-z0-9_.-]` (spaces and other runes → `_`) and truncated to 128 characters.

`{workflowId}` in default REST URLs is a **literal placeholder** for the caller. `Addr` is not a template variable.

### `ToolDocContext` fields

| Field | Meaning |
| --- | --- |
| `PlanID` | `info.x-planId` |
| `Version` | raw `info.version` (no leading `v`) |
| `Title`, `Summary`, `Description` | Arazzo `info.*` |
| `Workflows` | `[]WorkflowDoc` (`ID`, `Summary`, `Description`, `SummaryOrDescription`) |
| `WorkflowIDs` | comma-separated workflow ids |
| `SafePlanID` | plan id with non `[A-Za-z0-9_-]` replaced (`_`). Dots not kept. |
| `SafeVersion` | version with non `[A-Za-z0-9_.-]` replaced. Dots kept. |
| `VersionSegment` | `"v" + Version` |
| `PublicBaseURL` | trimmed origin or empty |
| `APIRoot` | `PublicBaseURL + "/api/v1"` or `/api/v1` |
| `RESTExecuteLatestURL` | `{APIRoot}/plans/{PlanID}/{workflowId}` |
| `RESTExecuteVersionedURL` | `{APIRoot}/plans/{PlanID}/{VersionSegment}/{workflowId}` |
| `OpenAPILatestURL` | `{APIRoot}/openapi/{PlanID}` |
| `OpenAPIVersionedURL` | `{APIRoot}/openapi/{PlanID}/{VersionSegment}` |

Templates use `missingkey=zero`. Invalid template syntax fails `engine.New`.

## Sample binary

```bash
go run ./cmd/engine -addr localhost:8080 -specs testdata/arazzo/plans
```

Loads the sample Pet Store plans. Execute still needs an `Executor`; this binary does not set one. Use [examples/arazzo-fs](examples.md#arazzo-fs).

## Coding-agent checklist (Arazzo)

- Set `ArazzoLoaders`; otherwise plan routes and `run_*` tools do not exist.
- Point `FileLoader` at the plans dir only; OpenAPI sources stay beside it (`../sources/...`).
- Implement `Executor`; nil is 501 on execute, OpenAPI still works.
- MCP args for `run_*` wrap `{workflowId, inputs}`; REST execute POST body **is** `inputs`.
- MCP `query` and `POST /api/v1/plans/query` share `{query, data}` and the execute result object.
- Path version token is `v` + `info.version` (`v1.0.0`), not `1.0.0`.
- Generated OpenAPI `paths` keys omit the `/api/v1` prefix. They describe execute routes, not `/plans/query`.
- `PublicBaseURL` must be set if you want absolute URLs in MCP descriptions.
- Treat `query` / `POST /plans/query` as not implemented until matching is wired.

## Next

- [Examples](examples.md#arazzo-fs)
- Internals (contributors): [docs/contributors/arazzo.md](../contributors/arazzo.md)
