package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestGroupOpsHistoryJournalRoundTripsAllFiveReceiptKinds(t *testing.T) {
	plans, groupChats, snapshots, groups, nodes := newGroupOpsTerminalFake(), newGroupOpsTerminalFake(), newGroupOpsTerminalFake(), newGroupOpsTerminalFake(), newGroupOpsTerminalFake()
	journal, err := newGroupOpsHistoryJournal(plans, groupChats, snapshots, groups, nodes)
	if err != nil {
		t.Fatal(err)
	}
	for index, kind := range []string{groupOpsPlansKind, groupOpsGroupChatsKind, groupOpsSnapshotsKind, groupOpsGroupsKind, groupOpsNodesKind} {
		receipt := groupOpsReceipt(byte(index + 1))
		if err := journal.RecordGroupOpsHistory(context.Background(), kind, receipt); err != nil {
			t.Fatalf("record %s: %v", kind, err)
		}
		got, found, err := journal.LoadGroupOpsHistory(context.Background(), kind, receipt.SourceIdentifier)
		if err != nil || !found || got != receipt {
			t.Fatalf("load %s = %#v/%v/%v", kind, got, found, err)
		}
	}
	if len(plans.values) != 1 || len(groupChats.values) != 1 || len(snapshots.values) != 1 || len(groups.values) != 1 || len(nodes.values) != 1 {
		t.Fatalf("receipt scopes crossed: %d/%d/%d/%d/%d", len(plans.values), len(groupChats.values), len(snapshots.values), len(groups.values), len(nodes.values))
	}
}

func TestGroupOpsHistoryJournalRejectsUnsafeReceiptAndMalformedTerminal(t *testing.T) {
	fake := newGroupOpsTerminalFake()
	journal, err := newGroupOpsHistoryJournal(fake, newGroupOpsTerminalFake(), newGroupOpsTerminalFake(), newGroupOpsTerminalFake(), newGroupOpsTerminalFake())
	if err != nil {
		t.Fatal(err)
	}
	receipt := groupOpsReceipt(1)
	nonCanonicalSource := groupOpsReceipt(0xab)
	nonCanonicalSource.SourceIdentifier = strings.ToUpper(nonCanonicalSource.SourceIdentifier)
	for _, invalid := range []groupopsport.HistoricalReceipt{
		{SourceIdentifier: receipt.SourceIdentifier, PayloadDigest: receipt.PayloadDigest, TargetID: receipt.TargetID, TargetDigest: receipt.TargetDigest, Replayed: true},
		{SourceIdentifier: receipt.SourceIdentifier, TargetID: receipt.TargetID, TargetDigest: receipt.TargetDigest},
		{SourceIdentifier: receipt.SourceIdentifier, PayloadDigest: receipt.PayloadDigest, TargetDigest: receipt.TargetDigest},
		{SourceIdentifier: "ABC", PayloadDigest: receipt.PayloadDigest, TargetID: receipt.TargetID, TargetDigest: receipt.TargetDigest},
		nonCanonicalSource,
	} {
		if err := journal.RecordGroupOpsHistory(context.Background(), groupOpsPlansKind, invalid); !errors.Is(err, groupopsport.ErrHistoryInvalid) {
			t.Fatalf("invalid receipt error = %v", err)
		}
	}
	if err := journal.RecordGroupOpsHistory(context.Background(), "other", receipt); !errors.Is(err, groupopsport.ErrHistoryInvalid) {
		t.Fatalf("unknown kind error = %v", err)
	}
	fake.values[receipt.SourceIdentifier] = TerminalReceipt{SourceKeyDigest: groupOpsDigest(1), PayloadDigest: receipt.PayloadDigest, Disposition: "archive", Reason: "runtime_state", Metadata: map[string]any{}}
	if _, _, err := journal.LoadGroupOpsHistory(context.Background(), groupOpsPlansKind, receipt.SourceIdentifier); !errors.Is(err, groupopsport.ErrHistoryConflict) {
		t.Fatalf("archive terminal error = %v", err)
	}
	fake.values[receipt.SourceIdentifier] = TerminalReceipt{SourceKeyDigest: groupOpsDigest(1), PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: "41", TargetDigest: receipt.TargetDigest, Metadata: map[string]any{"unexpected": true}}
	if _, _, err := journal.LoadGroupOpsHistory(context.Background(), groupOpsPlansKind, receipt.SourceIdentifier); !errors.Is(err, groupopsport.ErrHistoryConflict) {
		t.Fatalf("metadata terminal error = %v", err)
	}
}

func TestNewGroupOpsHistoryJournalBindsExactScopeAndRun(t *testing.T) {
	plans := groupOpsScopedJournal(groupOpsPlansTable, "group_ops_plans", "run")
	groupChats := groupOpsScopedJournal(groupOpsGroupChatsTable, "group_ops_v1_history_directory", "run")
	snapshots := groupOpsScopedJournal(groupOpsSnapshotsTable, "group_ops_v1_history_directory", "run")
	groups := groupOpsScopedJournal(groupOpsGroupsTable, "group_ops_v1_history_groups", "run")
	nodes := groupOpsScopedJournal(groupOpsNodesTable, "group_ops_v1_history_nodes", "run")
	if _, err := NewGroupOpsHistoryJournal(plans, groupChats, snapshots, groups, nodes); err != nil {
		t.Fatal(err)
	}
	wrongTarget := groupOpsScopedJournal(groupOpsNodesTable, "group_ops_v1_history_groups", "run")
	if _, err := NewGroupOpsHistoryJournal(plans, groupChats, snapshots, groups, wrongTarget); !errors.Is(err, groupopsport.ErrHistoryInvalid) {
		t.Fatalf("wrong target error = %v", err)
	}
	differentRun := groupOpsScopedJournal(groupOpsNodesTable, "group_ops_v1_history_nodes", "other-run")
	if _, err := NewGroupOpsHistoryJournal(plans, groupChats, snapshots, groups, differentRun); !errors.Is(err, groupopsport.ErrHistoryInvalid) {
		t.Fatalf("different run error = %v", err)
	}
	wrongVersion := groupOpsScopedJournal(groupOpsNodesTable, "group_ops_v1_history_nodes", "run")
	wrongVersion.scope.ImportVersion = "v1-groupops-a2"
	if _, err := NewGroupOpsHistoryJournal(plans, groupChats, snapshots, groups, wrongVersion); !errors.Is(err, groupopsport.ErrHistoryInvalid) {
		t.Fatalf("wrong version error = %v", err)
	}
}

type groupOpsTerminalFake struct {
	values map[string]TerminalReceipt
}

func newGroupOpsTerminalFake() *groupOpsTerminalFake {
	return &groupOpsTerminalFake{values: map[string]TerminalReceipt{}}
}

func (journal *groupOpsTerminalFake) LoadTerminal(_ context.Context, source string) (TerminalReceipt, bool, error) {
	receipt, found := journal.values[source]
	return receipt, found, nil
}

func (journal *groupOpsTerminalFake) Record(_ context.Context, receipt TerminalReceipt) error {
	key := SourceIdentifier(receipt.SourceKeyDigest)
	if found, exists := journal.values[key]; exists && !sameGroupOpsOwnerTerminal(found, receipt) {
		return ErrConflict
	}
	journal.values[key] = receipt
	return nil
}

func sameGroupOpsOwnerTerminal(left, right TerminalReceipt) bool {
	return left.SourceKeyDigest == right.SourceKeyDigest && left.PayloadDigest == right.PayloadDigest &&
		left.Disposition == right.Disposition && left.Reason == right.Reason && left.TargetID == right.TargetID &&
		left.TargetDigest == right.TargetDigest && len(left.Metadata) == len(right.Metadata)
}

func groupOpsReceipt(first byte) groupopsport.HistoricalReceipt {
	return groupopsport.HistoricalReceipt{
		SourceIdentifier: SourceIdentifier(groupOpsDigest(first)),
		PayloadDigest:    groupOpsDigest(first + 10),
		TargetID:         int64(first) + 40,
		TargetDigest:     groupOpsDigest(first + 20),
	}
}

func groupOpsDigest(first byte) (digest [sha256.Size]byte) {
	digest[0] = first
	return digest
}

func groupOpsScopedJournal(tableID, targetTable, archiveRunID string) *Journal {
	return &Journal{scope: Scope{ImportVersion: groupOpsImportVersion, ArchiveRunID: archiveRunID, AdapterID: v1archive.DefaultAdapterID, TableID: tableID, TargetDomain: groupOpsTargetDomain, TargetTable: targetTable}, tx: func(context.Context) (pgx.Tx, error) {
		return nil, nil
	}}
}
