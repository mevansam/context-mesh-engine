// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

// Petstore MCP + REST engine: Arazzo plans over Petstore 3 (local Docker or hosted) and the async order adapter.
package main

import (
	"context"
	"flag"
	"log"
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
	dual := flag.Bool("dual", false, "serve both MCP and REST (default is REST only)")
	flag.Parse()

	petstoreBase, err := resolvePetstoreBase(*petstore, *petstoreURL)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	e, err := engine.New(engine.Options{
		Addr: *addr,
		ArazzoLoaders: []arazzo.Loader{
			arazzo.NewFileLoader(plansDir()),
		},
		ArazzoExecutor: newHTTPExec(*asyncURL, petstoreBase),
		PolicyLoader: &arazzo.FilePolicyLoader{
			Dir:  policiesDir(),
			Data: map[string]any{"petstoreBase": petstoreBase},
		},
		PublicBaseURL:  "http://" + *addr,
		DualMCPandREST: *dual,
	})
	if err != nil {
		log.Fatal(err)
	}
	if *dual {
		log.Printf("MCP http://%s%s  REST http://%s%s  async %s  petstore %s",
			*addr, engine.MCPPath, *addr, e.APIPrefix(), *asyncURL, petstoreBase)
	} else {
		log.Printf("REST http://%s%s  async %s  petstore %s",
			*addr, e.APIPrefix(), *asyncURL, petstoreBase)
	}
	if err := e.ListenAndServe(ctx); err != nil {
		log.Fatal(err)
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
