// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package plans

import "encoding/json"

// OpenAPIJSON builds an OAS 3.1 document describing POST execute paths.
// versioned: paths include /plans/{planId}/{versionSegment}/{workflowId}
// latest: paths use /plans/{planId}/{workflowId}
func OpenAPIJSON(e *Entry, latest bool) ([]byte, error) {
	title := e.PlanID
	if e.Doc.Info != nil && e.Doc.Info.Title != "" {
		title = e.Doc.Info.Title
	}
	version := e.Version
	paths := map[string]any{}
	for _, wf := range e.Doc.Workflows {
		if wf == nil || wf.WorkflowId == "" {
			continue
		}
		schema, err := nodeToJSON(wf.Inputs)
		if err != nil {
			return nil, err
		}
		p := "/plans/" + e.PlanID + "/" + wf.WorkflowId
		if !latest {
			p = "/plans/" + e.PlanID + "/" + e.VersionSegment() + "/" + wf.WorkflowId
		}
		paths[p] = map[string]any{
			"post": map[string]any{
				"operationId": wf.WorkflowId,
				"summary":     wf.Summary,
				"description": wf.Description,
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": schema,
						},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "workflow result",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"workflowId": map[string]any{"type": "string"},
										"success":    map[string]any{"type": "boolean"},
										"inputs":     map[string]any{"type": "object"},
										"outputs":    map[string]any{"type": "object"},
										"steps":      map[string]any{"type": "array"},
										"error":      map[string]any{"type": "string"},
										"durationMs": map[string]any{"type": "integer"},
									},
								},
							},
						},
					},
				},
			},
		}
	}
	doc := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":   title,
			"version": version,
		},
		"paths": paths,
	}
	return json.Marshal(doc)
}
