package v1domain

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1legacymarketingcurrent"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

const legacyMarketingPrivateDigestDomain = "ai-crm-v2/v1-legacy-marketing-history"

// LegacyMarketingHistoryWriter owns the inert Segment history rows. It must
// create or replay its receipt in the caller transaction.
type LegacyMarketingHistoryWriter interface {
	ImportLegacyMarketingState(context.Context, string, segmentport.HistoricalLegacyMarketingState) (segmentport.LegacyMarketingHistoryReceipt, error)
	ImportLegacyMarketingValue(context.Context, string, segmentport.HistoricalLegacyMarketingValue) (segmentport.LegacyMarketingHistoryReceipt, error)
}

type LegacyMarketingHistoryImportResult struct {
	ImportedStates int
	ImportedValues int
	Quarantined    int
	Replayed       int
}

type LegacyMarketingHistoryImporter struct {
	archive       ArchiveSource
	uow           UnitOfWork
	writer        LegacyMarketingHistoryWriter
	journal       LegacyMarketingHistoryImportJournal
	sourceHMACKey []byte
}

func NewLegacyMarketingHistoryImporter(archive ArchiveSource, uow UnitOfWork, writer LegacyMarketingHistoryWriter, journal LegacyMarketingHistoryImportJournal, sourceHMACKey []byte) (*LegacyMarketingHistoryImporter, error) {
	if nilLegacyMarketingHistory(archive) || nilLegacyMarketingHistory(uow) || nilLegacyMarketingHistory(writer) || nilLegacyMarketingHistory(journal) || len(sourceHMACKey) < sha256.Size {
		return nil, ErrInvalidScope
	}
	return &LegacyMarketingHistoryImporter{archive: archive, uow: uow, writer: writer, journal: journal, sourceHMACKey: append([]byte(nil), sourceHMACKey...)}, nil
}

func (importer *LegacyMarketingHistoryImporter) Import(ctx context.Context, archiveRunID string) (LegacyMarketingHistoryImportResult, error) {
	if importer == nil || ctx == nil || archiveRunID == "" || nilLegacyMarketingHistory(importer.archive) || nilLegacyMarketingHistory(importer.uow) || nilLegacyMarketingHistory(importer.writer) || nilLegacyMarketingHistory(importer.journal) || len(importer.sourceHMACKey) < sha256.Size || importer.journal.ValidateLegacyMarketingHistoryImportScope(archiveRunID) != nil {
		return LegacyMarketingHistoryImportResult{}, ErrInvalidScope
	}
	stateRows, err := importer.readRows(ctx, archiveRunID, legacyMarketingStateTable)
	if err != nil {
		return LegacyMarketingHistoryImportResult{}, err
	}
	valueRows, err := importer.readRows(ctx, archiveRunID, legacyMarketingValueTable)
	if err != nil {
		return LegacyMarketingHistoryImportResult{}, err
	}
	history := v1legacymarketingcurrent.AdaptHistory(stateRows, valueRows)
	result := LegacyMarketingHistoryImportResult{}
	for index, decision := range history.MarketingStateCurrent {
		imported, replayed, importErr := importer.importState(ctx, stateRows[index], decision)
		if importErr != nil {
			return LegacyMarketingHistoryImportResult{}, importErr
		}
		if imported {
			result.ImportedStates++
		} else {
			result.Quarantined++
		}
		if replayed {
			result.Replayed++
		}
	}
	for index, decision := range history.MarketingValueSegmentCurrent {
		imported, replayed, importErr := importer.importValue(ctx, valueRows[index], decision)
		if importErr != nil {
			return LegacyMarketingHistoryImportResult{}, importErr
		}
		if imported {
			result.ImportedValues++
		} else {
			result.Quarantined++
		}
		if replayed {
			result.Replayed++
		}
	}
	if result.ImportedStates+result.ImportedValues+result.Quarantined != len(stateRows)+len(valueRows) {
		return LegacyMarketingHistoryImportResult{}, ErrConflict
	}
	return result, nil
}

func (importer *LegacyMarketingHistoryImporter) readRows(ctx context.Context, archiveRunID, table string) ([]v1archive.ArchivedRow, error) {
	rows := make([]v1archive.ArchivedRow, 0)
	seen := map[[sha256.Size]byte]struct{}{}
	ordinal := int64(1)
	err := importer.archive.EachTableRow(ctx, archiveRunID, table, func(row v1archive.ArchivedRow) error {
		if !importer.validSource(row, table, ordinal) {
			return ErrConflict
		}
		ordinal++
		if _, duplicate := seen[row.SourceKeyHMAC]; duplicate {
			return ErrConflict
		}
		seen[row.SourceKeyHMAC] = struct{}{}
		rows = append(rows, row)
		return nil
	})
	return rows, err
}

func (importer *LegacyMarketingHistoryImporter) validSource(row v1archive.ArchivedRow, table string, ordinal int64) bool {
	if (table != legacyMarketingStateTable && table != legacyMarketingValueTable) || row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != ordinal ||
		row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) || !json.Valid(row.Payload) {
		return false
	}
	payload, payloadErr := v1archive.PayloadHMAC(importer.sourceHMACKey, table[len("public/"):], row.Payload)
	fields, fieldErr := v1archive.FieldHMAC(importer.sourceHMACKey, table[len("public/"):], row.RedactedFields)
	return payloadErr == nil && fieldErr == nil && payload == row.PayloadHMAC && fields == row.FieldHMAC
}

func (importer *LegacyMarketingHistoryImporter) importState(ctx context.Context, row v1archive.ArchivedRow, decision v1legacymarketingcurrent.Result[v1legacymarketingcurrent.MarketingStateCurrentFact]) (bool, bool, error) {
	if reason := legacyMarketingDecisionReason(decision.Disposition, decision.Fact != nil, decision.Reason, "invalid_legacy_marketing_state"); reason != "" {
		return importer.quarantine(ctx, legacyMarketingStateKind, row, reason)
	}
	fact := *decision.Fact
	if !legacyMarketingEnvelopeMatches(row, fact.Source) {
		return false, false, ErrConflict
	}
	value := segmentport.HistoricalLegacyMarketingState{
		SourceKeyDigest: row.SourceKeyHMAC, SourcePayloadDigest: row.PayloadHMAC, SourceFieldDigest: row.FieldHMAC,
		SourceID: fact.SourceID, ExternalUserIDDigest: importer.privateDigest(legacyMarketingStateTable, "external_userid", []byte(fact.ExternalUserID)),
		ScenarioKey: fact.ScenarioKey, MarketingPhase: fact.MarketingPhase, PhaseLabel: fact.PhaseLabel, PhaseReason: fact.PhaseReason,
		LifecycleStatus: fact.LifecycleStatus, LastBatchSourceID: copyLegacyMarketingID(fact.LastBatchSourceID), LastBatchStatus: fact.LastBatchStatus,
		LastBatchWindowStart: fact.LastBatchWindowStart, LastBatchWindowEnd: fact.LastBatchWindowEnd, LastTriggerMessageAt: fact.LastTriggerMessageAt,
		EnteredAt: normalizeLegacyMarketingTime(fact.EnteredAt), ExitedAt: normalizeLegacyMarketingTime(fact.ExitedAt), ExitReason: fact.ExitReason,
		StatePayloadDigest: importer.privateDigest(legacyMarketingStateTable, "source_payload_json", fact.SourcePayload),
		CreatedAt:          normalizeLegacyMarketingTimeValue(fact.CreatedAt), UpdatedAt: normalizeLegacyMarketingTimeValue(fact.UpdatedAt),
	}
	return importer.write(ctx, legacyMarketingStateKind, row, func(tx context.Context) (segmentport.LegacyMarketingHistoryReceipt, error) {
		return importer.writer.ImportLegacyMarketingState(tx, SourceIdentifier(row.SourceKeyHMAC), value)
	})
}

func (importer *LegacyMarketingHistoryImporter) importValue(ctx context.Context, row v1archive.ArchivedRow, decision v1legacymarketingcurrent.Result[v1legacymarketingcurrent.MarketingValueSegmentCurrentFact]) (bool, bool, error) {
	if reason := legacyMarketingDecisionReason(decision.Disposition, decision.Fact != nil, decision.Reason, "invalid_legacy_marketing_value"); reason != "" {
		return importer.quarantine(ctx, legacyMarketingValueKind, row, reason)
	}
	fact := *decision.Fact
	if !legacyMarketingEnvelopeMatches(row, fact.Source) {
		return false, false, ErrConflict
	}
	value := segmentport.HistoricalLegacyMarketingValue{
		SourceKeyDigest: row.SourceKeyHMAC, SourcePayloadDigest: row.PayloadHMAC, SourceFieldDigest: row.FieldHMAC,
		SourceID: fact.SourceID, ExternalUserIDDigest: importer.privateDigest(legacyMarketingValueTable, "external_userid", []byte(fact.ExternalUserID)),
		ScenarioKey: fact.ScenarioKey, ValueSegment: fact.ValueSegment, SegmentLabel: fact.SegmentLabel, Score: fact.Score,
		ScoreBreakdownDigest: importer.privateDigest(legacyMarketingValueTable, "score_breakdown_json", fact.ScoreBreakdown),
		StatePayloadDigest:   importer.privateDigest(legacyMarketingValueTable, "source_payload_json", fact.SourcePayload),
		CreatedAt:            normalizeLegacyMarketingTimeValue(fact.CreatedAt), UpdatedAt: normalizeLegacyMarketingTimeValue(fact.UpdatedAt),
	}
	return importer.write(ctx, legacyMarketingValueKind, row, func(tx context.Context) (segmentport.LegacyMarketingHistoryReceipt, error) {
		return importer.writer.ImportLegacyMarketingValue(tx, SourceIdentifier(row.SourceKeyHMAC), value)
	})
}

func (importer *LegacyMarketingHistoryImporter) write(ctx context.Context, kind string, row v1archive.ArchivedRow, apply func(context.Context) (segmentport.LegacyMarketingHistoryReceipt, error)) (imported, replayed bool, err error) {
	err = importer.uow.Within(ctx, func(tx context.Context) error {
		imported, replayed = false, false
		receipt, writeErr := apply(tx)
		if errors.Is(writeErr, segmentport.ErrLegacyMarketingHistoryInvalid) {
			replayed, err = importer.recordQuarantine(tx, kind, row, "legacy_marketing_history_target_invalid")
			return err
		}
		if writeErr != nil {
			return writeErr
		}
		if err = importer.verifyReceipt(tx, kind, row, receipt); err != nil {
			return err
		}
		imported, replayed = true, receipt.Replayed
		return nil
	})
	return imported, replayed, err
}

func (importer *LegacyMarketingHistoryImporter) quarantine(ctx context.Context, kind string, row v1archive.ArchivedRow, reason string) (bool, bool, error) {
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		var recordErr error
		replayed, recordErr = importer.recordQuarantine(tx, kind, row, reason)
		return recordErr
	})
	return false, replayed, err
}

func (importer *LegacyMarketingHistoryImporter) recordQuarantine(ctx context.Context, kind string, row v1archive.ArchivedRow, reason string) (bool, error) {
	if reason == "" {
		return false, ErrInvalidScope
	}
	source := SourceIdentifier(row.SourceKeyHMAC)
	existing, found, err := importer.journal.LoadLegacyMarketingHistoryTerminal(ctx, kind, source)
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
	return false, importer.journal.RecordLegacyMarketingHistoryTerminal(ctx, kind, TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: "quarantine", Reason: reason})
}

func (importer *LegacyMarketingHistoryImporter) verifyReceipt(ctx context.Context, kind string, row v1archive.ArchivedRow, receipt segmentport.LegacyMarketingHistoryReceipt) error {
	if receipt.Kind != kind || receipt.SourceIdentifier != SourceIdentifier(row.SourceKeyHMAC) || receipt.PayloadDigest != row.PayloadHMAC || receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	terminal, found, err := importer.journal.LoadLegacyMarketingHistoryTerminal(ctx, kind, receipt.SourceIdentifier)
	if err != nil {
		return err
	}
	if !found || terminal.SourceKeyDigest != row.SourceKeyHMAC || terminal.PayloadDigest != row.PayloadHMAC || terminal.Disposition != "import" || terminal.Reason != "" ||
		terminal.TargetID != strconv.FormatInt(receipt.TargetID, 10) || terminal.TargetDigest != receipt.TargetDigest || len(terminal.Metadata) != 0 {
		return ErrConflict
	}
	return nil
}

func legacyMarketingDecisionReason(disposition v1legacymarketingcurrent.Disposition, hasFact bool, reason, fallback string) string {
	if disposition == v1legacymarketingcurrent.DispositionCandidate && hasFact {
		return ""
	}
	if disposition == v1legacymarketingcurrent.DispositionQuarantine && reason != "" {
		return reason
	}
	return fallback
}

func legacyMarketingEnvelopeMatches(row v1archive.ArchivedRow, source v1legacymarketingcurrent.SourceRecord) bool {
	return source.SourceKeyHMAC == row.SourceKeyHMAC && source.PayloadHMAC == row.PayloadHMAC && source.FieldHMAC == row.FieldHMAC && source.SourceOrdinal == row.SourceOrdinal
}

func (importer *LegacyMarketingHistoryImporter) privateDigest(table, field string, value []byte) [sha256.Size]byte {
	mac := hmac.New(sha256.New, importer.sourceHMACKey)
	_, _ = mac.Write([]byte(legacyMarketingPrivateDigestDomain))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(table))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(field))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(value)
	var result [sha256.Size]byte
	copy(result[:], mac.Sum(nil))
	return result
}

func copyLegacyMarketingID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func normalizeLegacyMarketingTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	normalized := value.UTC().Truncate(time.Microsecond)
	return &normalized
}

func normalizeLegacyMarketingTimeValue(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}

func nilLegacyMarketingHistory(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Ptr || reflected.Kind() == reflect.Interface || reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Slice || reflected.Kind() == reflect.Func) && reflected.IsNil()
}
