package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1outboundtaskhistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

const (
	outboundTaskHistoryImportVersion = "v1-outbound-task-history-a1"
	outboundTaskHistoryTargetTable   = "outbound_v1_task_history"
)

// OutboundTaskHistoryWriter owns the inert Outbound history target. It must
// use the caller transaction and never create a current task or effect.
type OutboundTaskHistoryWriter interface {
	Import(context.Context, string, outboundport.HistoricalOutboundTask) (outboundport.OutboundTaskHistoryReceipt, error)
}

// OutboundTaskHistoryImportJournal is the concrete import receipt boundary;
// the main integration provides the CLI and reconciliation wiring.
type OutboundTaskHistoryImportJournal interface {
	outboundport.OutboundTaskHistoryJournal
	LoadTerminal(context.Context, string) (TerminalReceipt, bool, error)
	Record(context.Context, TerminalReceipt) error
	ValidateOutboundTaskHistoryImportScope(string) error
}

type OutboundTaskHistoryImportResult struct{ ImportedTasks, Quarantined, Replayed int }

type OutboundTaskHistoryImporter struct {
	archive       ArchiveSource
	uow           UnitOfWork
	writer        OutboundTaskHistoryWriter
	journal       OutboundTaskHistoryImportJournal
	sourceHMACKey []byte
}

func NewOutboundTaskHistoryImporter(archive ArchiveSource, uow UnitOfWork, writer OutboundTaskHistoryWriter, journal OutboundTaskHistoryImportJournal, sourceHMACKey []byte) (*OutboundTaskHistoryImporter, error) {
	if archive == nil || uow == nil || writer == nil || journal == nil || len(sourceHMACKey) < sha256.Size {
		return nil, ErrInvalidScope
	}
	return &OutboundTaskHistoryImporter{archive: archive, uow: uow, writer: writer, journal: journal, sourceHMACKey: append([]byte(nil), sourceHMACKey...)}, nil
}

// Import records archived rows as immutable observations. A legacy source ID
// is never a V2 task ID, and this path intentionally has no batch, event,
// queue, Provider, or broadcast-job write.
func (importer *OutboundTaskHistoryImporter) Import(ctx context.Context, archiveRunID string) (OutboundTaskHistoryImportResult, error) {
	if importer == nil || ctx == nil || importer.archive == nil || importer.uow == nil || importer.writer == nil || importer.journal == nil {
		return OutboundTaskHistoryImportResult{}, ErrInvalidScope
	}
	if err := importer.journal.ValidateOutboundTaskHistoryImportScope(archiveRunID); err != nil {
		return OutboundTaskHistoryImportResult{}, err
	}

	rows := make([]v1archive.ArchivedRow, 0)
	seenKeys := map[[sha256.Size]byte]struct{}{}
	seenIDs := map[int64]struct{}{}
	expectedOrdinal := int64(1)
	if err := importer.archive.EachTableRow(ctx, archiveRunID, v1outboundtaskhistory.OutboundTasksTableID, func(row v1archive.ArchivedRow) error {
		sourceID, valid := validOutboundTaskHistoryArchiveRow(row, expectedOrdinal, importer.sourceHMACKey)
		if !valid {
			return ErrConflict
		}
		expectedOrdinal++
		if _, exists := seenKeys[row.SourceKeyHMAC]; exists {
			return ErrConflict
		}
		seenKeys[row.SourceKeyHMAC] = struct{}{}
		if _, exists := seenIDs[sourceID]; exists {
			return ErrConflict
		}
		seenIDs[sourceID] = struct{}{}
		rows = append(rows, row)
		return nil
	}); err != nil {
		return OutboundTaskHistoryImportResult{}, err
	}

	history := v1outboundtaskhistory.AdaptHistory(rows)
	if history.SourceCount() != len(rows) || len(history.Tasks) != len(rows) {
		return OutboundTaskHistoryImportResult{}, ErrConflict
	}
	result := OutboundTaskHistoryImportResult{}
	for index, decision := range history.Tasks {
		row := rows[index]
		imported, replayed := false, false
		if err := importer.uow.Within(ctx, func(tx context.Context) error {
			// A transaction callback may retry; only the final committed attempt
			// contributes to the observable counters.
			imported, replayed = false, false
			if decision.Disposition != v1outboundtaskhistory.DispositionCandidate || decision.Fact == nil {
				var err error
				replayed, err = recordOutboundTaskHistoryQuarantine(tx, importer.journal, row, outboundTaskHistoryReason(decision.Reason))
				return err
			}
			value, err := outboundTaskHistoryValue(row, *decision.Fact)
			if err != nil {
				return err
			}
			receipt, err := importer.writer.Import(tx, SourceIdentifier(row.SourceKeyHMAC), value)
			if errors.Is(err, outboundport.ErrOutboundTaskHistoryInvalid) {
				replayed, err = recordOutboundTaskHistoryQuarantine(tx, importer.journal, row, "outbound_task_history_target_invalid")
				return err
			}
			if err != nil {
				return err
			}
			if err = verifyOutboundTaskHistoryReceipt(tx, importer.journal, row, receipt); err != nil {
				return err
			}
			imported, replayed = true, receipt.Replayed
			return nil
		}); err != nil {
			return OutboundTaskHistoryImportResult{}, err
		}
		if imported {
			result.ImportedTasks++
		} else {
			result.Quarantined++
		}
		if replayed {
			result.Replayed++
		}
	}
	return result, nil
}

func validOutboundTaskHistoryArchiveRow(row v1archive.ArchivedRow, ordinal int64, sourceHMACKey []byte) (int64, bool) {
	if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != v1outboundtaskhistory.OutboundTasksTableID || row.SourceOrdinal != ordinal ||
		row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) || !json.Valid(row.Payload) || len(sourceHMACKey) < sha256.Size {
		return 0, false
	}
	var source struct {
		ID int64 `json:"id"`
	}
	if json.Unmarshal(row.Payload, &source) != nil {
		return 0, false
	}
	sourceKeyJSON, err := json.Marshal([]int64{source.ID})
	if err != nil {
		return 0, false
	}
	sourceKey, sourceErr := v1archive.SourceKeyHMAC(sourceHMACKey, "outbound_tasks", sourceKeyJSON)
	payload, payloadErr := v1archive.PayloadHMAC(sourceHMACKey, "outbound_tasks", row.Payload)
	fields, fieldsErr := v1archive.FieldHMAC(sourceHMACKey, "outbound_tasks", row.RedactedFields)
	if sourceErr != nil || payloadErr != nil || fieldsErr != nil || sourceKey != row.SourceKeyHMAC || payload != row.PayloadHMAC || fields != row.FieldHMAC {
		return 0, false
	}
	return source.ID, true
}

func outboundTaskHistoryValue(row v1archive.ArchivedRow, fact v1outboundtaskhistory.OutboundTaskHistoryFact) (outboundport.HistoricalOutboundTask, error) {
	if [sha256.Size]byte(fact.Source.SourceKeyDigest) != row.SourceKeyHMAC || [sha256.Size]byte(fact.Source.PayloadDigest) != row.PayloadHMAC || [sha256.Size]byte(fact.Source.FieldDigest) != row.FieldHMAC {
		return outboundport.HistoricalOutboundTask{}, ErrConflict
	}
	return outboundport.HistoricalOutboundTask{
		SourceID: fact.SourceID, TaskType: fact.TaskType, Status: fact.Status, CreatedAt: fact.CreatedAt,
		// BroadcastJobHistoryID intentionally remains nil. The owner writer is
		// the only layer allowed to prove a historical parent relation.
		BroadcastJobHistoryID: nil,
		RequestPayloadDigest:  [sha256.Size]byte(fact.RequestPayloadDigest), ResponsePayloadDigest: [sha256.Size]byte(fact.ResponsePayloadDigest),
		WeComTaskIDDigest: copyOutboundTaskHistoryDigest(fact.WeComTaskIDDigest), TraceIDDigest: [sha256.Size]byte(fact.TraceIDDigest),
		LegacyBroadcastJobID: cloneOutboundTaskHistoryInt64(fact.LegacyBroadcastJobID),
		SourceKeyDigest:      row.SourceKeyHMAC, SourcePayloadDigest: row.PayloadHMAC, SourceFieldDigest: row.FieldHMAC,
		RedactedRoots: append([]string(nil), fact.RedactedRoots...),
	}, nil
}

func copyOutboundTaskHistoryDigest(value *v1outboundtaskhistory.OpaqueDigest) *[sha256.Size]byte {
	if value == nil {
		return nil
	}
	copy := [sha256.Size]byte(*value)
	return &copy
}

func cloneOutboundTaskHistoryInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func recordOutboundTaskHistoryQuarantine(ctx context.Context, journal OutboundTaskHistoryImportJournal, row v1archive.ArchivedRow, reason string) (bool, error) {
	if journal == nil || reason == "" {
		return false, ErrInvalidScope
	}
	source := SourceIdentifier(row.SourceKeyHMAC)
	existing, found, err := journal.LoadTerminal(ctx, source)
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
	return false, journal.Record(ctx, TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: "quarantine", Reason: reason})
}

func verifyOutboundTaskHistoryReceipt(ctx context.Context, journal OutboundTaskHistoryImportJournal, row v1archive.ArchivedRow, receipt outboundport.OutboundTaskHistoryReceipt) error {
	if journal == nil || receipt.SourceIdentifier != SourceIdentifier(row.SourceKeyHMAC) || receipt.PayloadDigest != row.PayloadHMAC || receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	terminal, found, err := journal.LoadTerminal(ctx, receipt.SourceIdentifier)
	if err != nil {
		return err
	}
	if !found || terminal.SourceKeyDigest != row.SourceKeyHMAC || terminal.PayloadDigest != row.PayloadHMAC || terminal.Disposition != "import" || terminal.Reason != "" ||
		terminal.TargetID != strconv.FormatInt(receipt.TargetID, 10) || terminal.TargetDigest != receipt.TargetDigest || len(terminal.Metadata) != 0 {
		return ErrConflict
	}
	return nil
}

func outboundTaskHistoryReason(reason string) string {
	if reason == "" {
		return "invalid_outbound_task_history"
	}
	return reason
}
