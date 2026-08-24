// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mevansam/context-mesh-engine/arazzo"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

const (
	petstoreLocalBase   = "http://localhost:8090/api/v3"
	petstoreHostedBase  = "https://petstore3.swagger.io/api/v3"
	defaultPetstoreBase = petstoreLocalBase
	defaultAsyncBase    = "http://localhost:8091"
	confirmWait         = 6 * time.Second
)

// resolvePetstoreBase maps -petstore local|hosted to an origin.
// urlOverride (-petstore-url) wins when set.
func resolvePetstoreBase(kind, urlOverride string) (string, error) {
	if u := strings.TrimSpace(urlOverride); u != "" {
		return strings.TrimRight(u, "/"), nil
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "local":
		return petstoreLocalBase, nil
	case "hosted":
		return petstoreHostedBase, nil
	default:
		return "", fmt.Errorf("-petstore must be local or hosted, got %q", kind)
	}
}

type httpExec struct {
	client    *http.Client
	petstore  []string
	asyncBase string
}

func newHTTPExec(asyncBase, petstoreBase string) *httpExec {
	if asyncBase == "" {
		asyncBase = defaultAsyncBase
	}
	if petstoreBase == "" {
		petstoreBase = defaultPetstoreBase
	}
	return &httpExec{
		client:    &http.Client{Timeout: 20 * time.Second},
		petstore:  []string{strings.TrimRight(petstoreBase, "/")},
		asyncBase: strings.TrimRight(asyncBase, "/"),
	}
}

func (e *httpExec) Execute(ctx context.Context, req *arazzo.ExecutionRequest) (*arazzo.ExecutionResponse, error) {
	method, path, err := resolveHTTP(req)
	if err != nil {
		return nil, err
	}
	bases := e.petstore
	poll := false
	if isAsyncSource(req) {
		bases = []string{e.asyncBase}
		poll = strings.EqualFold(method, http.MethodGet)
	}

	var last *arazzo.ExecutionResponse
	var lastErr error
	deadline := time.Now().Add(confirmWait)
	for {
		for _, base := range bases {
			resp, err := e.do(ctx, base, method, path, req)
			if err != nil {
				lastErr = err
				continue
			}
			last = resp
			if resp.StatusCode >= 500 {
				lastErr = fmt.Errorf("%s returned %d", resp.URL, resp.StatusCode)
				continue
			}
			if resp.StatusCode == http.StatusNotFound && !poll {
				lastErr = fmt.Errorf("%s returned %d", resp.URL, resp.StatusCode)
				continue
			}
			if poll && resp.StatusCode == http.StatusNotFound && time.Now().Before(deadline) {
				lastErr = fmt.Errorf("confirmation not ready")
				break
			}
			return resp, nil
		}
		if !poll || !time.Now().Before(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	if last != nil {
		return last, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no backend responded")
}

func (e *httpExec) do(ctx context.Context, base, method, path string, req *arazzo.ExecutionRequest) (*arazzo.ExecutionResponse, error) {
	u, err := url.Parse(strings.TrimRight(base, "/") + path)
	if err != nil {
		return nil, err
	}

	q := u.Query()
	header := make(http.Header)
	header.Set("Accept", "application/json")
	used := map[string]bool{}

	if req.Source != nil && req.Source.OpenAPIDocument != nil {
		applyOpenAPIParams(req.Source.OpenAPIDocument, method, path, req.Parameters, u, q, header, used)
	}
	for name, value := range req.Parameters {
		if used[name] {
			continue
		}
		s := stringify(value)
		switch strings.ToLower(name) {
		case "authorization", "api_key", "api-key", "ordercorrelationid", "orderrequestid":
			header.Set(name, s)
		default:
			q.Set(name, s)
		}
	}
	u.RawQuery = q.Encode()

	var body io.Reader
	if req.RequestBody != nil {
		raw, err := json.Marshal(req.RequestBody)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
		ct := req.ContentType
		if ct == "" {
			ct = "application/json"
		}
		header.Set("Content-Type", ct)
	}

	httpReq, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), u.String(), body)
	if err != nil {
		return nil, err
	}
	httpReq.Header = header

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	decoded := decodeJSONBody(raw)
	return &arazzo.ExecutionResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header.Clone(),
		Body:       decoded,
		URL:        u.String(),
		Method:     httpReq.Method,
	}, nil
}

func isAsyncSource(req *arazzo.ExecutionRequest) bool {
	if req == nil {
		return false
	}
	if req.Source != nil && req.Source.Name == "asyncOrderApiDescription" {
		return true
	}
	return strings.Contains(req.OperationPath, "asyncOrderApiDescription")
}

func resolveHTTP(req *arazzo.ExecutionRequest) (method, path string, err error) {
	if p, m, ok := parseOperationPath(req.OperationPath); ok {
		return m, p, nil
	}
	opID := lastSegment(req.OperationID)
	if opID == "" {
		opID = lastSegment(req.OperationPath)
	}
	if req.Source != nil && req.Source.OpenAPIDocument != nil && opID != "" {
		if m, p, ok := findByOperationID(req.Source.OpenAPIDocument, opID); ok {
			return m, p, nil
		}
	}
	return "", "", fmt.Errorf("cannot resolve HTTP method/path for operationId %q operationPath %q", req.OperationID, req.OperationPath)
}

func applyOpenAPIParams(doc *v3.Document, method, path string, params map[string]any, u *url.URL, q url.Values, header http.Header, used map[string]bool) {
	if doc == nil || doc.Paths == nil || doc.Paths.PathItems == nil {
		return
	}
	item := doc.Paths.PathItems.GetOrZero(path)
	if item == nil {
		return
	}
	var op *v3.Operation
	if ops := item.GetOperations(); ops != nil {
		op = ops.GetOrZero(strings.ToLower(method))
	}
	var all []*v3.Parameter
	all = append(all, item.Parameters...)
	if op != nil {
		all = append(all, op.Parameters...)
	}
	for _, p := range all {
		if p == nil {
			continue
		}
		v, ok := params[p.Name]
		if !ok {
			continue
		}
		used[p.Name] = true
		s := stringify(v)
		switch p.In {
		case "path":
			u.Path = strings.ReplaceAll(u.Path, "{"+p.Name+"}", s)
		case "header":
			header.Set(p.Name, s)
		default:
			q.Set(p.Name, s)
		}
	}
}

func findByOperationID(doc *v3.Document, operationID string) (method, path string, ok bool) {
	if doc == nil || doc.Paths == nil || doc.Paths.PathItems == nil {
		return "", "", false
	}
	for p, item := range doc.Paths.PathItems.FromOldest() {
		if item == nil {
			continue
		}
		for m, op := range item.GetOperations().FromOldest() {
			if op != nil && op.OperationId == operationID {
				return m, p, true
			}
		}
	}
	return "", "", false
}

func parseOperationPath(operationPath string) (path, method string, ok bool) {
	const marker = "#/paths/"
	idx := strings.Index(operationPath, marker)
	if idx < 0 {
		return "", "", false
	}
	fragment := operationPath[idx:]
	parts := strings.Split(strings.TrimPrefix(fragment, "#/"), "/")
	if len(parts) < 3 || parts[0] != "paths" {
		return "", "", false
	}
	path = unescapePointer(parts[1])
	method = strings.ToLower(unescapePointer(parts[2]))
	return path, method, path != "" && method != ""
}

func unescapePointer(s string) string {
	s = strings.ReplaceAll(s, "~1", "/")
	return strings.ReplaceAll(s, "~0", "~")
}

func lastSegment(s string) string {
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}

func stringify(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case fmt.Stringer:
		return t.String()
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(b)
	}
}

func decodeJSONBody(raw []byte) any {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var decoded any
	if err := dec.Decode(&decoded); err != nil {
		return string(raw)
	}
	return canonicalJSON(decoded)
}

func canonicalJSON(v any) any {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		if f, err := t.Float64(); err == nil {
			return f
		}
		return string(t)
	case map[string]any:
		for k, val := range t {
			t[k] = canonicalJSON(val)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = canonicalJSON(val)
		}
		return t
	default:
		return v
	}
}
