// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// Arazzo filesystem loader: MCP run_* tools and REST /api/v1/plans + /openapi.
package main

import (
	"context"
	"log"

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

func main() {
	e, err := engine.New(engine.Options{
		Addr: "localhost:8080",
		ArazzoLoaders: []arazzo.Loader{
			arazzo.NewFileLoader("testdata/arazzo/plans"),
		},
		ArazzoExecutor: okExec{},
		PublicBaseURL:  "http://localhost:8080",
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(e.ListenAndServe(context.Background()))
}
