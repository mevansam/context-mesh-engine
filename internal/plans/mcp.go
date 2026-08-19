// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package plans

import (
	"context"
	"fmt"

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

// RegisterMCP adds query (stub) and one run_* tool per catalog entry.
func RegisterMCP(server *mcp.Server, catalog *Catalog, runner *Runner, tmpls arazzo.ToolDocTemplates, publicBaseURL string) error {
	tmpls = arazzo.MergeTemplates(tmpls)
	seen := map[string]string{}

	mcp.AddTool(server, &mcp.Tool{
		Name:        tmpls.QueryName,
		Title:       tmpls.QueryTitle,
		Description: tmpls.QueryDescription,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in queryArgs) (*mcp.CallToolResult, any, error) {
		res, err := runner.Query(ctx, in.Query, in.Data)
		if err != nil {
			return nil, nil, err
		}
		return nil, res, nil
	})

	for _, e := range catalog.Entries() {
		title, summary, desc := "", "", ""
		if e.Doc.Info != nil {
			title = e.Doc.Info.Title
			summary = e.Doc.Info.Summary
			desc = e.Doc.Info.Description
		}
		ctx := arazzo.NewToolDocContext(e.PlanID, e.Version, title, summary, desc, e.Workflows, publicBaseURL)
		name, toolTitle, toolDesc, err := arazzo.RenderToolDoc(tmpls, ctx)
		if err != nil {
			return fmt.Errorf("%s@%s: tool doc: %w", e.PlanID, e.Version, err)
		}
		if prev, ok := seen[name]; ok {
			return fmt.Errorf("duplicate MCP tool name %q (%s and %s@%s)", name, prev, e.PlanID, e.Version)
		}
		seen[name] = e.PlanID + "@" + e.Version

		schema, err := InputSchema(e.Doc)
		if err != nil {
			return fmt.Errorf("%s@%s: input schema: %w", e.PlanID, e.Version, err)
		}

		planID, version := e.PlanID, e.Version
		mcp.AddTool(server, &mcp.Tool{
			Name:        name,
			Title:       toolTitle,
			Description: toolDesc,
			InputSchema: schema,
		}, func(ctx context.Context, _ *mcp.CallToolRequest, in runArgs) (*mcp.CallToolResult, any, error) {
			res, err := runner.Run(ctx, planID, version, in.WorkflowID, in.Inputs)
			if err != nil {
				return nil, nil, err
			}
			return nil, res, nil
		})
	}
	return nil
}
