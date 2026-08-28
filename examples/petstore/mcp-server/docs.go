// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package main

import (
	_ "embed"
	"net/http"
)

//go:embed docs/index.html
var docsPage []byte

//go:embed docs/login.html
var docsLoginPage []byte

// docsController serves Swagger UI at GET /docs and the client-token gate
// at GET /docs/login (after APIPrefix). Both stay unauthenticated; the
// login page stores the token in sessionStorage so the UI can authorize
// GET /openapi.
type docsController struct{}

func (docsController) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /docs/login", serveDocsLogin)
	mux.HandleFunc("GET /docs", serveDocs)
	mux.HandleFunc("GET /docs/", serveDocs)
}

func serveDocs(w http.ResponseWriter, _ *http.Request) {
	writeHTML(w, docsPage)
}

func serveDocsLogin(w http.ResponseWriter, _ *http.Request) {
	writeHTML(w, docsLoginPage)
}

func writeHTML(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
