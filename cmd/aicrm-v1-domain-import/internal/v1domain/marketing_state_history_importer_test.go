package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

type marketingStateArchiveFake struct {
	rows map[string][]v1archive.ArchivedRow
}

func (source marketingStateArchiveFake) EachTableRow(ctx context.Context, run, table string, each func(v1archive.ArchivedRow) error) error {
	if ctx == nil || run != "archive-run" {
		return ErrInvalidScope
	}
	for _, row := range source.rows[table] {
		if err := each(row); err != nil {
			return err
		}
	}
	return nil
}

type marketingStateTxKey struct{}
type marketingStateUOWFake struct{}

func (marketingStateUOWFake) Within(ctx context.Context, fn func(context.Context) error) error {
	return fn(context.WithValue(ctx, marketingStateTxKey{}, true))
}

type marketingStateJournalFake struct{ terminals map[string]TerminalReceipt }

func newMarketingStateJournalFake() *marketingStateJournalFake {
	return &marketingStateJournalFake{terminals: map[string]TerminalReceipt{}}
}
func marketingStateJournalKey(kind, source string) string { return kind + "/" + source }
func (journal *marketingStateJournalFake) ValidateMarketingStateHistoryImportScope(run string) error {
	if journal == nil || run != "archive-run" {
		return ErrInvalidScope
	}
	return nil
}
func (journal *marketingStateJournalFake) LoadMarketingStateHistoryTerminal(_ context.Context, kind, source string) (TerminalReceipt, bool, error) {
	value, found := journal.terminals[marketingStateJournalKey(kind, source)]
	return value, found, nil
}
func (journal *marketingStateJournalFake) RecordMarketingStateHistoryTerminal(_ context.Context, kind string, receipt TerminalReceipt) error {
	key := marketingStateJournalKey(kind, SourceIdentifier(receipt.SourceKeyDigest))
	if existing, found := journal.terminals[key]; found && (existing.SourceKeyDigest != receipt.SourceKeyDigest || existing.PayloadDigest != receipt.PayloadDigest || existing.Disposition != receipt.Disposition || existing.Reason != receipt.Reason || existing.TargetID != receipt.TargetID || existing.TargetDigest != receipt.TargetDigest) {
		return ErrConflict
	}
	journal.terminals[key] = receipt
	return nil
}
func (journal *marketingStateJournalFake) LoadMarketingStateHistory(ctx context.Context, kind, source string) (segmentport.MarketingStateHistoryReceipt, bool, error) {
	terminal, found, err := journal.LoadMarketingStateHistoryTerminal(ctx, kind, source)
	if err != nil || !found {
		return segmentport.MarketingStateHistoryReceipt{}, found, err
	}
	if terminal.Disposition != "import" || terminal.TargetID == "" {
		return segmentport.MarketingStateHistoryReceipt{}, false, ErrConflict
	}
	id, err := strconv.ParseInt(terminal.TargetID, 10, 64)
	if err != nil {
		return segmentport.MarketingStateHistoryReceipt{}, false, err
	}
	return segmentport.MarketingStateHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetID: id, TargetDigest: terminal.TargetDigest}, true, nil
}
func (journal *marketingStateJournalFake) RecordMarketingStateHistory(ctx context.Context, receipt segmentport.MarketingStateHistoryReceipt) error {
	if ctx.Value(marketingStateTxKey{}) != true || receipt.Replayed || receipt.TargetID < 1 {
		return ErrConflict
	}
	key, _ := ParseSourceIdentifier(receipt.SourceIdentifier)
	return journal.RecordMarketingStateHistoryTerminal(ctx, receipt.Kind, TerminalReceipt{SourceKeyDigest: key, PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest})
}

type marketingStateWriterFake struct {
	journal                   *marketingStateJournalFake
	next                      int64
	writes                    map[string]int
	snapshot                  segmentport.HistoricalMarketingStateSnapshot
	change                    segmentport.HistoricalMarketingStateChange
	valueSnapshot             segmentport.HistoricalValueSegmentSnapshot
	valueChange               segmentport.HistoricalValueSegmentChange
	returnInvalid, badReceipt bool
}

func newMarketingStateWriterFake(journal *marketingStateJournalFake) *marketingStateWriterFake {
	return &marketingStateWriterFake{journal: journal, next: 100, writes: map[string]int{}}
}
func (writer *marketingStateWriterFake) ImportMarketingStateSnapshot(ctx context.Context, source string, value segmentport.HistoricalMarketingStateSnapshot) (segmentport.MarketingStateHistoryReceipt, error) {
	writer.snapshot = value
	return writer.write(ctx, marketingStateSnapshotKind, source, value.SourcePayloadDigest)
}
func (writer *marketingStateWriterFake) ImportMarketingStateChange(ctx context.Context, source string, value segmentport.HistoricalMarketingStateChange) (segmentport.MarketingStateHistoryReceipt, error) {
	writer.change = value
	return writer.write(ctx, marketingStateChangeKind, source, value.SourcePayloadDigest)
}
func (writer *marketingStateWriterFake) ImportValueSegmentSnapshot(ctx context.Context, source string, value segmentport.HistoricalValueSegmentSnapshot) (segmentport.MarketingStateHistoryReceipt, error) {
	writer.valueSnapshot = value
	return writer.write(ctx, valueSegmentSnapshotKind, source, value.SourcePayloadDigest)
}
func (writer *marketingStateWriterFake) ImportValueSegmentChange(ctx context.Context, source string, value segmentport.HistoricalValueSegmentChange) (segmentport.MarketingStateHistoryReceipt, error) {
	writer.valueChange = value
	return writer.write(ctx, valueSegmentChangeKind, source, value.SourcePayloadDigest)
}
func (writer *marketingStateWriterFake) write(ctx context.Context, kind, source string, payload [sha256.Size]byte) (segmentport.MarketingStateHistoryReceipt, error) {
	if ctx.Value(marketingStateTxKey{}) != true {
		return segmentport.MarketingStateHistoryReceipt{}, errors.New("missing caller transaction")
	}
	if writer.returnInvalid {
		return segmentport.MarketingStateHistoryReceipt{}, segmentport.ErrMarketingStateHistoryInvalid
	}
	if existing, found, err := writer.journal.LoadMarketingStateHistory(ctx, kind, source); err != nil || found {
		if err != nil {
			return segmentport.MarketingStateHistoryReceipt{}, err
		}
		existing.Replayed = true
		return existing, nil
	}
	writer.next++
	writer.writes[kind]++
	digest := sha256.Sum256([]byte(kind + "/" + source))
	receipt := segmentport.MarketingStateHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetID: writer.next, TargetDigest: digest}
	if err := writer.journal.RecordMarketingStateHistory(ctx, receipt); err != nil {
		return segmentport.MarketingStateHistoryReceipt{}, err
	}
	if writer.badReceipt {
		receipt.TargetID++
	}
	return receipt, nil
}

func TestMarketingStateHistoryImporterImportsFourFactsAndReplays(t *testing.T) {
	archive := marketingStateArchiveFake{rows: map[string][]v1archive.ArchivedRow{
		marketingStateSnapshotTable: {marketingStateRow(marketingStateSnapshotTable, 1, 1, marketingSnapshotPayload(-1))},
		marketingStateChangeTable:   {marketingStateRow(marketingStateChangeTable, 1, 2, marketingChangePayload(0))},
		valueSegmentSnapshotTable:   {marketingStateRow(valueSegmentSnapshotTable, 1, 3, valueSnapshotPayload(-2, -7, 4))},
		valueSegmentChangeTable:     {marketingStateRow(valueSegmentChangeTable, 1, 4, valueChangePayload(3, 7, -4))},
	}}
	journal := newMarketingStateJournalFake()
	writer := newMarketingStateWriterFake(journal)
	importer, err := NewMarketingStateHistoryImporter(archive, marketingStateUOWFake{}, writer, journal)
	if err != nil {
		t.Fatal(err)
	}
	got, err := importer.Import(context.Background(), "archive-run")
	if err != nil || got != (MarketingStateHistoryImportResult{ImportedMarketingStateSnapshots: 1, ImportedMarketingStateChanges: 1, ImportedValueSegmentSnapshots: 1, ImportedValueSegmentChanges: 1}) {
		t.Fatalf("first import=%+v err=%v", got, err)
	}
	if writer.snapshot.SourceID != -1 || writer.change.SourceID != 0 || writer.valueSnapshot.SourceID != -2 || writer.valueSnapshot.SegmentRank != -7 || writer.valueSnapshot.Score != 4 || writer.valueChange.Score != -4 ||
		writer.snapshot.SourceFieldDigest == ([sha256.Size]byte{}) || writer.valueSnapshot.StatePayloadDigest == writer.valueSnapshot.SourcePayloadDigest {
		t.Fatalf("source fidelity lost: snapshot=%+v change=%+v value_snapshot=%+v value_change=%+v", writer.snapshot, writer.change, writer.valueSnapshot, writer.valueChange)
	}
	replayed, err := importer.Import(context.Background(), "archive-run")
	if err != nil || replayed != (MarketingStateHistoryImportResult{ImportedMarketingStateSnapshots: 1, ImportedMarketingStateChanges: 1, ImportedValueSegmentSnapshots: 1, ImportedValueSegmentChanges: 1, Replayed: 4}) {
		t.Fatalf("replay=%+v err=%v", replayed, err)
	}
	for _, kind := range []string{marketingStateSnapshotKind, marketingStateChangeKind, valueSegmentSnapshotKind, valueSegmentChangeKind} {
		if writer.writes[kind] != 1 {
			t.Fatalf("replay wrote %s: %v", kind, writer.writes)
		}
	}
}

func TestMarketingStateHistoryImporterQuarantinesOwnerInvalidAndOutOfRangeValue(t *testing.T) {
	row := marketingStateRow(marketingStateSnapshotTable, 1, 1, marketingSnapshotPayload(1))
	journal := newMarketingStateJournalFake()
	writer := newMarketingStateWriterFake(journal)
	writer.returnInvalid = true
	importer, err := NewMarketingStateHistoryImporter(marketingStateArchiveFake{rows: map[string][]v1archive.ArchivedRow{marketingStateSnapshotTable: {row}}}, marketingStateUOWFake{}, writer, journal)
	if err != nil {
		t.Fatal(err)
	}
	got, err := importer.Import(context.Background(), "archive-run")
	if err != nil || got.Quarantined != 1 || got.ImportedMarketingStateSnapshots != 0 {
		t.Fatalf("owner invalid result=%+v err=%v", got, err)
	}

	row = marketingStateRow(valueSegmentSnapshotTable, 1, 2, valueSnapshotPayload(1, int64(1<<31), 0))
	journal = newMarketingStateJournalFake()
	writer = newMarketingStateWriterFake(journal)
	importer, err = NewMarketingStateHistoryImporter(marketingStateArchiveFake{rows: map[string][]v1archive.ArchivedRow{valueSegmentSnapshotTable: {row}}}, marketingStateUOWFake{}, writer, journal)
	if err != nil {
		t.Fatal(err)
	}
	got, err = importer.Import(context.Background(), "archive-run")
	if err != nil || got.Quarantined != 1 || len(writer.writes) != 0 {
		t.Fatalf("out of range result=%+v writes=%v err=%v", got, writer.writes, err)
	}

	row = marketingStateRow(marketingStateSnapshotTable, 1, 3, marketingSnapshotPayload(1))
	journal = newMarketingStateJournalFake()
	writer = newMarketingStateWriterFake(journal)
	writer.badReceipt = true
	importer, err = NewMarketingStateHistoryImporter(marketingStateArchiveFake{rows: map[string][]v1archive.ArchivedRow{marketingStateSnapshotTable: {row}}}, marketingStateUOWFake{}, writer, journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) {
		t.Fatalf("receipt drift err=%v", err)
	}
}

func TestMarketingStateInt32Boundaries(t *testing.T) {
	for _, value := range []int64{-(1 << 31), 1<<31 - 1} {
		if got := marketingStateInt32(value); got == nil || int64(*got) != value {
			t.Fatalf("boundary %d=%v", value, got)
		}
	}
	for _, value := range []int64{-(1 << 31) - 1, 1 << 31} {
		if got := marketingStateInt32(value); got != nil {
			t.Fatalf("out of range %d=%v", value, *got)
		}
	}
}

func TestMarketingStateHistoryImporterQuarantinesRedactedCandidateAndRejectsBadArchive(t *testing.T) {
	row := marketingStateRow(marketingStateSnapshotTable, 1, 1, marketingSnapshotPayload(1))
	row.RedactedFields = []string{"external_userid"}
	journal := newMarketingStateJournalFake()
	writer := newMarketingStateWriterFake(journal)
	importer, err := NewMarketingStateHistoryImporter(marketingStateArchiveFake{rows: map[string][]v1archive.ArchivedRow{marketingStateSnapshotTable: {row}}}, marketingStateUOWFake{}, writer, journal)
	if err != nil {
		t.Fatal(err)
	}
	got, err := importer.Import(context.Background(), "archive-run")
	if err != nil || got.Quarantined != 1 || len(writer.writes) != 0 {
		t.Fatalf("redacted result=%+v writes=%v err=%v", got, writer.writes, err)
	}

	row = marketingStateRow(marketingStateSnapshotTable, 1, 2, marketingSnapshotPayload(1))
	row.FieldHMAC = [sha256.Size]byte{}
	journal = newMarketingStateJournalFake()
	writer = newMarketingStateWriterFake(journal)
	importer, err = NewMarketingStateHistoryImporter(marketingStateArchiveFake{rows: map[string][]v1archive.ArchivedRow{marketingStateSnapshotTable: {row}}}, marketingStateUOWFake{}, writer, journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) {
		t.Fatalf("unsigned archive err=%v", err)
	}
}

func marketingStateRow(table string, ordinal, seed int64, payload []byte) v1archive.ArchivedRow {
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal, SourceKeyHMAC: sha256.Sum256([]byte(fmt.Sprintf("%s/key/%d", table, seed))), PayloadHMAC: sha256.Sum256(payload), FieldHMAC: sha256.Sum256([]byte(fmt.Sprintf("%s/field/%d", table, seed))), Payload: payload}
}

func marketingSnapshotPayload(id int64) []byte {
	return marketingPayload(map[string]any{"id": id, "person_id": nil, "external_userid": "external", "automation_key": "a", "main_stage": "main", "sub_stage": "sub", "activated": false, "converted": true, "eligible_for_conversion": false, "lifecycle_status": "paused", "last_activation_at": "", "last_conversion_marked_at": "", "last_message_at": "", "last_batch_id": nil, "last_batch_status": "", "last_batch_window_start": "", "last_batch_window_end": "", "last_trigger_message_at": "", "entered_at": nil, "exited_at": nil, "exit_reason": "", "state_payload_json": map[string]any{"flag": false}, "created_at": marketingStateTime(), "updated_at": marketingStateTime()})
}
func marketingChangePayload(id int64) []byte {
	return marketingPayload(map[string]any{"id": id, "person_id": nil, "external_userid": "external", "automation_key": "a", "main_stage": "main", "sub_stage": "sub", "activated": false, "converted": false, "eligible_for_conversion": true, "batch_id": nil, "lifecycle_status": "paused", "exit_reason": "", "last_activation_at": "", "last_conversion_marked_at": "", "last_message_at": "", "change_reason": "", "state_payload_json": map[string]any{"flag": false}, "recorded_at": marketingStateTime(), "created_at": marketingStateTime()})
}
func valueSnapshotPayload(id, rank, score int64) []byte {
	return marketingPayload(map[string]any{"id": id, "external_userid": "external", "segment": "A", "segment_rank": rank, "score": score, "scoring_version": "v1", "computed_reason": "", "submission_id": nil, "matched_question_ids_json": []any{}, "source_payload_json": map[string]any{"source": "v1"}, "evaluated_at": marketingStateTime(), "computed_at": marketingStateTime(), "created_at": marketingStateTime(), "updated_at": marketingStateTime()})
}
func valueChangePayload(id, rank, score int64) []byte {
	return marketingPayload(map[string]any{"id": id, "external_userid": "external", "segment": "A", "segment_rank": rank, "score": score, "scoring_version": "v1", "change_reason": "", "submission_id": nil, "matched_question_ids_json": []any{}, "source_payload_json": map[string]any{"source": "v1"}, "evaluated_at": marketingStateTime(), "recorded_at": marketingStateTime(), "created_at": marketingStateTime()})
}
func marketingPayload(value map[string]any) []byte {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return payload
}
func marketingStateTime() string {
	return time.Date(2026, 8, 28, 1, 2, 3, 456000000, time.UTC).Format(time.RFC3339Nano)
}
