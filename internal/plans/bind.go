// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"go.yaml.in/yaml/v4"
)

func workflowInputs(e *Entry, workflowID string) *yaml.Node {
	wf := workflowByID(e, workflowID)
	if wf == nil {
		return nil
	}
	return wf.Inputs
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

// SplitRESTOutputs copies REST/HTTP header x-source outputs onto response
// headers and removes them from the JSON body. MCP callers should use the
// full outputs map. Missing required header outputs return [ErrInternal].
func SplitRESTOutputs(e *Entry, workflowID string, outputs map[string]any) (body map[string]any, headers http.Header, err error) {
	_, params, err := splitOpenAPIOutputs(workflowByID(e, workflowID))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrInternal, err)
	}
	body = map[string]any{}
	for k, v := range outputs {
		body[k] = v
	}
	if len(params) == 0 {
		return body, nil, nil
	}
	headers = make(http.Header)
	for _, p := range params {
		v, ok := body[p.Key]
		if !ok || v == nil {
			if p.Required {
				return nil, nil, fmt.Errorf("%w: missing required output %q", ErrInternal, p.Key)
			}
			continue
		}
		s, err := formatHeaderValue(v)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: output %q: %w", ErrInternal, p.Key, err)
		}
		if s == "" {
			if p.Required {
				return nil, nil, fmt.Errorf("%w: missing required output %q", ErrInternal, p.Key)
			}
			delete(body, p.Key)
			continue
		}
		headers.Set(p.Name, s)
		delete(body, p.Key)
	}
	return body, headers, nil
}

func formatHeaderValue(v any) (string, error) {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t), nil
	case json.Number:
		return t.String(), nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
		return fmt.Sprint(t), nil
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}
