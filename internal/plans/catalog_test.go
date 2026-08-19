// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package plans

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mevansam/context-mesh-engine/arazzo"
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
	r := NewRunner(c, nil)
	_, err := r.Run(context.Background(), "petstore", "1.1.0", "pingHealth", map[string]any{})
	if err != ErrNoExecutor {
		t.Fatalf("err = %v, want ErrNoExecutor", err)
	}

	exec := &stubExec{}
	r = NewRunner(c, exec)
	res, err := r.Run(context.Background(), "petstore", "1.1.0", "pingHealth", map[string]any{"name": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("success=false error=%s", res.Error)
	}
	if exec.n != 1 {
		t.Fatalf("executor calls = %d", exec.n)
	}

	_, err = r.Run(context.Background(), "nope", "1.0.0", "pingHealth", nil)
	if err == nil {
		t.Fatal("expected not found")
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
