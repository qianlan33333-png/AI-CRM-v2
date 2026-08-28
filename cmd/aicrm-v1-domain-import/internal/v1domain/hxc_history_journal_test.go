package v1domain

import (
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1hxchistory"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestHXCHistoryJournalPinsEightSourceScopes(t *testing.T) {
	values := hxcHistoryJournalsForTest(t, "archive-run")
	journal, err := NewHXCHistoryJournal(
		values[hxcport.HXCHistoryMeta], values[hxcport.HXCHistorySnapshot], values[hxcport.HXCHistoryActivationStatus], values[hxcport.HXCHistoryHuangxiaocanActivation],
		values[hxcport.HXCHistoryLead], values[hxcport.HXCHistoryBatch], values[hxcHistorySendRecordsKind], values[hxcHistorySendConfigKind],
	)
	if err != nil || journal.ValidateHXCHistoryImportScope("archive-run") != nil {
		t.Fatalf("journal=%#v err=%v", journal, err)
	}
	if values[hxcport.HXCHistoryMeta].scope.TargetTable != hxcHistoryMetaTarget || values[hxcport.HXCHistoryActivationStatus].scope.TargetTable != hxcHistoryActivationTarget ||
		values[hxcport.HXCHistoryHuangxiaocanActivation].scope.TargetTable != hxcHistoryActivationTarget || values[hxcHistorySendRecordsKind].scope.TargetTable != hxcHistoryArchiveTarget {
		t.Fatal("HXC source scopes drifted")
	}
}

func TestHXCHistoryJournalRejectsMixedRunAndArchiveKindsAsOwnerReceipts(t *testing.T) {
	values := hxcHistoryJournalsForTest(t, "archive-run")
	wrong, err := NewJournal(Scope{ImportVersion: hxcHistoryImportVersion, ArchiveRunID: "other-run", AdapterID: v1archive.DefaultAdapterID,
		TableID: v1hxchistory.SendConfigTableID, TargetDomain: hxcHistoryDomain, TargetTable: hxcHistoryArchiveTarget})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = NewHXCHistoryJournal(values[hxcport.HXCHistoryMeta], values[hxcport.HXCHistorySnapshot], values[hxcport.HXCHistoryActivationStatus], values[hxcport.HXCHistoryHuangxiaocanActivation], values[hxcport.HXCHistoryLead], values[hxcport.HXCHistoryBatch], values[hxcHistorySendRecordsKind], wrong); err == nil {
		t.Fatal("mixed archive run accepted")
	}
	if _, err = hxcHistoryTerminalFromReceipt(hxcport.HXCHistoryReceipt{Kind: hxcHistorySendRecordsKind}); err == nil {
		t.Fatal("archive-only kind accepted as owner receipt")
	}
}

func hxcHistoryJournalsForTest(t *testing.T, run string) map[string]*Journal {
	t.Helper()
	values := make(map[string]*Journal, len(hxcHistoryScopes))
	for _, scope := range hxcHistoryScopes {
		journal, err := NewJournal(Scope{ImportVersion: hxcHistoryImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID,
			TableID: scope.table, TargetDomain: hxcHistoryDomain, TargetTable: scope.target})
		if err != nil {
			t.Fatal(err)
		}
		values[scope.kind] = journal
	}
	return values
}
