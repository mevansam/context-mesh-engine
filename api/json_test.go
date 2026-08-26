// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mevansam/context-mesh-engine/api"
)

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	api.WriteJSON(rec, http.StatusCreated, api.HealthResponse{Status: "ok"})

	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want %d", rec.Code, http.StatusCreated)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q", ct)
	}

	var got api.HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "ok" {
		t.Fatalf("status = %q", got.Status)
	}
}

func TestReadJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"status":"ok"}`))
	var got api.HealthResponse
	if err := api.ReadJSON(httptest.NewRecorder(), req, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "ok" {
		t.Fatalf("status = %q", got.Status)
	}
}

func TestReadJSONUnknownField(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"nope":1}`))
	var got api.HealthResponse
	if err := api.ReadJSON(httptest.NewRecorder(), req, &got); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	api.WriteError(rec, http.StatusBadRequest, "nope")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d", rec.Code)
	}
	var body api.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "nope" {
		t.Fatalf("error = %q", body.Error)
	}
}
