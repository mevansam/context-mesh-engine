// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package arazzo

import (
	"context"
	"time"
)

// DefaultToolHelpCacheTTL is used when [engine.Options.ToolHelpCacheTTL] is zero.
const DefaultToolHelpCacheTTL = 5 * time.Minute

// ToolHelpKind selects which tool a [ToolHelpLookup] request refers to.
type ToolHelpKind string

const (
	// ToolHelpKindPlan is a run_* tool for one catalog (planId, version).
	ToolHelpKindPlan ToolHelpKind = "plan"
	// ToolHelpKindQuery is the global query tool (no planId/version).
	ToolHelpKindQuery ToolHelpKind = "query"
)

// ToolHelpRequest is the key for [ToolHelpLookup.Lookup].
type ToolHelpRequest struct {
	Kind    ToolHelpKind
	PlanID  string
	Version string
}

// ToolHelp is optional title and description templates for one tool.
// Empty fields fall back to [ToolDocTemplates] / [DefaultToolDocTemplates].
// RESTDescription falls back to Description when empty.
type ToolHelp struct {
	Title           string
	Description     string
	RESTDescription string
}

// ToolHelpLookup returns per-tool title/description templates.
// Lookups run on MCP tools/list and GET /tools, not at engine.New.
// A nil *ToolHelp or a zero ToolHelp means use default templates.
type ToolHelpLookup interface {
	Lookup(ctx context.Context, req ToolHelpRequest) (*ToolHelp, error)
}

type defaultToolHelpLookup struct{}

// DefaultToolHelpLookup returns empty help so [OverlayToolHelp] uses
// [DefaultToolDocTemplates].
func DefaultToolHelpLookup() ToolHelpLookup {
	return defaultToolHelpLookup{}
}

func (defaultToolHelpLookup) Lookup(context.Context, ToolHelpRequest) (*ToolHelp, error) {
	return nil, nil
}

// OverlayToolHelp copies non-empty help templates onto tmpls. Empty
// RESTDescription uses Description. Kind selects plan vs query fields.
func OverlayToolHelp(tmpls ToolDocTemplates, help ToolHelp, kind ToolHelpKind) ToolDocTemplates {
	tmpls = MergeTemplates(tmpls)
	rest := help.RESTDescription
	if rest == "" {
		rest = help.Description
	}
	if kind == ToolHelpKindQuery {
		if help.Title != "" {
			tmpls.QueryTitle = help.Title
		}
		if help.Description != "" {
			tmpls.QueryDescription = help.Description
		}
		if rest != "" {
			tmpls.RESTQueryDescription = rest
		}
		return tmpls
	}
	if help.Title != "" {
		tmpls.Title = help.Title
	}
	if help.Description != "" {
		tmpls.Description = help.Description
	}
	if rest != "" {
		tmpls.RESTDescription = rest
	}
	return tmpls
}

// RenderWithHelp overlays help then renders plan or query docs.
func RenderWithHelp(tmpls ToolDocTemplates, help ToolHelp, kind ToolHelpKind, ctx ToolDocContext) (RenderedToolDoc, error) {
	tmpls = OverlayToolHelp(tmpls, help, kind)
	if kind == ToolHelpKindQuery {
		return RenderQueryDoc(tmpls, ctx)
	}
	return RenderToolDoc(tmpls, ctx)
}
