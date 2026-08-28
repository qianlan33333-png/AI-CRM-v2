package main

import "testing"

func TestCustomerStateHistoryScopeAndSelector(t *testing.T) {
	journal, err := newCustomerStateHistoryJournal("archive-run")
	if err != nil || journal.ValidateCustomerStateHistoryImportScope("archive-run") != nil {
		t.Fatal("customer_state_scope_not_exact")
	}
	if journal.ValidateCustomerStateHistoryImportScope("other-run") == nil {
		t.Fatal("customer_state_scope_accepts_other_archive")
	}
	if !validDomain("customer-state-history") || validDomain("customer_state_history") {
		t.Fatal("customer_state_selector_not_exact")
	}
	if customerStateHistoryImportVersion == staticImportVersion || customerStateHistoryImportVersion == domainImportVersion {
		t.Fatal("customer_state_version_not_isolated")
	}
}
