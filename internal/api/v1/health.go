// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package apiv1

import (
	"net/http"

	"github.com/mevansam/context-mesh-engine/api"
	iapi "github.com/mevansam/context-mesh-engine/internal/api"
)

// HealthController serves GET /health on the REST mux.
type HealthController struct{}

// Register implements api.Controller.
func (c *HealthController) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", c.Get)
}

// Get reports process liveness as JSON.
func (c *HealthController) Get(w http.ResponseWriter, _ *http.Request) {
	iapi.WriteJSON(w, http.StatusOK, api.HealthResponse{Status: "ok"})
}
