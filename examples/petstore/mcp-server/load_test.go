// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package main

import (
	"io"
	"log/slog"
	"testing"

	"github.com/mevansam/context-mesh-engine/arazzo"
	"github.com/mevansam/context-mesh-engine/engine"
)

func TestPetstorePlanLoads(t *testing.T) {
	e, err := engine.New(engine.Options{
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		ArazzoLoaders: []arazzo.Loader{arazzo.NewFileLoader("plans")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.APIPrefix() != engine.APIv1Prefix {
		t.Fatalf("prefix = %s", e.APIPrefix())
	}
}

func TestParseOperationPath(t *testing.T) {
	path, method, ok := parseOperationPath("{$sourceDescriptions.petStoreDescription.url}#/paths/~1pet~1findByStatus/get")
	if !ok || path != "/pet/findByStatus" || method != "get" {
		t.Fatalf("got path=%q method=%q ok=%v", path, method, ok)
	}
}

func TestLastSegment(t *testing.T) {
	if got := lastSegment("$sourceDescriptions.asyncOrderApiDescription.placeOrder"); got != "placeOrder" {
		t.Fatalf("got %q", got)
	}
}

func TestIsAsyncSource(t *testing.T) {
	req := &arazzo.ExecutionRequest{OperationPath: "$sourceDescriptions.asyncOrderApiDescription.placeOrder"}
	if !isAsyncSource(req) {
		t.Fatal("expected async source")
	}
}
