// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"reflect"
	"testing"
)

func TestApplyRedact(t *testing.T) {
	in := map[string]any{
		"pet": map[string]any{"id": 1.0, "name": "a", "tag/x": "y"},
		"items": []any{
			map[string]any{"n": "one"},
			map[string]any{"n": "two"},
		},
	}
	out, err := applyRedact(in, []string{"/pet/id", "/items/1/n", "/pet/tag~1x", "/missing"}, "***")
	if err != nil {
		t.Fatal(err)
	}
	pet := out["pet"].(map[string]any)
	if pet["id"] != "***" || pet["name"] != "a" || pet["tag/x"] != "***" {
		t.Fatalf("pet = %#v", pet)
	}
	items := out["items"].([]any)
	if items[0].(map[string]any)["n"] != "one" || items[1].(map[string]any)["n"] != "***" {
		t.Fatalf("items = %#v", items)
	}
	if in["pet"].(map[string]any)["id"] != 1.0 {
		t.Fatal("original mutated")
	}
}

func TestApplyRedact_DefaultMaskNull(t *testing.T) {
	out, err := applyRedact(map[string]any{"a": 1.0}, []string{"/a"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out["a"] != nil {
		t.Fatalf("a = %#v", out["a"])
	}
}

func TestApplyRedact_Malformed(t *testing.T) {
	for _, p := range []string{"no-slash", "/~", "/~2", ""} {
		if _, err := applyRedact(map[string]any{"a": 1}, []string{p}, nil); err == nil {
			t.Fatalf("pointer %q should fail", p)
		}
	}
}

func TestCloneJSONMap_Empty(t *testing.T) {
	got := cloneJSONMap(nil)
	if !reflect.DeepEqual(got, map[string]any{}) {
		t.Fatalf("got %#v", got)
	}
}

func TestApplyRedact_PreservesInt64(t *testing.T) {
	const id int64 = 9056269108963012608
	in := map[string]any{
		"orderId": id,
		"nested":  map[string]any{"n": id},
		"secret":  "x",
	}
	out, err := applyRedact(in, []string{"/secret"}, "***")
	if err != nil {
		t.Fatal(err)
	}
	if out["orderId"] != id {
		t.Fatalf("orderId = %#v (%T)", out["orderId"], out["orderId"])
	}
	if out["nested"].(map[string]any)["n"] != id {
		t.Fatalf("nested = %#v", out["nested"])
	}
	if in["secret"] != "x" {
		t.Fatal("original mutated")
	}
}
