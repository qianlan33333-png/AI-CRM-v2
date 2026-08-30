package main

import (
	"slices"
	"strings"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
)

func TestFinalMigrationManifestRegistersExactlyFortyDomains(t *testing.T) {
	manifest, err := loadFinalMigrationManifest("../../docs/release/final-v1-domain-migration-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Domains) != finalManifestDomainCount || len(finalDomainSpecs) != finalManifestDomainCount {
		t.Fatalf("manifest/spec domains = %d/%d, want %d", len(manifest.Domains), len(finalDomainSpecs), finalManifestDomainCount)
	}
	for _, item := range manifest.Domains {
		if item.Domain == "all" {
			t.Fatal("final manifest must not use the five-domain all shortcut")
		}
		if _, found := finalDomainSpecs[item.Domain]; !found {
			t.Fatalf("manifest domain %q has no final verifier", item.Domain)
		}
	}
	for _, domain := range []string{"campaign", "survey", "media", "radar", "shop"} {
		spec := finalDomainSpecs[domain]
		if spec.ImportVersion != domainImportVersion || len(spec.SourceTables) == 0 {
			t.Fatalf("%s was not explicitly split from domain=all: %#v", domain, spec)
		}
	}
	if !slices.Contains(finalDomainNames(manifest), "customer-timeline-history") {
		t.Fatal("final manifest is missing a late-history domain")
	}
	scopes := finalDomainsByReconciliationScope(manifest)
	if len(scopes) != 36 {
		t.Fatalf("reconciliation scopes = %d, want 36", len(scopes))
	}
	if shared := scopes[domainImportVersion]; !slices.Equal(shared, []string{"campaign", "media", "radar", "shop", "survey"}) {
		t.Fatalf("v1-domain-a1 must be one explicit five-domain proof, got %#v", shared)
	}
	for version, domains := range scopes {
		if version != domainImportVersion && len(domains) != 1 {
			t.Fatalf("non-shared reconciliation scope %s covers %#v", version, domains)
		}
	}
	if finalVerificationModel != "36_immutable_domain_reconciliations_then_editable_projection_then_read_only_aggregate" {
		t.Fatalf("unexpected verification model %q", finalVerificationModel)
	}
	short := manifest
	short.Domains = append([]finalMigrationDomain(nil), manifest.Domains[:len(manifest.Domains)-1]...)
	if err := validateFinalMigrationManifest(short); err == nil || !strings.Contains(err.Error(), "enumerate") {
		t.Fatalf("short manifest was accepted: %v", err)
	}
	shortcut := manifest
	shortcut.Domains = append([]finalMigrationDomain(nil), manifest.Domains...)
	shortcut.Domains[0].Domain = "all"
	if err := validateFinalMigrationManifest(shortcut); err == nil || !strings.Contains(err.Error(), "invalid domain") {
		t.Fatalf("all shortcut was accepted: %v", err)
	}
}

func TestFinalReconcileRejectsAmbiguousOrSourceConnectedInvocationBeforeDatabase(t *testing.T) {
	t.Setenv("AICRM_DM01_SOURCE_DATABASE_URL", "")
	environment := appconfig.V1ArchiveRuntime{TargetDatabaseURL: "postgres:///must-not-connect"}
	if err := run([]string{"--mode=final-reconcile", "--domain=all", "--archive-run-id=archive", "--dm01-run-id=1"}, environment); err == nil || !strings.Contains(err.Error(), "domain=final") {
		t.Fatalf("domain=all was accepted as final reconciliation: %v", err)
	}
	if err := run([]string{"--mode=final-reconcile", "--domain=final", "--archive-run-id=archive"}, environment); err == nil || !strings.Contains(err.Error(), "dm01-run-id") {
		t.Fatalf("missing DM01 identity proof reached database: %v", err)
	}
	environment.SourceDatabaseURL = "postgres:///forbidden-v1"
	if err := run([]string{"--mode=final-reconcile", "--domain=final", "--archive-run-id=archive", "--dm01-run-id=1"}, environment); err == nil || !strings.Contains(err.Error(), "connections to be unset") {
		t.Fatalf("final reconciliation accepted a source connection: %v", err)
	}
}

func TestFinalReconcileUsesStoredSelectionForSingleDomainReceipt(t *testing.T) {
	current := reconciliationCounts{Receipts: 7, Imported: 5, Archived: 1, Quarantined: 1, Verified: 7}
	reconciled := reconciliationCounts{Selected: 7, Receipts: 7, Imported: 5, Archived: 1, Quarantined: 1, Verified: 7}
	if sameCounts(current, reconciled) {
		t.Fatal("a receipt-only domain cannot invent selected_source_count")
	}
	if !sameReceiptCounts(current, reconciled) {
		t.Fatal("receipt-only domain must validate its current receipt counts before loading stored selection")
	}
	current.Selected = reconciled.Selected
	if !sameCounts(current, reconciled) {
		t.Fatal("stored selected_source_count was not restored after receipt validation")
	}
}

func TestFinalReconciliationGroupsFailClosedWhenMissingOrIncomplete(t *testing.T) {
	manifest, err := loadFinalMigrationManifest("../../docs/release/final-v1-domain-migration-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	scopes := finalDomainsByReconciliationScope(manifest)
	actual := make(map[string]reconciliationCounts, len(scopes))
	reconciled := make(map[string]reconciliationCounts, len(scopes))
	for version := range scopes {
		actual[version] = reconciliationCounts{}
		reconciled[version] = reconciliationCounts{}
	}
	if err := validateFinalReconciliationGroups(scopes, actual, reconciled); err != nil {
		t.Fatalf("complete zero-row reconciliation groups rejected: %v", err)
	}
	delete(reconciled, staticImportVersion)
	if err := validateFinalReconciliationGroups(scopes, actual, reconciled); err == nil || !strings.Contains(err.Error(), "is missing") {
		t.Fatalf("missing reconciliation group accepted: %v", err)
	}
	reconciled[staticImportVersion] = reconciliationCounts{}
	actual[domainImportVersion] = reconciliationCounts{Selected: 4, Receipts: 4, Imported: 4, Verified: 4}
	reconciled[domainImportVersion] = reconciliationCounts{Selected: 5, Receipts: 5, Imported: 5, Verified: 5}
	if err := validateFinalReconciliationGroups(scopes, actual, reconciled); err == nil || !strings.Contains(err.Error(), "does not cover") {
		t.Fatalf("incomplete shared reconciliation group accepted: %v", err)
	}
}

func TestFinalPreflightClassifiesOnlyWholeUnsealedScopes(t *testing.T) {
	manifest, err := loadFinalMigrationManifest("../../docs/release/final-v1-domain-migration-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	scopes := finalDomainsByReconciliationScope(manifest)
	sealed := make(map[string]bool, len(scopes))
	receipts := make(map[string]int64, len(scopes))
	for version := range scopes {
		sealed[version] = true
	}
	for _, domain := range []string{"hxc-chat-job-history", "hxc-member-usage-history", "cycle-observation-history", "customer-timeline-history", "audience-activity-history"} {
		sealed[finalDomainSpecs[domain].ImportVersion] = false
	}
	missing, err := classifyFinalPreflightScopes(scopes, sealed, receipts)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"audience-activity-history", "customer-timeline-history", "cycle-observation-history", "hxc-chat-job-history", "hxc-member-usage-history"}
	if !slices.Equal(missing, want) {
		t.Fatalf("missing domains = %#v, want %#v", missing, want)
	}
	receipts[domainImportVersion] = 1
	sealed[domainImportVersion] = false
	if _, err = classifyFinalPreflightScopes(scopes, sealed, receipts); err == nil || !strings.Contains(err.Error(), "receipts without reconciliation") {
		t.Fatalf("partial shared scope was accepted: %v", err)
	}
	if err = validateFinalPreflightVersionSet(scopes, []string{staticImportVersion, "unknown-import-version"}); err == nil || !strings.Contains(err.Error(), "unknown import version") {
		t.Fatalf("unknown journal version was accepted: %v", err)
	}
}

func TestFinalIdentityProofRejectsEmptyOrPartiallyVerifiedRun(t *testing.T) {
	for _, proof := range []finalIdentityProof{
		{DM01RunID: 1},
		{DM01RunID: 1, MappingCount: 2, VerifiedMapping: 1},
	} {
		if err := validateFinalIdentityProof(proof); err == nil {
			t.Fatalf("identity proof %#v was accepted", proof)
		}
	}
	if err := validateFinalIdentityProof(finalIdentityProof{DM01RunID: 1, MappingCount: 2, VerifiedMapping: 2}); err != nil {
		t.Fatalf("complete identity proof rejected: %v", err)
	}
}

func TestFinalEditableProjectionProofRequiresCompleteCurrentObjects(t *testing.T) {
	valid := finalEditableProjectionProof{
		ProductSourceCount: 29, ProductProjectedCount: 29, ProductReceiptBoundCount: 29,
		ServicePeriodSourceCount: 2, ServicePeriodProjectedCount: 2,
		ProductLegacyImageSourceCount: 46, ProductImageReferenceCount: 0, ProductLegacyReferenceCount: 0,
		AudienceSourceCount: 4, AudienceProjectedCount: 4,
		AudienceIdentitySkippedCount: 0,
		AudienceGroupSourceCount:     1, AudienceGroupProjectedCount: 1,
		AudienceSourceMembers: 517, AudienceMappedMembers: 517, AudienceProjectedMembers: 517,
		AutomationAgentSourceCount: 6, AutomationAgentProjectedCount: 6, AutomationAgentPausedCount: 6, AutomationAgentDisabledCount: 6, AutomationMaterialReferenceCount: 0,
		TargetDigest: strings.Repeat("01", 32),
	}
	if err := validateFinalEditableProjectionProof(valid); err != nil {
		t.Fatalf("complete editable projection rejected: %v", err)
	}
	for _, mutate := range []func(*finalEditableProjectionProof){
		func(value *finalEditableProjectionProof) { value.ProductProjectedCount-- },
		func(value *finalEditableProjectionProof) { value.ProductReceiptBoundCount-- },
		func(value *finalEditableProjectionProof) { value.ServicePeriodProjectedCount-- },
		func(value *finalEditableProjectionProof) { value.ProductImageReferenceCount++ },
		func(value *finalEditableProjectionProof) { value.ProductLegacyReferenceCount++ },
		func(value *finalEditableProjectionProof) { value.AudienceProjectedCount-- },
		func(value *finalEditableProjectionProof) { value.AudienceIdentitySkippedCount++ },
		func(value *finalEditableProjectionProof) { value.AudienceGroupProjectedCount-- },
		func(value *finalEditableProjectionProof) { value.AudienceMappedMembers-- },
		func(value *finalEditableProjectionProof) { value.AudienceProjectedMembers-- },
		func(value *finalEditableProjectionProof) { value.AutomationAgentProjectedCount-- },
		func(value *finalEditableProjectionProof) { value.AutomationAgentPausedCount-- },
		func(value *finalEditableProjectionProof) { value.AutomationAgentDisabledCount-- },
		func(value *finalEditableProjectionProof) { value.AutomationMaterialReferenceCount++ },
		func(value *finalEditableProjectionProof) { value.TargetDigest = "broken" },
	} {
		candidate := valid
		mutate(&candidate)
		if err := validateFinalEditableProjectionProof(candidate); err == nil {
			t.Fatalf("incomplete editable projection accepted: %+v", candidate)
		}
	}
	withSkipped := valid
	withSkipped.AudienceSourceCount = 6
	withSkipped.AudienceIdentitySkippedCount = 2
	if err := validateFinalEditableProjectionProof(withSkipped); err != nil {
		t.Fatalf("explicitly skipped Audience packages rejected: %v", err)
	}
}

func TestHXCMemberUsageHistoryRequiresLocalKeysBeforeConnecting(t *testing.T) {
	for _, mode := range []string{"import", "reconcile"} {
		for _, kind := range []string{"source", "hmac", "aes"} {
			env := appconfig.V1ArchiveRuntime{TargetDatabaseURL: "postgres:///must-not-connect", ArchiveKey: strings.Repeat("k", 32), SourceHMACKey: strings.Repeat("h", 32)}
			switch kind {
			case "source":
				env.SourceDatabaseURL = "postgres:///forbidden-v1"
			case "hmac":
				env.SourceHMACKey = ""
			case "aes":
				env.ArchiveKey = ""
			}
			err := run([]string{"--domain=hxc-member-usage-history", "--mode=" + mode, "--archive-run-id=archive"}, env)
			if err == nil || (!strings.Contains(err.Error(), "local-only archive keys") && !strings.Contains(err.Error(), "32-byte archive key")) {
				t.Fatalf("%s/%s: %v", mode, kind, err)
			}
		}
	}
	if !validDomain("hxc-member-usage-history") {
		t.Fatal("domain not selectable")
	}
}

func TestParseActorIDs(t *testing.T) {
	actors, err := parseActorIDs("QianLan=1,ZhaoYanFang=2")
	if err != nil || actors["QianLan"] != 1 || actors["ZhaoYanFang"] != 2 {
		t.Fatalf("actors/error = %#v/%v", actors, err)
	}
	for _, invalid := range []string{"", "QianLan", "QianLan=0", "QianLan=1,QianLan=2", " QianLan=1"} {
		if _, err := parseActorIDs(invalid); err == nil {
			t.Fatalf("%q unexpectedly accepted", invalid)
		}
	}
}

func TestHXCChatJobHistoryRequiresLocalArchiveKeys(t *testing.T) {
	if !validDomain("hxc-chat-job-history") {
		t.Fatal("history domain missing")
	}
	for _, mode := range []string{"import", "reconcile"} {
		for _, field := range []string{"source", "hmac", "archive"} {
			env := appconfig.V1ArchiveRuntime{TargetDatabaseURL: "invalid://must-not-connect", SourceHMACKey: strings.Repeat("h", 32), ArchiveKey: strings.Repeat("k", 32)}
			switch field {
			case "source":
				env.SourceDatabaseURL = "postgres:///forbidden-source"
			case "hmac":
				env.SourceHMACKey = ""
			case "archive":
				env.ArchiveKey = ""
			}
			err := run([]string{"--domain=hxc-chat-job-history", "--mode=" + mode, "--archive-run-id=archive"}, env)
			if err == nil || (!strings.Contains(err.Error(), "local-only archive keys") && !strings.Contains(err.Error(), "32-byte archive key")) {
				t.Fatalf("%s/%s did not fail before DB configuration: %v", mode, field, err)
			}
		}
	}
}

func TestCycleObservationRequiresLocalArchiveKeysBeforeConnecting(t *testing.T) {
	if !validDomain("cycle-observation-history") {
		t.Fatal("cycle observation domain missing")
	}
	for _, mode := range []string{"import", "reconcile"} {
		for _, change := range []func(*appconfig.V1ArchiveRuntime){
			func(v *appconfig.V1ArchiveRuntime) { v.SourceDatabaseURL = "postgres:///never-read-v1" },
			func(v *appconfig.V1ArchiveRuntime) { v.SourceHMACKey = "" },
			func(v *appconfig.V1ArchiveRuntime) { v.ArchiveKey = "" },
		} {
			env := appconfig.V1ArchiveRuntime{TargetDatabaseURL: "invalid-target-must-not-connect", SourceHMACKey: strings.Repeat("h", 32), ArchiveKey: strings.Repeat("k", 32)}
			change(&env)
			err := run([]string{"--domain=cycle-observation-history", "--mode=" + mode, "--archive-run-id=archive"}, env)
			if err == nil || (!strings.Contains(err.Error(), "local-only archive keys") && !strings.Contains(err.Error(), "32-byte archive key")) {
				t.Fatalf("guard failed before connection: %v", err)
			}
		}
	}
}

func TestStaticAndChannelPackagesRequireExplicitDM01BeforeConnecting(t *testing.T) {
	t.Setenv("AICRM_DM01_SOURCE_HMAC_KEY", "")
	for _, domain := range []string{"static", "channel"} {
		err := run([]string{"--domain=" + domain, "--archive-run-id=archive", "--migration-actor=1"}, appconfig.V1ArchiveRuntime{
			TargetDatabaseURL: "postgres:///must-not-connect", ArchiveKey: strings.Repeat("k", 32),
		})
		if err == nil || !strings.Contains(err.Error(), "dm01-run-id") {
			t.Fatalf("missing DM01 binding must fail before database access: %v", err)
		}
		if !validDomain(domain) {
			t.Fatal("package must be explicitly selectable")
		}
	}
	if staticImportVersion == domainImportVersion || channelImportVersion == staticImportVersion || channelImportVersion == domainImportVersion {
		t.Fatal("packages must use separate immutable import versions")
	}
}
