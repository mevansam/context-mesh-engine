// Copyright 2026 Fidelity Investments. All rights reserved.
// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package arazzo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mevansam/context-mesh-engine/arazzo"
)

func TestDefaultToolHelpLookup_Empty(t *testing.T) {
	help, err := arazzo.DefaultToolHelpLookup().Lookup(context.Background(), arazzo.ToolHelpRequest{
		Kind: arazzo.ToolHelpKindPlan, PlanID: "petstore", Version: "1.0.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if help != nil {
		t.Fatalf("got %#v, want nil", help)
	}
}

func TestRenderWithHelp_EmptyUsesDefaults(t *testing.T) {
	ctx := arazzo.NewToolDocContext(
		"petstore", "1.1.0", "Pet Store Workflows", "summary", "desc",
		[]arazzo.WorkflowDoc{{ID: "pingHealth", Summary: "Check API health"}},
		"http://localhost:8080", "",
	)
	def, err := arazzo.RenderToolDoc(arazzo.ToolDocTemplates{}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := arazzo.RenderWithHelp(arazzo.ToolDocTemplates{}, arazzo.ToolHelp{}, arazzo.ToolHelpKindPlan, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != def.Title || got.Description != def.Description || got.RESTDescription != def.RESTDescription {
		t.Fatalf("empty help should match defaults\ngot  %#v\nwant %#v", got, def)
	}
}

func TestRenderWithHelp_RESTFallsBackToDescription(t *testing.T) {
	ctx := arazzo.NewToolDocContext("petstore", "1.0.0", "t", "", "", nil, "http://example.test", "")
	doc, err := arazzo.RenderWithHelp(arazzo.ToolDocTemplates{}, arazzo.ToolHelp{
		Title:       "Custom {{.PlanID}}",
		Description: "help for {{.PlanID}} {{.Version}}",
	}, arazzo.ToolHelpKindPlan, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Title != "Custom petstore" {
		t.Fatalf("title = %q", doc.Title)
	}
	if doc.Description != "help for petstore 1.0.0" {
		t.Fatalf("description = %q", doc.Description)
	}
	if doc.RESTDescription != doc.Description {
		t.Fatalf("REST should reuse Description, got %q want %q", doc.RESTDescription, doc.Description)
	}
	if strings.Contains(doc.RESTDescription, "POST ") {
		t.Fatalf("REST fallback should not use default REST template:\n%s", doc.RESTDescription)
	}
}

func TestRenderWithHelp_RESTDescriptionWins(t *testing.T) {
	ctx := arazzo.NewToolDocContext("petstore", "1.0.0", "t", "", "", nil, "", "")
	doc, err := arazzo.RenderWithHelp(arazzo.ToolDocTemplates{}, arazzo.ToolHelp{
		Description:     "mcp {{.PlanID}}",
		RESTDescription: "rest {{.PlanID}}",
	}, arazzo.ToolHelpKindPlan, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Description != "mcp petstore" || doc.RESTDescription != "rest petstore" {
		t.Fatalf("got MCP %q REST %q", doc.Description, doc.RESTDescription)
	}
}

func TestRenderWithHelp_QueryRESTWins(t *testing.T) {
	ctx := arazzo.NewToolDocContext("plan", "1.0.0", "", "", "", nil, "http://example.test", "/api")
	doc, err := arazzo.RenderWithHelp(arazzo.ToolDocTemplates{}, arazzo.ToolHelp{
		Title:           "Ask",
		Description:     "mcp-q",
		RESTDescription: "POST {{.RESTQueryURL}}",
	}, arazzo.ToolHelpKindQuery, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Name != "query" || doc.Title != "Ask" {
		t.Fatalf("name=%q title=%q", doc.Name, doc.Title)
	}
	if doc.Description != "mcp-q" {
		t.Fatalf("MCP = %q", doc.Description)
	}
	if !strings.Contains(doc.RESTDescription, "POST http://example.test/api/plans/query") {
		t.Fatalf("REST = %q", doc.RESTDescription)
	}
}

func TestOverlayToolHelp_TitleOnlyLeavesDefaultDescriptions(t *testing.T) {
	def := arazzo.DefaultToolDocTemplates()
	plan := arazzo.OverlayToolHelp(arazzo.ToolDocTemplates{}, arazzo.ToolHelp{Title: "Only"}, arazzo.ToolHelpKindPlan)
	if plan.Title != "Only" {
		t.Fatalf("title = %q", plan.Title)
	}
	if plan.Description != def.Description || plan.RESTDescription != def.RESTDescription {
		t.Fatal("plan description templates should stay defaults")
	}
	query := arazzo.OverlayToolHelp(arazzo.ToolDocTemplates{}, arazzo.ToolHelp{Title: "Ask"}, arazzo.ToolHelpKindQuery)
	if query.QueryTitle != "Ask" {
		t.Fatalf("query title = %q", query.QueryTitle)
	}
	if query.QueryDescription != def.QueryDescription || query.RESTQueryDescription != def.RESTQueryDescription {
		t.Fatal("query description templates should stay defaults")
	}
}
