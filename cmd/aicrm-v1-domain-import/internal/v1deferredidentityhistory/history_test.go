package v1deferredidentityhistory

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestAdaptDeferredIdentityFactsPreservesSafeFactsAndOpaqueEvidence(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	created := time.Date(2026, 8, 27, 12, 0, 0, 123456789, time.FixedZone("source", 8*60*60))
	personRow := deferredIdentityRow(t, key, PeopleTableID, -7, map[string]any{
		"id": int64(-7), "mobile": "13800138000", "third_party_user_id": "legacy-user", "created_at": created, "updated_at": created.Add(time.Minute),
	})
	person, err := AdaptPerson(personRow, key)
	if err != nil {
		t.Fatal(err)
	}
	if person.SourceID != -7 || person.MobileDigest == (OpaqueDigest{}) || person.ThirdPartyUserIDDigest == (OpaqueDigest{}) || person.PrivateDigest == (OpaqueDigest{}) {
		t.Fatalf("person fact = %#v", person)
	}
	if got := person.CreatedAt; !got.Equal(time.Date(2026, 8, 27, 4, 0, 0, 123456000, time.UTC)) {
		t.Fatalf("person timestamp = %s", got)
	}

	conflictRow := deferredIdentityRow(t, key, IdentityConflictsTableID, 0, map[string]any{
		"id": int64(0), "conflict_type": "duplicate", "unionid": "union-a", "candidate_unionid": "union-b", "external_userid": "ext", "openid": "open", "mobile": "13900139000",
		"source_type": "wecom", "source_key": "private-key", "payload_json": nil, "source_payload_json": map[string]any{"token": "hidden"},
		"status": "open", "resolution_status": "pending", "resolution_note": "private", "created_at": created, "updated_at": created.Add(time.Minute), "resolved_at": nil,
	})
	conflict, err := AdaptConflict(conflictRow, key)
	if err != nil {
		t.Fatal(err)
	}
	if conflict.SourceID != 0 || conflict.ResolvedAt != nil || conflict.ConflictType != "duplicate" || conflict.ResolutionStatus != "pending" || conflict.PayloadJSONDigest == (OpaqueDigest{}) || conflict.SourcePayloadDigest == (OpaqueDigest{}) {
		t.Fatalf("conflict fact = %#v", conflict)
	}

	mapRow := deferredIdentityRow(t, key, ExternalContactIdentityMapID, 9, map[string]any{
		"id": int64(9), "corp_id": "corp", "external_userid": "external", "unionid": "union", "openid": "open", "follow_user_userid": "staff", "name": "Jane", "type": nil,
		"avatar": "https://avatar.example", "gender": nil, "status": "active", "raw_profile": map[string]any{"api_token": "private"},
		"first_seen_at": created, "last_seen_at": created.Add(time.Hour), "created_at": created, "updated_at": created.Add(2 * time.Hour),
	})
	identity, err := AdaptMissingRootIdentity(mapRow, key)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Type != nil || identity.GenderDigest != nil || identity.Status != "active" || identity.RawProfileDigest == (OpaqueDigest{}) {
		t.Fatalf("identity map fact = %#v", identity)
	}
	if len(identity.RedactedRoots) != 1 || identity.RedactedRoots[0] != "raw_profile.api_token" {
		t.Fatalf("redacted roots = %#v", identity.RedactedRoots)
	}
	encoded, err := json.Marshal(identity)
	if err != nil || strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), "external") {
		t.Fatalf("identity fact leaked source text: %s err=%v", encoded, err)
	}
}

func TestAdaptDeferredIdentityRejectsArchiveIntegrityAndShapeDrift(t *testing.T) {
	key := bytes.Repeat([]byte{3}, 32)
	stamp := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	valid := deferredIdentityRow(t, key, PeopleTableID, 1, map[string]any{
		"id": int64(1), "mobile": "", "third_party_user_id": "", "created_at": stamp, "updated_at": stamp,
	})
	cases := map[string]v1archive.ArchivedRow{
		"wrong source HMAC":  func() v1archive.ArchivedRow { value := valid; value.SourceKeyHMAC[0]++; return value }(),
		"wrong payload HMAC": func() v1archive.ArchivedRow { value := valid; value.PayloadHMAC[0]++; return value }(),
		"wrong field HMAC":   func() v1archive.ArchivedRow { value := valid; value.FieldHMAC[0]++; return value }(),
		"wrong table":        func() v1archive.ArchivedRow { value := valid; value.TableID = IdentityConflictsTableID; return value }(),
		"wrong ordinal":      func() v1archive.ArchivedRow { value := valid; value.SourceOrdinal = 0; return value }(),
	}
	for name, row := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := AdaptPerson(row, key); !errors.Is(err, ErrArchiveRow) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	extra := deferredIdentityRow(t, key, PeopleTableID, 1, map[string]any{
		"id": int64(1), "mobile": "", "third_party_user_id": "", "created_at": stamp, "updated_at": stamp, "unexpected": true,
	})
	if _, err := AdaptPerson(extra, key); !errors.Is(err, ErrFact) {
		t.Fatalf("extra field error = %v", err)
	}
	missingRequired := deferredIdentityRow(t, key, IdentityConflictsTableID, 2, map[string]any{
		"id": int64(2), "conflict_type": "", "unionid": "", "candidate_unionid": "", "external_userid": "", "openid": "", "mobile": "", "source_type": "", "source_key": "",
		"payload_json": nil, "source_payload_json": nil, "status": "", "resolution_status": "", "resolution_note": "", "created_at": stamp, "updated_at": stamp, "resolved_at": nil,
	})
	if _, err := AdaptConflict(missingRequired, key); err != nil {
		t.Fatalf("json null fields must remain valid: %v", err)
	}
}

func TestAdaptMissingRootIdentityPreservesNullableTypedFields(t *testing.T) {
	key := bytes.Repeat([]byte{9}, 32)
	stamp := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	row := deferredIdentityRow(t, key, ExternalContactIdentityMapID, -4, map[string]any{
		"id": int64(-4), "corp_id": "", "external_userid": "", "unionid": "", "openid": "", "follow_user_userid": "", "name": "", "type": int32(-1),
		"avatar": "", "gender": int32(0), "status": "", "raw_profile": nil, "first_seen_at": stamp, "last_seen_at": stamp, "created_at": stamp, "updated_at": stamp,
	})
	fact, err := AdaptMissingRootIdentity(row, key)
	if err != nil {
		t.Fatal(err)
	}
	if fact.SourceID != -4 || fact.Type == nil || *fact.Type != -1 || fact.GenderDigest == nil || *fact.GenderDigest == (OpaqueDigest{}) {
		t.Fatalf("nullable map fields = %#v", fact)
	}
}

func deferredIdentityRow(t *testing.T, key []byte, table string, id int64, value map[string]any) v1archive.ArchivedRow {
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
	source, err := v1archive.SourceKeyHMAC(key, name, []byte("["+strconv.FormatInt(id, 10)+"]"))
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
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: 1, SourceKeyHMAC: source, PayloadHMAC: payloadHMAC, FieldHMAC: field, Payload: payload, RedactedFields: roots}
}
