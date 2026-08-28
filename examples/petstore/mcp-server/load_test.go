// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mevansam/context-mesh-engine/arazzo"
	"github.com/mevansam/context-mesh-engine/engine"
	"github.com/mevansam/context-mesh-engine/examples/petstore/petstore-auth-server/jwtx"
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
	e.AddController(docsController{})

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
	if _, ok := props["pet"]; !ok {
		t.Fatalf("retrievePet 200 schema missing pet: %#v", schema)
	}
	if _, ok := props["inputs"]; ok {
		t.Fatalf("200 schema should be outputs, not trace: %#v", schema)
	}
	rb, _ := op["requestBody"].(map[string]any)
	rcontent, _ := rb["content"].(map[string]any)
	rapp, _ := rcontent["application/json"].(map[string]any)
	rschema, _ := rapp["schema"].(map[string]any)
	rprops, _ := rschema["properties"].(map[string]any)
	if _, ok := rprops["status"]; !ok {
		t.Fatalf("retrievePet request schema missing status: %#v", rschema)
	}
	if _, ok := rprops["policyHints"]; ok {
		t.Fatalf("consumer schema leaked policyHints: %#v", rschema)
	}
	if rschema["additionalProperties"] != false {
		t.Fatalf("retrievePet request schema should be closed: %#v", rschema)
	}
	if desc, _ := op["description"].(string); strings.Contains(desc, "policyHints") {
		t.Fatalf("description leaked policyHints: %s", desc)
	}

	docs, err := http.Get(ts.URL + "/api/docs")
	if err != nil {
		t.Fatal(err)
	}
	defer docs.Body.Close()
	if docs.StatusCode != http.StatusOK {
		t.Fatalf("docs status = %d", docs.StatusCode)
	}
	if ct := docs.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("docs Content-Type = %q", ct)
	}
}

func TestPetstoreCatalogRequiresClientJWT(t *testing.T) {
	secret := []byte("petstore-demo-hs256")
	bearer := clientBearer(secret)
	e, err := engine.New(engine.Options{
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		ArazzoLoaders:   []arazzo.Loader{arazzo.NewFileLoader(plansDir())},
		RESTHandlerWrap: func(h http.Handler) http.Handler { return wrapRESTPlans(h, bearer) },
	})
	if err != nil {
		t.Fatal(err)
	}
	e.AddController(docsController{})
	ts := httptest.NewServer(e.Handler())
	t.Cleanup(ts.Close)

	get := func(path string, hdr http.Header) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, ts.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		for k, vs := range hdr {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	health := get("/api/health", nil)
	health.Body.Close()
	if health.StatusCode != http.StatusOK {
		t.Fatalf("health = %d", health.StatusCode)
	}
	docs := get("/api/docs", nil)
	docs.Body.Close()
	if docs.StatusCode != http.StatusOK {
		t.Fatalf("docs = %d", docs.StatusCode)
	}
	unauth := get("/api/openapi/petstore", nil)
	unauth.Body.Close()
	if unauth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("openapi without token = %d, want 401", unauth.StatusCode)
	}
	toolsUnauth := get("/api/tools", nil)
	toolsUnauth.Body.Close()
	if toolsUnauth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("tools without token = %d, want 401", toolsUnauth.StatusCode)
	}

	tok, err := jwtx.SignClient(secret, "petstore-mcp", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	hdr := http.Header{"Authorization": []string{"Bearer " + tok}}
	authz := get("/api/openapi/petstore", hdr)
	authz.Body.Close()
	if authz.StatusCode != http.StatusOK {
		t.Fatalf("openapi with token = %d", authz.StatusCode)
	}
	tools := get("/api/tools", hdr)
	tools.Body.Close()
	if tools.StatusCode != http.StatusOK {
		t.Fatalf("tools with token = %d", tools.StatusCode)
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

func TestDecodeJSONBody_LargeInt(t *testing.T) {
	const id int64 = 9056269108963012608
	got := decodeJSONBody([]byte(`{"payload":{"orderId":9056269108963012608}}`))
	m, _ := got.(map[string]any)
	payload, _ := m["payload"].(map[string]any)
	if payload["orderId"] != id {
		t.Fatalf("orderId = %#v (%T)", payload["orderId"], payload["orderId"])
	}
}

func TestExecute_FallsBackOn404(t *testing.T) {
	v3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not on v3", http.StatusNotFound)
	}))
	t.Cleanup(v3.Close)
	v2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":9056269108963012608,"status":"placed"}`))
	}))
	t.Cleanup(v2.Close)

	e := &httpExec{
		client:   http.DefaultClient,
		petstore: []string{v3.URL, v2.URL},
	}
	resp, err := e.Execute(t.Context(), &arazzo.ExecutionRequest{
		OperationPath: "x#/paths/~1store~1order~11/get",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body=%v", resp.StatusCode, resp.Body)
	}
	body, _ := resp.Body.(map[string]any)
	if body["id"] != int64(9056269108963012608) {
		t.Fatalf("body = %#v", resp.Body)
	}
}

func TestResolvePetstoreBase(t *testing.T) {
	got, err := resolvePetstoreBase("local", "")
	if err != nil || got != petstoreLocalBase {
		t.Fatalf("local: %q %v", got, err)
	}
	got, err = resolvePetstoreBase("hosted", "")
	if err != nil || got != petstoreHostedBase {
		t.Fatalf("hosted: %q %v", got, err)
	}
	got, err = resolvePetstoreBase("hosted", "http://example:9/api/v3/")
	if err != nil || got != "http://example:9/api/v3" {
		t.Fatalf("override: %q %v", got, err)
	}
	if _, err := resolvePetstoreBase("other", ""); err == nil {
		t.Fatal("expected error for unknown -petstore")
	}
}
