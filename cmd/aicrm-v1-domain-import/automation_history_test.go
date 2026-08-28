package main

import "testing"

func TestAutomationHistoryProductionComposition(t *testing.T) {
	journal, err := newAutomationHistoryJournal("v1-full-archive-20260827")
	if err != nil {
		t.Fatal(err)
	}
	if err = journal.ValidateAutomationHistoryImportScope("v1-full-archive-20260827"); err != nil {
		t.Fatal(err)
	}
	if err = journal.ValidateAutomationHistoryImportScope("other-run"); err == nil {
		t.Fatal("wrong archive scope accepted")
	}
	if !validDomain("automation-history") || validDomain("automation") {
		t.Fatal("history domain selector not closed")
	}
	if _, err = newAutomationHistoryJournal(""); err == nil {
		t.Fatal("empty archive run accepted")
	}
}
