// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mevansam/context-mesh-engine/arazzo"
	"github.com/mevansam/context-mesh-engine/examples/petstore/petstore-auth-server/jwtx"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

// endUserHeader is the extra JWT the engine does not read. RequireBearerToken
// only sees Authorization. This header is verified in RequestPreprocessor.
const endUserHeader = "X-End-User-Token"

// jwtVerifier implements go-sdk auth.TokenVerifier for the calling-app JWT.
// Wired through engine.Options.MCPHandlerWrap / RESTHandlerWrap. A failed
// verify is 401 before the engine runner runs.
type jwtVerifier struct {
	secret []byte
}

func (v *jwtVerifier) verifyClient(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
	c, err := jwtx.ParseClient(v.secret, token)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", auth.ErrInvalidToken, err)
	}
	exp := time.Time{}
	if c.ExpiresAt != nil {
		exp = c.ExpiresAt.Time
	}
	return &auth.TokenInfo{
		UserID:     c.Subject,
		Expiration: exp,
		Extra: map[string]any{
			"tokenUse": jwtx.TokenUseClient,
			"sub":      c.Subject,
		},
	}, nil
}

// clientBearer is Options.MCPHandlerWrap (and the inner wrap for REST).
// RequireBearerToken runs jwtVerifier before the engine handler. Invalid
// Authorization is 401; the runner never sees the request.
func clientBearer(secret []byte) func(http.Handler) http.Handler {
	v := &jwtVerifier{secret: secret}
	return auth.RequireBearerToken(v.verifyClient, &auth.RequireBearerTokenOptions{
		AllowMissingExpiration: false,
	})
}

// wrapRESTPlans is Options.RESTHandlerWrap. After the engine StripPrefix of
// APIPrefix, paths are /health, /tools, /openapi/…, /plans/…. Bearer is
// required only on POST /plans/ (execute). Catalog GET stays unauthenticated.
func wrapRESTPlans(inner http.Handler, bearer func(http.Handler) http.Handler) http.Handler {
	protected := bearer(inner)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/plans/") {
			protected.ServeHTTP(w, r)
			return
		}
		inner.ServeHTTP(w, r)
	})
}

// dualJWTPreprocessor is Options.RequestPreprocessor (arazzo.RequestPreprocessor).
// The engine calls Process on REST execute/query and MCP run_*/query, then
// stores PolicyRequestContext on ctx. OPA sees input.auth / input.headers;
// those maps are not merged into workflow $inputs. Process error → 401.
type dualJWTPreprocessor struct {
	secret []byte
}

func (p *dualJWTPreprocessor) Process(_ context.Context, src arazzo.RequestSource) (*arazzo.PolicyRequestContext, error) {
	raw := ""
	if src.Header != nil {
		raw = strings.TrimSpace(src.Header.Get(endUserHeader))
	}
	if raw == "" {
		return nil, fmt.Errorf("missing %s", endUserHeader)
	}
	user, err := jwtx.ParseUser(p.secret, raw)
	if err != nil {
		return nil, fmt.Errorf("end-user token: %w", err)
	}
	headers := map[string]string{}
	if src.Header != nil {
		if id := strings.TrimSpace(src.Header.Get("X-Request-Id")); id != "" {
			headers["x-request-id"] = id
		}
	}
	authObj := map[string]any{
		"endUser": map[string]any{
			"username":   user.Username,
			"userStatus": user.UserStatus,
			"sub":        user.Subject,
		},
	}
	if src.ClientAuth != nil {
		authObj["client"] = src.ClientAuth
	}
	return &arazzo.PolicyRequestContext{Headers: headers, Auth: authObj}, nil
}
