# Arazzo plans (contributors)

SDK usage: [docs/users/arazzo.md](../users/arazzo.md) (contracts), [docs/users/adapters.md](../users/adapters.md) (how to implement loaders, executors, matchers, policy, tool help). Change this document when catalog, runner, templates, policy, or generated OpenAPI behavior changes.

## Layout

| Path | Role |
| --- | --- |
| `arazzo/loader.go` | `Loader`, `Source`, `Executor` aliases |
| `arazzo/matcher.go` | `QueryMatcher`, `PlanCatalog`, `QueryMatch` |
| `arazzo/fileloader.go` | Recursive `.yaml/.yml/.json`; `BaseURL` **must** end with `/` |
| `arazzo/policy.go` | `PolicyLoader`, `PolicyBundle`, `PolicyHintsKey` |
| `arazzo/request.go` | `RequestPreprocessor`, `PolicyRequestContext`, `RequestSource` |
| `arazzo/secrets.go` | `SecretsProvider`, `MapSecrets`, `SecretInputs` flattening keys |
| `arazzo/filepolicy.go` | `{planId}/{version}/inbound.rego` + `outbound.rego` |
| `arazzo/tooldoc.go` | Recipes vs `ToolDocContext`; `SanitizeToolName` |
| `arazzo/toolhelp.go` | `ToolHelpLookup`; overlay; default lookup |
| `internal/plans/help.go` | TTL cache (`internal/ttlcache`); MCP `tools/list` middleware; REST overlay |
| `internal/ttlcache/` | Generic singleflight TTL cache |
| `internal/plans/catalog.go` | Parse, skip, duplicate, `ResolveSources`, latest, `View()` |
| `internal/plans/runner.go` | `NewEngine` per `Run`; inbound then workflow then outbound |
| `internal/plans/policy.go` | Compile `data.plan.inbound` / `data.plan.outbound`; cache |
| `internal/plans/redact.go` | RFC 6901 output redaction |
| `internal/plans/schema.go` | MCP `oneOf` + `workflowId` const |
| `internal/plans/openapi.go` | OAS 3.1 catalog index + per-plan specs; paths without `APIPrefix` |
| `internal/plans/mcp.go` | `query` + one `run_*` tool per catalog entry |
| `internal/api/v1/plans.go` | `GET /openapi`, `POST /plans/query`, `POST /plans/...`, `GET /openapi/{planId}`; 400/403/404/500/501 |
| `internal/api/v1/tools.go` | `GET /tools` (MCP `tools/list` envelope; REST descriptions for Arazzo tools) |
| `engine/engine.go` | `New` wires loaders → catalog → MCP + REST |
| `testdata/arazzo/` | Fixtures |

`engine.New` loads the catalog only when `len(Options.ArazzoLoaders) > 0`. Failures abort construction (templates, parse, validate, resolve, duplicate plan key, duplicate tool name).

## Load pipeline (`catalog.addSource`)

1. `libopenapi.NewArazzoDocument`
2. Skip if `info` nil, `x-planId` empty, or `info.version` empty (warn via `slog`)
3. `info.version` must be a semantic version **without** a leading `v` (`golang.org/x/mod/semver` on `"v"+version`). Invalid → error
4. `ResolveSources` (attaches OpenAPI docs onto the Arazzo model)
5. `Validate`; **errors** fail load; **warnings** are allowed
6. Duplicate `(planId, version)` → error

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
3. If `PolicyCache` is set, load/compile the bundle for `(planId, version)` (TTL cache, on demand). Load error without a cached module bundle → `ErrPolicyLoad`.
4. If inbound compiled: eval `data.plan.inbound`. Non-boolean/`false` `allow` → `ErrPolicyDenied`. On allow, copy inputs, drop caller `policyHints` and `policyHints.*` keys, set `$inputs.policyHints` from `hints` when present, and flatten leaves to dotted keys (`policyHints.petStatus`) so stock libopenapi `$inputs` lookup (single key, not nested walk) can resolve `$inputs.policyHints.petStatus`. `input.headers` / `input.auth` come from `PolicyRequestContext` (preprocessor), not from Rego `http.send`.
5. If `SecretsProvider` is set, strip caller `secrets` / `secrets.*`, then flatten `Options.SecretInputs` names onto `$inputs.secrets.<name>`.
6. Nil executor → `ErrNoExecutor` (`executor not configured`)
7. `libarazzo.NewEngine(doc, executor, sources)` then `RunWorkflow`
8. Return the workflow **outputs** map (`{}` if none). `success: false` becomes an error.
9. If outbound compiled: eval `data.plan.outbound` on `{inputs, outputs}`. Deny → `ErrPolicyDenied` (no outputs returned). Else replace `outputs` or apply `redact`/`mask`.

Do **not** reuse `libopenapi/arazzo.Engine` across calls (documented not concurrency-safe). Cache `*high.Arazzo` and `[]*ResolvedSource` on `Entry` only. Do **not** parse `.rego` in `FileLoader` / `catalog.addSource`.

`Runner.Query`:

1. Matcher nil → `ErrQueryNotImplemented` (defensive; the MCP tool and REST route are not registered)
2. Empty/whitespace `query` → `ErrEmptyQuery` (400)
3. `matcher.Match` with `QueryRequest{Query, Data, Catalog: catalog.View()}`
4. Nil match or empty `PlanID`/`WorkflowID` → `ErrNotFound`
5. Resolve version: empty → `Latest(PlanID)`; else `Get`. Miss → `ErrNotFound` (`… not loaded`)
6. Workflow id must exist on that entry → else `ErrNotFound`
7. Inputs: `match.Inputs` if non-nil, else `Data`
8. `Run` (same outputs as direct execute)

`Catalog.View()` is `arazzo.PlanCatalog`. It does not snapshot the catalog; `Get`/`Latest`/`Plans` copy metadata only when called. Matcher must not use a catalog miss as “no match”; the engine always verifies after `Match`.

HTTP (`plans.go`): `GET /openapi` (catalog) is always registered. `POST /plans/query` is registered only when `Runner.QueryEnabled()`. Execute and per-plan OpenAPI routes are registered when catalog is non-nil. `ErrNoExecutor` → 501; `ErrNotFound` → 404; `ErrPolicyDenied` → 403; `ErrPolicyLoad` → 500; other runner errors → 400. Invalid JSON body → 400 `invalid json body`. HTTP **200** body is the outputs object.

MCP (`mcp.go`): `query` is added only when `QueryEnabled()`. It calls the same `Runner.Query`. Runner `error` on `query` or `run_*` becomes a tool error. Nil error + outputs map → structured content.

POST body decoder allows unknown fields and empty body; cap 1 MiB. This is **not** `api.ReadJSON` (which rejects unknown fields).

## MCP tools

`RegisterMCP`:

1. Merge templates; if `QueryEnabled()`, add `query` (`queryArgs`: `query`, optional `data`) — same contract as `POST /plans/query`
2. For each catalog entry: render **name** from `ToolDoc`, reject duplicate **names**, `InputSchema`, `mcp.AddTool` with captured `planID`/`version`. Title/description placeholders use default templates. `HelpCache` records each tool for on-demand lookup.

`InputSchema` is top-level `type: object` + `oneOf` of `{workflowId: const, inputs: workflow schema}`. Do not put overlapping workflow input schemas in a single `properties.inputs.oneOf` — JSON Schema `oneOf` fails when more than one branch matches.

Query name and `run_*` name are `ToolDoc` recipes. Title and MCP/REST descriptions are filled on `tools/list` / `GET /tools` via `ToolHelpLookup` (cached). Empty `RESTDescription` from the registry uses `Description`. REST URLs in default templates use `Options.APIPrefix`.

## OpenAPI generator

Two layers. Both are OAS **3.1.0** JSON. Paths omit `APIPrefix` (REST mux is `StripPrefix`’d). Live URLs are `{APIPrefix}` + the document path.

### Catalog index — `CatalogOpenAPIJSON`

`GET /openapi` is **always** registered (`PlansController` with a nil catalog when there are no loaders).

| Path in the document | When | What it describes |
| --- | --- | --- |
| `GET /tools` | always | MCP `tools/list`. 200 schema is `components.schemas.ListToolsResult`, inferred at runtime from go-sdk `mcp.ListToolsResult` (`jsonschema.For`). Optional query `cursor` is `ListToolsParams.cursor`. |
| `POST /plans/query` | `QueryMatcher` set | MCP tool `query`. Body `{query, data}`; 200 is workflow outputs. |
| `POST /plans/{planId}/{workflowId}` | latest entry for that `planId` | Path-item **`$ref`** to the child spec: `./{planId}#/paths/~1plans~1{planId}~1{workflowId}` |

`$ref` is relative to the catalog document URL. With default prefix, `GET /api/openapi` plus `./petstore#/paths/~1plans~1petstore~1pingHealth` resolves to `GET /api/openapi/petstore` (the latest child spec). Versioned child specs (`GET /openapi/{planId}/v{version}`) are **not** inlined in the index.

Do not copy backend OpenAPI (`sourceDescriptions`) into these documents. Those specs are for libopenapi step execution only.

### Per-plan child — `OpenAPIJSON(entry, latest bool)`

- `latest == true` → paths `/plans/{planId}/{workflowId}`
- `latest == false` → `/plans/{planId}/{versionSegment}/{workflowId}`

`info.title` from Arazzo if set, else `planId`. `info.version` is the raw catalog version. Request body schema is the workflow `inputs` JSON Schema. **200** schema is an object whose `properties` are the Arazzo `outputs` names (expression values are not types).

### REST resources vs MCP tools

MCP granularity is **one tool per plan version** (`run_*`) plus optional `query`. REST granularity is **one URL per workflow** (and one list URL, one query URL). Same `Runner`.

| REST (after `APIPrefix` strip) | HTTP | MCP | Body / args |
| --- | --- | --- | --- |
| `GET /tools` | list | JSON-RPC `tools/list` | Optional `?cursor=` = `ListToolsParams.cursor`. Envelope is `ListToolsResult` (`ttlMs`, `cacheScope`, `tools`). Arazzo `description` on REST is the REST template; MCP list keeps MCP text. |
| `POST /plans/query` | match + execute | tool `query` | `{ "query", "data" }` both sides. 200 / structured content = workflow **outputs**. Route and tool omitted unless `QueryMatcher` is set. |
| `POST /plans/{planId}/{workflowId}` | execute **latest** | `run_{plan}_v{latest}` with that `workflowId` | REST body **is** `inputs`. MCP args are `{ "workflowId", "inputs" }`. |
| `POST /plans/{planId}/v{version}/{workflowId}` | execute that version | `run_{plan}_v{version}` | Same body split as latest. |
| `GET /openapi` | catalog OAS | (none) | Index: `/tools` + `$ref`s to latest child specs. Always registered. |
| `GET /openapi/{planId}` | latest child OAS | (none) | Describes latest execute paths for that plan. |
| `GET /openapi/{planId}/v{version}` | versioned child OAS | (none) | Describes that version’s execute paths. |

`run_*` `inputSchema` is JSON Schema `oneOf` with `workflowId` **const** per workflow. Generated plan OpenAPI does **not** use that wrapper: `operationId` is the `workflowId`, and the request schema is that workflow’s `inputs` only.

`GET /tools` is implemented by opening an in-memory MCP session against the shared `mcp.Server` (`internal/api/v1/tools.go`), then overlaying REST descriptions. Catalog OpenAPI **documents** that route; it does not call `ListTools` when generating the spec.

## Templates

`ToolDoc.Name` / `Title` / `Description` / `RESTDescription` are `text/template` executed with `missingkey=zero`. `{{.Title}}` is Arazzo info.title, not the MCP title recipe. MCP `tools/list` middleware and `GET /tools` overlay clone `*mcp.Tool` so the MCP registry is not mutated.

`engine.New` renders run templates (and query templates when `QueryMatcher` is set) once with a dummy context **before** load so syntax errors fail fast even if the catalog is empty. Per-entry **name** render still happens in `RegisterMCP`. Registry `Lookup` is not called at `New`.

After render, `SanitizeToolName` keeps `[A-Za-z0-9_.-]` and truncates to 128. Empty name is an error.

## Invariants

1. Skip missing `x-planId` / version; duplicate `(planId, version)` is fatal.
2. Resolve sources before Validate.
3. New libopenapi Engine per run.
4. Nil executor: catalog + OpenAPI work; execute is 501 / MCP tool error.
5. Nil `QueryMatcher`: do not register MCP `query` or `POST /plans/query`; omit `/plans/query` from the catalog OAS; after Match, missing loaded plan is 404.
6. Nil `PolicyLoader`: skip inbound/outbound. Policy is on-demand, not at `New`. Fail closed on load/compile errors.
7. Plan REST is under `Options.APIPrefix` only (default `/api`). Do not `StripPrefix` `/mcp`.
8. Templates are recipes; `Addr` is not a template field; `{workflowId}` in URLs is literal.
9. FileLoader root for tests is `testdata/arazzo/plans`, never the parent that contains `sources/openapi.yaml`.
10. Help `Lookup` is on `tools/list` / `GET /tools` only. Lookup errors must not fail the list.
11. `GET /openapi` is always registered. Per-plan `GET /openapi/{planId}` and execute `POST /plans/...` are registered only when loaders produced a catalog.
12. Keep OpenAPI paths unprefixed. Catalog plan paths must `$ref` `./{planId}#/paths/...`, not inline child operations.

## Tests to update when you change behavior

| File | Behavior |
| --- | --- |
| `arazzo/tooldoc_test.go` | FileLoader skip `ignore.txt`; BaseURL trailing `/`; MCP vs REST default descriptions; invalid/empty-name templates; MergeTemplates |
| `arazzo/toolhelp_test.go` | Default lookup; REST falls back to Description; distinct RESTDescription; query overlay |
| `internal/plans/help_test.go` | Cache TTL / always-refresh; stale-on-error; singleflight; REST surface; middleware skips non-list |
| `internal/ttlcache/cache_test.go` | Generic TTL / stale-on-error / singleflight |
| `internal/plans/policy_test.go` | Inbound deny skips executor; outbound deny hides outputs; redact/replace; fail closed; `input.auth` / flatten hints |
| `internal/plans/redact_test.go` | JSON Pointer mask, missing skip, malformed deny |
| `arazzo/filepolicy_test.go` | inbound/outbound/data overlay; missing nil; unsafe segments |
| `internal/plans/mcp_test.go` | RegisterMCP run/query tools; duplicate names; invalid templates |
| `internal/plans/catalog_test.go` | skip `no-plan-id`; reject `v`-prefixed / non-semver version; latest `1.1.0`; duplicate loaders; runner; schema oneOf length; OAS path keys; catalog `$ref` + `ListToolsResult` |
| `engine/arazzo_test.go` | invalid templates fail `New`; OpenAPI without executor; catalog `GET /openapi`; REST 501; REST 403 policy deny; MCP `query` + `POST /plans/query`; `run_*` + REST share executor; on-demand `ToolHelpLookup`; lookup errors use defaults |
| `engine/engine_test.go` | `GET /openapi` without loaders still describes `/tools` |

Fixtures live under `testdata/arazzo/`. `FileLoader` must be pointed at **`plans/`**, not `testdata/arazzo/` (otherwise `sources/openapi.yaml` is parsed as Arazzo and fails). Latest petstore version in tests is `1.1.0`.

```text
testdata/arazzo/
  plans/
    petstore-v1.0.0.yaml   x-planId: petstore, version 1.0.0, workflow pingHealth
    petstore-v1.1.0.yaml   same planId, version 1.1.0, pingHealth + echoName
    no-plan-id.yaml        skipped (no x-planId)
    ignore.txt             skipped (not yaml/json)
  sources/
    openapi.yaml           OpenAPI 3; operationId getHealth
                           referenced from plans as ../sources/openapi.yaml
```

## Coding-agent checklist (contributors)

- Do not import `internal/plans` from public packages except `engine` (already does).
- Do not add a second `arazzo.Engine` cache.
- Keep `FSRoots` covering `../sources`.
- Keep OpenAPI paths unprefixed. Catalog index `$ref`s child specs; do not inline plan operations in `CatalogOpenAPIJSON`.
- Keep `oneOf` at the tool-args object with `workflowId` const.
- If you add a REST route, add it on the **v1** mux with a method pattern, and add an `engine` test.
- If you change skip/fail rules, update `docs/users/arazzo.md` in the same change.
- If you change public adapter interfaces, update `docs/users/adapters.md`.
