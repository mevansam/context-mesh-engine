// Copyright 2026 Fidelity Investments. All rights reserved.
// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"context"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mevansam/context-mesh-engine/arazzo"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type runArgs struct {
	WorkflowID string         `json:"workflowId"`
	Inputs     map[string]any `json:"inputs"`
}

type queryArgs struct {
	Query string         `json:"query" jsonschema:"natural language query"`
	Data  map[string]any `json:"data,omitempty" jsonschema:"data inputs referenced by the query"`
}

// RegisterMCP adds one run_* tool per catalog entry. When the runner has a
// QueryMatcher, it also adds the query tool. The returned map is tool name →
// REST description for GET /tools overlays.
func RegisterMCP(server *mcp.Server, catalog *Catalog, runner *Runner, tmpls arazzo.ToolDocTemplates, publicBaseURL, apiPrefix string) (map[string]string, error) {
	tmpls = arazzo.MergeTemplates(tmpls)
	seen := map[string]string{}
	restDescs := map[string]string{}

	if runner != nil && runner.QueryEnabled() {
		qctx := arazzo.NewToolDocContext("plan", "1.0.0", "", "", "", nil, publicBaseURL, apiPrefix)
		qdoc, err := arazzo.RenderQueryDoc(tmpls, qctx)
		if err != nil {
			return nil, fmt.Errorf("query tool doc: %w", err)
		}
		restDescs[qdoc.Name] = qdoc.RESTDescription
		mcp.AddTool(server, &mcp.Tool{
			Name:        qdoc.Name,
			Title:       qdoc.Title,
			Description: qdoc.Description,
		}, func(ctx context.Context, _ *mcp.CallToolRequest, in queryArgs) (*mcp.CallToolResult, any, error) {
			res, err := runner.Query(ctx, in.Query, in.Data)
			if err != nil {
				return nil, nil, err
			}
			return nil, res, nil
		})
	}

	for _, e := range catalog.Entries() {
		title, summary, desc := "", "", ""
		if e.Doc.Info != nil {
			title = e.Doc.Info.Title
			summary = e.Doc.Info.Summary
			desc = e.Doc.Info.Description
		}
		ctx := arazzo.NewToolDocContext(e.PlanID, e.Version, title, summary, desc, e.Workflows, publicBaseURL, apiPrefix)
		doc, err := arazzo.RenderToolDoc(tmpls, ctx)
		if err != nil {
			return nil, fmt.Errorf("%s@%s: tool doc: %w", e.PlanID, e.Version, err)
		}
		if prev, ok := seen[doc.Name]; ok {
			return nil, fmt.Errorf("duplicate MCP tool name %q (%s and %s@%s)", doc.Name, prev, e.PlanID, e.Version)
		}
		seen[doc.Name] = e.PlanID + "@" + e.Version
		restDescs[doc.Name] = doc.RESTDescription

		schema, err := InputSchema(e.Doc)
		if err != nil {
			return nil, fmt.Errorf("%s@%s: input schema: %w", e.PlanID, e.Version, err)
		}

		planID, version := e.PlanID, e.Version
		mcp.AddTool(server, &mcp.Tool{
			Name:         doc.Name,
			Title:        doc.Title,
			Description:  doc.Description,
			InputSchema:  schema,
			OutputSchema: &jsonschema.Schema{Type: "object"},
		}, func(ctx context.Context, _ *mcp.CallToolRequest, in runArgs) (*mcp.CallToolResult, any, error) {
			res, err := runner.Run(ctx, planID, version, in.WorkflowID, in.Inputs)
			if err != nil {
				return nil, nil, err
			}
			return nil, res, nil
		})
	}
	return restDescs, nil
}
