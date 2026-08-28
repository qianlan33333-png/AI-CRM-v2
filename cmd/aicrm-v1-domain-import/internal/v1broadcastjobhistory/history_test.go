package v1broadcastjobhistory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestAdaptHistoryPreservesAllObservedFieldsWithoutCreatingCurrentWork(t *testing.T) {
	if len(manifestFields) != 53 {
		t.Fatalf("manifest_field_count=%d", len(manifestFields))
	}
	at := time.Date(2026, 8, 28, 9, 10, 11, 123456789, time.FixedZone("V1", 8*60*60))
	row := broadcastJobRow(t, 1, broadcastJobPayload(at))
	result := AdaptHistory(row)
	if result.Disposition != DispositionCandidate || result.Reason != "" || result.Fact == nil {
		t.Fatalf("candidate_rejected=%#v", result)
	}
	fact := result.Fact
	if fact.SourceID != 7 || fact.OriginalSourceType != "campaign" || fact.SourceTable != "campaigns" || fact.OriginalStatus != "unknown_after_dispatch" ||
		fact.TargetCount != -2 || fact.AttemptCount != -3 || fact.SentCount != -4 || fact.FailedCount != -5 || fact.MaxAttempts != -6 ||
		fact.LegacyOutboundTaskID == nil || *fact.LegacyOutboundTaskID != -8 || fact.LegacyExternalEffectJobID == nil || *fact.LegacyExternalEffectJobID != -9 ||
		fact.BusinessDomain == nil || *fact.BusinessDomain != "legacy-domain" || fact.Channel == nil || *fact.Channel != "wecom" || fact.TargetKind == nil || *fact.TargetKind != "unionid" || fact.FailureType == nil || *fact.FailureType != "legacy_failure" ||
		fact.IdempotencyKeyDigest == nil || fact.ScheduledFor.Nanosecond()%1000 != 0 || fact.CreatedAt.Nanosecond()%1000 != 0 || fact.ApprovedAt == nil || fact.ApprovedAt.Location() != time.UTC ||
		!fact.SideEffectExecuted || !fact.ProviderResultReceived || !fact.ReconciliationRequired {
		t.Fatalf("historical_values_changed=%#v", fact)
	}
	if fact.SourceReferenceDigest != OpaqueDigest(sha256.Sum256([]byte(`"legacy-source"`))) || fact.ContentPayloadDigest != OpaqueDigest(sha256.Sum256([]byte(`{"body":"private"}`))) ||
		fact.TargetUnionIDsDigest != OpaqueDigest(sha256.Sum256([]byte(`["private-union"]`))) || fact.SourceKeyDigest != row.SourceKeyHMAC || fact.SourcePayloadDigest != row.PayloadHMAC || fact.ArchiveFieldDigest != row.FieldHMAC {
		t.Fatal("opaque_or_archive_binding_changed")
	}
}

func TestAdaptHistoryAllowsOnlyOpaqueRedactionAndNeverRecoversIt(t *testing.T) {
	at := time.Date(2026, 8, 28, 9, 10, 11, 0, time.UTC)
	payload := broadcastJobPayload(at)
	payload["content_payload"] = "[REDACTED]"
	payload["target_unionids_json"] = "[REDACTED]"
	row := broadcastJobRow(t, 1, payload, "content_payload.body", "target_unionids_json[0]")
	result := AdaptHistory(row)
	if result.Disposition != DispositionCandidate || result.Fact == nil || len(result.Fact.RedactedRoots) != 2 || result.Fact.RedactedRoots[0] != "content_payload" || result.Fact.RedactedRoots[1] != "target_unionids_json" {
		t.Fatalf("opaque_redaction_not_preserved=%#v", result)
	}
	if result.Fact.ContentPayloadDigest == (OpaqueDigest{}) || result.Fact.TargetUnionIDsDigest == (OpaqueDigest{}) {
		t.Fatal("redacted_material_not_bound_to_archive")
	}
	for _, field := range []string{"id", "status", "scheduled_for", "side_effect_executed", "business_domain"} {
		changed := broadcastJobRow(t, 1, broadcastJobPayload(at), field)
		if got := AdaptHistory(changed); got.Disposition != DispositionQuarantine || got.Reason != ReasonRequiredRedacted {
			t.Fatalf("required_redaction_accepted field=%s result=%#v", field, got)
		}
	}
	if got := AdaptHistory(broadcastJobRow(t, 1, broadcastJobPayload(at), "unknown.secret")); got.Reason != ReasonUnknownRedactedField {
		t.Fatalf("unknown_redaction_accepted=%#v", got)
	}
}

func TestAdaptHistoryQuarantinesManifestAndShapeErrors(t *testing.T) {
	at := time.Date(2026, 8, 28, 9, 10, 11, 0, time.UTC)
	for name, mutate := range map[string]func(map[string]any){
		"missing_required": func(value map[string]any) { delete(value, "status") },
		"null_required":    func(value map[string]any) { value["content_type"] = nil },
		"zero_source":      func(value map[string]any) { value["id"] = int64(0) },
		"invalid_time":     func(value map[string]any) { value["scheduled_for"] = "not-a-time" },
	} {
		t.Run(name, func(t *testing.T) {
			payload := broadcastJobPayload(at)
			mutate(payload)
			if got := AdaptHistory(broadcastJobRow(t, 1, payload)); got.Disposition != DispositionQuarantine || got.Fact != nil {
				t.Fatalf("invalid_shape_accepted=%#v", got)
			}
		})
	}
}

func TestPreflightConservesRowsAndRejectsDuplicateSourceKeys(t *testing.T) {
	at := time.Date(2026, 8, 28, 9, 10, 11, 0, time.UTC)
	first := broadcastJobRow(t, 1, broadcastJobPayload(at))
	secondPayload := broadcastJobPayload(at)
	secondPayload["id"] = int64(8)
	second := broadcastJobRow(t, 2, secondPayload, "metadata_json.secret")
	report, err := Preflight(context.Background(), archiveRows{rows: []v1archive.ArchivedRow{first, second}}, "run-1")
	if err != nil || report.SourceRows != 2 || report.Candidates != 2 || report.Quarantined != 0 || report.RedactedRoots["metadata_json"] != 1 {
		t.Fatalf("preflight_not_conserved report=%#v err=%v", report, err)
	}
	second.SourceKeyHMAC = first.SourceKeyHMAC
	if _, err := Preflight(context.Background(), archiveRows{rows: []v1archive.ArchivedRow{first, second}}, "run-1"); !errors.Is(err, ErrInvalidArchiveRow) {
		t.Fatalf("duplicate_source_key_accepted=%v", err)
	}
	second = broadcastJobRow(t, 2, broadcastJobPayload(at))
	if _, err := Preflight(context.Background(), archiveRows{rows: []v1archive.ArchivedRow{first, second}}, "run-1"); !errors.Is(err, ErrInvalidArchiveRow) {
		t.Fatalf("duplicate_source_id_accepted=%v", err)
	}
	quarantined := broadcastJobRow(t, 1, broadcastJobPayload(at), "status")
	report, err = Preflight(context.Background(), archiveRows{rows: []v1archive.ArchivedRow{quarantined}}, "run-1")
	if err != nil || report.Quarantined != 1 || report.RedactedRoots["status"] != 1 || report.Reasons[ReasonRequiredRedacted] != 1 {
		t.Fatalf("quarantined_redaction_not_accounted report=%#v err=%v", report, err)
	}
}

func broadcastJobPayload(at time.Time) map[string]any {
	return map[string]any{
		"id": int64(7), "source_type": "campaign", "source_id": "legacy-source", "source_table": "campaigns", "scheduled_for": at, "priority": int32(-1), "batch_key": "batch-key", "status": "unknown_after_dispatch", "requires_approval": true,
		"approved_by": "legacy-approver", "approved_at": at, "cancelled_by": "", "cancelled_at": nil, "cancel_reason": "", "target_count": int32(-2), "target_summary": "private targets", "content_type": "text", "content_payload": json.RawMessage(`{"body":"private"}`),
		"content_summary": "private summary", "attempt_count": int32(-3), "last_error": "legacy error", "outbound_task_id": int64(-8), "sent_count": int32(-4), "failed_count": int32(-5), "trace_id": "trace", "created_by": "legacy-owner", "created_at": at, "updated_at": at,
		"claimed_at": at, "sent_at": at, "claim_token": "legacy-token", "lease_expires_at": at, "business_domain": "legacy-domain", "idempotency_key": "legacy-idempotency", "channel": "wecom", "target_kind": "unionid", "failure_type": "legacy_failure",
		"retry_policy_json": json.RawMessage(`{"max":1}`), "metadata_json": json.RawMessage(`{"private":true}`), "target_unionids_json": json.RawMessage(`["private-union"]`), "max_attempts": int32(-6), "next_retry_at": at, "dispatch_started_at": at,
		"side_effect_executed": true, "provider_result_received": true, "result_summary_json": json.RawMessage(`{"status":"old"}`), "reconciliation_required": true, "completed_at": at, "hold_reason": "", "hold_at": at, "external_effect_job_id": int64(-9), "execution_id": "execution", "execution_owner": "legacy-executor",
	}
}

func broadcastJobRow(t *testing.T, ordinal int64, payload map[string]any, redacted ...string) v1archive.ArchivedRow {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: BroadcastJobsTableID, SourceOrdinal: ordinal, Payload: encoded, RedactedFields: redacted,
		SourceKeyHMAC: sha256.Sum256([]byte("source-key-" + string(rune(ordinal)))), PayloadHMAC: sha256.Sum256(encoded), FieldHMAC: sha256.Sum256([]byte("fields-" + string(rune(ordinal))))}
}

type archiveRows struct{ rows []v1archive.ArchivedRow }

func (archive archiveRows) EachTableRow(_ context.Context, _ string, table string, emit func(v1archive.ArchivedRow) error) error {
	if table != BroadcastJobsTableID {
		return ErrInvalidArchiveRow
	}
	for _, row := range archive.rows {
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}
