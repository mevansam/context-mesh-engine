// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

// Package httpserver owns the root HTTP mux, listener, and graceful shutdown.
package httpserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

// MuxOptions configures sibling mounting of MCP and REST handlers.
type MuxOptions struct {
	MCPPath    string
	APIPrefix  string
	MCPHandler http.Handler
	APIHandler http.Handler
	APITimeout time.Duration
	WrapMCP    func(http.Handler) http.Handler
	WrapREST   func(http.Handler) http.Handler
}

// NewMux mounts MCP and REST as siblings. A nil MCPHandler or APIHandler
// is not mounted. It does not StripPrefix on MCP. APITimeout wraps only
// the REST handler.
func NewMux(opts MuxOptions) http.Handler {
	mux := http.NewServeMux()

	if opts.MCPHandler != nil {
		h := opts.MCPHandler
		if opts.WrapMCP != nil {
			h = opts.WrapMCP(h)
		}
		mux.Handle(opts.MCPPath, h)
		mux.Handle(opts.MCPPath+"/", h)
	}

	if opts.APIHandler != nil {
		api := opts.APIHandler
		if opts.WrapREST != nil {
			api = opts.WrapREST(api)
		}
		if opts.APITimeout > 0 {
			api = http.TimeoutHandler(api, opts.APITimeout, "request timeout\n")
		}
		mux.Handle(opts.APIPrefix+"/", http.StripPrefix(opts.APIPrefix, api))
	}

	return mux
}

// ListenAndServe serves handler on addr until ctx is cancelled, then Shutdown.
// WriteTimeout is left unset so GET SSE can hang.
func ListenAndServe(ctx context.Context, addr string, handler http.Handler, readHeaderTimeout time.Duration) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			_ = srv.Close()
			return err
		}
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
