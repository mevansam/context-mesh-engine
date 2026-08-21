// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package arazzo

import (
	"context"
	"iter"
)

// QueryMatcher selects a plan and workflow for MCP query and
// POST {APIPrefix}/plans/query. Semantic matching (vector search, and so
// on) lives in the application; this SDK only calls Match and then
// checks that the selection is loaded in this process.
//
// A nil matcher means the query tool and REST route are not registered.
//
// Match must not treat a miss in [QueryRequest.Catalog] as “no match”.
// The engine verifies the loaded catalog after Match returns.
type QueryMatcher interface {
	Match(ctx context.Context, req QueryRequest) (*QueryMatch, error)
}

// QueryRequest is the natural-language query plus a handle to plans
// loaded in this engine. Catalog is an interface value; the engine does
// not copy the catalog on each query.
type QueryRequest struct {
	Query   string
	Data    map[string]any
	Catalog PlanCatalog
}

// QueryMatch is the matcher’s selection from its (possibly global) registry.
type QueryMatch struct {
	// PlanID is info.x-planId. Required.
	PlanID string
	// Version is info.version. Empty means the latest version loaded
	// in this engine for PlanID.
	Version string
	// WorkflowID is the workflow to run. Required.
	WorkflowID string
	// Inputs are passed to the workflow. Nil means use [QueryRequest.Data].
	Inputs map[string]any
}

// PlanCatalog is a read-only view of Arazzo plans loaded into this engine.
// Implementations copy metadata only when a method is called.
type PlanCatalog interface {
	Get(planID, version string) (PlanSummary, bool)
	Latest(planID string) (PlanSummary, bool)
	Plans() iter.Seq[PlanSummary]
}

// PlanSummary is catalog metadata for matching or display. It does not
// include the parsed Arazzo document.
type PlanSummary struct {
	PlanID      string
	Version     string
	Title       string
	Summary     string
	Description string
	Workflows   []WorkflowDoc
}
