// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/mevansam/context-mesh-engine/arazzo"
	"github.com/mevansam/context-mesh-engine/internal/ttlcache"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
)

const (
	inboundQuery  = "data.plan.inbound"
	outboundQuery = "data.plan.outbound"
)

var (
	// ErrPolicyDenied is returned when inbound or outbound policy does not allow.
	ErrPolicyDenied = errors.New("policy denied")
	// ErrPolicyLoad is returned when a policy bundle cannot be loaded or compiled
	// and no usable cached bundle exists.
	ErrPolicyLoad = errors.New("policy load failed")
)

// PolicyDeniedError is an inbound or outbound deny.
type PolicyDeniedError struct {
	Phase  string
	Reason string
}

func (e *PolicyDeniedError) Error() string {
	if e == nil {
		return ErrPolicyDenied.Error()
	}
	if e.Reason != "" {
		return fmt.Sprintf("%s (%s): %s", ErrPolicyDenied, e.Phase, e.Reason)
	}
	return fmt.Sprintf("%s (%s)", ErrPolicyDenied, e.Phase)
}

func (e *PolicyDeniedError) Unwrap() error { return ErrPolicyDenied }

func policyDenied(phase, reason string) error {
	return &PolicyDeniedError{Phase: phase, Reason: reason}
}

type compiledPolicy struct {
	loaded   bool
	inbound  *rego.PreparedEvalQuery
	outbound *rego.PreparedEvalQuery
}

func (c compiledPolicy) hasModules() bool {
	return c.inbound != nil || c.outbound != nil
}

// PolicyCache compiles OPA bundles on demand and reuses them until TTL elapses.
type PolicyCache struct {
	logger *slog.Logger
	cache  *ttlcache.Cache[string, compiledPolicy]
}

// NewPolicyCache wraps loader. ttl == 0 uses [arazzo.DefaultPolicyCacheTTL].
// A negative ttl disables caching (every Run compiles).
func NewPolicyCache(loader arazzo.PolicyLoader, ttl time.Duration, logger *slog.Logger) *PolicyCache {
	if ttl == 0 {
		ttl = arazzo.DefaultPolicyCacheTTL
	}
	if logger == nil {
		logger = slog.Default()
	}
	c := &PolicyCache{logger: logger}
	c.cache = ttlcache.New(ttl, func(ctx context.Context, key string) (compiledPolicy, error) {
		planID, version, _ := splitPolicyKey(key)
		b, err := loader.Load(ctx, arazzo.PolicyRequest{PlanID: planID, Version: version})
		if err != nil {
			return compiledPolicy{}, err
		}
		return compileBundle(ctx, b)
	})
	return c
}

func splitPolicyKey(key string) (planID, version string, ok bool) {
	planID, version, ok = strings.Cut(key, "\x00")
	return planID, version, ok
}

func policyCacheKey(planID, version string) string {
	return planID + "\x00" + version
}

func (c *PolicyCache) get(ctx context.Context, planID, version string) (compiledPolicy, error) {
	cp, err := c.cache.Get(ctx, policyCacheKey(planID, version))
	if err != nil {
		if !cp.loaded || !cp.hasModules() {
			return compiledPolicy{}, fmt.Errorf("%w: %w", ErrPolicyLoad, err)
		}
		c.logger.Warn("policy load failed; using cached bundle",
			"planId", planID, "version", version, "err", err)
		return cp, nil
	}
	return cp, nil
}

func compileBundle(ctx context.Context, b *arazzo.PolicyBundle) (compiledPolicy, error) {
	if b == nil || (!b.HasInbound() && !b.HasOutbound()) {
		return compiledPolicy{loaded: true}, nil
	}
	var data map[string]any
	if len(b.Data) > 0 {
		if err := json.Unmarshal(b.Data, &data); err != nil {
			return compiledPolicy{}, fmt.Errorf("policy data: %w", err)
		}
	}
	out := compiledPolicy{loaded: true}
	if b.HasInbound() {
		q, err := prepareRego(ctx, inboundQuery, "inbound.rego", string(b.Inbound), data)
		if err != nil {
			return compiledPolicy{}, fmt.Errorf("inbound: %w", err)
		}
		out.inbound = q
	}
	if b.HasOutbound() {
		q, err := prepareRego(ctx, outboundQuery, "outbound.rego", string(b.Outbound), data)
		if err != nil {
			return compiledPolicy{}, fmt.Errorf("outbound: %w", err)
		}
		out.outbound = q
	}
	return out, nil
}

func prepareRego(ctx context.Context, query, name, module string, data map[string]any) (*rego.PreparedEvalQuery, error) {
	opts := []func(*rego.Rego){
		rego.Query(query),
		rego.Module(name, module),
	}
	if data != nil {
		opts = append(opts, rego.Store(inmem.NewFromObject(data)))
	}
	pq, err := rego.New(opts...).PrepareForEval(ctx)
	if err != nil {
		return nil, err
	}
	return &pq, nil
}

func evalDecision(ctx context.Context, q *rego.PreparedEvalQuery, input map[string]any) (map[string]any, error) {
	rs, err := q.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, err
	}
	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		return nil, nil
	}
	v := rs[0].Expressions[0].Value
	if v == nil {
		return nil, nil
	}
	obj, ok := asObject(v)
	if !ok {
		return nil, fmt.Errorf("decision is not an object")
	}
	return obj, nil
}

func asObject(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, false
		}
		var out map[string]any
		if err := json.Unmarshal(b, &out); err != nil || out == nil {
			return nil, false
		}
		return out, true
	}
}

func decisionAllow(d map[string]any) bool {
	if d == nil {
		return false
	}
	v, ok := d["allow"]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func decisionReason(d map[string]any) string {
	if d == nil {
		return ""
	}
	s, _ := d["reason"].(string)
	return s
}

func policyEvalInput(ctx context.Context, planID, version, workflowID string, inputs map[string]any, outputs map[string]any) map[string]any {
	in := map[string]any{
		"planId":     planID,
		"version":    version,
		"workflowId": workflowID,
		"inputs":     inputs,
	}
	if outputs != nil {
		in["outputs"] = outputs
	}
	if pc := arazzo.PolicyRequestFromContext(ctx); pc != nil {
		if pc.Headers != nil {
			in["headers"] = pc.Headers
		}
		if pc.Auth != nil {
			in["auth"] = pc.Auth
		}
	}
	return in
}

func applyInbound(ctx context.Context, q *rego.PreparedEvalQuery, planID, version, workflowID string, inputs map[string]any) (map[string]any, error) {
	stripped := stripPolicyHints(inputs)
	d, err := evalDecision(ctx, q, policyEvalInput(ctx, planID, version, workflowID, stripped, nil))
	if err != nil {
		return nil, fmt.Errorf("%w: inbound eval: %w", ErrPolicyLoad, err)
	}
	if !decisionAllow(d) {
		return nil, policyDenied("inbound", decisionReason(d))
	}
	out := stripped
	if hints, ok := d["hints"]; ok && hints != nil {
		h, ok := asObject(hints)
		if !ok {
			return nil, policyDenied("inbound", "hints is not an object")
		}
		out[arazzo.PolicyHintsKey] = h
		// Stock libopenapi treats $inputs.a.b as the single key "a.b", not a nested
		// walk. Flatten so $inputs.policyHints.petStatus resolves.
		flattenPolicyHints(out, arazzo.PolicyHintsKey, h)
	}
	return out, nil
}

func applyOutbound(ctx context.Context, q *rego.PreparedEvalQuery, planID, version, workflowID string, inputs, outputs map[string]any) (map[string]any, error) {
	d, err := evalDecision(ctx, q, policyEvalInput(ctx, planID, version, workflowID, inputs, outputs))
	if err != nil {
		return nil, fmt.Errorf("%w: outbound eval: %w", ErrPolicyLoad, err)
	}
	if !decisionAllow(d) {
		return nil, policyDenied("outbound", decisionReason(d))
	}
	if raw, ok := d["outputs"]; ok && raw != nil {
		repl, ok := asObject(raw)
		if !ok {
			return nil, policyDenied("outbound", "outputs is not an object")
		}
		return repl, nil
	}
	pointers, err := redactPointers(d["redact"])
	if err != nil {
		return nil, policyDenied("outbound", err.Error())
	}
	if len(pointers) == 0 {
		return outputs, nil
	}
	mask, err := outboundMask(d["mask"])
	if err != nil {
		return nil, policyDenied("outbound", err.Error())
	}
	redacted, err := applyRedact(outputs, pointers, mask)
	if err != nil {
		return nil, policyDenied("outbound", err.Error())
	}
	return redacted, nil
}

func stripPolicyHints(inputs map[string]any) map[string]any {
	return stripPrefixed(inputs, arazzo.PolicyHintsKey)
}

func stripSecrets(inputs map[string]any) map[string]any {
	return stripPrefixed(inputs, arazzo.SecretsKey)
}

func stripPrefixed(inputs map[string]any, key string) map[string]any {
	out := map[string]any{}
	prefix := key + "."
	for k, v := range inputs {
		if k == key || strings.HasPrefix(k, prefix) {
			continue
		}
		out[k] = v
	}
	return out
}

func injectSecrets(ctx context.Context, inputs map[string]any, p arazzo.SecretsProvider, names []string) (map[string]any, error) {
	if p == nil || len(names) == 0 {
		return inputs, nil
	}
	out := inputs
	if out == nil {
		out = map[string]any{}
	}
	bag := map[string]any{}
	for _, name := range names {
		v, err := p.Get(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("secret %q: %w", name, err)
		}
		bag[name] = v
	}
	out[arazzo.SecretsKey] = bag
	flattenPolicyHints(out, arazzo.SecretsKey, bag)
	return out, nil
}

func flattenPolicyHints(dst map[string]any, prefix string, obj map[string]any) {
	for k, v := range obj {
		key := prefix + "." + k
		dst[key] = v
		if nested, ok := v.(map[string]any); ok {
			flattenPolicyHints(dst, key, nested)
		}
	}
}

func redactPointers(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	switch s := v.(type) {
	case []string:
		return s, nil
	case []any:
		out := make([]string, 0, len(s))
		for _, item := range s {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("redact entries must be strings")
			}
			out = append(out, str)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("redact must be an array of strings")
	}
}

func outboundMask(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("mask must be a string")
	}
	return s, nil
}
