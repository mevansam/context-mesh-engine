// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package arazzo

import "testing"

func TestReservedInputKey(t *testing.T) {
	for _, name := range []string{
		PolicyHintsKey, PolicyHintsKey + ".petStatus",
		SecretsKey, SecretsKey + ".hmac",
	} {
		if !ReservedInputKey(name) {
			t.Errorf("%q: want reserved", name)
		}
	}
	for _, name := range []string{"status", "policyHintsFoo", "mysecrets", "vault"} {
		if ReservedInputKey(name) {
			t.Errorf("%q: want not reserved", name)
		}
	}
}

func TestLeaksReservedInputs(t *testing.T) {
	if !LeaksReservedInputs("uses $inputs.policyHints.petStatus") {
		t.Fatal("policyHints")
	}
	if !LeaksReservedInputs("see $inputs.secrets.hmac") {
		t.Fatal("$inputs.secrets")
	}
	if LeaksReservedInputs("Username comes from the end-user JWT.") {
		t.Fatal("clean text")
	}
	if LeaksReservedInputs("this API never stores user secrets in the body") {
		t.Fatal("bare secrets should not match")
	}
}
