package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyMatrixMatchesFrozenLunaBoundary(t *testing.T) {
	if got, status := classifyMatrix(matrixRecord{Disposition: "DEPRECATED"}); got != "RETIREMENT_APPROVED" || status != "RETIRED" {
		t.Fatalf("deprecated = %s", got)
	}
	if got, status := classifyMatrix(matrixRecord{TriggeredAPI: "none (client-only)"}); got != "UI_ONLY" || status != "FRONTEND_INTEGRATION_DEFERRED" {
		t.Fatalf("client-only = %s", got)
	}
	if got, status := classifyMatrix(matrixRecord{}); got != "BACKEND_REQUIRED" || status != "UNMAPPED" {
		t.Fatalf("default = %s", got)
	}
}

func TestRouteClassificationUsesOnlyTheFrozenFourClasses(t *testing.T) {
	for _, classification := range []string{"BACKEND_REQUIRED", "EXTERNAL_PROTOCOL", "UI_ONLY", "RETIRED"} {
		if disposition, status := routeClassificationState(classification); disposition != classification || status == "" {
			t.Fatalf("classification %s = %s/%s", classification, disposition, status)
		}
	}
	if disposition, status := routeClassificationState("NEEDS_HUMAN_EVIDENCE"); disposition != "" || status != "" {
		t.Fatalf("unfrozen classification was accepted: %s/%s", disposition, status)
	}
}

func TestValidateBreakdownsAndReadinessUseAuthoritativeClassification(t *testing.T) {
	data, err := build(
		repoFile("docs/feature-matrix.csv"),
		repoFile("docs/api-mapping.jsonl"),
		repoFile("docs/migration-mapping.jsonl"),
		repoFile("docs/replacement/legacy-route-classification.csv"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := countAt(data.routes, 22, "BACKEND_REQUIRED"); got != 487 {
		t.Fatalf("backend route count = %d", got)
	}
	if err := validateBreakdowns(data); err != nil {
		t.Fatalf("current breakdown rejected: %v", err)
	}
	if rendered := renderReadiness(data); !strings.Contains(rendered, "487 BACKEND_REQUIRED, 177 EXTERNAL_PROTOCOL") || !strings.Contains(rendered, "0 unclassified") {
		t.Fatalf("authoritative route breakdown was not rendered: %s", rendered)
	}
	logoutRows := map[string]bool{"LEGACY-API-0760": false, "LEGACY-API-0761": false}
	for _, row := range data.routes {
		if (row[0] == "LEGACY-API-0760" || row[0] == "LEGACY-API-0761") && row[22] != "BACKEND_REQUIRED" {
			t.Fatalf("logout backend surface %s was excluded as %s", row[0], row[22])
		}
		if _, ok := logoutRows[row[0]]; ok {
			logoutRows[row[0]] = true
		}
	}
	for id, seen := range logoutRows {
		if !seen {
			t.Fatalf("logout backend surface %s is missing", id)
		}
	}
}

func TestLoadRouteClassificationsFailsClosedForInvalidRows(t *testing.T) {
	apis := []apiRecord{{MappingID: "LEGACY-API-0001"}, {MappingID: "LEGACY-API-0002"}}
	for _, tc := range []struct {
		name, csv string
		wantErr   bool
	}{
		{"missing", "mapping_id,ledger_line,classification,classification_reason,domain_owner_or_reassignment,candidate_v2_api_or_semantics,direct_v2_reference_count,feature_matrix_ids,source_evidence,evidence_refs,notes\nLEGACY-API-0001,2,BACKEND_REQUIRED,r,d,c,0,NONE,docs/api-mapping.jsonl:LEGACY-API-0001,e,n\n", true},
		{"invalid-classification", "mapping_id,ledger_line,classification,classification_reason,domain_owner_or_reassignment,candidate_v2_api_or_semantics,direct_v2_reference_count,feature_matrix_ids,source_evidence,evidence_refs,notes\nLEGACY-API-0001,2,NEEDS_HUMAN_EVIDENCE,r,d,c,0,NONE,docs/api-mapping.jsonl:LEGACY-API-0001,e,n\nLEGACY-API-0002,3,BACKEND_REQUIRED,r,d,c,0,NONE,docs/api-mapping.jsonl:LEGACY-API-0002,e,n\n", true},
		{"wrong-order", "mapping_id,ledger_line,classification,classification_reason,domain_owner_or_reassignment,candidate_v2_api_or_semantics,direct_v2_reference_count,feature_matrix_ids,source_evidence,evidence_refs,notes\nLEGACY-API-0002,2,BACKEND_REQUIRED,r,d,c,0,NONE,docs/api-mapping.jsonl:LEGACY-API-0002,e,n\nLEGACY-API-0001,3,BACKEND_REQUIRED,r,d,c,0,NONE,docs/api-mapping.jsonl:LEGACY-API-0001,e,n\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "route-classification.csv")
			if err := os.WriteFile(path, []byte(tc.csv), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := loadRouteClassifications(path, apis)
			if (err != nil) != tc.wantErr {
				t.Fatalf("loadTriage error = %v, wantErr %t", err, tc.wantErr)
			}
		})
	}
}

func TestFrozenAssetsAreMachineReadableAndOpenAPIPresent(t *testing.T) {
	assets, err := frozenAssets()
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 79 {
		t.Fatalf("assets = %d, want 79", len(assets))
	}
	packages := map[string]bool{}
	operations := map[string]bool{}
	for _, asset := range assets {
		packages[asset[0]] = true
		if operations[asset[2]] {
			t.Fatalf("duplicate operationId %s", asset[2])
		}
		operations[asset[2]] = true
	}
	if len(packages) != 12 {
		t.Fatalf("packages = %d, want 12", len(packages))
	}
	for _, asset := range assets {
		if asset[2] == "getServicePeriodMemberGridSchema" || asset[2] == "queryServicePeriodMemberGrid" {
			if asset[1] != "NONE_NEW;REUSES_00064_service_period_members" {
				t.Fatalf("%s migration ref = %s", asset[2], asset[1])
			}
		}
		if asset[0] == "00073" && asset[3] != "docs/evidence/p4/ee01-internal-event-safe-export-local-core.md" {
			t.Fatalf("EE01 source evidence = %s", asset[3])
		}
	}
}

func TestDM01OverlayIsClosedAndDoesNotClaimCutover(t *testing.T) {
	data, err := build(
		repoFile("docs/feature-matrix.csv"),
		repoFile("docs/api-mapping.jsonl"),
		repoFile("docs/migration-mapping.jsonl"),
		repoFile("docs/replacement/legacy-route-classification.csv"),
	)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"LEGACY-T14-006": true, "LEGACY-T14-149": true, "LEGACY-T14-152": true,
		"LEGACY-T14-153": true, "LEGACY-T14-154": true, "LEGACY-T14-155": true,
		"LEGACY-T14-176": true, "LEGACY-T14-230": true, "LEGACY-T14-231": true,
		"LEGACY-T14-313": true, "LEGACY-T14-314": true,
	}
	seen := map[string]bool{}
	for _, row := range data.migrations {
		if !want[row[0]] {
			continue
		}
		if seen[row[0]] {
			t.Fatalf("duplicate DM01 overlay row %s", row[0])
		}
		seen[row[0]] = true
		if row[18] != "NOT_EXECUTED" || row[33] != "LOCAL_VERIFIED" || row[34] != "NOT_EXECUTED" || row[35] != "NOT_EXECUTED" || row[36] != "LOCAL_VERIFIED" {
			t.Fatalf("%s overclaims DM01 state: %v", row[0], row[18:37])
		}
		if _, err := os.Stat(repoFile(row[32])); err != nil {
			t.Fatalf("%s evidence ref is not readable: %v", row[0], err)
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("DM01 overlay rows = %d, want %d", len(seen), len(want))
	}
}

func TestValidateReadinessRejectsAnyRendererDrift(t *testing.T) {
	data := output{}
	path := filepath.Join(t.TempDir(), "cutover-readiness.md")
	if err := os.WriteFile(path, []byte(renderReadiness(data)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateReadiness(path, data); err != nil {
		t.Fatalf("deterministic readiness rejected: %v", err)
	}
	if err := os.WriteFile(path, []byte("tampered\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateReadiness(path, data); err == nil {
		t.Fatal("tampered readiness was accepted")
	}
}
