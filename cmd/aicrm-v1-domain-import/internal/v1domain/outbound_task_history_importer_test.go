package v1domain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1outboundtaskhistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

type outboundTaskHistoryArchiveFake struct{ rows []v1archive.ArchivedRow }

func (fake *outboundTaskHistoryArchiveFake) EachTableRow(_ context.Context, run, table string, callback func(v1archive.ArchivedRow) error) error {
	if run != "archive-run" || table != v1outboundtaskhistory.OutboundTasksTableID {
		return ErrInvalidScope
	}
	for _, row := range fake.rows {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type outboundTaskHistoryTxKey struct{}

var outboundTaskHistoryHMACKey = bytes.Repeat([]byte{7}, sha256.Size)

type outboundTaskHistoryRuntimeFake struct {
	terminals map[string]TerminalReceipt
	values    map[int64]outboundport.HistoricalOutboundTask
	writeErr  error
	calls     int
	checks    int
	commits   int
	retryOnce bool
}

func newOutboundTaskHistoryRuntimeFake() *outboundTaskHistoryRuntimeFake {
	return &outboundTaskHistoryRuntimeFake{terminals: map[string]TerminalReceipt{}, values: map[int64]outboundport.HistoricalOutboundTask{}}
}

func (fake *outboundTaskHistoryRuntimeFake) Within(ctx context.Context, callback func(context.Context) error) error {
	for attempt := 0; ; attempt++ {
		terminals, values := copyOutboundTaskHistoryTerminals(fake.terminals), copyOutboundTaskHistoryValues(fake.values)
		if err := callback(context.WithValue(ctx, outboundTaskHistoryTxKey{}, true)); err != nil {
			fake.terminals, fake.values = terminals, values
			return err
		}
		if fake.retryOnce && attempt == 0 {
			fake.terminals, fake.values = terminals, values
			continue
		}
		fake.commits++
		return nil
	}
}

func (fake *outboundTaskHistoryRuntimeFake) ValidateOutboundTaskHistoryImportScope(run string) error {
	if run != "archive-run" {
		return ErrInvalidScope
	}
	return nil
}

func (fake *outboundTaskHistoryRuntimeFake) LoadTerminal(ctx context.Context, source string) (TerminalReceipt, bool, error) {
	if ctx.Value(outboundTaskHistoryTxKey{}) != true {
		return TerminalReceipt{}, false, errors.New("missing transaction")
	}
	value, found := fake.terminals[source]
	return value, found, nil
}

func (fake *outboundTaskHistoryRuntimeFake) Record(ctx context.Context, receipt TerminalReceipt) error {
	if ctx.Value(outboundTaskHistoryTxKey{}) != true {
		return errors.New("missing transaction")
	}
	key := SourceIdentifier(receipt.SourceKeyDigest)
	if old, found := fake.terminals[key]; found && !reflect.DeepEqual(old, receipt) {
		return ErrConflict
	}
	fake.terminals[key] = receipt
	return nil
}

func (fake *outboundTaskHistoryRuntimeFake) LoadOutboundTaskHistory(ctx context.Context, source string) (outboundport.OutboundTaskHistoryReceipt, bool, error) {
	terminal, found, err := fake.LoadTerminal(ctx, source)
	if err != nil || !found || terminal.Disposition != "import" {
		return outboundport.OutboundTaskHistoryReceipt{}, false, err
	}
	id, err := strconv.ParseInt(terminal.TargetID, 10, 64)
	if err != nil {
		return outboundport.OutboundTaskHistoryReceipt{}, false, err
	}
	return outboundport.OutboundTaskHistoryReceipt{SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetDigest: terminal.TargetDigest, TargetID: id}, true, nil
}

func (fake *outboundTaskHistoryRuntimeFake) RecordOutboundTaskHistory(ctx context.Context, receipt outboundport.OutboundTaskHistoryReceipt) error {
	key, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil {
		return err
	}
	return fake.Record(ctx, TerminalReceipt{SourceKeyDigest: key, PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest})
}

func (fake *outboundTaskHistoryRuntimeFake) Import(ctx context.Context, source string, value outboundport.HistoricalOutboundTask) (outboundport.OutboundTaskHistoryReceipt, error) {
	if ctx.Value(outboundTaskHistoryTxKey{}) != true {
		return outboundport.OutboundTaskHistoryReceipt{}, errors.New("writer outside transaction")
	}
	fake.calls++
	if fake.writeErr != nil {
		return outboundport.OutboundTaskHistoryReceipt{}, fake.writeErr
	}
	if existing, found, err := fake.LoadOutboundTaskHistory(ctx, source); err != nil {
		return outboundport.OutboundTaskHistoryReceipt{}, err
	} else if found {
		expected := value
		expected.ID = existing.TargetID
		actual, present := fake.values[existing.TargetID]
		if !present || !reflect.DeepEqual(actual, expected) {
			return outboundport.OutboundTaskHistoryReceipt{}, outboundport.ErrOutboundTaskHistoryConflict
		}
		fake.checks++
		existing.Replayed = true
		return existing, nil
	}
	value.ID = int64(100 + len(fake.values))
	fake.values[value.ID] = value
	receipt := outboundport.OutboundTaskHistoryReceipt{SourceIdentifier: source, PayloadDigest: value.SourcePayloadDigest, TargetID: value.ID,
		TargetDigest: sha256.Sum256([]byte("outbound-task-history-target/" + source))}
	if err := fake.RecordOutboundTaskHistory(ctx, receipt); err != nil {
		return outboundport.OutboundTaskHistoryReceipt{}, err
	}
	return receipt, nil
}

func copyOutboundTaskHistoryTerminals(values map[string]TerminalReceipt) map[string]TerminalReceipt {
	copy := make(map[string]TerminalReceipt, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}

func copyOutboundTaskHistoryValues(values map[int64]outboundport.HistoricalOutboundTask) map[int64]outboundport.HistoricalOutboundTask {
	copy := make(map[int64]outboundport.HistoricalOutboundTask, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}

func outboundTaskHistoryRow(t *testing.T, sourceID, ordinal int64, mutate func(map[string]any)) v1archive.ArchivedRow {
	t.Helper()
	stamp := time.Date(2026, 8, 28, 12, 0, 0, 123456000, time.FixedZone("V1", 8*60*60))
	value := map[string]any{
		"id": sourceID, "task_type": "legacy_send", "request_payload": "request-private", "response_payload": "response-private", "wecom_task_id": "wecom-private",
		"status": "unknown_terminal", "created_at": stamp, "trace_id": "trace-private", "broadcast_job_id": int64(-4),
	}
	if mutate != nil {
		mutate(value)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal("fixture_encode_failed")
	}
	source, err := v1archive.SourceKeyHMAC(outboundTaskHistoryHMACKey, "outbound_tasks", []byte("["+strconv.FormatInt(sourceID, 10)+"]"))
	if err != nil {
		t.Fatal("source_hmac_failed")
	}
	payloadHMAC, err := v1archive.PayloadHMAC(outboundTaskHistoryHMACKey, "outbound_tasks", payload)
	if err != nil {
		t.Fatal("payload_hmac_failed")
	}
	field, err := v1archive.FieldHMAC(outboundTaskHistoryHMACKey, "outbound_tasks", nil)
	if err != nil {
		t.Fatal("field_hmac_failed")
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: v1outboundtaskhistory.OutboundTasksTableID, SourceOrdinal: ordinal,
		SourceKeyHMAC: source, PayloadHMAC: payloadHMAC, FieldHMAC: field, Payload: payload}
}

func outboundTaskHistoryImporterFixture(t *testing.T, rows ...v1archive.ArchivedRow) (*OutboundTaskHistoryImporter, *outboundTaskHistoryRuntimeFake) {
	t.Helper()
	runtime := newOutboundTaskHistoryRuntimeFake()
	importer, err := NewOutboundTaskHistoryImporter(&outboundTaskHistoryArchiveFake{rows: rows}, runtime, runtime, runtime, outboundTaskHistoryHMACKey)
	if err != nil {
		t.Fatal("create_importer_failed")
	}
	return importer, runtime
}

func TestOutboundTaskHistoryImporterWritesOnlyInertHistoryAndReplays(t *testing.T) {
	row := outboundTaskHistoryRow(t, -7, 1, nil)
	importer, runtime := outboundTaskHistoryImporterFixture(t, row)
	first, err := importer.Import(context.Background(), "archive-run")
	if err != nil || first != (OutboundTaskHistoryImportResult{ImportedTasks: 1}) || runtime.calls != 1 || runtime.commits != 1 {
		t.Fatal("first_history_import_failed")
	}
	value, found := runtime.values[100]
	if !found || value.ID != 100 || value.SourceID != -7 || value.BroadcastJobHistoryID != nil || value.LegacyBroadcastJobID == nil || *value.LegacyBroadcastJobID != -4 ||
		value.RequestPayloadDigest == ([sha256.Size]byte{}) || value.ResponsePayloadDigest == ([sha256.Size]byte{}) || value.WeComTaskIDDigest == nil || value.TraceIDDigest == ([sha256.Size]byte{}) ||
		value.SourceKeyDigest != row.SourceKeyHMAC || value.SourcePayloadDigest != row.PayloadHMAC || value.SourceFieldDigest != row.FieldHMAC ||
		value.CreatedAt != time.Date(2026, 8, 28, 4, 0, 0, 123456000, time.UTC) {
		t.Fatalf("inert_history_fact_not_preserved=%+v", value)
	}
	second, err := importer.Import(context.Background(), "archive-run")
	if err != nil || second != (OutboundTaskHistoryImportResult{ImportedTasks: 1, Replayed: 1}) || runtime.calls != 2 || runtime.checks != 1 || len(runtime.values) != 1 {
		t.Fatal("writer_replay_was_not_verified")
	}
}

func TestOutboundTaskHistoryImporterQuarantinesAdapterAndOwnerFailures(t *testing.T) {
	invalidSource := outboundTaskHistoryRow(t, 0, 1, func(value map[string]any) { delete(value, "status") })
	// Re-authenticate the intentionally malformed archived shape. It is an
	// immutable, authentic source row that must quarantine without a write.
	payload, err := json.Marshal(map[string]any{
		"id": int64(0), "task_type": "legacy_send", "request_payload": "request-private", "response_payload": "response-private", "wecom_task_id": "wecom-private",
		"created_at": time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC), "trace_id": "trace-private", "broadcast_job_id": int64(-4),
	})
	if err != nil {
		t.Fatal("fixture_encode_failed")
	}
	invalidSource.Payload = payload
	invalidSource.PayloadHMAC, err = v1archive.PayloadHMAC(outboundTaskHistoryHMACKey, "outbound_tasks", payload)
	if err != nil {
		t.Fatal("payload_hmac_failed")
	}
	importer, runtime := outboundTaskHistoryImporterFixture(t, invalidSource)
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (OutboundTaskHistoryImportResult{Quarantined: 1}) || runtime.calls != 0 || len(runtime.values) != 0 {
		t.Fatal("authenticated_invalid_source_not_quarantined")
	}
	terminal := runtime.terminals[SourceIdentifier(invalidSource.SourceKeyHMAC)]
	if terminal.Disposition != "quarantine" || terminal.Reason != v1outboundtaskhistory.ReasonInvalidSourcePayload || terminal.TargetID != "" || terminal.TargetDigest != ([sha256.Size]byte{}) {
		t.Fatal("invalid_source_terminal_not_exact")
	}

	row := outboundTaskHistoryRow(t, 7, 1, nil)
	importer, runtime = outboundTaskHistoryImporterFixture(t, row)
	runtime.writeErr = outboundport.ErrOutboundTaskHistoryInvalid
	result, err = importer.Import(context.Background(), "archive-run")
	if err != nil || result != (OutboundTaskHistoryImportResult{Quarantined: 1}) || runtime.calls != 1 || len(runtime.values) != 0 {
		t.Fatal("invalid_owner_history_not_quarantined")
	}
	if terminal := runtime.terminals[SourceIdentifier(row.SourceKeyHMAC)]; terminal.Reason != "outbound_task_history_target_invalid" {
		t.Fatal("owner_quarantine_reason_changed")
	}
}

func TestOutboundTaskHistoryImporterRejectsDuplicateKeyAndTargetDrift(t *testing.T) {
	first := outboundTaskHistoryRow(t, 7, 1, nil)
	duplicateKey := outboundTaskHistoryRow(t, 7, 2, nil)
	importer, runtime := outboundTaskHistoryImporterFixture(t, first, duplicateKey)
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) || runtime.calls != 0 {
		t.Fatal("duplicate_source_key_accepted")
	}

	bad := outboundTaskHistoryRow(t, 7, 1, nil)
	bad.Payload[3] ^= 1
	importer, runtime = outboundTaskHistoryImporterFixture(t, bad)
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) || runtime.calls != 0 {
		t.Fatal("unsigned_archive_row_accepted")
	}

	row := outboundTaskHistoryRow(t, 8, 1, nil)
	importer, runtime = outboundTaskHistoryImporterFixture(t, row)
	if _, err := importer.Import(context.Background(), "archive-run"); err != nil {
		t.Fatal("first_import_failed")
	}
	drifted := runtime.values[100]
	drifted.Status = "drifted_target"
	runtime.values[100] = drifted
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, outboundport.ErrOutboundTaskHistoryConflict) || runtime.calls != 2 {
		t.Fatal("target_drift_was_accepted")
	}
}

func TestOutboundTaskHistoryImporterRequiresAndClonesArchiveSourceKey(t *testing.T) {
	runtime := newOutboundTaskHistoryRuntimeFake()
	if _, err := NewOutboundTaskHistoryImporter(&outboundTaskHistoryArchiveFake{}, runtime, runtime, runtime, []byte("short")); !errors.Is(err, ErrInvalidScope) {
		t.Fatal("short_source_hmac_key_accepted")
	}
	key := append([]byte(nil), outboundTaskHistoryHMACKey...)
	importer, err := NewOutboundTaskHistoryImporter(&outboundTaskHistoryArchiveFake{rows: []v1archive.ArchivedRow{outboundTaskHistoryRow(t, 7, 1, nil)}}, runtime, runtime, runtime, key)
	if err != nil {
		t.Fatal("valid_source_hmac_key_rejected")
	}
	key[0] ^= 1
	if _, err = importer.Import(context.Background(), "archive-run"); err != nil {
		t.Fatal("constructor_did_not_clone_source_hmac_key")
	}
}

func TestOutboundTaskHistoryImporterReportsCommittedRetryOnly(t *testing.T) {
	importer, runtime := outboundTaskHistoryImporterFixture(t, outboundTaskHistoryRow(t, 7, 1, nil))
	runtime.retryOnce = true
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (OutboundTaskHistoryImportResult{ImportedTasks: 1}) || len(runtime.values) != 1 || runtime.calls != 2 || runtime.commits != 1 {
		t.Fatal("retry_outcome_was_not_reset")
	}
	if _, err = importer.Import(context.Background(), "other-run"); !errors.Is(err, ErrInvalidScope) {
		t.Fatal("wrong_archive_run_accepted")
	}
}
