package v1hxcchatjobhistory

import (
	"crypto/sha256"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestAdaptChatJobPreservesAllColumnsWithoutSerializingPrivateSource(t *testing.T) {
	key := []byte(strings.Repeat("k", sha256.Size))
	created := time.Date(2026, 8, 29, 9, 2, 3, 123456789, time.FixedZone("source", 8*60*60))
	updated := time.Date(2026, 8, 29, 10, 2, 3, 987654321, time.FixedZone("other", -3*60*60))
	value := chatJobFixture(created, updated)
	value["id"] = int64(-7)
	value["queue_id"] = int64(-2)
	value["member_id"] = nil
	value["send_record_id"] = int64(0)
	value["request_payload_json"] = nil
	value["accepted_payload_json"] = []any{"accepted"}
	value["callback_payload_json"] = map[string]any{"callback": false}
	value["send_result_json"] = "result"
	value["finished_at"] = "legacy civil time without timezone"

	row := chatJobRow(t, key, value)
	fact, err := AdaptChatJob(row, key)
	if err != nil {
		t.Fatal(err)
	}
	if fact.SourceID != -7 || fact.QueueID == nil || *fact.QueueID != -2 || fact.MemberID != nil || fact.SendRecordID == nil || *fact.SendRecordID != 0 ||
		fact.OriginalStatus != "legacy_terminal" || fact.SendChannel != "legacy_channel" || fact.FinishedAt != "legacy civil time without timezone" ||
		string(fact.RequestPayloadJSON) != "null" || string(fact.AcceptedPayloadJSON) != `["accepted"]` || string(fact.SendResultJSON) != `"result"` {
		t.Fatalf("source values changed: %#v", fact)
	}
	if fact.Source.SourceKeyDigest != row.SourceKeyHMAC || fact.Source.PayloadDigest != row.PayloadHMAC || fact.Source.FieldDigest != row.FieldHMAC ||
		fact.CreatedAt.Location() != time.UTC || fact.CreatedAt.Nanosecond() != 123456000 || fact.UpdatedAt.Location() != time.UTC || fact.UpdatedAt.Nanosecond() != 987654000 {
		t.Fatalf("source proof or timestamp changed: %#v", fact)
	}

	encoded, err := json.Marshal(fact)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"external-contact-private", "phone-private", "message-private", "session-private", "task-private", "reply-private", "error-private", "callback", "source_key_digest"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("private source leaked: %q json=%s", private, encoded)
		}
	}
}

func TestAdaptChatJobAllowsJSONNullButRejectsMissingOrWrongColumns(t *testing.T) {
	key := []byte(strings.Repeat("k", sha256.Size))
	at := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	value := chatJobFixture(at, at)
	value["request_payload_json"] = nil
	value["accepted_payload_json"] = nil
	value["callback_payload_json"] = nil
	value["send_result_json"] = nil
	if fact, err := AdaptChatJob(chatJobRow(t, key, value), key); err != nil || string(fact.CallbackPayloadJSON) != "null" {
		t.Fatalf("JSON null was not preserved: fact=%#v err=%v", fact, err)
	}

	for name, mutate := range map[string]func(map[string]any){
		"missing JSON column": func(value map[string]any) { delete(value, "request_payload_json") },
		"extra column":        func(value map[string]any) { value["unexpected"] = true },
		"required null":       func(value map[string]any) { value["status"] = nil },
		"nullable ID type":    func(value map[string]any) { value["queue_id"] = "not-an-id" },
		"signed ID type":      func(value map[string]any) { value["id"] = "not-an-id" },
		"timestamp type":      func(value map[string]any) { value["created_at"] = "not-a-time" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := chatJobFixture(at, at)
			mutate(changed)
			if _, err := AdaptChatJob(chatJobRow(t, key, changed), key); err != ErrFact {
				t.Fatalf("invalid source accepted: err=%v", err)
			}
		})
	}
}

func TestAdaptChatJobRejectsArchiveAuthenticationOrdinalAndRedaction(t *testing.T) {
	key := []byte(strings.Repeat("k", sha256.Size))
	at := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	row := chatJobRow(t, key, chatJobFixture(at, at))
	for name, mutate := range map[string]func(*v1archive.ArchivedRow){
		"wrong adapter":   func(value *v1archive.ArchivedRow) { value.AdapterID = "wrong" },
		"wrong table":     func(value *v1archive.ArchivedRow) { value.TableID = "public/other" },
		"missing ordinal": func(value *v1archive.ArchivedRow) { value.SourceOrdinal = 0 },
		"source HMAC":     func(value *v1archive.ArchivedRow) { value.SourceKeyHMAC[0]++ },
		"payload HMAC":    func(value *v1archive.ArchivedRow) { value.PayloadHMAC[0]++ },
		"field HMAC":      func(value *v1archive.ArchivedRow) { value.FieldHMAC[0]++ },
	} {
		t.Run(name, func(t *testing.T) {
			changed := row
			mutate(&changed)
			if _, err := AdaptChatJob(changed, key); err != ErrArchiveRow {
				t.Fatalf("archive drift accepted: err=%v", err)
			}
		})
	}
	if _, err := AdaptChatJob(row, key[:sha256.Size-1]); err != ErrArchiveRow {
		t.Fatalf("short HMAC key accepted: err=%v", err)
	}

	redacted := chatJobFixture(at, at)
	redacted["request_payload_json"] = map[string]any{"access_token": "private"}
	redactedRow := chatJobRow(t, key, redacted)
	if len(redactedRow.RedactedFields) == 0 {
		t.Fatal("fixture did not create redaction")
	}
	if _, err := AdaptChatJob(redactedRow, key); err != ErrRequiredFieldRedacted {
		t.Fatalf("required redaction accepted: err=%v", err)
	}
}

func TestAdaptChatJobRejectsSourceKeyPayloadMismatch(t *testing.T) {
	key := []byte(strings.Repeat("k", sha256.Size))
	at := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	row := chatJobRow(t, key, chatJobFixture(at, at))

	var fields map[string]any
	if err := json.Unmarshal(row.Payload, &fields); err != nil {
		t.Fatal(err)
	}
	fields["id"] = int64(88)
	payload, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	row.Payload = payload
	payloadDigest, err := v1archive.PayloadHMAC(key, "automation_laohuang_chat_job", row.Payload)
	if err != nil {
		t.Fatal(err)
	}
	row.PayloadHMAC = payloadDigest
	if _, err = AdaptChatJob(row, key); err != ErrArchiveRow {
		t.Fatalf("source key mismatch accepted: err=%v", err)
	}
}

func chatJobFixture(created, updated time.Time) map[string]any {
	return map[string]any{
		"id": int64(9), "queue_id": nil, "member_id": int64(7),
		"external_contact_id": "external-contact-private", "phone": "phone-private", "external_message_id": "message-private", "external_session_id": "session-private", "laohuang_task_id": "task-private",
		"request_payload_json": map[string]any{"request": "private"}, "accepted_payload_json": map[string]any{"accepted": "private"}, "callback_payload_json": map[string]any{"callback": "private"},
		"status": "legacy_terminal", "reply_text": "reply-private", "error_code": "error-private", "error_message": "error-private", "send_channel": "legacy_channel",
		"send_record_id": nil, "send_result_json": map[string]any{"result": "private"}, "created_at": created, "updated_at": updated, "finished_at": "not-a-timestamp",
	}
}

func chatJobRow(t *testing.T, key []byte, value map[string]any) v1archive.ArchivedRow {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	payload, roots, err := v1archive.RedactPayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	keyJSON, err := json.Marshal([]json.RawMessage{fields["id"]})
	if err != nil {
		t.Fatal(err)
	}
	table := strings.TrimPrefix(ChatJobsTableID, "public/")
	source, err := v1archive.SourceKeyHMAC(key, table, keyJSON)
	if err != nil {
		t.Fatal(err)
	}
	payloadDigest, err := v1archive.PayloadHMAC(key, table, payload)
	if err != nil {
		t.Fatal(err)
	}
	field, err := v1archive.FieldHMAC(key, table, roots)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{
		AdapterID: v1archive.DefaultAdapterID, TableID: ChatJobsTableID, SourceOrdinal: 1,
		SourceKeyHMAC: source, PayloadHMAC: payloadDigest, FieldHMAC: field, Payload: payload, RedactedFields: roots,
	}
}

func TestChatJobManifestOrder(t *testing.T) {
	if got, want := strings.Join(chatJobFields, ","), "id,queue_id,member_id,external_contact_id,phone,external_message_id,external_session_id,laohuang_task_id,request_payload_json,accepted_payload_json,callback_payload_json,status,reply_text,error_code,error_message,send_channel,send_record_id,send_result_json,created_at,updated_at,finished_at"; got != want {
		t.Fatalf("manifest order changed: got=%s want=%s", got, want)
	}
	if len(chatJobFields) != 21 || strconv.IntSize != 64 {
		t.Fatalf("unexpected source-width or integer size: fields=%d int=%d", len(chatJobFields), strconv.IntSize)
	}
}
