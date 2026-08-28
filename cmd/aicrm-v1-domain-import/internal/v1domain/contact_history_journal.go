package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1contacthistory"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	contactHistoryImportVersion          = "v1-contact-history-a1"
	contactHistorySidebarTargetTable     = "contact_v1_sidebar_profile_history"
	contactHistoryOwnerResultTargetTable = "contact_v1_owner_migration_result_history"
	contactHistoryContextTargetTable     = "contact_v1_owner_migration_context_archive"
	contactHistoryContextArchiveReason   = "owner_migration_context"
)

// ContactHistoryJournal keeps all four source tables in one immutable import
// scope. Only the two formal target types implement the Contact-owned journal;
// sessions and previews can only receive archive terminal receipts.
type ContactHistoryJournal struct {
	sidebar, ownerResults, sessions, previews *Journal
}

var _ contactport.ContactHistoryJournal = (*ContactHistoryJournal)(nil)

func NewContactHistoryJournal(sidebar, ownerResults, sessions, previews *Journal) (*ContactHistoryJournal, error) {
	if !validContactHistoryScope(sidebar, v1contacthistory.SidebarProfileFieldsTableID, contactHistorySidebarTargetTable) ||
		!validContactHistoryScope(ownerResults, v1contacthistory.OwnerMigrationResultsTableID, contactHistoryOwnerResultTargetTable) ||
		!validContactHistoryScope(sessions, v1contacthistory.OwnerMigrationSessionsTableID, contactHistoryContextTargetTable) ||
		!validContactHistoryScope(previews, v1contacthistory.OwnerMigrationPreviewsTableID, contactHistoryContextTargetTable) ||
		!sameContactHistoryScope(sidebar, ownerResults) || !sameContactHistoryScope(sidebar, sessions) || !sameContactHistoryScope(sidebar, previews) {
		return nil, ErrInvalidScope
	}
	return &ContactHistoryJournal{sidebar: sidebar, ownerResults: ownerResults, sessions: sessions, previews: previews}, nil
}

func validContactHistoryScope(journal *Journal, sourceTable, targetTable string) bool {
	return journal != nil && journal.tx != nil && journal.scope.valid() &&
		journal.scope.ImportVersion == contactHistoryImportVersion && journal.scope.AdapterID == v1archive.DefaultAdapterID &&
		journal.scope.TableID == sourceTable && journal.scope.TargetDomain == "contact" && journal.scope.TargetTable == targetTable
}

func sameContactHistoryScope(left, right *Journal) bool {
	return left != nil && right != nil && left.scope.ImportVersion == right.scope.ImportVersion &&
		left.scope.ArchiveRunID == right.scope.ArchiveRunID && left.scope.AdapterID == right.scope.AdapterID
}

func (journal *ContactHistoryJournal) ValidateContactHistoryImportScope(archiveRunID string) error {
	if journal == nil || archiveRunID == "" || !validContactHistoryScope(journal.sidebar, v1contacthistory.SidebarProfileFieldsTableID, contactHistorySidebarTargetTable) ||
		!validContactHistoryScope(journal.ownerResults, v1contacthistory.OwnerMigrationResultsTableID, contactHistoryOwnerResultTargetTable) ||
		!validContactHistoryScope(journal.sessions, v1contacthistory.OwnerMigrationSessionsTableID, contactHistoryContextTargetTable) ||
		!validContactHistoryScope(journal.previews, v1contacthistory.OwnerMigrationPreviewsTableID, contactHistoryContextTargetTable) ||
		!sameContactHistoryScope(journal.sidebar, journal.ownerResults) || !sameContactHistoryScope(journal.sidebar, journal.sessions) ||
		!sameContactHistoryScope(journal.sidebar, journal.previews) || journal.sidebar.scope.ArchiveRunID != archiveRunID {
		return ErrInvalidScope
	}
	return nil
}

func (journal *ContactHistoryJournal) LoadContactHistory(ctx context.Context, kind, sourceIdentifier string) (contactport.ContactHistoryReceipt, bool, error) {
	selected, err := journal.forKind(kind)
	if err != nil || ctx == nil {
		return contactport.ContactHistoryReceipt{}, false, ErrInvalidScope
	}
	terminal, found, err := selected.LoadTerminal(ctx, sourceIdentifier)
	if err != nil || !found {
		return contactport.ContactHistoryReceipt{}, found, err
	}
	receipt, err := contactHistoryReceiptFromTerminal(kind, sourceIdentifier, terminal)
	return receipt, err == nil, err
}

func (journal *ContactHistoryJournal) RecordContactHistory(ctx context.Context, receipt contactport.ContactHistoryReceipt) error {
	selected, err := journal.forKind(receipt.Kind)
	if err != nil || ctx == nil {
		return ErrInvalidScope
	}
	terminal, err := contactHistoryTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return selected.Record(ctx, terminal)
}

func (journal *ContactHistoryJournal) LoadTerminal(ctx context.Context, tableID, sourceIdentifier string) (TerminalReceipt, bool, error) {
	selected, err := journal.forTable(tableID)
	if err != nil || ctx == nil {
		return TerminalReceipt{}, false, ErrInvalidScope
	}
	return selected.LoadTerminal(ctx, sourceIdentifier)
}

func (journal *ContactHistoryJournal) RecordTerminal(ctx context.Context, tableID string, receipt TerminalReceipt) error {
	selected, err := journal.forTable(tableID)
	if err != nil || ctx == nil {
		return ErrInvalidScope
	}
	return selected.Record(ctx, receipt)
}

func (journal *ContactHistoryJournal) forKind(kind string) (*Journal, error) {
	if journal == nil {
		return nil, ErrInvalidScope
	}
	switch kind {
	case contactport.ContactHistorySidebar:
		return journal.sidebar, nil
	case contactport.ContactHistoryOwnerResult:
		return journal.ownerResults, nil
	default:
		return nil, ErrInvalidScope
	}
}

func (journal *ContactHistoryJournal) forTable(tableID string) (*Journal, error) {
	if journal == nil {
		return nil, ErrInvalidScope
	}
	switch tableID {
	case v1contacthistory.SidebarProfileFieldsTableID:
		return journal.sidebar, nil
	case v1contacthistory.OwnerMigrationResultsTableID:
		return journal.ownerResults, nil
	case v1contacthistory.OwnerMigrationSessionsTableID:
		return journal.sessions, nil
	case v1contacthistory.OwnerMigrationPreviewsTableID:
		return journal.previews, nil
	default:
		return nil, ErrInvalidScope
	}
}

func contactHistoryReceiptFromTerminal(kind, sourceIdentifier string, terminal TerminalReceipt) (contactport.ContactHistoryReceipt, error) {
	sourceKey, err := ParseSourceIdentifier(sourceIdentifier)
	targetID, targetErr := positiveID(terminal.TargetID)
	if err != nil || targetErr != nil || sourceKey == ([sha256.Size]byte{}) || sourceKey != terminal.SourceKeyDigest ||
		terminal.PayloadDigest == ([sha256.Size]byte{}) || terminal.Disposition != "import" || terminal.Reason != "" ||
		terminal.TargetDigest == ([sha256.Size]byte{}) || len(terminal.Metadata) != 0 || strconv.FormatInt(targetID, 10) != terminal.TargetID ||
		(kind != contactport.ContactHistorySidebar && kind != contactport.ContactHistoryOwnerResult) {
		return contactport.ContactHistoryReceipt{}, ErrConflict
	}
	return contactport.ContactHistoryReceipt{Kind: kind, SourceIdentifier: sourceIdentifier, PayloadDigest: terminal.PayloadDigest,
		TargetID: targetID, TargetDigest: terminal.TargetDigest}, nil
}

func contactHistoryTerminalFromReceipt(receipt contactport.ContactHistoryReceipt) (TerminalReceipt, error) {
	sourceKey, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || sourceKey == ([sha256.Size]byte{}) || receipt.PayloadDigest == ([sha256.Size]byte{}) ||
		receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.Replayed ||
		(receipt.Kind != contactport.ContactHistorySidebar && receipt.Kind != contactport.ContactHistoryOwnerResult) {
		return TerminalReceipt{}, ErrInvalidScope
	}
	return TerminalReceipt{SourceKeyDigest: sourceKey, PayloadDigest: receipt.PayloadDigest, Disposition: "import",
		TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest}, nil
}
