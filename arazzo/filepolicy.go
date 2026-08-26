// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package arazzo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FilePolicyLoader loads OPA modules from
// {Dir}/{planId}/{version}/inbound.rego and outbound.rego.
// Optional data.json in that directory is merged with [FilePolicyLoader.Data]
// (struct fields override file keys) and returned as [PolicyBundle.Data].
type FilePolicyLoader struct {
	Dir  string
	Data map[string]any
}

// NewFilePolicyLoader returns a filesystem [PolicyLoader] rooted at dir.
func NewFilePolicyLoader(dir string) *FilePolicyLoader {
	return &FilePolicyLoader{Dir: dir}
}

// Load implements [PolicyLoader]. Missing directories or modules are not
// an error: a nil bundle means no policy. Invalid planId/version (path
// separators or "..") is an error.
func (l *FilePolicyLoader) Load(ctx context.Context, req PolicyRequest) (*PolicyBundle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := policyDir(l.Dir, req.PlanID, req.Version)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	inbound, err := readOptional(filepath.Join(dir, "inbound.rego"))
	if err != nil {
		return nil, err
	}
	outbound, err := readOptional(filepath.Join(dir, "outbound.rego"))
	if err != nil {
		return nil, err
	}
	if len(inbound) == 0 && len(outbound) == 0 {
		return nil, nil
	}

	data, err := mergePolicyData(filepath.Join(dir, "data.json"), l.Data)
	if err != nil {
		return nil, err
	}
	return &PolicyBundle{Inbound: inbound, Outbound: outbound, Data: data}, nil
}

func policyDir(root, planID, version string) (string, error) {
	if err := safePolicySegment(planID, "planId"); err != nil {
		return "", err
	}
	if err := safePolicySegment(version, "version"); err != nil {
		return "", err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(absRoot, planID, version)
	rel, err := filepath.Rel(absRoot, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("policy path escapes root")
	}
	return dir, nil
}

func safePolicySegment(s, name string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("policy %s is empty", name)
	}
	if strings.Contains(s, "..") || strings.ContainsAny(s, `/\`) {
		return fmt.Errorf("policy %s %q is not a safe path segment", name, s)
	}
	return nil
}

func readOptional(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return nil, nil
	}
	return b, nil
}

func mergePolicyData(path string, extra map[string]any) ([]byte, error) {
	out := map[string]any{}
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil && len(bytes.TrimSpace(b)) > 0 {
		if err := json.Unmarshal(b, &out); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if out == nil {
			out = map[string]any{}
		}
	}
	for k, v := range extra {
		out[k] = v
	}
	if len(out) == 0 {
		return nil, nil
	}
	return json.Marshal(out)
}
