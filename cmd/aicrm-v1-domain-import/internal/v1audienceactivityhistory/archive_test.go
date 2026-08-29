package v1audienceactivityhistory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestStreamAuthenticatesBothTablesInFixedBatches(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	runs := makeAudienceActivityRows(t, PackageRunsTableID, 251, key)
	events := makeAudienceActivityRows(t, MemberEventsTableID, 1, key)
	archive := audienceActivityArchive{rows: map[string][]v1archive.ArchivedRow{PackageRunsTableID: runs, MemberEventsTableID: events}}
	consumer := &audienceActivityConsumer{}
	summary, err := Stream(context.Background(), archive, "archive-run", key, audienceActivityVerifier{}, consumer)
	if err != nil {
		t.Fatal(err)
	}
	if summary != (StreamSummary{PackageRuns: 251, MemberEvents: 1, Candidates: 252}) {
		t.Fatalf("summary=%#v", summary)
	}
	if got, want := consumer.runBatchSizes, []int{250, 1}; !sameInts(got, want) {
		t.Fatalf("run batches=%v want=%v", got, want)
	}
	if got, want := consumer.eventBatchSizes, []int{1}; !sameInts(got, want) {
		t.Fatalf("event batches=%v want=%v", got, want)
	}
	if consumer.firstRunOrdinal != 1 || consumer.firstEventOrdinal != 1 {
		t.Fatalf("per-table ordinal did not reset: run=%d event=%d", consumer.firstRunOrdinal, consumer.firstEventOrdinal)
	}
}

func TestStreamRedactionQuarantinesBeforeTypedDecode(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	row := makeAudienceActivityRows(t, MemberEventsTableID, 1, key)[0]
	row.RedactedFields = []string{"unionid"}
	row.Payload = mustJSON(t, map[string]any{
		"id": int64(1), "package_id": int64(10), "run_id": nil, "member_current_id": nil,
		"event_type": "entered", "identity_type": "unionid", "identity_value": "value",
		"unionid": map[string]any{"redacted": true}, "mobile_hash": "hash", "owner_userid": "owner",
		"event_source_key": "source", "payload_hash": "payload", "payload_json": nil,
		"internal_event_id": "event", "idempotency_key": "idempotency",
		"occurred_at": "2026-08-27T01:02:03.123456Z", "created_at": "2026-08-27T01:02:04.123456Z",
	})
	refreshAudienceActivityRowHMAC(t, &row, key)
	consumer := &audienceActivityConsumer{}
	summary, err := Stream(context.Background(), audienceActivityArchive{rows: map[string][]v1archive.ArchivedRow{MemberEventsTableID: {row}}}, "archive-run", key, audienceActivityVerifier{}, consumer)
	if err != nil {
		t.Fatal(err)
	}
	if summary != (StreamSummary{MemberEvents: 1, Quarantined: 1}) || consumer.eventBatchSizes[0] != 1 || consumer.eventQuarantined != 1 {
		t.Fatalf("redaction must be a delivered quarantine: summary=%#v consumer=%#v", summary, consumer)
	}
}

func TestStreamRejectsEnvelopeDriftBeforeConsumer(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	rows := makeAudienceActivityRows(t, PackageRunsTableID, 1, key)
	rows[0].PayloadHMAC[0]++
	consumer := &audienceActivityConsumer{}
	_, err := Stream(context.Background(), audienceActivityArchive{rows: map[string][]v1archive.ArchivedRow{PackageRunsTableID: rows}}, "archive-run", key, audienceActivityVerifier{}, consumer)
	if !errors.Is(err, ErrArchiveRow) || len(consumer.runBatchSizes) != 0 {
		t.Fatalf("unverified row reached consumer: err=%v consumer=%#v", err, consumer)
	}
}

type audienceActivityArchive struct {
	rows map[string][]v1archive.ArchivedRow
}

func (source audienceActivityArchive) EachTableRow(_ context.Context, run, table string, emit func(v1archive.ArchivedRow) error) error {
	if run != "archive-run" {
		return errors.New("unexpected run")
	}
	for _, row := range source.rows[table] {
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}

type audienceActivityVerifier struct{}

func (audienceActivityVerifier) VerifyAudienceActivityTerminal(_ context.Context, _ string, source SourceEnvelope, _ Disposition, _ string) error {
	if source.SourceOrdinal < 1 || source.SourceKeyHMAC == ([sha256.Size]byte{}) || source.PayloadHMAC == ([sha256.Size]byte{}) || source.FieldHMAC == ([sha256.Size]byte{}) {
		return ErrArchiveRow
	}
	return nil
}

type audienceActivityConsumer struct {
	runBatchSizes, eventBatchSizes     []int
	firstRunOrdinal, firstEventOrdinal int64
	eventQuarantined                   int
}

func (consumer *audienceActivityConsumer) ConsumeAudienceActivityPackageRunBatch(_ context.Context, values []PackageRunResult) error {
	consumer.runBatchSizes = append(consumer.runBatchSizes, len(values))
	if consumer.firstRunOrdinal == 0 && len(values) != 0 {
		consumer.firstRunOrdinal = values[0].Source.SourceOrdinal
	}
	return nil
}

func (consumer *audienceActivityConsumer) ConsumeAudienceActivityMemberEventBatch(_ context.Context, values []MemberEventResult) error {
	consumer.eventBatchSizes = append(consumer.eventBatchSizes, len(values))
	if consumer.firstEventOrdinal == 0 && len(values) != 0 {
		consumer.firstEventOrdinal = values[0].Source.SourceOrdinal
	}
	for _, value := range values {
		if value.Disposition == DispositionQuarantine {
			consumer.eventQuarantined++
		}
	}
	return nil
}

func makeAudienceActivityRows(t *testing.T, table string, count int, key []byte) []v1archive.ArchivedRow {
	t.Helper()
	rows := make([]v1archive.ArchivedRow, 0, count)
	for ordinal := 1; ordinal <= count; ordinal++ {
		payload := audienceActivityPayload(table, int64(ordinal))
		row := v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: int64(ordinal), Payload: mustJSON(t, payload)}
		refreshAudienceActivityRowHMAC(t, &row, key)
		rows = append(rows, row)
	}
	return rows
}

func audienceActivityPayload(table string, id int64) map[string]any {
	if table == PackageRunsTableID {
		return map[string]any{"id": id, "package_id": int64(10), "version_id": nil, "run_type": "refresh", "status": "done", "refresh_started_at": "2026-08-27T01:02:03.123456Z", "refresh_finished_at": nil, "last_watermark_at": nil, "next_watermark_at": nil, "returned_count": 1, "entered_count": 2, "updated_count": 3, "exited_count": 4, "member_event_count": 5, "duration_ms": 6, "error_message": "", "created_at": "2026-08-27T01:02:04.123456Z"}
	}
	return map[string]any{"id": id, "package_id": int64(10), "run_id": nil, "member_current_id": nil, "event_type": "entered", "identity_type": "unionid", "identity_value": "value", "unionid": "union", "mobile_hash": "hash", "owner_userid": "owner", "event_source_key": "source", "payload_hash": "payload", "payload_json": nil, "internal_event_id": "event", "idempotency_key": "idempotency", "occurred_at": "2026-08-27T01:02:03.123456Z", "created_at": "2026-08-27T01:02:04.123456Z"}
}

func refreshAudienceActivityRowHMAC(t *testing.T, row *v1archive.ArchivedRow, key []byte) {
	t.Helper()
	table := row.TableID[len("public/"):]
	canonical, _, err := v1archive.RedactPayload(row.Payload)
	if err != nil {
		t.Fatal(err)
	}
	row.Payload = canonical
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(canonical, &fields); err != nil {
		t.Fatal(err)
	}
	keyJSON, err := json.Marshal([]json.RawMessage{fields["id"]})
	if err != nil {
		t.Fatal(err)
	}
	row.SourceKeyHMAC, err = v1archive.SourceKeyHMAC(key, table, keyJSON)
	if err != nil {
		t.Fatal(err)
	}
	row.PayloadHMAC, err = v1archive.PayloadHMAC(key, table, canonical)
	if err != nil {
		t.Fatal(err)
	}
	row.FieldHMAC, err = v1archive.FieldHMAC(key, table, row.RedactedFields)
	if err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func sameInts(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
