// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"encoding/json"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mevansam/context-mesh-engine/arazzo"
	high "github.com/pb33f/libopenapi/datamodel/high/arazzo"
	"github.com/pb33f/libopenapi/orderedmap"
	"go.yaml.in/yaml/v4"
)

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
	if n == nil {
		return closeConsumerInputSchema(map[string]any{"type": "object"}), nil
	}
	var v any
	if err := n.Decode(&v); err != nil {
		return nil, err
	}
	return closeConsumerInputSchema(stripReservedInputSchema(v)), nil
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
