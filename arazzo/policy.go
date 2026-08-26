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
// base URL). A nil *PolicyBundle from Load means no policy for the key.
type PolicyBundle struct {
	Inbound  []byte
	Outbound []byte
	Data     []byte
}

// HasInbound reports whether the bundle includes an inbound module.
func (b *PolicyBundle) HasInbound() bool {
	return b != nil && len(b.Inbound) > 0
}

// HasOutbound reports whether the bundle includes an outbound module.
func (b *PolicyBundle) HasOutbound() bool {
	return b != nil && len(b.Outbound) > 0
}

// PolicyLoader returns OPA modules for a plan version, if any.
// Lookups run on execute (MCP run_*, REST, query), not at [engine.New].
// A nil *PolicyBundle means skip inbound and outbound for that key.
type PolicyLoader interface {
	Load(ctx context.Context, req PolicyRequest) (*PolicyBundle, error)
}
