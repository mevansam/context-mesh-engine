// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mevansam/context-mesh-engine/arazzo"
	"github.com/mevansam/context-mesh-engine/engine"
)

func TestPetstorePlanLoads(t *testing.T) {
	e, err := engine.New(engine.Options{
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		ArazzoLoaders: []arazzo.Loader{arazzo.NewFileLoader(plansDir())},
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.APIPrefix() != engine.DefaultAPIPrefix {
		t.Fatalf("prefix = %s", e.APIPrefix())
	}

	ts := httptest.NewServer(e.Handler())
	t.Cleanup(ts.Close)
	resp, err := http.Get(ts.URL + "/api/openapi/petstore")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("openapi status = %d", resp.StatusCode)
	}
	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	paths, _ := doc["paths"].(map[string]any)
	post, _ := paths["/plans/petstore/retrievePet"].(map[string]any)
	op, _ := post["post"].(map[string]any)
	responses, _ := op["responses"].(map[string]any)
	ok200, _ := responses["200"].(map[string]any)
	content, _ := ok200["content"].(map[string]any)
	appJSON, _ := content["application/json"].(map[string]any)
	schema, _ := appJSON["schema"].(map[string]any)
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["petId"]; !ok {
		t.Fatalf("retrievePet 200 schema missing petId: %#v", schema)
	}
	if _, ok := props["inputs"]; ok {
		t.Fatalf("200 schema should be outputs, not trace: %#v", schema)
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
