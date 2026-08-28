package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1broadcastjobhistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

type broadcastJobHistoryArchiveFake struct{ rows []v1archive.ArchivedRow }

func (fake *broadcastJobHistoryArchiveFake) EachTableRow(_ context.Context, run, table string, callback func(v1archive.ArchivedRow) error) error {
	if run != "archive-run" || table != v1broadcastjobhistory.BroadcastJobsTableID {
		return ErrInvalidScope
	}
	for _, row := range fake.rows {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type broadcastJobHistoryTxKey struct{}

type broadcastJobHistoryRuntimeFake struct {
	terminals map[string]TerminalReceipt
	values    map[int64]outboundport.HistoricalBroadcastJob

	writeErr              error
	writes, checks, calls int
	records, commits      int
	retryOnce             bool
}

func newBroadcastJobHistoryRuntimeFake() *broadcastJobHistoryRuntimeFake {
	return &broadcastJobHistoryRuntimeFake{terminals: map[string]TerminalReceipt{}, values: map[int64]outboundport.HistoricalBroadcastJob{}}
}

func (fake *broadcastJobHistoryRuntimeFake) Within(ctx context.Context, callback func(context.Context) error) error {
	for attempt := 0; ; attempt++ {
		terminals, values := copyBroadcastJobHistoryTerminals(fake.terminals), copyBroadcastJobHistoryValues(fake.values)
		if err := callback(context.WithValue(ctx, broadcastJobHistoryTxKey{}, true)); err != nil {
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

func (fake *broadcastJobHistoryRuntimeFake) ValidateBroadcastJobHistoryImportScope(run string) error {
	if run != "archive-run" {
		return ErrInvalidScope
	}
	return nil
}

func (fake *broadcastJobHistoryRuntimeFake) LoadTerminal(ctx context.Context, source string) (TerminalReceipt, bool, error) {
	if ctx.Value(broadcastJobHistoryTxKey{}) != true {
		return TerminalReceipt{}, false, errors.New("missing transaction")
	}
	value, found := fake.terminals[source]
	return value, found, nil
}

func (fake *broadcastJobHistoryRuntimeFake) Record(ctx context.Context, receipt TerminalReceipt) error {
	if ctx.Value(broadcastJobHistoryTxKey{}) != true {
		return errors.New("missing transaction")
	}
	key := SourceIdentifier(receipt.SourceKeyDigest)
	if old, found := fake.terminals[key]; found && !reflect.DeepEqual(old, receipt) {
		return ErrConflict
	}
	fake.records++
	fake.terminals[key] = receipt
	return nil
}

func (fake *broadcastJobHistoryRuntimeFake) LoadBroadcastJobHistory(ctx context.Context, source string) (outboundport.BroadcastJobHistoryReceipt, bool, error) {
	terminal, found, err := fake.LoadTerminal(ctx, source)
	if err != nil || !found || terminal.Disposition != "import" {
		return outboundport.BroadcastJobHistoryReceipt{}, false, err
	}
	id, err := strconv.ParseInt(terminal.TargetID, 10, 64)
	if err != nil {
		return outboundport.BroadcastJobHistoryReceipt{}, false, err
	}
	return outboundport.BroadcastJobHistoryReceipt{SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetDigest: terminal.TargetDigest, TargetID: id}, true, nil
}

func (fake *broadcastJobHistoryRuntimeFake) RecordBroadcastJobHistory(ctx context.Context, receipt outboundport.BroadcastJobHistoryReceipt) error {
	key, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil {
		return err
	}
	return fake.Record(ctx, TerminalReceipt{SourceKeyDigest: key, PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest})
}

func (fake *broadcastJobHistoryRuntimeFake) Import(ctx context.Context, source string, value outboundport.HistoricalBroadcastJob) (outboundport.BroadcastJobHistoryReceipt, error) {
	if ctx.Value(broadcastJobHistoryTxKey{}) != true {
		return outboundport.BroadcastJobHistoryReceipt{}, errors.New("writer outside transaction")
	}
	fake.calls++
	if fake.writeErr != nil {
		return outboundport.BroadcastJobHistoryReceipt{}, fake.writeErr
	}
	if existing, found, err := fake.LoadBroadcastJobHistory(ctx, source); err != nil {
		return outboundport.BroadcastJobHistoryReceipt{}, err
	} else if found {
		expected := value
		expected.ID = existing.TargetID
		actual, present := fake.values[existing.TargetID]
		if !present || !reflect.DeepEqual(actual, expected) {
			return outboundport.BroadcastJobHistoryReceipt{}, outboundport.ErrBroadcastJobHistoryConflict
		}
		fake.checks++
		existing.Replayed = true
		return existing, nil
	}
	value.ID = int64(100 + len(fake.values))
	fake.values[value.ID] = value
	receipt := outboundport.BroadcastJobHistoryReceipt{SourceIdentifier: source, PayloadDigest: value.SourcePayloadDigest, TargetID: value.ID,
		TargetDigest: sha256.Sum256([]byte("broadcast-job-history-target/" + source))}
	if err := fake.RecordBroadcastJobHistory(ctx, receipt); err != nil {
		return outboundport.BroadcastJobHistoryReceipt{}, err
	}
	fake.writes++
	return receipt, nil
}

func copyBroadcastJobHistoryTerminals(source map[string]TerminalReceipt) map[string]TerminalReceipt {
	copy := make(map[string]TerminalReceipt, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func copyBroadcastJobHistoryValues(source map[int64]outboundport.HistoricalBroadcastJob) map[int64]outboundport.HistoricalBroadcastJob {
	copy := make(map[int64]outboundport.HistoricalBroadcastJob, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func broadcastJobHistoryRow(t *testing.T, sourceID, ordinal int64, mutate func(map[string]any)) v1archive.ArchivedRow {
	t.Helper()
	stamp := time.Date(2026, 8, 28, 12, 0, 0, 123456000, time.FixedZone("V1", 8*60*60))
	value := map[string]any{
		"id": sourceID, "source_type": "legacy_unknown", "source_id": "legacy-source", "source_table": "legacy_table", "scheduled_for": stamp,
		"priority": int32(-1), "batch_key": "private-batch", "status": "legacy_unknown", "requires_approval": true, "approved_by": "actor", "approved_at": nil,
		"cancelled_by": "", "cancelled_at": nil, "cancel_reason": "", "target_count": int32(-2), "target_summary": "private", "content_type": "legacy_blob",
		"content_payload": map[string]any{"private": true}, "content_summary": "private", "attempt_count": int32(-3), "last_error": "", "outbound_task_id": int64(-4),
		"sent_count": int32(-5), "failed_count": int32(-6), "trace_id": "trace", "created_by": "actor", "created_at": stamp, "updated_at": stamp,
		"claimed_at": nil, "sent_at": nil, "claim_token": "[REDACTED]", "lease_expires_at": nil, "business_domain": nil, "idempotency_key": nil,
		"channel": nil, "target_kind": nil, "failure_type": nil, "retry_policy_json": map[string]any{}, "metadata_json": map[string]any{}, "target_unionids_json": []any{"private"},
		"max_attempts": int32(-7), "next_retry_at": nil, "dispatch_started_at": nil, "side_effect_executed": true, "provider_result_received": true,
		"result_summary_json": map[string]any{}, "reconciliation_required": true, "completed_at": nil, "hold_reason": "", "hold_at": nil,
		"external_effect_job_id": nil, "execution_id": "execution", "execution_owner": "owner",
	}
	if mutate != nil {
		mutate(value)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal("fixture_encode_failed")
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: v1broadcastjobhistory.BroadcastJobsTableID, SourceOrdinal: ordinal,
		SourceKeyHMAC: sha256.Sum256([]byte("broadcast-job-key/" + strconv.FormatInt(ordinal, 10))), PayloadHMAC: sha256.Sum256(payload),
		FieldHMAC: sha256.Sum256([]byte("broadcast-job-fields/" + strconv.FormatInt(ordinal, 10))), Payload: payload,
		RedactedFields: []string{"claim_token"}}
}

func broadcastJobHistoryImporterFixture(t *testing.T, rows ...v1archive.ArchivedRow) (*BroadcastJobHistoryImporter, *broadcastJobHistoryRuntimeFake) {
	t.Helper()
	runtime := newBroadcastJobHistoryRuntimeFake()
	importer, err := NewBroadcastJobHistoryImporter(&broadcastJobHistoryArchiveFake{rows: rows}, runtime, runtime, runtime)
	if err != nil {
		t.Fatal("create_importer_failed")
	}
	return importer, runtime
}

func TestBroadcastJobHistoryImporterWritesOnlyImmutableHistoryAndReplays(t *testing.T) {
	row := broadcastJobHistoryRow(t, 7, 1, nil)
	importer, runtime := broadcastJobHistoryImporterFixture(t, row)
	first, err := importer.Import(context.Background(), "archive-run")
	if err != nil || first != (BroadcastJobHistoryImportResult{ImportedJobs: 1}) || runtime.calls != 1 || runtime.commits != 1 {
		t.Fatal("first_history_import_failed")
	}
	value, found := runtime.values[100]
	if !found || value.SourceID != 7 || value.Priority != -1 || value.LegacyOutboundTaskID == nil || *value.LegacyOutboundTaskID != -4 ||
		value.SourceFieldDigest != row.FieldHMAC || value.SourcePayloadDigest != row.PayloadHMAC || value.SideEffectExecuted != true || value.ProviderResultReceived != true ||
		value.CreatedAt != time.Date(2026, 8, 28, 4, 0, 0, 123456000, time.UTC) || value.ClaimTokenDigest == ([sha256.Size]byte{}) {
		t.Fatal("inert_history_fact_not_preserved")
	}
	second, err := importer.Import(context.Background(), "archive-run")
	if err != nil || second != (BroadcastJobHistoryImportResult{ImportedJobs: 1, Replayed: 1}) || runtime.calls != 2 || runtime.checks != 1 || len(runtime.values) != 1 {
		t.Fatal("writer_replay_was_not_verified")
	}
}

func TestBroadcastJobHistoryImporterQuarantinesCandidateFailureWithoutWriter(t *testing.T) {
	row := broadcastJobHistoryRow(t, 7, 1, func(value map[string]any) { delete(value, "status") })
	importer, runtime := broadcastJobHistoryImporterFixture(t, row)
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (BroadcastJobHistoryImportResult{Quarantined: 1}) || runtime.calls != 0 || len(runtime.values) != 0 {
		t.Fatal("invalid_source_was_not_quarantined")
	}
	terminal := runtime.terminals[SourceIdentifier(row.SourceKeyHMAC)]
	if terminal.Disposition != "quarantine" || terminal.Reason != v1broadcastjobhistory.ReasonInvalidSourcePayload || terminal.TargetID != "" || terminal.TargetDigest != ([sha256.Size]byte{}) {
		t.Fatal("quarantine_terminal_not_exact")
	}
}

func TestBroadcastJobHistoryImporterQuarantinesOwnerInvalidWithoutCurrentEffect(t *testing.T) {
	row := broadcastJobHistoryRow(t, 7, 1, nil)
	importer, runtime := broadcastJobHistoryImporterFixture(t, row)
	runtime.writeErr = outboundport.ErrBroadcastJobHistoryInvalid
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (BroadcastJobHistoryImportResult{Quarantined: 1}) || runtime.calls != 1 || len(runtime.values) != 0 {
		t.Fatal("invalid_owner_history_not_quarantined")
	}
	terminal := runtime.terminals[SourceIdentifier(row.SourceKeyHMAC)]
	if terminal.Disposition != "quarantine" || terminal.Reason != "broadcast_job_target_invalid" || terminal.TargetID != "" {
		t.Fatal("owner_quarantine_not_exact")
	}
}

func TestBroadcastJobHistoryImporterRejectsArchiveIntegrityAndWriterDrift(t *testing.T) {
	first := broadcastJobHistoryRow(t, 7, 1, nil)
	duplicate := broadcastJobHistoryRow(t, 7, 2, nil)
	importer, runtime := broadcastJobHistoryImporterFixture(t, first, duplicate)
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) || runtime.calls != 1 {
		t.Fatal("duplicate_source_id_was_accepted")
	}
	badDigest := broadcastJobHistoryRow(t, 7, 1, nil)
	badDigest.FieldHMAC = [sha256.Size]byte{}
	importer, runtime = broadcastJobHistoryImporterFixture(t, badDigest)
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) || runtime.calls != 0 {
		t.Fatal("unsigned_archive_row_was_accepted")
	}
	row := broadcastJobHistoryRow(t, 8, 1, nil)
	importer, runtime = broadcastJobHistoryImporterFixture(t, row)
	if _, err := importer.Import(context.Background(), "archive-run"); err != nil {
		t.Fatal("first_import_failed")
	}
	drifted := runtime.values[100]
	drifted.OriginalStatus = "drifted_target"
	runtime.values[100] = drifted
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, outboundport.ErrBroadcastJobHistoryConflict) || runtime.calls != 2 {
		t.Fatal("target_drift_was_accepted")
	}
}

func TestBroadcastJobHistoryImporterReportsOnlyCommittedRetryOutcome(t *testing.T) {
	importer, runtime := broadcastJobHistoryImporterFixture(t, broadcastJobHistoryRow(t, 7, 1, nil))
	runtime.retryOnce = true
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (BroadcastJobHistoryImportResult{ImportedJobs: 1}) || len(runtime.values) != 1 || runtime.calls != 2 || runtime.commits != 1 {
		t.Fatal("retry_outcome_was_not_reset")
	}
	if _, err = importer.Import(context.Background(), "other-run"); !errors.Is(err, ErrInvalidScope) {
		t.Fatal("wrong_archive_run_accepted")
	}
}
