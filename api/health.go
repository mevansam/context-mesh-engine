// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package api

// HealthResponse is the JSON body for the default health endpoint
// (GET under Options.APIPrefix, typically /api/health).
type HealthResponse struct {
	Status string `json:"status"`
}
