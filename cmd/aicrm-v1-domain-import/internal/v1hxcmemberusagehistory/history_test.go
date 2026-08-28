package v1hxcmemberusagehistory

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var adapterTestKey = []byte("01234567890123456789012345678901")

func TestAdaptMemberUsageObservationPreservesAllManifestFields(t *testing.T) {
	row := memberUsageRow(t, map[string]any{
		"generation": int64(-7), "unionid": "union-private", "owner_userid": "", "mobile_hash": "hash-private",
		"is_member": false, "is_registered": true, "registered_at": "2026-08-01T08:00:00.123456789+08:00", "has_real_usage": true,
		"first_used_at": nil, "last_used_at": "2026-08-02T00:00:00.9999999Z", "member_since": nil, "membership_expires_at": "2026-08-03T00:00:00Z",
		"membership_tier": "", "membership_status": "paused", "membership_source": "legacy", "registration_source": "", "usage_source": "chat",
		"updated_at": nil, "payload_json": nil, "projected_at": "2026-08-04T00:00:00.123456789Z",
	})
	result := AdaptMemberUsageObservation(row, adapterTestKey, 1)
	if result.Disposition != DispositionCandidate || result.Fact == nil || result.Reason != "" {
		t.Fatalf("result = %#v", result)
	}
	fact := result.Fact
	if fact.Generation != -7 || fact.IsMember || !fact.IsRegistered || !fact.HasRealUsage || fact.ResolverUnionID() != "union-private" || fact.LegacyOwnerUserID() != "" || fact.MobileHash() != "hash-private" {
		t.Fatalf("fact lost signed/private/bool source values: %#v", fact)
	}
	if fact.RegisteredAt == nil || fact.RegisteredAt.Format(time.RFC3339Nano) != "2026-08-01T00:00:00.123456Z" || fact.FirstUsedAt != nil || fact.LastUsedAt == nil || fact.LastUsedAt.Format(time.RFC3339Nano) != "2026-08-02T00:00:00.999999Z" || fact.MemberSince != nil || fact.MembershipExpiresAt == nil || fact.UpdatedAt != nil || fact.ProjectedAt.Format(time.RFC3339Nano) != "2026-08-04T00:00:00.123456Z" {
		t.Fatalf("time fields = %#v", fact)
	}
	if string(fact.PayloadJSON) != "null" || fact.MembershipTier != "" || fact.MembershipStatus != "paused" || fact.MembershipSource != "legacy" || fact.RegistrationSource != "" || fact.UsageSource != "chat" {
		t.Fatalf("text/json fields = %#v", fact)
	}
	encoded, err := json.Marshal(fact)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"union-private", "hash-private", "payload_json", "SourceKeyHMAC"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("private value leaked in JSON: %s", private)
		}
	}
}

func TestAdaptMemberUsageObservationAuthenticatesCanonicalArchiveEnvelope(t *testing.T) {
	row := memberUsageRow(t, nil)
	for name, mutate := range map[string]func(*v1archive.ArchivedRow){
		"source key": func(row *v1archive.ArchivedRow) { row.SourceKeyHMAC[0]++ },
		"payload":    func(row *v1archive.ArchivedRow) { row.PayloadHMAC[0]++ },
		"field":      func(row *v1archive.ArchivedRow) { row.FieldHMAC[0]++ },
		"ordinal":    func(row *v1archive.ArchivedRow) { row.SourceOrdinal++ },
		"wrong key": func(row *v1archive.ArchivedRow) {
			row.Payload = append([]byte(nil), row.Payload...)
			row.Payload[0] = '['
		},
	} {
		t.Run(name, func(t *testing.T) {
			copy := cloneRow(row)
			mutate(&copy)
			result := AdaptMemberUsageObservation(copy, adapterTestKey, 1)
			if result.Disposition != DispositionQuarantine || result.Reason != ReasonInvalidSourceEnvelope || result.Fact != nil {
				t.Fatalf("result = %#v", result)
			}
		})
	}

	wrongOrder := cloneRow(row)
	var source sourceJSON
	if err := json.Unmarshal(wrongOrder.Payload, &source); err != nil {
		t.Fatal(err)
	}
	keyJSON, err := memberUsageSourceKeyJSON(source.Generation, source.OwnerUserID, source.UnionID)
	if err != nil {
		t.Fatal(err)
	}
	key, err := v1archive.SourceKeyHMAC(adapterTestKey, "ai_audience_hxc_member_usage_projection", keyJSON)
	if err != nil {
		t.Fatal(err)
	}
	wrongOrder.SourceKeyHMAC = key
	result := AdaptMemberUsageObservation(wrongOrder, adapterTestKey, 1)
	if result.Disposition != DispositionQuarantine || result.Reason != ReasonInvalidSourceEnvelope {
		t.Fatalf("wrong canonical PK order accepted: %#v", result)
	}

	compact := cloneRow(row)
	keyJSON, err = json.Marshal([]any{source.Generation, source.UnionID, source.OwnerUserID})
	if err != nil {
		t.Fatal(err)
	}
	key, err = v1archive.SourceKeyHMAC(adapterTestKey, "ai_audience_hxc_member_usage_projection", keyJSON)
	if err != nil {
		t.Fatal(err)
	}
	compact.SourceKeyHMAC = key
	if result = AdaptMemberUsageObservation(compact, adapterTestKey, 1); result.Disposition != DispositionQuarantine || result.Reason != ReasonInvalidSourceEnvelope {
		t.Fatalf("compact source-key JSON accepted: %#v", result)
	}
}

func TestMemberUsageSourceKeyJSONMatchesPostgresJSONBText(t *testing.T) {
	actual, err := memberUsageSourceKeyJSON(-7, `union "quoted" <tag>\path`, `owner\slash`)
	if err != nil {
		t.Fatal(err)
	}
	const want = `[-7, "union \"quoted\" <tag>\\path", "owner\\slash"]`
	if string(actual) != want {
		t.Fatalf("source key JSON = %q, want %q", actual, want)
	}
}

func TestAdaptMemberUsageObservationDistinguishesNullEmptyAndRedaction(t *testing.T) {
	base := map[string]any{"owner_userid": ""}
	if result := AdaptMemberUsageObservation(memberUsageRow(t, base), adapterTestKey, 1); result.Disposition != DispositionCandidate {
		t.Fatalf("empty required text must preserve source value: %#v", result)
	}

	nullOwner := AdaptMemberUsageObservation(memberUsageRow(t, map[string]any{"owner_userid": nil}), adapterTestKey, 1)
	if nullOwner.Disposition != DispositionQuarantine || nullOwner.Reason != ReasonInvalidSourcePayload {
		t.Fatalf("required null = %#v", nullOwner)
	}

	redacted := memberUsageRow(t, nil)
	redacted.RedactedFields = []string{"owner_userid"}
	redacted = authenticateRow(t, redacted)
	result := AdaptMemberUsageObservation(redacted, adapterTestKey, 1)
	if result.Disposition != DispositionQuarantine || result.Reason != ReasonRequiredFieldRedacted {
		t.Fatalf("required redaction = %#v", result)
	}

	unknown := memberUsageRow(t, nil)
	unknown.RedactedFields = []string{"not_a_manifest_field"}
	unknown = authenticateRow(t, unknown)
	result = AdaptMemberUsageObservation(unknown, adapterTestKey, 1)
	if result.Disposition != DispositionQuarantine || result.Reason != ReasonUnknownRedactedField {
		t.Fatalf("unknown redaction = %#v", result)
	}

	nested := memberUsageRow(t, nil)
	nested.RedactedFields = []string{"payload_json.secret"}
	nested = authenticateRow(t, nested)
	result = AdaptMemberUsageObservation(nested, adapterTestKey, 1)
	if result.Disposition != DispositionQuarantine || result.Reason != ReasonRequiredFieldRedacted {
		t.Fatalf("nested redaction = %#v", result)
	}
}

func TestAdaptMemberUsageObservationRejectsMissingExtraAndInvalidTypedFields(t *testing.T) {
	for name, change := range map[string]func(map[string]any){
		"missing": func(value map[string]any) { delete(value, "usage_source") },
		"extra":   func(value map[string]any) { value["unexpected"] = true },
		"bad boolean": func(value map[string]any) {
			value["has_real_usage"] = "true"
		},
		"bad timestamp": func(value map[string]any) { value["projected_at"] = "not-a-time" },
		"invalid json":  func(value map[string]any) { value["payload_json"] = make(chan int) },
	} {
		t.Run(name, func(t *testing.T) {
			value := memberUsagePayload()
			change(value)
			var raw []byte
			var err error
			if name == "invalid json" {
				raw = []byte(strings.Replace(string(mustJSON(t, memberUsagePayload())), `"payload_json":{"safe":true}`, `"payload_json":`, 1))
			} else {
				raw, err = json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
			}
			row := authenticateRow(t, v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: MemberUsageProjectionTableID, SourceOrdinal: 1, Payload: raw})
			result := AdaptMemberUsageObservation(row, adapterTestKey, 1)
			wantReason := ReasonInvalidSourcePayload
			if name == "invalid json" {
				wantReason = ReasonInvalidSourceEnvelope
			}
			if result.Disposition != DispositionQuarantine || result.Reason != wantReason || result.Fact != nil {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func memberUsageRow(t *testing.T, override map[string]any) v1archive.ArchivedRow {
	t.Helper()
	value := memberUsagePayload()
	for key, item := range override {
		value[key] = item
	}
	return authenticateRow(t, v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: MemberUsageProjectionTableID, SourceOrdinal: 1, Payload: mustJSON(t, value)})
}

func memberUsagePayload() map[string]any {
	return map[string]any{
		"generation": int64(1), "unionid": "unionid-private", "owner_userid": "owner-private", "mobile_hash": "mobile-hash-private",
		"is_member": true, "is_registered": false, "registered_at": nil, "has_real_usage": false,
		"first_used_at": nil, "last_used_at": nil, "member_since": nil, "membership_expires_at": nil,
		"membership_tier": "tier", "membership_status": "status", "membership_source": "membership", "registration_source": "registration", "usage_source": "usage",
		"updated_at": nil, "payload_json": map[string]any{"safe": true}, "projected_at": "2026-08-01T00:00:00Z",
	}
}

func authenticateRow(t *testing.T, row v1archive.ArchivedRow) v1archive.ArchivedRow {
	t.Helper()
	var value sourceJSON
	if json.Unmarshal(row.Payload, &value) != nil {
		row.SourceKeyHMAC = [32]byte{1}
		row.PayloadHMAC = [32]byte{2}
		row.FieldHMAC = [32]byte{3}
		return row
	}
	keyJSON, err := memberUsageSourceKeyJSON(value.Generation, value.UnionID, value.OwnerUserID)
	if err != nil {
		t.Fatal(err)
	}
	key, err := v1archive.SourceKeyHMAC(adapterTestKey, "ai_audience_hxc_member_usage_projection", keyJSON)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := v1archive.PayloadHMAC(adapterTestKey, "ai_audience_hxc_member_usage_projection", row.Payload)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := v1archive.FieldHMAC(adapterTestKey, "ai_audience_hxc_member_usage_projection", row.RedactedFields)
	if err != nil {
		t.Fatal(err)
	}
	row.SourceKeyHMAC, row.PayloadHMAC, row.FieldHMAC = key, payload, fields
	return row
}

func cloneRow(row v1archive.ArchivedRow) v1archive.ArchivedRow {
	row.Payload = append([]byte(nil), row.Payload...)
	row.RedactedFields = append([]string(nil), row.RedactedFields...)
	return row
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
