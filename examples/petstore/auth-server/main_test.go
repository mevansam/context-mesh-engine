// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mevansam/context-mesh-engine/examples/petstore/auth-server/jwtx"
)

func TestClientCredentialsScopes(t *testing.T) {
	s := &authServer{
		jwtSecret:    []byte("petstore-demo-hs256"),
		clientID:     defaultClientID,
		clientSecret: defaultClientSecret,
	}

	post := func(body string) *http.Response {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.handleToken(rec, req)
		return rec.Result()
	}

	resp := post(`{"grant_type":"client_credentials","client_id":"petstore-mcp","client_secret":"mcp-secret"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("default status = %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["scope"] != jwtx.JoinScope(jwtx.DefaultClientScopes) {
		t.Fatalf("default scope = %#v", out["scope"])
	}
	tok, _ := out["access_token"].(string)
	cc, err := jwtx.ParseClient(s.jwtSecret, tok)
	if err != nil {
		t.Fatal(err)
	}
	if cc.Scope != jwtx.JoinScope(jwtx.DefaultClientScopes) {
		t.Fatalf("jwt scope = %q", cc.Scope)
	}

	resp = post(`{"grant_type":"client_credentials","client_id":"petstore-mcp","client_secret":"mcp-secret","scope":"pets:read"}`)
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["scope"] != "pets:read" {
		t.Fatalf("requested scope = %#v", out["scope"])
	}

	resp = post(`{"grant_type":"client_credentials","client_id":"petstore-mcp","client_secret":"mcp-secret","scope":"admin"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid_scope status = %d", resp.StatusCode)
	}
}
