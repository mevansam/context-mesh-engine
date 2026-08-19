// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package arazzo_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mevansam/context-mesh-engine/arazzo"
)

func TestFileLoader_LoadsYAMLSkipsOthers(t *testing.T) {
	dir := filepath.Join("..", "testdata", "arazzo", "plans")
	srcs, err := arazzo.NewFileLoader(dir).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, s := range srcs {
		names[filepath.Base(s.URI)] = true
		if s.BaseURL == "" {
			t.Fatalf("%s: empty BaseURL", s.URI)
		}
		if s.BaseURL[len(s.BaseURL)-1] != '/' {
			t.Fatalf("%s: BaseURL %q must end with / so relative source URLs resolve as a directory", s.URI, s.BaseURL)
		}
	}
	if !names["petstore-v1.0.0.yaml"] || !names["petstore-v1.1.0.yaml"] || !names["no-plan-id.yaml"] {
		t.Fatalf("got %v", names)
	}
	if names["ignore.txt"] {
		t.Fatal("loaded ignore.txt")
	}
}

func TestFileLoader_MissingDir(t *testing.T) {
	_, err := arazzo.NewFileLoader(filepath.Join(os.TempDir(), "no-such-arazzo-dir")).Load(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRenderToolDoc_Defaults(t *testing.T) {
	ctx := arazzo.NewToolDocContext(
		"petstore", "1.1.0", "Pet Store Workflows", "summary", "desc",
		[]arazzo.WorkflowDoc{{ID: "pingHealth", Summary: "Check API health"}},
		"http://localhost:8080", "",
	)
	name, title, desc, err := arazzo.RenderToolDoc(arazzo.ToolDocTemplates{}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if name != "run_petstore_v1.1.0" {
		t.Fatalf("name = %q", name)
	}
	if title != "Pet Store Workflows (petstore v1.1.0)" {
		t.Fatalf("title = %q", title)
	}
	if want := "http://localhost:8080/api/v1/plans/petstore/v1.1.0/{workflowId}"; !strings.Contains(desc, want) {
		t.Fatalf("description missing %q:\n%s", want, desc)
	}
	if want := "GET http://localhost:8080/api/v1/openapi/petstore"; !strings.Contains(desc, want) {
		t.Fatalf("description missing %q:\n%s", want, desc)
	}
	if strings.Contains(desc, "{{") {
		t.Fatalf("unexpanded template in description:\n%s", desc)
	}
}

func TestRenderToolDoc_InvalidTemplate(t *testing.T) {
	_, _, _, err := arazzo.RenderToolDoc(arazzo.ToolDocTemplates{Name: "{{.Nope"}, arazzo.NewToolDocContext("p", "1", "t", "", "", nil, "", ""))
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestRenderQueryDoc_CustomPrefix(t *testing.T) {
	ctx := arazzo.NewToolDocContext("p", "1", "t", "", "", nil, "http://example.test", "/service/v2")
	name, title, desc, err := arazzo.RenderQueryDoc(arazzo.ToolDocTemplates{}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if name != "query" {
		t.Fatalf("name = %q", name)
	}
	if title != "Query plans" {
		t.Fatalf("title = %q", title)
	}
	if want := "POST http://example.test/service/v2/plans/query"; !strings.Contains(desc, want) {
		t.Fatalf("description missing %q:\n%s", want, desc)
	}
}

func TestSanitizeToolName(t *testing.T) {
	if got := arazzo.SanitizeToolName("run petstore/v1"); got != "run_petstore_v1" {
		t.Fatalf("got %q", got)
	}
}
