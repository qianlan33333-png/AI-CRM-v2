package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1marketingstatehistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

type MarketingStateHistoryWriter interface {
	ImportMarketingStateSnapshot(context.Context, string, segmentport.HistoricalMarketingStateSnapshot) (segmentport.MarketingStateHistoryReceipt, error)
	ImportMarketingStateChange(context.Context, string, segmentport.HistoricalMarketingStateChange) (segmentport.MarketingStateHistoryReceipt, error)
	ImportValueSegmentSnapshot(context.Context, string, segmentport.HistoricalValueSegmentSnapshot) (segmentport.MarketingStateHistoryReceipt, error)
	ImportValueSegmentChange(context.Context, string, segmentport.HistoricalValueSegmentChange) (segmentport.MarketingStateHistoryReceipt, error)
}

type MarketingStateHistoryImportResult struct {
	ImportedMarketingStateSnapshots int
	ImportedMarketingStateChanges   int
	ImportedValueSegmentSnapshots   int
	ImportedValueSegmentChanges     int
	Quarantined                     int
	Replayed                        int
}

type MarketingStateHistoryImporter struct {
	archive ArchiveSource
	uow     UnitOfWork
	writer  MarketingStateHistoryWriter
	journal MarketingStateHistoryImportJournal
}

func NewMarketingStateHistoryImporter(archive ArchiveSource, uow UnitOfWork, writer MarketingStateHistoryWriter, journal MarketingStateHistoryImportJournal) (*MarketingStateHistoryImporter, error) {
	if archive == nil || uow == nil || writer == nil || journal == nil {
		return nil, ErrInvalidScope
	}
	return &MarketingStateHistoryImporter{archive: archive, uow: uow, writer: writer, journal: journal}, nil
}

func (importer *MarketingStateHistoryImporter) Import(ctx context.Context, archiveRunID string) (MarketingStateHistoryImportResult, error) {
	if importer == nil || ctx == nil || importer.archive == nil || importer.uow == nil || importer.writer == nil || importer.journal == nil ||
		importer.journal.ValidateMarketingStateHistoryImportScope(archiveRunID) != nil {
		return MarketingStateHistoryImportResult{}, ErrInvalidScope
	}
	marketingCurrent, err := importer.readRows(ctx, archiveRunID, marketingStateSnapshotTable)
	if err != nil {
		return MarketingStateHistoryImportResult{}, err
	}
	marketingHistory, err := importer.readRows(ctx, archiveRunID, marketingStateChangeTable)
	if err != nil {
		return MarketingStateHistoryImportResult{}, err
	}
	valueCurrent, err := importer.readRows(ctx, archiveRunID, valueSegmentSnapshotTable)
	if err != nil {
		return MarketingStateHistoryImportResult{}, err
	}
	valueHistory, err := importer.readRows(ctx, archiveRunID, valueSegmentChangeTable)
	if err != nil {
		return MarketingStateHistoryImportResult{}, err
	}

	history := v1marketingstatehistory.AdaptHistory(marketingCurrent, marketingHistory, valueCurrent, valueHistory)
	result := MarketingStateHistoryImportResult{}
	for index, decision := range history.MarketingStateCurrent {
		imported, replayed, importErr := importer.importMarketingStateSnapshot(ctx, marketingCurrent[index], decision)
		if importErr != nil {
			return MarketingStateHistoryImportResult{}, importErr
		}
		if imported {
			result.ImportedMarketingStateSnapshots++
		} else {
			result.Quarantined++
		}
		if replayed {
			result.Replayed++
		}
	}
	for index, decision := range history.MarketingStateHistory {
		imported, replayed, importErr := importer.importMarketingStateChange(ctx, marketingHistory[index], decision)
		if importErr != nil {
			return MarketingStateHistoryImportResult{}, importErr
		}
		if imported {
			result.ImportedMarketingStateChanges++
		} else {
			result.Quarantined++
		}
		if replayed {
			result.Replayed++
		}
	}
	for index, decision := range history.ValueSegmentCurrent {
		imported, replayed, importErr := importer.importValueSegmentSnapshot(ctx, valueCurrent[index], decision)
		if importErr != nil {
			return MarketingStateHistoryImportResult{}, importErr
		}
		if imported {
			result.ImportedValueSegmentSnapshots++
		} else {
			result.Quarantined++
		}
		if replayed {
			result.Replayed++
		}
	}
	for index, decision := range history.ValueSegmentHistory {
		imported, replayed, importErr := importer.importValueSegmentChange(ctx, valueHistory[index], decision)
		if importErr != nil {
			return MarketingStateHistoryImportResult{}, importErr
		}
		if imported {
			result.ImportedValueSegmentChanges++
		} else {
			result.Quarantined++
		}
		if replayed {
			result.Replayed++
		}
	}
	if result.ImportedMarketingStateSnapshots+result.ImportedMarketingStateChanges+result.ImportedValueSegmentSnapshots+result.ImportedValueSegmentChanges+result.Quarantined != len(marketingCurrent)+len(marketingHistory)+len(valueCurrent)+len(valueHistory) {
		return MarketingStateHistoryImportResult{}, ErrConflict
	}
	return result, nil
}

func (importer *MarketingStateHistoryImporter) readRows(ctx context.Context, archiveRunID, table string) ([]v1archive.ArchivedRow, error) {
	rows := []v1archive.ArchivedRow{}
	seen := map[[sha256.Size]byte]struct{}{}
	ordinal := int64(1)
	err := importer.archive.EachTableRow(ctx, archiveRunID, table, func(row v1archive.ArchivedRow) error {
		if !validMarketingStateHistoryRow(row, table, ordinal) {
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

func validMarketingStateHistoryRow(row v1archive.ArchivedRow, table string, ordinal int64) bool {
	return row.AdapterID == v1archive.DefaultAdapterID && row.TableID == table && row.SourceOrdinal == ordinal &&
		row.SourceKeyHMAC != ([sha256.Size]byte{}) && row.PayloadHMAC != ([sha256.Size]byte{}) && row.FieldHMAC != ([sha256.Size]byte{}) && json.Valid(row.Payload)
}

func (importer *MarketingStateHistoryImporter) importMarketingStateSnapshot(ctx context.Context, row v1archive.ArchivedRow, decision v1marketingstatehistory.Result[v1marketingstatehistory.MarketingStateCurrentFact]) (bool, bool, error) {
	if reason := marketingStateHistoryDecisionReason(decision.Disposition, decision.Fact != nil, decision.Reason, "invalid_marketing_state_snapshot"); reason != "" {
		return importer.quarantine(ctx, marketingStateSnapshotKind, row, reason)
	}
	fact := *decision.Fact
	if !marketingStateHistoryEnvelopeMatches(row, fact.Source) {
		return false, false, ErrConflict
	}
	value := segmentport.HistoricalMarketingStateSnapshot{SourceKeyDigest: [sha256.Size]byte(fact.Source.SourceKeyDigest), SourcePayloadDigest: [sha256.Size]byte(fact.Source.PayloadDigest), SourceFieldDigest: [sha256.Size]byte(fact.Source.FieldDigest),
		SourceID: fact.SourceID, PersonSourceID: fact.PersonSourceID, ExternalUserIDDigest: [sha256.Size]byte(fact.ExternalUserIDDigest), AutomationKey: fact.AutomationKey, MainStage: fact.MainStage, SubStage: fact.SubStage,
		Activated: fact.Activated, Converted: fact.Converted, EligibleForConversion: fact.EligibleForConversion, LifecycleStatus: fact.LifecycleStatus, LastActivationAt: fact.LastActivationAt,
		LastConversionMarkedAt: fact.LastConversionMarkedAt, LastMessageAt: fact.LastMessageAt, LastBatchSourceID: fact.LastBatchSourceID, LastBatchStatus: fact.LastBatchStatus,
		LastBatchWindowStart: fact.LastBatchWindowStart, LastBatchWindowEnd: fact.LastBatchWindowEnd, LastTriggerMessageAt: fact.LastTriggerMessageAt, EnteredAt: fact.EnteredAt, ExitedAt: fact.ExitedAt,
		ExitReason: fact.ExitReason, StatePayloadDigest: [sha256.Size]byte(fact.StatePayloadDigest), CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt}
	return importer.write(ctx, marketingStateSnapshotKind, row, func(tx context.Context) (segmentport.MarketingStateHistoryReceipt, error) {
		return importer.writer.ImportMarketingStateSnapshot(tx, SourceIdentifier(row.SourceKeyHMAC), value)
	})
}

func (importer *MarketingStateHistoryImporter) importMarketingStateChange(ctx context.Context, row v1archive.ArchivedRow, decision v1marketingstatehistory.Result[v1marketingstatehistory.MarketingStateHistoryFact]) (bool, bool, error) {
	if reason := marketingStateHistoryDecisionReason(decision.Disposition, decision.Fact != nil, decision.Reason, "invalid_marketing_state_change"); reason != "" {
		return importer.quarantine(ctx, marketingStateChangeKind, row, reason)
	}
	fact := *decision.Fact
	if !marketingStateHistoryEnvelopeMatches(row, fact.Source) {
		return false, false, ErrConflict
	}
	value := segmentport.HistoricalMarketingStateChange{SourceKeyDigest: [sha256.Size]byte(fact.Source.SourceKeyDigest), SourcePayloadDigest: [sha256.Size]byte(fact.Source.PayloadDigest), SourceFieldDigest: [sha256.Size]byte(fact.Source.FieldDigest),
		SourceID: fact.SourceID, PersonSourceID: fact.PersonSourceID, BatchSourceID: fact.BatchSourceID, ExternalUserIDDigest: [sha256.Size]byte(fact.ExternalUserIDDigest), AutomationKey: fact.AutomationKey,
		MainStage: fact.MainStage, SubStage: fact.SubStage, Activated: fact.Activated, Converted: fact.Converted, EligibleForConversion: fact.EligibleForConversion,
		LifecycleStatus: fact.LifecycleStatus, LastActivationAt: fact.LastActivationAt, LastConversionMarkedAt: fact.LastConversionMarkedAt, LastMessageAt: fact.LastMessageAt,
		ExitReason: fact.ExitReason, ChangeReason: fact.ChangeReason, StatePayloadDigest: [sha256.Size]byte(fact.StatePayloadDigest), RecordedAt: fact.RecordedAt, CreatedAt: fact.CreatedAt}
	return importer.write(ctx, marketingStateChangeKind, row, func(tx context.Context) (segmentport.MarketingStateHistoryReceipt, error) {
		return importer.writer.ImportMarketingStateChange(tx, SourceIdentifier(row.SourceKeyHMAC), value)
	})
}

func (importer *MarketingStateHistoryImporter) importValueSegmentSnapshot(ctx context.Context, row v1archive.ArchivedRow, decision v1marketingstatehistory.Result[v1marketingstatehistory.ValueSegmentCurrentFact]) (bool, bool, error) {
	if reason := marketingStateHistoryDecisionReason(decision.Disposition, decision.Fact != nil, decision.Reason, "invalid_value_segment_snapshot"); reason != "" {
		return importer.quarantine(ctx, valueSegmentSnapshotKind, row, reason)
	}
	fact := *decision.Fact
	rank, score, ok := marketingStateInt32(fact.SegmentRank), marketingStateInt32(fact.Score), true
	if rank == nil || score == nil {
		ok = false
	}
	if !ok || !marketingStateHistoryEnvelopeMatches(row, fact.Source) {
		if !ok {
			return importer.quarantine(ctx, valueSegmentSnapshotKind, row, "value_segment_integer_out_of_range")
		}
		return false, false, ErrConflict
	}
	value := segmentport.HistoricalValueSegmentSnapshot{SourceKeyDigest: [sha256.Size]byte(fact.Source.SourceKeyDigest), SourcePayloadDigest: [sha256.Size]byte(fact.Source.PayloadDigest), SourceFieldDigest: [sha256.Size]byte(fact.Source.FieldDigest),
		SourceID: fact.SourceID, ExternalUserIDDigest: [sha256.Size]byte(fact.ExternalUserIDDigest), Segment: fact.Segment, SegmentRank: *rank, Score: *score, ScoringVersion: fact.ScoringVersion,
		SubmissionSourceID: fact.SubmissionSourceID, MatchedQuestionIDsDigest: [sha256.Size]byte(fact.MatchedQuestionIDsDigest), StatePayloadDigest: [sha256.Size]byte(fact.SourcePayloadDigest), ComputedReason: fact.ComputedReason,
		EvaluatedAt: fact.EvaluatedAt, ComputedAt: fact.ComputedAt, CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt}
	return importer.write(ctx, valueSegmentSnapshotKind, row, func(tx context.Context) (segmentport.MarketingStateHistoryReceipt, error) {
		return importer.writer.ImportValueSegmentSnapshot(tx, SourceIdentifier(row.SourceKeyHMAC), value)
	})
}

func (importer *MarketingStateHistoryImporter) importValueSegmentChange(ctx context.Context, row v1archive.ArchivedRow, decision v1marketingstatehistory.Result[v1marketingstatehistory.ValueSegmentHistoryFact]) (bool, bool, error) {
	if reason := marketingStateHistoryDecisionReason(decision.Disposition, decision.Fact != nil, decision.Reason, "invalid_value_segment_change"); reason != "" {
		return importer.quarantine(ctx, valueSegmentChangeKind, row, reason)
	}
	fact := *decision.Fact
	rank, score, ok := marketingStateInt32(fact.SegmentRank), marketingStateInt32(fact.Score), true
	if rank == nil || score == nil {
		ok = false
	}
	if !ok || !marketingStateHistoryEnvelopeMatches(row, fact.Source) {
		if !ok {
			return importer.quarantine(ctx, valueSegmentChangeKind, row, "value_segment_integer_out_of_range")
		}
		return false, false, ErrConflict
	}
	value := segmentport.HistoricalValueSegmentChange{SourceKeyDigest: [sha256.Size]byte(fact.Source.SourceKeyDigest), SourcePayloadDigest: [sha256.Size]byte(fact.Source.PayloadDigest), SourceFieldDigest: [sha256.Size]byte(fact.Source.FieldDigest),
		SourceID: fact.SourceID, ExternalUserIDDigest: [sha256.Size]byte(fact.ExternalUserIDDigest), Segment: fact.Segment, SegmentRank: *rank, Score: *score, ScoringVersion: fact.ScoringVersion,
		SubmissionSourceID: fact.SubmissionSourceID, MatchedQuestionIDsDigest: [sha256.Size]byte(fact.MatchedQuestionIDsDigest), StatePayloadDigest: [sha256.Size]byte(fact.SourcePayloadDigest), ChangeReason: fact.ChangeReason,
		EvaluatedAt: fact.EvaluatedAt, RecordedAt: fact.RecordedAt, CreatedAt: fact.CreatedAt}
	return importer.write(ctx, valueSegmentChangeKind, row, func(tx context.Context) (segmentport.MarketingStateHistoryReceipt, error) {
		return importer.writer.ImportValueSegmentChange(tx, SourceIdentifier(row.SourceKeyHMAC), value)
	})
}

func marketingStateInt32(value int64) *int32 {
	if value < -1<<31 || value > 1<<31-1 {
		return nil
	}
	converted := int32(value)
	return &converted
}

func (importer *MarketingStateHistoryImporter) write(ctx context.Context, kind string, row v1archive.ArchivedRow, apply func(context.Context) (segmentport.MarketingStateHistoryReceipt, error)) (imported, replayed bool, err error) {
	err = importer.uow.Within(ctx, func(tx context.Context) error {
		imported, replayed = false, false
		receipt, writeErr := apply(tx)
		if errors.Is(writeErr, segmentport.ErrMarketingStateHistoryInvalid) {
			var terminalErr error
			replayed, terminalErr = importer.recordQuarantine(tx, kind, row, "marketing_state_history_target_invalid")
			return terminalErr
		}
		if writeErr != nil {
			return writeErr
		}
		if verifyErr := importer.verifyReceipt(tx, kind, row, receipt); verifyErr != nil {
			return verifyErr
		}
		imported, replayed = true, receipt.Replayed
		return nil
	})
	return imported, replayed, err
}

func (importer *MarketingStateHistoryImporter) quarantine(ctx context.Context, kind string, row v1archive.ArchivedRow, reason string) (bool, bool, error) {
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		var terminalErr error
		replayed, terminalErr = importer.recordQuarantine(tx, kind, row, reason)
		return terminalErr
	})
	return false, replayed, err
}

func (importer *MarketingStateHistoryImporter) recordQuarantine(ctx context.Context, kind string, row v1archive.ArchivedRow, reason string) (bool, error) {
	if reason == "" {
		return false, ErrInvalidScope
	}
	source := SourceIdentifier(row.SourceKeyHMAC)
	existing, found, err := importer.journal.LoadMarketingStateHistoryTerminal(ctx, kind, source)
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
	return false, importer.journal.RecordMarketingStateHistoryTerminal(ctx, kind, TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: "quarantine", Reason: reason})
}

func (importer *MarketingStateHistoryImporter) verifyReceipt(ctx context.Context, kind string, row v1archive.ArchivedRow, receipt segmentport.MarketingStateHistoryReceipt) error {
	if receipt.Kind != kind || receipt.SourceIdentifier != SourceIdentifier(row.SourceKeyHMAC) || receipt.PayloadDigest != row.PayloadHMAC || receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	terminal, found, err := importer.journal.LoadMarketingStateHistoryTerminal(ctx, kind, receipt.SourceIdentifier)
	if err != nil {
		return err
	}
	if !found || terminal.SourceKeyDigest != row.SourceKeyHMAC || terminal.PayloadDigest != row.PayloadHMAC || terminal.Disposition != "import" || terminal.Reason != "" ||
		terminal.TargetID != strconv.FormatInt(receipt.TargetID, 10) || terminal.TargetDigest != receipt.TargetDigest || len(terminal.Metadata) != 0 {
		return ErrConflict
	}
	return nil
}

func marketingStateHistoryDecisionReason(disposition v1marketingstatehistory.Disposition, hasFact bool, reason, fallback string) string {
	if disposition == v1marketingstatehistory.DispositionCandidate && hasFact {
		return ""
	}
	if disposition == v1marketingstatehistory.DispositionQuarantine && reason != "" {
		return reason
	}
	return fallback
}

func marketingStateHistoryEnvelopeMatches(row v1archive.ArchivedRow, envelope v1marketingstatehistory.SourceEnvelope) bool {
	return [sha256.Size]byte(envelope.SourceKeyDigest) == row.SourceKeyHMAC && [sha256.Size]byte(envelope.PayloadDigest) == row.PayloadHMAC && [sha256.Size]byte(envelope.FieldDigest) == row.FieldHMAC
}
