package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1contacthistory"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

// ContactHistoryWriter owns the read-only Contact target rows. Each method
// must write and verify its ContactHistoryReceipt in the caller transaction.
type ContactHistoryWriter interface {
	WriteSidebarProfile(context.Context, string, [sha256.Size]byte, contactport.HistoricalSidebarProfile) (contactport.ContactHistoryReceipt, error)
	WriteOwnerMigrationResult(context.Context, string, [sha256.Size]byte, contactport.HistoricalOwnerMigrationResult) (contactport.ContactHistoryReceipt, error)
}

// ContactHistoryCustomerResolver returns only a previously proven DM01 root.
// A nil result is an intentionally unresolved historical customer relation.
type ContactHistoryCustomerResolver interface {
	ResolveHistoricalContactCustomer(context.Context, string) (*int64, error)
}

type contactHistoryImportJournal interface {
	contactport.ContactHistoryJournal
	LoadTerminal(context.Context, string, string) (TerminalReceipt, bool, error)
	RecordTerminal(context.Context, string, TerminalReceipt) error
	ValidateContactHistoryImportScope(string) error
}

type ContactHistoryImportResult struct {
	Imported, Archived, Quarantined, Replayed int
}

// ContactHistoryImporter imports only static Contact history. Sessions and
// previews are sealed as archive-only context; it never invokes owner-move,
// staff, token, Provider, event, or queue behaviour.
type ContactHistoryImporter struct {
	archive  ArchiveSource
	uow      UnitOfWork
	writer   ContactHistoryWriter
	resolver ContactHistoryCustomerResolver
	journal  contactHistoryImportJournal
}

func NewContactHistoryImporter(archive ArchiveSource, uow UnitOfWork, writer ContactHistoryWriter, resolver ContactHistoryCustomerResolver, journal contactHistoryImportJournal) (*ContactHistoryImporter, error) {
	if archive == nil || uow == nil || writer == nil || resolver == nil || journal == nil {
		return nil, ErrInvalidScope
	}
	return &ContactHistoryImporter{archive: archive, uow: uow, writer: writer, resolver: resolver, journal: journal}, nil
}

func (importer *ContactHistoryImporter) Import(ctx context.Context, archiveRunID string) (ContactHistoryImportResult, error) {
	if importer == nil || ctx == nil || importer.archive == nil || importer.uow == nil || importer.writer == nil || importer.resolver == nil || importer.journal == nil {
		return ContactHistoryImportResult{}, ErrInvalidScope
	}
	if err := importer.journal.ValidateContactHistoryImportScope(archiveRunID); err != nil {
		return ContactHistoryImportResult{}, err
	}
	sidebarRows, err := importer.readRows(ctx, archiveRunID, v1contacthistory.SidebarProfileFieldsTableID)
	if err != nil {
		return ContactHistoryImportResult{}, err
	}
	sessionRows, err := importer.readRows(ctx, archiveRunID, v1contacthistory.OwnerMigrationSessionsTableID)
	if err != nil {
		return ContactHistoryImportResult{}, err
	}
	previewRows, err := importer.readRows(ctx, archiveRunID, v1contacthistory.OwnerMigrationPreviewsTableID)
	if err != nil {
		return ContactHistoryImportResult{}, err
	}
	resultRows, err := importer.readRows(ctx, archiveRunID, v1contacthistory.OwnerMigrationResultsTableID)
	if err != nil {
		return ContactHistoryImportResult{}, err
	}
	ownerContext, err := v1contacthistory.BuildOwnerMigrationContext(sessionRows, previewRows)
	if err != nil {
		return ContactHistoryImportResult{}, err
	}

	result := ContactHistoryImportResult{}
	for _, row := range sidebarRows {
		if err := importer.importSidebar(ctx, row, &result); err != nil {
			return ContactHistoryImportResult{}, err
		}
	}
	for _, row := range sessionRows {
		if err := importer.archiveContext(ctx, row, &result); err != nil {
			return ContactHistoryImportResult{}, err
		}
	}
	for _, row := range previewRows {
		if err := importer.archiveContext(ctx, row, &result); err != nil {
			return ContactHistoryImportResult{}, err
		}
	}
	for _, row := range resultRows {
		if err := importer.importOwnerResult(ctx, row, ownerContext, &result); err != nil {
			return ContactHistoryImportResult{}, err
		}
	}
	if result.Imported+result.Archived+result.Quarantined != len(sidebarRows)+len(sessionRows)+len(previewRows)+len(resultRows) {
		return ContactHistoryImportResult{}, ErrConflict
	}
	return result, nil
}

func (importer *ContactHistoryImporter) readRows(ctx context.Context, archiveRunID, tableID string) ([]v1archive.ArchivedRow, error) {
	rows := make([]v1archive.ArchivedRow, 0)
	seen := map[[sha256.Size]byte]struct{}{}
	expectedOrdinal := int64(1)
	err := importer.archive.EachTableRow(ctx, archiveRunID, tableID, func(row v1archive.ArchivedRow) error {
		if !validContactHistoryArchiveRow(row, tableID, expectedOrdinal) {
			return ErrConflict
		}
		expectedOrdinal++
		if _, duplicate := seen[row.SourceKeyHMAC]; duplicate {
			return ErrConflict
		}
		seen[row.SourceKeyHMAC] = struct{}{}
		rows = append(rows, row)
		return nil
	})
	return rows, err
}

func validContactHistoryArchiveRow(row v1archive.ArchivedRow, tableID string, ordinal int64) bool {
	return row.AdapterID == v1archive.DefaultAdapterID && row.TableID == tableID && row.SourceOrdinal == ordinal &&
		row.SourceKeyHMAC != ([sha256.Size]byte{}) && row.PayloadHMAC != ([sha256.Size]byte{}) && row.FieldHMAC != ([sha256.Size]byte{})
}

func (importer *ContactHistoryImporter) importSidebar(ctx context.Context, row v1archive.ArchivedRow, result *ContactHistoryImportResult) error {
	decision := v1contacthistory.AdaptSidebarProfile(row)
	if decision.Disposition != v1contacthistory.DispositionCandidate || decision.Candidate == nil {
		return importer.quarantine(ctx, row, fixedContactHistoryReason(decision.Reason, "invalid_sidebar_history"), result)
	}
	replayed, imported := false, false
	if err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed, imported = false, false
		value, err := importer.sidebarValue(tx, row, *decision.Candidate)
		if err != nil {
			return err
		}
		receipt, err := importer.writer.WriteSidebarProfile(tx, SourceIdentifier(row.SourceKeyHMAC), row.PayloadHMAC, value)
		if errors.Is(err, contactport.ErrContactHistoryInvalid) {
			var terminalErr error
			replayed, terminalErr = recordContactHistoryTerminal(tx, importer.journal, row, "quarantine", "sidebar_target_invalid")
			return terminalErr
		}
		if err != nil {
			return err
		}
		if err = verifyContactHistoryReceipt(tx, importer.journal, row, contactport.ContactHistorySidebar, receipt); err != nil {
			return err
		}
		replayed, imported = receipt.Replayed, true
		return nil
	}); err != nil {
		return err
	}
	if imported {
		result.Imported++
	} else {
		result.Quarantined++
	}
	if replayed {
		result.Replayed++
	}
	return nil
}

func (importer *ContactHistoryImporter) sidebarValue(ctx context.Context, row v1archive.ArchivedRow, source v1contacthistory.SidebarProfileHistory) (contactport.HistoricalSidebarProfile, error) {
	customerID, err := importer.resolver.ResolveHistoricalContactCustomer(ctx, source.UnionID)
	if err != nil {
		return contactport.HistoricalSidebarProfile{}, err
	}
	if customerID != nil && *customerID < 1 {
		return contactport.HistoricalSidebarProfile{}, ErrConflict
	}
	return contactport.HistoricalSidebarProfile{SourceKeyDigest: row.SourceKeyHMAC, CustomerID: copyContactHistoryID(customerID),
		Source: source.Source, Industry: source.Industry, IndustryDescription: source.IndustryDescription,
		NeedsBlockersFollowup: source.NeedsBlockersFollowup, UpdatedAt: contactHistoryTime(source.UpdatedAt),
		SourcePayloadDigest: row.PayloadHMAC}, nil
}

func (importer *ContactHistoryImporter) archiveContext(ctx context.Context, row v1archive.ArchivedRow, result *ContactHistoryImportResult) error {
	replayed := false
	if err := importer.uow.Within(ctx, func(tx context.Context) error {
		var err error
		replayed, err = recordContactHistoryTerminal(tx, importer.journal, row, "archive", contactHistoryContextArchiveReason)
		return err
	}); err != nil {
		return err
	}
	result.Archived++
	if replayed {
		result.Replayed++
	}
	return nil
}

func (importer *ContactHistoryImporter) importOwnerResult(ctx context.Context, row v1archive.ArchivedRow, ownerContext v1contacthistory.OwnerMigrationContext, result *ContactHistoryImportResult) error {
	decision := v1contacthistory.AdaptOwnerMigrationResult(row)
	if decision.Disposition != v1contacthistory.DispositionCandidate || decision.Candidate == nil {
		return importer.quarantine(ctx, row, fixedContactHistoryReason(decision.Reason, "invalid_owner_migration_result"), result)
	}
	relations, reason := ownerContext.SessionRelation(*decision.Candidate)
	if reason != "" {
		return importer.quarantine(ctx, row, reason, result)
	}
	replayed, imported := false, false
	if err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed, imported = false, false
		value := ownerResultValue(row, *decision.Candidate, relations)
		receipt, err := importer.writer.WriteOwnerMigrationResult(tx, SourceIdentifier(row.SourceKeyHMAC), row.PayloadHMAC, value)
		if errors.Is(err, contactport.ErrContactHistoryInvalid) {
			var terminalErr error
			replayed, terminalErr = recordContactHistoryTerminal(tx, importer.journal, row, "quarantine", "owner_result_target_invalid")
			return terminalErr
		}
		if err != nil {
			return err
		}
		if err = verifyContactHistoryReceipt(tx, importer.journal, row, contactport.ContactHistoryOwnerResult, receipt); err != nil {
			return err
		}
		replayed, imported = receipt.Replayed, true
		return nil
	}); err != nil {
		return err
	}
	if imported {
		result.Imported++
	} else {
		result.Quarantined++
	}
	if replayed {
		result.Replayed++
	}
	return nil
}

func ownerResultValue(row v1archive.ArchivedRow, source v1contacthistory.OwnerMigrationResultHistory, relations v1contacthistory.OwnerMigrationRelations) contactport.HistoricalOwnerMigrationResult {
	return contactport.HistoricalOwnerMigrationResult{SourceKeyDigest: row.SourceKeyHMAC, ScopeType: source.ScopeType,
		FileHash: source.FileHash, PreviewHash: source.PreviewHash, TotalRows: int64(source.TotalRows), EligibleCount: int64(source.EligibleCount),
		WeComSuccess: int64(source.WeComSuccess), WeComFailed: int64(source.WeComFailed), CRMUpdated: int64(source.CRMUpdated),
		IncludeWeComTransfer: source.IncludeWeComTransfer, TransferWelcomeMessage: source.TransferWelcomeMessage,
		SessionRelation: relations.SessionRelation, PreviewRelation: relations.PreviewRelation, CreatedAt: contactHistoryTime(source.CreatedAt),
		ExecutedAt: contactHistoryTime(source.ExecutedAt), SourcePayloadDigest: row.PayloadHMAC}
}

func (importer *ContactHistoryImporter) quarantine(ctx context.Context, row v1archive.ArchivedRow, reason string, result *ContactHistoryImportResult) error {
	replayed := false
	if err := importer.uow.Within(ctx, func(tx context.Context) error {
		var err error
		replayed, err = recordContactHistoryTerminal(tx, importer.journal, row, "quarantine", reason)
		return err
	}); err != nil {
		return err
	}
	result.Quarantined++
	if replayed {
		result.Replayed++
	}
	return nil
}

func recordContactHistoryTerminal(ctx context.Context, journal contactHistoryImportJournal, row v1archive.ArchivedRow, disposition, reason string) (bool, error) {
	if journal == nil || reason == "" || (disposition != "archive" && disposition != "quarantine") {
		return false, ErrInvalidScope
	}
	sourceIdentifier := SourceIdentifier(row.SourceKeyHMAC)
	existing, found, err := journal.LoadTerminal(ctx, row.TableID, sourceIdentifier)
	if err != nil {
		return false, err
	}
	if found {
		if existing.SourceKeyDigest != row.SourceKeyHMAC || existing.PayloadDigest != row.PayloadHMAC || existing.Disposition != disposition ||
			existing.Reason != reason || existing.TargetID != "" || existing.TargetDigest != ([sha256.Size]byte{}) || len(existing.Metadata) != 0 {
			return false, ErrConflict
		}
		return true, nil
	}
	return false, journal.RecordTerminal(ctx, row.TableID, TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC,
		Disposition: disposition, Reason: reason})
}

func verifyContactHistoryReceipt(ctx context.Context, journal contactHistoryImportJournal, row v1archive.ArchivedRow, kind string, receipt contactport.ContactHistoryReceipt) error {
	if journal == nil || receipt.Kind != kind || receipt.SourceIdentifier != SourceIdentifier(row.SourceKeyHMAC) || receipt.PayloadDigest != row.PayloadHMAC ||
		receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	recorded, found, err := journal.LoadContactHistory(ctx, kind, receipt.SourceIdentifier)
	if err != nil || !found || recorded.Kind != kind || recorded.SourceIdentifier != receipt.SourceIdentifier || recorded.PayloadDigest != receipt.PayloadDigest ||
		recorded.TargetID != receipt.TargetID || recorded.TargetDigest != receipt.TargetDigest {
		return ErrConflict
	}
	return nil
}

func fixedContactHistoryReason(reason, fallback string) string {
	if reason == "" {
		return fallback
	}
	return reason
}

func copyContactHistoryID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func contactHistoryTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}
