package v1radarhistory

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAdaptClickPreservesOnlyInertFacts(t *testing.T) {
	click := clickFixture()
	result := AdaptClick(rawClick(t, click))
	if result.Disposition != DispositionCandidate || result.Fact == nil || result.Reason != "" {
		t.Fatal("valid radar click was not a candidate")
	}
	fact := result.Fact
	if fact.SourceID != 11 || fact.LinkSourceID != 8 || fact.Code != "r-v1-code" || fact.RawStage != "legacy_open" || fact.SourceChannel != "campaign" || fact.TargetTypeSnapshot != "pdf" || fact.ErrorCode != "" {
		t.Fatal("non-PII source fact changed")
	}
	if fact.CreatedAt.Format(time.RFC3339Nano) != "2026-08-28T08:09:10.123456+08:00" {
		t.Fatal("source time changed")
	}
	if fact.Source.UnionID != "unionid-private" || fact.Sensitive.OpenID == (OpaqueDigest{}) || fact.Sensitive.QueryParams == (OpaqueDigest{}) {
		t.Fatal("private identity or field digests missing")
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"unionid-private", "openid-private", "external-private", "campaign-private", "staff-private", "agent-private", "127.0.0.1", "person-private", "referrer-private", "query-private"} {
		if strings.Contains(string(encoded), raw) {
			t.Fatal("sensitive source material escaped candidate JSON")
		}
	}
}

func TestAdaptClicksQuarantinesBadShapeAndAmbiguousSource(t *testing.T) {
	invalid := clickFixture()
	invalid["stage"] = 3
	decision := AdaptClick(rawClick(t, invalid))
	if decision.Disposition != DispositionQuarantine || decision.Reason != "radar_click_shape_invalid" || decision.SourceID != 11 || decision.Fact != nil {
		t.Fatal("invalid source type did not quarantine")
	}
	row := rawClick(t, clickFixture())
	duplicate := AdaptClicks([]json.RawMessage{row, row})
	for _, decision := range duplicate {
		if decision.Disposition != DispositionQuarantine || decision.Reason != "radar_click_source_ambiguous" || decision.SourceID != 11 || decision.Fact != nil {
			t.Fatal("duplicate source click did not quarantine")
		}
	}
	missing := clickFixture()
	delete(missing, "query_params_json")
	if decision := AdaptClick(rawClick(t, missing)); decision.Disposition != DispositionQuarantine || decision.Reason != "radar_click_shape_invalid" {
		t.Fatal("missing required JSON field was accepted")
	}
}

func clickFixture() map[string]any {
	stamp := time.Date(2026, 8, 28, 8, 9, 10, 123456000, time.FixedZone("v1-source", 8*60*60))
	return map[string]any{
		"id": int64(11), "link_id": int64(8), "code": "r-v1-code", "stage": "legacy_open", "openid": "openid-private", "unionid": "unionid-private", "external_userid": "external-private",
		"source_channel": "campaign", "campaign_id": "campaign-private", "staff_id": "staff-private", "user_agent": "agent-private", "ip": "127.0.0.1", "created_at": stamp,
		"target_type_snapshot": "pdf", "person_id": "person-private", "ip_hash": "hash-private", "source_channel_snapshot": "campaign-snapshot", "campaign_id_snapshot": "campaign-private", "staff_id_snapshot": "staff-private",
		"referer": "referrer-private", "query_params_json": map[string]any{"q": "query-private"}, "error_code": "",
	}
}

func rawClick(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
