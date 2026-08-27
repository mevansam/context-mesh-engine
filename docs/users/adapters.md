# Adapters

The engine is a host. You supply the pieces that talk to **your** catalogs, backends, and documentation systems. Wire each adapter through [`engine.Options`](configuration.md#options-reference).

| Adapter | `Options` field | Required? | Role |
| --- | --- | --- | --- |
| [`Loader`](#loader) | `ArazzoLoaders` | For plans | Produce Arazzo document bytes |
| [`Executor`](#executor) | `ArazzoExecutor` | For execute | Perform one backend HTTP call per workflow step |
| [`QueryMatcher`](#querymatcher) | `QueryMatcher` | For `query` | Select a plan from a (usually global) registry |
| [`PolicyLoader`](#policyloader) | `PolicyLoader` | No | Optional OPA inbound/outbound modules per plan version |
| [`RequestPreprocessor`](#requestpreprocessor) | `RequestPreprocessor` | No | Headers + extra JWTs → OPA `input.headers` / `input.auth` |
| [`SecretsProvider`](#secretsprovider) | `SecretsProvider` | No | Named secrets for Executor JWT minting and optional `$inputs.secrets.*` |
| [`ToolHelpLookup`](#toolhelplookup) | `ToolHelpLookup` | No | Per-plan / query title and description templates at list time |
| [`ToolDocTemplates`](#tooldoctemplates) | `ToolDoc` | No | Global name/title/description recipes |
| [`Controller`](#rest-controllers) | `AddController` | No | Extra REST routes under `APIPrefix` |

Nil `ArazzoLoaders`: no plan tools or plan REST. Nil `ArazzoExecutor`: catalog and OpenAPI still work; execute is **501**. Nil `QueryMatcher`: MCP `query` and `POST /plans/query` are **not** registered. Nil `PolicyLoader`: skip inbound/outbound checks. Nil `ToolHelpLookup`: built-in / `ToolDoc` recipes.

Do not import `internal/`. All types below live in `github.com/mevansam/context-mesh-engine/arazzo` or `.../api`.

---

## Loader

A loader returns raw Arazzo documents. `engine.New` parses, validates, and indexes them.

```go
type Loader interface {
    Load(ctx context.Context) ([]Source, error)
}

type Source struct {
    URI     string // locator for error messages (usually a filesystem path)
    Data    []byte // raw Arazzo YAML or JSON
    BaseURL string // resolve relative sourceDescriptions URLs
}
```

### Built-in filesystem loader

```go
arazzo.NewFileLoader("/path/to/plans")
```

Walks **recursively** for `.yaml`, `.yml`, and `.json`. Point it at the **plans directory**, not a tree that also contains OpenAPI files you do not want parsed as Arazzo. It does **not** load `.rego` files; use [`PolicyLoader`](#policyloader).

`Source.BaseURL` is `file://` + the Arazzo file’s directory **with a trailing slash**. Relative `sourceDescriptions[].url` values resolve against that directory (`../sources/openapi.yaml` is the intended layout). Without the trailing slash, `../` climbs one extra level (RFC 3986).

### Custom loader

Implement `Loader` for HTTP, embed, object storage, or git. Honor `ctx`. Set `BaseURL` so OpenAPI/Arazzo sources beside the spec still resolve.

```go
type bytesLoader struct {
    uri, baseURL string
    data         []byte
}

func (l bytesLoader) Load(context.Context) ([]arazzo.Source, error) {
    return []arazzo.Source{{
        URI:     l.uri,
        Data:    l.data,
        BaseURL: l.baseURL, // e.g. "file:///app/plans/"
    }}, nil
}
```

You may pass several loaders. Duplicate `(x-planId, info.version)` across them fails `New`. Spec skip/fail rules: [Arazzo plans](arazzo.md#spec-requirements).

---

## Executor

The engine does not call domain APIs itself. Each workflow step becomes one `Execute` call. The type is an alias of `libopenapi/arazzo.Executor`.

```go
type Executor interface {
    Execute(ctx context.Context, req *ExecutionRequest) (*ExecutionResponse, error)
}
```

| Type | Fields you typically use |
| --- | --- |
| `ExecutionRequest` | `OperationID`, `OperationPath`, `Method`, `Parameters`, `RequestBody`, `ContentType`, `Source` |
| `ExecutionResponse` | `StatusCode` (required for Arazzo success criteria), `Headers`, `Body`, `URL`, `Method` |

`StatusCode` is what `$statusCode == 200` (and similar) is evaluated against. Return a response, not a Go error, for HTTP 4xx/5xx that the plan should observe. Return a Go error for transport failures you do not want the workflow engine to treat as a status code.

### Stub (tests and samples)

```go
type stubExec struct{}

func (stubExec) Execute(_ context.Context, _ *arazzo.ExecutionRequest) (*arazzo.ExecutionResponse, error) {
    return &arazzo.ExecutionResponse{
        StatusCode: 200,
        Body:       map[string]any{"ok": true},
    }, nil
}
```

[arazzo-fs](examples.md#arazzo-fs) uses this pattern.

### HTTP client (production)

Dispatch on `OperationPath` / `OperationID` / `Source`, then call the real origin. [petstore](examples.md#petstore) implements a real client against Petstore 3 and an async order adapter.

Sketch:

```go
type httpExec struct {
    client *http.Client
    base   string
}

func (e httpExec) Execute(ctx context.Context, req *arazzo.ExecutionRequest) (*arazzo.ExecutionResponse, error) {
    // Resolve Method + URL from req.OperationID / req.OperationPath / req.Parameters.
    httpReq, err := http.NewRequestWithContext(ctx, req.Method, urlFor(e.base, req), bodyReader(req.RequestBody))
    if err != nil {
        return nil, err
    }
    if req.ContentType != "" {
        httpReq.Header.Set("Content-Type", req.ContentType)
    }
    resp, err := e.client.Do(httpReq)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    body, err := decodeJSON(resp.Body)
    if err != nil {
        return nil, err
    }
    return &arazzo.ExecutionResponse{
        StatusCode: resp.StatusCode,
        Body:       body,
        URL:        httpReq.URL.String(),
        Method:     httpReq.Method,
    }, nil
}
```

Exact `ExecutionRequest` fields (`OperationPath` vs `OperationID`, parameter placement) follow libopenapi’s Arazzo engine. `urlFor`, `bodyReader`, and `decodeJSON` are yours to implement. Inspect `req` in a stub if you are unsure what a given plan emits.

### Nil executor

| Surface | Behavior |
| --- | --- |
| `GET /tools`, `GET /openapi/...` | Work |
| `POST /plans/{planId}/...` | **501** `{"error":"executor not configured"}` |
| MCP `run_*` | Tool error (`IsError: true`), not a JSON-RPC protocol error |
| `query` | Not registered without a matcher; with a matcher, same 501/tool error on execute |

This module does not ship an HTTP client executor.

---

## QueryMatcher

Semantic matching is **not** part of this SDK. Implement `QueryMatcher` in the application (vector search, a rules engine, a hosted registry). The engine only calls `Match`, then checks that the selected plan is **loaded in this process** and runs it.

```go
type QueryMatcher interface {
    Match(ctx context.Context, req QueryRequest) (*QueryMatch, error)
}

type QueryRequest struct {
    Query   string
    Data    map[string]any
    Catalog PlanCatalog // plans loaded in this engine; not a copied slice
}

type QueryMatch struct {
    PlanID     string         // required (x-planId)
    Version    string         // empty → latest version loaded *here*
    WorkflowID string         // required
    Inputs     map[string]any // nil → use QueryRequest.Data
}

type PlanCatalog interface {
    Get(planID, version string) (PlanSummary, bool)
    Latest(planID string) (PlanSummary, bool)
    Plans() iter.Seq[PlanSummary]
}
```

### Contract

1. Return whatever the **global** registry selected. Do **not** treat `Catalog.Get == false` as “no match”. A global hit that was never loaded here is HTTP **404** / MCP tool error (`… not loaded`).
2. Empty `Version` means latest among versions **loaded in this process**, not latest in the global registry.
3. `nil` match, empty `PlanID`, or empty `WorkflowID` → 404 / tool error.
4. Matcher error → 400 / tool error.
5. Empty `query` string is rejected by the engine before `Match` (**400** / tool error).

`Catalog` is optional: use it to list loaded workflows after an id match, or ignore it. `Plans()` is an iterator; nothing is copied until you range it.

### Dummy matcher (samples)

Always select a known workflow. The engine still verifies the plan is loaded.

```go
type pingMatcher struct{}

func (pingMatcher) Match(_ context.Context, req arazzo.QueryRequest) (*arazzo.QueryMatch, error) {
    return &arazzo.QueryMatch{
        PlanID:     "petstore",
        WorkflowID: "pingHealth",
        Inputs:     req.Data,
    }, nil
}
```

[arazzo-fs](examples.md#arazzo-fs) ships this. [petstore](examples.md#petstore) leaves `QueryMatcher` nil, so `query` is unpublished.

### Request / response

Same job on MCP tool `query` and `POST {APIPrefix}/plans/query`:

```json
{ "query": "natural language", "data": { } }
```

Success payload is the workflow **outputs** object (same as direct execute). Direct `run_*` and `POST /plans/{planId}/...` do **not** use the matcher.

---

## ToolDocTemplates

`Options.ToolDoc` fields are Go `text/template` **recipes** executed against [`ToolDocContext`](#tooldoccontext). They are not nested under `.ToolDoc`.

Write `{{.Title}}` for Arazzo `info.title`. Do **not** write `{{.ToolDoc.Title}}`.

| Field | Used on |
| --- | --- |
| `Name` | MCP `tools/list` and `GET /tools` (not looked up) |
| `Title` | Both, unless `ToolHelpLookup` returns `Title` |
| `Description` | MCP `tools/list` only (default has no REST URLs) |
| `RESTDescription` | `GET /tools` only (default has no MCP wording) |
| `QueryName` | Both, when `QueryMatcher` is set (not looked up) |
| `QueryTitle` | Both, unless lookup `Kind: query` returns `Title` |
| `QueryDescription` | MCP `query` only |
| `RESTQueryDescription` | `GET /tools` `query` entry; default includes `{{.RESTQueryURL}}` |

Empty fields fall back to `arazzo.DefaultToolDocTemplates()`. Default name recipe: `run_{{.SafePlanID}}_v{{.SafeVersion}}`. After render, the name is sanitized to MCP runes `[A-Za-z0-9_.-]` (other runes → `_`) and truncated to 128 characters.

`{workflowId}` in default REST URLs is a **literal placeholder** for the caller. `Addr` is not a template variable. Templates use `missingkey=zero`. Invalid syntax fails `engine.New`.

Override globally:

```go
e, err := engine.New(engine.Options{
    ArazzoLoaders: loaders,
    ToolDoc: arazzo.ToolDocTemplates{
        Title: `{{.PlanID}} {{.Version}}`,
    },
})
```

Per-plan strings belong in [`ToolHelpLookup`](#toolhelplookup), not in `ToolDoc`.

---

## ToolHelpLookup

Optional registry of **per-plan** and **query** title/description templates. Lookups run on MCP `tools/list` and `GET /tools`, **not** in `engine.New`.

```go
type ToolHelpLookup interface {
    Lookup(ctx context.Context, req ToolHelpRequest) (*ToolHelp, error)
}

type ToolHelpRequest struct {
    Kind    ToolHelpKind // ToolHelpKindPlan or ToolHelpKindQuery
    PlanID  string       // set when Kind is plan
    Version string
}

type ToolHelp struct {
    Title           string // text/template
    Description     string // MCP tools/list
    RESTDescription string // GET /tools; empty → reuse Description
}
```

### Behavior

- `nil` `*ToolHelp` or a zero `ToolHelp` → `ToolDoc` / built-in defaults.
- Empty `RESTDescription` → use `Description` (MCP and REST share that template). If both are empty → `ToolDoc` / built-in REST default.
- Lookup **error**: last cached templates if any, otherwise defaults. The list request does **not** fail.
- Successful results are cached for `Options.ToolHelpCacheTTL` (default **5m**). Zero in `Options` means that default. A **negative** duration disables caching.
- Names stay on `ToolDoc.Name` / `QueryName`.

### In-memory registry

```go
type helpRegistry struct {
    plans map[string]arazzo.ToolHelp // key: planId + "\x00" + version
    query arazzo.ToolHelp
}

func (h helpRegistry) Lookup(_ context.Context, req arazzo.ToolHelpRequest) (*arazzo.ToolHelp, error) {
    switch req.Kind {
    case arazzo.ToolHelpKindQuery:
        if h.query == (arazzo.ToolHelp{}) {
            return nil, nil
        }
        help := h.query
        return &help, nil
    case arazzo.ToolHelpKindPlan:
        help, ok := h.plans[req.PlanID+"\x00"+req.Version]
        if !ok {
            return nil, nil
        }
        return &help, nil
    default:
        return nil, nil
    }
}

func main() {
    e, err := engine.New(engine.Options{
        ArazzoLoaders: loaders,
        ToolHelpLookup: helpRegistry{
            plans: map[string]arazzo.ToolHelp{
                "petstore\x001.1.0": {
                    Title:       "Pet store {{.Version}}",
                    Description: "Run workflows on {{.PlanID}}.",
                },
            },
        },
        ToolHelpCacheTTL: 5 * time.Minute,
    })
    // ...
}
```

Replace the map with HTTP, SQL, or an agent-memory store. Keep `Lookup` cheap: list requests call it once per distinct `(planId, version)` plus once for `query` when that tool exists. The engine single-flights concurrent lookups for the same key.

`DefaultToolHelpLookup()` returns empty help so `OverlayToolHelp` uses `DefaultToolDocTemplates()`.

---

## PolicyLoader

Optional OPA modules for a `(planId, version)`. Loaded **on execute** (MCP `run_*`, REST, `query`), not in `engine.New`. A bundle may include inbound, outbound, or both.

```go
type PolicyLoader interface {
    Load(ctx context.Context, req PolicyRequest) (*PolicyBundle, error)
}

type PolicyRequest struct {
    PlanID  string
    Version string
}

type PolicyBundle struct {
    Inbound  []byte // package plan.inbound; empty = no inbound
    Outbound []byte // package plan.outbound; empty = no outbound
    Data     []byte // optional JSON object for OPA document data
}
```

Nil `*PolicyBundle` → skip both phases for that key. Nil `Options.PolicyLoader` → skip policy for every plan.

Do **not** return `.rego` files from [`Loader`](#loader). Keep Arazzo specs and policy modules in separate trees.

### Built-in filesystem loader

```go
&arazzo.FilePolicyLoader{
    Dir:  "/etc/policies",
    Data: map[string]any{"petstoreBase": "http://localhost:8090/api/v3"},
}
```

Layout: `{Dir}/{planId}/{version}/inbound.rego` and/or `outbound.rego`, optional `data.json`. `Data` overlay keys win over `data.json`. `planId` and `version` must be single path segments (no `..` or slashes). Missing directory or modules → nil bundle, not an error.

Successful compiles are cached for `Options.PolicyCacheTTL` (default **5m**). Zero in `Options` means that default. A **negative** duration disables caching. Load/compile errors fail the request (**500**) unless a previously compiled bundle with modules is still in cache. Deny is **403** and does not return workflow outputs.

### Inbound and outbound

Query `data.plan.inbound` before the workflow runs, and `data.plan.outbound` after success. Decisions are objects:

```json
{ "allow": true, "hints": { "petStatus": "available" } }
```

```json
{ "allow": true, "redact": ["/pet/photoUrls"], "mask": "***" }
```

- Default **deny**: missing or non-boolean `allow` is deny. Use `default allow := false` in Rego.
- On inbound allow, `hints` (if present) is written to workflow input `$inputs.policyHints` (nested object). Leaves are also copied as dotted keys (`policyHints.petStatus`) because Arazzo `$inputs.a.b` is a single input name in libopenapi, not a nested path. Caller-supplied `policyHints` and `policyHints.*` keys are discarded.
- If [`RequestPreprocessor`](#requestpreprocessor) ran, OPA also receives `input.headers` (allowlisted) and `input.auth` (client + end-user claims). These are not workflow `$inputs`.
- If there is no inbound module, `policyHints` is not injected.
- Outbound deny → **403**; outputs are not returned (the workflow has already run).
- Outbound `outputs` object, if present, **replaces** workflow outputs and ignores `redact`.
- Otherwise `redact` is RFC 6901 JSON Pointers into outputs. Missing pointers are skipped; malformed pointers deny. Default `mask` is JSON `null`.

---

## RequestPreprocessor

Optional. Runs on REST execute/`query` and MCP `run_*`/`query` **before** inbound OPA. Verify extra JWTs (`x-*` headers), call a remote user-info service, and return a JSON-friendly `Auth` object plus allowlisted `Headers`.

```go
type RequestPreprocessor interface {
    Process(ctx context.Context, src RequestSource) (*PolicyRequestContext, error)
}
```

`RequestSource.ClientAuth` is filled when `auth.RequireBearerToken` already verified the calling-application bearer token. Error from `Process` is **401**.

Do not put `Authorization` or raw user JWTs in `Headers`. Petstore: [`mcp-server/auth.go`](../../examples/petstore/mcp-server/auth.go).

---

## SecretsProvider

```go
type SecretsProvider interface {
    Get(ctx context.Context, name string) (string, error)
}
```

`arazzo.MapSecrets` is an in-memory map. The host `Executor` uses this to mint a **downstream** JWT for domain APIs (point 6). Names in `Options.SecretInputs` are flattened onto `$inputs.secrets.<name>` for Arazzo expressions. Caller-supplied `secrets` keys are stripped. Keep HMAC signing keys out of `SecretInputs` unless the plan must see them.

---

## ToolDocContext

Data bag passed to `ToolDoc` and help templates.

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

For `Kind: query`, context is a placeholder (`plan` / `1.0.0`); URL fields still reflect `PublicBaseURL` and `APIPrefix`.

---

## REST controllers

```go
type Controller interface {
    Register(mux *http.ServeMux)
}

func WriteJSON(w http.ResponseWriter, status int, v any)
func WriteError(w http.ResponseWriter, status int, msg string)
func ReadJSON(w http.ResponseWriter, r *http.Request, v any) error
```

`ReadJSON` rejects unknown JSON fields and caps the body at **1 MiB**. The `ResponseWriter` is passed to `http.MaxBytesReader` so an oversized body can close the connection. Plan execute POST does **not** use `ReadJSON` (unknown fields allowed; same 1 MiB cap).

The mux passed to `Register` is already stripped of `Options.APIPrefix`. Pattern `GET /items` is `GET {APIPrefix}/items`.

```go
type ItemsController struct{}

func (c *ItemsController) Register(mux *http.ServeMux) {
    mux.HandleFunc("GET /items", c.list)
    mux.HandleFunc("POST /items", c.create)
}

func (c *ItemsController) list(w http.ResponseWriter, r *http.Request) {
    api.WriteJSON(w, http.StatusOK, map[string]any{"items": []string{}})
}

func (c *ItemsController) create(w http.ResponseWriter, r *http.Request) {
    var body map[string]any
    if err := api.ReadJSON(w, r, &body); err != nil {
        api.WriteError(w, http.StatusBadRequest, err.Error())
        return
    }
    api.WriteJSON(w, http.StatusCreated, body)
}

e.AddController(&ItemsController{})
```

Use method-aware patterns (`GET /items`). The root mux is stdlib `ServeMux` (Go 1.22+).

---

## Wiring example

```go
e, err := engine.New(engine.Options{
    Addr:           "localhost:8080",
    DualMCPandREST: true,
    PublicBaseURL:  "http://localhost:8080",
    ArazzoLoaders: []arazzo.Loader{
        arazzo.NewFileLoader("/etc/plans"),
    },
    ArazzoExecutor:   httpExec{client: http.DefaultClient, base: "https://api.example"},
    QueryMatcher:     registryMatcher{/* vector index */},
    ToolHelpLookup:   helpRegistry{/* CMS or DB */},
    ToolHelpCacheTTL: 5 * time.Minute,
    PolicyLoader:     arazzo.NewFilePolicyLoader("/etc/policies"),
    PolicyCacheTTL:   5 * time.Minute,
})
if err != nil {
    log.Fatal(err)
}
e.AddController(&ItemsController{})
log.Fatal(e.ListenAndServe(ctx))
```

## Next

- [Arazzo plans](arazzo.md) — what `New` registers, spec rules, MCP/REST payloads, OpenAPI.
- [Examples](examples.md) — stub vs live adapters.
- [Configuration](configuration.md) — every `Options` field.
