package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryAcceptsIdentityOwnedValueBoundary(t *testing.T) {
	if err := checkRepository(newFixture(t)); err != nil {
		t.Fatalf("valid fixture rejected: %v", err)
	}
}

func TestRepositoryRejectsIdentityValueBoundaryRegressions(t *testing.T) {
	tests := []struct {
		name string
		want string
		edit func(*testing.T, string)
	}{
		{"raw identity production marker", "raw identity marker forbidden", write("internal/identity/app/leak.go", "package app\nconst leaked = \"raw_identity\"\n")},
		{"raw identity SQL marker", "raw identity marker forbidden", write("internal/identity/store/queries/leak.sql", "INSERT INTO identities (raw_identity) VALUES ('x');\n")},
		{"raw identity OpenAPI marker", "raw identity marker forbidden", write("api/openapi.yaml", resolveSchema("raw_identity"))},
		{"normalized OpenAPI marker", "normalized_value is storage-only", write("api/openapi.yaml", resolveSchema("normalized_value"))},
		{"normalized outside identity store", "normalized_value allowed only", write("internal/contact/store/queries/leak.sql", "INSERT INTO pending_events (normalized_value) VALUES ('x');\n")},
		{"non exact normalized lookup", "normalized_value allowed only", write("internal/identity/store/queries/leak.sql", "SELECT normalized_value FROM identities WHERE normalized_value = $1;\n")},
		{"fingerprint Resolve port key", "fingerprint forbidden in Resolve contract", write("internal/identity/port/port.go", "package port\ntype Service interface { Resolve(fingerprint string) error }\n")},
		{"fingerprint Resolve OpenAPI key", "raw identity or fingerprint forbidden", write("api/openapi.yaml", resolveSchema("fingerprint"))},
		{"fingerprint lookup surrogate", "fingerprint cannot be used", write("internal/identity/store/queries/leak.sql", "SELECT id FROM identities WHERE review_fingerprint = $1;\n")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := newFixture(t)
			test.edit(t, root)
			err := checkRepository(root)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v, want %q", err, test.want)
			}
		})
	}
}

func newFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, directory := range []string{"api", "cmd", "internal/identity/port", "internal/identity/store/queries", "migrations"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(directory)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, root, "internal/identity/port/port.go", "package port\ntype Service interface { Resolve(value string) error }\n")
	writeFile(t, root, "api/openapi.yaml", resolveSchema("value"))
	writeFile(t, root, "migrations/00010_identity_storage.sql", "CREATE TABLE identities (normalized_value TEXT NOT NULL);\n")
	writeFile(t, root, "internal/identity/store/queries/identities.sql", "SELECT normalized_value FROM identities WHERE kind = $1 AND scope = $2 AND normalized_value = $3;\n")
	return root
}

func resolveSchema(property string) string {
	return "components:\n  schemas:\n    ResolveIdentityRequest:\n      properties:\n        " + property + ":\n          type: string\n    NextSchema:\n      type: object\n"
}

func write(rel, content string) func(*testing.T, string) {
	return func(t *testing.T, root string) { writeFile(t, root, rel, content) }
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
