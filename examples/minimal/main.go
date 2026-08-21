// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// Minimal engine: one MCP ping tool and GET /api/health on one listener.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/mevansam/context-mesh-engine/engine"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	dual := flag.Bool("dual", false, "serve both MCP and REST (default is REST only)")
	flag.Parse()

	e, err := engine.New(engine.Options{Addr: "localhost:8080", DualMCPandREST: *dual})
	if err != nil {
		log.Fatal(err)
	}
	mcp.AddTool(e.MCP(), &mcp.Tool{Name: "ping", Description: "liveness probe"}, ping)
	log.Fatal(e.ListenAndServe(context.Background()))
}

func ping(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "pong"}},
	}, nil, nil
}
