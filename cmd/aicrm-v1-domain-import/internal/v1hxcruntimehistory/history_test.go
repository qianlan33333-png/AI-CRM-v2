package v1hxcruntimehistory

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var testHMACKey = bytes.Repeat([]byte{0x61}, sha256.Size)

func TestAdaptSenderConfigPreservesHistoricalScalarsAndHidesPrivateSource(t *testing.T) {
	payload := senderConfigPayload(t, map[string]any{
		"id":            int64(-7),
		"sender_userid": "staff-private",
		"display_name":  "姓名-private",
		"priority":      int64(-9),
		"is_active":     false,
		"created_at":    "2025-04-05T06:07:08.123456789+08:00",
		"updated_at":    "2024-01-02T03:04:05.000000999-03:00",
	})
	fact, err := AdaptSenderConfig(payload, testHMACKey)
	if err != nil {
		t.Fatalf("adapt sender config: %v", err)
	}
	if fact.SourceID != -7 || fact.Priority != -9 || fact.OriginalIsActive || fact.PrivateDigest == (OpaqueDigest{}) {
		t.Fatalf("historical scalar changed: %#v", fact)
	}
	if fact.CreatedAt.Location() != time.UTC || fact.CreatedAt.Nanosecond() != 123456000 || fact.UpdatedAt.Location() != time.UTC || fact.UpdatedAt.Nanosecond() != 0 {
		t.Fatalf("times were not normalized to UTC microseconds: %#v", fact)
	}
	encoded, err := json.Marshal(fact)
	if err != nil || strings.Contains(string(encoded), "staff-private") || strings.Contains(string(encoded), "姓名-private") {
		t.Fatalf("private sender source leaked: %s", encoded)
	}
}

func TestAdaptSendRecordPreservesSignedFactsAndNullableSource(t *testing.T) {
	payload := sendRecordPayload(t, map[string]any{
		"id":                           int64(-13),
		"task_type":                    "",
		"status":                       "legacy_queued",
		"selected_count":               int64(-1),
		"eligible_count":               int64(0),
		"sent_count":                   int64(1),
		"skipped_count":                int64(-2),
		"image_count":                  int64(-3),
		"planned_count":                int64(4),
		"queued_count":                 int64(-5),
		"dispatching_count":            int64(6),
		"succeeded_count":              int64(-7),
		"failed_count":                 int64(8),
		"blocked_count":                int64(-9),
		"cancelled_count":              int64(10),
		"include_do_not_disturb":       false,
		"target_source":                "",
		"target_source_id":             int64(-11),
		"last_status_sync_at":          "2025-04-05T06:07:08.123456789+08:00",
		"created_at":                   "2024-01-02T03:04:05.000000999-03:00",
		"last_refreshed_at":            nil,
		"record_key":                   "record-private",
		"content_preview":              "13800138000 private content",
		"operator":                     "operator-private",
		"target_unionids_json":         []any{"union-private"},
		"sender_userids_json":          []any{"sender-private"},
		"external_effect_job_ids_json": []any{"effect-private"},
	})
	fact, err := AdaptSendRecord(payload, testHMACKey)
	if err != nil {
		t.Fatalf("adapt send record: %v", err)
	}
	if fact.SourceID != -13 || fact.TaskType != "" || fact.OriginalStatus != "legacy_queued" || fact.SelectedCount != -1 || fact.EligibleCount != 0 ||
		fact.SentCount != 1 || fact.SkippedCount != -2 || fact.ImageCount != -3 || fact.PlannedCount != 4 || fact.QueuedCount != -5 ||
		fact.DispatchingCount != 6 || fact.SucceededCount != -7 || fact.FailedCount != 8 || fact.BlockedCount != -9 || fact.CancelledCount != 10 ||
		fact.IncludeDoNotDisturb || fact.TargetSource != "" || fact.TargetSourceID == nil || *fact.TargetSourceID != -11 || fact.PrivateDigest == (OpaqueDigest{}) {
		t.Fatalf("historical send record changed: %#v", fact)
	}
	if fact.LastStatusSyncAt == nil || fact.LastStatusSyncAt.Location() != time.UTC || fact.LastStatusSyncAt.Nanosecond() != 123456000 ||
		fact.CreatedAt.Location() != time.UTC || fact.CreatedAt.Nanosecond() != 0 || fact.LastRefreshedAt != nil {
		t.Fatalf("nullable/time source changed: %#v", fact)
	}
	encoded, err := json.Marshal(fact)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"record-private", "13800138000", "operator-private", "union-private", "sender-private", "effect-private"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("private source leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestPrivateDigestPreservesNullEmptyAndCanonicalPrivateJSON(t *testing.T) {
	nullFact, err := AdaptSendRecord(sendRecordPayload(t, map[string]any{"idempotency_key": nil}), testHMACKey)
	if err != nil {
		t.Fatal(err)
	}
	emptyFact, err := AdaptSendRecord(sendRecordPayload(t, map[string]any{"idempotency_key": ""}), testHMACKey)
	if err != nil {
		t.Fatal(err)
	}
	if nullFact.PrivateDigest == emptyFact.PrivateDigest {
		t.Fatal("private null and empty string were conflated")
	}
	left, err := privateDigest(testHMACKey, SendRecordsTableID, map[string]json.RawMessage{"private": json.RawMessage(`{"b":2,"a":1}`)}, []string{"private"})
	if err != nil {
		t.Fatal(err)
	}
	right, err := privateDigest(testHMACKey, SendRecordsTableID, map[string]json.RawMessage{"private": json.RawMessage(`{"a":1,"b":2}`)}, []string{"private"})
	if err != nil || left != right {
		t.Fatal("private tuple was not canonical")
	}
}

func TestAdapterRejectsMissingNullExtraWrongTypeAndShortKey(t *testing.T) {
	validConfig := senderConfigPayload(t, nil)
	validRecord := sendRecordPayload(t, nil)
	for _, test := range []struct {
		name    string
		payload []byte
		adapt   func([]byte, []byte) error
		key     []byte
	}{
		{"missing_config_field", []byte(`{"id":1}`), func(payload, key []byte) error { _, err := AdaptSenderConfig(payload, key); return err }, testHMACKey},
		{"null_required_config_field", senderConfigPayload(t, map[string]any{"priority": nil}), func(payload, key []byte) error { _, err := AdaptSenderConfig(payload, key); return err }, testHMACKey},
		{"extra_record_field", appendExtra(t, validRecord), func(payload, key []byte) error { _, err := AdaptSendRecord(payload, key); return err }, testHMACKey},
		{"wrong_record_count_type", sendRecordPayload(t, map[string]any{"selected_count": "1"}), func(payload, key []byte) error { _, err := AdaptSendRecord(payload, key); return err }, testHMACKey},
		{"short_key", validConfig, func(payload, key []byte) error { _, err := AdaptSenderConfig(payload, key); return err }, testHMACKey[:sha256.Size-1]},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.adapt(test.payload, test.key); !errorsIsInvalid(err) {
				t.Fatalf("invalid source accepted: %v", err)
			}
		})
	}
}

func senderConfigPayload(t *testing.T, override map[string]any) []byte {
	t.Helper()
	value := map[string]any{
		"id": 1, "sender_userid": "sender", "display_name": "name", "priority": 100, "is_active": true,
		"created_at": "2025-01-02T03:04:05.123456Z", "updated_at": "2025-01-03T04:05:06.654321Z",
	}
	for key, item := range override {
		value[key] = item
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func sendRecordPayload(t *testing.T, override map[string]any) []byte {
	t.Helper()
	value := map[string]any{
		"id": 1, "record_key": "record", "task_type": "user_ops_batch_send", "outbound_task_ids_json": []any{}, "task_results_json": []any{},
		"selected_count": 1, "eligible_count": 2, "sent_count": 3, "skipped_count": 4, "skipped_reasons_json": map[string]any{},
		"include_do_not_disturb": true, "content_preview": "content", "image_count": 5, "sender_userids_json": []any{}, "filter_snapshot_json": map[string]any{},
		"operator": "operator", "status": "created", "status_label": "created", "last_status_sync_at": nil, "created_at": "2025-01-02T03:04:05.123456Z",
		"target_unionids_json": []any{}, "idempotency_key": nil, "execution_backend": "legacy", "external_effect_job_ids_json": []any{}, "external_effect_status_summary_json": map[string]any{},
		"planned_count": 6, "queued_count": 7, "dispatching_count": 8, "succeeded_count": 9, "failed_count": 10, "blocked_count": 11, "cancelled_count": 12,
		"last_refreshed_at": nil, "target_source": "legacy", "target_source_id": nil,
	}
	for key, item := range override {
		value[key] = item
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func appendExtra(t *testing.T, payload []byte) []byte {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil {
		t.Fatal(err)
	}
	value["unexpected"] = true
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func errorsIsInvalid(err error) bool { return err == ErrInvalidSource }
