// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package api

import (
	"net/http"

	iapi "github.com/mevansam/context-mesh-engine/internal/api"
)

// ErrorBody is the standard JSON error payload for REST under Options.APIPrefix.
type ErrorBody = iapi.ErrorBody

// WriteJSON writes v as application/json with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	iapi.WriteJSON(w, status, v)
}

// WriteError writes a JSON error object with the given status code.
func WriteError(w http.ResponseWriter, status int, msg string) {
	iapi.WriteError(w, status, msg)
}

// ReadJSON decodes a JSON request body into v.
// w is passed to [http.MaxBytesReader] so an oversized body can close the connection.
func ReadJSON(w http.ResponseWriter, r *http.Request, v any) error {
	return iapi.ReadJSON(w, r, v)
}
