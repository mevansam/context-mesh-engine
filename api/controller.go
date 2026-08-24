// Copyright 2026 Fidelity Investments. All rights reserved.
// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

// Package api provides REST helpers and the controller registration
// interface used under Options.APIPrefix (default /api).
package api

import "net/http"

// Controller registers HTTP routes on a ServeMux.
//
// Routes should be method-aware (for example "GET /health"). The mux
// passed to Register is already stripped of Options.APIPrefix.
type Controller interface {
	Register(mux *http.ServeMux)
}
