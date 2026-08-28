// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestClassifyError(t *testing.T) {
	cases := []struct {
		err     error
		status  int
		message string
	}{
		{ErrNoExecutor, http.StatusNotImplemented, "executor not configured"},
		{fmt.Errorf("%w: petstore@1.1.0", ErrNotFound), http.StatusNotFound, "plan not found"},
		{fmt.Errorf("%w: missing token", ErrUnauthorized), http.StatusUnauthorized, "unauthorized"},
		{policyDenied("inbound", "browsers cannot purchase"), http.StatusForbidden, "policy denied"},
		{fmt.Errorf("%w: inbound compile", ErrPolicyLoad), http.StatusInternalServerError, "internal error"},
		{fmt.Errorf("%w: secret hmac", ErrInternal), http.StatusInternalServerError, "internal error"},
		{ErrUnexpectedInputs, http.StatusBadRequest, "unexpected fields in inputs"},
		{ErrEmptyQuery, http.StatusBadRequest, "query is required"},
		{errors.New("step getPet failed: $inputs.policyHints.petStatus"), http.StatusBadRequest, "workflow failed"},
	}
	for _, tc := range cases {
		got := ClassifyError(tc.err)
		if got.Status != tc.status || got.Message != tc.message {
			t.Errorf("%v: %+v, want %d %q", tc.err, got, tc.status, tc.message)
		}
	}
}
