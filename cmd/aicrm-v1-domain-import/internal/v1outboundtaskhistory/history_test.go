package v1outboundtaskhistory

import (
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestAdaptHistoryPreservesNineColumnObservation(t *testing.T) {
	if len(manifestFields) != 9 {
		t.Fatalf("manifest_field_count=%d", len(manifestFields))
	}
	at := time.Date(2026, 8, 28, 9, 30, 0, 123456789, time.FixedZone("source", 8*60*60))
	value := fixture(at)
	history := AdaptHistory([]v1archive.ArchivedRow{archiveRow(t, 1, value)})
	if history.SourceCount() != 1 || history.TerminalCount() != 1 {
		t.Fatalf("source_conservation_changed=%+v", history)
	}
	result := history.Tasks[0]
	if result.Disposition != DispositionCandidate || result.Reason != "" || result.Fact == nil {
		t.Fatalf("candidate_rejected=%+v", result)
	}
	fact := result.Fact
	if fact.SourceID != -7 || fact.TaskType != "legacy_send" || fact.Status != "unknown_terminal" ||
		fact.CreatedAt.Location() != time.UTC || fact.CreatedAt.Nanosecond()%1000 != 0 ||
		fact.WeComTaskIDDigest != nil || fact.LegacyBroadcastJobID == nil || *fact.LegacyBroadcastJobID != -8 ||
		fact.RequestPayloadDigest == (OpaqueDigest{}) || fact.ResponsePayloadDigest == (OpaqueDigest{}) || fact.TraceIDDigest == (OpaqueDigest{}) ||
		fact.Source.SourceKeyDigest == (OpaqueDigest{}) || fact.Source.PayloadDigest == (OpaqueDigest{}) || fact.Source.FieldDigest == (OpaqueDigest{}) {
		t.Fatalf("historical_values_changed=%+v", fact)
	}
	if want := OpaqueDigest(sha256.Sum256([]byte("v1-outbound-task-history-field-v1\x00public/outbound_tasks\x00request_payload\x00\"request-private\""))); fact.RequestPayloadDigest != want {
		t.Fatalf("request_digest_changed got=%x want=%x", fact.RequestPayloadDigest, want)
	}
}

func TestAdaptHistoryPreservesNullableAndEmptyPrivateFieldsWithoutLeakingThem(t *testing.T) {
	at := time.Date(2026, 8, 28, 9, 30, 0, 0, time.UTC)
	value := fixture(at)
	value["request_payload"] = ""
	value["response_payload"] = ""
	value["wecom_task_id"] = ""
	value["trace_id"] = ""
	value["broadcast_job_id"] = nil
	history := AdaptHistory([]v1archive.ArchivedRow{archiveRow(t, 1, value)})
	fact := mustCandidate(t, history.Tasks[0])
	if fact.WeComTaskIDDigest == nil || *fact.WeComTaskIDDigest == (OpaqueDigest{}) || fact.LegacyBroadcastJobID != nil ||
		fact.RequestPayloadDigest == (OpaqueDigest{}) || fact.ResponsePayloadDigest == (OpaqueDigest{}) || fact.TraceIDDigest == (OpaqueDigest{}) {
		t.Fatalf("nullable_or_empty_source_lost=%+v", fact)
	}
	encoded, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"request-private", "response-private", "wecom-private", "trace-private", "broadcast_job_id", "source_key_digest"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("private_source_material_leaked=%q json=%s", private, encoded)
		}
	}
}

func TestAdaptHistoryRecordsRedactionWithoutClaimingPlaintextDigest(t *testing.T) {
	at := time.Date(2026, 8, 28, 9, 30, 0, 0, time.UTC)
	value := fixture(at)
	value["request_payload"] = "[REDACTED]"
	value["wecom_task_id"] = "[REDACTED]"
	row := archiveRow(t, 1, value, "request_payload.secret", "wecom_task_id")
	fact := mustCandidate(t, AdaptHistory([]v1archive.ArchivedRow{row}).Tasks[0])
	if got := strings.Join(fact.RedactedRoots, ","); got != "request_payload,wecom_task_id" || fact.WeComTaskIDDigest == nil {
		t.Fatalf("redaction_metadata_lost roots=%v fact=%+v", fact.RedactedRoots, fact)
	}
	if fact.RequestPayloadDigest != fieldDigest("request_payload", json.RawMessage(`"[REDACTED]"`)) || fact.RequestPayloadDigest == fieldDigest("request_payload", json.RawMessage(`"request-private"`)) {
		t.Fatal("redacted_digest_claimed_unavailable_plaintext")
	}
	unknown := archiveRow(t, 1, fixture(at), "unknown.secret")
	if got := AdaptHistory([]v1archive.ArchivedRow{unknown}).Tasks[0]; got.Disposition != DispositionQuarantine || got.Reason != ReasonUnknownRedactedField {
		t.Fatalf("unknown_redaction_accepted=%+v", got)
	}
}

func TestAdaptHistoryQuarantinesBadEnvelopeShapeAndDuplicateSignedIDs(t *testing.T) {
	at := time.Date(2026, 8, 28, 9, 30, 0, 0, time.UTC)
	row := archiveRow(t, 1, fixture(at))
	for _, mutate := range []func(*v1archive.ArchivedRow){
		func(value *v1archive.ArchivedRow) { value.AdapterID = "wrong" },
		func(value *v1archive.ArchivedRow) { value.TableID = "public/other" },
		func(value *v1archive.ArchivedRow) { value.SourceOrdinal = 2 },
		func(value *v1archive.ArchivedRow) { value.SourceKeyHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.PayloadHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.FieldHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.Payload = []byte(`{`) },
	} {
		changed := row
		mutate(&changed)
		if got := AdaptHistory([]v1archive.ArchivedRow{changed}).Tasks[0]; got.Disposition != DispositionQuarantine || got.Reason != ReasonInvalidArchiveRow {
			t.Fatalf("bad_archive_accepted=%+v", got)
		}
	}
	for name, mutate := range map[string]func(map[string]any){
		"missing":       func(value map[string]any) { delete(value, "status") },
		"extra":         func(value map[string]any) { value["unexpected"] = true },
		"required_null": func(value map[string]any) { value["task_type"] = nil },
		"bad_time":      func(value map[string]any) { value["created_at"] = "not-a-time" },
	} {
		t.Run(name, func(t *testing.T) {
			value := fixture(at)
			mutate(value)
			if got := AdaptHistory([]v1archive.ArchivedRow{archiveRow(t, 1, value)}).Tasks[0]; got.Disposition != DispositionQuarantine || got.Reason != ReasonInvalidSourcePayload {
				t.Fatalf("bad_shape_accepted=%+v", got)
			}
		})
	}
	first := fixture(at)
	first["id"] = int64(0)
	second := fixture(at)
	second["id"] = int64(0)
	history := AdaptHistory([]v1archive.ArchivedRow{archiveRow(t, 1, first), archiveRow(t, 2, second)})
	for _, result := range history.Tasks {
		if result.Disposition != DispositionQuarantine || result.Reason != ReasonDuplicateSourceID || result.SourceID != 0 || result.Fact != nil {
			t.Fatalf("duplicate_signed_source_id_not_isolated=%+v", result)
		}
	}
	negative := fixture(at)
	negative["id"] = int64(-1)
	if got := AdaptHistory([]v1archive.ArchivedRow{archiveRow(t, 1, negative)}).Tasks[0]; got.Disposition != DispositionCandidate || got.SourceID != -1 {
		t.Fatalf("signed_source_id_rejected=%+v", got)
	}
}

func fixture(at time.Time) map[string]any {
	return map[string]any{
		"id": int64(-7), "task_type": "legacy_send", "request_payload": "request-private", "response_payload": "response-private", "wecom_task_id": nil,
		"status": "unknown_terminal", "created_at": at, "trace_id": "trace-private", "broadcast_job_id": int64(-8),
	}
}

func archiveRow(t *testing.T, ordinal int64, value map[string]any, redacted ...string) v1archive.ArchivedRow {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{
		AdapterID: v1archive.DefaultAdapterID, TableID: OutboundTasksTableID, SourceOrdinal: ordinal,
		SourceKeyHMAC: sha256.Sum256([]byte("source-key/" + string(rune(ordinal)))),
		PayloadHMAC:   sha256.Sum256(payload), FieldHMAC: sha256.Sum256([]byte("field/" + string(rune(ordinal)))),
		Payload: payload, RedactedFields: redacted,
	}
}

func mustCandidate(t *testing.T, result Result) *OutboundTaskHistoryFact {
	t.Helper()
	if result.Disposition != DispositionCandidate || result.Fact == nil {
		t.Fatalf("expected_candidate=%+v", result)
	}
	return result.Fact
}
