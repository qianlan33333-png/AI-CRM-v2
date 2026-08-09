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

func TestTemporaryDatabaseURL(t *testing.T) {
	got, err := temporaryDatabaseURL(fixedDatabaseURL, "aicrm_query_plan_abcdef")
	if err != nil {
		t.Fatal(err)
	}
	want := "postgres://postgres:postgres@127.0.0.1:5432/aicrm_query_plan_abcdef?sslmode=disable"
	if got != want || got == fixedDatabaseURL {
		t.Fatalf("temporary URL = %q; want %q", got, want)
	}
	if _, err := temporaryDatabaseURL(fixedDatabaseURL, "unsafe-name"); err == nil {
		t.Fatal("unsafe database name was accepted")
	}
}
