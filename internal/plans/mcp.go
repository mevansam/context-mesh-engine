// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"context"
	"fmt"
	"log/slog"
	"time"

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

// RegisterMCPConfig is documentation and help-lookup wiring for plan tools.
type RegisterMCPConfig struct {
	Templates     arazzo.ToolDocTemplates
	HelpLookup    arazzo.ToolHelpLookup
	HelpCacheTTL  time.Duration
	Logger        *slog.Logger
	PublicBaseURL string
	APIPrefix     string
}

// RegisterMCP adds one run_* tool per catalog entry. When the runner has a
// QueryMatcher, it also adds the query tool. Title and description are filled
// on tools/list / GET /tools via the returned [HelpCache].
func RegisterMCP(server *mcp.Server, catalog *Catalog, runner *Runner, cfg RegisterMCPConfig) (*HelpCache, error) {
	tmpls := arazzo.MergeTemplates(cfg.Templates)
	cache := newHelpCache(tmpls, cfg.HelpLookup, cfg.HelpCacheTTL, cfg.Logger)
	seen := map[string]string{}

	if runner != nil && runner.QueryEnabled() {
		qctx := arazzo.NewToolDocContext("plan", "1.0.0", "", "", "", nil, cfg.PublicBaseURL, cfg.APIPrefix)
		qdoc, err := arazzo.RenderQueryDoc(tmpls, qctx)
		if err != nil {
			return nil, fmt.Errorf("query tool doc: %w", err)
		}
		cache.add(qdoc.Name, helpTarget{kind: arazzo.ToolHelpKindQuery, docCtx: qctx})
		mcp.AddTool(server, &mcp.Tool{
			Name:        qdoc.Name,
			Title:       qdoc.Title,
			Description: qdoc.Description,
		}, func(ctx context.Context, req *mcp.CallToolRequest, in queryArgs) (*mcp.CallToolResult, any, error) {
			ctx, err := runner.EnrichContext(ctx, RequestSourceFromMCP(req))
			if err != nil {
				return nil, nil, err
			}
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
		docCtx := arazzo.NewToolDocContext(e.PlanID, e.Version, title, summary, desc, e.Workflows, cfg.PublicBaseURL, cfg.APIPrefix)
		doc, err := arazzo.RenderToolDoc(tmpls, docCtx)
		if err != nil {
			return nil, fmt.Errorf("%s@%s: tool doc: %w", e.PlanID, e.Version, err)
		}
		if prev, ok := seen[doc.Name]; ok {
			return nil, fmt.Errorf("duplicate MCP tool name %q (%s and %s@%s)", doc.Name, prev, e.PlanID, e.Version)
		}
		seen[doc.Name] = e.PlanID + "@" + e.Version
		cache.add(doc.Name, helpTarget{
			kind:    arazzo.ToolHelpKindPlan,
			planID:  e.PlanID,
			version: e.Version,
			docCtx:  docCtx,
		})

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
		}, func(ctx context.Context, req *mcp.CallToolRequest, in runArgs) (*mcp.CallToolResult, any, error) {
			ctx, err := runner.EnrichContext(ctx, RequestSourceFromMCP(req))
			if err != nil {
				return nil, nil, err
			}
			res, err := runner.Run(ctx, planID, version, in.WorkflowID, in.Inputs)
			if err != nil {
				return nil, nil, err
			}
			return nil, res, nil
		})
	}
	return cache, nil
}
