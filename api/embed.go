// Package api holds the canonical HTTP contract assets of the repository.
package api

import (
	"bytes"
	_ "embed"
)

// openAPISpec is the compile-time embedded canonical OpenAPI contract. It is
// the single HTTP source of truth; runtime router enumeration is forbidden.
//
//go:embed openapi.yaml
var openAPISpec []byte

// OpenAPISpec returns a copy of the embedded canonical OpenAPI contract bytes.
// The package-level embedded slice is never exposed, so callers cannot mutate
// the canonical contract.
func OpenAPISpec() []byte {
	return bytes.Clone(openAPISpec)
}
