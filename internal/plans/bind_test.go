// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mevansam/context-mesh-engine/arazzo"
	high "github.com/pb33f/libopenapi/datamodel/high/arazzo"
)

func TestMergeRESTInputs(t *testing.T) {
	n := yamlMapping(t, `
type: object
required: [requestId, status]
properties:
  requestId:
    type: string
    x-source:
      interface: rest
      protocol: http
      in: header
      name: x-request-id
  session:
    type: string
    x-source:
      interface: rest
      in: cookie
      name: sid
  status:
    type: string
    x-source:
      interface: rest
      protocol: http
      in: query
  note:
    type: string
`)
	_, params, err := splitOpenAPIInputs(n)
	if err != nil {
		t.Fatal(err)
	}
	if len(params) != 3 {
		t.Fatalf("params = %#v", params)
	}
	byKey := map[string]restHTTPParam{}
	for _, p := range params {
		byKey[p.Key] = p
	}
	if byKey["requestId"].Name != "x-request-id" || byKey["requestId"].In != "header" {
		t.Fatalf("requestId = %#v", byKey["requestId"])
	}
	if byKey["session"].Name != "sid" || byKey["session"].In != "cookie" {
		t.Fatalf("session = %#v", byKey["session"])
	}
	if byKey["status"].Name != "status" || byKey["status"].In != "query" {
		t.Fatalf("status = %#v", byKey["status"])
	}

	req := httptest.NewRequest(http.MethodPost, "/plans/p/wf?status=available", nil)
	req.Header.Set("X-Request-Id", "abc")
	req.AddCookie(&http.Cookie{Name: "sid", Value: "sess"})
	got, err := mergeRESTInputs(map[string]any{"note": "hi"}, req, params)
	if err != nil {
		t.Fatal(err)
	}
	if got["requestId"] != "abc" || got["session"] != "sess" || got["status"] != "available" || got["note"] != "hi" {
		t.Fatalf("merged = %#v", got)
	}

	_, err = mergeRESTInputs(map[string]any{"requestId": "from-body"}, req, params)
	if !errors.Is(err, ErrUnexpectedInputs) {
		t.Fatalf("body conflict err = %v", err)
	}

	missing := httptest.NewRequest(http.MethodPost, "/plans/p/wf", nil)
	_, err = mergeRESTInputs(nil, missing, params)
	if !errors.Is(err, ErrMissingInput) {
		t.Fatalf("missing required err = %v", err)
	}

	optional := httptest.NewRequest(http.MethodPost, "/plans/p/wf?status=sold", nil)
	optional.Header.Set("x-request-id", "id-1")
	got, err = mergeRESTInputs(nil, optional, params)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["session"]; ok {
		t.Fatalf("optional cookie should be omitted: %#v", got)
	}
	if got["requestId"] != "id-1" || got["status"] != "sold" {
		t.Fatalf("optional merge = %#v", got)
	}
}

func TestRunner_BindsRESTSources(t *testing.T) {
	c := loadPetstore(t)
	e, ok := c.Get("petstore", "1.1.0")
	if !ok {
		t.Fatal("1.1.0")
	}
	for _, wf := range e.Doc.Workflows {
		if wf != nil && wf.WorkflowId == "pingHealth" {
			wf.Inputs = yamlMapping(t, `
type: object
required: [requestId]
properties:
  requestId:
    type: string
    x-source:
      interface: rest
      protocol: http
      in: header
      name: x-request-id
  name:
    type: string
`)
		}
	}
	r := NewRunner(c, &stubExec{}, nil)
	req := httptest.NewRequest(http.MethodPost, "/plans/petstore/pingHealth", nil)
	req.Header.Set("X-Request-Id", "rid")
	ctx := WithRESTRequest(context.Background(), req)
	if _, err := r.Run(ctx, "petstore", "1.1.0", "pingHealth", map[string]any{"name": "x"}); err != nil {
		t.Fatal(err)
	}

	_, err := r.Run(ctx, "petstore", "1.1.0", "pingHealth", map[string]any{
		"name":      "x",
		"requestId": "from-body",
	})
	if !errors.Is(err, ErrUnexpectedInputs) {
		t.Fatalf("body rest key err = %v", err)
	}

	_, err = r.Run(WithRESTRequest(context.Background(), httptest.NewRequest(http.MethodPost, "/", nil)),
		"petstore", "1.1.0", "pingHealth", map[string]any{"name": "x"})
	if !errors.Is(err, ErrMissingInput) {
		t.Fatalf("missing header err = %v", err)
	}

	if _, err := r.Run(context.Background(), "petstore", "1.1.0", "pingHealth", map[string]any{
		"name":      "mcp",
		"requestId": "from-json",
	}); err != nil {
		t.Fatalf("MCP JSON should keep requestId: %v", err)
	}

	qr := NewRunner(c, &stubExec{}, staticMatcher{match: &arazzo.QueryMatch{
		PlanID:     "petstore",
		WorkflowID: "pingHealth",
		Inputs:     map[string]any{"name": "q"},
	}})
	qreq := httptest.NewRequest(http.MethodPost, "/plans/query", nil)
	qreq.Header.Set("X-Request-Id", "qid")
	if _, err := qr.Query(WithRESTRequest(context.Background(), qreq), "is up", nil); err != nil {
		t.Fatalf("REST query bind: %v", err)
	}
}

func TestBindRESTInputs_NoParams(t *testing.T) {
	e := &Entry{
		Doc: &high.Arazzo{
			Workflows: []*high.Workflow{{
				WorkflowId: "pingHealth",
				Inputs: yamlMapping(t, `
type: object
properties:
  name:
    type: string
`),
			}},
		},
	}
	body := map[string]any{"name": "x"}
	got, err := bindRESTInputs(e, "pingHealth", body, httptest.NewRequest(http.MethodPost, "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	if got["name"] != "x" {
		t.Fatalf("got = %#v", got)
	}
}
