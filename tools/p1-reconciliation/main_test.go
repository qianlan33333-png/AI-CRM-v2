package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func frozenPaths() paths {
	return paths{"../../docs/evidence/p1/legacy-routes-6cb989c.json", "../../docs/api-mapping.jsonl", "../../docs/evidence/p1/route-triage.csv", "../../docs/evidence/p1/migration-lifecycle-index-6cb989c.json", "../../docs/migration-mapping.jsonl"}
}

func TestFrozenReconciliation(t *testing.T) {
	got, err := reconcile(frozenPaths())
	if err != nil {
		t.Fatal(err)
	}
	want := "p1-reconciliation: PASS (routes=781 s02=156 s03=184 s04=441 migrate_routes=501 deferred_post_launch_routes=268 not_migrated_routes=12 tables=316 fields=3313 pending_routes=0 pending_tables=0)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRejectsUnsafeMutations(t *testing.T) {
	for _, tc := range []struct {
		name, file string
		mutate     func(any)
	}{
		{"duplicate route", "routes", func(v any) { d := v.(*map[string]any); r := (*d)["routes"].([]any); r[1] = r[0] }},
		{"wrong partition", "api", func(v any) { r := v.(*[]map[string]any); (*r)[0]["partition"] = "S02" }},
		{"missing route status", "api", func(v any) { r := v.(*[]map[string]any); (*r)[0]["disposition"] = "" }},
		{"fake approved route", "api", func(v any) {
			r := v.(*[]map[string]any)
			approveRoute((*r)[0])
		}},
		{"approved route missing evidence", "api", func(v any) {
			r := v.(*[]map[string]any)
			for _, row := range *r {
				if row["mapping_id"] == "LEGACY-API-0012" {
					row["decision_evidence"] = []any{}
					return
				}
			}
		}},
		{"approved route reactivated", "api", func(v any) {
			r := v.(*[]map[string]any)
			for _, row := range *r {
				if row["mapping_id"] == "LEGACY-API-0012" {
					row["candidate_v2_operation_id"] = "reactivateLegacyRoute"
					return
				}
			}
		}},
		{"tier B mislabeled not migrated", "api", func(v any) {
			r := v.(*[]map[string]any)
			for _, row := range *r {
				if row["disposition"] == "DEFERRED_POST_LAUNCH" {
					approveRoute(row)
					return
				}
			}
		}},
		{"missing lifecycle table", "lifecycle", func(v any) {
			d := v.(*map[string]any)
			r := (*d)["tables"].([]any)
			(*d)["tables"] = r[1:]
			(*d)["table_count"] = float64(315)
		}},
		{"missing field reason", "migration", func(v any) {
			r := v.(*[]map[string]any)
			(*r)[0]["field_mappings"].([]any)[0].(map[string]any)["reason"] = ""
		}},
		{"identity to customers", "migration", func(v any) {
			r := v.(*[]map[string]any)
			for _, row := range *r {
				for _, item := range row["field_mappings"].([]any) {
					f := item.(map[string]any)
					if identity.MatchString(f["source"].(string)) {
						f["target"] = "planned:customers.name"
						f["reason"] = "Map " + row["legacy_table"].(string) + "." + f["source"].(string) + " to planned:customers.name."
						return
					}
				}
			}
		}},
		{"execution reactivation", "migration", func(v any) {
			r := v.(*[]map[string]any)
			for _, row := range *r {
				if row["legacy_table"] == "broadcast_jobs" {
					row["safety_rule"] = "resume pending sends"
					return
				}
			}
		}},
		{"fake signoff", "migration", func(v any) {
			r := v.(*[]map[string]any)
			(*r)[0]["decision"] = "MIGRATE"
			(*r)[0]["signoff"] = "APPROVED"
		}},
		{"absent source drop", "migration", func(v any) {
			r := v.(*[]map[string]any)
			for _, row := range *r {
				if row["source_presence"] == "ABSENT_AT_HEAD" {
					row["decision"] = "DROP"
					row["decision_evidence"] = []any{"G1-D02-2026-08-10", "approved_by=repository_owner", "approved_at=2026-08-10", "decision=DROP"}
					return
				}
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := mutatedFixture(t, tc.file, tc.mutate)
			if _, err := reconcile(p); err == nil {
				t.Fatal("mutation was accepted")
			}
		})
	}
}

func approveRoute(row map[string]any) {
	row["candidate_v2_operation_id"] = "NOT_APPLICABLE"
	row["candidate_v2_method"] = "NOT_APPLICABLE"
	row["candidate_v2_path"] = "NOT_APPLICABLE"
	row["disposition"] = "NOT_MIGRATED"
	row["disposition_reason"] = "G1-D01 approved tier C route as not migrated."
	row["signoff"] = "APPROVED"
	row["decision_evidence"] = []any{map[string]any{"decision_id": "G1-D01", "approved_by": "repository_owner", "approved_at": "2026-08-10", "decision": "NOT_MIGRATED"}}
}

func mutatedFixture(t *testing.T, kind string, mutate func(any)) paths {
	t.Helper()
	dir := t.TempDir()
	source := frozenPaths()
	result := paths{}
	files := map[string]string{"routes": source.routes, "api": source.api, "triage": source.triage, "lifecycle": source.lifecycle, "migration": source.migration}
	for name, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(dir, name+".json")
		if name == kind {
			if name == "api" || name == "migration" {
				rows := []map[string]any{}
				for _, line := range splitLines(data) {
					var row map[string]any
					if err := json.Unmarshal(line, &row); err != nil {
						t.Fatal(err)
					}
					rows = append(rows, row)
				}
				mutate(&rows)
				data = nil
				for _, row := range rows {
					encoded, _ := json.Marshal(row)
					data = append(data, encoded...)
					data = append(data, '\n')
				}
			} else {
				var doc map[string]any
				if err := json.Unmarshal(data, &doc); err != nil {
					t.Fatal(err)
				}
				mutate(&doc)
				data, _ = json.Marshal(doc)
			}
		}
		if err := os.WriteFile(out, data, 0600); err != nil {
			t.Fatal(err)
		}
		switch name {
		case "routes":
			result.routes = out
		case "api":
			result.api = out
		case "triage":
			result.triage = out
		case "lifecycle":
			result.lifecycle = out
		case "migration":
			result.migration = out
		}
	}
	return result
}

func splitLines(data []byte) [][]byte {
	result := [][]byte{}
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				result = append(result, data[start:i])
			}
			start = i + 1
		}
	}
	return result
}
