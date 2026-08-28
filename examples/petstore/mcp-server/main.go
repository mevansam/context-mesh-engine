// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

// Petstore MCP + REST host built on context-mesh-engine.
//
// The engine owns catalog load, MCP run_* tools, REST /plans, OPA eval order
// (inbound → workflow → outbound), and the HTTP mux. This process supplies
// the adapters in engine.Options. Files:
//
//	main.go     — engine.New wiring (copy this)
//	docs.go     — GET /docs Swagger UI (AddController)
//	auth.go     — MCPHandlerWrap / RESTHandlerWrap / RequestPreprocessor
//	executor.go — ArazzoExecutor (HTTP to Petstore + async adapter)
//
// Nil QueryMatcher: query is not registered. Default listen is REST only;
// -dual also mounts Streamable HTTP at engine.MCPPath (/mcp).
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/mevansam/context-mesh-engine/arazzo"
	"github.com/mevansam/context-mesh-engine/engine"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "MCP/REST listen address")
	asyncURL := flag.String("async-order-url", defaultAsyncBase, "async-order-server origin")
	petstore := flag.String("petstore", "local", "Petstore 3 target: local (Docker on :8090) or hosted (petstore3.swagger.io)")
	petstoreURL := flag.String("petstore-url", "", "override Petstore 3 OpenAPI origin")
	jwtSecret := flag.String("jwt-secret", "petstore-demo-hs256", "HS256 secret shared with petstore-auth-server")
	dual := flag.Bool("dual", false, "serve both MCP and REST (default is REST only)")
	flag.Parse()

	petstoreBase, err := resolvePetstoreBase(*petstore, *petstoreURL)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	e, err := engine.New(hostOptions(*addr, *asyncURL, petstoreBase, *jwtSecret, *dual))
	if err != nil {
		log.Fatal(err)
	}
	e.AddController(docsController{})
	docsURL := "http://" + *addr + e.APIPrefix() + "/docs"
	if *dual {
		log.Printf("MCP http://%s%s  REST http://%s%s  docs %s  async %s  petstore %s",
			*addr, engine.MCPPath, *addr, e.APIPrefix(), docsURL, *asyncURL, petstoreBase)
	} else {
		log.Printf("REST http://%s%s  docs %s  async %s  petstore %s",
			*addr, e.APIPrefix(), docsURL, *asyncURL, petstoreBase)
	}
	if err := e.ListenAndServe(ctx); err != nil {
		log.Fatal(err)
	}
}

// hostOptions is the engine.Options this example sets. Unset fields keep
// engine defaults (REST-only mux, no query matcher, no SecretInputs).
func hostOptions(addr, asyncURL, petstoreBase, jwtSecret string, dual bool) engine.Options {
	secret := []byte(jwtSecret)

	// HMAC used by SecretsProvider and by httpExec to mint a downstream JWT.
	// SecretInputs is left unset so the engine does not copy this onto $inputs.
	secrets := arazzo.MapSecrets{downstreamHMACSecret: jwtSecret}

	// go-sdk RequireBearerToken: verifies Authorization. The engine does not
	// parse JWTs; this wrap is MCPHandlerWrap / the inner RESTHandlerWrap.
	bearer := clientBearer(secret)

	return engine.Options{
		Addr: addr,

		// Loader: parse Arazzo from plans/. Engine indexes (x-planId, version),
		// resolves sourceDescriptions, registers run_* and POST /plans/{planId}/….
		ArazzoLoaders: []arazzo.Loader{
			arazzo.NewFileLoader(plansDir()),
		},

		// Executor: without this, catalog and GET /openapi still work; execute is 501.
		// Each Arazzo step is one Execute call (see httpExec).
		ArazzoExecutor: newHTTPExec(asyncURL, petstoreBase, secrets),

		// PolicyLoader: engine evals data.plan.inbound before any step and
		// data.plan.outbound after. .rego is not passed to FileLoader.
		// Data is Rego data.* (optional); inbound uses JWT claims, not http.send.
		PolicyLoader: &arazzo.FilePolicyLoader{
			Dir:  policiesDir(),
			Data: map[string]any{"petstoreBase": petstoreBase},
		},

		// RequestPreprocessor: extra JWTs (X-End-User-Token) and OPA input.auth.
		// Runs on execute/query before inbound. Error → 401. Nil would skip this.
		RequestPreprocessor: &dualJWTPreprocessor{secret: secret},

		// SecretsProvider: named secrets for the Executor. Empty SecretInputs
		// means the engine will not flatten them onto $inputs.secrets.*.
		SecretsProvider: secrets,

		// Origin written into GET /api/tools REST descriptions (not inferred from Addr).
		PublicBaseURL: "http://" + addr,

		// All false = REST only. DualMCPandREST also mounts /mcp.
		DualMCPandREST: dual,

		// Wraps apply to child handlers only, never the root mux.
		// MCP: every Streamable HTTP request (initialize, tools/list, run_*).
		// REST: wrapRESTPlans requires bearer on GET /tools, GET /openapi/…, and POST /plans/.
		// GET /health and GET /docs stay open.
		MCPHandlerWrap:  bearer,
		RESTHandlerWrap: func(h http.Handler) http.Handler { return wrapRESTPlans(h, bearer) },
	}
}

// plansDir is plans/ next to this source file. runtime.Caller records the
// compile-time path of main.go, so go run / go test work from any cwd.
func plansDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("cannot resolve path of main.go")
	}
	return filepath.Join(filepath.Dir(file), "plans")
}

func policiesDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("cannot resolve path of main.go")
	}
	return filepath.Join(filepath.Dir(file), "policies")
}
