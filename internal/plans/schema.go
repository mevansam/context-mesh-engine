// Copyright 2026 Fidelity Investments. All rights reserved.
// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"encoding/json"

	"github.com/google/jsonschema-go/jsonschema"
	high "github.com/pb33f/libopenapi/datamodel/high/arazzo"
	"github.com/pb33f/libopenapi/orderedmap"
	"go.yaml.in/yaml/v4"
)

func objectSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "object"}
}

func nodeToSchema(n *yaml.Node) (*jsonschema.Schema, error) {
	if n == nil {
		return objectSchema(), nil
	}
	var v any
	if err := n.Decode(&v); err != nil {
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
		return map[string]any{"type": "object"}, nil
	}
	var v any
	if err := n.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
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
			Required: []string{"workflowId", "inputs"},
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
