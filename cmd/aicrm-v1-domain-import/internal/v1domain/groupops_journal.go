package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"

	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	groupOpsImportVersion = "v1-groupops-a1"
	groupOpsTargetDomain  = "groupops"

	groupOpsPlansKind      = "plans"
	groupOpsGroupChatsKind = "group_chats"
	groupOpsSnapshotsKind  = "snapshots"
	groupOpsGroupsKind     = "groups"
	groupOpsNodesKind      = "nodes"

	groupOpsPlansTable      = "public/automation_group_ops_plans"
	groupOpsGroupChatsTable = "public/group_chats"
	groupOpsSnapshotsTable  = "public/wecom_group_chat_snapshots"
	groupOpsGroupsTable     = "public/automation_group_ops_plan_groups"
	groupOpsNodesTable      = "public/automation_group_ops_plan_nodes"
)

// groupOpsTerminalJournal remains private so production construction is bound
// to the migration-owned Journal rather than a second receipt mechanism.
type groupOpsTerminalJournal interface {
	LoadTerminal(context.Context, string) (TerminalReceipt, bool, error)
	Record(context.Context, TerminalReceipt) error
}

// GroupOpsHistoryJournal gives the Group Ops owner five narrowly scoped,
// immutable history receipt streams. It does not record runtime execution or
// Provider state.
type GroupOpsHistoryJournal struct {
	plans      groupOpsTerminalJournal
	groupChats groupOpsTerminalJournal
	snapshots  groupOpsTerminalJournal
	groups     groupOpsTerminalJournal
	nodes      groupOpsTerminalJournal
}

var _ groupopsport.HistoricalJournal = (*GroupOpsHistoryJournal)(nil)

func NewGroupOpsHistoryJournal(plans, groupChats, snapshots, groups, nodes *Journal) (*GroupOpsHistoryJournal, error) {
	if !validGroupOpsJournalScope(plans, groupOpsPlansTable, "group_ops_plans") ||
		!validGroupOpsJournalScope(groupChats, groupOpsGroupChatsTable, "group_ops_v1_history_directory") ||
		!validGroupOpsJournalScope(snapshots, groupOpsSnapshotsTable, "group_ops_v1_history_directory") ||
		!validGroupOpsJournalScope(groups, groupOpsGroupsTable, "group_ops_v1_history_groups") ||
		!validGroupOpsJournalScope(nodes, groupOpsNodesTable, "group_ops_v1_history_nodes") ||
		!sameGroupOpsJournalRun(plans, groupChats) || !sameGroupOpsJournalRun(plans, snapshots) ||
		!sameGroupOpsJournalRun(plans, groups) || !sameGroupOpsJournalRun(plans, nodes) {
		return nil, groupopsport.ErrHistoryInvalid
	}
	return &GroupOpsHistoryJournal{plans: plans, groupChats: groupChats, snapshots: snapshots, groups: groups, nodes: nodes}, nil
}

func newGroupOpsHistoryJournal(plans, groupChats, snapshots, groups, nodes groupOpsTerminalJournal) (*GroupOpsHistoryJournal, error) {
	if plans == nil || groupChats == nil || snapshots == nil || groups == nil || nodes == nil {
		return nil, groupopsport.ErrHistoryInvalid
	}
	return &GroupOpsHistoryJournal{plans: plans, groupChats: groupChats, snapshots: snapshots, groups: groups, nodes: nodes}, nil
}

func validGroupOpsJournalScope(journal *Journal, tableID, targetTable string) bool {
	return journal != nil && journal.tx != nil && journal.scope.valid() &&
		journal.scope.ImportVersion == groupOpsImportVersion && journal.scope.AdapterID == v1archive.DefaultAdapterID &&
		journal.scope.TableID == tableID && journal.scope.TargetDomain == groupOpsTargetDomain && journal.scope.TargetTable == targetTable
}

func sameGroupOpsJournalRun(left, right *Journal) bool {
	return left != nil && right != nil && left.scope.ImportVersion == right.scope.ImportVersion &&
		left.scope.ArchiveRunID == right.scope.ArchiveRunID && left.scope.AdapterID == right.scope.AdapterID
}

func (journal *GroupOpsHistoryJournal) LoadGroupOpsHistory(ctx context.Context, kind, sourceIdentifier string) (groupopsport.HistoricalReceipt, bool, error) {
	selected, err := journal.selectJournal(kind)
	if err != nil {
		return groupopsport.HistoricalReceipt{}, false, err
	}
	sourceKey, err := ParseSourceIdentifier(sourceIdentifier)
	if err != nil || sourceKey == ([sha256.Size]byte{}) || sourceIdentifier != SourceIdentifier(sourceKey) {
		return groupopsport.HistoricalReceipt{}, false, groupopsport.ErrHistoryInvalid
	}
	terminal, found, err := selected.LoadTerminal(ctx, sourceIdentifier)
	if err != nil || !found {
		return groupopsport.HistoricalReceipt{}, found, err
	}
	receipt, err := groupOpsReceiptFromTerminal(sourceIdentifier, terminal)
	if err != nil {
		return groupopsport.HistoricalReceipt{}, false, err
	}
	return receipt, true, nil
}

func (journal *GroupOpsHistoryJournal) RecordGroupOpsHistory(ctx context.Context, kind string, receipt groupopsport.HistoricalReceipt) error {
	selected, err := journal.selectJournal(kind)
	if err != nil {
		return err
	}
	terminal, err := groupOpsTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return selected.Record(ctx, terminal)
}

func (journal *GroupOpsHistoryJournal) selectJournal(kind string) (groupOpsTerminalJournal, error) {
	if journal == nil {
		return nil, groupopsport.ErrHistoryInvalid
	}
	switch kind {
	case groupOpsPlansKind:
		if journal.plans != nil {
			return journal.plans, nil
		}
	case groupOpsGroupChatsKind:
		if journal.groupChats != nil {
			return journal.groupChats, nil
		}
	case groupOpsSnapshotsKind:
		if journal.snapshots != nil {
			return journal.snapshots, nil
		}
	case groupOpsGroupsKind:
		if journal.groups != nil {
			return journal.groups, nil
		}
	case groupOpsNodesKind:
		if journal.nodes != nil {
			return journal.nodes, nil
		}
	}
	return nil, groupopsport.ErrHistoryInvalid
}

func groupOpsTerminalFromReceipt(receipt groupopsport.HistoricalReceipt) (TerminalReceipt, error) {
	sourceKey, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || sourceKey == ([sha256.Size]byte{}) || receipt.SourceIdentifier != SourceIdentifier(sourceKey) || receipt.PayloadDigest == ([sha256.Size]byte{}) ||
		receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.Replayed {
		return TerminalReceipt{}, groupopsport.ErrHistoryInvalid
	}
	return TerminalReceipt{
		SourceKeyDigest: sourceKey,
		PayloadDigest:   receipt.PayloadDigest,
		Disposition:     "import",
		TargetID:        strconv.FormatInt(receipt.TargetID, 10),
		TargetDigest:    receipt.TargetDigest,
		Metadata:        map[string]any{},
	}, nil
}

func groupOpsReceiptFromTerminal(sourceIdentifier string, terminal TerminalReceipt) (groupopsport.HistoricalReceipt, error) {
	sourceKey, err := ParseSourceIdentifier(sourceIdentifier)
	if err != nil || sourceKey == ([sha256.Size]byte{}) || sourceIdentifier != SourceIdentifier(sourceKey) || terminal.SourceKeyDigest != sourceKey ||
		terminal.PayloadDigest == ([sha256.Size]byte{}) || terminal.TargetDigest == ([sha256.Size]byte{}) ||
		terminal.Disposition != "import" || terminal.Reason != "" || len(terminal.Metadata) != 0 {
		return groupopsport.HistoricalReceipt{}, groupopsport.ErrHistoryConflict
	}
	targetID, err := strconv.ParseInt(terminal.TargetID, 10, 64)
	if err != nil || targetID < 1 || strconv.FormatInt(targetID, 10) != terminal.TargetID {
		return groupopsport.HistoricalReceipt{}, groupopsport.ErrHistoryConflict
	}
	return groupopsport.HistoricalReceipt{
		SourceIdentifier: sourceIdentifier,
		PayloadDigest:    terminal.PayloadDigest,
		TargetID:         targetID,
		TargetDigest:     terminal.TargetDigest,
	}, nil
}
