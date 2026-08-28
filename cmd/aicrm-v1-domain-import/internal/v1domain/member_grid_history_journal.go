package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1membergridhistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

const (
	memberGridHistoryImportVersion      = "v1-member-grid-history-a1"
	memberGridHistoryViewTargetTable    = "product_v1_member_view_history"
	memberGridHistoryUsageTargetTable   = "product_v1_member_usage_history"
	memberGridHistoryContextTargetTable = "product_v1_member_grid_context_archive"
)

// MemberGridHistoryJournal keeps the five frozen source tables in one import
// scope. Only views and usage snapshots can obtain Product-owned receipts.
type MemberGridHistoryJournal struct {
	views, usage, syncRuns, collaborators, shares *Journal
}

var _ productport.MemberGridHistoryJournal = (*MemberGridHistoryJournal)(nil)

func NewMemberGridHistoryJournal(views, usage, syncRuns, collaborators, shares *Journal) (*MemberGridHistoryJournal, error) {
	journal := &MemberGridHistoryJournal{views: views, usage: usage, syncRuns: syncRuns, collaborators: collaborators, shares: shares}
	if !journal.valid() {
		return nil, ErrInvalidScope
	}
	return journal, nil
}

func (journal *MemberGridHistoryJournal) ValidateMemberGridHistoryImportScope(archiveRunID string) error {
	if journal == nil || archiveRunID == "" || !journal.valid() || journal.views.scope.ArchiveRunID != archiveRunID {
		return ErrInvalidScope
	}
	return nil
}

func (journal *MemberGridHistoryJournal) LoadMemberGridHistory(ctx context.Context, kind, sourceIdentifier string) (productport.MemberGridHistoryReceipt, bool, error) {
	selected, err := journal.forKind(kind)
	if err != nil || ctx == nil {
		return productport.MemberGridHistoryReceipt{}, false, ErrInvalidScope
	}
	terminal, found, err := selected.LoadTerminal(ctx, sourceIdentifier)
	if err != nil || !found {
		return productport.MemberGridHistoryReceipt{}, found, err
	}
	receipt, err := memberGridHistoryReceiptFromTerminal(kind, sourceIdentifier, terminal)
	return receipt, err == nil, err
}

func (journal *MemberGridHistoryJournal) RecordMemberGridHistory(ctx context.Context, receipt productport.MemberGridHistoryReceipt) error {
	selected, err := journal.forKind(receipt.Kind)
	if err != nil || ctx == nil {
		return ErrInvalidScope
	}
	terminal, err := memberGridHistoryTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return selected.Record(ctx, terminal)
}

func (journal *MemberGridHistoryJournal) LoadTerminal(ctx context.Context, tableID, sourceIdentifier string) (TerminalReceipt, bool, error) {
	selected, err := journal.forTable(tableID)
	if err != nil || ctx == nil {
		return TerminalReceipt{}, false, ErrInvalidScope
	}
	return selected.LoadTerminal(ctx, sourceIdentifier)
}

func (journal *MemberGridHistoryJournal) RecordTerminal(ctx context.Context, tableID string, receipt TerminalReceipt) error {
	selected, err := journal.forTable(tableID)
	if err != nil || ctx == nil {
		return ErrInvalidScope
	}
	return selected.Record(ctx, receipt)
}

func (journal *MemberGridHistoryJournal) forKind(kind string) (*Journal, error) {
	if journal == nil {
		return nil, ErrInvalidScope
	}
	switch kind {
	case productport.MemberGridHistoryView:
		return journal.views, nil
	case productport.MemberGridHistoryUsage:
		return journal.usage, nil
	default:
		return nil, ErrInvalidScope
	}
}

func (journal *MemberGridHistoryJournal) forTable(tableID string) (*Journal, error) {
	if journal == nil {
		return nil, ErrInvalidScope
	}
	switch tableID {
	case v1membergridhistory.MemberViewsTableID:
		return journal.views, nil
	case v1membergridhistory.UsageSnapshotsTableID:
		return journal.usage, nil
	case v1membergridhistory.UsageSyncRunsTableID:
		return journal.syncRuns, nil
	case v1membergridhistory.MemberCollaboratorsTableID:
		return journal.collaborators, nil
	case v1membergridhistory.MemberSharesTableID:
		return journal.shares, nil
	default:
		return nil, ErrInvalidScope
	}
}

func (journal *MemberGridHistoryJournal) valid() bool {
	if journal == nil || !validMemberGridHistoryScope(journal.views, v1membergridhistory.MemberViewsTableID, memberGridHistoryViewTargetTable) ||
		!validMemberGridHistoryScope(journal.usage, v1membergridhistory.UsageSnapshotsTableID, memberGridHistoryUsageTargetTable) ||
		!validMemberGridHistoryScope(journal.syncRuns, v1membergridhistory.UsageSyncRunsTableID, memberGridHistoryContextTargetTable) ||
		!validMemberGridHistoryScope(journal.collaborators, v1membergridhistory.MemberCollaboratorsTableID, memberGridHistoryContextTargetTable) ||
		!validMemberGridHistoryScope(journal.shares, v1membergridhistory.MemberSharesTableID, memberGridHistoryContextTargetTable) {
		return false
	}
	return sameMemberGridHistoryScope(journal.views, journal.usage) && sameMemberGridHistoryScope(journal.views, journal.syncRuns) &&
		sameMemberGridHistoryScope(journal.views, journal.collaborators) && sameMemberGridHistoryScope(journal.views, journal.shares)
}

func validMemberGridHistoryScope(journal *Journal, tableID, targetTable string) bool {
	return journal != nil && journal.tx != nil && journal.scope.valid() && journal.scope.ImportVersion == memberGridHistoryImportVersion &&
		journal.scope.AdapterID == v1archive.DefaultAdapterID && journal.scope.TableID == tableID && journal.scope.TargetDomain == "product" && journal.scope.TargetTable == targetTable
}

func sameMemberGridHistoryScope(left, right *Journal) bool {
	return left != nil && right != nil && left.scope.ImportVersion == right.scope.ImportVersion &&
		left.scope.ArchiveRunID == right.scope.ArchiveRunID && left.scope.AdapterID == right.scope.AdapterID
}

func memberGridHistoryReceiptFromTerminal(kind, sourceIdentifier string, terminal TerminalReceipt) (productport.MemberGridHistoryReceipt, error) {
	sourceKey, err := ParseSourceIdentifier(sourceIdentifier)
	targetID, idErr := positiveID(terminal.TargetID)
	if err != nil || idErr != nil || sourceKey == ([sha256.Size]byte{}) || sourceKey != terminal.SourceKeyDigest ||
		terminal.PayloadDigest == ([sha256.Size]byte{}) || terminal.Disposition != "import" || terminal.Reason != "" ||
		terminal.TargetDigest == ([sha256.Size]byte{}) || len(terminal.Metadata) != 0 || strconv.FormatInt(targetID, 10) != terminal.TargetID ||
		(kind != productport.MemberGridHistoryView && kind != productport.MemberGridHistoryUsage) {
		return productport.MemberGridHistoryReceipt{}, ErrConflict
	}
	return productport.MemberGridHistoryReceipt{Kind: kind, SourceIdentifier: sourceIdentifier, PayloadDigest: terminal.PayloadDigest,
		TargetID: targetID, TargetDigest: terminal.TargetDigest}, nil
}

func memberGridHistoryTerminalFromReceipt(receipt productport.MemberGridHistoryReceipt) (TerminalReceipt, error) {
	sourceKey, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || sourceKey == ([sha256.Size]byte{}) || receipt.PayloadDigest == ([sha256.Size]byte{}) || receipt.TargetID < 1 ||
		receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.Replayed ||
		(receipt.Kind != productport.MemberGridHistoryView && receipt.Kind != productport.MemberGridHistoryUsage) {
		return TerminalReceipt{}, ErrInvalidScope
	}
	return TerminalReceipt{SourceKeyDigest: sourceKey, PayloadDigest: receipt.PayloadDigest, Disposition: "import",
		TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest}, nil
}
