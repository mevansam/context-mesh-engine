// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"context"
	"net/http"

	"github.com/mevansam/context-mesh-engine/arazzo"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type restRequestCtxKey struct{}

type restBindBox struct {
	Request    *http.Request
	PlanID     string
	Version    string
	WorkflowID string
}

// WithRESTRequest stores r so [Runner.Run] can bind REST/HTTP x-source inputs
// and emit REST/HTTP x-source outputs as response headers.
func WithRESTRequest(ctx context.Context, r *http.Request) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if r == nil {
		return ctx
	}
	return context.WithValue(ctx, restRequestCtxKey{}, &restBindBox{Request: r})
}

func restRequestFrom(ctx context.Context) *http.Request {
	if ctx == nil {
		return nil
	}
	switch v := ctx.Value(restRequestCtxKey{}).(type) {
	case *http.Request:
		return v
	case *restBindBox:
		if v == nil {
			return nil
		}
		return v.Request
	default:
		return nil
	}
}

func rememberRESTWorkflow(ctx context.Context, planID, version, workflowID string) {
	box, _ := ctx.Value(restRequestCtxKey{}).(*restBindBox)
	if box == nil {
		return
	}
	box.PlanID = planID
	box.Version = version
	box.WorkflowID = workflowID
}

func restWorkflowFrom(ctx context.Context) (planID, version, workflowID string, ok bool) {
	box, _ := ctx.Value(restRequestCtxKey{}).(*restBindBox)
	if box == nil || box.WorkflowID == "" {
		return "", "", "", false
	}
	return box.PlanID, box.Version, box.WorkflowID, true
}

// RESTWorkflowFrom is the plan/workflow Run recorded on a REST request context.
func RESTWorkflowFrom(ctx context.Context) (planID, version, workflowID string, ok bool) {
	return restWorkflowFrom(ctx)
}

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
