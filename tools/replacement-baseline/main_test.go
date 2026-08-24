package main

import (
	"os"
	"path/filepath"
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

func TestClassifyRouteProtectsPublicProtocolsAndDrift(t *testing.T) {
	for _, tc := range []struct{ id, audience, effect, disposition, tier, want string }{
		{"LEGACY-API-0053", "admin", "none", "MIGRATE", "B", "UNCLASSIFIED_SOURCE_DRIFT"},
		{"LEGACY-API-0001", "public_h5", "none", "MIGRATE", "A", "EXTERNAL_PROTOCOL"},
		{"LEGACY-API-0002", "callback", "none", "MIGRATE", "A", "EXTERNAL_PROTOCOL"},
		{"LEGACY-API-0003", "external_integration", "none", "MIGRATE", "A", "EXTERNAL_PROTOCOL"},
		{"LEGACY-API-0753", "admin", "staging_disabled", "MIGRATE", "A", "EXTERNAL_PROTOCOL"},
		{"LEGACY-API-0758", "admin", "none", "MIGRATE", "A", "UNCLASSIFIED"},
		{"LEGACY-API-0004", "admin", "staging_disabled", "MIGRATE", "A", "UNCLASSIFIED"},
	} {
		record := apiRecord{MappingID: tc.id}
		record.Manifest.Audience = tc.audience
		record.Manifest.ExternalEffects = tc.effect
		record.Disposition = tc.disposition
		if tc.id == "LEGACY-API-0753" {
			record.Manifest.AccessScope = "public"
			record.Manifest.AuthScheme = "provider_oauth_state"
			record.Manifest.CapabilityOwner = "auth_wecom"
		}
		got, status := classifyRoute(record, triageRecord{RecommendedTier: tc.tier})
		if got != tc.want {
			t.Fatalf("%s: got %s want %s", tc.id, got, tc.want)
		}
		if tc.want == "EXTERNAL_PROTOCOL" && status != "INVENTORIED" {
			t.Fatalf("%s: protocol status = %s", tc.id, status)
		}
	}
}

func TestTriageDispositionMapping(t *testing.T) {
	for _, tc := range []struct {
		tier, disposition string
		want              bool
	}{
		{"A", "MIGRATE", true}, {"B", "DEFERRED_POST_LAUNCH", true}, {"C", "NOT_MIGRATED", true}, {"B", "MIGRATE", false},
	} {
		if got := triageMatchesDisposition(tc.tier, tc.disposition); got != tc.want {
			t.Fatalf("%s/%s = %t, want %t", tc.tier, tc.disposition, got, tc.want)
		}
	}
}

func TestRouteDriftClearsWhenTriageMatches(t *testing.T) {
	record := apiRecord{MappingID: "LEGACY-API-0053", Disposition: "MIGRATE"}
	got, status := classifyRoute(record, triageRecord{MappingID: record.MappingID, RecommendedTier: "A"})
	if got != "UNCLASSIFIED" || status != "UNCLASSIFIED" {
		t.Fatalf("matched route remained drift: %s/%s", got, status)
	}
}

func TestLoadTriageFailsClosedForMissingOrDuplicateIDs(t *testing.T) {
	apis := []apiRecord{{MappingID: "LEGACY-API-0001"}, {MappingID: "LEGACY-API-0002"}}
	for _, tc := range []struct {
		name, csv string
		wantErr   bool
	}{
		{"valid", "mapping_id,recommended_tier\nLEGACY-API-0001,A\nLEGACY-API-0002,B\n", false},
		{"missing", "mapping_id,recommended_tier\nLEGACY-API-0001,A\n", true},
		{"duplicate", "mapping_id,recommended_tier\nLEGACY-API-0001,A\nLEGACY-API-0001,A\n", true},
		{"stale", "mapping_id,recommended_tier\nLEGACY-API-0001,A\nLEGACY-API-9999,B\n", true},
		{"invalid-tier", "mapping_id,recommended_tier\nLEGACY-API-0001,D\nLEGACY-API-0002,B\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "route-triage.csv")
			if err := os.WriteFile(path, []byte(tc.csv), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := loadTriage(path, apis)
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
	if len(assets) != 76 {
		t.Fatalf("assets = %d, want 76", len(assets))
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
	if len(packages) != 11 {
		t.Fatalf("packages = %d, want 11", len(packages))
	}
	for _, asset := range assets {
		if asset[2] == "getServicePeriodMemberGridSchema" || asset[2] == "queryServicePeriodMemberGrid" {
			if asset[1] != "NONE_NEW;REUSES_00064_service_period_members" {
				t.Fatalf("%s migration ref = %s", asset[2], asset[1])
			}
		}
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
