// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// Embed Handler() in your own http.Server when you already own lifecycle.
package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/mevansam/context-mesh-engine/engine"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	e, err := engine.New(engine.Options{})
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
