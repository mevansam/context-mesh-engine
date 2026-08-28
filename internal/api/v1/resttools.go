// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package apiv1

import (
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// restToolsBody converts an MCP tools/list result into JSON suitable for
// GET /tools. MCP protocol fields (_meta, resultType) are omitted; tool
// entries keep schemas and descriptions but not per-tool _meta.
func restToolsBody(res *mcp.ListToolsResult) (map[string]any, error) {
	b, err := json.Marshal(res)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	pruneRESTToolsMap(m)
	return m, nil
}

func pruneRESTToolsMap(m map[string]any) {
	delete(m, "_meta")
	delete(m, "resultType")
	tools, _ := m["tools"].([]any)
	for _, raw := range tools {
		if tm, ok := raw.(map[string]any); ok {
			delete(tm, "_meta")
		}
	}
}
