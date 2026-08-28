package v1contactreferencehistory

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestAdaptExternalContactBindingPreservesEveryManifestField(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	created := time.Date(2026, 8, 27, 12, 0, 0, 123456789, time.FixedZone("source", 8*60*60))
	row := contactReferenceRow(t, key, ExternalContactBindingsTableID, 4, map[string]any{
		"external_userid": "external-1", "person_id": int64(-8), "first_bound_by_userid": "binder", "first_owner_userid": "first-owner", "last_owner_userid": "last-owner",
		"created_at": created, "updated_at": created.Add(time.Minute),
	})
	fact, err := AdaptExternalContactBinding(row, key)
	if err != nil {
		t.Fatal(err)
	}
	if fact.ExternalUserID != "external-1" || fact.PersonID != -8 || fact.FirstBoundByUserID != "binder" || fact.FirstOwnerUserID != "first-owner" || fact.LastOwnerUserID != "last-owner" {
		t.Fatalf("binding fields not preserved: %#v", fact)
	}
	if got := fact.CreatedAt; !got.Equal(time.Date(2026, 8, 27, 4, 0, 0, 123456000, time.UTC)) {
		t.Fatalf("created_at = %s", got)
	}
	if fact.Source.SourceKeyDigest == ([32]byte{}) || fact.Source.PayloadDigest == ([32]byte{}) || fact.Source.FieldDigest == ([32]byte{}) {
		t.Fatalf("missing source envelope: %#v", fact.Source)
	}
}

func TestAdaptDirectoryMemberPreservesEveryManifestField(t *testing.T) {
	key := bytes.Repeat([]byte{2}, 32)
	stamp := time.Date(2026, 8, 27, 12, 0, 0, 987654321, time.FixedZone("source", -7*60*60))
	status := int32(-2)
	row := contactReferenceRow(t, key, AdminWeComDirectoryMembersTableID, 2, map[string]any{
		"id": int64(-9), "wecom_corpid": "corp-a", "wecom_userid": "user-a", "display_name": "display", "department_ids_json": "[3,1]", "position": "", "wecom_status": status,
		"is_active": false, "synced_at": stamp, "raw_payload_json": "{\"raw\":true}", "created_at": stamp.Add(time.Second), "updated_at": stamp.Add(2 * time.Second),
		"corp_id": "corp-b", "department_name": "dept", "mobile": "13800138000", "avatar_url": "https://avatar.invalid/a", "first_seen_at": stamp.Add(3 * time.Second), "last_synced_at": stamp.Add(4 * time.Second), "updated_by": "operator",
	})
	fact, err := AdaptDirectoryMember(row, key)
	if err != nil {
		t.Fatal(err)
	}
	if fact.ID != -9 || fact.WeComCorpID != "corp-a" || fact.WeComUserID != "user-a" || fact.DisplayName != "display" || fact.DepartmentIDsJSON != "[3,1]" || fact.Position != "" || fact.WeComStatus == nil || *fact.WeComStatus != -2 || fact.IsActive || fact.RawPayloadJSON != "{\"raw\":true}" || fact.CorpID != "corp-b" || fact.DepartmentName != "dept" || fact.Mobile != "13800138000" || fact.AvatarURL != "https://avatar.invalid/a" || fact.UpdatedBy != "operator" {
		t.Fatalf("directory fields not preserved: %#v", fact)
	}
	for name, got := range map[string]time.Time{
		"synced_at": fact.SyncedAt, "created_at": fact.CreatedAt, "updated_at": fact.UpdatedAt, "first_seen_at": fact.FirstSeenAt, "last_synced_at": fact.LastSyncedAt,
	} {
		if got.Location() != time.UTC || got.Nanosecond()%1000 != 0 {
			t.Fatalf("%s not UTC microsecond normalized: %s", name, got)
		}
	}
}

func TestAdaptContactReferenceRejectsIntegrityShapeAndRedactionDrift(t *testing.T) {
	key := bytes.Repeat([]byte{3}, 32)
	stamp := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	valid := contactReferenceRow(t, key, ExternalContactBindingsTableID, 1, map[string]any{
		"external_userid": "", "person_id": int64(0), "first_bound_by_userid": "", "first_owner_userid": "", "last_owner_userid": "", "created_at": stamp, "updated_at": stamp,
	})
	for name, row := range map[string]v1archive.ArchivedRow{
		"source hmac":  func() v1archive.ArchivedRow { value := valid; value.SourceKeyHMAC[0]++; return value }(),
		"payload hmac": func() v1archive.ArchivedRow { value := valid; value.PayloadHMAC[0]++; return value }(),
		"field hmac":   func() v1archive.ArchivedRow { value := valid; value.FieldHMAC[0]++; return value }(),
		"ordinal":      func() v1archive.ArchivedRow { value := valid; value.SourceOrdinal = 0; return value }(),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := AdaptExternalContactBinding(row, key); !errors.Is(err, ErrArchiveRow) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	extra := contactReferenceRow(t, key, ExternalContactBindingsTableID, 1, map[string]any{
		"external_userid": "", "person_id": int64(0), "first_bound_by_userid": "", "first_owner_userid": "", "last_owner_userid": "", "created_at": stamp, "updated_at": stamp, "extra": true,
	})
	if _, err := AdaptExternalContactBinding(extra, key); !errors.Is(err, ErrFact) {
		t.Fatalf("extra field error = %v", err)
	}
	missing := contactReferenceRow(t, key, AdminWeComDirectoryMembersTableID, 2, directoryPayload(stamp))
	var fields map[string]any
	if err := json.Unmarshal(missing.Payload, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "updated_by")
	missing = contactReferenceRow(t, key, AdminWeComDirectoryMembersTableID, 2, fields)
	if _, err := AdaptDirectoryMember(missing, key); !errors.Is(err, ErrFact) {
		t.Fatalf("missing field error = %v", err)
	}
	redacted := contactReferenceRow(t, key, ExternalContactBindingsTableID, 1, map[string]any{
		"external_userid": "external-1", "person_id": int64(0), "first_bound_by_userid": "", "first_owner_userid": "", "last_owner_userid": "", "created_at": stamp, "updated_at": stamp, "token": "secret",
	})
	if _, err := AdaptExternalContactBinding(redacted, key); !errors.Is(err, ErrRequiredFieldRedacted) {
		t.Fatalf("redacted field error = %v", err)
	}
}

func TestAdaptDirectoryMemberPreservesNilSeparatelyFromEmpty(t *testing.T) {
	key := bytes.Repeat([]byte{4}, 32)
	stamp := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	payload := directoryPayload(stamp)
	payload["wecom_status"] = nil
	payload["display_name"] = ""
	fact, err := AdaptDirectoryMember(contactReferenceRow(t, key, AdminWeComDirectoryMembersTableID, 1, payload), key)
	if err != nil {
		t.Fatal(err)
	}
	if fact.WeComStatus != nil || fact.DisplayName != "" {
		t.Fatalf("nullable/empty distinction lost: %#v", fact)
	}
	encoded, err := json.Marshal(fact)
	if err != nil || strings.Contains(string(encoded), "wecom") {
		t.Fatalf("private fact leaked through JSON: %s err=%v", encoded, err)
	}
}

func directoryPayload(stamp time.Time) map[string]any {
	return map[string]any{
		"id": int64(1), "wecom_corpid": "", "wecom_userid": "", "display_name": "", "department_ids_json": "", "position": "", "wecom_status": int32(0), "is_active": false,
		"synced_at": stamp, "raw_payload_json": "", "created_at": stamp, "updated_at": stamp, "corp_id": "", "department_name": "", "mobile": "", "avatar_url": "", "first_seen_at": stamp, "last_synced_at": stamp, "updated_by": "",
	}
}

func contactReferenceRow(t *testing.T, key []byte, table string, ordinal int64, value map[string]any) v1archive.ArchivedRow {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	payload, roots, err := v1archive.RedactPayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	name := strings.TrimPrefix(table, "public/")
	var sourceKey []byte
	if table == ExternalContactBindingsTableID {
		var decoded map[string]json.RawMessage
		if err := json.Unmarshal(payload, &decoded); err != nil {
			t.Fatal(err)
		}
		sourceKey, err = json.Marshal([]json.RawMessage{decoded["external_userid"]})
	} else {
		var decoded map[string]json.RawMessage
		if err := json.Unmarshal(payload, &decoded); err != nil {
			t.Fatal(err)
		}
		sourceKey, err = json.Marshal([]json.RawMessage{decoded["id"]})
	}
	if err != nil {
		t.Fatal(err)
	}
	source, err := v1archive.SourceKeyHMAC(key, name, sourceKey)
	if err != nil {
		t.Fatal(err)
	}
	payloadHMAC, err := v1archive.PayloadHMAC(key, name, payload)
	if err != nil {
		t.Fatal(err)
	}
	field, err := v1archive.FieldHMAC(key, name, roots)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal, SourceKeyHMAC: source, PayloadHMAC: payloadHMAC, FieldHMAC: field, Payload: payload, RedactedFields: roots}
}
