// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"errors"
	"log/slog"
	"net/http"
)

var (
	// ErrUnexpectedInputs is returned when the caller sends fields that are
	// not in the workflow's consumer input schema (including policyHints and secrets).
	ErrUnexpectedInputs = errors.New("unexpected fields in inputs")
	// ErrInternal is a host/config failure that must not leak to the caller.
	ErrInternal = errors.New("internal error")
)

// PublicError is the transport-safe view of a runner or plan error.
type PublicError struct {
	Status  int
	Message string
}

// ClassifyError maps an internal error to a stable public status and message.
// The original error must be logged by the caller; it is not included here.
func ClassifyError(err error) PublicError {
	switch {
	case err == nil:
		return PublicError{Status: http.StatusOK}
	case errors.Is(err, ErrNoExecutor):
		return PublicError{Status: http.StatusNotImplemented, Message: ErrNoExecutor.Error()}
	case errors.Is(err, ErrQueryNotImplemented):
		return PublicError{Status: http.StatusNotImplemented, Message: ErrQueryNotImplemented.Error()}
	case errors.Is(err, ErrNotFound):
		return PublicError{Status: http.StatusNotFound, Message: ErrNotFound.Error()}
	case errors.Is(err, ErrUnauthorized):
		return PublicError{Status: http.StatusUnauthorized, Message: ErrUnauthorized.Error()}
	case errors.Is(err, ErrPolicyDenied):
		return PublicError{Status: http.StatusForbidden, Message: ErrPolicyDenied.Error()}
	case errors.Is(err, ErrEmptyQuery):
		return PublicError{Status: http.StatusBadRequest, Message: ErrEmptyQuery.Error()}
	case errors.Is(err, ErrUnexpectedInputs):
		return PublicError{Status: http.StatusBadRequest, Message: ErrUnexpectedInputs.Error()}
	case errors.Is(err, ErrPolicyLoad), errors.Is(err, ErrInternal):
		return PublicError{Status: http.StatusInternalServerError, Message: ErrInternal.Error()}
	default:
		return PublicError{Status: http.StatusBadRequest, Message: "workflow failed"}
	}
}

// LogAndPublic logs the full error and returns the public view for REST/MCP.
func LogAndPublic(logger *slog.Logger, err error) PublicError {
	pub := ClassifyError(err)
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn("plan request failed", "err", err, "status", pub.Status, "public", pub.Message)
	return pub
}
