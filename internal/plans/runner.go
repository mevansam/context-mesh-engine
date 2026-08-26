// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mevansam/context-mesh-engine/arazzo"
	libarazzo "github.com/pb33f/libopenapi/arazzo"
	"go.yaml.in/yaml/v4"
)

var (
	// ErrNotFound is returned when a plan, version, or workflow is missing.
	ErrNotFound = errors.New("plan not found")
	// ErrNoExecutor is returned when Run is called without an Executor.
	ErrNoExecutor = errors.New("executor not configured")
	// ErrQueryNotImplemented is returned by [Runner.Query] when no matcher
	// is set. The MCP tool and REST route are not registered in that case.
	ErrQueryNotImplemented = errors.New("query is not implemented")
	// ErrEmptyQuery is returned when the query string is empty.
	ErrEmptyQuery = errors.New("query is required")
)

// Runner executes workflows via a new libopenapi Engine per call.
type Runner struct {
	catalog  *Catalog
	executor libarazzo.Executor
	matcher  arazzo.QueryMatcher
	policy   *PolicyCache
}

// NewRunner wires a catalog to a (possibly nil) executor and query matcher.
func NewRunner(catalog *Catalog, executor libarazzo.Executor, matcher arazzo.QueryMatcher) *Runner {
	return &Runner{catalog: catalog, executor: executor, matcher: matcher}
}

// SetPolicy attaches an on-demand OPA cache. Nil disables inbound/outbound checks.
func (r *Runner) SetPolicy(p *PolicyCache) {
	if r != nil {
		r.policy = p
	}
}

// Catalog returns the loaded plans.
func (r *Runner) Catalog() *Catalog {
	return r.catalog
}

// QueryEnabled is true when a [arazzo.QueryMatcher] was passed to [NewRunner].
func (r *Runner) QueryEnabled() bool {
	return r != nil && r.matcher != nil
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

	runInputs := inputs
	var compiled compiledPolicy
	if r.policy != nil {
		var err error
		compiled, err = r.policy.get(ctx, planID, version)
		if err != nil {
			return nil, err
		}
		if compiled.inbound != nil {
			runInputs, err = applyInbound(ctx, compiled.inbound, planID, version, workflowID, inputs)
			if err != nil {
				return nil, err
			}
		}
	}

	if r.executor == nil {
		return nil, ErrNoExecutor
	}
	eng := libarazzo.NewEngine(e.Doc, r.executor, e.Sources)
	res, err := eng.RunWorkflow(ctx, workflowID, runInputs)
	if err != nil {
		return nil, err
	}
	if !res.Success {
		if res.Error != nil {
			return nil, res.Error
		}
		return nil, fmt.Errorf("workflow %s failed", workflowID)
	}
	out := nativeOutputs(res.Outputs)
	if compiled.outbound != nil {
		out, err = applyOutbound(ctx, compiled.outbound, planID, version, workflowID, runInputs, out)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// nativeOutputs converts libopenapi output values to JSON-friendly Go types.
// JSON Pointers into objects/arrays yield *yaml.Node; agents must not see that AST.
func nativeOutputs(out map[string]any) map[string]any {
	if out == nil {
		return map[string]any{}
	}
	for k, v := range out {
		out[k] = nativeOutput(v)
	}
	return out
}

func nativeOutput(v any) any {
	switch n := v.(type) {
	case *yaml.Node:
		var decoded any
		if err := n.Decode(&decoded); err != nil {
			return v
		}
		return decoded
	case yaml.Node:
		return nativeOutput(&n)
	default:
		return v
	}
}

// Query asks the matcher for a plan, verifies it is loaded, then [Runner.Run]s it.
func (r *Runner) Query(ctx context.Context, query string, data map[string]any) (map[string]any, error) {
	if r.matcher == nil {
		return nil, ErrQueryNotImplemented
	}
	if strings.TrimSpace(query) == "" {
		return nil, ErrEmptyQuery
	}
	match, err := r.matcher.Match(ctx, arazzo.QueryRequest{
		Query:   query,
		Data:    data,
		Catalog: r.catalog.View(),
	})
	if err != nil {
		return nil, err
	}
	if match == nil || match.PlanID == "" || match.WorkflowID == "" {
		return nil, fmt.Errorf("%w: no matching plan", ErrNotFound)
	}

	var e *Entry
	var ok bool
	if strings.TrimSpace(match.Version) == "" {
		e, ok = r.catalog.Latest(match.PlanID)
	} else {
		e, ok = r.catalog.Get(match.PlanID, match.Version)
	}
	if !ok {
		ver := match.Version
		if ver == "" {
			ver = "latest"
		}
		return nil, fmt.Errorf("%w: %s@%s not loaded", ErrNotFound, match.PlanID, ver)
	}

	found := false
	for _, id := range e.WorkflowIDs() {
		if id == match.WorkflowID {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: workflow %s", ErrNotFound, match.WorkflowID)
	}

	inputs := match.Inputs
	if inputs == nil {
		inputs = data
	}
	return r.Run(ctx, e.PlanID, e.Version, match.WorkflowID, inputs)
}
