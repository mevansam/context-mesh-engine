# Arazzo plans (SDK users)

Load [Arazzo](https://spec.openapis.org/arazzo/latest.html) workflow documents, expose **one MCP tool per plan version**, and execute the same workflows over REST. MCP and REST share one runner.

Public package: `github.com/mevansam/context-mesh-engine/arazzo`.

This is not enabled until `engine.Options.ArazzoLoaders` is non-empty. Empty loaders: no `query` tool, no `run_*` tools, no plan or OpenAPI REST routes. `GET {APIPrefix}/tools` is always registered; with loaders it includes each `run_*`, and `query` only when `QueryMatcher` is set.

REST paths below use the default prefix `/api`. Set `Options.APIPrefix` (or `cmd/engine -api-prefix`) to change it. Generated OpenAPI `paths` omit that prefix either way.

## What `engine.New` registers

When `ArazzoLoaders` is set, `New` loads every document from every loader, then:

| Surface | Name / path |
| --- | --- |
| MCP `query` | `query` — only if `QueryMatcher` is set |
| MCP `run_*` | one per catalog entry; default name `run_{{.SafePlanID}}_v{{.SafeVersion}}` |
| REST `tools` | `GET /api/tools` — MCP `tools/list` envelope; Arazzo descriptions use REST templates |
| REST `query` | `POST /api/plans/query` — only if `QueryMatcher` is set |
| REST execute (latest) | `POST /api/plans/{planId}/{workflowId}` |
| REST execute (versioned) | `POST /api/plans/{planId}/{version}/{workflowId}` |
| OpenAPI (latest) | `GET /api/openapi/{planId}` |
| OpenAPI (versioned) | `GET /api/openapi/{planId}/{version}` |

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
    QueryMatcher:   myMatcher{},  // nil: query tool and POST /plans/query are omitted
    PublicBaseURL:  "http://localhost:8080",
    // DualMCPandREST: true,      // also mount /mcp; default is REST only
})
```

`PublicBaseURL` is the origin written into MCP tool descriptions (REST POST and OpenAPI GET URLs). It is not `Addr`. Empty → path-only URLs (`{APIPrefix}/plans/...`).

Runnable sample with a stub executor: [examples/arazzo-fs](../../examples/arazzo-fs/README.md). Live Petstore (MCP + async orders): [examples/petstore](../../examples/petstore/README.md).

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

- `GET /api/tools` still works (lists `run_*`; `query` only if `QueryMatcher` is set)
- `GET /api/openapi/...` still works
- `POST /api/plans/...` execute → **501** `{"error":"executor not configured"}`
- `POST /api/plans/query` is not registered if `QueryMatcher` is nil (**404**); if a matcher is set, same as execute (**501** executor, or **404** if the match is not loaded)
- MCP `run_*` → tool error (`IsError: true`), not a JSON-RPC protocol error
- MCP `query` is not registered if `QueryMatcher` is nil

This module does not ship an HTTP client executor. [examples/arazzo-fs](../../examples/arazzo-fs/README.md) uses a stub that always returns 200. [examples/petstore](../../examples/petstore/README.md) implements a real HTTP client against a local Docker Petstore 3 server and the local async order adapter.

## MCP and REST: `query`

Same job on both surfaces: the caller sends a **simple, direct** natural-language question plus a clear outline of the inputs they have. Your app’s [`QueryMatcher`](#querymatcher) selects a plan from a **global** registry (vector search, and so on). The engine then checks that plan is **loaded in this process** and executes it. Success payload is the workflow **outputs** object (same as direct execute).

| Surface | How |
| --- | --- |
| MCP | tool `query` |
| REST | `POST /api/plans/query` |

Body / arguments:

```json
{ "query": "natural language", "data": { } }
```

`data` is the input outline (optional object). Used as workflow inputs when the matcher does not set `Inputs`. Override display strings with `ToolDoc.QueryName`, `QueryTitle`, `QueryDescription` (MCP) and `RESTQueryDescription` (`GET /tools`).

| Matcher / catalog | REST | MCP |
| --- | --- | --- |
| `QueryMatcher` nil | route not registered (404) | tool not registered |
| Empty `query` | 400 | tool error |
| Matcher error | 400 | tool error |
| No selection (`nil` match) | 404 | tool error |
| Plan/version/workflow not loaded here | 404 | tool error |
| Match + execute OK | 200 outputs | structured content = outputs |

Direct `run_*` and `POST /api/plans/{planId}/...` do not use the matcher.

## QueryMatcher

Semantic matching is **not** part of this SDK. Implement `arazzo.QueryMatcher` in the application (typically a vector lookup against a global plan registry):

```go
type QueryMatcher interface {
    Match(ctx context.Context, req QueryRequest) (*QueryMatch, error)
}

type QueryRequest struct {
    Query   string
    Data    map[string]any
    Catalog PlanCatalog // loaded plans; interface value, not a copied slice
}

type QueryMatch struct {
    PlanID     string         // required (x-planId)
    Version    string         // empty → latest version loaded in this engine
    WorkflowID string         // required
    Inputs     map[string]any // nil → use QueryRequest.Data
}

type PlanCatalog interface {
    Get(planID, version string) (PlanSummary, bool)
    Latest(planID string) (PlanSummary, bool)
    Plans() iter.Seq[PlanSummary]
}
```

`Match` should return whatever the **global** registry selected. Do **not** treat `Catalog.Get == false` as “no match”; the engine verifies the loaded catalog after `Match` returns. A global hit that was never loaded into this process is HTTP **404** / MCP tool error (`… not loaded`).

`Catalog` is for optional lookup or listing of what this process loaded (for example to pick `workflowId` after an id match). You may ignore it. `Plans()` is an iterator: nothing is copied until you range it.

Empty `Version` means latest among versions **loaded here**, not latest in the global registry.

Wire it with `engine.Options.QueryMatcher`. Example: [examples/arazzo-fs](../../examples/arazzo-fs/README.md) ships a dummy matcher that always selects `petstore` / `pingHealth`.

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

Successful `run_*` returns structured content that **is** the workflow outputs object (go-sdk `StructuredContent`). Same JSON as REST 200. Runner errors become MCP tool errors.

## REST execute

POST body is the workflow **inputs object** (no `workflowId` wrapper). `workflowId` is the path. Empty body is allowed (`{}` or no body).

```bash
curl -s -X POST http://localhost:8080/api/plans/petstore/pingHealth \
  -H 'Content-Type: application/json' \
  -d '{"name":"demo"}'

curl -s -X POST http://localhost:8080/api/plans/petstore/v1.0.0/pingHealth \
  -H 'Content-Type: application/json' \
  -d '{"name":"demo"}'
```

### Result JSON (MCP structured content and REST 200)

The body is the workflow **outputs** map from the Arazzo document (not the engine trace). A workflow with no `outputs` returns `{}`.

```json
{
  "petId": 1
}
```

Generated OpenAPI `200` schemas use those output names as object properties. HTTP errors:

| Status | When |
| --- | --- |
| 400 | Invalid JSON body, workflow failed, or runner error that is not not-found / no-executor |
| 404 | Unknown `planId`, version, or `workflowId` |
| 501 | `ArazzoExecutor` is nil |

Body: `{"error":"<message>"}`.

## OpenAPI

`GET /api/openapi/{planId}` and `GET /api/openapi/{planId}/{version}` return OAS **3.1.0** JSON (`Content-Type: application/json`).

Paths **inside** that document omit `Options.APIPrefix` (the REST mux is `StripPrefix`’d). With the default prefix:

- HTTP: `GET /api/openapi/petstore`
- Document path: `/plans/petstore/pingHealth` → real URL `POST /api/plans/petstore/pingHealth`

Latest document: `/plans/{planId}/{workflowId}`. Versioned document: `/plans/{planId}/v{version}/{workflowId}`. Request body schema is that workflow’s Arazzo `inputs`. **200** schema is an object with a property per Arazzo `outputs` name.

404 if the plan or version is missing. OpenAPI does **not** require an executor.

## Tool documentation templates

`Options.ToolDoc` (`arazzo.ToolDocTemplates`) fields are Go `text/template` **recipes** executed against `ToolDocContext`. They are not nested under `.ToolDoc`.

Write `{{.Title}}` for Arazzo `info.title`. Do **not** write `{{.ToolDoc.Title}}`.

| Field | Used on |
| --- | --- |
| `Name`, `Title` | both MCP `tools/list` and `GET /tools` |
| `Description` | MCP `tools/list` only (no REST URLs in the default) |
| `RESTDescription` | `GET /tools` only (no MCP wording in the default) |
| `QueryName`, `QueryTitle` | both, when `QueryMatcher` is set |
| `QueryDescription` | MCP `query` only |
| `RESTQueryDescription` | `GET /tools` `query` entry; default includes `{{.RESTQueryURL}}` |

Empty fields fall back to `DefaultToolDocTemplates()`.

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
| `APIRoot` | `PublicBaseURL` + `APIPrefix`, or `APIPrefix` alone (default `/api`) |
| `RESTQueryURL` | `{APIRoot}/plans/query` |
| `RESTExecuteLatestURL` | `{APIRoot}/plans/{PlanID}/{workflowId}` |
| `RESTExecuteVersionedURL` | `{APIRoot}/plans/{PlanID}/{VersionSegment}/{workflowId}` |
| `OpenAPILatestURL` | `{APIRoot}/openapi/{PlanID}` |
| `OpenAPIVersionedURL` | `{APIRoot}/openapi/{PlanID}/{VersionSegment}` |

Templates use `missingkey=zero`. Invalid template syntax fails `engine.New`.

## Sample binary

```bash
go run ./cmd/engine -addr localhost:8080 -specs testdata/arazzo/plans
```

Loads the sample Pet Store plans. Execute still needs an `Executor`; this binary does not set one. Use [examples/arazzo-fs](../../examples/arazzo-fs/README.md) for a stub, or [examples/petstore](../../examples/petstore/README.md) for live HTTP against local Docker Petstore 3 plus the async order adapter.

## Coding-agent checklist (Arazzo)

- Set `ArazzoLoaders`; otherwise plan routes and `run_*` tools do not exist.
- Point `FileLoader` at the plans dir only; OpenAPI sources stay beside it (`../sources/...`).
- Implement `Executor`; nil is 501 on execute, OpenAPI still works.
- Implement `QueryMatcher` to publish MCP `query` / `POST /plans/query`; nil omits both. Matcher uses your global registry; the engine rejects plans not loaded here.
- MCP args for `run_*` wrap `{workflowId, inputs}`; REST execute POST body **is** `inputs`.
- MCP `query` and `POST /api/plans/query` share `{query, data}` and the execute **outputs** object.
- Path version token is `v` + `info.version` (`v1.0.0`), not `1.0.0`.
- Generated OpenAPI `paths` keys omit `Options.APIPrefix`. They describe execute routes, not `/plans/query`. **200** is the workflow outputs object.
- `PublicBaseURL` must be set if you want absolute URLs in REST descriptions (and in custom MCP templates that use URL fields).

## Next

- [Examples](examples.md)
- Internals (contributors): [docs/contributors/arazzo.md](../contributors/arazzo.md)
