// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package plans

import (
	"context"
	"errors"
	"fmt"

	libarazzo "github.com/pb33f/libopenapi/arazzo"
)

var (
	// ErrNotFound is returned when a plan, version, or workflow is missing.
	ErrNotFound = errors.New("plan not found")
	// ErrNoExecutor is returned when Run is called without an Executor.
	ErrNoExecutor = errors.New("executor not configured")
	// ErrQueryNotImplemented is returned by MCP query and POST /plans/query
	// until semantic plan matching is wired.
	ErrQueryNotImplemented = errors.New("query is not implemented")
)

// Runner executes workflows via a new libopenapi Engine per call.
type Runner struct {
	catalog  *Catalog
	executor libarazzo.Executor
}

// NewRunner wires a catalog to a (possibly nil) executor.
func NewRunner(catalog *Catalog, executor libarazzo.Executor) *Runner {
	return &Runner{catalog: catalog, executor: executor}
}

// Catalog returns the loaded plans.
func (r *Runner) Catalog() *Catalog {
	return r.catalog
}

// Run executes workflowID on planID at the given raw info.version.
// The returned map is the workflow outputs object (empty if none).
func (r *Runner) Run(ctx context.Context, planID, version, workflowID string, inputs map[string]any) (map[string]any, error) {
	e, ok := r.catalog.Get(planID, version)
	if !ok {
		return nil, fmt.Errorf("%w: %s@%s", ErrNotFound, planID, version)
	}
	found := false
	for _, id := range e.WorkflowIDs() {
		if id == workflowID {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: workflow %s", ErrNotFound, workflowID)
	}
	if r.executor == nil {
		return nil, ErrNoExecutor
	}
	eng := libarazzo.NewEngine(e.Doc, r.executor, e.Sources)
	res, err := eng.RunWorkflow(ctx, workflowID, inputs)
	if err != nil {
		return nil, err
	}
	if !res.Success {
		if res.Error != nil {
			return nil, res.Error
		}
		return nil, fmt.Errorf("workflow %s failed", workflowID)
	}
	out := res.Outputs
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

// Query is the shared MCP/REST stub for natural-language plan matching.
// Both ingresses must call this rather than duplicating the 501.
func (r *Runner) Query(_ context.Context, _ string, _ map[string]any) (map[string]any, error) {
	return nil, ErrQueryNotImplemented
}
