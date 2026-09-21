// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"context"
	"fmt"
	"strings"

	high "github.com/pb33f/libopenapi/datamodel/high/arazzo"
	"go.yaml.in/yaml/v4"
)

const (
	workflowSecurityExt = "x-security"
	securityTypeHTTP    = "http"
	securityTypeAPIKey  = "apiKey"
	securityTypeOAuth2  = "oauth2"
	securityTypeOIDC    = "openIdConnect"
)

// SecurityScheme is an OAS 3.1 security scheme documented on generated REST
// OpenAPI. It is not enforced by the engine; wraps still verify tokens.
type SecurityScheme struct {
	Name             string
	Type             string // http, apiKey, oauth2, openIdConnect
	Description      string
	Scheme           string // http: bearer | basic
	BearerFormat     string
	In               string // apiKey: header | query | cookie
	APIKeyName       string
	Flows            *OAuthFlows
	OpenIDConnectURL string
}

// OAuthFlows is the OAS oauth2 flows object.
type OAuthFlows struct {
	Implicit          *OAuthFlow
	Password          *OAuthFlow
	ClientCredentials *OAuthFlow
	AuthorizationCode *OAuthFlow
}

// OAuthFlow is one OAS oauth2 flow.
type OAuthFlow struct {
	AuthorizationURL string
	TokenURL         string
	RefreshURL       string
	Scopes           map[string]string
}

// SchemeRef is one name in an OAS security requirement object.
type SchemeRef struct {
	Name   string
	Scopes []string
}

// SecurityRequirement is one OAS security requirement (AND of SchemeRefs).
// A slice of requirements is OR.
type SecurityRequirement struct {
	Schemes []SchemeRef
}

// NormalizeSecuritySchemes validates host-configured OAS security schemes.
func NormalizeSecuritySchemes(schemes []SecurityScheme) ([]SecurityScheme, error) {
	out := make([]SecurityScheme, 0, len(schemes))
	seen := map[string]struct{}{}
	for i, s := range schemes {
		name := strings.TrimSpace(s.Name)
		typ := strings.TrimSpace(s.Type)
		if name == "" {
			return nil, fmt.Errorf("OpenAPISecuritySchemes[%d]: name is required", i)
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("OpenAPISecuritySchemes: duplicate name %q", name)
		}
		seen[name] = struct{}{}
		s.Name = name
		s.Type = typ
		s.Description = strings.TrimSpace(s.Description)
		if err := validateSecurityScheme(i, s); err != nil {
			return nil, err
		}
		switch s.Type {
		case securityTypeHTTP:
			s.Scheme = strings.ToLower(strings.TrimSpace(s.Scheme))
			s.BearerFormat = strings.TrimSpace(s.BearerFormat)
		case securityTypeAPIKey:
			s.In = strings.ToLower(strings.TrimSpace(s.In))
			s.APIKeyName = strings.TrimSpace(s.APIKeyName)
		case securityTypeOIDC:
			s.OpenIDConnectURL = strings.TrimSpace(s.OpenIDConnectURL)
		}
		out = append(out, s)
	}
	return out, nil
}

func validateSecurityScheme(i int, s SecurityScheme) error {
	prefix := fmt.Sprintf("OpenAPISecuritySchemes[%d]", i)
	switch s.Type {
	case securityTypeHTTP:
		scheme := strings.ToLower(strings.TrimSpace(s.Scheme))
		if scheme != "bearer" && scheme != "basic" {
			return fmt.Errorf("%s: http scheme must be bearer or basic", prefix)
		}
	case securityTypeAPIKey:
		in := strings.ToLower(strings.TrimSpace(s.In))
		if in != sourceInHeader && in != sourceInCookie && in != "query" {
			return fmt.Errorf("%s: apiKey in must be header, query, or cookie", prefix)
		}
		if strings.TrimSpace(s.APIKeyName) == "" {
			return fmt.Errorf("%s: apiKey name is required", prefix)
		}
	case securityTypeOAuth2:
		if s.Flows == nil {
			return fmt.Errorf("%s: oauth2 flows is required", prefix)
		}
		n := 0
		if err := validateOAuthFlow(prefix+".flows.implicit", s.Flows.Implicit, true, false); err != nil {
			return err
		}
		if s.Flows.Implicit != nil {
			n++
		}
		if err := validateOAuthFlow(prefix+".flows.password", s.Flows.Password, false, true); err != nil {
			return err
		}
		if s.Flows.Password != nil {
			n++
		}
		if err := validateOAuthFlow(prefix+".flows.clientCredentials", s.Flows.ClientCredentials, false, true); err != nil {
			return err
		}
		if s.Flows.ClientCredentials != nil {
			n++
		}
		if err := validateOAuthFlow(prefix+".flows.authorizationCode", s.Flows.AuthorizationCode, true, true); err != nil {
			return err
		}
		if s.Flows.AuthorizationCode != nil {
			n++
		}
		if n == 0 {
			return fmt.Errorf("%s: oauth2 flows must set at least one flow", prefix)
		}
	case securityTypeOIDC:
		if strings.TrimSpace(s.OpenIDConnectURL) == "" {
			return fmt.Errorf("%s: openIdConnectUrl is required", prefix)
		}
	default:
		return fmt.Errorf("%s: type must be http, apiKey, oauth2, or openIdConnect", prefix)
	}
	return nil
}

func validateOAuthFlow(path string, f *OAuthFlow, needAuth, needToken bool) error {
	if f == nil {
		return nil
	}
	if needAuth && strings.TrimSpace(f.AuthorizationURL) == "" {
		return fmt.Errorf("%s: authorizationUrl is required", path)
	}
	if needToken && strings.TrimSpace(f.TokenURL) == "" {
		return fmt.Errorf("%s: tokenUrl is required", path)
	}
	return nil
}

// NormalizeSecurity validates document-level security requirements against
// already-normalized schemes. Empty reqs is valid (schemes documented only).
func NormalizeSecurity(reqs []SecurityRequirement, schemes []SecurityScheme) ([]SecurityRequirement, error) {
	byName := securitySchemeMap(schemes)
	out := make([]SecurityRequirement, 0, len(reqs))
	for i, r := range reqs {
		if len(r.Schemes) == 0 {
			return nil, fmt.Errorf("OpenAPISecurity[%d]: at least one scheme is required", i)
		}
		nr := SecurityRequirement{Schemes: make([]SchemeRef, 0, len(r.Schemes))}
		for j, ref := range r.Schemes {
			name := strings.TrimSpace(ref.Name)
			if name == "" {
				return nil, fmt.Errorf("OpenAPISecurity[%d].Schemes[%d]: name is required", i, j)
			}
			s, ok := byName[name]
			if !ok {
				return nil, fmt.Errorf("OpenAPISecurity[%d]: unknown scheme %q", i, name)
			}
			if err := validateRequirementScopes(fmt.Sprintf("OpenAPISecurity[%d].Schemes[%d]", i, j), s, ref.Scopes); err != nil {
				return nil, err
			}
			nr.Schemes = append(nr.Schemes, SchemeRef{Name: name, Scopes: append([]string(nil), ref.Scopes...)})
		}
		out = append(out, nr)
	}
	return out, nil
}

func validateRequirementScopes(path string, s SecurityScheme, scopes []string) error {
	if s.Type != securityTypeOAuth2 && s.Type != securityTypeOIDC {
		return nil
	}
	if s.Type == securityTypeOIDC {
		return nil
	}
	allowed := oauthScopeSet(s)
	for _, sc := range scopes {
		sc = strings.TrimSpace(sc)
		if sc == "" {
			continue
		}
		if _, ok := allowed[sc]; !ok {
			return fmt.Errorf("%s: scope %q is not declared on scheme %q", path, sc, s.Name)
		}
	}
	return nil
}

func oauthScopeSet(s SecurityScheme) map[string]struct{} {
	out := map[string]struct{}{}
	if s.Flows == nil {
		return out
	}
	for _, f := range []*OAuthFlow{s.Flows.Implicit, s.Flows.Password, s.Flows.ClientCredentials, s.Flows.AuthorizationCode} {
		if f == nil {
			continue
		}
		for k := range f.Scopes {
			out[k] = struct{}{}
		}
	}
	return out
}

func securitySchemeMap(schemes []SecurityScheme) map[string]SecurityScheme {
	out := map[string]SecurityScheme{}
	for _, s := range schemes {
		out[s.Name] = s
	}
	return out
}

// CheckSecurityCollisions fails when a security scheme uses the same HTTP
// location as a CommonRESTParam or an x-source REST lift.
func CheckSecurityCollisions(c *Catalog, schemes []SecurityScheme, common []RESTCommonParam) error {
	occupied := map[string]string{}
	for _, p := range common {
		occupied[restCommonParamKey(p.In, p.Name)] = "CommonRESTParams " + p.Name
	}
	if c != nil {
		for _, e := range c.Entries() {
			if e == nil || e.Doc == nil {
				continue
			}
			for _, wf := range e.Doc.Workflows {
				if wf == nil {
					continue
				}
				_, lifted, err := splitOpenAPIInputs(wf.Inputs)
				if err != nil {
					return err
				}
				for _, p := range lifted {
					occupied[restCommonParamKey(p.In, p.Name)] = "x-source " + p.Name
				}
			}
		}
	}
	for _, s := range schemes {
		var key, label string
		switch s.Type {
		case securityTypeHTTP:
			if strings.EqualFold(strings.TrimSpace(s.Scheme), "bearer") {
				key = restCommonParamKey(sourceInHeader, "Authorization")
				label = "http bearer Authorization"
			}
		case securityTypeAPIKey:
			key = restCommonParamKey(s.In, s.APIKeyName)
			label = "apiKey " + s.APIKeyName
		}
		if key == "" {
			continue
		}
		if prev, ok := occupied[key]; ok {
			return fmt.Errorf("OpenAPISecuritySchemes %q (%s) collides with %s", s.Name, label, prev)
		}
	}
	return nil
}

func validateWorkflowSecurityShape(wf *high.Workflow) error {
	_, err := parseWorkflowSecurity(wf)
	return err
}

func parseWorkflowSecurity(wf *high.Workflow) ([]SecurityRequirement, error) {
	if wf == nil || wf.Extensions == nil {
		return nil, nil
	}
	n, ok := wf.Extensions.Get(workflowSecurityExt)
	if !ok || n == nil {
		return nil, nil
	}
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	var raw any
	if err := n.Decode(&raw); err != nil {
		return nil, fmt.Errorf("%s: %w", workflowSecurityExt, err)
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a sequence of security requirement objects", workflowSecurityExt)
	}
	var out []SecurityRequirement
	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be an object", workflowSecurityExt, i)
		}
		if len(m) == 0 {
			return nil, fmt.Errorf("%s[%d]: at least one scheme is required", workflowSecurityExt, i)
		}
		req := SecurityRequirement{}
		for name, v := range m {
			name = strings.TrimSpace(name)
			if name == "" {
				return nil, fmt.Errorf("%s[%d]: scheme name is required", workflowSecurityExt, i)
			}
			scopes, err := stringList(v)
			if err != nil {
				return nil, fmt.Errorf("%s[%d].%s: %w", workflowSecurityExt, i, name, err)
			}
			req.Schemes = append(req.Schemes, SchemeRef{Name: name, Scopes: scopes})
		}
		out = append(out, req)
	}
	return out, nil
}

func stringList(v any) ([]string, error) {
	switch t := v.(type) {
	case nil:
		return []string{}, nil
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("scopes must be an array of strings")
			}
			out = append(out, s)
		}
		return out, nil
	case []string:
		return append([]string(nil), t...), nil
	default:
		return nil, fmt.Errorf("scopes must be an array of strings")
	}
}

// CheckWorkflowSecurity ensures each workflow x-security names configured
// schemes and oauth2 scopes declared on those schemes.
func CheckWorkflowSecurity(c *Catalog, schemes []SecurityScheme) error {
	if c == nil {
		return nil
	}
	byName := securitySchemeMap(schemes)
	for _, e := range c.Entries() {
		if e == nil || e.Doc == nil {
			continue
		}
		for _, wf := range e.Doc.Workflows {
			if wf == nil || wf.WorkflowId == "" {
				continue
			}
			reqs, err := parseWorkflowSecurity(wf)
			if err != nil {
				return fmt.Errorf("%s workflow %s: %w", e.PlanID, wf.WorkflowId, err)
			}
			if len(reqs) == 0 {
				continue
			}
			if len(schemes) == 0 {
				return fmt.Errorf("%s workflow %s: %s requires OpenAPISecuritySchemes", e.PlanID, wf.WorkflowId, workflowSecurityExt)
			}
			for _, req := range reqs {
				for _, ref := range req.Schemes {
					s, ok := byName[ref.Name]
					if !ok {
						return fmt.Errorf("%s workflow %s: %s unknown scheme %q", e.PlanID, wf.WorkflowId, workflowSecurityExt, ref.Name)
					}
					if err := validateRequirementScopes(workflowSecurityExt, s, ref.Scopes); err != nil {
						return fmt.Errorf("%s workflow %s: %w", e.PlanID, wf.WorkflowId, err)
					}
				}
			}
		}
	}
	return nil
}

func securityJSON(reqs []SecurityRequirement) []any {
	out := make([]any, 0, len(reqs))
	for _, r := range reqs {
		m := map[string]any{}
		for _, ref := range r.Schemes {
			scopes := ref.Scopes
			if scopes == nil {
				scopes = []string{}
			}
			m[ref.Name] = scopes
		}
		out = append(out, m)
	}
	return out
}

func securitySchemesJSON(schemes []SecurityScheme) map[string]any {
	out := map[string]any{}
	for _, s := range schemes {
		out[s.Name] = securitySchemeJSON(s)
	}
	return out
}

func securitySchemeJSON(s SecurityScheme) map[string]any {
	m := map[string]any{"type": s.Type}
	if s.Description != "" {
		m["description"] = s.Description
	}
	switch s.Type {
	case securityTypeHTTP:
		m["scheme"] = strings.ToLower(strings.TrimSpace(s.Scheme))
		if bf := strings.TrimSpace(s.BearerFormat); bf != "" {
			m["bearerFormat"] = bf
		}
	case securityTypeAPIKey:
		m["in"] = strings.ToLower(strings.TrimSpace(s.In))
		m["name"] = strings.TrimSpace(s.APIKeyName)
	case securityTypeOAuth2:
		m["flows"] = oauthFlowsJSON(s.Flows)
	case securityTypeOIDC:
		m["openIdConnectUrl"] = strings.TrimSpace(s.OpenIDConnectURL)
	}
	return m
}

func oauthFlowsJSON(f *OAuthFlows) map[string]any {
	out := map[string]any{}
	if f == nil {
		return out
	}
	if f.Implicit != nil {
		out["implicit"] = oauthFlowJSON(f.Implicit, true, false)
	}
	if f.Password != nil {
		out["password"] = oauthFlowJSON(f.Password, false, true)
	}
	if f.ClientCredentials != nil {
		out["clientCredentials"] = oauthFlowJSON(f.ClientCredentials, false, true)
	}
	if f.AuthorizationCode != nil {
		out["authorizationCode"] = oauthFlowJSON(f.AuthorizationCode, true, true)
	}
	return out
}

func oauthFlowJSON(f *OAuthFlow, auth, token bool) map[string]any {
	m := map[string]any{}
	if auth {
		m["authorizationUrl"] = strings.TrimSpace(f.AuthorizationURL)
	}
	if token {
		m["tokenUrl"] = strings.TrimSpace(f.TokenURL)
	}
	if u := strings.TrimSpace(f.RefreshURL); u != "" {
		m["refreshUrl"] = u
	}
	scopes := map[string]any{}
	for k, v := range f.Scopes {
		scopes[k] = v
	}
	m["scopes"] = scopes
	return m
}

func applySecuritySchemes(doc map[string]any, schemes []SecurityScheme) {
	if len(schemes) == 0 {
		return
	}
	comps, _ := doc["components"].(map[string]any)
	if comps == nil {
		comps = map[string]any{}
		doc["components"] = comps
	}
	comps["securitySchemes"] = securitySchemesJSON(schemes)
}

func applyDocumentSecurity(doc map[string]any, reqs []SecurityRequirement) {
	if len(reqs) == 0 {
		return
	}
	doc["security"] = securityJSON(reqs)
}

func operationSecurity(wf *high.Workflow, fallback []SecurityRequirement) []SecurityRequirement {
	reqs, err := parseWorkflowSecurity(wf)
	if err != nil || len(reqs) == 0 {
		return fallback
	}
	return reqs
}

func requirementNeedsAuth(reqs []SecurityRequirement) bool {
	return len(reqs) > 0
}

func scopesSatisfied(have []string, reqs []SecurityRequirement) bool {
	set := map[string]struct{}{}
	for _, s := range have {
		s = strings.TrimSpace(s)
		if s != "" {
			set[s] = struct{}{}
		}
	}
	for _, req := range reqs {
		ok := true
		for _, ref := range req.Schemes {
			for _, sc := range ref.Scopes {
				sc = strings.TrimSpace(sc)
				if sc == "" {
					continue
				}
				if _, hit := set[sc]; !hit {
					ok = false
					break
				}
			}
			if !ok {
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func checkWorkflowScopes(ctx context.Context, wf *high.Workflow) error {
	reqs, err := parseWorkflowSecurity(wf)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInternal, err)
	}
	if !requirementNeedsAuth(reqs) {
		return nil
	}
	scopes, ok := clientScopes(ctx)
	if !ok {
		return fmt.Errorf("%w: missing client token", ErrUnauthorized)
	}
	if scopesSatisfied(scopes, reqs) {
		return nil
	}
	id := ""
	if wf != nil {
		id = wf.WorkflowId
	}
	return fmt.Errorf("%w: workflow %s", ErrInsufficientScope, id)
}
