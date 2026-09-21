// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"context"
	"strings"
	"testing"

	"github.com/mevansam/context-mesh-engine/arazzo"
	high "github.com/pb33f/libopenapi/datamodel/high/arazzo"
	"github.com/pb33f/libopenapi/orderedmap"
	"go.yaml.in/yaml/v4"
)

func TestNormalizeSecuritySchemes(t *testing.T) {
	_, err := NormalizeSecuritySchemes([]SecurityScheme{{Type: "oauth2"}})
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("empty name: %v", err)
	}
	_, err = NormalizeSecuritySchemes([]SecurityScheme{{
		Name: "planOAuth", Type: "oauth2",
		Flows: &OAuthFlows{ClientCredentials: &OAuthFlow{TokenURL: "http://auth/token"}},
	}, {
		Name: "planOAuth", Type: "http", Scheme: "bearer",
	}})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("dup: %v", err)
	}
	got, err := NormalizeSecuritySchemes([]SecurityScheme{{
		Name: "planOAuth", Type: "oauth2", Description: "Calling-application OAuth",
		Flows: &OAuthFlows{ClientCredentials: &OAuthFlow{
			TokenURL: "http://localhost:8092/oauth/token",
			Scopes:   map[string]string{"pets:read": "Find pets", "tools:list": "List tools"},
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Name != "planOAuth" || got[0].Type != "oauth2" {
		t.Fatalf("%#v", got)
	}
}

func TestNormalizeSecurity_UnknownScheme(t *testing.T) {
	schemes, err := NormalizeSecuritySchemes([]SecurityScheme{{Name: "planOAuth", Type: "http", Scheme: "bearer"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = NormalizeSecurity([]SecurityRequirement{{Schemes: []SchemeRef{{Name: "nope"}}}}, schemes)
	if err == nil || !strings.Contains(err.Error(), "unknown scheme") {
		t.Fatalf("unknown: %v", err)
	}
	_, err = NormalizeSecurity([]SecurityRequirement{{Schemes: []SchemeRef{{Name: "planOAuth", Scopes: []string{"pets:read"}}}}}, schemes)
	if err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeSecurity_OAuthScopeMustBeDeclared(t *testing.T) {
	schemes, err := NormalizeSecuritySchemes([]SecurityScheme{{
		Name: "planOAuth", Type: "oauth2",
		Flows: &OAuthFlows{ClientCredentials: &OAuthFlow{
			TokenURL: "http://auth/token",
			Scopes:   map[string]string{"pets:read": "Find pets"},
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = NormalizeSecurity([]SecurityRequirement{{
		Schemes: []SchemeRef{{Name: "planOAuth", Scopes: []string{"pets:write"}}},
	}}, schemes)
	if err == nil || !strings.Contains(err.Error(), "not declared") {
		t.Fatalf("undeclared scope: %v", err)
	}
}

func TestCheckSecurityCollisions(t *testing.T) {
	common := []RESTCommonParam{{Name: "Authorization", In: "header"}}
	schemes := []SecurityScheme{{Name: "clientBearer", Type: "http", Scheme: "bearer"}}
	err := CheckSecurityCollisions(nil, schemes, common)
	if err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("bearer vs Authorization: %v", err)
	}
}

func TestParseWorkflowSecurity(t *testing.T) {
	ext := orderedmap.New[string, *yaml.Node]()
	ext.Set(workflowSecurityExt, yamlMapping(t, `
- planOAuth: [pets:read]
`))
	wf := &high.Workflow{WorkflowId: "retrievePet", Extensions: ext}
	reqs, err := parseWorkflowSecurity(wf)
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 1 || len(reqs[0].Schemes) != 1 || reqs[0].Schemes[0].Name != "planOAuth" {
		t.Fatalf("%#v", reqs)
	}
	if got := reqs[0].Schemes[0].Scopes; len(got) != 1 || got[0] != "pets:read" {
		t.Fatalf("scopes = %#v", got)
	}
}

func TestCheckWorkflowScopes(t *testing.T) {
	ext := orderedmap.New[string, *yaml.Node]()
	ext.Set(workflowSecurityExt, yamlMapping(t, `
- planOAuth: [pets:write, orders:create]
`))
	wf := &high.Workflow{WorkflowId: "purchasePet", Extensions: ext}

	if err := checkWorkflowScopes(context.Background(), wf); err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("no token: %v", err)
	}

	ctx := withClientAuth(context.Background(), map[string]any{"scopes": []string{"pets:read"}})
	if err := checkWorkflowScopes(ctx, wf); err == nil || !strings.Contains(err.Error(), "insufficient scope") {
		t.Fatalf("missing scope: %v", err)
	}

	ctx = withClientAuth(context.Background(), map[string]any{"scopes": []string{"pets:write", "orders:create", "tools:list"}})
	if err := checkWorkflowScopes(ctx, wf); err != nil {
		t.Fatal(err)
	}

	if err := checkWorkflowScopes(context.Background(), &high.Workflow{WorkflowId: "ping"}); err != nil {
		t.Fatalf("no x-security: %v", err)
	}
}

func TestCheckWorkflowSecurity(t *testing.T) {
	src := []byte(inlinePlanPrefix + `
workflows:
  - workflowId: ping
    x-security:
      - planOAuth: [pets:read]
    steps:
      - stepId: s
        operationId: getHealth
        successCriteria:
          - condition: $statusCode == 200
`)
	c, err := Load(context.Background(), []arazzo.Loader{errLoader{srcs: []arazzo.Source{inlinePlanSource(t, src)}}}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckWorkflowSecurity(c, nil); err == nil || !strings.Contains(err.Error(), "requires OpenAPISecuritySchemes") {
		t.Fatalf("no schemes: %v", err)
	}
	schemes, err := NormalizeSecuritySchemes([]SecurityScheme{{
		Name: "planOAuth", Type: "oauth2",
		Flows: &OAuthFlows{ClientCredentials: &OAuthFlow{
			TokenURL: "http://auth/token",
			Scopes:   map[string]string{"pets:read": "Find pets"},
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckWorkflowSecurity(c, schemes); err != nil {
		t.Fatal(err)
	}

	bad := []byte(inlinePlanPrefix + `
workflows:
  - workflowId: ping
    x-security:
      - nope: []
    steps:
      - stepId: s
        operationId: getHealth
        successCriteria:
          - condition: $statusCode == 200
`)
	c, err = Load(context.Background(), []arazzo.Loader{errLoader{srcs: []arazzo.Source{inlinePlanSource(t, bad)}}}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckWorkflowSecurity(c, schemes); err == nil || !strings.Contains(err.Error(), "unknown scheme") {
		t.Fatalf("unknown: %v", err)
	}
}

func TestScopesSatisfied_OR(t *testing.T) {
	reqs := []SecurityRequirement{
		{Schemes: []SchemeRef{{Name: "a", Scopes: []string{"pets:write"}}}},
		{Schemes: []SchemeRef{{Name: "a", Scopes: []string{"pets:read"}}}},
	}
	if !scopesSatisfied([]string{"pets:read"}, reqs) {
		t.Fatal("OR should match second requirement")
	}
	if scopesSatisfied([]string{"tools:list"}, reqs) {
		t.Fatal("unrelated scope")
	}
}
