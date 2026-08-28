// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const catalogOpenAPIVersion = "1.0.0"

var (
	listToolsSchemaOnce sync.Once
	listToolsSchema     any
	listToolsSchemaErr  error
)

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
		p := executePath(e.PlanID, wf.WorkflowId, e.VersionSegment(), latest)
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
						"description": "workflow outputs",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": outputsToJSONSchema(wf.Outputs),
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

// CatalogOpenAPIJSON builds an OAS 3.1 index for the loaded catalog.
// GET /tools uses a schema inferred from go-sdk [mcp.ListToolsResult].
// Each latest-plan execute path is a path-item $ref into ./ {planId}.
// queryEnabled adds POST /plans/query (MCP tool query).
func CatalogOpenAPIJSON(c *Catalog, queryEnabled bool) ([]byte, error) {
	toolsSchema, err := listToolsResultSchema()
	if err != nil {
		return nil, err
	}
	paths := map[string]any{
		"/tools": map[string]any{
			"get": map[string]any{
				"operationId": "listTools",
				"summary":     "List tools",
				"description": "REST equivalent of MCP JSON-RPC method tools/list. Response is mcp.ListToolsResult from the linked go-sdk (ttlMs, cacheScope, tools). Arazzo plan/query descriptions use REST templates.",
				"parameters": []any{
					map[string]any{
						"name":        "cursor",
						"in":          "query",
						"required":    false,
						"description": "MCP tools/list pagination cursor (ListToolsParams.cursor).",
						"schema":      map[string]any{"type": "string"},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "MCP ListToolsResult",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"$ref": "#/components/schemas/ListToolsResult",
								},
							},
						},
					},
				},
			},
		},
	}
	if queryEnabled {
		paths["/plans/query"] = map[string]any{
			"post": map[string]any{
				"operationId": "queryPlans",
				"summary":     "Query and execute a plan",
				"description": "REST equivalent of MCP tool query. Body is {query, data}; 200 is the selected workflow's outputs.",
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{
								"type":     "object",
								"required": []any{"query"},
								"properties": map[string]any{
									"query": map[string]any{"type": "string"},
									"data":  map[string]any{"type": "object", "additionalProperties": true},
								},
							},
						},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "workflow outputs",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"type": "object"},
							},
						},
					},
				},
			},
		}
	}
	if c != nil {
		ids := make([]string, 0, len(c.latest))
		for id := range c.latest {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, planID := range ids {
			e, ok := c.Latest(planID)
			if !ok || e.Doc == nil {
				continue
			}
			for _, wf := range e.Doc.Workflows {
				if wf == nil || wf.WorkflowId == "" {
					continue
				}
				p := executePath(e.PlanID, wf.WorkflowId, e.VersionSegment(), true)
				paths[p] = map[string]any{
					"$ref": "./" + planID + "#/paths/" + jsonPointerEscape(p),
				}
			}
		}
	}
	doc := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "Arazzo plan catalog",
			"version":     catalogOpenAPIVersion,
			"description": "Index of REST surfaces for this process. GET /tools is MCP tools/list. Plan execute paths $ref the latest child spec at ./ {planId} (GET /openapi/{planId}). Versioned child specs are GET /openapi/{planId}/v{version}.",
		},
		"paths": paths,
		"components": map[string]any{
			"schemas": map[string]any{
				"ListToolsResult": toolsSchema,
			},
		},
	}
	return json.Marshal(doc)
}

func executePath(planID, workflowID, versionSegment string, latest bool) string {
	if latest {
		return "/plans/" + planID + "/" + workflowID
	}
	return "/plans/" + planID + "/" + versionSegment + "/" + workflowID
}

func jsonPointerEscape(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	s = strings.ReplaceAll(s, "/", "~1")
	return s
}

func listToolsResultSchema() (any, error) {
	listToolsSchemaOnce.Do(func() {
		s, err := jsonschema.For[mcp.ListToolsResult](&jsonschema.ForOptions{
			TypeSchemas: map[reflect.Type]*jsonschema.Schema{
				reflect.TypeFor[any](): {
					Description: "JSON Schema object (MCP Tool.inputSchema / outputSchema)",
				},
			},
		})
		if err != nil {
			listToolsSchemaErr = err
			return
		}
		b, err := json.Marshal(s)
		if err != nil {
			listToolsSchemaErr = err
			return
		}
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			listToolsSchemaErr = err
			return
		}
		listToolsSchema = v
	})
	return listToolsSchema, listToolsSchemaErr
}
