// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// Petstore MCP + REST engine: Arazzo plans over petstore3 and the async order adapter.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/mevansam/context-mesh-engine/arazzo"
	"github.com/mevansam/context-mesh-engine/engine"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "MCP/REST listen address")
	asyncURL := flag.String("async-order-url", defaultAsyncBase, "async-order-server origin")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	e, err := engine.New(engine.Options{
		Addr: *addr,
		ArazzoLoaders: []arazzo.Loader{
			arazzo.NewFileLoader(plansDir()),
		},
		ArazzoExecutor: newHTTPExec(*asyncURL),
		PublicBaseURL:  "http://" + *addr,
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("MCP http://%s%s  REST http://%s%s  async %s  petstore %s",
		*addr, engine.MCPPath, *addr, e.APIPrefix(), *asyncURL, petstore3Base)
	if err := e.ListenAndServe(ctx); err != nil {
		log.Fatal(err)
	}
}

func plansDir() string {
	for _, p := range []string{
		filepath.Join("examples", "petstore", "mcp-server", "plans"),
		"plans",
	} {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return filepath.Join("examples", "petstore", "mcp-server", "plans")
}
