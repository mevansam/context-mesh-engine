# context-mesh-engine

Go SDK for an **engine** that turns governed [Arazzo](https://spec.openapis.org/arazzo/latest.html) workflow plans into two equivalent surfaces:

- **MCP tools** for agents (`/mcp`)
- **REST + OpenAPI** for ordinary HTTP clients (`/api/v1`)

Both surfaces execute the **same** plan: a versioned, validated orchestration across the domain APIs of a [data mesh](https://martinfowler.com/articles/data-mesh-principles.html)—not an ad-hoc chain of tool calls invented at inference time.

It depends on the [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) for Streamable HTTP and on [libopenapi](https://github.com/pb33f/libopenapi) for Arazzo parse, validate, source resolution, and workflow execution.

## Why plans instead of letting the model call each API

A user query over a data mesh often cannot be answered from one backend. It needs a sequence of calls across independently owned domain APIs (orders, inventory, identity, billing, and so on), each with its own OpenAPI contract.

If you expose those operations as individual MCP tools (or leave the model to pick REST calls), the LLM must **invent** the orchestration: which APIs to hit, in what order, how to map fields, and when the answer is complete. That reasoning is statistical. Two runs of the same question can take different paths, skip a required step, or join data incorrectly. The model is doing workflow design at inference time.

Arazzo plans move that design **out of the model**. Authors publish a versioned document (`x-planId` + `info.version`) that names the steps, operations, inputs, and success criteria. The engine runs that document the same way every time and returns a structured trace. The LLM still interprets the user’s question and chooses a plan plus inputs; it does not decide how domain APIs are chained.

| Without a plan | With an engine-executed plan |
| --- | --- |
| Path through the mesh is inferred per request | Path is the published workflow |
| Order, mapping, and “done” can change between runs | Same steps, criteria, and retries on every run |
| Failures are whatever the model tries next | The runner applies the plan’s success and failure rules |
| Agents and REST clients each re-implement composition | One catalog and one runner: MCP `run_*` and `POST /api/v1/plans/...` |
| Review means reading prompts and traces after the fact | Review means accepting a plan version before it can run |

The unit of work is a **pre-built, validated, governed plan**, not unbounded tool-use against every domain operation.

## What you run

One process, one TCP port:

| Surface | Where | Role |
| --- | --- | --- |
| MCP `query` | `/mcp` | Natural-language entry over the plan registry. The caller sends a **simple, direct** question plus a clear outline of the inputs they have. The engine semantically matches that against loaded plans, selects one, and executes it. |
| MCP `run_*` | `/mcp` | Direct execute of a known plan version. Arguments: `workflowId` + `inputs`. |
| REST `query` | `POST /api/v1/plans/query` | Same as MCP `query`: natural-language match + execute. JSON body is `{ "query": "...", "data": { } }`. Success payload is the same result object as execute. |
| REST execute | `POST /api/v1/plans/{planId}/...` | Same as MCP `run_*`; JSON body is the workflow inputs. |
| REST OpenAPI | `GET /api/v1/openapi/{planId}` | Generated OAS 3.1 for those execute paths (latest or a specific version). |

`query` (MCP or REST) is for when the caller should not pick a `run_*` tool or execute URL itself. Matching stays inside the registry of **pre-built plans**; the model still does not compose domain API calls. `run_*` and `POST /api/v1/plans/{planId}/...` are for when the plan and version are already known.

You supply **loaders** (filesystem or your own) and an **Executor** that performs the actual domain HTTP calls. The engine loads plans, exposes tools and OpenAPI, and runs steps through libopenapi’s Arazzo engine.

## Quick start

```bash
go run ./cmd/engine -addr localhost:8080
```

```bash
curl -s http://localhost:8080/api/v1/health
# {"status":"ok"}
```

Plans + a stub executor: `go run ./examples/arazzo-fs` (from the repository root). MCP: `http://localhost:8080/mcp`.

## Documentation

**[docs/README.md](docs/README.md)** — pick one path:

| Path | Audience |
| --- | --- |
| [Use the SDK](docs/users/getting-started.md) | Embed `engine.Engine`; MCP, REST, Arazzo options |
| [Contribute](docs/contributors/getting-started.md) | Change this repository |

## License

MIT. See [LICENSE](LICENSE).
