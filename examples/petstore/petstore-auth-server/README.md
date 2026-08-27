# petstore-auth-server

Demo OAuth token endpoint for the petstore host. It is **not** a full authorization server. It issues HS256 JWTs that `mcp-server` verifies.

| Grant | Purpose | JWT `token_use` |
| --- | --- | --- |
| `client_credentials` | Calling application (`Authorization: Bearer`) | `client` |
| `password` | End user (`X-End-User-Token`) | `user` |

Password grant calls Petstore [loginUser](https://petstore3.swagger.io/#/user/loginUser) (`GET /user/login`) then [getUserByName](https://petstore3.swagger.io/#/user/getUserByName) (`GET /user/{username}`) so `userStatus` is in the user JWT. Inbound OPA reads that claim; it does not `http.send` to Petstore.

Default listen: `localhost:8092`. Same `-petstore` / `-petstore-url` flags as the other Go processes. Shared HMAC: `-jwt-secret` (default `petstore-demo-hs256`). Must match `mcp-server`.

```bash
go run ./examples/petstore/petstore-auth-server
```

```bash
# application token
curl -s -X POST http://localhost:8092/oauth/token \
  -H 'Content-Type: application/json' \
  -d '{"grant_type":"client_credentials","client_id":"petstore-mcp","client_secret":"mcp-secret"}'

# end-user token (seed Petstore users first)
curl -s -X POST http://localhost:8092/oauth/token \
  -H 'Content-Type: application/json' \
  -d '{"grant_type":"password","username":"buyer","password":"abc123"}'
```

How to run this with `mcp-server`: **[../README.md](../README.md)**.
