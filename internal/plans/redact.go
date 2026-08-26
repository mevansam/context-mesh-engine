// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package plans

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func cloneJSONMap(in map[string]any) (map[string]any, error) {
	if in == nil {
		return map[string]any{}, nil
	}
	b, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

// applyRedact sets each RFC 6901 pointer in doc to mask. Missing pointers
// are skipped. Malformed pointers or a pointer to the document root fail.
func applyRedact(doc map[string]any, pointers []string, mask any) (map[string]any, error) {
	out, err := cloneJSONMap(doc)
	if err != nil {
		return nil, err
	}
	var root any = out
	for _, p := range pointers {
		next, err := redactAt(root, p, mask)
		if err != nil {
			return nil, err
		}
		root = next
	}
	m, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("redact: result is not an object")
	}
	return m, nil
}

func redactAt(doc any, pointer string, mask any) (any, error) {
	tokens, err := parseJSONPointer(pointer)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, fmt.Errorf("redact: pointer %q must not target the document root", pointer)
	}
	if err := setPointer(doc, tokens, mask); err != nil {
		if isMissing(err) {
			return doc, nil
		}
		return nil, err
	}
	return doc, nil
}

type missingError struct{}

func (missingError) Error() string { return "missing" }

func isMissing(err error) bool {
	_, ok := err.(missingError)
	return ok
}

func parseJSONPointer(p string) ([]string, error) {
	if p == "" {
		return nil, nil
	}
	if !strings.HasPrefix(p, "/") {
		return nil, fmt.Errorf("redact: malformed pointer %q", p)
	}
	parts := strings.Split(p[1:], "/")
	out := make([]string, len(parts))
	for i, part := range parts {
		unesc, err := unescapePointerToken(part)
		if err != nil {
			return nil, fmt.Errorf("redact: malformed pointer %q", p)
		}
		out[i] = unesc
	}
	return out, nil
}

func unescapePointerToken(s string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '~' {
			b.WriteByte(s[i])
			continue
		}
		if i+1 >= len(s) || (s[i+1] != '0' && s[i+1] != '1') {
			return "", fmt.Errorf("bad escape")
		}
		if s[i+1] == '0' {
			b.WriteByte('~')
		} else {
			b.WriteByte('/')
		}
		i++
	}
	return b.String(), nil
}

func setPointer(cur any, tokens []string, mask any) error {
	if len(tokens) == 0 {
		return fmt.Errorf("redact: empty pointer remainder")
	}
	key := tokens[0]
	last := len(tokens) == 1
	switch node := cur.(type) {
	case map[string]any:
		if last {
			if _, ok := node[key]; !ok {
				return missingError{}
			}
			node[key] = mask
			return nil
		}
		child, ok := node[key]
		if !ok {
			return missingError{}
		}
		return setPointer(child, tokens[1:], mask)
	case []any:
		idx, err := parseArrayIndex(key, len(node))
		if err != nil {
			if isMissing(err) {
				return err
			}
			return fmt.Errorf("redact: malformed array index %q", key)
		}
		if last {
			node[idx] = mask
			return nil
		}
		return setPointer(node[idx], tokens[1:], mask)
	default:
		return missingError{}
	}
}

func parseArrayIndex(s string, n int) (int, error) {
	if s == "" || (len(s) > 1 && s[0] == '0') || strings.HasPrefix(s, "-") {
		return 0, fmt.Errorf("bad index")
	}
	idx, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("bad index")
	}
	if idx < 0 || idx >= n {
		return 0, missingError{}
	}
	return idx, nil
}
