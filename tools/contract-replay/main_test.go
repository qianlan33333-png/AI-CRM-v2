package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateManifest(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "empty", input: `{"version":1,"cases":[]}`},
		{name: "malformed", input: `{`, wantErr: "decode manifest"},
		{name: "trailing", input: `{"version":1,"cases":[]} {}`, wantErr: "exactly one JSON value"},
		{name: "unknown", input: `{"version":1,"cases":[],"extra":true}`, wantErr: "unknown field"},
		{name: "duplicate field", input: `{"version":1,"version":1,"cases":[]}`, wantErr: "duplicate field"},
		{name: "version", input: `{"version":2,"cases":[]}`, wantErr: "version must be 1"},
		{name: "missing cases", input: `{"version":1}`, wantErr: "cases must be an array"},
		{name: "null cases", input: `{"version":1,"cases":null}`, wantErr: "cases must be an array"},
		{name: "nonempty", input: `{"version":1,"cases":[{"id":"one"}]}`, wantErr: "execution adapter is not implemented"},
		{name: "oversized", input: strings.Repeat(" ", maxManifestBytes+1), wantErr: "manifest exceeds"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateManifest(strings.NewReader(test.input))
			if test.wantErr == "" && err != nil {
				t.Fatalf("validateManifest() error = %v", err)
			}
			if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("validateManifest() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"cases":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{path}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, stderr = %q", code, stderr.String())
	}
	if got := stdout.String(); got != "contract-replay: PASS (0 cases; validation only)\n" {
		t.Fatalf("stdout = %q", got)
	}
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("run(nil) = %d, want 2", code)
	}
}
