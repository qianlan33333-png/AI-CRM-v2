package v1membergridhistory

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestUsageSnapshotRecoveryAdaptsTrueAndFalseWithoutMutatingArchive(t *testing.T) {
	for _, want := range []bool{false, true} {
		t.Run(map[bool]string{false: "false", true: "true"}[want], func(t *testing.T) {
			original, sourceKeyJSON, fullPayload, key := usageRecoveryFixture(t, want)
			before := cloneArchivedRow(original)
			entry, err := BuildUsageSnapshotRecoveryEntry(original, sourceKeyJSON, fullPayload, key, FixedUsageSnapshotRecoveryScope())
			if err != nil {
				t.Fatal(err)
			}
			decision, err := AdaptUsageSnapshotRecovery(original, entry, key)
			if err != nil {
				t.Fatal(err)
			}
			if decision.Disposition != DispositionCandidate || decision.Record == nil || decision.Record.HasTokenUsage != want {
				t.Fatalf("decision=%#v want candidate bool=%t", decision, want)
			}
			if !sameArchivedRow(before, original) {
				t.Fatal("immutable archive row mutated")
			}
		})
	}
}

func TestUsageSnapshotRecoveryRejectsNonBooleanAndAdditionalRedaction(t *testing.T) {
	original, sourceKeyJSON, fullPayload, key := usageRecoveryFixture(t, true)
	for _, payload := range [][]byte{
		bytes.Replace(fullPayload, []byte(`true`), []byte(`"true"`), 1),
		append(append([]byte(nil), fullPayload[:len(fullPayload)-1]...), []byte(`,"other_token":"x"}`)...),
	} {
		if _, err := BuildUsageSnapshotRecoveryEntry(original, sourceKeyJSON, payload, key, FixedUsageSnapshotRecoveryScope()); err == nil {
			t.Fatal("unsafe recovered payload accepted")
		}
	}
}

func TestUsageSnapshotRecoveryRejectsDigestScopeAndEntryTampering(t *testing.T) {
	original, sourceKeyJSON, fullPayload, key := usageRecoveryFixture(t, true)
	entry, err := BuildUsageSnapshotRecoveryEntry(original, sourceKeyJSON, fullPayload, key, FixedUsageSnapshotRecoveryScope())
	if err != nil {
		t.Fatal(err)
	}

	changedOriginal := original
	changedOriginal.Payload = bytes.Replace(changedOriginal.Payload, []byte(`"plan"`), []byte(`"other-plan"`), 1)
	if _, err := BuildUsageSnapshotRecoveryEntry(changedOriginal, sourceKeyJSON, fullPayload, key, FixedUsageSnapshotRecoveryScope()); err == nil {
		t.Fatal("changed archive payload accepted")
	}
	if _, err := AdaptUsageSnapshotRecovery(changedOriginal, entry, key); err == nil {
		t.Fatal("changed archive payload accepted by recovery adapter")
	}
	if _, err := BuildUsageSnapshotRecoveryEntry(original, sourceKeyJSON, fullPayload, append([]byte(nil), key[:31]...), FixedUsageSnapshotRecoveryScope()); err == nil {
		t.Fatal("wrong archive source HMAC key accepted")
	}
	wrongScope := FixedUsageSnapshotRecoveryScope()
	wrongScope.ArchiveRunID = "other-run"
	if _, err := BuildUsageSnapshotRecoveryEntry(original, sourceKeyJSON, fullPayload, key, wrongScope); err == nil {
		t.Fatal("wrong recovery scope accepted")
	}

	for _, mutate := range []func(*UsageSnapshotRecoveryEntry){
		func(value *UsageSnapshotRecoveryEntry) { value.HasTokenUsage = false },
		func(value *UsageSnapshotRecoveryEntry) { value.SourceKeyHMAC[0]++ },
		func(value *UsageSnapshotRecoveryEntry) { value.Scope.DumpSHA256 = "other-dump" },
	} {
		changed := entry
		mutate(&changed)
		if _, err := AdaptUsageSnapshotRecovery(original, changed, key); err == nil {
			t.Fatal("tampered recovery entry accepted")
		}
	}
}

func TestUsageSnapshotRecoveryRejectsWrongSourceIdentifierAndOriginalRedaction(t *testing.T) {
	original, sourceKeyJSON, fullPayload, key := usageRecoveryFixture(t, true)
	changedSource := append([]byte(nil), sourceKeyJSON...)
	changedSource[len(changedSource)-2] = 'x'
	if _, err := BuildUsageSnapshotRecoveryEntry(original, changedSource, fullPayload, key, FixedUsageSnapshotRecoveryScope()); err == nil {
		t.Fatal("wrong source key JSON accepted")
	}
	entry, err := BuildUsageSnapshotRecoveryEntry(original, sourceKeyJSON, fullPayload, key, FixedUsageSnapshotRecoveryScope())
	if err != nil {
		t.Fatal(err)
	}
	changedOriginal := original
	changedOriginal.RedactedFields = []string{"has_token_usage", "other"}
	if _, err := AdaptUsageSnapshotRecovery(changedOriginal, entry, key); err == nil {
		t.Fatal("additional original redaction accepted")
	}
}

func usageRecoveryFixture(t *testing.T, hasTokenUsage bool) (v1archive.ArchivedRow, []byte, []byte, []byte) {
	t.Helper()
	key := bytes.Repeat([]byte{7}, sha256.Size)
	sourceKeyJSON := []byte(`["private-user"]`)
	fullPayload, err := json.Marshal(map[string]any{
		"huangyoucan_user_id":   "private-user",
		"unionid":               "private-union",
		"mobile_md5":            "private-md5",
		"formally_logged_in":    true,
		"has_token_usage":       hasTokenUsage,
		"learning_plan_id":      "plan",
		"learning_plan_current": nil,
		"learning_plan_total":   nil,
		"open_count_7d":         1,
		"last_open_at":          nil,
		"refreshed_at":          "2026-08-01T01:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	redacted, fields, err := v1archive.RedactPayload(fullPayload)
	if err != nil {
		t.Fatal(err)
	}
	source, err := v1archive.SourceKeyHMAC(key, usageSnapshotRecoverySourceTable, sourceKeyJSON)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := v1archive.PayloadHMAC(key, usageSnapshotRecoverySourceTable, redacted)
	if err != nil {
		t.Fatal(err)
	}
	field, err := v1archive.FieldHMAC(key, usageSnapshotRecoverySourceTable, fields)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{
		AdapterID: v1archive.DefaultAdapterID, TableID: UsageSnapshotsTableID, SourceOrdinal: 1,
		SourceKeyHMAC: source, PayloadHMAC: payload, FieldHMAC: field, Payload: redacted, RedactedFields: fields,
	}, sourceKeyJSON, fullPayload, key
}

func cloneArchivedRow(value v1archive.ArchivedRow) v1archive.ArchivedRow {
	value.Payload = append([]byte(nil), value.Payload...)
	value.RedactedFields = append([]string(nil), value.RedactedFields...)
	return value
}

func sameArchivedRow(left, right v1archive.ArchivedRow) bool {
	return left.AdapterID == right.AdapterID && left.TableID == right.TableID && left.SourceOrdinal == right.SourceOrdinal &&
		left.SourceKeyHMAC == right.SourceKeyHMAC && left.PayloadHMAC == right.PayloadHMAC && left.FieldHMAC == right.FieldHMAC &&
		bytes.Equal(left.Payload, right.Payload) && len(left.RedactedFields) == len(right.RedactedFields) &&
		left.RedactedFields[0] == right.RedactedFields[0]
}
