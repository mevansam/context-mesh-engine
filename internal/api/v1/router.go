// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package apiv1

import "net/http"

// Router is the /api/v1 ServeMux (paths are relative after StripPrefix).
type Router struct {
	mux *http.ServeMux
}

// controller is the registration surface. It matches api.Controller
// without importing the public package (avoids an import cycle).
type controller interface {
	Register(mux *http.ServeMux)
}

// New returns an empty v1 router.
func New() *Router {
	return &Router{mux: http.NewServeMux()}
}

// Register adds a controller's routes.
func (r *Router) Register(c controller) {
	c.Register(r.mux)
}

// Handler returns the v1 mux.
func (r *Router) Handler() http.Handler {
	return r.mux
}
