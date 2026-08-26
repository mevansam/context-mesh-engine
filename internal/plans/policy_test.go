// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mevansam/context-mesh-engine/arazzo"
)

type staticPolicyLoader struct {
	n       atomic.Int32
	bundle  *arazzo.PolicyBundle
	err     error
	fail    atomic.Bool
	lastReq arazzo.PolicyRequest
}

func (s *staticPolicyLoader) Load(_ context.Context, req arazzo.PolicyRequest) (*arazzo.PolicyBundle, error) {
	s.n.Add(1)
	s.lastReq = req
	if s.fail.Load() {
		return nil, errors.New("down")
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.bundle, nil
}

func testPolicyCache(t *testing.T, l arazzo.PolicyLoader, ttl time.Duration) *PolicyCache {
	t.Helper()
	return NewPolicyCache(l, ttl, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestPolicy_InboundDenySkipsExecutor(t *testing.T) {
	c := loadPetstore(t)
	exec := &stubExec{}
	r := NewRunner(c, exec, nil)
	r.SetPolicy(testPolicyCache(t, &staticPolicyLoader{bundle: &arazzo.PolicyBundle{
		Inbound: []byte(`
package plan.inbound
import rego.v1
default allow := false
`),
	}}, time.Minute))
	_, err := r.Run(context.Background(), "petstore", "1.1.0", "pingHealth", map[string]any{"name": "x"})
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("err = %v", err)
	}
	if exec.n != 0 {
		t.Fatalf("executor calls = %d", exec.n)
	}
}

func TestPolicy_InboundAllowHintsAndStripCaller(t *testing.T) {
	c := loadPetstore(t)
	exec := &stubExec{}
	r := NewRunner(c, exec, nil)
	r.SetPolicy(testPolicyCache(t, &staticPolicyLoader{bundle: &arazzo.PolicyBundle{
		Inbound: []byte(`
package plan.inbound
import rego.v1
default allow := false
allow if input.workflowId == "pingHealth"
hints := {"mode": "read", "petStatus": "available"} if allow
`),
	}}, time.Minute))
	_, err := r.Run(context.Background(), "petstore", "1.1.0", "pingHealth", map[string]any{
		"name":                "x",
		arazzo.PolicyHintsKey: map[string]any{"mode": "forged"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPolicy_NoInboundDoesNotInjectHints(t *testing.T) {
	in := stripPolicyHints(map[string]any{"a": 1, arazzo.PolicyHintsKey: "x"})
	if _, ok := in[arazzo.PolicyHintsKey]; ok || in["a"] != 1 {
		t.Fatalf("%#v", in)
	}
}

func TestPolicy_OutboundDenyHidesOutputs(t *testing.T) {
	c := loadPetstore(t)
	exec := &stubExec{}
	r := NewRunner(c, exec, nil)
	r.SetPolicy(testPolicyCache(t, &staticPolicyLoader{bundle: &arazzo.PolicyBundle{
		Outbound: []byte(`
package plan.outbound
import rego.v1
default allow := false
`),
	}}, time.Minute))
	out, err := r.Run(context.Background(), "petstore", "1.1.0", "pingHealth", map[string]any{"name": "x"})
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("err = %v out=%v", err, out)
	}
	if out != nil {
		t.Fatalf("outputs = %#v", out)
	}
	if exec.n != 1 {
		t.Fatalf("executor calls = %d", exec.n)
	}
}

func TestPolicy_OutboundReplaceAndRedact(t *testing.T) {
	ctx := context.Background()
	repl, err := compileBundle(ctx, &arazzo.PolicyBundle{Outbound: []byte(`
package plan.outbound
import rego.v1
default allow := false
allow if true
outputs := {"only": true}
`)})
	if err != nil {
		t.Fatal(err)
	}
	got, err := applyOutbound(ctx, repl.outbound, "p", "1", "wf", nil, map[string]any{"secret": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["secret"]; ok || got["only"] != true {
		t.Fatalf("%#v", got)
	}

	red, err := compileBundle(ctx, &arazzo.PolicyBundle{Outbound: []byte(`
package plan.outbound
import rego.v1
default allow := false
allow if true
redact := ["/secret"]
mask := "***"
`)})
	if err != nil {
		t.Fatal(err)
	}
	got, err = applyOutbound(ctx, red.outbound, "p", "1", "wf", nil, map[string]any{"secret": "x", "ok": "y"})
	if err != nil {
		t.Fatal(err)
	}
	if got["secret"] != "***" || got["ok"] != "y" {
		t.Fatalf("%#v", got)
	}

	bad, err := compileBundle(ctx, &arazzo.PolicyBundle{Outbound: []byte(`
package plan.outbound
import rego.v1
default allow := false
allow if true
redact := ["not-a-pointer"]
`)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = applyOutbound(ctx, bad.outbound, "p", "1", "wf", nil, map[string]any{"a": 1})
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("err = %v", err)
	}
}

func TestPolicy_MissingAllowIsDeny(t *testing.T) {
	ctx := context.Background()
	cp, err := compileBundle(ctx, &arazzo.PolicyBundle{Inbound: []byte(`
package plan.inbound
import rego.v1
hints := {"x": 1}
`)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = applyInbound(ctx, cp.inbound, "p", "1", "wf", nil)
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("err = %v", err)
	}
}

func TestPolicy_LoadErrorFailClosed(t *testing.T) {
	c := loadPetstore(t)
	r := NewRunner(c, &stubExec{}, nil)
	r.SetPolicy(testPolicyCache(t, &staticPolicyLoader{err: errors.New("disk")}, time.Minute))
	_, err := r.Run(context.Background(), "petstore", "1.1.0", "pingHealth", map[string]any{"name": "x"})
	if !errors.Is(err, ErrPolicyLoad) {
		t.Fatalf("err = %v", err)
	}
}

func TestPolicy_CacheTTLAndStale(t *testing.T) {
	l := &staticPolicyLoader{bundle: &arazzo.PolicyBundle{Inbound: []byte(`
package plan.inbound
import rego.v1
default allow := false
allow if true
`)}}
	cache := testPolicyCache(t, l, 80*time.Millisecond)
	c := loadPetstore(t)
	r := NewRunner(c, &stubExec{}, nil)
	r.SetPolicy(cache)
	if _, err := r.Run(context.Background(), "petstore", "1.1.0", "pingHealth", map[string]any{"name": "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), "petstore", "1.1.0", "pingHealth", map[string]any{"name": "x"}); err != nil {
		t.Fatal(err)
	}
	if n := l.n.Load(); n != 1 {
		t.Fatalf("loads = %d", n)
	}
	time.Sleep(100 * time.Millisecond)
	l.fail.Store(true)
	if _, err := r.Run(context.Background(), "petstore", "1.1.0", "pingHealth", map[string]any{"name": "x"}); err != nil {
		t.Fatalf("stale should allow: %v", err)
	}
	if l.lastReq.PlanID != "petstore" || l.lastReq.Version != "1.1.0" {
		t.Fatalf("req = %+v", l.lastReq)
	}
}

func TestPolicy_NilLoaderSkipped(t *testing.T) {
	c := loadPetstore(t)
	r := NewRunner(c, &stubExec{}, nil)
	if _, err := r.Run(context.Background(), "petstore", "1.1.0", "pingHealth", map[string]any{"name": "x"}); err != nil {
		t.Fatal(err)
	}
}

func TestPolicy_InboundHTTPUserStatus(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/user/bob" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"username": "bob", "userStatus": 2})
	}))
	t.Cleanup(srv.Close)

	mod := []byte(`
package plan.inbound
import rego.v1

default allow := false

username := object.get(input.inputs, "username", "")

resp := http.send({
  "method": "GET",
  "url": concat("", [data.petstoreBase, "/user/", urlquery.encode(username)]),
  "raise_error": false,
}) if username != ""

default user_status := 1

user_status := to_number(object.get(resp.body, "userStatus", 1)) if {
  username != ""
  to_number(resp.status_code) == 200
  is_object(resp.body)
}

browse := {"retrievePet"}
buy := {"retrievePet", "purchasePet", "checkOrderStatus"}

allow if {
  user_status == 2
  input.workflowId in buy
}

allow if {
  user_status != 2
  input.workflowId in browse
}

hints := {"mode": "read", "petStatus": "available"} if user_status != 2
hints := {"mode": "buy", "petStatus": object.get(input.inputs, "status", "available")} if user_status == 2
`)
	data, _ := json.Marshal(map[string]any{"petstoreBase": srv.URL})
	cp, err := compileBundle(context.Background(), &arazzo.PolicyBundle{Inbound: mod, Data: data})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err = applyInbound(ctx, cp.inbound, "petstore", "0.0.1", "purchasePet", map[string]any{"username": "alice"})
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("alice purchase: %v", err)
	}
	in, err := applyInbound(ctx, cp.inbound, "petstore", "0.0.1", "retrievePet", map[string]any{"username": "alice", "status": "sold"})
	if err != nil {
		t.Fatal(err)
	}
	hints := in[arazzo.PolicyHintsKey].(map[string]any)
	if hints["petStatus"] != "available" {
		t.Fatalf("alice hints = %#v", hints)
	}
	in, err = applyInbound(ctx, cp.inbound, "petstore", "0.0.1", "purchasePet", map[string]any{"username": "bob", "status": "pending"})
	if err != nil {
		t.Fatal(err)
	}
	hints = in[arazzo.PolicyHintsKey].(map[string]any)
	if hints["mode"] != "buy" || hints["petStatus"] != "pending" {
		t.Fatalf("bob hints = %#v", hints)
	}
	if hits.Load() == 0 {
		t.Fatal("expected http.send")
	}
}

func TestDecisionAllow(t *testing.T) {
	if decisionAllow(nil) || decisionAllow(map[string]any{}) || decisionAllow(map[string]any{"allow": "true"}) {
		t.Fatal("expected deny")
	}
	if !decisionAllow(map[string]any{"allow": true}) {
		t.Fatal("expected allow")
	}
}
