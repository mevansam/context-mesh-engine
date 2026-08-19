// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package api

// HealthResponse is returned by GET /api/v1/health.
type HealthResponse struct {
	Status string `json:"status"`
}
