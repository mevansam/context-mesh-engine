// Copyright 2026 Fidelity Investments. All rights reserved.
// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCanonicalJSON_LargeInt(t *testing.T) {
	const raw = `{"orderId":9056269108963012608}`
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	got := CanonicalJSON(m).(map[string]any)
	id, ok := got["orderId"].(int64)
	if !ok || id != 9056269108963012608 {
		t.Fatalf("orderId = %#v (%T)", got["orderId"], got["orderId"])
	}
}

func TestDecodeMap_LargeInt(t *testing.T) {
	got, err := DecodeMap(strings.NewReader(`{"orderId":9056269108963012608,"name":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	id, ok := got["orderId"].(int64)
	if !ok || id != 9056269108963012608 {
		t.Fatalf("orderId = %#v (%T)", got["orderId"], got["orderId"])
	}
	if got["name"] != "x" {
		t.Fatalf("name = %#v", got["name"])
	}
}

func TestCanonicalJSON_FloatSliceAndString(t *testing.T) {
	got := CanonicalJSON([]any{json.Number("1.5"), json.Number("2"), map[string]any{"n": json.Number("3")}}).([]any)
	if got[0] != 1.5 {
		t.Fatalf("float = %#v", got[0])
	}
	if got[1] != int64(2) {
		t.Fatalf("int = %#v", got[1])
	}
	m := got[2].(map[string]any)
	if m["n"] != int64(3) {
		t.Fatalf("nested = %#v", m["n"])
	}
	if CanonicalJSON("keep") != "keep" {
		t.Fatal("passthrough")
	}
	if CanonicalJSON(json.Number("not-a-number")) != "not-a-number" {
		t.Fatal("invalid number should stay string")
	}
}

func TestReadJSON_OKAndUnknownField(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"error":"x"}`))
	var body ErrorBody
	if err := ReadJSON(req, &body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "x" {
		t.Fatalf("error = %q", body.Error)
	}
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"nope":1}`))
	if err := ReadJSON(req, &body); err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestDecodeMap_InvalidAndNull(t *testing.T) {
	if _, err := DecodeMap(strings.NewReader(`[`)); err == nil {
		t.Fatal("expected decode error")
	}
	got, err := DecodeMap(strings.NewReader(`null`))
	if err != nil || got != nil {
		t.Fatalf("null = %#v err=%v", got, err)
	}
}

func TestReadJSON_EmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Body = nil
	var got map[string]any
	if err := ReadJSON(req, &got); err == nil {
		t.Fatal("expected empty body error")
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, http.StatusBadRequest, "nope")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"error":"nope"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
