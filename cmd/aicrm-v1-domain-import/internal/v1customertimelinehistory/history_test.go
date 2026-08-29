package v1customertimelinehistory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var timelineTestKey = bytes.Repeat([]byte{0x53}, sha256.Size)

func TestAdaptTimelineEventPreservesAllTypedSourceColumns(t *testing.T) {
	at := time.Date(2026, 8, 29, 10, 11, 12, 123456789, time.FixedZone("source", 8*3600))
	row := timelineRow(t, -9, 1, map[string]any{
		"id": int64(-9), "event_id": "", "event_type": "legacy_unknown", "event_time": at, "title": "", "summary": "first\nsecond",
		"source_table": "legacy_table", "source_id": "-3", "metadata_json": json.RawMessage("null"), "created_at": at.Add(time.Second), "unionid": "private-union",
	})
	fact, err := AdaptTimelineEvent(row, timelineTestKey, 1)
	if err != nil {
		t.Fatal(err)
	}
	if fact.SourceID != -9 || fact.EventID != "" || fact.EventType != "legacy_unknown" || fact.Title != "" || fact.Summary != "first\nsecond" || fact.SourceTable != "legacy_table" || fact.SourceValue != "-3" || string(fact.MetadataJSON) != "null" || fact.UnionID != "private-union" {
		t.Fatalf("source values changed: %#v", fact)
	}
	if fact.EventTime.Location() != time.UTC || fact.EventTime.Nanosecond() != 123456000 || fact.CreatedAt.Nanosecond() != 123456000 || fact.Source.SourceKeyHMAC != row.SourceKeyHMAC || fact.Source.PayloadHMAC != row.PayloadHMAC || fact.Source.FieldHMAC != row.FieldHMAC {
		t.Fatalf("time or HMAC envelope changed: %#v", fact)
	}
	encoded, err := json.Marshal(fact)
	if err != nil || strings.Contains(string(encoded), "private-union") || strings.Contains(string(encoded), "first") {
		t.Fatalf("private timeline source leaked: %s err=%v", encoded, err)
	}
}

func TestAdaptTimelineEventRejectsMissingUnexpectedAndNonnullableNull(t *testing.T) {
	base := timelinePayload(1)
	for _, field := range timelineFields {
		t.Run("missing_"+field, func(t *testing.T) {
			payload := clonePayload(t, base)
			delete(payload, field)
			if _, err := AdaptTimelineEvent(timelineRow(t, 1, 1, payload), timelineTestKey, 1); !errors.Is(err, ErrFact) {
				t.Fatalf("missing %s error=%v", field, err)
			}
		})
		if timelineJSONLiteralNull[field] {
			continue
		}
		t.Run("null_"+field, func(t *testing.T) {
			payload := clonePayload(t, base)
			payload[field] = nil
			if _, err := AdaptTimelineEvent(timelineRow(t, 1, 1, payload), timelineTestKey, 1); !errors.Is(err, ErrFact) {
				t.Fatalf("nonnullable %s error=%v", field, err)
			}
		})
	}
	extra := clonePayload(t, base)
	extra["unexpected"] = true
	if _, err := AdaptTimelineEvent(timelineRow(t, 1, 1, extra), timelineTestKey, 1); !errors.Is(err, ErrFact) {
		t.Fatalf("unexpected field error=%v", err)
	}
}

func TestAdaptTimelineEventQuarantinesRedactedSourceOnlyInStream(t *testing.T) {
	payload := timelinePayload(1)
	payload["unionid"] = "[REDACTED]"
	row := timelineTopLevelRedactedRow(t, 1, 1, payload, "unionid")
	if _, err := AdaptTimelineEvent(row, timelineTestKey, 1); !errors.Is(err, ErrRequiredFieldRedacted) {
		t.Fatalf("redacted fact error=%v", err)
	}
	verifier := &timelineVerifier{}
	summary, err := Stream(context.Background(), timelineArchive{rows: []v1archive.ArchivedRow{row}}, "run", timelineTestKey, verifier, nil)
	if err != nil || summary.Rows != 1 || summary.Candidates != 0 || summary.Quarantined != 1 || len(verifier.calls) != 1 || verifier.calls[0].disposition != DispositionQuarantine || verifier.calls[0].reason != ReasonFieldRedacted {
		t.Fatalf("redacted source was not safely isolated: summary=%#v calls=%#v err=%v", summary, verifier.calls, err)
	}
}

func TestStreamBatchesAt250AndRejectsHMACDrift(t *testing.T) {
	rows := make([]v1archive.ArchivedRow, 251)
	for index := range rows {
		rows[index] = timelineRow(t, int64(index-10), int64(index+1), timelinePayload(int64(index-10)))
	}
	consumer := &timelineConsumer{}
	verifier := &timelineVerifier{}
	summary, err := Stream(context.Background(), timelineArchive{rows: rows}, "run", timelineTestKey, verifier, consumer)
	if err != nil || summary.Rows != 251 || summary.Candidates != 251 || summary.Quarantined != 0 || !sameInts(consumer.sizes(), []int{250, 1}) || len(verifier.calls) != 251 {
		t.Fatalf("unbounded or incomplete stream: summary=%#v sizes=%v calls=%d err=%v", summary, consumer.sizes(), len(verifier.calls), err)
	}
	for _, mutation := range []func(*v1archive.ArchivedRow){
		func(row *v1archive.ArchivedRow) { row.SourceKeyHMAC[0] ^= 0xff },
		func(row *v1archive.ArchivedRow) { row.PayloadHMAC[0] ^= 0xff },
		func(row *v1archive.ArchivedRow) { row.FieldHMAC[0] ^= 0xff },
	} {
		changed := rows[0]
		mutation(&changed)
		_, err := Stream(context.Background(), timelineArchive{rows: []v1archive.ArchivedRow{changed}}, "run", timelineTestKey, &timelineVerifier{}, nil)
		if !errors.Is(err, ErrArchiveRow) {
			t.Fatalf("HMAC drift accepted: %v", err)
		}
	}
}

func timelinePayload(id int64) map[string]any {
	at := time.Date(2026, 8, 29, 1, 2, 3, 456789123, time.UTC)
	return map[string]any{
		"id": id, "event_id": "event-" + strconv.FormatInt(id, 10), "event_type": "legacy", "event_time": at, "title": "title", "summary": "summary",
		"source_table": "source", "source_id": strconv.FormatInt(id, 10), "metadata_json": json.RawMessage(`{"recorded":true}`), "created_at": at, "unionid": "union",
	}
}

func timelineRow(t *testing.T, id, ordinal int64, payload map[string]any) v1archive.ArchivedRow {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	canonical, roots, err := v1archive.RedactPayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	source, err := v1archive.SourceKeyHMAC(timelineTestKey, archiveTableName(), []byte("["+strconv.FormatInt(id, 10)+"]"))
	if err != nil {
		t.Fatal(err)
	}
	payloadHMAC, err := v1archive.PayloadHMAC(timelineTestKey, archiveTableName(), canonical)
	if err != nil {
		t.Fatal(err)
	}
	fieldHMAC, err := v1archive.FieldHMAC(timelineTestKey, archiveTableName(), roots)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: TableID, SourceOrdinal: ordinal, SourceKeyHMAC: source, PayloadHMAC: payloadHMAC, FieldHMAC: fieldHMAC, Payload: canonical, RedactedFields: roots}
}

func timelineTopLevelRedactedRow(t *testing.T, id, ordinal int64, payload map[string]any, root string) v1archive.ArchivedRow {
	t.Helper()
	row := timelineRow(t, id, ordinal, payload)
	fieldHMAC, err := v1archive.FieldHMAC(timelineTestKey, archiveTableName(), []string{root})
	if err != nil {
		t.Fatal(err)
	}
	row.RedactedFields = []string{root}
	row.FieldHMAC = fieldHMAC
	return row
}

func clonePayload(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

type timelineArchive struct{ rows []v1archive.ArchivedRow }

func (source timelineArchive) EachTableRow(_ context.Context, run, table string, callback func(v1archive.ArchivedRow) error) error {
	if run != "run" || table != TableID || callback == nil {
		return fmt.Errorf("archive request invalid")
	}
	for _, row := range source.rows {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type timelineVerificationCall struct {
	SourceEnvelope
	disposition string
	reason      string
}
type timelineVerifier struct{ calls []timelineVerificationCall }

func (verifier *timelineVerifier) VerifyCustomerTimelineTerminal(_ context.Context, source SourceEnvelope, disposition, reason string) error {
	verifier.calls = append(verifier.calls, timelineVerificationCall{SourceEnvelope: source, disposition: disposition, reason: reason})
	return nil
}

type timelineConsumer struct{ batches []Batch }

func (consumer *timelineConsumer) ConsumeCustomerTimelineBatch(_ context.Context, batch Batch) error {
	consumer.batches = append(consumer.batches, batch)
	return nil
}

func (consumer *timelineConsumer) sizes() []int {
	result := make([]int, 0, len(consumer.batches))
	for _, batch := range consumer.batches {
		result = append(result, len(batch.Rows))
	}
	return result
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
