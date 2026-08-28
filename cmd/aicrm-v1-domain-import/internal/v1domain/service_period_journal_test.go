package v1domain

import (
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestServicePeriodHistoryJournalScopesAndReceipt(t *testing.T) {
	makeJournal := func(kind string) *Journal {
		scope := servicePeriodHistoryScopes[kind]
		j, err := NewJournal(Scope{ImportVersion: "v1-service-period-a1", ArchiveRunID: "archive", AdapterID: v1archive.DefaultAdapterID, TableID: scope[0], TargetDomain: "product", TargetTable: scope[1]})
		if err != nil {
			t.Fatal(err)
		}
		return j
	}
	d, e, v := makeJournal("definitions"), makeJournal("entitlements"), makeJournal("events")
	if _, err := NewServicePeriodHistoryJournal(d, e, v); err != nil {
		t.Fatal(err)
	}
	if _, err := NewServicePeriodHistoryJournal(d, v, e); err == nil {
		t.Fatal("mixed scopes accepted")
	}
	if _, err := NewServicePeriodHistoryJournal(nil, e, v); err == nil {
		t.Fatal("nil scope accepted")
	}
	e.scope.ArchiveRunID = "other"
	if _, err := NewServicePeriodHistoryJournal(d, e, v); err == nil {
		t.Fatal("mixed archive accepted")
	}
	source := SourceIdentifier([32]byte{1})
	terminal := TerminalReceipt{SourceKeyDigest: [32]byte{1}, PayloadDigest: [32]byte{2}, Disposition: "import", TargetID: "7", TargetDigest: [32]byte{3}}
	if got, err := servicePeriodHistoryReceipt(source, terminal); err != nil || got.TargetID != 7 {
		t.Fatalf("receipt=%+v err=%v", got, err)
	}
	terminal.TargetID = "007"
	if _, err := servicePeriodHistoryReceipt(source, terminal); err == nil {
		t.Fatal("noncanonical target accepted")
	}
}
