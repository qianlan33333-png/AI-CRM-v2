package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1membergridhistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

var memberGridHistoryTables = [...]string{
	v1membergridhistory.MemberViewsTableID,
	v1membergridhistory.UsageSnapshotsTableID,
	v1membergridhistory.UsageSyncRunsTableID,
	v1membergridhistory.MemberCollaboratorsTableID,
	v1membergridhistory.MemberSharesTableID,
}

// MemberGridHistoryWriter owns the two Product history targets. It must share
// the caller transaction with the migration journal and create no current Grid
// fact, entitlement, event, sharing grant, or external effect.
type MemberGridHistoryWriter interface {
	WriteMemberView(context.Context, string, [sha256.Size]byte, productport.HistoricalMemberView) (productport.MemberGridHistoryReceipt, error)
	WriteMemberUsage(context.Context, string, [sha256.Size]byte, productport.HistoricalMemberUsage) (productport.MemberGridHistoryReceipt, error)
}

type MemberGridHistoryReferenceResolver interface {
	ResolveHistoricalMemberGridCustomer(context.Context, string) (*int64, error)
	ResolveHistoricalMemberGridProduct(context.Context, int64) (*int64, error)
}

type memberGridHistoryImportJournal interface {
	productport.MemberGridHistoryJournal
	LoadTerminal(context.Context, string, string) (TerminalReceipt, bool, error)
	RecordTerminal(context.Context, string, TerminalReceipt) error
	ValidateMemberGridHistoryImportScope(string) error
}

type MemberGridHistoryImportResult struct {
	ImportedViews, ImportedUsage int
	Archived, Quarantined        int
	Replayed                     int
}

// MemberGridHistoryImporter applies a recovered bool only in memory after its
// frozen archive HMACs authenticate it. The sealed archive is never rewritten.
type MemberGridHistoryImporter struct {
	archive              ArchiveSource
	uow                  UnitOfWork
	writer               MemberGridHistoryWriter
	resolver             MemberGridHistoryReferenceResolver
	recoveryEntries      []v1membergridhistory.UsageSnapshotRecoveryEntry
	archiveSourceHMACKey []byte
	journal              memberGridHistoryImportJournal
}

func NewMemberGridHistoryImporter(archive ArchiveSource, uow UnitOfWork, writer MemberGridHistoryWriter, resolver MemberGridHistoryReferenceResolver, recoveryEntries []v1membergridhistory.UsageSnapshotRecoveryEntry, archiveSourceHMACKey []byte, journal memberGridHistoryImportJournal) (*MemberGridHistoryImporter, error) {
	if archive == nil || uow == nil || writer == nil || resolver == nil || journal == nil || len(archiveSourceHMACKey) == 0 || len(recoveryEntries) == 0 {
		return nil, ErrInvalidScope
	}
	return &MemberGridHistoryImporter{archive: archive, uow: uow, writer: writer, resolver: resolver,
		recoveryEntries:      append([]v1membergridhistory.UsageSnapshotRecoveryEntry(nil), recoveryEntries...),
		archiveSourceHMACKey: append([]byte(nil), archiveSourceHMACKey...), journal: journal}, nil
}

func (importer *MemberGridHistoryImporter) Import(ctx context.Context, archiveRunID string) (MemberGridHistoryImportResult, error) {
	if importer == nil || ctx == nil || importer.archive == nil || importer.uow == nil || importer.writer == nil || importer.resolver == nil || importer.journal == nil {
		return MemberGridHistoryImportResult{}, ErrInvalidScope
	}
	if err := importer.journal.ValidateMemberGridHistoryImportScope(archiveRunID); err != nil {
		return MemberGridHistoryImportResult{}, err
	}
	views, err := importer.readRows(ctx, archiveRunID, v1membergridhistory.MemberViewsTableID)
	if err != nil {
		return MemberGridHistoryImportResult{}, err
	}
	usage, err := importer.readRows(ctx, archiveRunID, v1membergridhistory.UsageSnapshotsTableID)
	if err != nil {
		return MemberGridHistoryImportResult{}, err
	}
	if err = importer.validateRecoveryCoverage(archiveRunID, usage); err != nil {
		return MemberGridHistoryImportResult{}, err
	}
	contexts := make(map[string][]v1archive.ArchivedRow, 3)
	for _, tableID := range memberGridHistoryTables[2:] {
		contexts[tableID], err = importer.readRows(ctx, archiveRunID, tableID)
		if err != nil {
			return MemberGridHistoryImportResult{}, err
		}
	}

	result := MemberGridHistoryImportResult{}
	for _, row := range views {
		if err = importer.importView(ctx, row, &result); err != nil {
			return MemberGridHistoryImportResult{}, err
		}
	}
	entries := importer.recoveryBySource()
	for _, row := range usage {
		if err = importer.importUsage(ctx, row, entries[row.SourceKeyHMAC], &result); err != nil {
			return MemberGridHistoryImportResult{}, err
		}
	}
	for _, tableID := range memberGridHistoryTables[2:] {
		for _, row := range contexts[tableID] {
			if err = importer.archiveContext(ctx, row, &result); err != nil {
				return MemberGridHistoryImportResult{}, err
			}
		}
	}
	return result, nil
}

func (importer *MemberGridHistoryImporter) readRows(ctx context.Context, archiveRunID, tableID string) ([]v1archive.ArchivedRow, error) {
	rows := make([]v1archive.ArchivedRow, 0)
	seen := map[[sha256.Size]byte]struct{}{}
	expectedOrdinal := int64(1)
	err := importer.archive.EachTableRow(ctx, archiveRunID, tableID, func(row v1archive.ArchivedRow) error {
		if !validMemberGridHistoryRow(row, tableID, expectedOrdinal) {
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

func (importer *MemberGridHistoryImporter) validateRecoveryCoverage(archiveRunID string, usage []v1archive.ArchivedRow) error {
	if archiveRunID != v1membergridhistory.FixedUsageSnapshotRecoveryScope().ArchiveRunID || len(importer.archiveSourceHMACKey) == 0 || len(usage) != len(importer.recoveryEntries) {
		return ErrInvalidScope
	}
	entries := importer.recoveryBySource()
	if len(entries) != len(importer.recoveryEntries) {
		return ErrInvalidScope
	}
	for _, entry := range importer.recoveryEntries {
		if entry.Scope != v1membergridhistory.FixedUsageSnapshotRecoveryScope() || entry.SourceKeyHMAC == ([sha256.Size]byte{}) ||
			entry.OriginalPayloadHMAC == ([sha256.Size]byte{}) || entry.OriginalFieldHMAC == ([sha256.Size]byte{}) || entry.EntryHMAC == ([sha256.Size]byte{}) {
			return ErrInvalidScope
		}
	}
	for _, row := range usage {
		entry, found := entries[row.SourceKeyHMAC]
		if !found {
			return ErrInvalidScope
		}
		if _, err := v1membergridhistory.AdaptUsageSnapshotRecovery(row, entry, importer.archiveSourceHMACKey); err != nil {
			return ErrInvalidScope
		}
	}
	return nil
}

func (importer *MemberGridHistoryImporter) recoveryBySource() map[[sha256.Size]byte]v1membergridhistory.UsageSnapshotRecoveryEntry {
	entries := make(map[[sha256.Size]byte]v1membergridhistory.UsageSnapshotRecoveryEntry, len(importer.recoveryEntries))
	for _, entry := range importer.recoveryEntries {
		entries[entry.SourceKeyHMAC] = entry
	}
	return entries
}

func (importer *MemberGridHistoryImporter) importView(ctx context.Context, row v1archive.ArchivedRow, result *MemberGridHistoryImportResult) error {
	decision := v1membergridhistory.AdaptMemberView(row)
	if decision.Disposition != v1membergridhistory.DispositionCandidate || decision.Record == nil {
		return importer.quarantine(ctx, row, memberGridHistoryReason(decision.Reason, "invalid_member_view"), result)
	}
	replayed, imported := false, false
	if err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed, imported = false, false
		productID, err := importer.resolveProduct(tx, decision.Record.ServiceProductID)
		if err != nil {
			return err
		}
		value := memberGridHistoryViewValue(row, *decision.Record, productID)
		receipt, err := importer.writer.WriteMemberView(tx, SourceIdentifier(row.SourceKeyHMAC), row.PayloadHMAC, value)
		if errors.Is(err, productport.ErrMemberGridHistoryInvalid) {
			var terminalErr error
			replayed, terminalErr = recordMemberGridHistoryTerminal(tx, importer.journal, row, "quarantine", "member_view_target_invalid")
			return terminalErr
		}
		if err != nil {
			return err
		}
		if err = verifyMemberGridHistoryReceipt(tx, importer.journal, row, productport.MemberGridHistoryView, receipt); err != nil {
			return err
		}
		replayed, imported = receipt.Replayed, true
		return nil
	}); err != nil {
		return err
	}
	if imported {
		result.ImportedViews++
	} else {
		result.Quarantined++
	}
	if replayed {
		result.Replayed++
	}
	return nil
}

func (importer *MemberGridHistoryImporter) importUsage(ctx context.Context, row v1archive.ArchivedRow, entry v1membergridhistory.UsageSnapshotRecoveryEntry, result *MemberGridHistoryImportResult) error {
	decision, err := v1membergridhistory.AdaptUsageSnapshotRecovery(row, entry, importer.archiveSourceHMACKey)
	if err != nil {
		return ErrConflict
	}
	if decision.Disposition != v1membergridhistory.DispositionCandidate || decision.Record == nil {
		return importer.quarantine(ctx, row, memberGridHistoryReason(decision.Reason, "invalid_member_usage"), result)
	}
	replayed, imported := false, false
	if err = importer.uow.Within(ctx, func(tx context.Context) error {
		replayed, imported = false, false
		customerID, resolveErr := importer.resolveCustomer(tx, decision.Record.UnionID)
		if resolveErr != nil {
			return resolveErr
		}
		recoveryDigest, digestErr := memberGridRecoveryEntryDigest(entry)
		if digestErr != nil {
			return ErrConflict
		}
		value := memberGridHistoryUsageValue(row, *decision.Record, customerID, recoveryDigest)
		receipt, writeErr := importer.writer.WriteMemberUsage(tx, SourceIdentifier(row.SourceKeyHMAC), row.PayloadHMAC, value)
		if errors.Is(writeErr, productport.ErrMemberGridHistoryInvalid) {
			var terminalErr error
			replayed, terminalErr = recordMemberGridHistoryTerminal(tx, importer.journal, row, "quarantine", "member_usage_target_invalid")
			return terminalErr
		}
		if writeErr != nil {
			return writeErr
		}
		if verifyErr := verifyMemberGridHistoryReceipt(tx, importer.journal, row, productport.MemberGridHistoryUsage, receipt); verifyErr != nil {
			return verifyErr
		}
		replayed, imported = receipt.Replayed, true
		return nil
	}); err != nil {
		return err
	}
	if imported {
		result.ImportedUsage++
	} else {
		result.Quarantined++
	}
	if replayed {
		result.Replayed++
	}
	return nil
}

func (importer *MemberGridHistoryImporter) archiveContext(ctx context.Context, row v1archive.ArchivedRow, result *MemberGridHistoryImportResult) error {
	disposition, reason := memberGridHistoryContextDisposition(row)
	replayed := false
	if err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		var err error
		replayed, err = recordMemberGridHistoryTerminal(tx, importer.journal, row, disposition, reason)
		return err
	}); err != nil {
		return err
	}
	if disposition == "archive" {
		result.Archived++
	} else {
		result.Quarantined++
	}
	if replayed {
		result.Replayed++
	}
	return nil
}

func (importer *MemberGridHistoryImporter) quarantine(ctx context.Context, row v1archive.ArchivedRow, reason string, result *MemberGridHistoryImportResult) error {
	replayed := false
	if err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		var err error
		replayed, err = recordMemberGridHistoryTerminal(tx, importer.journal, row, "quarantine", reason)
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

func (importer *MemberGridHistoryImporter) resolveProduct(ctx context.Context, sourceID int64) (*int64, error) {
	value, err := importer.resolver.ResolveHistoricalMemberGridProduct(ctx, sourceID)
	if err != nil || value == nil {
		return value, err
	}
	if *value < 1 {
		return nil, ErrConflict
	}
	copy := *value
	return &copy, nil
}

func (importer *MemberGridHistoryImporter) resolveCustomer(ctx context.Context, unionID string) (*int64, error) {
	if unionID == "" {
		return nil, nil
	}
	value, err := importer.resolver.ResolveHistoricalMemberGridCustomer(ctx, unionID)
	if err != nil || value == nil {
		return value, err
	}
	if *value < 1 {
		return nil, ErrConflict
	}
	copy := *value
	return &copy, nil
}

func memberGridHistoryViewValue(row v1archive.ArchivedRow, source v1membergridhistory.MemberViewHistory, productID *int64) productport.HistoricalMemberView {
	return productport.HistoricalMemberView{SourceKeyDigest: row.SourceKeyHMAC, SourceViewID: source.ID, SourceServiceProductID: source.ServiceProductID,
		ProductID: copyMemberGridHistoryID(productID), Name: source.Name, Position: int64(source.Position), IsDefault: source.IsDefault,
		SchemaVersion: source.SchemaVersion, ConfigDigest: sha256.Sum256(source.ConfigJSON), Version: int64(source.Version),
		CreatedAt: memberGridHistoryTime(source.CreatedAt), UpdatedAt: memberGridHistoryTime(source.UpdatedAt), SourcePayloadDigest: row.PayloadHMAC}
}

func memberGridHistoryUsageValue(row v1archive.ArchivedRow, source v1membergridhistory.UsageSnapshotHistory, customerID *int64, recoveryEntryDigest [sha256.Size]byte) productport.HistoricalMemberUsage {
	return productport.HistoricalMemberUsage{SourceKeyDigest: row.SourceKeyHMAC, CustomerID: copyMemberGridHistoryID(customerID), FormallyLoggedIn: source.FormallyLoggedIn,
		HasTokenUsage: source.HasTokenUsage, LearningPlanID: source.LearningPlanID, LearningPlanCurrent: memberGridHistoryInt64Pointer(source.LearningPlanCurrent),
		LearningPlanTotal: memberGridHistoryInt64Pointer(source.LearningPlanTotal), OpenCount7D: int64(source.OpenCount7D), LastOpenAt: memberGridHistoryOptionalTime(source.LastOpenAt),
		RefreshedAt: memberGridHistoryTime(source.RefreshedAt), SourcePayloadDigest: row.PayloadHMAC, RecoveryEntryDigest: recoveryEntryDigest}
}

func memberGridHistoryContextDisposition(row v1archive.ArchivedRow) (string, string) {
	switch row.TableID {
	case v1membergridhistory.UsageSyncRunsTableID:
		decision := v1membergridhistory.AdaptUsageSyncRun(row)
		if decision.Disposition == v1membergridhistory.DispositionArchive {
			return "archive", memberGridHistoryReason(decision.Reason, "usage_sync_archive")
		}
	case v1membergridhistory.MemberCollaboratorsTableID:
		decision := v1membergridhistory.AdaptMemberCollaborator(row)
		if decision.Disposition == v1membergridhistory.DispositionArchive {
			return "archive", memberGridHistoryReason(decision.Reason, "member_collaborator_archive")
		}
	case v1membergridhistory.MemberSharesTableID:
		decision := v1membergridhistory.AdaptMemberShare(row)
		if decision.Disposition == v1membergridhistory.DispositionArchive {
			return "archive", memberGridHistoryReason(decision.Reason, "member_share_archive")
		}
	}
	return "quarantine", "invalid_member_grid_context"
}

func recordMemberGridHistoryTerminal(ctx context.Context, journal memberGridHistoryImportJournal, row v1archive.ArchivedRow, disposition, reason string) (bool, error) {
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

func verifyMemberGridHistoryReceipt(ctx context.Context, journal memberGridHistoryImportJournal, row v1archive.ArchivedRow, kind string, receipt productport.MemberGridHistoryReceipt) error {
	if journal == nil || receipt.Kind != kind || receipt.SourceIdentifier != SourceIdentifier(row.SourceKeyHMAC) || receipt.PayloadDigest != row.PayloadHMAC ||
		receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	recorded, found, err := journal.LoadMemberGridHistory(ctx, kind, receipt.SourceIdentifier)
	if err != nil || !found || recorded.Kind != kind || recorded.SourceIdentifier != receipt.SourceIdentifier || recorded.PayloadDigest != receipt.PayloadDigest ||
		recorded.TargetID != receipt.TargetID || recorded.TargetDigest != receipt.TargetDigest {
		return ErrConflict
	}
	return nil
}

func validMemberGridHistoryRow(row v1archive.ArchivedRow, tableID string, ordinal int64) bool {
	return row.AdapterID == v1archive.DefaultAdapterID && row.TableID == tableID && row.SourceOrdinal == ordinal &&
		row.SourceKeyHMAC != ([sha256.Size]byte{}) && row.PayloadHMAC != ([sha256.Size]byte{}) && row.FieldHMAC != ([sha256.Size]byte{}) && json.Valid(row.Payload)
}

func memberGridRecoveryEntryDigest(entry v1membergridhistory.UsageSnapshotRecoveryEntry) ([sha256.Size]byte, error) {
	payload, err := json.Marshal(entry)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(payload), nil
}

func memberGridHistoryReason(reason, fallback string) string {
	if reason == "" {
		return fallback
	}
	return reason
}

func copyMemberGridHistoryID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func memberGridHistoryInt64Pointer(value *int) *int64 {
	if value == nil {
		return nil
	}
	converted := int64(*value)
	return &converted
}

func memberGridHistoryTime(value time.Time) time.Time { return value.UTC().Truncate(time.Microsecond) }

func memberGridHistoryOptionalTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	normalized := memberGridHistoryTime(*value)
	return &normalized
}
