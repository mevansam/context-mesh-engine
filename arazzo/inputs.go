// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package arazzo

import "strings"

// ReservedInputKey reports whether name is an engine-injected workflow input.
// Callers cannot set these keys; generated OpenAPI and MCP inputSchema omit them.
func ReservedInputKey(name string) bool {
	return reservedInputPrefix(name, PolicyHintsKey) || reservedInputPrefix(name, SecretsKey)
}

func reservedInputPrefix(name, key string) bool {
	return name == key || strings.HasPrefix(name, key+".")
}

// LeaksReservedInputs reports whether s names reserved engine inputs and
// should not be copied onto consumer-facing OpenAPI or MCP descriptions.
func LeaksReservedInputs(s string) bool {
	if s == "" {
		return false
	}
	if strings.Contains(s, PolicyHintsKey) {
		return true
	}
	return strings.Contains(s, "$inputs."+SecretsKey)
}
