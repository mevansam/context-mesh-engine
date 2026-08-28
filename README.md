# context-mesh-engine

Go SDK for an **engine** that turns governed [Arazzo](https://spec.openapis.org/arazzo/latest.html) workflow plans into two equivalent surfaces:

- **MCP tools** for agents (`/mcp`)
- **REST + OpenAPI** for ordinary HTTP clients (`/api` by default; `Options.APIPrefix`)

Both surfaces execute the **same** plan: a versioned, validated orchestration across the domain APIs of a [data mesh](https://martinfowler.com/articles/data-mesh-principles.html)—not an ad-hoc chain of tool calls invented at inference time.

It depends on the [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) for Streamable HTTP and on [libopenapi](https://github.com/pb33f/libopenapi) for Arazzo parse, validate, source resolution, and workflow execution.

## Why plans instead of letting the model call each API

A user query over a data mesh often cannot be answered from one backend. It needs a sequence of calls across independently owned domain APIs (orders, inventory, identity, billing, and so on), each with its own OpenAPI contract.

If you expose those operations as individual MCP tools (or leave the model to pick REST calls), the LLM must **invent** the orchestration: which APIs to hit, in what order, how to map fields, and when the answer is complete. That reasoning is statistical. Two runs of the same question can take different paths, skip a required step, or join data incorrectly. The model is doing workflow design at inference time.

Arazzo plans move that design **out of the model**. Authors publish a versioned document (`x-planId` + `info.version`) that names the steps, operations, inputs, and success criteria. The engine runs that document the same way every time and returns a structured trace. The LLM still interprets the user’s question and chooses a plan plus inputs; it does not decide how domain APIs are chained.


| Without a plan                                         | With an engine-executed plan                                      |
| ------------------------------------------------------ | ----------------------------------------------------------------- |
| Path through the mesh is inferred per request          | Path is the published workflow                                    |
| Order, mapping, and “done” can change between runs     | Same steps, criteria, and retries on every run                    |
| Failures are whatever the model tries next             | The runner applies the plan’s success and failure rules           |
| Agents and REST clients each re-implement composition  | One catalog and one runner: MCP `run_*` and `POST /api/plans/...` |
| Review means reading prompts and traces after the fact | Review means accepting a plan version before it can run           |


The unit of work is a **pre-built, validated, governed plan**, not unbounded tool-use against every domain operation.

## Why inbound and outbound policy

A published plan still runs for whoever can call `run_*` or `POST /api/plans/...`. Without a policy layer, **who may run which workflow** and **what may leave the engine** live in prompts, ad-hoc checks in the Executor, or nowhere. An agent can pick `purchasePet` as easily as `retrievePet`. A REST client can send a forged role. A successful step can return fields the caller should never see.

Optional [OPA](https://www.openpolicyagent.org/) modules attach to `(planId, version)` and run on **every** execute path (MCP `run_*`, REST, `query`). They are not prompts. They are versioned with the plan, default-deny, and fail closed if the bundle cannot load.

- **Inbound** (`data.plan.inbound`) runs **before** any step. Allow or deny; optional `hints` become `$inputs.policyHints` (caller-supplied `policyHints` are discarded). Deny is **403** and the workflow does not run. Hints are how policy constrains the plan (for example forcing a browse-only status) without putting that logic in the Arazzo document or trusting the model.
- **Outbound** (`data.plan.outbound`) runs **after** a successful workflow. Allow, redact JSON Pointers in the outputs, or replace the outputs map. Deny is **403** and outputs are **not** returned—even though the steps already ran. That is data-minimization and leak-stop, not a rollback of backends.

| Without policy | With inbound / outbound OPA |
| --- | --- |
| Any caller who can hit execute can run any loaded workflow | Allow lists are evaluated per `workflowId`, identity, and inputs |
| “Don’t purchase unless you are a buyer” is a prompt | Inbound deny is 403; the Executor is never called |
| Plan inputs are whatever the client or model sent | Hints are policy-authored; they cannot be forged on the request |
| Full workflow outputs go back to the agent or HTTP client | Outbound redact/replace strips fields (PII, photos, secrets) before return |
| MCP and REST can drift if you check auth in only one adapter | One runner: the same modules wrap both surfaces |
| A bad Executor or over-broad plan leaks on a 200 | Outbound is a second gate after the plan succeeds |
| Policy load failure might skip checks | Load/compile errors are **500** unless a compiled bundle is still cached |

Plans define **how** domain APIs are chained. Policy defines **who** may run that chain and **what** may leave the process. Together they keep orchestration deterministic **and** access governed—without asking the model to police itself.

## What you run

One process, one TCP port:


| Surface      | Where                          | Role                                                                                                                                                  |
| ------------ | ------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| MCP `query`  | `/mcp`                         | Natural-language entry, registered only when `QueryMatcher` is set. The matcher selects a plan; the engine runs it if that plan is loaded here.       |
| MCP `run_*`  | `/mcp`                         | Direct execute of a known plan version. Arguments: `workflowId` + `inputs`.                                                                           |
| REST `query` | `POST /api/plans/query`        | Same as MCP `query` (omitted without `QueryMatcher`). JSON body is `{ "query": "...", "data": { } }`. Success payload is the workflow outputs object. |
| REST execute | `POST /api/plans/{planId}/...` | Same as MCP `run_*`; JSON body is the workflow inputs.                                                                                                |
| REST OpenAPI | `GET /api/openapi`             | Catalog OAS 3.1 (`GET /tools` + `$ref` to each latest plan spec). Per-plan: `GET /api/openapi/{planId}`.                                              |
| REST tools   | `GET /api/tools`               | MCP `tools/list` envelope (`ttlMs`, `cacheScope`, `tools`); Arazzo descriptions are REST-specific.                                                    |


`query` (MCP or REST) is for when the caller should not pick a `run_*` tool or execute URL itself. Matching stays inside the registry of **pre-built plans**; the model still does not compose domain API calls. `run_`* and `POST /api/plans/{planId}/...` are for when the plan and version are already known.

You supply **loaders**, an **Executor** for domain HTTP, optionally a **PolicyLoader** for inbound/outbound OPA, and optionally a **QueryMatcher** for natural-language plan selection. The engine loads plans, exposes tools and OpenAPI, evaluates policy around `Run`, and executes steps through libopenapi’s Arazzo engine. Nil `PolicyLoader` skips those checks.

## Quick start

```bash
go run ./cmd/engine -addr localhost:8080
```

```bash
curl -s http://localhost:8080/api/health
# {"status":"ok"}
curl -s http://localhost:8080/api/tools
```

Plans + a stub executor: [examples/arazzo-fs](examples/arazzo-fs/README.md). Live Petstore demo: [examples/petstore](examples/petstore/README.md). MCP Streamable HTTP at `/mcp` requires `-dual` on the examples (or `DualMCPandREST` / `MCPOnly` when embedding). Default is REST only.

## Documentation

**[docs/README.md](docs/README.md)** — pick one path:


| Path                                               | Audience                                         |
| -------------------------------------------------- | ------------------------------------------------ |
| [Use the SDK](docs/users/getting-started.md)       | Embed `engine.Engine`; configuration, adapters, Arazzo |
| [Contribute](docs/contributors/getting-started.md) | Change this repository                           |




## License

Apache 2.0. See [LICENSE](LICENSE).