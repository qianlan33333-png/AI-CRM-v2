package v1campaigndefinitionhistory

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var testSourceHMACKey = bytes.Repeat([]byte{0x51}, sha256.Size)

func TestAdaptDefinitionPreservesNonCurrentHistoricalFact(t *testing.T) {
	payload := definitionPayload(t, 0, map[string]any{
		"campaign_code": "",
		"review_status": "legacy_rejected",
		"run_status":    "paused_after_expiry",
		"started_at":    "2020-01-01T00:00:00Z",
		"created_at":    "2024-01-02T03:04:05.123456+08:00",
		"updated_at":    "2020-01-01T00:00:00Z",
		"paused_reason": "",
	})
	row := archivedRow(t, DefinitionTableID, 1, 0, payload)

	fact, err := AdaptDefinition(row, testSourceHMACKey)
	if err != nil {
		t.Fatalf("adapt definition: %v", err)
	}
	if fact.SourceID != 0 || fact.Code != "" || fact.ReviewStatus != "legacy_rejected" || fact.RunStatus != "paused_after_expiry" || fact.PausedReason != "" {
		t.Fatalf("historical fields changed: %#v", fact)
	}
	if fact.StartedAt == nil || fact.CreatedAt.Before(*fact.StartedAt) {
		t.Fatalf("historical time ordering was not preserved: %#v", fact)
	}
	if fact.Source.SourceKeyDigest != OpaqueDigest(row.SourceKeyHMAC) || fact.Source.PayloadDigest != OpaqueDigest(row.PayloadHMAC) || fact.Source.FieldDigest != OpaqueDigest(row.FieldHMAC) || fact.PrivateDigest == (OpaqueDigest{}) {
		t.Fatalf("archive envelope missing from fact: %#v", fact.Source)
	}
}

func TestAdaptStepAcceptsSignedSourceIDsAndMasksContent(t *testing.T) {
	payload := stepPayload(t, -7, map[string]any{
		"campaign_id":                   0,
		"campaign_segment_id":           -9,
		"step_index":                    -2,
		"day_offset":                    -4,
		"skip_if_recently_touched_days": -3,
		"content_text":                  "第一行 13800138000\n第二行",
		"send_time":                     "",
		"timezone":                      "",
	})
	row := archivedRow(t, StepTableID, 1, -7, payload)

	fact, err := AdaptStep(row, testSourceHMACKey)
	if err != nil {
		t.Fatalf("adapt step: %v", err)
	}
	if fact.SourceID != -7 || fact.CampaignSourceID != 0 || fact.SegmentSourceID != -9 || fact.StepIndex != -2 || fact.DayOffset != -4 || fact.SkipRecentDays != -3 {
		t.Fatalf("signed source values changed: %#v", fact)
	}
	if fact.ContentMasked != "第一行 [masked-phone]\n第二行" || fact.SendTime != "" || fact.Timezone != "" {
		t.Fatalf("display-safe source text mismatch: %#v", fact)
	}
	if fact.ContentDigest == (OpaqueDigest{}) || fact.PrivateDigest == (OpaqueDigest{}) {
		t.Fatalf("private digests missing: %#v", fact)
	}
}

func TestAdaptRejectsArchiveHMACTampering(t *testing.T) {
	definition := archivedRow(t, DefinitionTableID, 1, 41, definitionPayload(t, 41, nil))
	step := archivedRow(t, StepTableID, 1, 42, stepPayload(t, 42, nil))

	tests := []struct {
		name  string
		row   v1archive.ArchivedRow
		adapt func(v1archive.ArchivedRow, []byte) error
	}{
		{
			name: "payload",
			row: func() v1archive.ArchivedRow {
				changed := definition
				changed.Payload = append([]byte(nil), changed.Payload...)
				changed.Payload = bytes.Replace(changed.Payload, []byte("history-code"), []byte("changed-code"), 1)
				return changed
			}(),
			adapt: func(row v1archive.ArchivedRow, key []byte) error { _, err := AdaptDefinition(row, key); return err },
		},
		{
			name: "source_key",
			row: func() v1archive.ArchivedRow {
				changed := definition
				changed.SourceKeyHMAC = sourceKey(t, DefinitionTableID, 42)
				return changed
			}(),
			adapt: func(row v1archive.ArchivedRow, key []byte) error { _, err := AdaptDefinition(row, key); return err },
		},
		{
			name: "field",
			row: func() v1archive.ArchivedRow {
				changed := step
				changed.FieldHMAC[0] ^= 0xff
				return changed
			}(),
			adapt: func(row v1archive.ArchivedRow, key []byte) error { _, err := AdaptStep(row, key); return err },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.adapt(test.row, testSourceHMACKey); err == nil {
				t.Fatal("tampered archive row accepted")
			}
		})
	}
}

func TestAdaptRejectsArchiveIdentityMismatch(t *testing.T) {
	row := archivedRow(t, DefinitionTableID, 1, 9, definitionPayload(t, 9, nil))
	for _, test := range []struct {
		name   string
		mutate func(*v1archive.ArchivedRow)
	}{
		{name: "adapter", mutate: func(row *v1archive.ArchivedRow) { row.AdapterID = "other" }},
		{name: "table", mutate: func(row *v1archive.ArchivedRow) { row.TableID = StepTableID }},
		{name: "ordinal", mutate: func(row *v1archive.ArchivedRow) { row.SourceOrdinal = 0 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := row
			test.mutate(&changed)
			if _, err := AdaptDefinition(changed, testSourceHMACKey); err == nil {
				t.Fatal("archive identity mismatch accepted")
			}
		})
	}
}

func TestAdaptDefinitionKeepsSensitiveSourcePrivate(t *testing.T) {
	const token = "approval-token-must-not-leak"
	payload := definitionPayload(t, 3, map[string]any{
		"created_by_agent":    "",
		"created_by_session":  "",
		"trace_id":            "",
		"owner_userid":        "",
		"approval_token_hash": token,
		"approved_by":         "",
		"metadata_json":       map[string]any{"access_token": token},
		"stats_json":          nil,
	})
	row := archivedRow(t, DefinitionTableID, 1, 3, payload)

	fact, err := AdaptDefinition(row, testSourceHMACKey)
	if err != nil {
		t.Fatalf("adapt empty sensitive source: %v", err)
	}
	if fact.PrivateDigest == (OpaqueDigest{}) {
		t.Fatal("private digest is zero")
	}
	if got := string(row.Payload); strings.Contains(got, token) || !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("archive fixture did not redact token: %s", got)
	}
	if got := printableFact(fact); strings.Contains(got, token) || strings.Contains(got, "created_by_agent") {
		t.Fatalf("private material leaked into fact: %s", got)
	}
	if got := fact.RedactedRoots; len(got) != 2 || got[0] != "approval_token_hash" || got[1] != "metadata_json" {
		t.Fatalf("redaction roots not preserved: %#v", got)
	}
	if fact.ApprovedAt != nil || fact.StartedAt != nil || fact.FinishedAt != nil || fact.PausedAt != nil {
		t.Fatalf("nullable times changed: %#v", fact)
	}
}

func TestAdaptRejectsUnsafeDisplayText(t *testing.T) {
	payload := stepPayload(t, 7, map[string]any{"content_text": "safe\u0000unsafe"})
	row := archivedRow(t, StepTableID, 1, 7, payload)
	if _, err := AdaptStep(row, testSourceHMACKey); err == nil {
		t.Fatal("NUL display text accepted")
	}
}

func archivedRow(t *testing.T, table string, ordinal, id int64, payload []byte) v1archive.ArchivedRow {
	t.Helper()
	canonical, fields, err := v1archive.RedactPayload(payload)
	if err != nil {
		t.Fatalf("redact payload: %v", err)
	}
	source := sourceKey(t, table, id)
	payloadDigest, err := v1archive.PayloadHMAC(testSourceHMACKey, strings.TrimPrefix(table, "public/"), canonical)
	if err != nil {
		t.Fatalf("payload HMAC: %v", err)
	}
	fieldDigest, err := v1archive.FieldHMAC(testSourceHMACKey, strings.TrimPrefix(table, "public/"), fields)
	if err != nil {
		t.Fatalf("field HMAC: %v", err)
	}
	return v1archive.ArchivedRow{
		AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal,
		SourceKeyHMAC: source, PayloadHMAC: payloadDigest, FieldHMAC: fieldDigest,
		Payload: canonical, RedactedFields: fields,
	}
}

func sourceKey(t *testing.T, table string, id int64) [sha256.Size]byte {
	t.Helper()
	key, err := v1archive.SourceKeyHMAC(testSourceHMACKey, strings.TrimPrefix(table, "public/"), []byte("["+strconv.FormatInt(id, 10)+"]"))
	if err != nil {
		t.Fatalf("source key HMAC: %v", err)
	}
	return key
}

func definitionPayload(t *testing.T, id int64, overrides map[string]any) []byte {
	t.Helper()
	value := map[string]any{
		"id": id, "campaign_code": "history-code", "display_name": "历史定义", "intent": "保留历史", "anchor_mode": "legacy",
		"anchor_date": "2024-01-02", "review_status": "legacy", "run_status": "inactive", "created_by_agent": "agent", "created_by_session": "session",
		"trace_id": "trace", "owner_userid": "owner", "approval_token_hash": "token", "approved_by": "approver", "approved_at": nil,
		"started_at": nil, "finished_at": nil, "paused_at": nil, "paused_reason": "", "metadata_json": map[string]any{}, "stats_json": map[string]any{},
		"created_at": "2024-01-02T03:04:05.123456+08:00", "updated_at": "2024-01-02T03:04:06.123456+08:00",
	}
	for key, override := range overrides {
		value[key] = override
	}
	return marshalPayload(t, value)
}

func stepPayload(t *testing.T, id int64, overrides map[string]any) []byte {
	t.Helper()
	value := map[string]any{
		"id": id, "campaign_id": int64(8), "campaign_segment_id": int64(9), "step_index": int32(1), "day_offset": int32(0),
		"send_time": "09:00", "timezone": "Asia/Shanghai", "content_text": "历史内容", "content_payload_json": map[string]any{},
		"stop_on_reply": true, "skip_if_recently_touched_days": int32(0), "agent_run_id": "run", "created_at": "2024-01-02T03:04:05.123456+08:00",
		"updated_at": "2024-01-02T03:04:06.123456+08:00",
	}
	for key, override := range overrides {
		value[key] = override
	}
	return marshalPayload(t, value)
}

func marshalPayload(t *testing.T, value map[string]any) []byte {
	t.Helper()
	result, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return result
}

func printableFact(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
