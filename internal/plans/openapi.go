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

const defaultOpenAPIPrefix = "/api"

// OpenAPIMeta is host URL data for generated OAS documents.
type OpenAPIMeta struct {
	// ServerURL is the Try-it-out origin (PublicBaseURL + APIPrefix),
	// for example http://localhost:8080/api. Empty uses APIPrefix only.
	ServerURL string
	// APIPrefix is the REST prefix (default /api). Catalog plan $refs are
	// prefix-absolute so they resolve from GET /openapi (no trailing slash).
	APIPrefix string
}

func (m OpenAPIMeta) prefix() string {
	p := m.APIPrefix
	if p == "" {
		p = defaultOpenAPIPrefix
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}

func (m OpenAPIMeta) serverURL() string {
	if strings.TrimSpace(m.ServerURL) != "" {
		return strings.TrimRight(m.ServerURL, "/")
	}
	return m.prefix()
}

func (m OpenAPIMeta) applyServers(doc map[string]any) {
	doc["servers"] = []any{map[string]any{"url": m.serverURL()}}
}

func (m OpenAPIMeta) planSpecRef(planID, path string) string {
	return m.prefix() + "/openapi/" + planID + "#/paths/" + jsonPointerEscape(path)
}

// OpenAPIServerURL joins PublicBaseURL and APIPrefix for OAS servers[].url.
func OpenAPIServerURL(publicBase, apiPrefix string) string {
	p := OpenAPIMeta{APIPrefix: apiPrefix}.prefix()
	b := strings.TrimRight(publicBase, "/")
	if b == "" {
		return p
	}
	return b + p
}

var (
	listToolsSchemaOnce sync.Once
	listToolsSchema     any
	listToolsSchemaErr  error
)

// OpenAPIJSON builds an OAS 3.1 document describing POST execute paths.
// versioned: paths include /plans/{planId}/{versionSegment}/{workflowId}
// latest: paths use /plans/{planId}/{workflowId}
func OpenAPIJSON(e *Entry, latest bool, meta OpenAPIMeta) ([]byte, error) {
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
		post := map[string]any{
			"operationId": wf.WorkflowId,
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
		}
		if s := consumerFacingText(wf.Summary); s != "" {
			post["summary"] = s
		}
		if d := consumerFacingText(wf.Description); d != "" {
			post["description"] = d
		}
		p := executePath(e.PlanID, wf.WorkflowId, e.VersionSegment(), latest)
		paths[p] = map[string]any{"post": post}
	}
	doc := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":   title,
			"version": version,
		},
		"paths": paths,
	}
	meta.applyServers(doc)
	return json.Marshal(doc)
}

// CatalogOpenAPIJSON builds an OAS 3.1 index for the loaded catalog.
// GET /tools uses a schema inferred from go-sdk [mcp.ListToolsResult].
// Each latest-plan execute path is a path-item $ref into
// {APIPrefix}/openapi/{planId} (prefix-absolute, so GET /openapi without a
// trailing slash still resolves). queryEnabled adds POST /plans/query.
func CatalogOpenAPIJSON(c *Catalog, queryEnabled bool, meta OpenAPIMeta) ([]byte, error) {
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
					"$ref": meta.planSpecRef(planID, p),
				}
			}
		}
	}
	doc := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "Arazzo plan catalog",
			"version":     catalogOpenAPIVersion,
			"description": "Index of REST surfaces for this process. GET /tools is MCP tools/list. Plan execute paths $ref the latest child spec at {APIPrefix}/openapi/{planId}. Versioned child specs are GET /openapi/{planId}/v{version}.",
		},
		"paths": paths,
		"components": map[string]any{
			"schemas": map[string]any{
				"ListToolsResult": toolsSchema,
			},
		},
	}
	meta.applyServers(doc)
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
