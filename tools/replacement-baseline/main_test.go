package main

import "testing"

func TestClassifyMatrixMatchesFrozenLunaBoundary(t *testing.T) {
	if got, status := classifyMatrix(matrixRecord{Disposition: "DEPRECATED"}); got != "RETIREMENT_APPROVED" || status != "RETIRED" {
		t.Fatalf("deprecated = %s", got)
	}
	if got, status := classifyMatrix(matrixRecord{TriggeredAPI: "none (client-only)"}); got != "UI_ONLY" || status != "REPLACED_BY_NEW_FRONTEND" {
		t.Fatalf("client-only = %s", got)
	}
	if got, status := classifyMatrix(matrixRecord{}); got != "BACKEND_REQUIRED" || status != "UNMAPPED" {
		t.Fatalf("default = %s", got)
	}
}

func TestClassifyRouteProtectsPublicProtocolsAndDrift(t *testing.T) {
	for _, tc := range []struct{ id, audience, effect, want string }{
		{"LEGACY-API-0053", "admin", "none", "UNCLASSIFIED_SOURCE_DRIFT"},
		{"LEGACY-API-0001", "public_h5", "none", "EXTERNAL_PROTOCOL"},
		{"LEGACY-API-0002", "callback", "none", "EXTERNAL_PROTOCOL"},
		{"LEGACY-API-0003", "external_integration", "none", "EXTERNAL_PROTOCOL"},
		{"LEGACY-API-0758", "admin", "none", "EXTERNAL_PROTOCOL"},
		{"LEGACY-API-0004", "admin", "staging_disabled", "UNCLASSIFIED"},
	} {
		record := apiRecord{MappingID: tc.id}
		record.Manifest.Audience = tc.audience
		record.Manifest.ExternalEffects = tc.effect
		got, status := classifyRoute(record)
		if got != tc.want {
			t.Fatalf("%s: got %s want %s", tc.id, got, tc.want)
		}
		if tc.want == "EXTERNAL_PROTOCOL" && status != "INVENTORIED" {
			t.Fatalf("%s: protocol status = %s", tc.id, status)
		}
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
	for _, asset := range assets {
		packages[asset[0]] = true
	}
	if len(packages) != 11 {
		t.Fatalf("packages = %d, want 11", len(packages))
	}
}
