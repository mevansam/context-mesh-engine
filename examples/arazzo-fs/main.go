// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// Arazzo filesystem loader: MCP run_* tools and REST /api/plans + /openapi.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/mevansam/context-mesh-engine/arazzo"
	"github.com/mevansam/context-mesh-engine/engine"
)

type okExec struct{}

func (okExec) Execute(_ context.Context, _ *arazzo.ExecutionRequest) (*arazzo.ExecutionResponse, error) {
	return &arazzo.ExecutionResponse{
		StatusCode: 200,
		Body:       map[string]any{"status": "ok"},
	}, nil
}

// pingMatcher is a stand-in for an external semantic index: it always
// selects the sample petstore pingHealth workflow. The engine still
// checks that plan is loaded here.
type pingMatcher struct{}

func (pingMatcher) Match(_ context.Context, req arazzo.QueryRequest) (*arazzo.QueryMatch, error) {
	return &arazzo.QueryMatch{
		PlanID:     "petstore",
		WorkflowID: "pingHealth",
		Inputs:     req.Data,
	}, nil
}

func main() {
	dual := flag.Bool("dual", false, "serve both MCP and REST (default is REST only)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [-dual] <plans-dir>\n", filepath.Base(os.Args[0]))
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	plansDir := flag.Arg(0)

	e, err := engine.New(engine.Options{
		Addr: "localhost:8080",
		ArazzoLoaders: []arazzo.Loader{
			arazzo.NewFileLoader(plansDir),
		},
		ArazzoExecutor: okExec{},
		QueryMatcher:   pingMatcher{},
		PublicBaseURL:  "http://localhost:8080",
		DualMCPandREST: *dual,
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(e.ListenAndServe(context.Background()))
}
