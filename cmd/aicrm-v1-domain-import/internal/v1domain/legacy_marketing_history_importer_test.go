package v1domain

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

type legacyMarketingImporterArchive struct {
	rows map[string][]v1archive.ArchivedRow
}

func (archive legacyMarketingImporterArchive) EachTableRow(ctx context.Context, run, table string, callback func(v1archive.ArchivedRow) error) error {
	if ctx == nil || run != "archive-run" {
		return ErrInvalidScope
	}
	for _, row := range archive.rows[table] {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type legacyMarketingImporterTxKey struct{}
type legacyMarketingImporterUOW struct{}

func (legacyMarketingImporterUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(context.WithValue(ctx, legacyMarketingImporterTxKey{}, true))
}

type legacyMarketingImporterJournal struct{ terminals map[string]TerminalReceipt }

func newLegacyMarketingImporterJournal() *legacyMarketingImporterJournal {
	return &legacyMarketingImporterJournal{terminals: map[string]TerminalReceipt{}}
}

func legacyMarketingImporterJournalKey(kind, source string) string { return kind + "/" + source }

func (journal *legacyMarketingImporterJournal) ValidateLegacyMarketingHistoryImportScope(run string) error {
	if journal == nil || run != "archive-run" {
		return ErrInvalidScope
	}
	return nil
}

func (journal *legacyMarketingImporterJournal) LoadLegacyMarketingHistoryTerminal(ctx context.Context, kind, source string) (TerminalReceipt, bool, error) {
	if ctx.Value(legacyMarketingImporterTxKey{}) != true {
		return TerminalReceipt{}, false, errors.New("journal outside transaction")
	}
	value, found := journal.terminals[legacyMarketingImporterJournalKey(kind, source)]
	return value, found, nil
}

func (journal *legacyMarketingImporterJournal) RecordLegacyMarketingHistoryTerminal(ctx context.Context, kind string, receipt TerminalReceipt) error {
	if ctx.Value(legacyMarketingImporterTxKey{}) != true {
		return errors.New("journal outside transaction")
	}
	key := legacyMarketingImporterJournalKey(kind, SourceIdentifier(receipt.SourceKeyDigest))
	if current, found := journal.terminals[key]; found && !reflect.DeepEqual(current, receipt) {
		return ErrConflict
	}
	journal.terminals[key] = receipt
	return nil
}

func (journal *legacyMarketingImporterJournal) LoadLegacyMarketingHistory(ctx context.Context, kind, source string) (segmentport.LegacyMarketingHistoryReceipt, bool, error) {
	terminal, found, err := journal.LoadLegacyMarketingHistoryTerminal(ctx, kind, source)
	if err != nil || !found {
		return segmentport.LegacyMarketingHistoryReceipt{}, found, err
	}
	if terminal.Disposition != "import" || terminal.Reason != "" || terminal.TargetID == "" || terminal.TargetDigest == ([sha256.Size]byte{}) || len(terminal.Metadata) != 0 {
		return segmentport.LegacyMarketingHistoryReceipt{}, false, ErrConflict
	}
	id, err := strconv.ParseInt(terminal.TargetID, 10, 64)
	if err != nil || id < 1 {
		return segmentport.LegacyMarketingHistoryReceipt{}, false, ErrConflict
	}
	return segmentport.LegacyMarketingHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetID: id, TargetDigest: terminal.TargetDigest}, true, nil
}

func (journal *legacyMarketingImporterJournal) RecordLegacyMarketingHistory(ctx context.Context, receipt segmentport.LegacyMarketingHistoryReceipt) error {
	if receipt.Replayed || receipt.TargetID < 1 || receipt.PayloadDigest == ([sha256.Size]byte{}) || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	key, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil {
		return err
	}
	return journal.RecordLegacyMarketingHistoryTerminal(ctx, receipt.Kind, TerminalReceipt{SourceKeyDigest: key, PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest})
}

type legacyMarketingImporterWriter struct {
	journal             *legacyMarketingImporterJournal
	next                int64
	writes              map[string]int
	state               segmentport.HistoricalLegacyMarketingState
	value               segmentport.HistoricalLegacyMarketingValue
	invalid, badReceipt bool
}

func newLegacyMarketingImporterWriter(journal *legacyMarketingImporterJournal) *legacyMarketingImporterWriter {
	return &legacyMarketingImporterWriter{journal: journal, next: 300, writes: map[string]int{}}
}

func (writer *legacyMarketingImporterWriter) ImportLegacyMarketingState(ctx context.Context, source string, value segmentport.HistoricalLegacyMarketingState) (segmentport.LegacyMarketingHistoryReceipt, error) {
	writer.state = value
	return writer.write(ctx, legacyMarketingStateKind, source, value.SourcePayloadDigest)
}

func (writer *legacyMarketingImporterWriter) ImportLegacyMarketingValue(ctx context.Context, source string, value segmentport.HistoricalLegacyMarketingValue) (segmentport.LegacyMarketingHistoryReceipt, error) {
	writer.value = value
	return writer.write(ctx, legacyMarketingValueKind, source, value.SourcePayloadDigest)
}

func (writer *legacyMarketingImporterWriter) write(ctx context.Context, kind, source string, payload [sha256.Size]byte) (segmentport.LegacyMarketingHistoryReceipt, error) {
	if ctx.Value(legacyMarketingImporterTxKey{}) != true {
		return segmentport.LegacyMarketingHistoryReceipt{}, errors.New("writer outside transaction")
	}
	if writer.invalid {
		return segmentport.LegacyMarketingHistoryReceipt{}, segmentport.ErrLegacyMarketingHistoryInvalid
	}
	if existing, found, err := writer.journal.LoadLegacyMarketingHistory(ctx, kind, source); err != nil || found {
		if err != nil {
			return segmentport.LegacyMarketingHistoryReceipt{}, err
		}
		existing.Replayed = true
		return existing, nil
	}
	writer.next++
	writer.writes[kind]++
	receipt := segmentport.LegacyMarketingHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetID: writer.next, TargetDigest: sha256.Sum256([]byte(kind + "/" + source))}
	if err := writer.journal.RecordLegacyMarketingHistory(ctx, receipt); err != nil {
		return segmentport.LegacyMarketingHistoryReceipt{}, err
	}
	if writer.badReceipt {
		receipt.TargetID++
	}
	return receipt, nil
}

func TestLegacyMarketingHistoryImporterImportsAndReplaysSourceFacts(t *testing.T) {
	key := makeLegacyMarketingImporterKey(64)
	stamp := time.Date(2026, 8, 28, 9, 10, 11, 123456789, time.FixedZone("source", 8*60*60))
	stateRow := legacyMarketingImporterRow(t, key, legacyMarketingStateTable, 1, 1, legacyMarketingImporterStatePayload(stamp))
	valueRow := legacyMarketingImporterRow(t, key, legacyMarketingValueTable, 1, 2, legacyMarketingImporterValuePayload(stamp))
	journal := newLegacyMarketingImporterJournal()
	writer := newLegacyMarketingImporterWriter(journal)
	importer, err := NewLegacyMarketingHistoryImporter(legacyMarketingImporterArchive{rows: map[string][]v1archive.ArchivedRow{
		legacyMarketingStateTable: {stateRow}, legacyMarketingValueTable: {valueRow},
	}}, legacyMarketingImporterUOW{}, writer, journal, key)
	if err != nil {
		t.Fatal(err)
	}

	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (LegacyMarketingHistoryImportResult{ImportedStates: 1, ImportedValues: 1}) {
		t.Fatalf("first import=%+v err=%v", result, err)
	}
	if writer.state.SourceID != -7 || writer.state.LastBatchSourceID == nil || *writer.state.LastBatchSourceID != -77 || writer.value.SourceID != -9 || writer.value.Score != -12 ||
		writer.state.EnteredAt != nil || writer.state.ExitedAt != nil || writer.state.CreatedAt.Location() != time.UTC || writer.state.CreatedAt.Nanosecond() != 123456000 {
		t.Fatal("source scalar, nullable reference, or timestamp fidelity changed")
	}
	if writer.state.ExternalUserIDDigest != legacyMarketingExpectedPrivateDigest(key, legacyMarketingStateTable, "external_userid", []byte("private-external")) ||
		writer.value.ScoreBreakdownDigest != legacyMarketingExpectedPrivateDigest(key, legacyMarketingValueTable, "score_breakdown_json", []byte(`{"score":-12}`)) ||
		writer.state.ExternalUserIDDigest == writer.value.ExternalUserIDDigest {
		t.Fatal("private digest domain separation changed")
	}
	if writer.state.SourceKeyDigest != stateRow.SourceKeyHMAC || writer.state.SourcePayloadDigest != stateRow.PayloadHMAC || writer.state.SourceFieldDigest != stateRow.FieldHMAC ||
		writer.value.SourceKeyDigest != valueRow.SourceKeyHMAC || writer.value.SourcePayloadDigest != valueRow.PayloadHMAC || writer.value.SourceFieldDigest != valueRow.FieldHMAC {
		t.Fatal("archive envelope changed")
	}

	replayed, err := importer.Import(context.Background(), "archive-run")
	if err != nil || replayed != (LegacyMarketingHistoryImportResult{ImportedStates: 1, ImportedValues: 1, Replayed: 2}) {
		t.Fatalf("replay=%+v err=%v", replayed, err)
	}
	if writer.writes[legacyMarketingStateKind] != 1 || writer.writes[legacyMarketingValueKind] != 1 {
		t.Fatal("replay wrote a second target")
	}
}

func TestLegacyMarketingHistoryImporterQuarantinesCandidateAndOwnerFailures(t *testing.T) {
	key := makeLegacyMarketingImporterKey(32)
	stamp := time.Date(2026, 8, 28, 9, 10, 11, 0, time.UTC)
	row := legacyMarketingImporterRow(t, key, legacyMarketingStateTable, 1, 1, legacyMarketingImporterStatePayload(stamp))
	row.RedactedFields = []string{"external_userid"}
	fieldHMAC, err := v1archive.FieldHMAC(key, "marketing_state_current", row.RedactedFields)
	if err != nil {
		t.Fatal(err)
	}
	row.FieldHMAC = fieldHMAC
	journal := newLegacyMarketingImporterJournal()
	writer := newLegacyMarketingImporterWriter(journal)
	importer, err := NewLegacyMarketingHistoryImporter(legacyMarketingImporterArchive{rows: map[string][]v1archive.ArchivedRow{legacyMarketingStateTable: {row}}}, legacyMarketingImporterUOW{}, writer, journal, key)
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (LegacyMarketingHistoryImportResult{Quarantined: 1}) || len(writer.writes) != 0 {
		t.Fatalf("redacted source result=%+v writes=%v err=%v", result, writer.writes, err)
	}

	row = legacyMarketingImporterRow(t, key, legacyMarketingStateTable, 1, 2, legacyMarketingImporterStatePayload(stamp))
	journal = newLegacyMarketingImporterJournal()
	writer = newLegacyMarketingImporterWriter(journal)
	writer.invalid = true
	importer, err = NewLegacyMarketingHistoryImporter(legacyMarketingImporterArchive{rows: map[string][]v1archive.ArchivedRow{legacyMarketingStateTable: {row}}}, legacyMarketingImporterUOW{}, writer, journal, key)
	if err != nil {
		t.Fatal(err)
	}
	result, err = importer.Import(context.Background(), "archive-run")
	if err != nil || result != (LegacyMarketingHistoryImportResult{Quarantined: 1}) {
		t.Fatalf("owner invalid result=%+v err=%v", result, err)
	}
}

func TestLegacyMarketingHistoryImporterRejectsArchiveOrReceiptDrift(t *testing.T) {
	key := makeLegacyMarketingImporterKey(32)
	stamp := time.Date(2026, 8, 28, 9, 10, 11, 0, time.UTC)
	row := legacyMarketingImporterRow(t, key, legacyMarketingStateTable, 1, 1, legacyMarketingImporterStatePayload(stamp))
	row.PayloadHMAC = [sha256.Size]byte{}
	journal := newLegacyMarketingImporterJournal()
	writer := newLegacyMarketingImporterWriter(journal)
	importer, err := NewLegacyMarketingHistoryImporter(legacyMarketingImporterArchive{rows: map[string][]v1archive.ArchivedRow{legacyMarketingStateTable: {row}}}, legacyMarketingImporterUOW{}, writer, journal, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) || len(writer.writes) != 0 {
		t.Fatalf("invalid archive err=%v writes=%v", err, writer.writes)
	}

	row = legacyMarketingImporterRow(t, key, legacyMarketingStateTable, 1, 2, legacyMarketingImporterStatePayload(stamp))
	journal = newLegacyMarketingImporterJournal()
	writer = newLegacyMarketingImporterWriter(journal)
	writer.badReceipt = true
	importer, err = NewLegacyMarketingHistoryImporter(legacyMarketingImporterArchive{rows: map[string][]v1archive.ArchivedRow{legacyMarketingStateTable: {row}}}, legacyMarketingImporterUOW{}, writer, journal, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) {
		t.Fatalf("receipt drift err=%v", err)
	}
}

func TestNewLegacyMarketingHistoryImporterAcceptsArchiveKeyLengthsAtLeastSHA256(t *testing.T) {
	for _, length := range []int{sha256.Size, 64} {
		journal := newLegacyMarketingImporterJournal()
		if _, err := NewLegacyMarketingHistoryImporter(legacyMarketingImporterArchive{}, legacyMarketingImporterUOW{}, newLegacyMarketingImporterWriter(journal), journal, makeLegacyMarketingImporterKey(length)); err != nil {
			t.Fatalf("key length %d error=%v", length, err)
		}
	}
	journal := newLegacyMarketingImporterJournal()
	if _, err := NewLegacyMarketingHistoryImporter(legacyMarketingImporterArchive{}, legacyMarketingImporterUOW{}, newLegacyMarketingImporterWriter(journal), journal, makeLegacyMarketingImporterKey(sha256.Size-1)); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("short key error=%v", err)
	}
}

func legacyMarketingImporterStatePayload(stamp time.Time) map[string]any {
	return map[string]any{
		"id": -7, "scenario_key": "legacy-scenario", "external_userid": "private-external", "marketing_phase": "phase", "phase_label": "label", "phase_reason": "reason",
		"lifecycle_status": "active", "last_batch_id": -77, "last_batch_status": "done", "last_batch_window_start": "civil-start", "last_batch_window_end": "civil-end",
		"last_trigger_message_at": "civil-trigger", "entered_at": nil, "exited_at": nil, "exit_reason": "", "source_payload_json": json.RawMessage(`{"source":true}`),
		"created_at": stamp, "updated_at": stamp,
	}
}

func legacyMarketingImporterValuePayload(stamp time.Time) map[string]any {
	return map[string]any{
		"id": -9, "scenario_key": "legacy-scenario", "external_userid": "private-external", "value_segment": "value", "segment_label": "label", "score": -12,
		"score_breakdown_json": json.RawMessage(`{"score":-12}`), "source_payload_json": json.RawMessage(`{"source":true}`), "created_at": stamp, "updated_at": stamp,
	}
}

func legacyMarketingImporterRow(t *testing.T, key []byte, table string, ordinal, seed int64, value any) v1archive.ArchivedRow {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	payloadHMAC, err := v1archive.PayloadHMAC(key, table[len("public/"):], payload)
	if err != nil {
		t.Fatal(err)
	}
	fieldHMAC, err := v1archive.FieldHMAC(key, table[len("public/"):], nil)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal,
		SourceKeyHMAC: sha256.Sum256([]byte(table + "/source/" + strconv.FormatInt(seed, 10))), PayloadHMAC: payloadHMAC, FieldHMAC: fieldHMAC, Payload: payload}
}

func makeLegacyMarketingImporterKey(length int) []byte {
	return make([]byte, length)
}

func legacyMarketingExpectedPrivateDigest(key []byte, table, field string, value []byte) [sha256.Size]byte {
	mac := hmac.New(sha256.New, key)
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
