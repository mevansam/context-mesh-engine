# Petstore demo: Arazzo plans on context-mesh-engine

This directory is the end-to-end reference for hosting [Arazzo](https://spec.openapis.org/arazzo/latest.html) workflows with [`context-mesh-engine`](../../docs/users/getting-started.md). It shows how a Go process becomes an MCP/REST plan host: you load documents, implement one HTTP `Executor`, optionally attach OPA, and the engine registers execute tools and REST routes.

Copy this example when you have real backends (OpenAPI and otherwise), not when you only need a stub. For load-from-disk without backends, use [arazzo-fs](../arazzo-fs/README.md). For engine-only embed, use [minimal](../minimal/README.md).

Default Petstore target is **local Docker**. Pass `-petstore hosted` for [petstore3.swagger.io](https://petstore3.swagger.io/).

## Table of contents

- [What this demo is](#what-this-demo-is)
- [Architecture](#architecture)
- [Identity and tokens](#identity-and-tokens)
- [Quick start](#quick-start)
- [The Arazzo plan](#the-arazzo-plan)
  - [Catalog identity and sources](#catalog-identity-and-sources)
  - [Workflow: retrievePet](#workflow-retrievepet)
  - [Workflow: purchasePet](#workflow-purchasepet)
  - [Workflow: checkOrderStatus](#workflow-checkorderstatus)
  - [Runtime expressions](#runtime-expressions)
- [OPA policies](#opa-policies)
  - [Layout and wiring](#layout-and-wiring)
  - [When policy runs](#when-policy-runs)
  - [Inbound (`inbound.rego`)](#inbound-inboundrego)
  - [Outbound (`outbound.rego`)](#outbound-outboundrego)
  - [How `policyHints` reach Arazzo](#how-policyhints-reach-arazzo)
- [Building services on context-mesh-engine](#building-services-on-context-mesh-engine)
  - [What the engine owns](#what-the-engine-owns)
  - [Host process: `engine.New`](#host-process-enginenew)
  - [Loader](#loader)
  - [Executor](#executor)
  - [PolicyLoader](#policyloader)
  - [OAuth and the engine SDK](#oauth-and-the-engine-sdk)
    - [Demo verification vs production](#demo-verification-vs-production)
  - [HTTP surfaces](#http-surfaces)
  - [AsyncAPI as OpenAPI HTTP](#asyncapi-as-openapi-http)
  - [Checklist for your own host](#checklist-for-your-own-host)
- [Operate the demo](#operate-the-demo)
  - [Petstore 3 in Docker](#petstore-3-in-docker)
  - [Hosted Petstore 3](#hosted-petstore-3)
  - [Run the Go servers](#run-the-go-servers)
  - [Seed users](#seed-users)
  - [Swagger UI](#swagger-ui)
  - [REST: retrieve, purchase, check order](#rest-retrieve-purchase-check-order)
  - [MCP](#mcp)
- [Further reading](#further-reading)

## What this demo is

Four processes cooperate. Only **`mcp-server`** is a `context-mesh-engine` host.

| Directory | Process | Role |
| --- | --- | --- |
| [`petstore-openapi-server/`](petstore-openapi-server/) | `localhost:8090` | Official [Petstore 3](https://github.com/swagger-api/swagger-petstore) in Docker. OpenAPI base `http://localhost:8090/api/v3`. |
| [`petstore-auth-server/`](petstore-auth-server/) | `localhost:8092` | Demo OAuth. Password grant calls [loginUser](https://petstore3.swagger.io/#/user/loginUser) then [getUserByName](https://petstore3.swagger.io/#/user/getUserByName) and issues an end-user JWT with `userStatus`. Client-credentials grant issues the calling-app bearer. |
| [`async-order-server/`](async-order-server/) | `localhost:8091` | HTTP adapter for the spec’s AsyncAPI order channels. `POST /place-order` → Petstore `POST /store/order`. |
| [`mcp-server/`](mcp-server/) | `localhost:8080` | Engine host. Plan [`mcp-server/plans/petstore.arazzo.yaml`](mcp-server/plans/petstore.arazzo.yaml) (`x-planId: petstore`, version `0.0.1`). |

Workflows: `retrievePet`, `purchasePet`, `checkOrderStatus`. First step is **`getUserByName`**, not `loginUser`. Inbound OPA reads `input.auth.endUser.userStatus` from the JWT (no `http.send`). The executor mints a **new** JWT for Petstore using `SecretsProvider`.

Do not run `mcp-server` at the same time as another example (or `cmd/engine`) on `localhost:8080`.

## Architecture

```mermaid
flowchart LR
  subgraph clients [Clients]
    REST["HTTP client / curl"]
    MCP["MCP agent"]
  end

  subgraph host ["mcp-server :8080 context-mesh-engine"]
    Engine["engine.New"]
    Catalog["Arazzo catalog"]
    OPA["OPA inbound / outbound"]
    Exec["Executor httpExec"]
    Engine --> Catalog
    Engine --> OPA
    Engine --> Exec
  end

  subgraph backends [Backends]
    Auth["petstore-auth-server :8092"]
    Pet["Petstore 3 :8090"]
    Async["async-order-server :8091"]
  end

  REST -->|"Bearer client JWT + X-End-User-Token"| Engine
  MCP -->|"same headers on /mcp"| Engine
  REST -.-> Auth
  MCP -.-> Auth
  Auth -->|"loginUser + getUserByName"| Pet
  Exec -->|"getUserByName, findByStatus, getOrderById + downstream JWT"| Pet
  Exec -->|"place-order, confirm-order"| Async
  Async -->|"POST /store/order"| Pet
```

Request path inside the host (every REST execute and every MCP `run_*`):

```mermaid
sequenceDiagram
  participant C as Client
  participant W as Bearer wrap
  participant Pre as RequestPreprocessor
  participant E as Engine REST or MCP
  participant R as Runner
  participant P as OPA inbound
  participant X as Executor
  participant O as OPA outbound

  C->>W: Authorization Bearer client JWT
  W->>Pre: X-End-User-Token
  Pre->>E: PolicyRequestContext auth.endUser
  E->>R: Run + EnrichContext
  R->>P: data.plan.inbound (inputs + auth, no http.send)
  alt deny
    P-->>C: 403
  else allow
    P->>R: inputs + policyHints.username
    loop each Arazzo step
      R->>X: ExecutionRequest
      X->>X: mint downstream JWT from SecretsProvider
      X-->>R: ExecutionResponse
    end
    R->>O: data.plan.outbound (inputs + outputs)
    alt deny
      O-->>C: 403 (workflow already ran)
    else allow
      O-->>C: 200 outputs (possibly redacted)
    end
  end
```

`mcp-server` and `async-order-server` must target the **same** Petstore origin (`-petstore local|hosted` or `-petstore-url`). Share `-jwt-secret` between `mcp-server` and `petstore-auth-server` (default `petstore-demo-hs256`).

## Identity and tokens

Two JWTs on every execute (REST `POST /plans/…` and MCP `/mcp`):

| Header | Issuer | Role |
| --- | --- | --- |
| `Authorization: Bearer` | auth-server `client_credentials` | Calling **client app**. go-sdk `RequireBearerToken` (`MCPHandlerWrap` / REST wrap on execute only). |
| `X-End-User-Token` | auth-server `password` grant | **End user**. `RequestPreprocessor` verifies HS256, copies `username` / `userStatus` into OPA `input.auth.endUser`. |

Inbound OPA **must not** call Petstore. `userStatus` is already on the user JWT because the auth-server ran [loginUser](https://petstore3.swagger.io/#/user/loginUser) then [getUserByName](https://petstore3.swagger.io/#/user/getUserByName) at token issue.

The host `Executor` mints a **new** HS256 JWT (`SecretsProvider` key `downstream-hmac`) for Petstore HTTP. That signing key is **not** listed in `SecretInputs`, so it never appears in `$inputs`. This demo verifies inbound tokens with the **same shared HMAC**, not an issuer public key — see [Demo verification vs production](#demo-verification-vs-production).

Demo client: `client_id=petstore-mcp`, `client_secret=mcp-secret`. Users: `browser` / `abc123` (`userStatus` 1), `buyer` / `abc123` (`userStatus` 2). Package: [`petstore-auth-server/jwtx/`](petstore-auth-server/jwtx/). Auth-server: [`petstore-auth-server/README.md`](petstore-auth-server/README.md). How those tokens are wired into `engine.Options`: [OAuth and the engine SDK](#oauth-and-the-engine-sdk).

## Quick start

From the **repository root**:

```bash
./examples/petstore/petstore-openapi-server/run.sh
# other terminals:
go run ./examples/petstore/async-order-server
go run ./examples/petstore/petstore-auth-server
go run ./examples/petstore/mcp-server
```

Seed users, mint tokens, then execute (see [Seed users](#seed-users) and [REST](#rest-retrieve-purchase-check-order)):

```bash
CLIENT=$(curl -s -X POST http://localhost:8092/oauth/token \
  -H 'Content-Type: application/json' \
  -d '{"grant_type":"client_credentials","client_id":"petstore-mcp","client_secret":"mcp-secret"}' \
  | jq -r .access_token)
USER=$(curl -s -X POST http://localhost:8092/oauth/token \
  -H 'Content-Type: application/json' \
  -d '{"grant_type":"password","username":"browser","password":"abc123"}' \
  | jq -r .access_token)

curl -s -X POST http://localhost:8080/api/plans/petstore/retrievePet \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLIENT" \
  -H "X-End-User-Token: $USER" \
  -d '{"status":"sold"}'
```

Default `mcp-server` is **REST only**. `-dual` also mounts MCP Streamable HTTP at `/mcp`.

## The Arazzo plan

File: [`mcp-server/plans/petstore.arazzo.yaml`](mcp-server/plans/petstore.arazzo.yaml).

It is based on the [Arazzo 1.1 specification example](https://spec.openapis.org/arazzo/latest.html), adapted for this engine and for stock libopenapi:

| Spec example | This plan |
| --- | --- |
| `arazzo: 1.1.0` | `arazzo: 1.0.1` (libopenapi validator accepts 1.0.x) |
| one mixed retrieve+purchase workflow | three workflows so policy can allow retrieve without purchase |
| `operationId: $sourceDescriptions…loginUser` | `operationId: getUserByName` (identity from end-user JWT) |
| AsyncAPI send/receive | HTTP operations on [`sources/async-order.openapi.yaml`](mcp-server/sources/async-order.openapi.yaml) |
| `$message.payload` (1.1) | `$response.body` / JSON Pointers (1.0) |

### Catalog identity and sources

```yaml
info:
  version: 0.0.1
  x-planId: petstore
sourceDescriptions:
  - name: petStoreDescription
    url: ../sources/petstore.openapi.yaml
    type: openapi
  - name: asyncOrderApiDescription
    url: ../sources/async-order.openapi.yaml
    type: openapi
```

`engine.New` indexes `(x-planId, info.version)`. That pair becomes:

| Surface | Value |
| --- | --- |
| MCP tool | `run_petstore_v0.0.1` (default `run_{{.SafePlanID}}_v{{.SafeVersion}}`) |
| REST latest | `POST /api/plans/petstore/{workflowId}` |
| REST versioned | `POST /api/plans/petstore/v0.0.1/{workflowId}` |
| Generated OpenAPI | `GET /api/openapi` (catalog), `GET /api/openapi/petstore` (plan) |

`info.version` must be semver **without** a leading `v` (`0.0.1`, not `v0.0.1`). URL path tokens prepend `v`. See [Arazzo plans — spec requirements](../../docs/users/arazzo.md#spec-requirements).

`FileLoader` is pointed at **`plans/` only**. Relative `sourceDescriptions[].url` resolve against the Arazzo file’s directory (`../sources/...`). Do not pass OpenAPI or `.rego` files to `FileLoader`.

OpenAPI operations used by the plan:

| Source | Operation | Used by |
| --- | --- | --- |
| Petstore | `getUserByName` (`GET /user/{username}`) | all three workflows (`$inputs.policyHints.username`) |
| Petstore | `GET /pet/findByStatus` | `retrievePet` (`operationPath` JSON Pointer) |
| Petstore | `getOrderById` (`GET /store/order/{orderId}`) | `checkOrderStatus` |
| Async adapter | `placeOrder` (`POST /place-order`) | `purchasePet` |
| Async adapter | `confirmOrder` (`GET /confirm-order`) | `purchasePet` |

### Workflow: retrievePet

**Purpose:** load the end user (`getUserByName`), then return the first pet matching a status that **inbound policy** chooses. The caller’s `status` input is a hint for buyers only; browsers always search `available`. Username comes from the end-user JWT via `policyHints.username`, not from the JSON body.

**Inputs (required):** `status` (`available` \| `pending` \| `sold`). Generated OpenAPI and MCP `inputSchema` omit `policyHints` (and `secrets`); those keys stay in the Arazzo file for execution. Callers must not send them — the engine strips them at run time.

**Steps**

```mermaid
flowchart TB
  IN["inputs: status"]
  POL["inbound OPA: JWT userStatus → allow + hints"]
  DENY["403"]
  U["getUserStep: GET /user/{policyHints.username}"]
  G["getPetStep: GET /pet/findByStatus?status=policyHints.petStatus"]
  EMPTY{"body[0] is null?"}
  END["workflow end: no pet"]
  OUT["outputs: requestor, petId, pet"]

  IN --> POL
  POL -->|deny| DENY
  POL -->|allow| U
  U --> G
  G --> EMPTY
  EMPTY -->|yes onSuccess noPetsAvailable| END
  EMPTY -->|no| OUT
```

**Call sequence**

```mermaid
sequenceDiagram
  participant C as Client
  participant H as mcp-server
  participant P as Petstore :8090

  C->>H: POST /api/plans/petstore/retrievePet (two JWTs)
  Note over H: inbound reads input.auth.endUser (no http.send)
  alt deny
    H-->>C: 403
  else allow
    H->>P: GET /user/{username} getUserByName + downstream JWT
    P-->>H: User
    H->>P: GET /pet/findByStatus status=policyHints.petStatus
    P-->>H: pet array
    alt body[0] is null
      H-->>C: workflow end no pet
    else pet found
      Note over H: outbound redact /pet/photoUrls if mode is read
      H-->>C: requestor, petId, pet
    end
  end
```

Step bindings ([plan](mcp-server/plans/petstore.arazzo.yaml)):

1. **`getUserStep`** — `operationId: getUserByName`. Path `username` is `$inputs.policyHints.username` (from the end-user JWT). Success: `$statusCode == 200`. Step outputs are `firstName` / `lastName` / `email`. Workflow output `requestor` is `$steps.getUserStep.outputs` (the whole map). Inbound still cannot see this.
2. **`getPetStep`** — `operationPath` JSON Pointer to `GET /pet/findByStatus`. Query `status` is **`$inputs.policyHints.petStatus`**, not `$inputs.status`. Success: 200. If `$response.body#/0 == null`, `onSuccess` type `end` stops without workflow outputs for a pet. Otherwise `petId` / `pet` from the first array element.

Workflow outputs: `requestor` (`$steps.getUserStep.outputs`) plus `$steps.getPetStep.outputs.petId` and `.pet`.

For a `browser` (`userStatus` 1), inbound sets `petStatus` to `available` even if the JSON body said `"status":"sold"`. Outbound redacts `pet.photoUrls` in that mode.

### Workflow: purchasePet

**Purpose:** load the user, place an order on the async adapter, poll until confirmation, return `orderId`. Requires inbound **buy** (`userStatus` 2). A `browser` call is **403** before any step runs.

**Inputs (required):** `petId`, `orderCorrelationId` (any unique string; AsyncAPI `orderRequestId`).

**Steps**

```mermaid
flowchart TB
  IN["inputs: petId, orderCorrelationId"]
  POL["inbound OPA: JWT userStatus must be 2"]
  DENY["403"]
  U["getUserStep: GET /user/{policyHints.username}"]
  PL["purchasePetStep: POST /place-order"]
  CF{"confirm-order 200?"}
  OUT["outputs: requestor, orderId"]

  IN --> POL
  POL -->|deny| DENY
  POL -->|allow| U
  U --> PL
  PL --> CF
  CF -->|404 retry up to 6s| CF
  CF -->|200| OUT
```

**Call sequence**

```mermaid
sequenceDiagram
  participant C as Client
  participant H as mcp-server
  participant P as Petstore :8090
  participant A as async-order-server :8091

  C->>H: POST /api/plans/petstore/purchasePet (two JWTs)
  Note over H: inbound reads input.auth.endUser
  alt deny
    H-->>C: 403
  else allow
    H->>P: GET /user/{username} getUserByName
    P-->>H: User
    H->>A: POST /place-order header orderCorrelationId body petId
    A->>P: POST /store/order id petId status placed
    P-->>A: order
    A-->>H: 200 accepted
    loop GET confirm-order until 200 or timeout
      H->>A: GET /confirm-order same correlation header
      alt not in map
        A-->>H: 404
      else stored
        A-->>H: 200 payload.orderId
      end
    end
    H-->>C: requestor, orderId
  end
```

Step bindings:

1. **`getUserStep`** — same as retrieve.
2. **`purchasePetStep`** — `operationPath: $sourceDescriptions.asyncOrderApiDescription.placeOrder`. Header `orderCorrelationId` from `$inputs.orderCorrelationId`. JSON body `{ petId: $inputs.petId }`. The executor treats source name `asyncOrderApiDescription` as the adapter origin (`-async-order-url`, default `http://localhost:8091`).
3. **`confirmPetPurchaseStep`** — `confirmOrder`. Same correlation header. Output `orderId: $response.body#/payload/orderId` (matches the adapter’s JSON envelope, not a Petstore `Order` object).

The executor **polls** GET confirm for up to 6 seconds on HTTP 404 (`confirmWait` in [`mcp-server/executor.go`](mcp-server/executor.go)). The adapter stores confirmations in memory keyed by correlation id ([`async-order-server/server.go`](async-order-server/server.go)).

Local Docker Petstore does not allocate order ids (omit `id` and you get `0`). The adapter always POSTs a generated non-zero `id`; hosted petstore3’s id is used when it returns non-zero.

### Workflow: checkOrderStatus

**Purpose:** load the user and `GET /store/order/{orderId}`. Buyers only.

**Inputs (required):** `orderId`.

**Steps**

```mermaid
flowchart TB
  IN["inputs: orderId"]
  POL["inbound OPA: JWT userStatus must be 2"]
  DENY["403"]
  U["getUserStep: GET /user/{policyHints.username}"]
  G["getOrderStep: GET /store/order/{orderId}"]
  OUT["outputs: requestor, orderId, petId, status, complete"]

  IN --> POL
  POL -->|deny| DENY
  POL -->|allow| U
  U --> G
  G --> OUT
```

**Call sequence**

```mermaid
sequenceDiagram
  participant C as Client
  participant H as mcp-server
  participant P as Petstore :8090

  C->>H: POST /api/plans/petstore/checkOrderStatus (two JWTs)
  Note over H: inbound reads input.auth.endUser
  alt deny
    H-->>C: 403
  else allow
    H->>P: GET /user/{username} getUserByName
    P-->>H: User
    H->>P: GET /store/order/{orderId}
    P-->>H: order
    H-->>C: requestor, orderId, petId, status, complete
  end
```

`status` is whatever Petstore stored (`placed`, `approved`, `delivered`). Petstore has **no PUT** for orders; status is set at `POST /store/order`. After `purchasePet`, `checkOrderStatus` reads that row back.

### Runtime expressions

Expressions the plan relies on:

| Expression | Meaning in this engine |
| --- | --- |
| `$inputs.policyHints.username` | End-user JWT username (inbound hint) |
| `$inputs.policyHints.petStatus` | Single input **name** `policyHints.petStatus` (see [hints](#how-policyhints-reach-arazzo)) |
| `$statusCode == 200` | `ExecutionResponse.StatusCode` |
| `$response.body` | Decoded JSON body |
| `$response.body#/0/id` | JSON Pointer into the body |
| `$sourceDescriptions.petStoreDescription.url` | Resolved OpenAPI source URL (used in `operationPath`) |

The engine does not evaluate expressions itself; libopenapi’s Arazzo runtime does, using the `Executor` result for each step.

## OPA policies

Directory: [`mcp-server/policies/petstore/0.0.1/`](mcp-server/policies/petstore/0.0.1/).

Packages **must** be `plan.inbound` and `plan.outbound`. The engine evaluates `data.plan.inbound` and `data.plan.outbound`. Missing or non-boolean `allow` is **deny**. Use `default allow := false`.

### Layout and wiring

`FilePolicyLoader` reads `{Dir}/{planId}/{version}/`:

```text
mcp-server/policies/
  petstore/
    0.0.1/
      inbound.rego
      outbound.rego
```

`planId` and `version` are single path segments matching `x-planId` and `info.version` (`0.0.1`, not `v0.0.1`). Optional `data.json` in that folder is merged with `FilePolicyLoader.Data` (struct keys win). This host injects the Petstore origin at process start:

```go
PolicyLoader: &arazzo.FilePolicyLoader{
    Dir:  policiesDir(),
    Data: map[string]any{"petstoreBase": petstoreBase},
},
```

Rego does **not** `http.send`. Identity is `input.auth.endUser` from the preprocessor. Compiled bundles are cached for `Options.PolicyCacheTTL` (default 5 minutes). Load/compile failure is **500** (fail closed) unless a previously compiled bundle is still cached. Deny is **403**. Invalid preprocessor (missing/bad `X-End-User-Token`) is **401**.

Do not put `.rego` on `ArazzoLoaders`.

### When policy runs

```mermaid
flowchart TB
  START["Run(planId, version, workflowId, inputs)"]
  STRIP["Strip caller policyHints and policyHints.*"]
  IN["Eval data.plan.inbound<br/>input: planId, version, workflowId, inputs"]
  DENYI["403 inbound"]
  HINTS["If allow and hints object:<br/>set inputs.policyHints + dotted leaves"]
  WF["libopenapi RunWorkflow → Executor per step"]
  OUT["Eval data.plan.outbound<br/>input: same + outputs"]
  DENYO["403 outbound — outputs not returned"]
  REDACT["Optional redact / replace outputs"]
  OK["200 workflow outputs"]

  START --> STRIP --> IN
  IN -->|allow false| DENYI
  IN -->|allow true| HINTS --> WF --> OUT
  OUT -->|allow false| DENYO
  OUT -->|allow true| REDACT --> OK
```

OPA `input` for inbound:

```json
{
  "planId": "petstore",
  "version": "0.0.1",
  "workflowId": "retrievePet",
  "inputs": { "status": "sold" },
  "auth": {
    "endUser": { "username": "browser", "userStatus": 1 },
    "client": { "UserID": "petstore-mcp" }
  },
  "headers": {}
}
```

Outbound adds `"outputs": { ... }` (the Arazzo workflow outputs map) and sees `inputs` **after** hints were applied (`policyHints` nested object is present).

### Inbound (`inbound.rego`)

File: [`inbound.rego`](mcp-server/policies/petstore/0.0.1/inbound.rego).

1. Read `input.auth.endUser` (`username`, `userStatus`). Missing user JWT is **401** before OPA (preprocessor). Empty username in claims is deny.
2. `user_status` is `to_number` of that claim (default 1).
3. Allow sets:
   - `user_status == 2` → `{retrievePet, purchasePet, checkOrderStatus}`
   - otherwise → `{retrievePet}` only
4. Hints always include `username` from the JWT:
   - `user_status != 2` → `{"mode": "read", "petStatus": "available", "username": …}`
   - `user_status == 2` → `{"mode": "buy", "petStatus": object.get(input.inputs, "status", "available"), "username": …}`

| Username | Password (auth-server only) | `userStatus` | Allowed workflows | `policyHints.petStatus` |
| --- | --- | --- | --- | --- |
| `browser` | `abc123` | `1` | `retrievePet` only | always `available` |
| `buyer` | `abc123` | `2` | retrieve, purchase, check order | caller `status`, default `available` |

A `browser` `purchasePet` is denied **before** any Arazzo step. Policy is the source of truth for hints; forged `policyHints` in the request body are discarded. Password is never a plan input.

### Outbound (`outbound.rego`)

File: [`outbound.rego`](mcp-server/policies/petstore/0.0.1/outbound.rego).

- `allow` if `input.outputs` is an object (including `{}`).
- If `input.inputs.policyHints.mode == "read"` and `workflowId == "retrievePet"`, `redact := ["/pet/photoUrls"]`.

Redact entries are RFC 6901 pointers into **workflow outputs**. Default mask is JSON `null`. Outbound deny hides outputs even though steps already ran.

### How `policyHints` reach Arazzo

Stock libopenapi treats `$inputs.policyHints.petStatus` as the **single map key** `"policyHints.petStatus"`, not a nested walk of `policyHints` then `petStatus`.

On inbound allow the engine therefore stores both:

- `inputs["policyHints"]` — nested object (OPA outbound uses `object.get(input.inputs, "policyHints", {})`)
- `inputs["policyHints.petStatus"]`, `inputs["policyHints.mode"]` — dotted leaves for Arazzo `$inputs.policyHints.*`

Caller keys `policyHints` and `policyHints.*` are stripped first. Implementation: `applyInbound` / `flattenPolicyHints` in `internal/plans/policy.go`. Public constant: [`arazzo.PolicyHintsKey`](../../arazzo/policy.go).

When you write a plan, use `$inputs.policyHints.<leaf>` for injected leaves. Do not assume `$inputs.policyHints` as an object is visible to step parameters unless you also flatten (the engine does).

## Building services on context-mesh-engine

The engine is a **host**, not a pet-store client. You supply catalogs, backends, and policy. Public packages: `engine` and `arazzo`. Do not import `internal/`.

### What the engine owns

| The engine | You |
| --- | --- |
| Parse/validate Arazzo, resolve `sourceDescriptions` | `Loader` that yields YAML/JSON bytes |
| MCP `run_*` + REST `/plans` + generated OpenAPI | `Executor` that performs each step’s HTTP call |
| Optional OPA compile/eval around `Run` | `PolicyLoader` + `.rego`; `RequestPreprocessor` for extra JWTs |
| HTTP listener (`ListenAndServe`) or `Handler()` | Process flags, TLS, extra REST controllers, `MCPHandlerWrap` / `RESTHandlerWrap` |
| Tool names from `x-planId` + version | Optional `ToolDoc` / `ToolHelpLookup` |

Nil `ArazzoLoaders`: no plan tools or plan REST. Nil `ArazzoExecutor`: catalog and OpenAPI still work; execute is **501**. Nil `PolicyLoader`: skip inbound/outbound. Nil `QueryMatcher`: MCP `query` and `POST /plans/query` are **not** registered (this demo leaves it unset).

### Host process: `engine.New`

[`mcp-server/main.go`](mcp-server/main.go) (`hostOptions`) is the pattern to copy. Each `engine.Options` field is commented there with what the engine does if you omit it.

```go
e, err := engine.New(hostOptions(*addr, *asyncURL, petstoreBase, *jwtSecret, *dual))
```

`plansDir()` / `policiesDir()` use `runtime.Caller` so `go run` and `go test` resolve `plans/` and `policies/` next to `main.go`, independent of cwd.

`PublicBaseURL` is the origin written into REST tool descriptions (`GET /api/tools`). It is not inferred from `Addr`. Empty → path-only URLs.

`New` **fails** if a document cannot be parsed, Arazzo validation fails, sources cannot be resolved, two documents share `(x-planId, version)`, `info.version` is not semver (or starts with `v`), or MCP tool names collide.

### Loader

```go
arazzo.NewFileLoader("/path/to/plans")
```

Walks recursively for `.yaml`, `.yml`, `.json`. `Source.BaseURL` is `file://` + the Arazzo file’s directory **with a trailing slash** so `../sources/openapi.yaml` resolves correctly (RFC 3986).

For HTTP, embed, or object storage, implement `arazzo.Loader` (`Load(ctx) ([]Source, error)`). Honor `ctx`. Set `BaseURL`. Duplicate plan identities across loaders fail `New`.

### Executor

Type: `arazzo.Executor` (alias of libopenapi `arazzo.Executor`).

```go
Execute(ctx context.Context, req *arazzo.ExecutionRequest) (*arazzo.ExecutionResponse, error)
```

Each workflow step is **one** `Execute` call. This demo’s implementation is [`httpExec`](mcp-server/executor.go):

1. Resolve HTTP method and path from `OperationPath` (`#/paths/~1pet~1findByStatus/get`) or `operationId` on `req.Source.OpenAPIDocument`.
2. Choose origin: Petstore base URL unless the source is `asyncOrderApiDescription`, then `-async-order-url`.
3. Apply OpenAPI parameter `in` (path / query / header). Remaining names: `Authorization` and correlation headers go to headers; other names to query.
4. If `Authorization` is still empty, mint a downstream HS256 JWT from `SecretsProvider` (`downstream-hmac`) and set `Authorization: Bearer`. This is a **new** token, not the client or end-user JWT.
5. Marshal `RequestBody` as JSON when present.
6. Return `ExecutionResponse{StatusCode, Headers, Body, URL, Method}`. `Body` is decoded JSON (`UseNumber` then canonicalized to `int64` when possible) so JSON Pointers and success criteria see numbers, not strings.

Return a **response** (not a Go error) for HTTP 4xx/5xx the plan should observe (`$statusCode == 200` fails the step). Return a Go error for transport failures.

Async confirm: if the source is `asyncOrderApiDescription` and the method is GET, 404 is retried until `confirmWait` (6s).

Your executor can be gRPC, a message bus, or a stub. The plan only needs `StatusCode` and a `Body` the expressions understand. [arazzo-fs](../arazzo-fs/README.md) returns `200` and `{"status":"ok"}` for every step.

### PolicyLoader

```go
type PolicyLoader interface {
    Load(ctx context.Context, req PolicyRequest) (*PolicyBundle, error)
}
```

`PolicyRequest` is `{PlanID, Version}`. Nil `*PolicyBundle` means no policy for that key (not an error). Implement this if policies live in a DB or sidecar; `FilePolicyLoader` is the filesystem default.

Rego decision objects:

```json
{ "allow": true, "hints": { "petStatus": "available", "mode": "read" } }
```

```json
{ "allow": true, "redact": ["/pet/photoUrls"], "mask": "***" }
```

`hints` is inbound-only. Outbound may set `outputs` (replaces the workflow map and ignores `redact`) or `redact` pointers. Details: [Adapters — PolicyLoader](../../docs/users/adapters.md#policyloader).

### OAuth and the engine SDK

The engine does **not** implement OAuth. It exposes four host-owned seams on [`engine.Options`](../../docs/users/configuration.md#options-reference). This demo uses all of them. Operator-facing token table: [Identity and tokens](#identity-and-tokens). Contracts: [Configuration — Auth](../../docs/users/configuration.md#auth), [Adapters — RequestPreprocessor](../../docs/users/adapters.md#requestpreprocessor), [Adapters — SecretsProvider](../../docs/users/adapters.md#secretsprovider).

| Token | Who mints it | SDK seam | Where it is consumed |
| --- | --- | --- | --- |
| Calling-app JWT (`Authorization: Bearer`) | [`petstore-auth-server`](petstore-auth-server/) `client_credentials` | `MCPHandlerWrap` / `RESTHandlerWrap` with go-sdk [`auth.RequireBearerToken`](https://github.com/modelcontextprotocol/go-sdk/blob/main/auth/auth.go) | HTTP middleware **before** REST catalog + execute / MCP Streamable HTTP |
| End-user JWT (`X-End-User-Token`) | auth-server `password` grant (`loginUser` + `getUserByName`) | `RequestPreprocessor` | `Runner.EnrichContext` on execute/`query`, **before** inbound OPA |
| Downstream JWT (Petstore `Authorization`) | this host’s `Executor` | `SecretsProvider` (`Get("downstream-hmac")`) | Each OpenAPI step if the plan did not already set `Authorization` |

`RequireBearerToken` verifies **one** bearer. Extra JWTs on `x-*` headers are not that middleware’s job. They belong on `RequestPreprocessor`, which may also call a remote user-info service. This demo does not: `userStatus` is already a claim on the user JWT.

#### What you pass to `engine.New`

From [`mcp-server/main.go`](mcp-server/main.go) and [`mcp-server/auth.go`](mcp-server/auth.go):

1. **`MCPHandlerWrap` / `RESTHandlerWrap`** — wrap **child** handlers (`/mcp` and the REST mux after `APIPrefix` strip), never the root mux. The engine applies `RESTHandlerWrap` **before** `APITimeout`. Petstore wraps **all** MCP traffic (so `initialize` and `tools/list` need the client JWT). REST wrap is `wrapRESTPlans`: client JWT on `GET /tools`, `GET /openapi/…`, and `POST /plans/…`. `GET /health` and `GET /docs` stay open.
2. **`RequestPreprocessor`** — `dualJWTPreprocessor` reads `X-End-User-Token`, verifies HS256 (`jwtx.ParseUser`), and returns `PolicyRequestContext`. `Auth.endUser` is `{username, userStatus, sub}`. `Auth.client` is copied from `RequestSource.ClientAuth` (go-sdk `TokenInfo` after the bearer wrap). Allowlisted headers only (`X-Request-Id` → `x-request-id`). Do not put raw `Authorization` or the user JWT into `Headers`.
3. **`SecretsProvider`** — `arazzo.MapSecrets{"downstream-hmac": jwt-secret}` shared with the auth-server’s `-jwt-secret`. The same map is passed into `newHTTPExec`. **`SecretInputs` is empty**: the HMAC key is not flattened onto `$inputs.secrets.*`. Caller-supplied `secrets` / `secrets.*` keys are stripped either way.
4. **`PolicyLoader`** — inbound reads `input.auth.endUser`, not `http.send`. Invalid preprocessor is **401**; inbound deny is **403**.

The engine stores the preprocessor result on `context.Context` (`arazzo.WithPolicyRequest`). Inbound/outbound OPA read it as `input.auth` / `input.headers`. The `Executor` reads the same context to set `sub`/`username` on the **new** downstream JWT. It does not forward the caller’s bearer.

#### Demo verification vs production

[`petstore-auth-server/jwtx/jwt.go`](petstore-auth-server/jwtx/jwt.go) is a **demo** crypto helper, not a production verifier. Auth-server and mcp-server share `-jwt-secret` and sign/verify **HS256** (symmetric HMAC). `parseHS256` returns that secret as the key. There is no issuer public key, JWKS, RS256, or ES256.

`iss` (`petstore-auth`) and `aud` (`petstore-mcp`) are **written** on sign. They are **not** checked on parse (`jwt.WithIssuer` / `jwt.WithAudience` are unused). `ParseWithClaims` still enforces `exp`. After HMAC succeeds, the host only checks `token_use` (`client` vs `user`) and that the user token has `username`.

| | This demo | Production host |
| --- | --- | --- |
| Algorithm | HS256, one shared secret | RS256 or ES256 (asymmetric) |
| Verify with | HMAC key both processes know | Issuer **public** key / JWKS (`jwks_uri` from discovery) |
| `iss` / `aud` | Set, not validated | Must match your IdP issuer and this API’s audience |
| Key location | `-jwt-secret` / `MapSecrets` | Private key stays on the IdP; host fetches JWKS (cache + rotation) |
| Downstream call | Same HMAC, new JWT (`IssuerEngine`) | Separate credential: token exchange, IdP client-credentials for the domain API, or mTLS — **not** the inbound access token |

The **SDK seams do not change**. `RequireBearerToken` still takes a `TokenVerifier`; `RequestPreprocessor` still verifies extra JWTs; `SecretsProvider` still supplies whatever the `Executor` needs to mint or fetch a downstream credential. You replace `jwtx.ParseClient` / `ParseUser` (and downstream signing) with JWKS-backed verification and an explicit `iss`/`aud` check. Do not give the plan host the IdP’s private key. Do not share an HMAC between IdP and host in production.

Petstore 3 in this demo does not validate that downstream JWT; the header is there to show the executor pattern. A real domain API would verify it the same way (JWKS / `iss` / `aud`).

#### Request path (execute)

```mermaid
sequenceDiagram
  participant C as Client
  participant W as Handler wrap
  participant E as Engine REST or MCP
  participant Pre as RequestPreprocessor
  participant P as OPA inbound
  participant X as Executor
  participant S as SecretsProvider
  participant API as Petstore / async adapter

  C->>W: Authorization Bearer (client JWT)
  alt invalid or missing client JWT
    W-->>C: 401
  else verified
    W->>E: TokenInfo on request context
    Note over E: REST POST /plans or MCP run_*
    E->>Pre: RequestSource (headers + ClientAuth)
    alt missing or invalid X-End-User-Token
      Pre-->>C: 401
    else ok
      Pre->>P: input.auth.endUser + input.auth.client
      alt deny
        P-->>C: 403
      else allow
        loop each Arazzo step
          X->>S: Get("downstream-hmac")
          S-->>X: signing key
          X->>API: Authorization Bearer (new JWT)
          API-->>X: HTTP response
        end
      end
    end
  end
```

#### Host code you own vs engine code

| You implement (this example) | The engine does |
| --- | --- |
| HS256 parse/sign ([`petstore-auth-server/jwtx/`](petstore-auth-server/jwtx/)) | Mount wraps on MCP and REST children |
| `jwtVerifier.verifyClient` for `RequireBearerToken` | `RequestSourceFromHTTP` / `RequestSourceFromMCP` (`TokenInfo` → `ClientAuth`) |
| `dualJWTPreprocessor` | `Runner.EnrichContext` → `ErrUnauthorized` (**401**) |
| `httpExec.downstreamBearer` | Strip caller `secrets`; inject only `SecretInputs` names |
| Demo IdP ([`petstore-auth-server`](petstore-auth-server/)) | Eval inbound/outbound; never issue tokens |

Copy this pattern when the calling app and the end user are different principals, inbound must not call the domain API, and domain APIs need a **host-minted** credential. If you only have one bearer, set `MCPHandlerWrap` / `RESTHandlerWrap` and leave `RequestPreprocessor` nil.

### HTTP surfaces

`DualMCPandREST`, `MCPOnly`, and `RESTOnly` are mutually exclusive. **All false (default) = REST only**; `/mcp` is not mounted. `-dual` sets `DualMCPandREST`.

When `ArazzoLoaders` is set, `New` registers:

| Surface | Path / name |
| --- | --- |
| REST execute latest | `POST {APIPrefix}/plans/{planId}/{workflowId}` |
| REST execute versioned | `POST {APIPrefix}/plans/{planId}/{version}/{workflowId}` |
| REST OpenAPI catalog | `GET {APIPrefix}/openapi` |
| REST OpenAPI plan | `GET {APIPrefix}/openapi/{planId}` |
| REST Swagger UI (this example) | `GET {APIPrefix}/docs` |
| REST tools | `GET {APIPrefix}/tools` (MCP `tools/list` envelope; REST descriptions) |
| REST health | `GET {APIPrefix}/health` |
| MCP `run_*` | only if MCP is mounted (`-dual` or `MCPOnly`) |
| MCP `query` | only if `QueryMatcher` is set **and** MCP is mounted |

REST POST body is the workflow **inputs object** (no `workflowId` wrapper). MCP arguments wrap `workflowId` + `inputs`. HTTP 200 body is the Arazzo **outputs** map.

| HTTP status | Meaning |
| --- | --- |
| 200 | Outputs JSON |
| 400 | Invalid JSON, workflow failure, other runner error |
| 401 | Bearer wrap or `RequestPreprocessor` rejected the call |
| 403 | Inbound or outbound deny |
| 404 | Unknown plan, version, or workflow |
| 500 | Policy load/compile failed |
| 501 | No executor |

### AsyncAPI as OpenAPI HTTP

The official Arazzo 1.1 example uses AsyncAPI 3 send/receive. This engine’s executor is HTTP-shaped, so the plan’s second source is an OpenAPI **binding** of those channels:

- Spec: [`async-order-server/pet-asyncapi.yaml`](async-order-server/pet-asyncapi.yaml)
- Binding: [`mcp-server/sources/async-order.openapi.yaml`](mcp-server/sources/async-order.openapi.yaml)
- Server: [`async-order-server/`](async-order-server/)

| HTTP | AsyncAPI |
| --- | --- |
| `POST /place-order` | `placeOrder` |
| `GET /confirm-order` | `confirmOrder` |
| Header `orderCorrelationId` / `orderRequestId` | correlation |
| Confirm body `{ headers, payload: { orderId } }` | message envelope |

That is the pattern for any non-OpenAPI backend: publish a small OpenAPI document as a `sourceDescription`, and implement the HTTP in a sidecar (or inside the executor). The engine never speaks AsyncAPI natively.

### Checklist for your own host

1. Author Arazzo 1.0.x with `info.x-planId` and semver `info.version` (no leading `v`).
2. Put OpenAPI (and adapter OpenAPI) beside the plan; use relative `sourceDescriptions` URLs.
3. Point `FileLoader` at the **plans directory only**.
4. Implement `Executor`: map `operationId` / `operationPath` to your origins; set `StatusCode` and JSON `Body`. Mint a **new** downstream JWT from `SecretsProvider` (do not forward the caller’s bearer).
5. Optional: `{policies}/{planId}/{version}/inbound.rego` and `outbound.rego`; pass `FilePolicyLoader{Dir, Data}`. Inbound reads `input.auth` from `RequestPreprocessor`, not `http.send`.
6. `engine.New` + `ListenAndServe` (or `Handler()` on your `http.Server`). Wire `MCPHandlerWrap` / `RESTHandlerWrap` for the client JWT; preprocessor for extra `x-*` JWTs.
7. Default REST; `-dual` when an MCP client needs `/mcp`.
8. Leave `QueryMatcher` nil until you have a real matcher; agents call `run_*` or REST execute.
9. Exercise 401 (bad/missing tokens), deny (403), missing executor (501), and generated `GET /api/openapi/{planId}`.

Smaller embed without plans: [minimal](../minimal/README.md). Plans without backends: [arazzo-fs](../arazzo-fs/README.md). Full `Options`: [Configuration](../../docs/users/configuration.md).

## Operate the demo

### Petstore 3 in Docker

`run.sh` **builds** `context-mesh-petstore3:local` from [petstore-openapi-server/Dockerfile](petstore-openapi-server/Dockerfile) when that tag is missing (upstream [swaggerapi/petstore3:latest](https://hub.docker.com/r/swaggerapi/petstore3) by default; override with `--upstream` / `PETSTORE_UPSTREAM`). The local image adds an SLF4J 1.7 binding so Jetty does not print `StaticLoggerBinder` errors. `run.sh` stays in the **foreground**; Ctrl+C removes the container and its in-memory data. If pull or build times out, the Docker VM often cannot reach Hub / Maven Central while a VPN is up — disconnect VPN, restart Docker Desktop, retry, or use `-petstore hosted`.

From the **repository root**:

```bash
./examples/petstore/petstore-openapi-server/run.sh
```

Windows (PowerShell):

```powershell
./examples/petstore/petstore-openapi-server/run.ps1
```

Force a fresh image: `run.sh --rebuild` / `run.ps1 -Rebuild`. Pass `--upstream IMAGE` / `-Upstream IMAGE` to change the Docker Hub base (default `swaggerapi/petstore3:latest`).

Host port **8090** avoids clashing with `mcp-server` on 8080 (the process inside the container still uses 8080).

```bash
curl -s http://localhost:8090/api/v3/openapi.json | head
curl -s 'http://localhost:8090/api/v3/pet/findByStatus?status=available' | head
```

Stop: Ctrl+C in the `run.sh` terminal (container `--rm`). Image names and port: [petstore-openapi-server/README.md](petstore-openapi-server/README.md). If you change the port, pass `-petstore-url http://localhost:PORT/api/v3` to **async-order-server**, **petstore-auth-server**, and **mcp-server**.

### Hosted Petstore 3

Skip Docker. The public demo is often flaky (`/pet/findByStatus`, `/store/order`).

```bash
go run ./examples/petstore/async-order-server -petstore hosted
go run ./examples/petstore/mcp-server -petstore hosted          # REST only
go run ./examples/petstore/mcp-server -dual -petstore hosted    # REST + /mcp (instead of the line above)
```

Direct checks use `https://petstore3.swagger.io/api/v3/...`.

Custom origin:

```bash
go run ./examples/petstore/mcp-server -petstore-url http://127.0.0.1:8090/api/v3
```

### Run the Go servers

For **local** Petstore, start Docker first. Then from the **repository root**:

```bash
go run ./examples/petstore/async-order-server
go run ./examples/petstore/petstore-auth-server
go run ./examples/petstore/mcp-server
```

Optional flags:

```bash
go run ./examples/petstore/async-order-server -addr localhost:8091 -petstore local
go run ./examples/petstore/petstore-auth-server -addr localhost:8092 -petstore local
go run ./examples/petstore/mcp-server -addr localhost:8080 -async-order-url http://localhost:8091 -petstore local
go run ./examples/petstore/mcp-server -dual
```

Share `-jwt-secret` between auth-server and mcp-server when you override it.

Checks:

```bash
curl -s http://localhost:8091/health
curl -s http://localhost:8092/health
curl -s http://localhost:8080/api/health
curl -s http://localhost:8080/api/tools
curl -s http://localhost:8080/api/openapi
curl -s http://localhost:8080/api/openapi/petstore
```

### Seed users

The auth-server calls `GET /user/login` then `GET /user/{username}` so `userStatus` is in the **user JWT**. Seed Petstore users before requesting password-grant tokens:

```bash
curl -s -X POST http://localhost:8090/api/v3/user \
  -H 'Content-Type: application/json' \
  -d '{"username":"browser","password":"abc123","userStatus":1,"firstName":"Pat","lastName":"Browser","email":"browser@example.com"}'

curl -s -X POST http://localhost:8090/api/v3/user \
  -H 'Content-Type: application/json' \
  -d '{"username":"buyer","password":"abc123","userStatus":2,"firstName":"Alex","lastName":"Buyer","email":"buyer@example.com"}'
```

Hosted origin: `https://petstore3.swagger.io/api/v3/user`.

Mint tokens (default client `petstore-mcp` / `mcp-secret`):

```bash
CLIENT=$(curl -s -X POST http://localhost:8092/oauth/token \
  -H 'Content-Type: application/json' \
  -d '{"grant_type":"client_credentials","client_id":"petstore-mcp","client_secret":"mcp-secret"}' \
  | jq -r .access_token)
BROWSER=$(curl -s -X POST http://localhost:8092/oauth/token \
  -H 'Content-Type: application/json' \
  -d '{"grant_type":"password","username":"browser","password":"abc123"}' \
  | jq -r .access_token)
BUYER=$(curl -s -X POST http://localhost:8092/oauth/token \
  -H 'Content-Type: application/json' \
  -d '{"grant_type":"password","username":"buyer","password":"abc123"}' \
  | jq -r .access_token)
```

### Swagger UI

mcp-server serves HTML at **`http://localhost:8080/api/docs`** ([`mcp-server/docs/index.html`](mcp-server/docs/index.html), registered with `e.AddController`). Catalog OpenAPI requires a client JWT, so a first visit with no token redirects to **`/api/docs/login`**. That page stores the token in `sessionStorage` (not in the URL) and sends you back to `/api/docs`, which then loads [Swagger UI](https://github.com/swagger-api/swagger-ui) from a CDN (`swagger-ui-dist@5.27.1`) and fetches this process’s generated OAS on the **same origin** (no CORS). The first spec request already has `Authorization: Bearer`.

| Dropdown | Spec |
| --- | --- |
| Catalog | `GET /api/openapi` — `/tools` plus `$ref` to each latest plan |
| petstore (latest) | `GET /api/openapi/petstore` |
| petstore v0.0.1 | `GET /api/openapi/petstore/v0.0.1` |

The sticky bar on `/api/docs` is for **Try it out**. A `requestInterceptor` copies the fields into headers; it does not log the values. **Log out** clears `sessionStorage` and returns to login.

1. Seed users and mint `$CLIENT`, `$BROWSER` / `$BUYER` as above.
2. Open [http://localhost:8080/api/docs](http://localhost:8080/api/docs) (needs network once for the CDN). You land on `/api/docs/login` until a client token is stored.
3. Paste the client access token (`$CLIENT`) and submit. The catalog spec should load.
4. Paste an end-user access token into **End-user JWT** (`X-End-User-Token`) before **Try it out** on `POST /plans/…`.
5. Pick a spec in the dropdown. Execute `retrievePet` / `purchasePet` / `checkOrderStatus` with the same token rules as curl (`browser` retrieve-only; `buyer` may purchase).

`GET /health` and `GET /docs` (login + UI HTML) stay unauthenticated. `GET /tools` and `GET /openapi/…` need the client JWT (the UI sends it after login). `POST /plans/` needs both JWTs.

### REST: retrieve, purchase, check order

A `browser` token can only retrieve. Inbound forces `petStatus` to `available`. Missing tokens are **401**.

```bash
curl -s -X POST http://localhost:8080/api/plans/petstore/retrievePet \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLIENT" \
  -H "X-End-User-Token: $BROWSER" \
  -d '{"status":"sold"}'
```

`buyer` may pass `status` through as `policyHints.petStatus`:

```bash
curl -s -X POST http://localhost:8080/api/plans/petstore/retrievePet \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLIENT" \
  -H "X-End-User-Token: $BUYER" \
  -d '{"status":"available"}'
```

Versioned URL: `POST /api/plans/petstore/v0.0.1/retrievePet`. Pick a `petId` from the response (or `pet.id`). Direct Petstore: `GET http://localhost:8090/api/v3/pet/1`.

Purchase requires the async adapter and a **buyer**. A `browser` token returns **403**. `orderCorrelationId` is any unique string:

```bash
curl -s -X POST http://localhost:8080/api/plans/petstore/purchasePet \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLIENT" \
  -H "X-End-User-Token: $BUYER" \
  -d '{"petId":1,"orderCorrelationId":"demo-order-1"}'
```

Save `orderId`, then:

```bash
curl -s -X POST http://localhost:8080/api/plans/petstore/checkOrderStatus \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLIENT" \
  -H "X-End-User-Token: $BUYER" \
  -d '{"orderId":1}'
```

Replace `orderId` with the id from `purchasePet`.

Petstore has no PUT for orders. To plant a known id:

```bash
curl -s -X POST http://localhost:8090/api/v3/store/order \
  -H 'Accept: application/json' -H 'Content-Type: application/json' \
  -d '{"id":900001,"petId":1,"quantity":1,"status":"placed","complete":false}'

curl -s -X POST http://localhost:8080/api/plans/petstore/checkOrderStatus \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLIENT" \
  -H "X-End-User-Token: $BUYER" \
  -d '{"orderId":900001}'
```

Direct GET (no engine): `curl -s -H 'Accept: application/json' http://localhost:8090/api/v3/store/order/900001`.

### MCP

Start with `-dual` so Streamable HTTP is at **`http://localhost:8080/mcp`** (not `/mcp/`).

```bash
go run ./examples/petstore/mcp-server -dual
```

Cursor: Settings → MCP → add a **URL** server `http://localhost:8080/mcp`. Restart the agent chat so it reloads tools.

This example does not set `QueryMatcher`, so `query` is not registered. The plan registers:

| Tool | Use |
| --- | --- |
| `run_petstore_v0.0.1` | `workflowId` is `retrievePet`, `purchasePet`, or `checkOrderStatus` |

Example prompts:

- “Using the petstore MCP tools, retrieve an available pet as the browser user.”
- “Purchase pet id 1 with orderCorrelationId demo-order-1 as buyer.”
- “Check order status for orderId \<id from purchase\>, same buyer.”

The MCP client must send `Authorization: Bearer <client JWT>` and `X-End-User-Token: <user JWT>`. The agent should call `run_petstore_v0.0.1` with:

```json
{
  "workflowId": "retrievePet",
  "inputs": { "status": "available" }
}
```

```json
{
  "workflowId": "purchasePet",
  "inputs": {
    "petId": 1,
    "orderCorrelationId": "demo-order-1"
  }
}
```

```json
{
  "workflowId": "checkOrderStatus",
  "inputs": { "orderId": 900001 }
}
```

Go client: [docs/users/configuration.md](../../docs/users/configuration.md#mcp-client) (`mcp.StreamableClientTransport{Endpoint: "http://localhost:8080/mcp"}`).

REST equivalent of `tools/list` (no session): `curl -s http://localhost:8080/api/tools`.

MCP handshake: POST JSON-RPC `initialize`, copy `Mcp-Session-Id` from the response headers, then `tools/list`. The engine answers POST with SSE (`event: message` / `data: …`). `-dual` mounts `/mcp` behind the client bearer wrap; `initialize` and `tools/list` need `Authorization` but not `X-End-User-Token`.

```bash
curl -sS -D - -X POST http://localhost:8080/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "Authorization: Bearer $CLIENT" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"curl","version":"0"}}}'
```

```bash
curl -sS -X POST http://localhost:8080/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "Authorization: Bearer $CLIENT" \
  -H 'Mcp-Session-Id: <session>' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  | awk '/^data: /{print substr($0,7)}' | jq -r '.result.tools[].name'
```

Replace `<session>` with the `Mcp-Session-Id` value. Expect `run_petstore_v0.0.1`.

## Further reading

| Topic | Doc |
| --- | --- |
| `engine.Options`, listener vs `Handler()` | [Configuration](../../docs/users/configuration.md) |
| Loader, Executor, PolicyLoader, QueryMatcher | [Adapters](../../docs/users/adapters.md) |
| Catalog, MCP `run_*`, REST execute, spec rules | [Arazzo plans](../../docs/users/arazzo.md) |
| Which example to copy | [Examples](../../docs/users/examples.md) |
| This host, short form | [mcp-server/README.md](mcp-server/README.md) |
| Demo OAuth | [petstore-auth-server/README.md](petstore-auth-server/README.md) |
| Adapter HTTP | [async-order-server/README.md](async-order-server/README.md) |
| Docker Petstore | [petstore-openapi-server/README.md](petstore-openapi-server/README.md) |
