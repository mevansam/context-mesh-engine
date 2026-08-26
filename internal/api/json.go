// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const maxJSONBody = 1 << 20 // 1 MiB

// WriteJSON writes v as application/json with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ErrorBody is the standard JSON error payload for REST responses.
type ErrorBody struct {
	Error string `json:"error"`
}

// WriteError writes a JSON error object with the given status code.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, ErrorBody{Error: msg})
}

// ReadJSON decodes a JSON request body into v.
func ReadJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return fmt.Errorf("empty body")
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxJSONBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}

// DecodeMap decodes a JSON object, keeping integers that do not fit in
// float64 (JSON numbers become int64 when they parse as integers).
func DecodeMap(r io.Reader) (map[string]any, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	var inputs map[string]any
	if err := dec.Decode(&inputs); err != nil {
		return nil, err
	}
	if inputs == nil {
		return nil, nil
	}
	return CanonicalJSON(inputs).(map[string]any), nil
}

// CanonicalJSON converts json.Number values to int64 when they are
// integers, otherwise float64. Maps and slices are walked in place.
func CanonicalJSON(v any) any {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		if f, err := t.Float64(); err == nil {
			return f
		}
		return string(t)
	case map[string]any:
		for k, val := range t {
			t[k] = CanonicalJSON(val)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = CanonicalJSON(val)
		}
		return t
	default:
		return v
	}
}
