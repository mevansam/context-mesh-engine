// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package api

import (
	"encoding/json"
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
