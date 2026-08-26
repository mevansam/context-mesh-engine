// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package arazzo

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFilePolicyLoader_InboundOutboundAndData(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "petstore", "0.0.1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "inbound.rego"), []byte("package plan.inbound\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "outbound.rego"), []byte("package plan.outbound\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data.json"), []byte(`{"petstoreBase":"http://file"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	l := &FilePolicyLoader{Dir: root, Data: map[string]any{"petstoreBase": "http://host"}}
	b, err := l.Load(context.Background(), PolicyRequest{PlanID: "petstore", Version: "0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if !b.HasInbound() || !b.HasOutbound() {
		t.Fatalf("bundle = %#v", b)
	}
	var data map[string]any
	if err := json.Unmarshal(b.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data["petstoreBase"] != "http://host" {
		t.Fatalf("data = %#v, host overlay should win", data)
	}
}

func TestFilePolicyLoader_InboundOnly(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "p", "1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "inbound.rego"), []byte("package plan.inbound\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := NewFilePolicyLoader(root).Load(context.Background(), PolicyRequest{PlanID: "p", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if !b.HasInbound() || b.HasOutbound() {
		t.Fatalf("bundle = %#v", b)
	}
}

func TestFilePolicyLoader_MissingIsNil(t *testing.T) {
	b, err := NewFilePolicyLoader(t.TempDir()).Load(context.Background(), PolicyRequest{PlanID: "p", Version: "1"})
	if err != nil || b != nil {
		t.Fatalf("got %#v %v", b, err)
	}
}

func TestFilePolicyLoader_EmptyDirIsNil(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "p", "1"), 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := NewFilePolicyLoader(root).Load(context.Background(), PolicyRequest{PlanID: "p", Version: "1"})
	if err != nil || b != nil {
		t.Fatalf("got %#v %v", b, err)
	}
}

func TestFilePolicyLoader_RejectsUnsafeSegments(t *testing.T) {
	l := NewFilePolicyLoader(t.TempDir())
	for _, req := range []PolicyRequest{
		{PlanID: "../x", Version: "1"},
		{PlanID: "p/q", Version: "1"},
		{PlanID: "p", Version: ".."},
		{PlanID: "", Version: "1"},
	} {
		if _, err := l.Load(context.Background(), req); err == nil {
			t.Fatalf("expected error for %+v", req)
		}
	}
}
