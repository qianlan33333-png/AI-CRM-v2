package main

import "testing"

func TestMarketingStateHistoryScopeAndSelector(t *testing.T) {
	journal, err := newMarketingStateHistoryJournal("archive-run")
	if err != nil || journal.ValidateMarketingStateHistoryImportScope("archive-run") != nil {
		t.Fatal("marketing_state_scope_not_exact")
	}
	if journal.ValidateMarketingStateHistoryImportScope("other-run") == nil {
		t.Fatal("marketing_state_scope_accepts_other_archive")
	}
	if !validDomain("marketing-state-history") || validDomain("marketing_state_history") {
		t.Fatal("marketing_state_selector_not_exact")
	}
	if marketingStateHistoryImportVersion == staticImportVersion || marketingStateHistoryImportVersion == domainImportVersion {
		t.Fatal("marketing_state_version_not_isolated")
	}
}
