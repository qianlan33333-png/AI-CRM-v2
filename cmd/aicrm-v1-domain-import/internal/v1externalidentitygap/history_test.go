package v1externalidentitygap

import (
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestAdaptPreservesOnlyAuthenticatedIdentityFacts(t *testing.T) {
	key := []byte("archive-source-key-32-bytes-long!!")
	for name, test := range map[string]struct {
		unionID *string
		route   RootRoute
	}{
		"null union remains unbound":            {route: RootRouteUnbound},
		"nonempty union requires verified root": {unionID: pointer("union-2"), route: RootRouteRequiresVerifiedRoot},
	} {
		t.Run(name, func(t *testing.T) {
			row := fixtureRow(t, key, 1, 7, "corp-a", "external-a", test.unionID)
			fact, err := Adapt(row, key)
			if err != nil {
				t.Fatal(err)
			}
			if fact.Scope != "wecom-corp:corp-a" || fact.ExternalUserID != "external-a" || fact.RootRoute != test.route || fact.SourceKeyHMAC != row.SourceKeyHMAC || fact.SourcePayloadHMAC != row.PayloadHMAC || fact.SourceFieldHMAC != row.FieldHMAC {
				t.Fatalf("unexpected fact: %#v", fact)
			}
			if (fact.UnionID == nil) != (test.unionID == nil) || fact.UnionID != nil && *fact.UnionID != *test.unionID {
				t.Fatalf("union routing was changed: %#v", fact)
			}
			encoded, err := json.Marshal(fact)
			if err != nil || string(encoded) != "{}" {
				t.Fatalf("fact must not serialize externally: %s err=%v", encoded, err)
			}
		})
	}

	empty := fixtureRawRow(t, key, 2, map[string]any{"id": 8, "corp_id": "corp-b", "external_userid": "external-b", "unionid": "", "updated_at": "2026-08-28T01:02:03Z"})
	fact, err := Adapt(empty, key)
	if err != nil || fact.UnionID != nil || fact.RootRoute != RootRouteUnbound {
		t.Fatalf("empty union must remain unbound: fact=%#v err=%v", fact, err)
	}
}

func TestAdaptFailsClosed(t *testing.T) {
	key := []byte("archive-source-key-32-bytes-long!!")
	valid := fixtureRow(t, key, 1, 7, "corp-a", "external-a", nil)
	for name, change := range map[string]func(*v1archive.ArchivedRow){
		"wrong adapter":      func(row *v1archive.ArchivedRow) { row.AdapterID = "wrong" },
		"wrong table":        func(row *v1archive.ArchivedRow) { row.TableID = "public/other" },
		"missing ordinal":    func(row *v1archive.ArchivedRow) { row.SourceOrdinal = 0 },
		"source hmac drift":  func(row *v1archive.ArchivedRow) { row.SourceKeyHMAC[0]++ },
		"payload hmac drift": func(row *v1archive.ArchivedRow) { row.PayloadHMAC[0]++ },
		"field hmac drift":   func(row *v1archive.ArchivedRow) { row.FieldHMAC[0]++ },
		"unknown redaction": func(row *v1archive.ArchivedRow) {
			setRedactions(t, key, row, []string{"raw_profile.api_token", "unknown"})
		},
		"required field redaction":    func(row *v1archive.ArchivedRow) { setRedactions(t, key, row, []string{"corp_id"}) },
		"invalid external whitespace": func(row *v1archive.ArchivedRow) { *row = fixtureRow(t, key, 1, 7, "corp-a", " external-a", nil) },
		"invalid union whitespace": func(row *v1archive.ArchivedRow) {
			*row = fixtureRow(t, key, 1, 7, "corp-a", "external-a", pointer(" union"))
		},
		"missing union field": func(row *v1archive.ArchivedRow) {
			*row = fixtureRawRow(t, key, 1, map[string]any{"id": 7, "corp_id": "corp-a", "external_userid": "external-a", "updated_at": "2026-08-28T01:02:03Z"})
		},
		"invalid update timestamp": func(row *v1archive.ArchivedRow) {
			*row = fixtureRawRow(t, key, 1, map[string]any{"id": 7, "corp_id": "corp-a", "external_userid": "external-a", "unionid": nil, "updated_at": "not-a-time"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			row := valid
			change(&row)
			_, err := Adapt(row, key)
			if !errors.Is(err, ErrInvalidSource) {
				t.Fatalf("expected fixed invalid source error, got %v", err)
			}
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Reason == "" {
				t.Fatalf("expected safe validation error, got %v", err)
			}
		})
	}
}

func fixtureRow(t *testing.T, key []byte, ordinal, id int64, corpID, externalUserID string, unionID *string) v1archive.ArchivedRow {
	t.Helper()
	payload := map[string]any{
		"id": id, "corp_id": corpID, "external_userid": externalUserID, "unionid": unionID,
		"updated_at": "2026-08-28T01:02:03Z", "raw_profile": map[string]any{"api_token": "never-copy"},
	}
	return fixtureRawRow(t, key, ordinal, payload)
}

func fixtureRawRow(t *testing.T, key []byte, ordinal int64, payload map[string]any) v1archive.ArchivedRow {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	canonical, fields, err := v1archive.RedactPayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	if value, ok := payload["id"].(int64); ok {
		id = value
	} else if value, ok := payload["id"].(int); ok {
		id = int64(value)
	}
	source, err := v1archive.SourceKeyHMAC(key, sourceTable, []byte("["+jsonNumber(id)+"]"))
	if err != nil {
		t.Fatal(err)
	}
	payloadHMAC, err := v1archive.PayloadHMAC(key, sourceTable, canonical)
	if err != nil {
		t.Fatal(err)
	}
	fieldHMAC, err := v1archive.FieldHMAC(key, sourceTable, fields)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: TableID, SourceOrdinal: ordinal, SourceKeyHMAC: source, PayloadHMAC: payloadHMAC, FieldHMAC: fieldHMAC, Payload: canonical, RedactedFields: fields}
}

func setRedactions(t *testing.T, key []byte, row *v1archive.ArchivedRow, fields []string) {
	t.Helper()
	fieldHMAC, err := v1archive.FieldHMAC(key, sourceTable, fields)
	if err != nil {
		t.Fatal(err)
	}
	row.RedactedFields, row.FieldHMAC = fields, fieldHMAC
}

func pointer(value string) *string { return &value }

func jsonNumber(value int64) string { return strconv.FormatInt(value, 10) }
