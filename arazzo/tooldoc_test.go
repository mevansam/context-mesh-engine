// Copyright 2026 Fidelity Investments. All rights reserved.
// Use of this source code is governed by the Apache 2.0 license
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
	doc, err := arazzo.RenderToolDoc(arazzo.ToolDocTemplates{}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Name != "run_petstore_v1.1.0" {
		t.Fatalf("name = %q", doc.Name)
	}
	if doc.Title != "Pet Store Workflows (petstore v1.1.0)" {
		t.Fatalf("title = %q", doc.Title)
	}
	if strings.Contains(doc.Description, "{{") {
		t.Fatalf("unexpanded template in MCP description:\n%s", doc.Description)
	}
	if strings.Contains(doc.Description, "POST ") || strings.Contains(doc.Description, "/api/plans/") || strings.Contains(doc.Description, "/api/openapi/") {
		t.Fatalf("MCP description must not mention REST:\n%s", doc.Description)
	}
	if !strings.Contains(doc.Description, "How to call this MCP tool") {
		t.Fatalf("MCP description missing how-to:\n%s", doc.Description)
	}
	if strings.Contains(strings.ToLower(doc.RESTDescription), "mcp") {
		t.Fatalf("REST description must not mention MCP:\n%s", doc.RESTDescription)
	}
	if want := "POST http://localhost:8080/api/plans/petstore/v1.1.0/{workflowId}"; !strings.Contains(doc.RESTDescription, want) {
		t.Fatalf("REST description missing %q:\n%s", want, doc.RESTDescription)
	}
	if want := "GET http://localhost:8080/api/openapi/petstore"; !strings.Contains(doc.RESTDescription, want) {
		t.Fatalf("REST description missing %q:\n%s", want, doc.RESTDescription)
	}
}

func TestRenderToolDoc_InvalidTemplate(t *testing.T) {
	_, err := arazzo.RenderToolDoc(arazzo.ToolDocTemplates{Name: "{{.Nope"}, arazzo.NewToolDocContext("p", "1", "t", "", "", nil, "", ""))
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestRenderToolDoc_InvalidRESTDescription(t *testing.T) {
	_, err := arazzo.RenderToolDoc(arazzo.ToolDocTemplates{RESTDescription: "{{.Nope"}, arazzo.NewToolDocContext("p", "1", "t", "", "", nil, "", ""))
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestRenderQueryDoc_CustomPrefix(t *testing.T) {
	ctx := arazzo.NewToolDocContext("p", "1", "t", "", "", nil, "http://example.test", "/service/v2")
	doc, err := arazzo.RenderQueryDoc(arazzo.ToolDocTemplates{}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Name != "query" {
		t.Fatalf("name = %q", doc.Name)
	}
	if doc.Title != "Query plans" {
		t.Fatalf("title = %q", doc.Title)
	}
	if strings.Contains(doc.Description, "POST ") || strings.Contains(doc.Description, "/plans/query") {
		t.Fatalf("MCP query description must not mention REST:\n%s", doc.Description)
	}
	if strings.Contains(strings.ToLower(doc.RESTDescription), "mcp") {
		t.Fatalf("REST query description must not mention MCP:\n%s", doc.RESTDescription)
	}
	if want := "POST http://example.test/service/v2/plans/query"; !strings.Contains(doc.RESTDescription, want) {
		t.Fatalf("REST query description missing %q:\n%s", want, doc.RESTDescription)
	}
}

func TestSanitizeToolName(t *testing.T) {
	if got := arazzo.SanitizeToolName("run petstore/v1"); got != "run_petstore_v1" {
		t.Fatalf("got %q", got)
	}
	long := strings.Repeat("a", 200)
	if got := arazzo.SanitizeToolName(long); len(got) != 128 {
		t.Fatalf("len = %d, want 128", len(got))
	}
}

func TestRenderToolDoc_EmptyName(t *testing.T) {
	_, err := arazzo.RenderToolDoc(arazzo.ToolDocTemplates{Name: "{{if false}}x{{end}}"}, arazzo.NewToolDocContext("p", "1", "t", "", "", nil, "", ""))
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("err = %v", err)
	}
}

func TestMergeTemplates_FillsDefaults(t *testing.T) {
	def := arazzo.DefaultToolDocTemplates()
	got := arazzo.MergeTemplates(arazzo.ToolDocTemplates{Name: "custom"})
	if got.Name != "custom" {
		t.Fatalf("custom name overwritten: %q", got.Name)
	}
	if got.Title != def.Title || got.Description != def.Description || got.RESTDescription != def.RESTDescription {
		t.Fatal("empty fields should use defaults")
	}
	if got.QueryName != def.QueryName || got.QueryTitle != def.QueryTitle {
		t.Fatal("empty query fields should use defaults")
	}
}

func TestFileLoader_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := arazzo.NewFileLoader(filepath.Join("..", "testdata", "arazzo", "plans")).Load(ctx)
	if err == nil {
		t.Fatal("expected cancelled context error")
	}
}

func TestRenderQueryDoc_InvalidTemplate(t *testing.T) {
	ctx := arazzo.NewToolDocContext("p", "1", "t", "", "", nil, "", "")
	if _, err := arazzo.RenderQueryDoc(arazzo.ToolDocTemplates{QueryName: "{{.Nope"}, ctx); err == nil {
		t.Fatal("expected query name parse error")
	}
}

func TestRenderToolDoc_InvalidTitleAndDescription(t *testing.T) {
	ctx := arazzo.NewToolDocContext("p", "1", "t", "", "", nil, "", "")
	if _, err := arazzo.RenderToolDoc(arazzo.ToolDocTemplates{Title: "{{.Nope"}, ctx); err == nil {
		t.Fatal("expected title parse error")
	}
	if _, err := arazzo.RenderToolDoc(arazzo.ToolDocTemplates{Description: "{{.Nope"}, ctx); err == nil {
		t.Fatal("expected description parse error")
	}
}

func TestNewToolDocContext_WorkflowFallbackAndPrefix(t *testing.T) {
	ctx := arazzo.NewToolDocContext(
		"plan/id", "1.0.0-beta", "t", "", "",
		[]arazzo.WorkflowDoc{{ID: "wf", Description: "first line\nsecond"}},
		"", "service/v2",
	)
	if ctx.SafePlanID != "plan_id" {
		t.Fatalf("SafePlanID = %q", ctx.SafePlanID)
	}
	if ctx.SafeVersion != "1.0.0-beta" {
		t.Fatalf("SafeVersion = %q", ctx.SafeVersion)
	}
	if ctx.APIRoot != "/service/v2" {
		t.Fatalf("APIRoot = %q", ctx.APIRoot)
	}
	if len(ctx.Workflows) != 1 || ctx.Workflows[0].SummaryOrDescription != "first line" {
		t.Fatalf("workflows = %+v", ctx.Workflows)
	}
	ctx2 := arazzo.NewToolDocContext("p", "1", "t", "", "", nil, "", "/")
	if ctx2.APIRoot != "/api" {
		t.Fatalf("empty/slash prefix APIRoot = %q", ctx2.APIRoot)
	}
	empty := arazzo.NewToolDocContext("p", "1", "t", "", "", []arazzo.WorkflowDoc{{}}, "", "")
	if empty.Workflows[0].SummaryOrDescription != "" {
		t.Fatalf("empty workflow summary = %q", empty.Workflows[0].SummaryOrDescription)
	}
}
