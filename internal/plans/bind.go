// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"fmt"
	"net/http"
	"strings"

	"go.yaml.in/yaml/v4"
)

func workflowInputs(e *Entry, workflowID string) *yaml.Node {
	if e == nil || e.Doc == nil {
		return nil
	}
	for _, wf := range e.Doc.Workflows {
		if wf != nil && wf.WorkflowId == workflowID {
			return wf.Inputs
		}
	}
	return nil
}

func bindRESTInputs(e *Entry, workflowID string, body map[string]any, r *http.Request) (map[string]any, error) {
	_, params, err := splitOpenAPIInputs(workflowInputs(e, workflowID))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInternal, err)
	}
	if len(params) == 0 {
		return body, nil
	}
	return mergeRESTInputs(body, r, params)
}

func mergeRESTInputs(body map[string]any, r *http.Request, params []restHTTPParam) (map[string]any, error) {
	out := map[string]any{}
	for k, v := range body {
		out[k] = v
	}
	for _, p := range params {
		if _, ok := out[p.Key]; ok {
			return nil, ErrUnexpectedInputs
		}
		val, ok := restValue(r, p)
		if !ok {
			if p.Required {
				return nil, ErrMissingInput
			}
			continue
		}
		out[p.Key] = val
	}
	return out, nil
}

func restValue(r *http.Request, p restHTTPParam) (string, bool) {
	if r == nil {
		return "", false
	}
	var raw string
	switch p.In {
	case "header":
		raw = r.Header.Get(p.Name)
	case "cookie":
		c, err := r.Cookie(p.Name)
		if err != nil {
			return "", false
		}
		raw = c.Value
	case "query":
		raw = r.URL.Query().Get(p.Name)
	default:
		return "", false
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	return raw, true
}
