# Arazzo plans

Load [Arazzo](https://spec.openapis.org/arazzo/latest.html) workflow documents, expose **one MCP tool per plan version**, and execute the same workflows over REST. MCP and REST share one runner.

Public package: `github.com/mevansam/context-mesh-engine/arazzo`. Wire adapters through `engine.Options` — see [Adapters](adapters.md). Do **not** import `internal/plans`.

This surface is off until `engine.Options.ArazzoLoaders` is non-empty. Empty loaders: no `query` tool, no `run_*` tools, no plan execute routes or per-plan OpenAPI. `GET {APIPrefix}/tools` and `GET {APIPrefix}/openapi` (catalog index) are always registered; with loaders the catalog `$ref`s each plan’s latest spec, `GET /tools` includes each `run_*`, and `query` only when `QueryMatcher` is set.

REST paths below use the default prefix `/api`. Set `Options.APIPrefix` to change it. Generated OpenAPI `paths` omit that prefix either way.

## What `engine.New` registers

When `ArazzoLoaders` is set, `New` loads every document from every loader, then:

| Surface | Name / path | Condition |
| --- | --- | --- |
| MCP `query` | `query` | `QueryMatcher` is set |
| MCP `run_*` | one per catalog entry; default name `run_{{.SafePlanID}}_v{{.SafeVersion}}` | always (with loaders) |
| REST tools | `GET /api/tools` | always (MCP envelope; Arazzo descriptions use REST templates) |
| REST catalog OpenAPI | `GET /api/openapi` | always (index: `/tools` + `$ref` to each latest plan spec) |
| REST query | `POST /api/plans/query` | `QueryMatcher` is set |
| REST execute (latest) | `POST /api/plans/{planId}/{workflowId}` | always (with loaders) |
| REST execute (versioned) | `POST /api/plans/{planId}/{version}/{workflowId}` | always (with loaders) |
| OpenAPI (latest plan) | `GET /api/openapi/{planId}` | always (with loaders) |
| OpenAPI (versioned plan) | `GET /api/openapi/{planId}/{version}` | always (with loaders) |

`{version}` in URLs is **`v` + Arazzo `info.version`**. Example: `info.version: 1.0.0` → path token `v1.0.0`.

**Latest** among versions of the same `x-planId`: if every version is semver (with or without a leading `v`), pick the highest semver; otherwise lexicographic maximum (`golang.org/x/mod/semver` + `sort.Strings`).

Each execute call constructs a **new** libopenapi `arazzo.Engine`. That type is not concurrency-safe; the catalog caches parsed docs and resolved sources instead.

## Spec requirements

Each Arazzo file must have:

```yaml
info:
  title: ...
  version: 1.0.0          # required; skipped if empty; must be semver without a leading v
  x-planId: petstore      # required Info extension; skipped if missing
```

Skipped (log a warning, do not fail `New`):

- no `info`
- empty `x-planId`
- empty `info.version`
- non-`.yaml` / `.yml` / `.json` files (`FileLoader` never opens them, including `.rego`)

`New` fails when:

- the document cannot be parsed
- Arazzo structural validation fails (`libopenapi/arazzo.Validate`)
- source resolution fails (OpenAPI/Arazzo `sourceDescriptions`)
- two documents share the same `(x-planId, info.version)`
- `info.version` is not a semantic version, or starts with `v`
- two rendered MCP tool **names** collide
- `ToolDoc` templates fail to parse or execute

Fixtures: `testdata/arazzo/plans/` (`no-plan-id.yaml` is skipped; `ignore.txt` is ignored). Point `FileLoader` at **`plans/`**, not `testdata/arazzo/` (otherwise `sources/openapi.yaml` is parsed as Arazzo).

How to implement loaders: [Adapters — Loader](adapters.md#loader).

## Embed

```go
e, err := engine.New(engine.Options{
    Addr: "localhost:8080",
    ArazzoLoaders: []arazzo.Loader{
        arazzo.NewFileLoader("/path/to/plans"),
    },
    ArazzoExecutor: myExecutor{}, // nil: OpenAPI works; execute is 501
    QueryMatcher:   myMatcher{},  // nil: query tool and POST /plans/query are omitted
    PolicyLoader:   arazzo.NewFilePolicyLoader("/path/to/policies"), // nil: skip OPA
    PublicBaseURL:  "http://localhost:8080",
    DualMCPandREST: true,         // also mount /mcp; default is REST only
})
```

`PublicBaseURL` is the origin written into REST tool descriptions. It is not `Addr`. Empty → path-only URLs (`{APIPrefix}/plans/...`).

Implementations: [Adapters](adapters.md). Runnable stubs: [arazzo-fs](examples.md#arazzo-fs). Live HTTP: [petstore](examples.md#petstore).

## MCP and REST: `run_*`

Direct execute when the caller already knows the plan and version.

### MCP arguments

```json
{
  "workflowId": "pingHealth",
  "inputs": { "name": "demo" }
}
```

`inputSchema` is JSON Schema `type: object` with `oneOf` branches. Each branch uses `workflowId` **const** so overlapping workflow input schemas still validate uniquely:

```json
{
  "type": "object",
  "oneOf": [
    {
      "type": "object",
      "required": ["workflowId", "inputs"],
      "properties": {
        "workflowId": { "const": "pingHealth" },
        "inputs": { }
      }
    }
  ]
}
```

Successful `run_*` returns structured content that **is** the workflow outputs object (go-sdk `StructuredContent`). Same JSON as REST 200. Runner errors become MCP tool errors (`IsError: true`), not JSON-RPC protocol errors.

### REST execute

POST body is the workflow **inputs object** (no `workflowId` wrapper). `workflowId` is the path. Empty body is allowed (`{}` or no body).

```bash
curl -s -X POST http://localhost:8080/api/plans/petstore/pingHealth \
  -H 'Content-Type: application/json' \
  -d '{"name":"demo"}'

curl -s -X POST http://localhost:8080/api/plans/petstore/v1.0.0/pingHealth \
  -H 'Content-Type: application/json' \
  -d '{"name":"demo"}'
```

### Result JSON

The body is the workflow **outputs** map from the Arazzo document (not the engine trace). A workflow with no `outputs` returns `{}`.

```json
{
  "petId": 1
}
```

Generated OpenAPI `200` schemas use those output names as object properties.

| HTTP status | Public `error` | When |
| --- | --- | --- |
| 200 | (outputs body) | Workflow succeeded |
| 400 | `invalid json body` | Malformed JSON |
| 400 | `unexpected fields in inputs` | Extra keys, including `policyHints` / `secrets` |
| 400 | `query is required` | Empty `query` |
| 400 | `workflow failed` | Step/libopenapi failure (detail logged only) |
| 401 | `unauthorized` | Preprocessor rejected the call |
| 403 | `policy denied` | Inbound or outbound OPA deny (reason logged only) |
| 404 | `plan not found` | Unknown `planId`, version, or `workflowId` |
| 500 | `internal error` | Policy load/compile or host failure |
| 501 | `executor not configured` | `ArazzoExecutor` is nil |

Body on error: `{"error":"<message>"}`. The full internal error is logged, not returned.

## MCP and REST: `query`

Same job on both surfaces: the caller sends a simple natural-language question plus optional inputs. Your [`QueryMatcher`](adapters.md#querymatcher) selects a plan from a **global** registry. The engine then checks that plan is **loaded in this process** and executes it.

| Surface | How |
| --- | --- |
| MCP | tool `query` |
| REST | `POST /api/plans/query` |

```json
{ "query": "natural language", "data": { } }
```

`data` is the input outline (optional object). Used as workflow inputs when the matcher does not set `Inputs`.

| Matcher / catalog | REST | MCP |
| --- | --- | --- |
| `QueryMatcher` nil | route not registered (404) | tool not registered |
| Empty `query` | 400 | tool error |
| Matcher error | 400 | tool error |
| No selection (`nil` match) | 404 | tool error |
| Plan/version/workflow not loaded here | 404 | tool error |
| Match + execute OK | 200 outputs | structured content = outputs |

Direct `run_*` and `POST /api/plans/{planId}/...` do not use the matcher.

Override display strings with `ToolDoc.QueryName` (name only) and [`ToolHelpLookup`](adapters.md#toolhelplookup) (`Kind: query`) for title/description, or `ToolDoc.QueryTitle` / `QueryDescription` / `RESTQueryDescription` as global fallbacks.

## OpenAPI

`GET /api/openapi` returns the **catalog** OAS **3.1.0** (`Content-Type: application/json`). It always describes `GET /tools` (schema from go-sdk `mcp.ListToolsResult`). When plans are loaded, each latest execute path is a path-item `$ref` into that plan’s child spec (`{APIPrefix}/openapi/{planId}#/paths/...`). `POST /plans/query` is included only when `QueryMatcher` is set.

`GET /api/openapi/{planId}` and `GET /api/openapi/{planId}/{version}` return the **child** OAS **3.1.0** for that plan.

Paths **inside** those documents omit `Options.APIPrefix` (the REST mux is `StripPrefix`’d). `servers[].url` is `PublicBaseURL` + `APIPrefix` (or `/api` if the origin is empty) so Try-it-out clients hit the real execute URLs. With the default prefix:

- HTTP: `GET /api/openapi` → catalog index
- HTTP: `GET /api/openapi/petstore`
- Document path: `/plans/petstore/pingHealth` → real URL `POST /api/plans/petstore/pingHealth`

Latest child document: `/plans/{planId}/{workflowId}`. Versioned child document: `/plans/{planId}/v{version}/{workflowId}`. Request body schema is that workflow’s Arazzo `inputs` with reserved engine keys (`policyHints`, `secrets`, and dotted prefixes) omitted, then closed (`additionalProperties: false`). Extra caller fields, including reserved keys, are **400** `unexpected fields in inputs` on execute (REST, MCP `run_*`, and query after match). Workflow summary/description that name reserved keys are omitted too. MCP `run_*` `inputSchema` uses the same stripped, closed schema. Catalog `POST /plans/query` `data` stays an open object; `Run` still closes it against the selected workflow. **200** schema is an object with a property per Arazzo `outputs` name.

404 if the plan or version is missing. OpenAPI does **not** require an executor. Child documents describe execute routes, not `/plans/query` (that lives on the catalog index).

## Tool documentation

MCP and REST **share** tool names and titles. **Descriptions** are transport-specific so MCP `tools/list` does not mention REST URLs, and `GET /tools` does not mention MCP.

| Mechanism | When | Docs |
| --- | --- | --- |
| `Options.ToolDoc` | Global recipes for every plan/query tool | [ToolDocTemplates](adapters.md#tooldoctemplates) |
| `Options.ToolHelpLookup` | Per-plan / query templates at list time | [ToolHelpLookup](adapters.md#toolhelplookup) |

Help lookups run on `tools/list` / `GET /tools`, not at `New`. Lookup errors do not fail the list. Cache TTL defaults to 5 minutes; a negative duration always refreshes.

## OPA policy

Optional inbound/outbound [OPA](https://www.openpolicyagent.org/) modules run on every execute path (`run_*`, REST, `query`). Wire [`PolicyLoader`](adapters.md#policyloader); do not put `.rego` files on `ArazzoLoaders`.

- **Inbound** (`data.plan.inbound`) runs before the workflow. Allow may set `$inputs.policyHints` (nested object plus dotted keys for Arazzo `$inputs.policyHints.*`). Deny is **403** `policy denied` (the OPA `reason` is logged, not returned). The workflow does not run.
- **Outbound** (`data.plan.outbound`) runs after success. Deny is **403** `policy denied` and outputs are not returned. `redact` / `outputs` may reshape the response.

REST and MCP execute errors use the same public strings (`plan not found`, `unauthorized`, `policy denied`, `unexpected fields in inputs`, `workflow failed`, `internal error`). The full error is logged only.

A missing bundle for that `(planId, version)` skips both phases. Load/compile errors fail closed (**500** `internal error`) unless a compiled bundle is still cached.

## Sample binary

```bash
go run ./cmd/engine -addr localhost:8080 -specs testdata/arazzo/plans
```

Loads the sample Pet Store plans. Execute still needs an `Executor`; this binary does not set one. Use [arazzo-fs](examples.md#arazzo-fs) for a stub, or [petstore](examples.md#petstore) for live HTTP.

## Checklist

- Set `ArazzoLoaders`; otherwise plan execute routes and `run_*` tools do not exist. `GET /openapi` still describes `/tools`.
- Point `FileLoader` at the plans dir only; OpenAPI sources stay beside it (`../sources/...`).
- Optional [`PolicyLoader`](adapters.md#policyloader) for inbound/outbound OPA; keep `.rego` out of the Arazzo loader tree.
- Implement [`Executor`](adapters.md#executor); nil is 501 on execute, OpenAPI still works.
- Implement [`QueryMatcher`](adapters.md#querymatcher) to publish MCP `query` / `POST /plans/query`; nil omits both.
- MCP `run_*` args wrap `{workflowId, inputs}`; REST execute POST body **is** `inputs`.
- MCP `query` and `POST /api/plans/query` share `{query, data}` and the execute **outputs** object.
- Path version token is `v` + `info.version` (`v1.0.0`), not `1.0.0`.
- Generated OpenAPI `paths` keys omit `Options.APIPrefix`. Catalog plan paths `$ref` `{APIPrefix}/openapi/{planId}`. `servers` is `PublicBaseURL` + `APIPrefix`.
- Set `PublicBaseURL` if you want absolute URLs in REST descriptions.
- Optional [`ToolHelpLookup`](adapters.md#toolhelplookup) for per-plan/query title and description.

## Next

- [Adapters](adapters.md)
- [Examples](examples.md)
- Internals (contributors): [docs/contributors/arazzo.md](../contributors/arazzo.md)
