package v1customerstatehistory

import (
	"crypto/sha256"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestAdaptUserStatusCurrentPreservesSnapshotAndPrivateInputs(t *testing.T) {
	payload := []byte(`{"signup_status":"","signup_label_name":"L","customer_name_snapshot":"客户","owner_userid_snapshot":"owner","set_by_userid":"actor","set_at":"2026-08-28T11:22:33.123456789+08:00","wecom_tag_sync_status":"failed","wecom_tag_sync_error":"provider detail","status_flags_json":{"nested":[{"value":false}]},"created_at":"2026-08-28T03:22:33.123456789Z","updated_at":"2026-08-28T03:22:34.123456789Z","unionid":"union-1"}`)
	row := stateRow(UserStatusCurrentTableID, 1, payload)
	got := AdaptUserStatusCurrent(row)
	if got.Disposition != DispositionCandidate || got.Candidate == nil {
		t.Fatalf("adapt current = %+v", got)
	}
	fact := got.Candidate
	if fact.SignupStatus != "" || fact.SignupLabelName != "L" || fact.CustomerNameSnapshot != "客户" || fact.OwnerUserIDSnapshot != "owner" || fact.UnionID != "union-1" {
		t.Fatalf("scalar fidelity lost: %+v", fact)
	}
	if want := sha256.Sum256([]byte("actor")); fact.SetByUserIDDigest != want {
		t.Fatal("actor was not represented as a digest")
	}
	if want := sha256.Sum256([]byte("provider detail")); fact.WeComTagSyncErrorHash != want {
		t.Fatal("sync error was not represented as a digest")
	}
	if want := sha256.Sum256([]byte(`{"nested":[{"value":false}]}`)); fact.StatusFlagsDigest != want {
		t.Fatal("flags digest mismatch")
	}
	if want := time.Date(2026, 8, 28, 3, 22, 33, 123456000, time.UTC); !fact.SetAt.Equal(want) || fact.SetAt.Nanosecond() != 123456000 {
		t.Fatalf("set_at not canonical microsecond UTC: %s", fact.SetAt)
	}
	if fact.Envelope.SourceKeyDigest != row.SourceKeyHMAC || fact.Envelope.SourcePayloadDigest != row.PayloadHMAC || fact.Envelope.SourceFieldDigest != row.FieldHMAC {
		t.Fatal("archive envelope was not retained")
	}
}

func TestAdaptUserStatusHistoryAndTermTagPreserveZeroAndNegativeSourceValues(t *testing.T) {
	historyPayload := []byte(`{"id":-7,"old_signup_status":"old","new_signup_status":"new","old_label_name":"","new_label_name":"next","customer_name_snapshot":"c","owner_userid_snapshot":"o","set_by_userid":"actor","set_at":"2026-08-28T00:00:00Z","wecom_tag_sync_status":"ok","wecom_tag_sync_error":"","status_flags_json":null,"created_at":"2026-08-28T00:00:00Z","unionid":"u"}`)
	history := AdaptUserStatusHistory(stateRow(UserStatusHistoryTableID, 1, historyPayload))
	if history.Disposition != DispositionCandidate || history.Candidate == nil || history.Candidate.SourceID != -7 || history.Candidate.OldLabelName != "" {
		t.Fatalf("history source values were constrained: %+v", history)
	}
	tagPayload := []byte(`{"id":0,"tag_group_name":"g","tag_name":"t","class_term_no":-4,"class_term_label":"","is_active":false,"created_at":"2026-08-29T00:00:00Z","updated_at":"2026-08-28T00:00:00Z","strategy_id":"strategy","group_id":"group","tag_id":"tag"}`)
	tag := AdaptTermTagMapping(stateRow(TermTagMappingTableID, 1, tagPayload))
	if tag.Disposition != DispositionCandidate || tag.Candidate == nil || tag.Candidate.SourceID != 0 || tag.Candidate.ClassTermNo != -4 || tag.Candidate.IsActive || tag.Candidate.ClassTermLabel != "" {
		t.Fatalf("mapping source values were constrained: %+v", tag)
	}
	if !tag.Candidate.CreatedAt.After(tag.Candidate.UpdatedAt) {
		t.Fatal("source timestamp ordering was silently normalized")
	}
}

func TestAdaptRejectsEnvelopeAndRetainedFieldRedactionPaths(t *testing.T) {
	row := stateRow(UserStatusCurrentTableID, 1, currentPayload())
	for name, change := range map[string]func(*v1archive.ArchivedRow){
		"adapter":         func(value *v1archive.ArchivedRow) { value.AdapterID = "other" },
		"table":           func(value *v1archive.ArchivedRow) { value.TableID = "public/other" },
		"ordinal":         func(value *v1archive.ArchivedRow) { value.SourceOrdinal = 0 },
		"source_hmac":     func(value *v1archive.ArchivedRow) { value.SourceKeyHMAC = [sha256.Size]byte{} },
		"payload_hmac":    func(value *v1archive.ArchivedRow) { value.PayloadHMAC = [sha256.Size]byte{} },
		"field_hmac":      func(value *v1archive.ArchivedRow) { value.FieldHMAC = [sha256.Size]byte{} },
		"invalid_payload": func(value *v1archive.ArchivedRow) { value.Payload = []byte(`{"signup_status":`) },
	} {
		t.Run(name, func(t *testing.T) {
			changed := row
			change(&changed)
			if got := AdaptUserStatusCurrent(changed); got.Disposition != DispositionQuarantine || got.Reason != ReasonInvalidEnvelope {
				t.Fatalf("adapt = %+v", got)
			}
		})
	}
	for _, path := range []string{"unionid", "status_flags_json.items[0].token", "unknown[0].secret"} {
		changed := row
		changed.RedactedFields = []string{path}
		if got := AdaptUserStatusCurrent(changed); got.Disposition != DispositionQuarantine || got.Reason != ReasonRedactedField {
			t.Fatalf("redaction %q = %+v", path, got)
		}
	}
}

func TestAdaptRowsQuarantinesDuplicateArchiveSourceKeyWithoutSourceIDInference(t *testing.T) {
	first := stateRow(UserStatusHistoryTableID, 1, historyPayload(1))
	second := stateRow(UserStatusHistoryTableID, 2, historyPayload(0))
	second.SourceKeyHMAC = first.SourceKeyHMAC
	rows := AdaptUserStatusHistoryRows([]v1archive.ArchivedRow{first, second})
	for index, got := range rows {
		if got.Disposition != DispositionQuarantine || got.Reason != ReasonDuplicateSource || got.Candidate != nil {
			t.Fatalf("row %d duplicate handling = %+v", index, got)
		}
	}
}

func TestAdaptRejectsMissingOrNullRequiredValue(t *testing.T) {
	payload := map[string]any{}
	if err := json.Unmarshal(currentPayload(), &payload); err != nil {
		t.Fatal(err)
	}
	delete(payload, "unionid")
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got := AdaptUserStatusCurrent(stateRow(UserStatusCurrentTableID, 1, encoded)); got.Disposition != DispositionQuarantine || got.Reason != ReasonInvalidPayload {
		t.Fatalf("missing field = %+v", got)
	}
}

func currentPayload() []byte {
	return []byte(`{"signup_status":"s","signup_label_name":"l","customer_name_snapshot":"c","owner_userid_snapshot":"o","set_by_userid":"a","set_at":"2026-08-28T00:00:00Z","wecom_tag_sync_status":"ok","wecom_tag_sync_error":"","status_flags_json":[],"created_at":"2026-08-28T00:00:00Z","updated_at":"2026-08-28T00:00:00Z","unionid":"u"}`)
}

func historyPayload(id int64) []byte {
	return []byte(`{"id":` + jsonNumber(id) + `,"old_signup_status":"old","new_signup_status":"new","old_label_name":"old","new_label_name":"new","customer_name_snapshot":"c","owner_userid_snapshot":"o","set_by_userid":"a","set_at":"2026-08-28T00:00:00Z","wecom_tag_sync_status":"ok","wecom_tag_sync_error":"","status_flags_json":{},"created_at":"2026-08-28T00:00:00Z","unionid":"u"}`)
}

func jsonNumber(value int64) string {
	return strconv.FormatInt(value, 10)
}

func stateRow(table string, ordinal int64, payload []byte) v1archive.ArchivedRow {
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal,
		SourceKeyHMAC: sha256.Sum256([]byte(table + "/" + strconv.FormatInt(ordinal, 10))), PayloadHMAC: sha256.Sum256(payload),
		FieldHMAC: sha256.Sum256([]byte("fields/" + table)), Payload: payload}
}
