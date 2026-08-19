package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func frozenPaths() paths {
	return paths{routes: "../../docs/evidence/p1/legacy-routes-6cb989c.json", api: "../../docs/api-mapping.jsonl", triage: "../../docs/evidence/p1/route-triage.csv", triageAuthority: "../../docs/evidence/p1/route-triage.csv", lifecycle: "../../docs/evidence/p1/migration-lifecycle-index-6cb989c.json", migration: "../../docs/migration-mapping.jsonl", openapi: "../../api/openapi.yaml", generated: "../../internal/api/candidate/generated/server.gen.go", generatorConfig: "../../api/oapi-codegen-p1-candidate.yaml"}
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
	frozen := frozenPaths()
	facts, err := loadIntegrationFacts(frozen.openapi, frozen.generated, frozen.generatorConfig)
	if err != nil {
		t.Fatal(err)
	}
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
		{"triage A B swap preserves counts", "triage", func(v any) {
			records := v.(*[][]string)
			columns := map[string]int{}
			for index, name := range (*records)[0] {
				columns[name] = index
			}
			first, second := -1, -1
			for index, record := range (*records)[1:] {
				switch record[columns["mapping_id"]] {
				case "LEGACY-API-0421":
					first = index + 1
				case "LEGACY-API-0053":
					second = index + 1
				}
			}
			if first < 0 || second < 0 {
				t.Fatal("frozen A/B triage rows are missing")
			}
			(*records)[first][columns["recommended_tier"]] = "B"
			(*records)[second][columns["recommended_tier"]] = "A"
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
			if _, err := reconcileWithIntegrationFacts(p, facts); err == nil {
				t.Fatal("mutation was accepted")
			}
		})
	}
}

func TestTierAuthorityAllowsDeclarationAppendWithoutCheckerChange(t *testing.T) {
	authority := map[string]string{"LEGACY-API-9001": "A"}
	declared := map[string]string{"LEGACY-API-9001": "A", "LEGACY-API-9002": "B"}
	if err := verifyTierAuthority(declared, authority); err != nil {
		t.Fatalf("append-only tier declaration was rejected: %v", err)
	}
}

func TestTierAuthorityRejectsEqualCountSwap(t *testing.T) {
	authority := map[string]string{"LEGACY-API-9001": "A", "LEGACY-API-9002": "B"}
	forged := map[string]string{"LEGACY-API-9001": "B", "LEGACY-API-9002": "A"}
	if err := verifyTierAuthority(forged, authority); err == nil {
		t.Fatal("equal-count A/B tier swap was accepted")
	}
}

func TestTierAuthorityUsesParentIndexedDeclaration(t *testing.T) {
	repository := t.TempDir()
	triage := filepath.Join(repository, "facts", "route-triage.csv")
	if err := os.MkdirAll(filepath.Dir(triage), 0700); err != nil {
		t.Fatal(err)
	}
	writeTriage := func(rows string) {
		t.Helper()
		if err := os.WriteFile(triage, []byte("mapping_id,recommended_tier,human_signoff\n"+rows), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git := func(arguments ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", repository}, arguments...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", arguments, err, output)
		}
	}
	writeTriage("LEGACY-API-9001,A,APPROVED\n")
	git("init", "-q", "-b", "main")
	git("config", "user.name", "p1-test")
	git("config", "user.email", "p1-test@example.invalid")
	git("add", ".")
	git("commit", "-q", "-m", "base authority")
	if err := os.WriteFile(filepath.Join(repository, "candidate"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "candidate")
	git("commit", "-q", "-m", "candidate parent")

	writeTriage("LEGACY-API-9001,A,APPROVED\nLEGACY-API-9002,B,APPROVED\n")
	git("add", "facts/route-triage.csv")
	current, err := stagedOrFile(triage)
	if err != nil {
		t.Fatal(err)
	}
	declared, err := extractTiers(current)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := parentTiers(triage)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyTierAuthority(declared, authority); err != nil {
		t.Fatalf("append-only staged declaration was rejected: %v", err)
	}

	writeTriage("LEGACY-API-9001,B,APPROVED\nLEGACY-API-9002,A,APPROVED\n")
	git("add", "facts/route-triage.csv")
	current, err = stagedOrFile(triage)
	if err != nil {
		t.Fatal(err)
	}
	declared, err = extractTiers(current)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyTierAuthority(declared, authority); err == nil {
		t.Fatal("candidate-controlled authority and equal-count tier swap self-certified")
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
			evidence := []apiDecisionEvidence{
				{DecisionID: "G1-D02", ApprovedBy: "repository_owner", ApprovedAt: "2026-08-10", Decision: "MIGRATE"},
				{DecisionID: "P5-DECLARATIVE", ApprovedBy: "repository_owner", ApprovedAt: "2026-08-17", Decision: "MIGRATE"},
			}
			if err := validateIntegratedRoute(tc.id, "A", authority, tc.operation, "GET", tc.path, "P5-DECLARATIVE-"+tc.id[len(tc.id)-4:], "MIGRATE", "APPROVED", "declarative canonical route", evidence, facts); err != nil {
				t.Fatalf("future declarative route was rejected: %v", err)
			}
		})
	}
}

func TestIntegratedRouteValidationFailsClosed(t *testing.T) {
	id := "LEGACY-API-9001"
	authority := routeFact{Path: "/api/admin/media-fixtures", Owner: "media", Methods: []string{"GET"}}
	evidence := []apiDecisionEvidence{{DecisionID: "G1-D02", ApprovedBy: "repository_owner", ApprovedAt: "2026-08-10", Decision: "MIGRATE"}}
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
			if err := validateIntegratedRoute(id, "A", tc.authority, tc.operation, "GET", authority.Path, "P5-DECLARATIVE-9001", "MIGRATE", "APPROVED", "declarative canonical route", evidence, tc.facts); err == nil {
				t.Fatal("unsafe integrated route was accepted")
			}
		})
	}
}

func TestIntegratedRouteDecisionEvidenceAllowsFrozenOwnerAppend(t *testing.T) {
	authority := routeFact{Path: "/api/admin/media-fixtures", Owner: "media", Methods: []string{"GET"}}
	facts := integrationFacts{
		openAPI:           map[string][]openAPIRoute{"LEGACY-API-9001": {{Operation: "listLegacyMediaFixtures", Method: "GET", Path: authority.Path}}},
		requiresGenerated: map[string]bool{},
		generated:         map[string]bool{},
	}
	evidence := []apiDecisionEvidence{
		{DecisionID: "P4-0301-OWNER-FREEZE-2026-08-19", ApprovedBy: "repository_owner", ApprovedAt: "2026-08-19", Decision: "MIGRATE"},
		{DecisionID: "G1-D02", ApprovedBy: "repository_owner", ApprovedAt: "2026-08-10", Decision: "MIGRATE"},
	}
	if err := validateIntegratedRoute("LEGACY-API-9001", "A", authority, "listLegacyMediaFixtures", "GET", authority.Path, "P4-DECLARATIVE-9001", "MIGRATE", "APPROVED", "frozen local route", evidence, facts); err != nil {
		t.Fatalf("frozen owner evidence was rejected: %v", err)
	}
}

func TestIntegratedRouteDecisionEvidenceFailsClosed(t *testing.T) {
	valid := []apiDecisionEvidence{
		{DecisionID: "G1-D02", ApprovedBy: "repository_owner", ApprovedAt: "2026-08-10", Decision: "MIGRATE"},
		{DecisionID: "P4-0301-OWNER-FREEZE-2026-08-19", ApprovedBy: "repository_owner", ApprovedAt: "2026-08-19", Decision: "MIGRATE"},
	}
	for _, tc := range []struct {
		name     string
		evidence []apiDecisionEvidence
	}{
		{"missing fixed G1", []apiDecisionEvidence{{DecisionID: "P4-0301-OWNER-FREEZE-2026-08-19", ApprovedBy: "repository_owner", ApprovedAt: "2026-08-19", Decision: "MIGRATE"}}},
		{"fixed G1 wrong owner", []apiDecisionEvidence{{DecisionID: "G1-D02", ApprovedBy: "not_owner", ApprovedAt: "2026-08-10", Decision: "MIGRATE"}}},
		{"fixed G1 empty date", []apiDecisionEvidence{{DecisionID: "G1-D02", ApprovedBy: "repository_owner", Decision: "MIGRATE"}}},
		{"fixed G1 wrong decision", []apiDecisionEvidence{{DecisionID: "G1-D02", ApprovedBy: "repository_owner", ApprovedAt: "2026-08-10", Decision: "DEFERRED_POST_LAUNCH"}}},
		{"duplicate decision", append(append([]apiDecisionEvidence(nil), valid...), valid[1])},
		{"empty decision ID", []apiDecisionEvidence{{DecisionID: "G1-D02", ApprovedBy: "repository_owner", ApprovedAt: "2026-08-10", Decision: "MIGRATE"}, {ApprovedBy: "repository_owner", ApprovedAt: "2026-08-19", Decision: "MIGRATE"}}},
		{"wrong owner", []apiDecisionEvidence{{DecisionID: "G1-D02", ApprovedBy: "repository_owner", ApprovedAt: "2026-08-10", Decision: "MIGRATE"}, {DecisionID: "P4-0301-OWNER-FREEZE-2026-08-19", ApprovedBy: "not_owner", ApprovedAt: "2026-08-19", Decision: "MIGRATE"}}},
		{"empty date", []apiDecisionEvidence{{DecisionID: "G1-D02", ApprovedBy: "repository_owner", ApprovedAt: "2026-08-10", Decision: "MIGRATE"}, {DecisionID: "P4-0301-OWNER-FREEZE-2026-08-19", ApprovedBy: "repository_owner", Decision: "MIGRATE"}}},
		{"wrong decision", []apiDecisionEvidence{{DecisionID: "G1-D02", ApprovedBy: "repository_owner", ApprovedAt: "2026-08-10", Decision: "MIGRATE"}, {DecisionID: "P4-0301-OWNER-FREEZE-2026-08-19", ApprovedBy: "repository_owner", ApprovedAt: "2026-08-19", Decision: "DEFERRED_POST_LAUNCH"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if validIntegratedDecisionEvidence("A", tc.evidence) {
				t.Fatal("invalid integrated-route evidence was accepted")
			}
		})
	}
}

func TestIntegratedRouteTierBAllowsExistingSupersessionEvidence(t *testing.T) {
	evidence := []apiDecisionEvidence{{DecisionID: "P4-AB-ALL-2026-08-16", ApprovedBy: "repository_owner", ApprovedAt: "2026-08-16", Decision: "MIGRATE"}}
	if !validIntegratedDecisionEvidence("B", evidence) {
		t.Fatal("existing tier B supersession evidence was rejected")
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
	result := source
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
			} else if name == "triage" {
				records, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
				if err != nil {
					t.Fatal(err)
				}
				mutate(&records)
				var rendered bytes.Buffer
				writer := csv.NewWriter(&rendered)
				writer.WriteAll(records)
				if err := writer.Error(); err != nil {
					t.Fatal(err)
				}
				data = rendered.Bytes()
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
