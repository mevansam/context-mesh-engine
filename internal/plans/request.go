// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"net/http"

	"github.com/mevansam/context-mesh-engine/arazzo"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RequestSourceFromHTTP builds a preprocessor source from a REST request.
func RequestSourceFromHTTP(r *http.Request) arazzo.RequestSource {
	src := arazzo.RequestSource{}
	if r != nil {
		src.Header = r.Header.Clone()
		src.ClientAuth = clientAuthMap(auth.TokenInfoFromContext(r.Context()))
	}
	return src
}

// RequestSourceFromMCP builds a preprocessor source from an MCP tool call.
func RequestSourceFromMCP(req *mcp.CallToolRequest) arazzo.RequestSource {
	src := arazzo.RequestSource{}
	if req == nil || req.Extra == nil {
		return src
	}
	if req.Extra.Header != nil {
		src.Header = req.Extra.Header.Clone()
	}
	src.ClientAuth = clientAuthMap(req.Extra.TokenInfo)
	return src
}

func clientAuthMap(ti *auth.TokenInfo) map[string]any {
	if ti == nil {
		return nil
	}
	m := map[string]any{}
	if ti.UserID != "" {
		m["userId"] = ti.UserID
	}
	if len(ti.Scopes) > 0 {
		m["scopes"] = append([]string(nil), ti.Scopes...)
	}
	if !ti.Expiration.IsZero() {
		m["expiration"] = ti.Expiration.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	for k, v := range ti.Extra {
		m[k] = v
	}
	return m
}
