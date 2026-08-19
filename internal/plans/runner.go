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

// ResultJSON is the shared MCP/REST execution payload.
type ResultJSON struct {
	WorkflowID string           `json:"workflowId"`
	Success    bool             `json:"success"`
	Inputs     map[string]any   `json:"inputs,omitempty"`
	Outputs    map[string]any   `json:"outputs,omitempty"`
	Steps      []StepResultJSON `json:"steps,omitempty"`
	Error      string           `json:"error,omitempty"`
	DurationMs int64            `json:"durationMs"`
}

// StepResultJSON is one step in [ResultJSON].
type StepResultJSON struct {
	StepID     string         `json:"stepId"`
	Success    bool           `json:"success"`
	StatusCode int            `json:"statusCode"`
	Outputs    map[string]any `json:"outputs,omitempty"`
	Error      string         `json:"error,omitempty"`
	DurationMs int64          `json:"durationMs"`
	Retries    int            `json:"retries,omitempty"`
}

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
func (r *Runner) Run(ctx context.Context, planID, version, workflowID string, inputs map[string]any) (*ResultJSON, error) {
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
	return mapResult(res), nil
}

// Query is the shared MCP/REST stub for natural-language plan matching.
// Both ingresses must call this rather than duplicating the 501.
func (r *Runner) Query(_ context.Context, _ string, _ map[string]any) (*ResultJSON, error) {
	return nil, ErrQueryNotImplemented
}

func mapResult(res *libarazzo.WorkflowResult) *ResultJSON {
	out := &ResultJSON{
		WorkflowID: res.WorkflowId,
		Success:    res.Success,
		Inputs:     res.Inputs,
		Outputs:    res.Outputs,
		DurationMs: res.Duration.Milliseconds(),
	}
	if res.Error != nil {
		out.Error = res.Error.Error()
	}
	for _, st := range res.Steps {
		sj := StepResultJSON{
			StepID:     st.StepId,
			Success:    st.Success,
			StatusCode: st.StatusCode,
			Outputs:    st.Outputs,
			DurationMs: st.Duration.Milliseconds(),
			Retries:    st.Retries,
		}
		if st.Error != nil {
			sj.Error = st.Error.Error()
		}
		out.Steps = append(out.Steps, sj)
	}
	return out
}
