package v1domain

import (
	"errors"
	"testing"
)

func TestParseCampaignStepTarget(t *testing.T) {
	campaignCode, step, err := parseCampaignStepTarget("v1-history-campaign:42")
	if err != nil || campaignCode != "v1-history-campaign" || step != 42 {
		t.Fatalf("unexpected parse result: code=%q step=%d err=%v", campaignCode, step, err)
	}
}

func TestParseCampaignStepTargetRejectsInvalid(t *testing.T) {
	for _, value := range []string{"", "campaign", ":1", "campaign:0", "campaign:nope", "campaign:101", "bad:campaign:1"} {
		if _, _, err := parseCampaignStepTarget(value); !errors.Is(err, ErrConflict) {
			t.Fatalf("expected conflict for %q, got %v", value, err)
		}
	}
}

func TestReconciledTableSetIsClosed(t *testing.T) {
	if len(reconciledTables) != 10 || len(staticReconciledTables) != 6 || len(financeReconciledTables) != 2 || len(channelReconciledTables) != 9 || len(servicePeriodReconciledTables) != 3 || len(couponReconciledTables) != 4 || len(groupOpsReconciledTables) != 11 || len(audienceHistoryScopes) != 8 || len(contactHistoryReconciledTables) != 4 || len(memberGridHistoryReconciledTables) != 5 || len(campaignHistoryReconciledTables) != 5 || len(wecomContactHistoryReconciledTables) != 2 || len(targetBySourceTable) != len(reconciledTables)+len(staticReconciledTables)+len(financeReconciledTables)+3+len(servicePeriodReconciledTables)+len(couponReconciledTables)+5+len(audienceHistoryScopes)+5+len(campaignHistoryReconciledTables)+len(wecomContactHistoryReconciledTables) {
		t.Fatalf("unexpected reconciled table set")
	}
	seen := map[string]bool{}
	for _, table := range memberGridHistoryReconciledTables {
		if !isMemberGridHistorySource(table) || seen[table] {
			t.Fatalf("invalid Member Grid source set: %s", table)
		}
		seen[table] = true
		_, mapped := targetBySourceTable[table]
		if mapped != (table == "public/service_period_member_views" || table == "public/service_period_huangyoucan_usage_snapshot") {
			t.Fatalf("only Member Grid views and usage may have a history target: %s", table)
		}
	}
	all := append(append(append([]string(nil), reconciledTables...), staticReconciledTables...), financeReconciledTables...)
	all = append(all, servicePeriodReconciledTables...)
	all = append(all, couponReconciledTables...)
	all = append(all, groupOpsReconciledTables[:5]...)
	all = append(all, messageHistoryTableID)
	for _, scope := range audienceHistoryScopes {
		all = append(all, scope.source)
		mapping := targetBySourceTable[scope.source]
		if mapping.domain != "segment" || mapping.table != scope.target {
			t.Fatalf("audience history mapping mismatch for %s", scope.source)
		}
	}
	all = append(all, "public/sidebar_customer_profile_fields", "public/owner_migration_results")
	all = append(all, campaignHistoryReconciledTables...)
	all = append(all, wecomContactHistoryReconciledTables...)
	for _, table := range all {
		if seen[table] {
			t.Fatalf("duplicate source table %s", table)
		}
		seen[table] = true
		if _, found := targetBySourceTable[table]; !found {
			t.Fatalf("missing target mapping for %s", table)
		}
	}
	for _, table := range channelReconciledTables {
		if seen[table] {
			t.Fatalf("duplicate channel source table %s", table)
		}
		seen[table] = true
		_, mapped := targetBySourceTable[table]
		if mapped != (table == "public/automation_channel" || table == "public/automation_channel_contact" || table == "public/automation_channel_assignee") {
			t.Fatalf("only channel definitions and readonly relations may have a canonical import target: %s", table)
		}
	}
	for _, table := range []string{"public/owner_migration_import_sessions", "public/owner_migration_previews"} {
		if _, mapped := targetBySourceTable[table]; mapped {
			t.Fatalf("owner context must stay archive-only: %s", table)
		}
	}
}
