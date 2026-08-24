// Copyright 2026 Fidelity Investments. All rights reserved.
// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package arazzo

import (
	"context"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// FileLoader loads Arazzo documents from a local directory (recursive).
type FileLoader struct {
	Dir string
}

// NewFileLoader returns a Loader that walks dir for .yaml, .yml, and .json files.
func NewFileLoader(dir string) *FileLoader {
	return &FileLoader{Dir: dir}
}

// Load implements [Loader].
func (l *FileLoader) Load(ctx context.Context) ([]Source, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := filepath.Abs(l.Dir)
	if err != nil {
		return nil, err
	}

	var sources []Source
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		dir := filepath.Dir(path)
		// Trailing slash so relative source URLs resolve against this
		// directory, not its parent (RFC 3986 last-segment rule).
		base := (&url.URL{Scheme: "file", Path: filepath.ToSlash(dir) + "/"}).String()
		sources = append(sources, Source{
			URI:     path,
			Data:    data,
			BaseURL: base,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return sources, nil
}
