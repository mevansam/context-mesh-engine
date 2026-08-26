// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

// Package arazzo is the public plugin surface for loading Arazzo specs,
// executing workflow steps, matching queries to plans, documenting
// generated MCP tools, and loading optional OPA policy bundles.
package arazzo

import (
	"context"

	"github.com/pb33f/libopenapi/arazzo"
)

// Executor is the pluggable transport for backend API calls made by
// the libopenapi Arazzo engine. SDK implementors supply this.
type Executor = arazzo.Executor

// ExecutionRequest is the backend call the [Executor] must perform.
type ExecutionRequest = arazzo.ExecutionRequest

// ExecutionResponse is the backend result returned by an [Executor].
type ExecutionResponse = arazzo.ExecutionResponse

// Source is one Arazzo document produced by a [Loader].
type Source struct {
	// URI is a locator for error messages (usually a filesystem path).
	URI string
	// Data is the raw Arazzo document bytes.
	Data []byte
	// BaseURL is used as ResolveSources BaseURL (typically the file's
	// directory as a file:// URL with a trailing slash).
	BaseURL string
}

// Loader is a pluggable Arazzo spec source.
type Loader interface {
	Load(ctx context.Context) ([]Source, error)
}
