// Copyright 2026 Fidelity Investments. All rights reserved.
// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

// Embed Handler() in your own http.Server when you already own lifecycle.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/mevansam/context-mesh-engine/engine"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	dual := flag.Bool("dual", false, "serve both MCP and REST (default is REST only)")
	flag.Parse()

	e, err := engine.New(engine.Options{DualMCPandREST: *dual})
	if err != nil {
		log.Fatal(err)
	}
	mcp.AddTool(e.MCP(), &mcp.Tool{Name: "ping", Description: "liveness probe"}, ping)

	srv := &http.Server{
		Addr:              "localhost:8080",
		Handler:           e.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}

func ping(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "pong"}},
	}, nil, nil
}
