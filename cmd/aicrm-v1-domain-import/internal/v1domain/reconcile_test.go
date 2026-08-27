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
	if len(reconciledTables) != 10 || len(staticReconciledTables) != 6 || len(targetBySourceTable) != len(reconciledTables)+len(staticReconciledTables) {
		t.Fatalf("unexpected reconciled table set")
	}
	seen := map[string]bool{}
	for _, table := range append(append([]string(nil), reconciledTables...), staticReconciledTables...) {
		if seen[table] {
			t.Fatalf("duplicate source table %s", table)
		}
		seen[table] = true
		if _, found := targetBySourceTable[table]; !found {
			t.Fatalf("missing target mapping for %s", table)
		}
	}
}
