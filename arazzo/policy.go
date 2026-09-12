// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package arazzo

import (
	"context"
	"time"
)

// DefaultPolicyCacheTTL is used when [engine.Options.PolicyCacheTTL] is zero.
const DefaultPolicyCacheTTL = 5 * time.Minute

// PolicyHintsKey is the reserved workflow input that inbound policy may set.
// Caller-supplied values at this key (and dotted keys with this prefix) are
// discarded; the policy is the source of truth. The nested object is stored
// at this key. Leaves are also stored as dotted keys so Arazzo expressions
// like $inputs.policyHints.petStatus work with stock libopenapi ($inputs
// names are a single key, not a nested path).
const PolicyHintsKey = "policyHints"

// PolicyRequest is the key for [PolicyLoader.Load].
type PolicyRequest struct {
	PlanID  string
	Version string
}

// PolicyBundle is the optional OPA source for one (planId, version).
// Empty Inbound or Outbound means that phase is absent. Data is optional
// JSON object bytes merged into OPA document data (for example a backend
// base URL). Revision is an opaque cache token (etag, row version, content
// hash); empty means the engine refreshes on [DefaultPolicyCacheTTL] only.
// A nil *PolicyBundle from Load means no plan policy for the key.
type PolicyBundle struct {
	Inbound  []byte
	Outbound []byte
	Data     []byte
	Revision string
}

// HasInbound reports whether the bundle includes an inbound module.
func (b *PolicyBundle) HasInbound() bool {
	return b != nil && len(b.Inbound) > 0
}

// HasOutbound reports whether the bundle includes an outbound module.
func (b *PolicyBundle) HasOutbound() bool {
	return b != nil && len(b.Outbound) > 0
}

// PolicyLoader returns plan-scoped OPA modules for a plan version, if any.
// Lookups run on execute (MCP run_*, REST, query), not at [engine.New].
// A nil *PolicyBundle means skip plan inbound and outbound for that key.
//
// Load must not be used to fetch org-wide modules. If the same value also
// implements [SharedPolicySource], the engine calls LoadShared separately
// and interns those modules. Tenant, catalog, and storage handles belong
// on the loader value, not on [PolicyRequest].
type PolicyLoader interface {
	Load(ctx context.Context, req PolicyRequest) (*PolicyBundle, error)
}

// SharedPolicySource is an optional interface a [PolicyLoader] may implement
// to supply org-wide modules independently of any plan. The engine type-asserts
// Options.PolicyLoader; there is no separate Options field. A loader that
// only has per-plan rows implements [PolicyLoader] alone.
//
// Inbound must be package shared.inbound (query data.shared.inbound).
// Outbound must be package shared.outbound (query data.shared.outbound).
// Libraries are extra modules compiled into each plan query (and into the
// shared queries) so plan/shared Rego may import them (typically package
// lib.*). Keys are OPA module names, not filesystem paths, and must not be
// inbound.rego or outbound.rego.
type SharedPolicySource interface {
	LoadShared(ctx context.Context) (*SharedPolicy, error)
}

// SharedPolicy is org-wide OPA source returned by [SharedPolicySource].
// A nil *SharedPolicy from LoadShared means no shared modules.
// Shared inbound/outbound decisions are ANDed with the plan decision;
// only the plan may set hints, redact, outputs, or mask.
type SharedPolicy struct {
	Inbound   []byte
	Outbound  []byte
	Libraries map[string][]byte
	Revision  string
}

// HasInbound reports whether shared inbound is present.
func (s *SharedPolicy) HasInbound() bool {
	return s != nil && len(s.Inbound) > 0
}

// HasOutbound reports whether shared outbound is present.
func (s *SharedPolicy) HasOutbound() bool {
	return s != nil && len(s.Outbound) > 0
}

// HasLibraries reports whether any library module has source.
func (s *SharedPolicy) HasLibraries() bool {
	if s == nil {
		return false
	}
	for _, src := range s.Libraries {
		if len(src) > 0 {
			return true
		}
	}
	return false
}
