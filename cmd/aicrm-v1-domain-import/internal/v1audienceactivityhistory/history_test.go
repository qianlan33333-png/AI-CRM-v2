package v1audienceactivityhistory

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAdaptHistoryPreservesSourceFactsAndHidesPrivateInputs(t *testing.T) {
	runs, events := activityFixtures()
	history := AdaptHistory(raw(t, runs), raw(t, events))
	if history.PackageRuns[0].Disposition != DispositionCandidate || history.MemberEvents[0].Disposition != DispositionCandidate {
		t.Fatalf("unexpected dispositions: %+v", history)
	}
	run := history.PackageRuns[0].Fact
	if run.SourceID != 10 || run.PackageSourceID != 20 || run.VersionSourceID == nil || *run.VersionSourceID != 30 || run.ReturnedCount != -7 || run.DurationMS != -9 || run.PrivateDigest == (OpaqueDigest{}) {
		t.Fatalf("run source facts changed: %+v", run)
	}
	event := history.MemberEvents[0].Fact
	if event.SourceID != 40 || event.RunSourceID == nil || *event.RunSourceID != 10 || event.MemberSourceID == nil || *event.MemberSourceID != 50 || event.EventType != "entered" || event.PrivateDigest == (OpaqueDigest{}) {
		t.Fatalf("event source facts changed: %+v", event)
	}
	encoded, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"run-private", "identity-private", "union-private", "mobile-private", "owner-private", "payload-private", "idempotency-private"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("private source input leaked: %q", private)
		}
	}
}

func TestAdaptHistoryQuarantinesMissingPrivateFieldsAmbiguousIDsAndUnknownRun(t *testing.T) {
	runs, events := activityFixtures()
	delete(events[0], "payload_json")
	history := AdaptHistory(raw(t, runs), raw(t, events))
	if history.MemberEvents[0].Reason != "audience_activity_event_shape_invalid" {
		t.Fatalf("missing private field accepted: %+v", history.MemberEvents[0])
	}

	runs, events = activityFixtures()
	runs = append(runs, copyMap(runs[0]))
	history = AdaptHistory(raw(t, runs), raw(t, events))
	if history.PackageRuns[0].Reason != "audience_activity_source_id_ambiguous" || history.PackageRuns[1].Reason != "audience_activity_source_id_ambiguous" || history.MemberEvents[0].Reason != "audience_activity_event_run_unresolved" {
		t.Fatalf("ambiguous run was not propagated: %+v", history)
	}

	runs, events = activityFixtures()
	events[0]["run_id"] = int64(999)
	history = AdaptHistory(raw(t, runs), raw(t, events))
	if history.MemberEvents[0].Reason != "audience_activity_event_run_unresolved" {
		t.Fatalf("unknown run accepted: %+v", history.MemberEvents[0])
	}
}

func TestPrivateDigestChangesWithoutChangingPublicFact(t *testing.T) {
	runs, events := activityFixtures()
	first := AdaptHistory(raw(t, runs), raw(t, events))
	changed := copyMap(events[0])
	changed["payload_json"] = map[string]any{"private": "changed"}
	second := AdaptHistory(raw(t, runs), raw(t, []map[string]any{changed}))
	if first.MemberEvents[0].Fact.PrivateDigest == second.MemberEvents[0].Fact.PrivateDigest {
		t.Fatal("private source change did not change digest")
	}
	if first.MemberEvents[0].Fact.EventType != second.MemberEvents[0].Fact.EventType || !first.MemberEvents[0].Fact.OccurredAt.Equal(second.MemberEvents[0].Fact.OccurredAt) {
		t.Fatal("private change altered public fact")
	}
}

func activityFixtures() ([]map[string]any, []map[string]any) {
	stamp := time.Date(2026, 8, 29, 8, 30, 0, 123456000, time.FixedZone("source", 8*60*60))
	runs := []map[string]any{{
		"id": int64(10), "package_id": int64(20), "version_id": int64(30), "run_type": "incremental", "status": "succeeded", "refresh_started_at": stamp, "refresh_finished_at": nil, "last_watermark_at": nil, "next_watermark_at": stamp, "returned_count": int32(-7), "entered_count": int32(2), "updated_count": int32(3), "exited_count": int32(4), "member_event_count": int32(5), "duration_ms": int32(-9), "error_message": "run-private", "created_at": stamp,
	}}
	events := []map[string]any{{
		"id": int64(40), "package_id": int64(20), "run_id": int64(10), "member_current_id": int64(50), "event_type": "entered", "identity_type": "unionid", "identity_value": "identity-private", "unionid": "union-private", "mobile_hash": "mobile-private", "owner_userid": "owner-private", "event_source_key": "event-private", "payload_hash": "hash-private", "payload_json": map[string]any{"private": "payload-private"}, "internal_event_id": "internal-private", "idempotency_key": "idempotency-private", "occurred_at": stamp, "created_at": stamp,
	}}
	return runs, events
}

func raw(t *testing.T, values []map[string]any) []json.RawMessage {
	t.Helper()
	result := make([]json.RawMessage, len(values))
	for index, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		result[index] = encoded
	}
	return result
}

func copyMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}
