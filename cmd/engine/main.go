// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mevansam/context-mesh-engine/arazzo"
	"github.com/mevansam/context-mesh-engine/engine"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	addr := flag.String("addr", engine.DefaultAddr, "HTTP listen address")
	apiPrefix := flag.String("api-prefix", engine.APIv1Prefix, "REST path prefix (health, plans, OpenAPI)")
	specs := flag.String("specs", "", "directory of Arazzo YAML/JSON plans (recursive)")
	publicBase := flag.String("public-base-url", "", "origin for MCP tool REST URLs (default http://<addr>)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opts := engine.Options{Addr: *addr, APIPrefix: *apiPrefix}
	if *specs != "" {
		opts.ArazzoLoaders = []arazzo.Loader{arazzo.NewFileLoader(*specs)}
		opts.PublicBaseURL = *publicBase
		if opts.PublicBaseURL == "" {
			opts.PublicBaseURL = "http://" + *addr
		}
	}

	e, err := engine.New(opts)
	if err != nil {
		log.Fatal(err)
	}
	mcp.AddTool(e.MCP(), &mcp.Tool{
		Name:        "ping",
		Description: "liveness probe for MCP",
	}, ping)

	log.Printf("listening on http://%s%s (MCP) and http://%s%s/health (REST)", *addr, engine.MCPPath, *addr, e.APIPrefix())
	if err := e.ListenAndServe(ctx); err != nil {
		log.Fatal(err)
	}
}

func ping(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "pong"}},
	}, nil, nil
}
