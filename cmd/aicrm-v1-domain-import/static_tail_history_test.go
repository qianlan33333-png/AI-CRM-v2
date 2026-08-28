package main

import "testing"

func TestStaticTailHistoryScopeAndSelector(t *testing.T) {
	journal, err := newStaticTailHistoryJournal("archive-run")
	if err != nil || journal.ValidateStaticTailHistoryImportScope("archive-run") != nil {
		t.Fatal("static_tail_scope_not_exact")
	}
	if journal.ValidateStaticTailHistoryImportScope("other-run") == nil {
		t.Fatal("static_tail_scope_accepts_other_archive")
	}
	if !validDomain("static-tail-history") || validDomain("static_tail_history") {
		t.Fatal("static_tail_selector_not_exact")
	}
	if staticTailHistoryImportVersion == staticImportVersion || staticTailHistoryImportVersion == domainImportVersion {
		t.Fatal("static_tail_version_not_isolated")
	}
}
