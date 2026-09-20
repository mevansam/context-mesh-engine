// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mevansam/context-mesh-engine/arazzo"
	high "github.com/pb33f/libopenapi/datamodel/high/arazzo"
	"github.com/pb33f/libopenapi/orderedmap"
	"go.yaml.in/yaml/v4"
)

const (
	inputSourceExt      = "x-source"
	outputSchemaExt     = "x-outputs"
	sourceInterfaceREST = "rest"
	sourceInterfaceMCP  = "mcp"
	sourceProtocolHTTP  = "http"
	sourceInHeader      = "header"
	sourceInCookie      = "cookie"
)

// RESTCommonParam is a header or cookie documented on generated REST
// OpenAPI. It is not bound to Arazzo $inputs.
type RESTCommonParam struct {
	Name        string
	In          string // header or cookie
	Required    bool
	Description string
	Schema      map[string]any
}

// restHTTPParam is an OpenAPI parameter lifted from a workflow input
// property whose x-source is interface=rest, protocol=http, or from
// [RESTCommonParam].
type restHTTPParam struct {
	Key         string // Arazzo property name ($inputs.{Key}); empty for common params
	Name        string // HTTP parameter name (x-source.name, or Key)
	In          string
	Required    bool
	Description string
	Schema      map[string]any
}

func objectSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "object", AdditionalProperties: schemaFalse()}
}

func schemaFalse() *jsonschema.Schema {
	return &jsonschema.Schema{Not: &jsonschema.Schema{}}
}

func nodeToSchema(n *yaml.Node) (*jsonschema.Schema, error) {
	v, err := nodeToJSON(n)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	s := &jsonschema.Schema{}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, err
	}
	if s.Type == "" && len(s.OneOf) == 0 && len(s.AnyOf) == 0 && len(s.AllOf) == 0 {
		s.Type = "object"
	}
	return s, nil
}

func nodeToJSON(n *yaml.Node) (any, error) {
	v, err := decodeConsumerInputSchema(n)
	if err != nil {
		return nil, err
	}
	return stripVendorInputAttrs(v), nil
}

func decodeConsumerInputSchema(n *yaml.Node) (any, error) {
	if n == nil {
		return closeConsumerInputSchema(map[string]any{"type": "object"}), nil
	}
	var v any
	if err := n.Decode(&v); err != nil {
		return nil, err
	}
	return closeConsumerInputSchema(stripReservedInputSchema(v)), nil
}

// stripVendorInputAttrs removes x-source from consumer JSON Schema so MCP
// inputSchema and leftover OAS body schemas stay plain JSON Schema.
func stripVendorInputAttrs(v any) any {
	switch t := v.(type) {
	case map[string]any:
		delete(t, inputSourceExt)
		if props, ok := t["properties"].(map[string]any); ok {
			for k, p := range props {
				props[k] = stripVendorInputAttrs(p)
			}
		}
		if items, ok := t["items"]; ok {
			t["items"] = stripVendorInputAttrs(items)
		}
		if ap, ok := t["additionalProperties"]; ok {
			t["additionalProperties"] = stripVendorInputAttrs(ap)
		}
		for _, key := range []string{"oneOf", "anyOf", "allOf", "prefixItems"} {
			if arr, ok := t[key].([]any); ok {
				for i, item := range arr {
					arr[i] = stripVendorInputAttrs(item)
				}
			}
		}
		for _, key := range []string{"not", "if", "then", "else"} {
			if child, ok := t[key]; ok {
				t[key] = stripVendorInputAttrs(child)
			}
		}
		for _, key := range []string{"$defs", "definitions"} {
			if defs, ok := t[key].(map[string]any); ok {
				for k, def := range defs {
					defs[k] = stripVendorInputAttrs(def)
				}
			}
		}
		return t
	case []any:
		for i, item := range t {
			t[i] = stripVendorInputAttrs(item)
		}
		return t
	default:
		return v
	}
}

// splitOpenAPIInputs lifts top-level REST/HTTP x-source properties into
// OpenAPI parameters. Remaining properties stay on the JSON body schema.
func splitOpenAPIInputs(n *yaml.Node) (any, []restHTTPParam, error) {
	v, err := decodeConsumerInputSchema(n)
	if err != nil {
		return nil, nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return stripVendorInputAttrs(v), nil, nil
	}
	props, _ := m["properties"].(map[string]any)
	if len(props) == 0 {
		return stripVendorInputAttrs(v), nil, nil
	}
	drop := map[string]struct{}{}
	var params []restHTTPParam
	seen := map[string]struct{}{}
	for _, key := range schemaPropertyKeys(n) {
		seen[key] = struct{}{}
		prop, exists := props[key]
		if !exists {
			continue
		}
		param, lifted := liftRESTParam(key, prop, requiredHas(m["required"], key))
		if !lifted {
			continue
		}
		params = append(params, param)
		delete(props, key)
		drop[key] = struct{}{}
	}
	for key, prop := range props {
		if _, ok := seen[key]; ok {
			continue
		}
		param, lifted := liftRESTParam(key, prop, requiredHas(m["required"], key))
		if !lifted {
			continue
		}
		params = append(params, param)
		delete(props, key)
		drop[key] = struct{}{}
	}
	if len(drop) > 0 {
		if req, ok := m["required"]; ok {
			m["required"] = filterRequiredKeys(req, drop)
		}
	}
	if len(props) == 0 {
		delete(m, "properties")
	}
	if req, ok := m["required"]; ok && requiredEmpty(req) {
		delete(m, "required")
	}
	return stripVendorInputAttrs(m), params, nil
}

func liftRESTParam(key string, prop any, required bool) (restHTTPParam, bool) {
	pm, ok := prop.(map[string]any)
	if !ok {
		return restHTTPParam{}, false
	}
	src, ok := parseInputSource(pm[inputSourceExt])
	if !ok || src.Interface != sourceInterfaceREST {
		return restHTTPParam{}, false
	}
	protocol := src.Protocol
	if protocol == "" {
		protocol = sourceProtocolHTTP
	}
	if protocol != sourceProtocolHTTP {
		return restHTTPParam{}, false
	}
	switch src.In {
	case "header", "cookie", "query":
	default:
		return restHTTPParam{}, false
	}
	name := src.Name
	if name == "" {
		name = key
	}
	delete(pm, inputSourceExt)
	schema, _ := stripVendorInputAttrs(pm).(map[string]any)
	if schema == nil {
		schema = map[string]any{}
	}
	return restHTTPParam{Key: key, Name: name, In: src.In, Required: required, Schema: schema}, true
}

type inputSource struct {
	Interface string
	Protocol  string
	In        string
	Name      string
}

func parseInputSource(v any) (inputSource, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return inputSource{}, false
	}
	return inputSource{
		Interface: strings.ToLower(strings.TrimSpace(asString(m["interface"]))),
		Protocol:  strings.ToLower(strings.TrimSpace(asString(m["protocol"]))),
		In:        strings.ToLower(strings.TrimSpace(asString(m["in"]))),
		Name:      strings.TrimSpace(asString(m["name"])),
	}, true
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func schemaPropertyKeys(n *yaml.Node) []string {
	content := yamlMappingContent(n)
	for i := 0; i+1 < len(content); i += 2 {
		if yamlScalar(content[i]) != "properties" {
			continue
		}
		pc := yamlMappingContent(content[i+1])
		keys := make([]string, 0, len(pc)/2)
		for j := 0; j+1 < len(pc); j += 2 {
			keys = append(keys, yamlScalar(pc[j]))
		}
		return keys
	}
	return nil
}

func yamlMappingContent(n *yaml.Node) []*yaml.Node {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return nil
	}
	return n.Content
}

func yamlScalar(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	if n.Kind == yaml.ScalarNode {
		return n.Value
	}
	var s string
	if err := n.Decode(&s); err == nil {
		return s
	}
	return ""
}

func requiredHas(v any, key string) bool {
	switch req := v.(type) {
	case []any:
		for _, item := range req {
			if s, ok := item.(string); ok && s == key {
				return true
			}
		}
	case []string:
		for _, s := range req {
			if s == key {
				return true
			}
		}
	}
	return false
}

func filterRequiredKeys(v any, drop map[string]struct{}) any {
	switch req := v.(type) {
	case []any:
		out := make([]any, 0, len(req))
		for _, item := range req {
			s, ok := item.(string)
			if ok {
				if _, ok := drop[s]; ok {
					continue
				}
			}
			out = append(out, item)
		}
		return out
	case []string:
		out := make([]string, 0, len(req))
		for _, s := range req {
			if _, ok := drop[s]; ok {
				continue
			}
			out = append(out, s)
		}
		return out
	default:
		return v
	}
}

func requiredEmpty(v any) bool {
	switch req := v.(type) {
	case []any:
		return len(req) == 0
	case []string:
		return len(req) == 0
	default:
		return false
	}
}

func restParamsJSON(params []restHTTPParam) []any {
	out := make([]any, 0, len(params))
	for _, p := range params {
		item := map[string]any{
			"name":     p.Name,
			"in":       p.In,
			"required": p.Required,
			"schema":   p.Schema,
		}
		if d := strings.TrimSpace(p.Description); d != "" {
			item["description"] = d
		}
		out = append(out, item)
	}
	return out
}

func restCommonParamKey(in, name string) string {
	in = strings.ToLower(strings.TrimSpace(in))
	name = strings.TrimSpace(name)
	if in == sourceInHeader {
		return in + ":" + strings.ToLower(name)
	}
	return in + ":" + name
}

// NormalizeRESTCommonParams validates host-configured header/cookie params
// for OpenAPI. Name is required. In must be header or cookie. Duplicate
// names fail (headers compared case-insensitively). Empty Schema becomes
// {type: string}. These params are not bound to $inputs.
func NormalizeRESTCommonParams(params []RESTCommonParam) ([]restHTTPParam, error) {
	out := make([]restHTTPParam, 0, len(params))
	seen := map[string]string{}
	for i, p := range params {
		name := strings.TrimSpace(p.Name)
		in := strings.ToLower(strings.TrimSpace(p.In))
		if name == "" {
			return nil, fmt.Errorf("CommonRESTParams[%d]: name is required", i)
		}
		if in != sourceInHeader && in != sourceInCookie {
			return nil, fmt.Errorf("CommonRESTParams[%d]: in must be header or cookie", i)
		}
		key := restCommonParamKey(in, name)
		if prev, ok := seen[key]; ok {
			return nil, fmt.Errorf("CommonRESTParams: duplicate %s %q (also %q)", in, name, prev)
		}
		seen[key] = name
		schema := p.Schema
		if schema == nil {
			schema = map[string]any{"type": "string"}
		} else {
			schema = cloneMap(schema)
			if schema == nil {
				schema = map[string]any{"type": "string"}
			}
		}
		out = append(out, restHTTPParam{
			Name:        name,
			In:          in,
			Required:    p.Required,
			Description: strings.TrimSpace(p.Description),
			Schema:      schema,
		})
	}
	return out, nil
}

// CheckCommonParamCollisions fails when a common REST param uses the same
// in+name as a workflow x-source REST/HTTP lift.
func CheckCommonParamCollisions(c *Catalog, params []RESTCommonParam) error {
	if c == nil || len(params) == 0 {
		return nil
	}
	common, err := NormalizeRESTCommonParams(params)
	if err != nil {
		return err
	}
	keys := map[string]string{}
	for _, p := range common {
		keys[restCommonParamKey(p.In, p.Name)] = p.Name
	}
	for _, e := range c.Entries() {
		if e == nil || e.Doc == nil {
			continue
		}
		for _, wf := range e.Doc.Workflows {
			if wf == nil || wf.WorkflowId == "" {
				continue
			}
			_, lifted, err := splitOpenAPIInputs(wf.Inputs)
			if err != nil {
				return fmt.Errorf("%s %s: %w", e.PlanID, wf.WorkflowId, err)
			}
			for _, p := range lifted {
				key := restCommonParamKey(p.In, p.Name)
				if prev, ok := keys[key]; ok {
					return fmt.Errorf("workflow %s: x-source %s %q collides with CommonRESTParams %q", wf.WorkflowId, p.In, p.Name, prev)
				}
			}
		}
	}
	return nil
}

func withCommonParameters(op map[string]any, common []restHTTPParam) {
	if op == nil || len(common) == 0 {
		return
	}
	existing, _ := op["parameters"].([]any)
	op["parameters"] = append(restParamsJSON(common), existing...)
}

func (m OpenAPIMeta) normalizedCommonParams() ([]restHTTPParam, error) {
	if len(m.CommonParams) == 0 {
		return nil, nil
	}
	return NormalizeRESTCommonParams(m.CommonParams)
}

func hasJSONRequestBody(body any, lifted bool) bool {
	if !lifted {
		return true
	}
	m, ok := body.(map[string]any)
	if !ok {
		return true
	}
	props, _ := m["properties"].(map[string]any)
	return len(props) > 0
}

// stripReservedInputSchema removes engine-injected input names from a JSON
// Schema object. Only this schema level and combinators ($defs, oneOf, …)
// are walked so a nested consumer field named secrets is kept.
func stripReservedInputSchema(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	if props, ok := m["properties"].(map[string]any); ok {
		for k := range props {
			if arazzo.ReservedInputKey(k) {
				delete(props, k)
			}
		}
	}
	if req, ok := m["required"]; ok {
		m["required"] = filterReservedRequired(req)
	}
	for _, key := range []string{"oneOf", "anyOf", "allOf"} {
		if arr, ok := m[key].([]any); ok {
			for i, item := range arr {
				arr[i] = stripReservedInputSchema(item)
			}
		}
	}
	for _, key := range []string{"not", "if", "then", "else"} {
		if child, ok := m[key]; ok {
			m[key] = stripReservedInputSchema(child)
		}
	}
	for _, key := range []string{"$defs", "definitions"} {
		if defs, ok := m[key].(map[string]any); ok {
			for k, def := range defs {
				defs[k] = stripReservedInputSchema(def)
			}
		}
	}
	return m
}

// closeConsumerInputSchema sets additionalProperties: false on this schema
// object and combinators. Nested property schemas are not closed.
func closeConsumerInputSchema(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	if isConsumerObjectSchema(m) {
		m["additionalProperties"] = false
	}
	for _, key := range []string{"oneOf", "anyOf", "allOf"} {
		if arr, ok := m[key].([]any); ok {
			for i, item := range arr {
				arr[i] = closeConsumerInputSchema(item)
			}
		}
	}
	for _, key := range []string{"not", "if", "then", "else"} {
		if child, ok := m[key]; ok {
			m[key] = closeConsumerInputSchema(child)
		}
	}
	for _, key := range []string{"$defs", "definitions"} {
		if defs, ok := m[key].(map[string]any); ok {
			for k, def := range defs {
				defs[k] = closeConsumerInputSchema(def)
			}
		}
	}
	return m
}

func isConsumerObjectSchema(m map[string]any) bool {
	if t, ok := m["type"].(string); ok && t != "" && t != "object" {
		return false
	}
	return true
}

func consumerInputKeys(schema any) map[string]struct{} {
	m, ok := schema.(map[string]any)
	if !ok {
		return map[string]struct{}{}
	}
	keys := map[string]struct{}{}
	if props, ok := m["properties"].(map[string]any); ok {
		for k := range props {
			keys[k] = struct{}{}
		}
	}
	for _, key := range []string{"oneOf", "anyOf", "allOf"} {
		if arr, ok := m[key].([]any); ok {
			for _, item := range arr {
				for k := range consumerInputKeys(item) {
					keys[k] = struct{}{}
				}
			}
		}
	}
	return keys
}

func filterReservedRequired(v any) any {
	switch req := v.(type) {
	case []any:
		out := make([]any, 0, len(req))
		for _, item := range req {
			s, ok := item.(string)
			if ok && arazzo.ReservedInputKey(s) {
				continue
			}
			out = append(out, item)
		}
		return out
	case []string:
		out := make([]string, 0, len(req))
		for _, s := range req {
			if arazzo.ReservedInputKey(s) {
				continue
			}
			out = append(out, s)
		}
		return out
	default:
		return v
	}
}

func consumerFacingText(s string) string {
	if arazzo.LeaksReservedInputs(s) {
		return ""
	}
	return s
}

type schemaLoc int

const (
	schemaLocRoot schemaLoc = iota
	schemaLocTopProp
	schemaLocNested
)

func validateWorkflowIOSources(wf *high.Workflow) error {
	if wf == nil {
		return nil
	}
	if err := validateXSourcePlacement(wf.Inputs, "inputs"); err != nil {
		return err
	}
	return validateOutputSources(wf)
}

func validateXSourcePlacement(n *yaml.Node, root string) error {
	if n == nil {
		return nil
	}
	var v any
	if err := n.Decode(&v); err != nil {
		return fmt.Errorf("%s: %w", root, err)
	}
	return walkXSource(v, root, schemaLocRoot)
}

func walkXSource(v any, path string, loc schemaLoc) error {
	switch t := v.(type) {
	case map[string]any:
		if _, has := t[inputSourceExt]; has && loc != schemaLocTopProp {
			return fmt.Errorf("%s: x-source is only allowed on top-level properties", path)
		}
		if props, ok := t["properties"].(map[string]any); ok {
			child := schemaLocNested
			if loc == schemaLocRoot {
				child = schemaLocTopProp
			}
			for k, p := range props {
				if err := walkXSource(p, path+".properties."+k, child); err != nil {
					return err
				}
			}
		}
		if items, ok := t["items"]; ok {
			if err := walkXSource(items, path+".items", schemaLocNested); err != nil {
				return err
			}
		}
		if ap, ok := t["additionalProperties"]; ok {
			if _, isBool := ap.(bool); !isBool {
				if err := walkXSource(ap, path+".additionalProperties", schemaLocNested); err != nil {
					return err
				}
			}
		}
		for _, key := range []string{"oneOf", "anyOf", "allOf", "prefixItems"} {
			if arr, ok := t[key].([]any); ok {
				for i, item := range arr {
					if err := walkXSource(item, fmt.Sprintf("%s.%s[%d]", path, key, i), loc); err != nil {
						return err
					}
				}
			}
		}
		for _, key := range []string{"not", "if", "then", "else"} {
			if child, ok := t[key]; ok {
				if err := walkXSource(child, path+"."+key, loc); err != nil {
					return err
				}
			}
		}
		for _, key := range []string{"$defs", "definitions"} {
			if defs, ok := t[key].(map[string]any); ok {
				for k, def := range defs {
					if err := walkXSource(def, path+"."+key+"."+k, schemaLocNested); err != nil {
						return err
					}
				}
			}
		}
		return nil
	case []any:
		for i, item := range t {
			if err := walkXSource(item, fmt.Sprintf("%s[%d]", path, i), loc); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}

func validateOutputSources(wf *high.Workflow) error {
	n := workflowOutputSchemaNode(wf)
	if n == nil {
		return nil
	}
	if err := validateXSourcePlacement(n, outputSchemaExt); err != nil {
		return err
	}
	_, _, err := splitOpenAPIOutputs(wf)
	return err
}

func workflowOutputSchemaNode(wf *high.Workflow) *yaml.Node {
	if wf == nil || wf.Extensions == nil {
		return nil
	}
	n, ok := wf.Extensions.Get(outputSchemaExt)
	if !ok || n == nil {
		return nil
	}
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		return n.Content[0]
	}
	return n
}

func outputNameSet(outputs *orderedmap.Map[string, string]) map[string]struct{} {
	names := map[string]struct{}{}
	if orderedmap.Len(outputs) == 0 {
		return names
	}
	for pair := outputs.First(); pair != nil; pair = pair.Next() {
		names[pair.Key()] = struct{}{}
	}
	return names
}

func workflowByID(e *Entry, workflowID string) *high.Workflow {
	if e == nil || e.Doc == nil {
		return nil
	}
	for _, wf := range e.Doc.Workflows {
		if wf != nil && wf.WorkflowId == workflowID {
			return wf
		}
	}
	return nil
}

// splitOpenAPIOutputs lifts top-level REST/HTTP header x-source properties
// from workflow x-outputs into OpenAPI response headers. Remaining output
// names stay on the JSON body schema.
func splitOpenAPIOutputs(wf *high.Workflow) (any, []restHTTPParam, error) {
	body := outputsToJSONSchema(nil)
	if wf != nil {
		body = outputsToJSONSchema(wf.Outputs)
	}
	n := workflowOutputSchemaNode(wf)
	if n == nil {
		return body, nil, nil
	}
	var raw any
	if err := n.Decode(&raw); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", outputSchemaExt, err)
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("%s must be a JSON Schema object", outputSchemaExt)
	}
	var outputs *orderedmap.Map[string, string]
	if wf != nil {
		outputs = wf.Outputs
	}
	names := outputNameSet(outputs)
	props, _ := m["properties"].(map[string]any)
	bodyProps, _ := body["properties"].(map[string]any)
	if bodyProps == nil {
		bodyProps = map[string]any{}
		body["properties"] = bodyProps
	}
	for key := range props {
		if _, ok := names[key]; !ok {
			return nil, nil, fmt.Errorf("%s.properties.%s is not a workflow output", outputSchemaExt, key)
		}
	}
	if req, ok := m["required"]; ok {
		for _, key := range requiredStrings(req) {
			if _, ok := names[key]; !ok {
				return nil, nil, fmt.Errorf("%s.required lists %q, which is not a workflow output", outputSchemaExt, key)
			}
		}
	}
	drop := map[string]struct{}{}
	var params []restHTTPParam
	seenHeader := map[string]string{}
	seenKey := map[string]struct{}{}
	handle := func(key string, prop any) error {
		if _, ok := seenKey[key]; ok {
			return nil
		}
		seenKey[key] = struct{}{}
		param, lifted, err := liftOutputRESTParam(key, prop, requiredHas(m["required"], key))
		if err != nil {
			return err
		}
		stripped, _ := stripVendorInputAttrs(cloneMap(prop)).(map[string]any)
		if stripped == nil {
			stripped = map[string]any{}
		}
		if lifted {
			canon := strings.ToLower(param.Name)
			if prev, ok := seenHeader[canon]; ok {
				return fmt.Errorf("outputs %q and %q use the same header %q", prev, key, param.Name)
			}
			seenHeader[canon] = key
			params = append(params, param)
			delete(bodyProps, key)
			drop[key] = struct{}{}
			return nil
		}
		bodyProps[key] = stripped
		return nil
	}
	for _, key := range schemaPropertyKeys(n) {
		prop, exists := props[key]
		if !exists {
			continue
		}
		if err := handle(key, prop); err != nil {
			return nil, nil, err
		}
	}
	for key, prop := range props {
		if err := handle(key, prop); err != nil {
			return nil, nil, err
		}
	}
	if req, ok := m["required"]; ok {
		filtered := filterRequiredKeys(req, drop)
		if !requiredEmpty(filtered) {
			body["required"] = filtered
		} else {
			delete(body, "required")
		}
	}
	if len(bodyProps) == 0 {
		delete(body, "properties")
	}
	if req, ok := body["required"]; ok && requiredEmpty(req) {
		delete(body, "required")
	}
	return body, params, nil
}

func liftOutputRESTParam(key string, prop any, required bool) (restHTTPParam, bool, error) {
	pm, ok := prop.(map[string]any)
	if !ok {
		return restHTTPParam{}, false, nil
	}
	raw, has := pm[inputSourceExt]
	if !has {
		return restHTTPParam{}, false, nil
	}
	src, ok := parseInputSource(raw)
	if !ok {
		return restHTTPParam{}, false, fmt.Errorf("outputs.%s: x-source must be an object", key)
	}
	if src.Interface == sourceInterfaceMCP {
		return restHTTPParam{}, false, nil
	}
	if src.Interface != sourceInterfaceREST {
		return restHTTPParam{}, false, fmt.Errorf("outputs.%s: x-source.interface must be rest or mcp", key)
	}
	protocol := src.Protocol
	if protocol == "" {
		protocol = sourceProtocolHTTP
	}
	if protocol != sourceProtocolHTTP {
		return restHTTPParam{}, false, fmt.Errorf("outputs.%s: x-source.protocol must be http", key)
	}
	if src.In != sourceInHeader {
		return restHTTPParam{}, false, fmt.Errorf("outputs.%s: x-source.in must be header", key)
	}
	name := src.Name
	if name == "" {
		name = key
	}
	cloned := cloneMap(pm)
	delete(cloned, inputSourceExt)
	schema, _ := stripVendorInputAttrs(cloned).(map[string]any)
	if schema == nil {
		schema = map[string]any{}
	}
	return restHTTPParam{Key: key, Name: name, In: sourceInHeader, Required: required, Schema: schema}, true, nil
}

func cloneMap(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, val := range m {
		out[k] = val
	}
	return out
}

func requiredStrings(v any) []string {
	switch req := v.(type) {
	case []any:
		out := make([]string, 0, len(req))
		for _, item := range req {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string(nil), req...)
	default:
		return nil
	}
}

func restHeadersJSON(params []restHTTPParam) map[string]any {
	out := map[string]any{}
	for _, p := range params {
		out[p.Name] = map[string]any{
			"required": p.Required,
			"schema":   p.Schema,
		}
	}
	return out
}

// outputsToJSONSchema builds a JSON Schema object from Arazzo workflow
// output names. Values in the spec are runtime expressions, not types.
func outputsToJSONSchema(outputs *orderedmap.Map[string, string]) map[string]any {
	schema := map[string]any{"type": "object"}
	if orderedmap.Len(outputs) == 0 {
		return schema
	}
	props := map[string]any{}
	for pair := outputs.First(); pair != nil; pair = pair.Next() {
		props[pair.Key()] = map[string]any{}
	}
	schema["properties"] = props
	return schema
}

// InputSchema builds the MCP tool input schema for a plan.
// Each oneOf branch is {workflowId: const, inputs: that workflow's schema}
// so overlapping Arazzo input schemas still validate uniquely.
func InputSchema(doc *high.Arazzo) (*jsonschema.Schema, error) {
	var oneOf []*jsonschema.Schema
	for _, wf := range doc.Workflows {
		if wf == nil || wf.WorkflowId == "" {
			continue
		}
		s, err := nodeToSchema(wf.Inputs)
		if err != nil {
			return nil, err
		}
		if s.Title == "" {
			s.Title = wf.WorkflowId
		}
		id := any(wf.WorkflowId)
		oneOf = append(oneOf, &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"workflowId": {Const: &id},
				"inputs":     s,
			},
			Required:             []string{"workflowId", "inputs"},
			AdditionalProperties: schemaFalse(),
		})
	}
	if len(oneOf) == 0 {
		oneOf = []*jsonschema.Schema{objectSchema()}
	}
	return &jsonschema.Schema{
		Type:  "object",
		OneOf: oneOf,
	}, nil
}
