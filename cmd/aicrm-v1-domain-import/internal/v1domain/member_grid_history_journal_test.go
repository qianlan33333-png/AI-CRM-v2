package v1domain

import (
	"crypto/sha256"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1membergridhistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

func TestNewMemberGridHistoryJournalRequiresFiveExactScopes(t *testing.T) {
	newJournals := func(run string) [5]*Journal {
		mappings := [][2]string{
			{v1membergridhistory.MemberViewsTableID, memberGridHistoryViewTargetTable},
			{v1membergridhistory.UsageSnapshotsTableID, memberGridHistoryUsageTargetTable},
			{v1membergridhistory.UsageSyncRunsTableID, memberGridHistoryContextTargetTable},
			{v1membergridhistory.MemberCollaboratorsTableID, memberGridHistoryContextTargetTable},
			{v1membergridhistory.MemberSharesTableID, memberGridHistoryContextTargetTable},
		}
		var result [5]*Journal
		for index, mapping := range mappings {
			journal, err := NewJournal(Scope{ImportVersion: memberGridHistoryImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID,
				TableID: mapping[0], TargetDomain: "product", TargetTable: mapping[1]})
			if err != nil {
				t.Fatal(err)
			}
			result[index] = journal
		}
		return result
	}
	run := v1membergridhistory.FixedUsageSnapshotRecoveryScope().ArchiveRunID
	values := newJournals(run)
	if _, err := NewMemberGridHistoryJournal(values[0], values[1], values[2], values[3], values[4]); err != nil {
		t.Fatal(err)
	}
	values = newJournals(run)
	values[4].scope.ArchiveRunID = "another-archive-run"
	if _, err := NewMemberGridHistoryJournal(values[0], values[1], values[2], values[3], values[4]); err == nil {
		t.Fatal("mixed_archive_run_accepted")
	}
	values = newJournals(run)
	values[1].scope.TargetTable = memberGridHistoryContextTargetTable
	if _, err := NewMemberGridHistoryJournal(values[0], values[1], values[2], values[3], values[4]); err == nil {
		t.Fatal("usage_scope_target_accepted")
	}
}

func TestMemberGridHistoryReceiptRoundTripRejectsStaticDrift(t *testing.T) {
	var source, payload, target [sha256.Size]byte
	source[0], payload[0], target[0] = 1, 2, 3
	receipt := productport.MemberGridHistoryReceipt{Kind: productport.MemberGridHistoryView, SourceIdentifier: SourceIdentifier(source), PayloadDigest: payload, TargetID: 9, TargetDigest: target}
	terminal, err := memberGridHistoryTerminalFromReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := memberGridHistoryReceiptFromTerminal(productport.MemberGridHistoryView, receipt.SourceIdentifier, terminal)
	if err != nil || actual != receipt {
		t.Fatalf("roundtrip=%#v err=%v", actual, err)
	}
	terminal.TargetDigest = [sha256.Size]byte{}
	if _, err = memberGridHistoryReceiptFromTerminal(productport.MemberGridHistoryView, receipt.SourceIdentifier, terminal); err == nil {
		t.Fatal("target_drift_accepted")
	}
}
