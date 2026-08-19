# context-mesh-engine

Go SDK that hosts an [MCP](https://modelcontextprotocol.io) Streamable HTTP server and a JSON REST API on **one HTTP listener**.

- MCP clients: `http://<addr>/mcp` (POST JSON-RPC, GET SSE, DELETE session).
- REST clients: `http://<addr>/api/v1/...` (JSON). Default `GET /api/v1/health`.
- Optional [Arazzo](https://spec.openapis.org/arazzo/latest.html) plans: MCP `run_*` tools and `POST /api/v1/plans/...`.

This module **embeds** the official [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk). It does not fork that SDK.

## Quick start

From a clone of this repository (sibling `go-sdk` and `libopenapi` checkouts; see [contributor getting started](docs/contributors/getting-started.md) if `go run` fails):

```bash
go run ./cmd/engine -addr localhost:8080
```

```bash
curl -s http://localhost:8080/api/v1/health
# {"status":"ok"}
```

MCP endpoint: `http://localhost:8080/mcp`.

## Documentation

Start at **[docs/README.md](docs/README.md)**. Use only the path that matches what you are doing:

| Path | Audience |
| --- | --- |
| [Use the SDK](docs/users/getting-started.md) | Application authors embedding `engine.Engine` |
| [Contribute to the SDK](docs/contributors/getting-started.md) | People changing this repository |

## License

MIT. See [LICENSE](LICENSE).
