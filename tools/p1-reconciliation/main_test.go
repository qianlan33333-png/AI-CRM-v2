package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func frozenPaths() paths {
	return paths{"../../docs/evidence/p1/legacy-routes-6cb989c.json", "../../docs/api-mapping.jsonl", "../../docs/evidence/p1/route-triage.csv", "../../docs/evidence/p1/migration-lifecycle-index-6cb989c.json", "../../docs/migration-mapping.jsonl", "../../api/openapi.yaml", "../../internal/api/candidate/generated/server.gen.go", "../../api/oapi-codegen-p1-candidate.yaml"}
}

func TestFrozenReconciliation(t *testing.T) {
	got, err := reconcile(frozenPaths())
	if err != nil {
		t.Fatal(err)
	}
	want := "p1-reconciliation: PASS (routes=781 s02=156 s03=184 s04=441 migrate_routes=502 deferred_post_launch_routes=267 not_migrated_routes=12 tables=316 fields=3313 pending_routes=0 pending_tables=0)"
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
		{"integrated route operation forged", "api", func(v any) {
			r := v.(*[]map[string]any)
			for _, row := range *r {
				if row["mapping_id"] == "LEGACY-API-0421" {
					row["candidate_v2_operation_id"] = "getLegacyPushCenterForged"
					return
				}
			}
		}},
		{"integrated route missing target mapping", "api", func(v any) {
			r := v.(*[]map[string]any)
			for _, row := range *r {
				if row["mapping_id"] == "LEGACY-API-0422" {
					row["target_mapping_id"] = ""
					return
				}
			}
		}},
		{"integrated tier B decision forged", "api", func(v any) {
			r := v.(*[]map[string]any)
			for _, row := range *r {
				if row["mapping_id"] == "LEGACY-API-0053" {
					row["decision_evidence"] = []any{map[string]any{"decision_id": "P4-AB-ALL-2026-08-16", "approved_by": "repository_owner", "approved_at": "2026-08-16", "decision": "DEFERRED_POST_LAUNCH"}}
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

func TestIntegratedRoutesAreDerivedFromCanonicalFacts(t *testing.T) {
	dir := t.TempDir()
	openapi := `openapi: 3.0.3
info:
  title: future package fixture
  version: 1.0.0
paths:
  /api/admin/media-fixtures:
    get:
      operationId: listLegacyMediaFixtures
      tags: [p1-core-candidate]
      x-legacy-mapping-ids: [LEGACY-API-9001]
      responses:
        "200": {description: ok}
  /api/admin/order-fixtures:
    get:
      operationId: listLegacyOrderFixtures
      tags: [p1-core-candidate]
      x-legacy-mapping-ids: [LEGACY-API-9002]
      responses:
        "200": {description: ok}
`
	generated := `package generated
func (siw *ServerInterfaceWrapper) ListLegacyMediaFixtures() {}
func (siw *ServerInterfaceWrapper) ListLegacyOrderFixtures() {}
func register(r router, options options, wrapper *ServerInterfaceWrapper) {
  r.Get(options.BaseURL+"/api/admin/media-fixtures", wrapper.ListLegacyMediaFixtures)
  r.Get(options.BaseURL+"/api/admin/order-fixtures", wrapper.ListLegacyOrderFixtures)
}
`
	config := `output-options:
  include-tags:
    - p1-core-candidate
`
	write := func(name, data string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	facts, err := loadIntegrationFacts(write("openapi.yaml", openapi), write("server.gen.go", generated), write("oapi.yaml", config))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ id, operation, path, owner string }{
		{"LEGACY-API-9001", "listLegacyMediaFixtures", "/api/admin/media-fixtures", "media"},
		{"LEGACY-API-9002", "listLegacyOrderFixtures", "/api/admin/order-fixtures", "order"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			authority := routeFact{Path: tc.path, Owner: tc.owner, Methods: []string{"GET"}}
			evidence := []apiDecisionEvidence{{DecisionID: "P5-DECLARATIVE", ApprovedBy: "repository_owner", ApprovedAt: "2026-08-17", Decision: "MIGRATE"}}
			if err := validateIntegratedRoute(tc.id, authority, tc.operation, "GET", tc.path, "P5-DECLARATIVE-"+tc.id[len(tc.id)-4:], "MIGRATE", "APPROVED", "declarative canonical route", evidence, facts); err != nil {
				t.Fatalf("future declarative route was rejected: %v", err)
			}
		})
	}
}

func TestIntegratedRouteValidationFailsClosed(t *testing.T) {
	id := "LEGACY-API-9001"
	authority := routeFact{Path: "/api/admin/media-fixtures", Owner: "media", Methods: []string{"GET"}}
	evidence := []apiDecisionEvidence{{DecisionID: "P5-DECLARATIVE", ApprovedBy: "repository_owner", ApprovedAt: "2026-08-17", Decision: "MIGRATE"}}
	valid := integrationFacts{
		openAPI:           map[string][]openAPIRoute{id: {{Operation: "listLegacyMediaFixtures", Method: "GET", Path: authority.Path}}},
		requiresGenerated: map[string]bool{integrationKey("listLegacyMediaFixtures", "GET", authority.Path): true},
		generated:         map[string]bool{integrationKey("listLegacyMediaFixtures", "GET", authority.Path): true},
	}
	for _, tc := range []struct {
		name      string
		authority routeFact
		operation string
		facts     integrationFacts
	}{
		{"missing openapi", authority, "listLegacyMediaFixtures", integrationFacts{openAPI: map[string][]openAPIRoute{}, requiresGenerated: valid.requiresGenerated, generated: valid.generated}},
		{"duplicate openapi", authority, "listLegacyMediaFixtures", integrationFacts{openAPI: map[string][]openAPIRoute{id: {valid.openAPI[id][0], valid.openAPI[id][0]}}, requiresGenerated: valid.requiresGenerated, generated: valid.generated}},
		{"missing generated", authority, "listLegacyMediaFixtures", integrationFacts{openAPI: valid.openAPI, requiresGenerated: valid.requiresGenerated, generated: map[string]bool{}}},
		{"missing owner", routeFact{Path: authority.Path, Methods: authority.Methods}, "listLegacyMediaFixtures", valid},
		{"forged operation", authority, "listLegacyMediaForged", valid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateIntegratedRoute(id, tc.authority, tc.operation, "GET", authority.Path, "P5-DECLARATIVE-9001", "MIGRATE", "APPROVED", "declarative canonical route", evidence, tc.facts); err == nil {
				t.Fatal("unsafe integrated route was accepted")
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
	files := map[string]string{"routes": source.routes, "api": source.api, "triage": source.triage, "lifecycle": source.lifecycle, "migration": source.migration, "openapi": source.openapi, "generated": source.generated, "generatorConfig": source.generatorConfig}
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
		case "openapi":
			result.openapi = out
		case "generated":
			result.generated = out
		case "generatorConfig":
			result.generatorConfig = out
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
