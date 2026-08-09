package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validCatalog = `{"version":1,"ignore_paths":[],"cases":[{"operation_id":"contacts.list","case_id":"happy","request":{"method":"GET","path":"/api/customers","body":null},"expected_response":{"status":200,"body":{"count":1,"name":"Ada"}}}]}`
const validActual = `{"version":1,"cases":[{"operation_id":"contacts.list","case_id":"happy","request":{"method":"GET","path":"/api/customers","body":null},"actual_response":{"status":200,"body":{"count":1,"name":"Ada"}}}]}`

func TestValidateCatalog(t *testing.T) {
	tests := []struct{ name, input, want string }{
		{"empty", `{"version":1,"ignore_paths":[],"cases":[]}`, ""},
		{"valid", validCatalog, ""},
		{"malformed", `{`, "decode catalog"},
		{"trailing", `{"version":1,"ignore_paths":[],"cases":[]} {}`, "exactly one"},
		{"unknown", `{"version":1,"ignore_paths":[],"cases":[],"extra":true}`, "unknown field"},
		{"duplicate nested", strings.Replace(validCatalog, `"count":1`, `"count":1,"count":2`, 1), "duplicate field"},
		{"null ignores", `{"version":1,"ignore_paths":null,"cases":[]}`, "ignore_paths must be an array"},
		{"null cases", `{"version":1,"ignore_paths":[],"cases":null}`, "cases must be an array"},
		{"duplicate case", strings.Replace(validCatalog, `]}`, `,`+strings.TrimSuffix(strings.TrimPrefix(validCatalog, `{"version":1,"ignore_paths":[],"cases":[`), `]}`)+`]}`, 1), "duplicate case"},
		{"wildcard ignore", strings.Replace(validCatalog, `"ignore_paths":[]`, `"ignore_paths":["/cases/contacts.list/happy/response/body/*"]`, 1), "invalid exact ignore path"},
		{"missing ignore", strings.Replace(validCatalog, `"ignore_paths":[]`, `"ignore_paths":["/cases/contacts.list/happy/response/body/missing"]`, 1), "existing scalar leaf"},
		{"container ignore", strings.Replace(validCatalog, `"ignore_paths":[]`, `"ignore_paths":["/cases/contacts.list/happy/response/body"]`, 1), "invalid exact ignore path"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := validateCatalog([]byte(test.input))
			if test.want == "" && err != nil {
				t.Fatalf("validateCatalog() error = %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("error = %v; want %q", err, test.want)
			}
		})
	}
}

func TestCompareCatalog(t *testing.T) {
	catalogValue, bodies, err := validateCatalog([]byte(validCatalog))
	if err != nil {
		t.Fatal(err)
	}
	if err := compareCatalog(catalogValue, bodies, []byte(validActual)); err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(validActual, `"name":"Ada"`, `"name":"Grace"`, 1)
	if err := compareCatalog(catalogValue, bodies, []byte(changed)); err == nil || !strings.Contains(err.Error(), "/cases/contacts.list/happy/response/body/name") {
		t.Fatalf("changed response error = %v", err)
	}
	unknown := strings.Replace(validActual, `"actual_response"`, `"extra":true,"actual_response"`, 1)
	if err := compareCatalog(catalogValue, bodies, []byte(unknown)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown error = %v", err)
	}
}

func TestExactScalarIgnore(t *testing.T) {
	input := strings.Replace(validCatalog, `"ignore_paths":[]`, `"ignore_paths":["/cases/contacts.list/happy/response/body/name"]`, 1)
	value, bodies, err := validateCatalog([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(validActual, `"name":"Ada"`, `"name":"Grace"`, 1)
	if err := compareCatalog(value, bodies, []byte(changed)); err != nil {
		t.Fatal(err)
	}
	missing := strings.Replace(validActual, `,"name":"Ada"`, ``, 1)
	if err := compareCatalog(value, bodies, []byte(missing)); err == nil || !strings.Contains(err.Error(), "did not match actual response") {
		t.Fatalf("missing ignored field error = %v", err)
	}
}

func TestRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"ignore_paths":[],"cases":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"validate", path}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d; stderr = %q", code, stderr.String())
	}
	if stdout.String() != "snapshot-gate: PASS (0 cases; validation only)\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"compare", path}, strings.NewReader(`{"version":1,"cases":[]}`), &stdout, &stderr); code != 0 {
		t.Fatalf("compare = %d; stderr = %q", code, stderr.String())
	}
	if code := run(nil, strings.NewReader(""), &stdout, &stderr); code != 2 {
		t.Fatalf("run(nil) = %d", code)
	}
}
