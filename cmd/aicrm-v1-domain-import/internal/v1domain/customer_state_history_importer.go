package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1customerstatehistory"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

// CustomerStateHistoryWriter owns the three immutable Contact projections.
// Each method must use the importer caller transaction and write its journal
// receipt before returning.
type CustomerStateHistoryWriter interface {
	ImportCustomerStatusSnapshot(context.Context, string, contactport.HistoricalCustomerStatusSnapshot) (contactport.CustomerStateHistoryReceipt, error)
	ImportCustomerStatusChange(context.Context, string, contactport.HistoricalCustomerStatusChange) (contactport.CustomerStateHistoryReceipt, error)
	ImportClassTermTagMapping(context.Context, string, contactport.HistoricalClassTermTagMapping) (contactport.CustomerStateHistoryReceipt, error)
}

type CustomerStateHistoryImportResult struct {
	ImportedSnapshots, ImportedChanges, ImportedTermMappings int
	Quarantined, Replayed                                    int
}

type CustomerStateHistoryImporter struct {
	archive ArchiveSource
	uow     UnitOfWork
	writer  CustomerStateHistoryWriter
	journal CustomerStateHistoryImportJournal
}

func NewCustomerStateHistoryImporter(archive ArchiveSource, uow UnitOfWork, writer CustomerStateHistoryWriter, journal CustomerStateHistoryImportJournal) (*CustomerStateHistoryImporter, error) {
	if archive == nil || uow == nil || writer == nil || journal == nil {
		return nil, ErrInvalidScope
	}
	return &CustomerStateHistoryImporter{archive: archive, uow: uow, writer: writer, journal: journal}, nil
}

func (importer *CustomerStateHistoryImporter) Import(ctx context.Context, archiveRunID string) (CustomerStateHistoryImportResult, error) {
	if importer == nil || ctx == nil || importer.archive == nil || importer.uow == nil || importer.writer == nil || importer.journal == nil ||
		importer.journal.ValidateCustomerStateHistoryImportScope(archiveRunID) != nil {
		return CustomerStateHistoryImportResult{}, ErrInvalidScope
	}
	snapshots, err := importer.readRows(ctx, archiveRunID, customerStateHistorySnapshotTable)
	if err != nil {
		return CustomerStateHistoryImportResult{}, err
	}
	changes, err := importer.readRows(ctx, archiveRunID, customerStateHistoryChangeTable)
	if err != nil {
		return CustomerStateHistoryImportResult{}, err
	}
	terms, err := importer.readRows(ctx, archiveRunID, customerStateHistoryTermTable)
	if err != nil {
		return CustomerStateHistoryImportResult{}, err
	}

	result := CustomerStateHistoryImportResult{}
	for index, decision := range v1customerstatehistory.AdaptUserStatusCurrentRows(snapshots) {
		imported, replayed, err := importer.importSnapshot(ctx, snapshots[index], decision)
		if err != nil {
			return CustomerStateHistoryImportResult{}, err
		}
		if imported {
			result.ImportedSnapshots++
		} else {
			result.Quarantined++
		}
		if replayed {
			result.Replayed++
		}
	}
	for index, decision := range v1customerstatehistory.AdaptUserStatusHistoryRows(changes) {
		imported, replayed, err := importer.importChange(ctx, changes[index], decision)
		if err != nil {
			return CustomerStateHistoryImportResult{}, err
		}
		if imported {
			result.ImportedChanges++
		} else {
			result.Quarantined++
		}
		if replayed {
			result.Replayed++
		}
	}
	for index, decision := range v1customerstatehistory.AdaptTermTagMappingRows(terms) {
		imported, replayed, err := importer.importTerm(ctx, terms[index], decision)
		if err != nil {
			return CustomerStateHistoryImportResult{}, err
		}
		if imported {
			result.ImportedTermMappings++
		} else {
			result.Quarantined++
		}
		if replayed {
			result.Replayed++
		}
	}
	if result.ImportedSnapshots+result.ImportedChanges+result.ImportedTermMappings+result.Quarantined != len(snapshots)+len(changes)+len(terms) {
		return CustomerStateHistoryImportResult{}, ErrConflict
	}
	return result, nil
}

func (importer *CustomerStateHistoryImporter) readRows(ctx context.Context, archiveRunID, table string) ([]v1archive.ArchivedRow, error) {
	rows := []v1archive.ArchivedRow{}
	seen := map[[sha256.Size]byte]struct{}{}
	ordinal := int64(1)
	err := importer.archive.EachTableRow(ctx, archiveRunID, table, func(row v1archive.ArchivedRow) error {
		if !validCustomerStateHistoryRow(row, table, ordinal) {
			return ErrConflict
		}
		ordinal++
		if _, found := seen[row.SourceKeyHMAC]; found {
			return ErrConflict
		}
		seen[row.SourceKeyHMAC] = struct{}{}
		rows = append(rows, row)
		return nil
	})
	return rows, err
}

func validCustomerStateHistoryRow(row v1archive.ArchivedRow, table string, ordinal int64) bool {
	return row.AdapterID == v1archive.DefaultAdapterID && row.TableID == table && row.SourceOrdinal == ordinal &&
		row.SourceKeyHMAC != ([sha256.Size]byte{}) && row.PayloadHMAC != ([sha256.Size]byte{}) && row.FieldHMAC != ([sha256.Size]byte{}) && json.Valid(row.Payload)
}

func (importer *CustomerStateHistoryImporter) importSnapshot(ctx context.Context, row v1archive.ArchivedRow, decision v1customerstatehistory.Result[v1customerstatehistory.UserStatusCurrent]) (bool, bool, error) {
	if reason := customerStateHistoryDecisionReason(decision.Disposition, decision.Candidate != nil, decision.Reason, "invalid_customer_status_snapshot"); reason != "" {
		return importer.quarantine(ctx, customerStateHistorySnapshotKind, row, reason)
	}
	fact := *decision.Candidate
	if !customerStateHistoryEnvelopeMatches(row, fact.Envelope) {
		return false, false, ErrConflict
	}
	value := contactport.HistoricalCustomerStatusSnapshot{SourceKeyDigest: fact.Envelope.SourceKeyDigest, SourcePayloadDigest: fact.Envelope.SourcePayloadDigest, SourceFieldDigest: fact.Envelope.SourceFieldDigest,
		SignupStatus: fact.SignupStatus, SignupLabelName: fact.SignupLabelName, CustomerNameSnapshot: fact.CustomerNameSnapshot, OwnerUserIDSnapshot: fact.OwnerUserIDSnapshot,
		SetByUserIDDigest: fact.SetByUserIDDigest, SetAt: fact.SetAt, WeComTagSyncStatus: fact.WeComTagSyncStatus, WeComTagSyncErrorHash: fact.WeComTagSyncErrorHash,
		StatusFlagsDigest: fact.StatusFlagsDigest, CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt, UnionID: fact.UnionID}
	return importer.write(ctx, customerStateHistorySnapshotKind, row, func(tx context.Context) (contactport.CustomerStateHistoryReceipt, error) {
		return importer.writer.ImportCustomerStatusSnapshot(tx, SourceIdentifier(row.SourceKeyHMAC), value)
	})
}

func (importer *CustomerStateHistoryImporter) importChange(ctx context.Context, row v1archive.ArchivedRow, decision v1customerstatehistory.Result[v1customerstatehistory.UserStatusHistory]) (bool, bool, error) {
	if reason := customerStateHistoryDecisionReason(decision.Disposition, decision.Candidate != nil, decision.Reason, "invalid_customer_status_change"); reason != "" {
		return importer.quarantine(ctx, customerStateHistoryChangeKind, row, reason)
	}
	fact := *decision.Candidate
	if !customerStateHistoryEnvelopeMatches(row, fact.Envelope) {
		return false, false, ErrConflict
	}
	value := contactport.HistoricalCustomerStatusChange{SourceKeyDigest: fact.Envelope.SourceKeyDigest, SourcePayloadDigest: fact.Envelope.SourcePayloadDigest, SourceFieldDigest: fact.Envelope.SourceFieldDigest,
		SourceID: fact.SourceID, OldSignupStatus: fact.OldSignupStatus, NewSignupStatus: fact.NewSignupStatus, OldLabelName: fact.OldLabelName, NewLabelName: fact.NewLabelName,
		CustomerNameSnapshot: fact.CustomerNameSnapshot, OwnerUserIDSnapshot: fact.OwnerUserIDSnapshot, SetByUserIDDigest: fact.SetByUserIDDigest, SetAt: fact.SetAt,
		WeComTagSyncStatus: fact.WeComTagSyncStatus, WeComTagSyncErrorHash: fact.WeComTagSyncErrorHash, StatusFlagsDigest: fact.StatusFlagsDigest, CreatedAt: fact.CreatedAt, UnionID: fact.UnionID}
	return importer.write(ctx, customerStateHistoryChangeKind, row, func(tx context.Context) (contactport.CustomerStateHistoryReceipt, error) {
		return importer.writer.ImportCustomerStatusChange(tx, SourceIdentifier(row.SourceKeyHMAC), value)
	})
}

func (importer *CustomerStateHistoryImporter) importTerm(ctx context.Context, row v1archive.ArchivedRow, decision v1customerstatehistory.Result[v1customerstatehistory.TermTagMapping]) (bool, bool, error) {
	if reason := customerStateHistoryDecisionReason(decision.Disposition, decision.Candidate != nil, decision.Reason, "invalid_class_term_tag_mapping"); reason != "" {
		return importer.quarantine(ctx, customerStateHistoryTermKind, row, reason)
	}
	fact := *decision.Candidate
	if !customerStateHistoryEnvelopeMatches(row, fact.Envelope) {
		return false, false, ErrConflict
	}
	value := contactport.HistoricalClassTermTagMapping{SourceKeyDigest: fact.Envelope.SourceKeyDigest, SourcePayloadDigest: fact.Envelope.SourcePayloadDigest, SourceFieldDigest: fact.Envelope.SourceFieldDigest,
		SourceID: fact.SourceID, TagGroupName: fact.TagGroupName, TagName: fact.TagName, ClassTermNo: fact.ClassTermNo, ClassTermLabel: fact.ClassTermLabel,
		OriginalActive: fact.IsActive, CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt, StrategySourceID: fact.StrategyID, GroupSourceID: fact.GroupID, TagSourceID: fact.TagID}
	return importer.write(ctx, customerStateHistoryTermKind, row, func(tx context.Context) (contactport.CustomerStateHistoryReceipt, error) {
		return importer.writer.ImportClassTermTagMapping(tx, SourceIdentifier(row.SourceKeyHMAC), value)
	})
}

func (importer *CustomerStateHistoryImporter) write(ctx context.Context, kind string, row v1archive.ArchivedRow, apply func(context.Context) (contactport.CustomerStateHistoryReceipt, error)) (imported, replayed bool, err error) {
	err = importer.uow.Within(ctx, func(tx context.Context) error {
		imported, replayed = false, false
		receipt, writeErr := apply(tx)
		if errors.Is(writeErr, contactport.ErrCustomerStateHistoryInvalid) {
			var terminalErr error
			replayed, terminalErr = importer.recordQuarantine(tx, kind, row, "customer_state_history_target_invalid")
			return terminalErr
		}
		if writeErr != nil {
			return writeErr
		}
		if err := importer.verifyReceipt(tx, kind, row, receipt); err != nil {
			return err
		}
		imported, replayed = true, receipt.Replayed
		return nil
	})
	return imported, replayed, err
}

func (importer *CustomerStateHistoryImporter) quarantine(ctx context.Context, kind string, row v1archive.ArchivedRow, reason string) (bool, bool, error) {
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		var err error
		replayed, err = importer.recordQuarantine(tx, kind, row, reason)
		return err
	})
	return false, replayed, err
}

func (importer *CustomerStateHistoryImporter) recordQuarantine(ctx context.Context, kind string, row v1archive.ArchivedRow, reason string) (bool, error) {
	if reason == "" {
		return false, ErrInvalidScope
	}
	source := SourceIdentifier(row.SourceKeyHMAC)
	existing, found, err := importer.journal.LoadCustomerStateHistoryTerminal(ctx, kind, source)
	if err != nil {
		return false, err
	}
	if found {
		if existing.SourceKeyDigest != row.SourceKeyHMAC || existing.PayloadDigest != row.PayloadHMAC || existing.Disposition != "quarantine" || existing.Reason != reason ||
			existing.TargetID != "" || existing.TargetDigest != ([sha256.Size]byte{}) || len(existing.Metadata) != 0 {
			return false, ErrConflict
		}
		return true, nil
	}
	return false, importer.journal.RecordCustomerStateHistoryTerminal(ctx, kind, TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: "quarantine", Reason: reason})
}

func (importer *CustomerStateHistoryImporter) verifyReceipt(ctx context.Context, kind string, row v1archive.ArchivedRow, receipt contactport.CustomerStateHistoryReceipt) error {
	if receipt.Kind != kind || receipt.SourceIdentifier != SourceIdentifier(row.SourceKeyHMAC) || receipt.PayloadDigest != row.PayloadHMAC || receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	terminal, found, err := importer.journal.LoadCustomerStateHistoryTerminal(ctx, kind, receipt.SourceIdentifier)
	if err != nil {
		return err
	}
	if !found || terminal.SourceKeyDigest != row.SourceKeyHMAC || terminal.PayloadDigest != row.PayloadHMAC || terminal.Disposition != "import" || terminal.Reason != "" ||
		terminal.TargetID != strconv.FormatInt(receipt.TargetID, 10) || terminal.TargetDigest != receipt.TargetDigest || len(terminal.Metadata) != 0 {
		return ErrConflict
	}
	return nil
}

func customerStateHistoryDecisionReason(disposition v1customerstatehistory.Disposition, hasFact bool, reason, fallback string) string {
	if disposition == v1customerstatehistory.DispositionCandidate && hasFact {
		return ""
	}
	if disposition == v1customerstatehistory.DispositionQuarantine && reason != "" {
		return reason
	}
	return fallback
}

func customerStateHistoryEnvelopeMatches(row v1archive.ArchivedRow, envelope v1customerstatehistory.SourceEnvelope) bool {
	return envelope.SourceKeyDigest == row.SourceKeyHMAC && envelope.SourcePayloadDigest == row.PayloadHMAC && envelope.SourceFieldDigest == row.FieldHMAC
}
