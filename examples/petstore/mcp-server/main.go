// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// Petstore MCP + REST engine: Arazzo plans over local Petstore 3 and the async order adapter.
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
	petstoreURL := flag.String("petstore-url", defaultPetstoreBase, "Petstore 3 OpenAPI origin (local Docker)")
	dual := flag.Bool("dual", false, "serve both MCP and REST (default is REST only)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	e, err := engine.New(engine.Options{
		Addr: *addr,
		ArazzoLoaders: []arazzo.Loader{
			arazzo.NewFileLoader(plansDir()),
		},
		ArazzoExecutor: newHTTPExec(*asyncURL, *petstoreURL),
		PublicBaseURL:  "http://" + *addr,
		DualMCPandREST: *dual,
	})
	if err != nil {
		log.Fatal(err)
	}
	if *dual {
		log.Printf("MCP http://%s%s  REST http://%s%s  async %s  petstore %s",
			*addr, engine.MCPPath, *addr, e.APIPrefix(), *asyncURL, *petstoreURL)
	} else {
		log.Printf("REST http://%s%s  async %s  petstore %s",
			*addr, e.APIPrefix(), *asyncURL, *petstoreURL)
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
