// Copyright 2026 Fidelity Investments. All rights reserved.
// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mevansam/context-mesh-engine/arazzo"
	"go.yaml.in/yaml/v4"
)

func plansDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", "arazzo", "plans")
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func loadPetstore(t *testing.T) *Catalog {
	t.Helper()
	c, err := Load(context.Background(), []arazzo.Loader{arazzo.NewFileLoader(plansDir(t))}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLoad_SkipsMissingPlanIDAndPicksLatest(t *testing.T) {
	c := loadPetstore(t)
	if n := len(c.Entries()); n != 2 {
		t.Fatalf("entries = %d, want 2", n)
	}
	e, ok := c.Latest("petstore")
	if !ok {
		t.Fatal("missing latest petstore")
	}
	if e.Version != "1.1.0" {
		t.Fatalf("latest version = %q, want 1.1.0", e.Version)
	}
	if _, ok := c.Get("petstore", "1.0.0"); !ok {
		t.Fatal("missing 1.0.0")
	}
	if _, ok := c.GetBySegment("petstore", "v1.0.0"); !ok {
		t.Fatal("GetBySegment v1.0.0")
	}
	ids := e.WorkflowIDs()
	if len(ids) != 2 {
		t.Fatalf("latest workflows = %v", ids)
	}
}

func TestLoad_DuplicatePlanVersion(t *testing.T) {
	loader := arazzo.NewFileLoader(plansDir(t))
	_, err := Load(context.Background(), []arazzo.Loader{loader, loader}, discardLogger())
	if err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestPickLatest(t *testing.T) {
	if got := pickLatest([]string{"1.0.0", "1.1.0", "1.0.1"}); got != "1.1.0" {
		t.Fatalf("semver latest = %q", got)
	}
	if got := pickLatest([]string{"beta", "alpha"}); got != "beta" {
		t.Fatalf("lex latest = %q", got)
	}
}

type stubExec struct{ n int }

func (s *stubExec) Execute(_ context.Context, _ *arazzo.ExecutionRequest) (*arazzo.ExecutionResponse, error) {
	s.n++
	return &arazzo.ExecutionResponse{StatusCode: 200, Body: map[string]any{"ok": true}}, nil
}

func TestRunner_RunAndNoExecutor(t *testing.T) {
	c := loadPetstore(t)
	r := NewRunner(c, nil, nil)
	_, err := r.Run(context.Background(), "petstore", "1.1.0", "pingHealth", map[string]any{})
	if err != ErrNoExecutor {
		t.Fatalf("err = %v, want ErrNoExecutor", err)
	}

	exec := &stubExec{}
	r = NewRunner(c, exec, nil)
	res, err := r.Run(context.Background(), "petstore", "1.1.0", "pingHealth", map[string]any{"name": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("expected outputs map")
	}
	if exec.n != 1 {
		t.Fatalf("executor calls = %d", exec.n)
	}

	_, err = r.Run(context.Background(), "nope", "1.0.0", "pingHealth", nil)
	if err == nil {
		t.Fatal("expected not found")
	}
}

func TestRunner_QueryStub(t *testing.T) {
	c := loadPetstore(t)
	r := NewRunner(c, &stubExec{}, nil)
	if r.QueryEnabled() {
		t.Fatal("QueryEnabled with nil matcher")
	}
	_, err := r.Query(context.Background(), "check health", map[string]any{"name": "x"})
	if err != ErrQueryNotImplemented {
		t.Fatalf("err = %v, want ErrQueryNotImplemented", err)
	}
}

type staticMatcher struct {
	match *arazzo.QueryMatch
	err   error
}

func (s staticMatcher) Match(_ context.Context, _ arazzo.QueryRequest) (*arazzo.QueryMatch, error) {
	return s.match, s.err
}

func TestRunner_QueryMatcher(t *testing.T) {
	c := loadPetstore(t)
	exec := &stubExec{}

	r := NewRunner(c, exec, staticMatcher{match: &arazzo.QueryMatch{
		PlanID:     "petstore",
		WorkflowID: "pingHealth",
		Inputs:     map[string]any{"name": "from-match"},
	}})
	out, err := r.Query(context.Background(), "is the api up", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		t.Fatal("expected outputs")
	}
	if exec.n != 1 {
		t.Fatalf("executor calls = %d", exec.n)
	}

	r = NewRunner(c, exec, staticMatcher{match: &arazzo.QueryMatch{
		PlanID:     "petstore",
		Version:    "1.0.0",
		WorkflowID: "pingHealth",
	}})
	if _, err := r.Query(context.Background(), "v1", map[string]any{"name": "x"}); err != nil {
		t.Fatal(err)
	}

	r = NewRunner(c, exec, staticMatcher{match: &arazzo.QueryMatch{
		PlanID:     "unloaded",
		Version:    "9.9.9",
		WorkflowID: "pingHealth",
	}})
	_, err = r.Query(context.Background(), "global hit", nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("unloaded plan err = %v, want ErrNotFound", err)
	}

	r = NewRunner(c, exec, staticMatcher{match: &arazzo.QueryMatch{
		PlanID:     "petstore",
		WorkflowID: "noSuchWorkflow",
	}})
	_, err = r.Query(context.Background(), "bad workflow", nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("bad workflow err = %v, want ErrNotFound", err)
	}

	r = NewRunner(c, exec, staticMatcher{})
	_, err = r.Query(context.Background(), "nothing", nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("nil match err = %v, want ErrNotFound", err)
	}

	r = NewRunner(c, exec, staticMatcher{match: &arazzo.QueryMatch{PlanID: "petstore", WorkflowID: "pingHealth"}})
	_, err = r.Query(context.Background(), "   ", nil)
	if !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("empty query err = %v, want ErrEmptyQuery", err)
	}

	r = NewRunner(c, exec, staticMatcher{err: errors.New("index down")})
	_, err = r.Query(context.Background(), "anything", nil)
	if err == nil || err.Error() != "index down" {
		t.Fatalf("matcher err = %v, want index down", err)
	}
}

func TestCatalog_View(t *testing.T) {
	c := loadPetstore(t)
	view := c.View()
	s, ok := view.Latest("petstore")
	if !ok || s.PlanID != "petstore" || s.Version != "1.1.0" {
		t.Fatalf("latest = %+v ok=%v", s, ok)
	}
	s, ok = view.Get("petstore", "1.0.0")
	if !ok || s.Version != "1.0.0" {
		t.Fatalf("get 1.0.0 = %+v ok=%v", s, ok)
	}
	n := 0
	for range view.Plans() {
		n++
	}
	if n != 2 {
		t.Fatalf("Plans count = %d, want 2", n)
	}
}

func TestInputSchema_OneOf(t *testing.T) {
	c := loadPetstore(t)
	e, ok := c.Get("petstore", "1.1.0")
	if !ok {
		t.Fatal("missing 1.1.0")
	}
	s, err := InputSchema(e.Doc)
	if err != nil {
		t.Fatal(err)
	}
	if s.Type != "object" {
		t.Fatalf("type = %q", s.Type)
	}
	if len(s.OneOf) != 2 {
		t.Fatalf("oneOf = %d, want 2", len(s.OneOf))
	}
}

func TestOpenAPIJSON_LatestAndVersioned(t *testing.T) {
	c := loadPetstore(t)
	latest, ok := c.Latest("petstore")
	if !ok {
		t.Fatal("latest")
	}
	b, err := OpenAPIJSON(latest, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `/plans/petstore/pingHealth`) {
		t.Fatalf("latest paths: %s", b)
	}
	if strings.Contains(string(b), `"durationMs"`) || strings.Contains(string(b), `"workflowId"`) {
		t.Fatalf("200 schema should be workflow outputs, not the execution trace: %s", b)
	}
	if strings.Contains(string(b), `/plans/petstore/v1.1.0/`) {
		t.Fatalf("latest should not include version segment: %s", b)
	}
	b, err = OpenAPIJSON(latest, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `/plans/petstore/v1.1.0/echoName`) {
		t.Fatalf("versioned paths: %s", b)
	}
}

func TestNativeOutput_YAMLMapping(t *testing.T) {
	n := &yaml.Node{}
	if err := n.Encode(map[string]any{"id": 1, "name": "Dog", "status": "available"}); err != nil {
		t.Fatal(err)
	}
	got, ok := nativeOutput(n).(map[string]any)
	if !ok {
		t.Fatalf("got %T", nativeOutput(n))
	}
	if got["name"] != "Dog" || got["status"] != "available" {
		t.Fatalf("pet = %#v", got)
	}
	out := nativeOutputs(nil)
	if len(out) != 0 {
		t.Fatalf("nil outputs = %#v", out)
	}
}
