package v1audiencehistory

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAdaptHistoryProducesTypedStaticCandidates(t *testing.T) {
	groups, packages, versions, senders, rules, ruleVersions, segments := audienceFixtures()
	history := AdaptHistory(raw(t, groups), raw(t, packages), raw(t, versions), raw(t, senders), raw(t, rules), raw(t, ruleVersions), raw(t, segments))
	if history.PackageGroups[0].Disposition != DispositionCandidate || history.Packages[0].Disposition != DispositionCandidate || history.PackageVersions[0].Disposition != DispositionCandidate || history.PackageSenders[0].Disposition != DispositionCandidate || history.Rules[0].Disposition != DispositionCandidate || history.RuleVersions[0].Disposition != DispositionCandidate || history.Segments[0].Disposition != DispositionCandidate {
		t.Fatalf("expected all seven static tables to produce history candidates: %+v", history)
	}
	packageFact := history.Packages[0].Fact
	if packageFact.SourceID != 20 || packageFact.GroupSourceID == nil || *packageFact.GroupSourceID != 10 || packageFact.CurrentVersionSourceID == nil || *packageFact.CurrentVersionSourceID != 30 || packageFact.NaturalLanguageDefinition != "过去七天有使用行为" {
		t.Fatalf("package source fields lost: %+v", packageFact)
	}
	versionFact := history.PackageVersions[0].Fact
	if versionFact.SourceID != 30 || versionFact.PackageSourceID != 20 || versionFact.VersionNumber != 7 || versionFact.AIPrompt != "用自然语言解释人群" || versionFact.DefinitionDigest == (OpaqueDigest{}) {
		t.Fatalf("version source fields lost: %+v", versionFact)
	}
	if history.PackageSenders[0].Fact.Priority != 4 {
		t.Fatal("sender source priority lost")
	}
	if fact := history.Rules[0].Fact; fact.SourceID != 50 || fact.RuleKey != "v1_rule" || fact.OriginalStatus != "published" {
		t.Fatalf("rule source fields lost: %+v", fact)
	}
	if fact := history.Segments[0].Fact; fact.SourceID != 70 || fact.SegmentCode != "legacy_70" || fact.Version != 9 || fact.DefinitionDigest == (OpaqueDigest{}) {
		t.Fatalf("segment source fields lost: %+v", fact)
	}
}

func TestAdaptHistoryWithMembersPreservesPrivateSnapshotWithoutOneID(t *testing.T) {
	groups, packages, versions, senders, rules, ruleVersions, segments := audienceFixtures()
	members := audienceMemberFixtures()
	history := AdaptHistoryWithMembers(raw(t, groups), raw(t, packages), raw(t, versions), raw(t, senders), raw(t, rules), raw(t, ruleVersions), raw(t, segments), raw(t, members))
	member := history.AudienceMembers[0]
	if member.Disposition != DispositionCandidate || member.Fact == nil || member.Fact.SourceID != 80 || member.Fact.PackageSourceID != 20 || member.Fact.OriginalStatus != "active" || member.Fact.UnionID != "unionid-private" || member.Fact.PayloadDigest == (OpaqueDigest{}) {
		t.Fatalf("member snapshot fields lost: %+v", member)
	}
	encoded, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"unionid-private", "identity-private", "owner-private", "payload-private"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("private V1 member input leaked from candidate JSON: %q", private)
		}
	}
}

func TestAdaptHistoryWithMembersQuarantinesUnresolvedOrAmbiguousParents(t *testing.T) {
	groups, packages, versions, senders, rules, ruleVersions, segments := audienceFixtures()
	members := audienceMemberFixtures()
	members[0]["package_id"] = int64(999)
	history := AdaptHistoryWithMembers(raw(t, groups), raw(t, packages), raw(t, versions), raw(t, senders), raw(t, rules), raw(t, ruleVersions), raw(t, segments), raw(t, members))
	if history.AudienceMembers[0].Disposition != DispositionQuarantine || history.AudienceMembers[0].Reason != "audience_member_package_unresolved" {
		t.Fatalf("unresolved member package was not quarantined: %+v", history.AudienceMembers)
	}
	members = audienceMemberFixtures()
	members = append(members, copyMap(members[0]))
	history = AdaptHistoryWithMembers(raw(t, groups), raw(t, packages), raw(t, versions), raw(t, senders), raw(t, rules), raw(t, ruleVersions), raw(t, segments), raw(t, members))
	for _, member := range history.AudienceMembers {
		if member.Disposition != DispositionQuarantine || member.Reason != "audience_source_id_ambiguous" {
			t.Fatalf("ambiguous member source ID was not quarantined: %+v", history.AudienceMembers)
		}
	}
	packages[0]["group_id"] = int64(999)
	history = AdaptHistoryWithMembers(raw(t, groups), raw(t, packages), raw(t, versions), raw(t, senders), raw(t, rules), raw(t, ruleVersions), raw(t, segments), raw(t, audienceMemberFixtures()))
	if history.Packages[0].Reason != "audience_package_group_unresolved" || history.AudienceMembers[0].Reason != "audience_member_package_unresolved" {
		t.Fatalf("static parent quarantine did not flow to membership snapshot: packages=%+v members=%+v", history.Packages, history.AudienceMembers)
	}
}

func TestAdaptHistoryWithMembersPreservesOriginalTimesAndRejectsMalformedIdentity(t *testing.T) {
	groups, packages, versions, senders, rules, ruleVersions, segments := audienceFixtures()
	members := audienceMemberFixtures()
	zoned := time.Date(2026, 8, 28, 9, 30, 0, 123456000, time.FixedZone("source", 8*60*60))
	members[0]["first_entered_at"] = zoned
	members[0]["exited_at"] = nil
	history := AdaptHistoryWithMembers(raw(t, groups), raw(t, packages), raw(t, versions), raw(t, senders), raw(t, rules), raw(t, ruleVersions), raw(t, segments), raw(t, members))
	if fact := history.AudienceMembers[0].Fact; fact == nil || fact.ExitedAt != nil || fact.FirstEnteredAt.Format(time.RFC3339Nano) != zoned.Format(time.RFC3339Nano) {
		t.Fatalf("member timestamps changed: %+v", fact)
	}
	delete(members[0], "unionid")
	history = AdaptHistoryWithMembers(raw(t, groups), raw(t, packages), raw(t, versions), raw(t, senders), raw(t, rules), raw(t, ruleVersions), raw(t, segments), raw(t, members))
	if history.AudienceMembers[0].Reason != "audience_member_shape_invalid" {
		t.Fatalf("malformed private member identity was accepted: %+v", history.AudienceMembers[0])
	}
}

func TestAdaptHistoryPreservesNullableNegativeAndOriginalTimes(t *testing.T) {
	groups, packages, versions, senders, rules, ruleVersions, segments := audienceFixtures()
	packages[0]["group_id"] = nil
	packages[0]["current_version_id"] = nil
	packages[0]["incremental_interval_seconds"] = int64(-7)
	packages[0]["lookback_seconds"] = int64(-8)
	packages[0]["lease_expires_at"] = nil
	senders[0]["priority"] = int64(-2)
	versions[0]["version_number"] = int64(-3)
	versions[0]["template_version"] = int64(-4)
	segments[0]["cached_headcount"] = int64(-5)
	segments[0]["usage_count"] = int64(-6)
	zoned := time.Date(2026, 8, 28, 9, 30, 0, 123456000, time.FixedZone("source", 8*60*60))
	segments[0]["created_at"] = zoned
	segments[0]["updated_at"] = zoned
	history := AdaptHistory(raw(t, groups), raw(t, packages), raw(t, versions), raw(t, senders), raw(t, rules), raw(t, ruleVersions), raw(t, segments))
	packageFact := history.Packages[0].Fact
	if packageFact == nil || packageFact.GroupSourceID != nil || packageFact.CurrentVersionSourceID != nil || packageFact.IncrementalIntervalSecs != -7 || packageFact.LookbackSecs != -8 {
		t.Fatalf("nullable or negative package facts changed: %+v", packageFact)
	}
	if fact := history.PackageSenders[0].Fact; fact == nil || fact.Priority != -2 {
		t.Fatalf("negative sender priority changed: %+v", fact)
	}
	if fact := history.PackageVersions[0].Fact; fact == nil || fact.VersionNumber != -3 || fact.TemplateVersion == nil || *fact.TemplateVersion != -4 {
		t.Fatalf("negative version fields changed: %+v", fact)
	}
	if fact := history.Segments[0].Fact; fact == nil || fact.CachedHeadcount != -5 || fact.UsageCount != -6 || fact.CreatedAt.Format(time.RFC3339Nano) != zoned.Format(time.RFC3339Nano) {
		t.Fatalf("negative or zoned segment facts changed: %+v", fact)
	}
}

func TestAdaptHistoryQuarantinesAmbiguousAndUnresolvedParents(t *testing.T) {
	groups, packages, versions, senders, rules, ruleVersions, segments := audienceFixtures()
	packages = append(packages, copyMap(packages[0]))
	senders[0]["package_id"] = int64(999)
	ruleVersions[0]["rule_id"] = int64(999)
	history := AdaptHistory(raw(t, groups), raw(t, packages), raw(t, versions), raw(t, senders), raw(t, rules), raw(t, ruleVersions), raw(t, segments))
	if history.Packages[0].Reason != "audience_source_id_ambiguous" || history.Packages[1].Reason != "audience_source_id_ambiguous" {
		t.Fatalf("ambiguous source package was not quarantined: %+v", history.Packages)
	}
	if history.PackageVersions[0].Reason != "audience_package_version_package_unresolved" || history.PackageSenders[0].Reason != "audience_package_sender_package_unresolved" || history.RuleVersions[0].Reason != "audience_rule_version_rule_unresolved" {
		t.Fatalf("unresolved historical parents were not quarantined: versions=%+v senders=%+v rules=%+v", history.PackageVersions, history.PackageSenders, history.RuleVersions)
	}
}

func TestAdaptHistoryDoesNotSerializeExecutableOrSensitiveSourceMaterial(t *testing.T) {
	groups, packages, versions, senders, rules, ruleVersions, segments := audienceFixtures()
	packages[0]["lease_token"] = "lease-private"
	versions[0]["incremental_sql_text"] = "SELECT version_private"
	versions[0]["dependencies_json"] = map[string]any{"private": "dependency-private"}
	versions[0]["sample_rows_json"] = []any{map[string]any{"private": "sample-private"}}
	ruleVersions[0]["code_or_sql"] = "SELECT rule_private"
	ruleVersions[0]["params_schema"] = map[string]any{"private": "param-private"}
	segments[0]["sql_query"] = "SELECT segment_private"
	segments[0]["sql_params_json"] = map[string]any{"private": "segment-param-private"}
	segments[0]["cached_sample_json"] = []any{map[string]any{"private": "segment-sample-private"}}
	segments[0]["tags_json"] = []any{"segment-tag-private"}
	history := AdaptHistory(raw(t, groups), raw(t, packages), raw(t, versions), raw(t, senders), raw(t, rules), raw(t, ruleVersions), raw(t, segments))
	encoded, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"lease-private", "version_private", "dependency-private", "sample-private", "rule_private", "param-private", "segment_private", "segment-param-private", "segment-sample-private", "segment-tag-private", "sender-private", "owner-private"} {
		if strings.Contains(string(encoded), value) {
			t.Fatalf("source-only material leaked from candidate JSON: %q", value)
		}
	}
	if history.Packages[0].Fact.RuntimeDigest == (OpaqueDigest{}) || history.PackageVersions[0].Fact.DefinitionDigest == (OpaqueDigest{}) || history.RuleVersions[0].Fact.DefinitionDigest == (OpaqueDigest{}) || history.Segments[0].Fact.DefinitionDigest == (OpaqueDigest{}) {
		t.Fatal("opaque source fields did not leave a digest")
	}
	changed := copyMap(segments[0])
	changed["sql_query"] = "SELECT a_different_private_query"
	other := AdaptHistory(raw(t, groups), raw(t, packages), raw(t, versions), raw(t, senders), raw(t, rules), raw(t, ruleVersions), raw(t, []map[string]any{changed}))
	if other.Segments[0].Fact.DefinitionDigest == history.Segments[0].Fact.DefinitionDigest {
		t.Fatal("opaque source change did not change digest")
	}
}

func TestAdaptHistoryQuarantinesMissingOrMalformedShapeWithoutPayloadReason(t *testing.T) {
	groups, packages, versions, senders, rules, ruleVersions, segments := audienceFixtures()
	delete(versions[0], "template_key")
	segments[0]["sql_params_json"] = "not-jsonb-but-json-string-is-valid"
	history := AdaptHistory(raw(t, groups), raw(t, packages), raw(t, versions), raw(t, senders), raw(t, rules), raw(t, ruleVersions), raw(t, segments))
	if history.PackageVersions[0].Disposition != DispositionQuarantine || history.PackageVersions[0].Reason != "audience_package_version_shape_invalid" {
		t.Fatalf("missing source field was accepted: %+v", history.PackageVersions[0])
	}
	if history.Segments[0].Disposition != DispositionCandidate {
		t.Fatalf("opaque JSON value was treated as executable validation: %+v", history.Segments[0])
	}
	malformed := AdaptHistory(nil, []json.RawMessage{json.RawMessage(`{"id":`)}, nil, nil, nil, nil, nil)
	if malformed.Packages[0].Reason != "audience_package_shape_invalid" {
		t.Fatalf("malformed input leaked a payload-derived reason: %+v", malformed.Packages[0])
	}
}

func audienceFixtures() ([]map[string]any, []map[string]any, []map[string]any, []map[string]any, []map[string]any, []map[string]any, []map[string]any) {
	stamp := time.Date(2026, 8, 28, 9, 30, 0, 0, time.UTC)
	groups := []map[string]any{{"id": int64(10), "name": "历史人群", "created_at": stamp, "updated_at": stamp}}
	packages := []map[string]any{{
		"id": int64(20), "package_key": "legacy_package_20", "name": "历史包", "natural_language_definition": "过去七天有使用行为", "status": "active", "query_mode": "incremental", "identity_policy": "unionid", "current_version_id": int64(30), "incremental_enabled": true, "daily_enabled": false, "incremental_interval_seconds": int64(180), "daily_refresh_time": "08:00", "timezone": "Asia/Shanghai", "lookback_seconds": int64(86400), "last_incremental_watermark_at": nil, "last_daily_refreshed_at": nil, "next_incremental_refresh_at": nil, "next_daily_refresh_at": nil, "lease_token": "lease-default", "lease_expires_at": nil, "paused_reason": "", "created_at": stamp, "updated_at": stamp, "group_id": int64(10),
	}}
	versions := []map[string]any{{
		"id": int64(30), "package_id": int64(20), "version_number": int64(7), "status": "published", "incremental_sql_text": "SELECT default", "snapshot_sql_text": "SELECT snapshot", "ai_prompt": "用自然语言解释人群", "ai_rationale": "历史理由", "natural_language_explanation": "过去七天的使用快照", "dependencies_json": map[string]any{"view": "private"}, "explain_json": map[string]any{"plan": "private"}, "sample_rows_json": []any{"private"}, "validation_errors_json": []any{}, "created_at": stamp, "published_at": stamp, "parameters_json": map[string]any{"limit": 10}, "simple_sql_text": "SELECT simple", "simple_compiled_sql_text": "SELECT compiled", "template_key": "legacy_template", "template_version": int64(3), "template_params_json": map[string]any{"private": true}, "template_fingerprint": "fp-20",
	}}
	senders := []map[string]any{{"id": int64(40), "package_id": int64(20), "sender_userid": "sender-private", "display_name": "历史发送人", "priority": int64(4), "status": "enabled", "created_at": stamp, "updated_at": stamp}}
	rules := []map[string]any{{"id": int64(50), "rule_key": "v1_rule", "display_name": "历史规则", "description": "历史说明", "rule_type": "sql", "owner": "owner-private", "status": "published", "created_at": stamp, "updated_at": stamp}}
	ruleVersions := []map[string]any{{"id": int64(60), "rule_id": int64(50), "version": int64(2), "executor_type": "sql", "code_or_sql": "SELECT rule", "params_schema": map[string]any{}, "output_schema": map[string]any{}, "refresh_policy": map[string]any{}, "status": "published", "published_at": stamp, "created_at": stamp}}
	segments := []map[string]any{{"id": int64(70), "segment_code": "legacy_70", "display_name": "历史细分", "description": "历史细分说明", "source_type": "sql", "sql_query": "SELECT segment", "sql_params_json": map[string]any{}, "sql_dialect": "postgres", "status": "active", "version": int64(9), "created_by_agent": "agent-private", "created_by_session": "session-private", "cached_headcount": int64(8), "cached_sample_json": []any{}, "last_refreshed_at": stamp, "last_refresh_error": "", "usage_count": int64(2), "tags_json": []any{}, "created_at": stamp, "updated_at": stamp}}
	return groups, packages, versions, senders, rules, ruleVersions, segments
}

func audienceMemberFixtures() []map[string]any {
	stamp := time.Date(2026, 8, 28, 9, 30, 0, 0, time.UTC)
	return []map[string]any{{
		"id": int64(80), "package_id": int64(20), "identity_type": "unionid", "identity_value": "identity-private", "status": "active", "mobile_hash": "mobile-private", "owner_userid": "owner-private", "event_source_key": "event-private", "payload_hash": "hash-private", "payload_json": map[string]any{"private": "payload-private"}, "first_entered_at": stamp, "last_seen_at": stamp, "last_updated_at": stamp, "exited_at": nil, "created_at": stamp, "updated_at": stamp, "unionid": "unionid-private",
	}}
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
