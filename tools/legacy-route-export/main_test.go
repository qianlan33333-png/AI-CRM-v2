package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sourceSHA = "6cb989c071255437d75953dabb943318a74eb8f4"

func manifest(path, name string) string {
	return "routes:\n- path: " + path + "\n  methods:\n  - GET\n  route_name: " + name + "\n  capability_owner: contact\n  runtime_owner: ai_crm_next\n  layer: api\n  external_effects: none\n  data_source: read_model\n  requires_auth: true\n  rollback: previous_release\n  audience: admin\n  auth_scheme: human_session\n  capability: admin_read\n  access_scope: global\n  pii_level: customer\n  csrf: false\n  rate_limit: authenticated\n  principal_types:\n  - human\n"
}

func writeManifest(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "routes.yml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunIsDeterministicAndSorted(t *testing.T) {
	path := writeManifest(t, manifest("/z", "z")+strings.TrimPrefix(manifest("/a", "a"), "routes:\n"))
	var first, second bytes.Buffer
	if err := run([]string{path, sourceSHA}, &first); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{path, sourceSHA}, &second); err != nil {
		t.Fatal(err)
	}
	if first.String() != second.String() {
		t.Fatal("same input produced different bytes")
	}
	output := first.String()
	if strings.Index(output, `"path": "/a"`) > strings.Index(output, `"path": "/z"`) || !strings.Contains(output, `"route_count": 2`) || !strings.Contains(output, sourceSHA) {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestMalformedInputsFailClosed(t *testing.T) {
	valid := manifest("/health", "health")
	cases := map[string]string{
		"no-final-newline": strings.TrimSuffix(valid, "\n"),
		"unknown-field":    strings.Replace(valid, "  layer: api", "  surprise: api", 1),
		"duplicate-field":  strings.Replace(valid, "  layer: api", "  layer: api\n  layer: api", 1),
		"scalar-methods":   strings.Replace(valid, "  methods:\n  - GET", "  methods: GET", 1),
		"duplicate-item":   strings.Replace(valid, "  - GET", "  - GET\n  - GET", 1),
		"bad-boolean":      strings.Replace(valid, "  csrf: false", "  csrf: no", 1),
		"missing-field":    strings.Replace(valid, "  route_name: health\n", "", 1),
		"duplicate-route":  valid + strings.TrimPrefix(valid, "routes:\n"),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if err := run([]string{writeManifest(t, body), sourceSHA}, &bytes.Buffer{}); err == nil {
				t.Fatal("malformed input was accepted")
			}
		})
	}
}

func TestRejectsBadSHAAndSymlink(t *testing.T) {
	target := writeManifest(t, manifest("/health", "health"))
	link := filepath.Join(t.TempDir(), "routes.yml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{target, "BAD"}, {link, sourceSHA}} {
		if err := run(args, &bytes.Buffer{}); err == nil {
			t.Fatal("unsafe argument was accepted")
		}
	}
}
