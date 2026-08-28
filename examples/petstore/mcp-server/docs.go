// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package main

import (
	_ "embed"
	"net/http"
)

//go:embed docs/index.html
var docsPage []byte

// docsController serves the Swagger UI page at GET /docs (after APIPrefix).
type docsController struct{}

func (docsController) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /docs", serveDocs)
	mux.HandleFunc("GET /docs/", serveDocs)
}

func serveDocs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(docsPage)
}
