package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeParams(t *testing.T) {
	got, count, err := normalizeParams("SELECT * FROM customers WHERE id = sqlc.arg(id) AND name = @name AND owner_id = sqlc.arg(id);")
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT * FROM customers WHERE id = $1 AND name = $2 AND owner_id = $1"
	if got != want || count != 2 {
		t.Fatalf("normalizeParams() = %q, %d; want %q, 2", got, count, want)
	}
}

func TestContainsTargetSeqScan(t *testing.T) {
	raw := []byte(`[{"Plan":{"Node Type":"Seq Scan","Relation Name":"customers"}}]`)
	got, err := containsTargetSeqScan(raw)
	if err != nil || got != "customers" {
		t.Fatalf("containsTargetSeqScan() = %q, %v", got, err)
	}
	partition := []byte(`[{"Plan":{"Node Type":"Seq Scan","Relation Name":"customer_events_202608"}}]`)
	if got, err := containsTargetSeqScan(partition); err != nil || got != "customer_events_202608" {
		t.Fatalf("partition scan = %q, %v", got, err)
	}
	if _, err := containsTargetSeqScan([]byte(`{}`)); err == nil {
		t.Fatal("malformed plan shape was accepted")
	}
}

func TestParseQueryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queries.sql")
	content := "-- name: ByID :one\nSELECT * FROM customers WHERE id = $1;\n\n-- name: Health :one\nSELECT 1;\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	queries, err := parseQueryFile("queries.sql", path)
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 1 || queries[0].Name != "ByID" {
		t.Fatalf("queries = %#v", queries)
	}
}

func TestPartitionSegmentQueries(t *testing.T) {
	regular, segment := partitionSegmentQueries([]query{
		{File: "internal/contact/store/queries/customers.sql", Name: "Contact"},
		{File: "internal/segment/store/queries/audience.sql", Name: "Segment"},
	})
	if len(regular) != 1 || regular[0].Name != "Contact" || len(segment) != 1 || segment[0].Name != "Segment" {
		t.Fatalf("partition = regular %#v, segment %#v", regular, segment)
	}
}

func TestAllQueryPathsIncludesUnchangedSQLcFiles(t *testing.T) {
	root := t.TempDir()
	for path, content := range map[string]string{
		"internal/contact/store/queries/current.sql":            "-- name: Current :one\nSELECT 1;\n",
		"internal/contact/store/queries/history_acceptance.sql": "-- name: Acceptance :one\nSELECT 1;\n",
		"internal/contact/store/generated/ignored.sql":          "SELECT 1;\n",
	} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := allQueryPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"internal/contact/store/queries/current.sql",
		"internal/contact/store/queries/history_acceptance.sql",
	}
	if len(paths) != len(want) {
		t.Fatalf("all query paths = %#v", paths)
	}
	for index := range want {
		if paths[index] != want[index] {
			t.Fatalf("all query paths = %#v; want %#v", paths, want)
		}
	}
}

func TestTemporaryDatabaseURL(t *testing.T) {
	const databaseURL = "postgres://postgres:postgres@127.0.0.1:55432/aicrm_test?sslmode=disable"
	got, err := temporaryDatabaseURL(databaseURL, "aicrm_query_plan_abcdef")
	if err != nil {
		t.Fatal(err)
	}
	want := "postgres://postgres:postgres@127.0.0.1:55432/aicrm_query_plan_abcdef?sslmode=disable"
	if got != want || got == databaseURL {
		t.Fatalf("temporary URL = %q; want %q", got, want)
	}
	if _, err := temporaryDatabaseURL(databaseURL, "unsafe-name"); err == nil {
		t.Fatal("unsafe database name was accepted")
	}
}

func TestValidateAcceptanceDatabaseURL(t *testing.T) {
	for _, test := range []struct {
		name        string
		databaseURL string
		wantError   bool
	}{
		{name: "dynamic IPv4 loopback", databaseURL: "postgres://postgres:postgres@127.0.0.1:55432/aicrm_test?sslmode=disable"},
		{name: "dynamic IPv6 loopback", databaseURL: "postgres://postgres:postgres@[::1]:55432/aicrm_test?sslmode=disable"},
		{name: "hostname rejected", databaseURL: "postgres://postgres:postgres@localhost:55432/aicrm_test?sslmode=disable", wantError: true},
		{name: "credential rejected", databaseURL: "postgres://postgres:secret@127.0.0.1:55432/aicrm_test?sslmode=disable", wantError: true},
		{name: "extra option rejected", databaseURL: "postgres://postgres:postgres@127.0.0.1:55432/aicrm_test?sslmode=disable&application_name=test", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateAcceptanceDatabaseURL(test.databaseURL)
			if test.wantError && err == nil {
				t.Fatal("validateAcceptanceDatabaseURL() error = nil, want rejection")
			}
			if !test.wantError && err != nil {
				t.Fatalf("validateAcceptanceDatabaseURL() error = %v, want nil", err)
			}
		})
	}
}
