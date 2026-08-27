// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package arazzo

import (
	"context"
	"fmt"
)

// SecretsKey is the reserved workflow input prefix for values injected from
// [SecretsProvider]. Caller-supplied secrets and secrets.* keys are discarded.
// Leaves are flattened (secrets.hmacKey) so $inputs.secrets.hmacKey works
// with stock libopenapi. Do not inject signing keys that must stay in the
// Executor; only names listed in [engine.Options.SecretInputs].
const SecretsKey = "secrets"

// SecretsProvider fetches a named secret (vault, env, local map).
// Used by the host Executor to mint downstream JWTs, and optionally to
// populate $inputs.secrets.* for Arazzo steps.
type SecretsProvider interface {
	Get(ctx context.Context, name string) (string, error)
}

// MapSecrets is an in-process [SecretsProvider].
type MapSecrets map[string]string

// Get implements [SecretsProvider].
func (m MapSecrets) Get(_ context.Context, name string) (string, error) {
	if m == nil {
		return "", fmt.Errorf("secret %q not found", name)
	}
	v, ok := m[name]
	if !ok {
		return "", fmt.Errorf("secret %q not found", name)
	}
	return v, nil
}
