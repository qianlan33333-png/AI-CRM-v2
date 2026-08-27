package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCompareAndReconcileFixture(t *testing.T) {
	directory := t.TempDir()
	left := filepath.Join(directory, "left.json")
	right := filepath.Join(directory, "right.json")
	dispositions := filepath.Join(directory, "dispositions.json")
	comparison := filepath.Join(directory, "comparison.json")
	report := filepath.Join(directory, "report.json")
	writeFixture(t, left, `{"version":1,"schemas":["public"],"tables":[{"schema":"public","table":"contacts","columns":[{"name":"id","type":"bigint","nullable":false,"ordinal":1}],"primary_key":["id"],"foreign_keys":[],"row_count":3,"watermark":{}}],"digest":"ignored"}`)
	writeFixture(t, right, `{"version":1,"schemas":["public"],"tables":[{"schema":"public","table":"customers","columns":[{"name":"id","type":"bigint","nullable":false,"ordinal":1}],"primary_key":["id"],"foreign_keys":[],"row_count":3,"watermark":{}}],"digest":"ignored"}`)
	writeFixture(t, dispositions, `{"version":1,"tables":[{"source":{"schema":"public","table":"contacts"},"target":{"schema":"public","table":"customers"},"disposition":"imported"}]}`)
	var stdout, stderr bytes.Buffer
	if err := run([]string{"compare", "--left", left, "--right", right, "--output", comparison}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"reconcile", "--source", left, "--target", right, "--dispositions", dispositions, "--output", report}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bytes), `"source_count_equals_terminal_disposition": true`) || !strings.Contains(string(bytes), `"target_row_count": 3`) {
		t.Fatalf("unexpected reconciliation report: %s", bytes)
	}
}

func TestReadJSONRejectsMultipleValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multiple.json")
	writeFixture(t, path, `{} {}`)
	var value map[string]any
	if err := readJSON(path, &value); err == nil {
		t.Fatal("expected multiple value error")
	}
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
