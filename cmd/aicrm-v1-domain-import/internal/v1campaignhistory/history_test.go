package v1campaignhistory

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var campaignFixture = json.RawMessage(`{
  "id":1,"campaign_code":"  repeated-code  ","display_name":" 历史父 ","intent":"原始意图",
  "anchor_mode":"legacy_mode","anchor_date":"源日期标签","review_status":"legacy_review","run_status":"active",
  "created_by_agent":" agent ","created_by_session":" private-session ","trace_id":"private-trace",
  "owner_userid":" private-owner ","approved_by":" private-approver ","approval_token_hash":"private-credential",
  "approved_at":"2026-08-28T01:00:00.123456+08:00","started_at":null,"finished_at":"2026-08-27T01:00:00+08:00","paused_at":null,
  "paused_reason":" 原因 ","metadata_json":{"sql_query":"never execute","access_token":"private-token"},"stats_json":{"sent":9007199254740993},
  "created_at":"2026-08-28T01:02:03.123456+08:00","updated_at":"2026-08-27T01:02:03.123456+08:00"
}`)

var segmentFixture = json.RawMessage(`{
  "id":2,"campaign_id":1,"segment_id":3,"segment_code":" 原分群code ","priority":-2,"label":"原分群名称",
  "created_at":"2026-08-28T01:02:03.123456+08:00"
}`)

var memberFixture = json.RawMessage(`{
  "id":4,"campaign_id":1,"campaign_segment_id":2,"segment_id":3,"member_id":9007199254740993,
  "joined_at":"2026-08-28T01:02:03.123456+08:00","anchor_date":"源日期标签","current_step_index":-1,
  "next_due_at":null,"status":"active","stop_reason":" 原因 ","last_step_sent_at":"2026-08-27T01:00:00.123456+08:00",
  "last_error_text":" private-error ","retry_count":-2,"trace_id":"private-member-trace",
  "created_at":"2026-08-28T01:02:03.123456+08:00","updated_at":"2026-08-27T01:02:03.123456+08:00","unionid":" private-union "
}`)

func changed(t *testing.T, raw json.RawMessage, key string, value any) json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal("fixture_invalid")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal("fixture_value_invalid")
	}
	fields[key] = encoded
	result, err := json.Marshal(fields)
	if err != nil {
		t.Fatal("fixture_encode_invalid")
	}
	return result
}

func without(t *testing.T, raw json.RawMessage, key string) json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal("fixture_invalid")
	}
	delete(fields, key)
	result, err := json.Marshal(fields)
	if err != nil {
		t.Fatal("fixture_encode_invalid")
	}
	return result
}

func fixtureHistory(campaigns, segments, members []json.RawMessage) History {
	if campaigns == nil {
		campaigns = []json.RawMessage{campaignFixture}
	}
	if segments == nil {
		segments = []json.RawMessage{segmentFixture}
	}
	if members == nil {
		members = []json.RawMessage{memberFixture}
	}
	return AdaptHistory(campaigns, segments, members)
}

func TestHistoricalFactsPreserveSourceValuesWithoutExecution(t *testing.T) {
	h := fixtureHistory(nil, nil, nil)
	if h.Campaigns[0].Disposition != Candidate || h.Segments[0].Disposition != Candidate || h.Members[0].Disposition != Candidate {
		t.Fatal("candidate_missing")
	}
	c, s, m := h.Campaigns[0].Fact, h.Segments[0].Fact, h.Members[0].Fact
	if c.SourceID != 1 || c.Code != "  repeated-code  " || c.DisplayName != " 历史父 " || c.Intent != "原始意图" || c.AnchorMode != "legacy_mode" || c.AnchorDate != "源日期标签" || c.ReviewStatus != "legacy_review" || c.RunStatus != "active" || c.PausedReason != " 原因 " {
		t.Fatal("campaign_source_changed")
	}
	if c.Source != (CampaignSource{" private-owner ", " agent ", " private-session ", " private-approver ", "private-trace"}) {
		t.Fatal("campaign_actor_sources_changed")
	}
	if c.ApprovedAt == nil || c.ApprovedAt.Format(time.RFC3339Nano) != "2026-08-28T01:00:00.123456+08:00" || c.FinishedAt == nil || c.StartedAt != nil || c.PausedAt != nil || !c.UpdatedAt.Before(c.CreatedAt) {
		t.Fatal("campaign_times_changed")
	}
	if s.SourceID != 2 || s.CampaignSourceID != 1 || s.SegmentSourceID != 3 || s.Code != " 原分群code " || s.Priority != -2 || s.Label != "原分群名称" || !s.CreatedAt.Equal(c.CreatedAt) {
		t.Fatal("segment_source_changed")
	}
	if m.SourceID != 4 || m.CampaignSourceID != 1 || m.CampaignSegmentSourceID != 2 || m.SegmentSourceID != 3 || m.MemberSourceID != 9007199254740993 || m.CurrentStepIndex != -1 || m.RetryCount != -2 || m.Status != "active" || m.AnchorDate != "源日期标签" || m.StopReason != " 原因 " {
		t.Fatal("member_source_changed")
	}
	if m.Source != (MemberSource{" private-union ", "private-member-trace", " private-error "}) || m.NextDueAt != nil || m.LastStepSentAt == nil || !m.UpdatedAt.Before(m.CreatedAt) || !m.JoinedAt.Equal(c.CreatedAt) || !m.CreatedAt.Equal(c.CreatedAt) {
		t.Fatal("member_sources_or_times_changed")
	}
	encoded, err := json.Marshal(h)
	if err != nil {
		t.Fatal("candidate_encode_failed")
	}
	for _, forbidden := range []string{"private-", "never execute", "metadata_json", "stats_json", "approval_token_hash", "customer_id", "unionid"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("private_or_executable_source_leaked")
		}
	}
}

func TestHistoryDoesNotFilterStatesOrInferCurrentIdentity(t *testing.T) {
	for _, status := range []string{"active", "finished", "completed", "cancelled", "paused", "", " unknown "} {
		h := fixtureHistory([]json.RawMessage{changed(t, campaignFixture, "run_status", status)}, nil, []json.RawMessage{changed(t, memberFixture, "status", status)})
		if h.Campaigns[0].Fact == nil || h.Members[0].Fact == nil || h.Campaigns[0].Fact.RunStatus != status || h.Members[0].Fact.Status != status {
			t.Fatal("source_status_filtered_or_rewritten")
		}
	}
	m := changed(t, changed(t, memberFixture, "unionid", ""), "member_id", int64(0))
	if got := fixtureHistory(nil, nil, []json.RawMessage{m}); got.Members[0].Fact == nil || got.Members[0].Fact.MemberSourceID != 0 || got.Members[0].Fact.Source.UnionID != "" {
		t.Fatal("missing_identity_was_guessed_or_filtered")
	}
	for _, field := range strings.Fields(campaignNullable) {
		if got := AdaptCampaign(changed(t, campaignFixture, field, nil)); got.Fact == nil {
			t.Fatal("nullable_campaign_time_rejected")
		}
	}
	for _, field := range strings.Fields(memberNullable) {
		if got := AdaptMember(changed(t, memberFixture, field, nil)); got.Fact == nil {
			t.Fatal("nullable_member_time_rejected")
		}
	}
}

func TestHistoryRelationsUseSourceParentsNotCodesOrCurrentTargets(t *testing.T) {
	for _, test := range []struct {
		kind, field string
		value       any
		reason      string
	}{
		{"segment", "campaign_id", 99, "segment_campaign_unresolved"},
		{"member", "campaign_id", 99, "member_campaign_unresolved"},
		{"member", "campaign_segment_id", 99, "member_campaign_segment_unresolved"},
		{"member", "segment_id", 99, "member_campaign_segment_mismatch"},
	} {
		t.Run(test.reason, func(t *testing.T) {
			var h History
			if test.kind == "segment" {
				h = fixtureHistory(nil, []json.RawMessage{changed(t, segmentFixture, test.field, test.value)}, nil)
			} else {
				h = fixtureHistory(nil, nil, []json.RawMessage{changed(t, memberFixture, test.field, test.value)})
			}
			if test.kind == "segment" {
				if h.Segments[0].Disposition != Pending || h.Segments[0].Fact != nil || h.Segments[0].Reason != test.reason || h.Members[0].Reason != "member_campaign_segment_unresolved" {
					t.Fatal("segment_parent_not_isolated")
				}
			} else if h.Members[0].Disposition != Pending || h.Members[0].Fact != nil || h.Members[0].Reason != test.reason {
				t.Fatal("member_parent_not_isolated")
			}
		})
	}
	parents := []json.RawMessage{campaignFixture, changed(t, campaignFixture, "id", int64(5))}
	h := fixtureHistory(parents, []json.RawMessage{changed(t, segmentFixture, "campaign_id", int64(5))}, nil)
	if h.Campaigns[0].Disposition != Candidate || h.Campaigns[1].Disposition != Candidate || h.Members[0].Reason != "member_campaign_segment_mismatch" {
		t.Fatal("same_code_was_used_as_parent_identity")
	}
	h = fixtureHistory(nil, nil, []json.RawMessage{memberFixture, changed(t, memberFixture, "id", int64(5))})
	if len(h.Members) != 2 || h.Members[0].Fact == nil || h.Members[1].Fact == nil {
		t.Fatal("members_were_deduplicated_by_identity")
	}
}

func TestHistoryDuplicateSourceIDsRemainIsolatedAndConserved(t *testing.T) {
	h := fixtureHistory([]json.RawMessage{campaignFixture, campaignFixture}, nil, nil)
	if len(h.Campaigns) != 2 || h.Campaigns[0].Reason != "duplicate_source_id" || h.Campaigns[1].Fact != nil || h.Segments[0].Reason != "segment_campaign_unresolved" || h.Members[0].Reason != "member_campaign_unresolved" {
		t.Fatal("duplicate_campaign_accepted")
	}
	h = fixtureHistory(nil, []json.RawMessage{segmentFixture, segmentFixture}, nil)
	if len(h.Segments) != 2 || h.Segments[0].Fact != nil || h.Segments[1].Reason != "duplicate_source_id" || h.Members[0].Reason != "member_campaign_segment_unresolved" {
		t.Fatal("duplicate_segment_accepted")
	}
	h = fixtureHistory(nil, nil, []json.RawMessage{memberFixture, memberFixture})
	if len(h.Members) != 2 || h.Members[0].Fact != nil || h.Members[1].Reason != "duplicate_source_id" {
		t.Fatal("duplicate_member_accepted")
	}
	h = fixtureHistory([]json.RawMessage{json.RawMessage(`{"private":"bad source"}`)}, nil, nil)
	if h.Campaigns[0].Reason != "campaign_json_invalid" || h.Members[0].Fact != nil {
		t.Fatal("invalid_parent_accepted")
	}
	empty := AdaptHistory(nil, nil, nil)
	if len(empty.Campaigns)+len(empty.Segments)+len(empty.Members) != 0 {
		t.Fatal("empty_source_fabricated")
	}
}

func TestHistoryJSONAndRequiredFieldShapes(t *testing.T) {
	for _, test := range []struct {
		fixture            json.RawMessage
		required, nullable string
		valid              func(json.RawMessage) bool
	}{
		{campaignFixture, campaignRequired, campaignNullable, func(raw json.RawMessage) bool { return AdaptCampaign(raw).Disposition == Candidate }},
		{segmentFixture, segmentRequired, "", func(raw json.RawMessage) bool { return AdaptSegment(raw).Disposition == Candidate }},
		{memberFixture, memberRequired, memberNullable, func(raw json.RawMessage) bool { return AdaptMember(raw).Disposition == Candidate }},
	} {
		for _, bad := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`[]`), json.RawMessage(`{"id":`), changed(t, test.fixture, "id", 0), changed(t, test.fixture, "id", "1"), changed(t, test.fixture, "id", 1.5), changed(t, test.fixture, "id", json.Number("9223372036854775808")), changed(t, test.fixture, "created_at", "invalid-time")} {
			if test.valid(bad) {
				t.Fatal("invalid_json_shape_accepted")
			}
		}
		for _, field := range strings.Fields(test.required) {
			if test.valid(without(t, test.fixture, field)) || test.valid(changed(t, test.fixture, field, nil)) {
				t.Fatalf("required_field_not_checked field=%s", field)
			}
		}
		for _, field := range strings.Fields(test.nullable) {
			if test.valid(without(t, test.fixture, field)) || test.valid(changed(t, test.fixture, field, "bad-time")) {
				t.Fatal("nullable_field_shape_not_checked")
			}
		}
	}
	if AdaptMember(changed(t, memberFixture, "retry_count", int64(2147483648))).Disposition != Invalid || AdaptSegment(changed(t, segmentFixture, "priority", 0.5)).Disposition != Invalid {
		t.Fatal("integer_width_not_preserved")
	}
}
