// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package arazzo

import (
	"context"
	"net/http"
)

type policyRequestKey struct{}

// PolicyRequestContext is the allowlisted request identity passed to OPA as
// input.headers and input.auth. It is not merged into workflow $inputs.
type PolicyRequestContext struct {
	Headers map[string]string
	Auth    map[string]any
}

// RequestSource is the raw inbound request for [RequestPreprocessor].
// Header is the HTTP (or MCP Extra.Header) map. ClientAuth is the verified
// calling-application identity when bearer middleware already ran (for
// example auth.TokenInfo fields). End-user JWTs in x-* headers are still
// on Header for the preprocessor to verify and enrich.
type RequestSource struct {
	Header     http.Header
	ClientAuth map[string]any
}

// RequestPreprocessor turns headers and prior client auth into a policy
// context. Extra JWTs (x-* headers) and remote claim enrichment belong here,
// not in Rego http.send. Nil [engine.Options.RequestPreprocessor] skips this.
type RequestPreprocessor interface {
	Process(ctx context.Context, src RequestSource) (*PolicyRequestContext, error)
}

// WithPolicyRequest stores pc on ctx for inbound/outbound OPA.
func WithPolicyRequest(ctx context.Context, pc *PolicyRequestContext) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, policyRequestKey{}, pc)
}

// PolicyRequestFromContext returns the value stored by [WithPolicyRequest].
func PolicyRequestFromContext(ctx context.Context) *PolicyRequestContext {
	if ctx == nil {
		return nil
	}
	pc, _ := ctx.Value(policyRequestKey{}).(*PolicyRequestContext)
	return pc
}
