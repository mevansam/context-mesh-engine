# Arazzo plans (contributors)

SDK usage: [docs/users/arazzo.md](../users/arazzo.md). Change this document when catalog, runner, templates, or generated OpenAPI behavior changes.

## Layout

| Path | Role |
| --- | --- |
| `arazzo/loader.go` | `Loader`, `Source`, `Executor` aliases |
| `arazzo/fileloader.go` | Recursive `.yaml/.yml/.json`; `BaseURL` **must** end with `/` |
| `arazzo/tooldoc.go` | Recipes vs `ToolDocContext`; `SanitizeToolName` |
| `internal/plans/catalog.go` | Parse, skip, duplicate, `ResolveSources`, latest |
| `internal/plans/runner.go` | `NewEngine` per `Run`; `ResultJSON` |
| `internal/plans/schema.go` | MCP `oneOf` + `workflowId` const |
| `internal/plans/openapi.go` | OAS 3.1; paths without `APIPrefix` |
| `internal/plans/mcp.go` | Stub `query` + one `run_*` tool per catalog entry |
| `internal/api/v1/plans.go` | `POST /plans/query`, `POST /plans/...`, `GET /openapi/...`; 400/404/501 |
| `internal/api/v1/tools.go` | `GET /tools` (MCP `tools/list` result) |
| `engine/engine.go` | `New` wires loaders → catalog → MCP + REST |
| `testdata/arazzo/` | Fixtures |

`engine.New` loads the catalog only when `len(Options.ArazzoLoaders) > 0`. Failures abort construction (templates, parse, validate, resolve, duplicate plan key, duplicate tool name).

## Load pipeline (`catalog.addSource`)

1. `libopenapi.NewArazzoDocument`
2. Skip if `info` nil, `x-planId` empty, or `info.version` empty (warn via `slog`)
3. `ResolveSources` (attaches OpenAPI docs onto the Arazzo model)
4. `Validate`; **errors** fail load; **warnings** are allowed
5. Duplicate `(planId, version)` → error

`x-planId` is read from `info.Extensions` (`yaml.ScalarNode` only).

`ResolveConfig`:

- `BaseURL` = loader `Source.BaseURL`
- `FSRoots` = Arazzo file directory **and its parent** (so `../sources/openapi.yaml` stays in-root)
- `OpenAPIFactory` / `ArazzoFactory` parse bytes with libopenapi

`FileLoader` trailing slash on `BaseURL` is required. Without it, `../sources` resolves one directory too high (`testdata/sources` instead of `testdata/arazzo/sources`). Covered by `arazzo` FileLoader tests.

## Latest version

`pickLatest`: if every version is valid semver (`v1.2.3` or `1.2.3`), `semver.Compare`; else `sort.Strings` and take last. REST `{version}` lookup is `GetBySegment`: require a leading `v`, then strip it (`v1.0.0` → `1.0.0`). `v` alone or missing `v` → not found.

## Runner

`Runner.Run`:

1. Catalog `Get(planID, rawVersion)` — not found → `ErrNotFound`
2. Workflow id must exist on that entry — else `ErrNotFound`
3. Nil executor → `ErrNoExecutor` (`executor not configured`)
4. `libarazzo.NewEngine(doc, executor, sources)` then `RunWorkflow`
5. Map `WorkflowResult` → `ResultJSON` (durations as milliseconds)

Do **not** reuse `libopenapi/arazzo.Engine` across calls (documented not concurrency-safe). Cache `*high.Arazzo` and `[]*ResolvedSource` on `Entry` only.

HTTP (`plans.go`): `POST /plans/query` calls `Runner.Query` (stub → `ErrQueryNotImplemented` → 501). Execute: `ErrNoExecutor` → 501; `ErrNotFound` → 404; other runner errors → 400. Invalid JSON body → 400 `invalid json body`. Successful result with `success: false` is still 200.

MCP (`mcp.go`): `query` calls the same `Runner.Query`. Runner `error` on `query` or `run_*` becomes a tool error (`mcp.AddTool` wraps it). Nil error + `ResultJSON` → structured content.

POST body decoder allows unknown fields and empty body; cap 1 MiB. This is **not** `api.ReadJSON` (which rejects unknown fields).

## MCP tools

`RegisterMCP`:

1. Merge templates; add stub `query` (`queryArgs`: `query`, optional `data`) — same contract as `POST /plans/query`
2. For each catalog entry: `RenderToolDoc`, reject duplicate **names**, `InputSchema`, `mcp.AddTool` with captured `planID`/`version`

`InputSchema` is top-level `type: object` + `oneOf` of `{workflowId: const, inputs: workflow schema}`. Do not put overlapping workflow input schemas in a single `properties.inputs.oneOf` — JSON Schema `oneOf` fails when more than one branch matches.

Query name/title/description and `run_*` name/title/description are templates (`RenderQueryDoc` / `RenderToolDoc`). REST URLs use `Options.APIPrefix`.

## OpenAPI generator

`OpenAPIJSON(entry, latest bool)`:

- `latest == true` → paths `/plans/{planId}/{workflowId}`
- `latest == false` → `/plans/{planId}/{versionSegment}/{workflowId}`

No `APIPrefix` on paths (matches `StripPrefix` on the REST mux). `info.title` from Arazzo if set, else `planId`. `info.version` is the raw catalog version.

## Templates

`ToolDoc.Name` / `Title` / `Description` are `text/template` executed with `missingkey=zero`. `{{.Title}}` is Arazzo info.title, not the MCP title recipe.

`engine.New` renders run and query templates once with a dummy context **before** load so syntax errors fail fast even if the catalog is empty. Per-entry render still happens in `RegisterMCP`.

After render, `SanitizeToolName` keeps `[A-Za-z0-9_.-]` and truncates to 128. Empty name is an error.

## Invariants

1. Skip missing `x-planId` / version; duplicate `(planId, version)` is fatal.
2. Resolve sources before Validate.
3. New libopenapi Engine per run.
4. Nil executor: catalog + OpenAPI work; execute is 501 / MCP tool error.
5. Plan REST is under `Options.APIPrefix` only (default `/api`). Do not `StripPrefix` `/mcp`.
6. Templates are recipes; `Addr` is not a template field; `{workflowId}` in URLs is literal.
7. FileLoader root for tests is `testdata/arazzo/plans`, never the parent that contains `sources/openapi.yaml`.

## Tests to update when you change behavior

| File | Behavior |
| --- | --- |
| `arazzo/tooldoc_test.go` | FileLoader skip `ignore.txt`; BaseURL trailing `/`; default tool name/URLs; invalid template |
| `internal/plans/catalog_test.go` | skip `no-plan-id`; latest `1.1.0`; duplicate loaders; runner; schema oneOf length; OAS path keys |
| `engine/arazzo_test.go` | invalid templates fail `New`; OpenAPI without executor; REST 501; MCP `query` + `POST /plans/query`; `run_*` + REST share executor |

Fixtures: `testdata/arazzo/plans/petstore-v1.0.0.yaml`, `petstore-v1.1.0.yaml` (`echoName` only on 1.1.0), `no-plan-id.yaml`, `ignore.txt`; `testdata/arazzo/sources/openapi.yaml` (`operationId: getHealth`).

## Coding-agent checklist (contributors)

- Do not import `internal/plans` from public packages except `engine` (already does).
- Do not add a second `arazzo.Engine` cache.
- Keep `FSRoots` covering `../sources`.
- Keep OpenAPI paths unprefixed.
- Keep `oneOf` at the tool-args object with `workflowId` const.
- If you add a REST route, add it on the **v1** mux with a method pattern, and add an `engine` test.
- If you change skip/fail rules, update `docs/users/arazzo.md` in the same change.
