package v1groupops

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAdaptHistoryKeepsGroupOpsAsSeparateArchivedFacts(t *testing.T) {
	groupChat, snapshot, plan, planGroup, planNode := groupOpsFixtures()
	history := AdaptHistory([]json.RawMessage{raw(t, groupChat)}, []json.RawMessage{raw(t, snapshot)}, []json.RawMessage{raw(t, plan)}, []json.RawMessage{raw(t, planGroup)}, []json.RawMessage{raw(t, planNode)})
	if history.GroupChats[0].Disposition != DispositionCandidate || history.Snapshots[0].Disposition != DispositionCandidate || history.Plans[0].Disposition != DispositionCandidate || history.PlanGroups[0].Disposition != DispositionCandidate || history.PlanNodes[0].Disposition != DispositionCandidate {
		t.Fatal("expected valid Group Ops historical candidates")
	}
	if len(history.GroupChats) != 1 || len(history.Snapshots) != 1 || history.GroupChats[0].Fact.ChatID != history.Snapshots[0].Fact.ChatID {
		t.Fatal("directory sources were not kept separate")
	}
	if plan := history.Plans[0].Fact; !plan.Archived || plan.SourceID != 30 || plan.OriginalStatus != "active" || plan.SourceOwnerUserID != "owner-private" {
		t.Fatal("plan was not archived historical only")
	}
	node := history.PlanNodes[0].Fact
	if node.DayIndex != 2 || node.TriggerTime != "09:30" || node.SortOrder != 1 || string(node.ContentPackage) != `{"kind":"legacy_package","reference":"opaque"}` {
		t.Fatal("node historical schedule or package was not preserved")
	}
}

func TestAdaptHistoryPreservesNullableGroupChatFacts(t *testing.T) {
	groupChat, _, _, _, _ := groupOpsFixtures()
	groupChat["group_name"] = nil
	groupChat["owner_userid"] = nil
	nullable := AdaptHistory([]json.RawMessage{raw(t, groupChat)}, nil, nil, nil, nil).GroupChats[0].Fact
	if nullable == nil || nullable.GroupName != nil || nullable.OwnerUserID != nil {
		t.Fatal("nullable group chat source facts were changed")
	}

	empty := copyMap(groupChat)
	empty["group_name"] = ""
	empty["owner_userid"] = ""
	preserved := AdaptHistory([]json.RawMessage{raw(t, empty)}, nil, nil, nil, nil).GroupChats[0].Fact
	if preserved == nil || preserved.GroupName == nil || preserved.OwnerUserID == nil || *preserved.GroupName != "" || *preserved.OwnerUserID != "" {
		t.Fatal("empty group chat source facts were conflated with null")
	}
}

func TestAdaptHistoryPreservesZonedSourceTimes(t *testing.T) {
	_, _, plan, _, _ := groupOpsFixtures()
	zoned := time.Date(2026, 8, 28, 9, 30, 0, 123456000, time.FixedZone("v1-source", 8*60*60))
	plan["created_at"] = zoned
	plan["updated_at"] = zoned
	fact := AdaptHistory(nil, nil, []json.RawMessage{raw(t, plan)}, nil, nil).Plans[0].Fact
	if fact == nil || fact.CreatedAt.Format(time.RFC3339Nano) != zoned.Format(time.RFC3339Nano) || fact.UpdatedAt.Format(time.RFC3339Nano) != zoned.Format(time.RFC3339Nano) {
		t.Fatal("zoned source times were changed")
	}
}

func TestAdaptHistoryQuarantinesUnknownParentAndAmbiguousOrUnrepresentableSource(t *testing.T) {
	_, _, plan, planGroup, planNode := groupOpsFixtures()
	noParent := AdaptHistory(nil, nil, nil, []json.RawMessage{raw(t, planGroup)}, []json.RawMessage{raw(t, planNode)})
	if noParent.PlanGroups[0].Reason != "group_ops_plan_group_plan_unresolved" || noParent.PlanNodes[0].Reason != "group_ops_plan_node_plan_unresolved" {
		t.Fatal("unresolved plan parent did not quarantine children")
	}

	duplicate := AdaptHistory(nil, nil, []json.RawMessage{raw(t, plan), raw(t, plan)}, []json.RawMessage{raw(t, planGroup)}, []json.RawMessage{raw(t, planNode)})
	if duplicate.Plans[0].Reason != "group_ops_plan_source_ambiguous" || duplicate.Plans[1].Reason != "group_ops_plan_source_ambiguous" || duplicate.PlanGroups[0].Reason != "group_ops_plan_group_plan_unresolved" || duplicate.PlanNodes[0].Reason != "group_ops_plan_node_plan_unresolved" {
		t.Fatal("ambiguous plan parent did not quarantine dependent rows")
	}

	brokenNode := copyMap(planNode)
	brokenNode["content_package_json"] = []any{"untyped"}
	broken := AdaptHistory(nil, nil, []json.RawMessage{raw(t, plan)}, nil, []json.RawMessage{raw(t, brokenNode)})
	if broken.PlanNodes[0].Disposition != DispositionQuarantine || broken.PlanNodes[0].Reason != "group_ops_plan_node_shape_invalid" {
		t.Fatal("unrepresentable node was not quarantined")
	}
	missingPlan := copyMap(plan)
	delete(missingPlan, "plan_name")
	missing := AdaptHistory(nil, nil, []json.RawMessage{raw(t, missingPlan)}, nil, nil)
	if missing.Plans[0].Disposition != DispositionQuarantine || missing.Plans[0].Reason != "group_ops_plan_shape_invalid" {
		t.Fatal("missing required plan field was not quarantined")
	}
}

func TestHistoryDoesNotSerializeSensitiveDirectoryOrPlanPayloads(t *testing.T) {
	groupChat, snapshot, plan, planGroup, planNode := groupOpsFixtures()
	history := AdaptHistory([]json.RawMessage{raw(t, groupChat)}, []json.RawMessage{raw(t, snapshot)}, []json.RawMessage{raw(t, plan)}, []json.RawMessage{raw(t, planGroup)}, []json.RawMessage{raw(t, planNode)})
	encoded, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"chat-private", "群名称", "owner-private", "owner-name", "webhook-private", "signature-private", "正文私密", "https://private.example", "content-private", "admin-private"} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("private V1 source value escaped historical candidate")
		}
	}
}

func groupOpsFixtures() (map[string]any, map[string]any, map[string]any, map[string]any, map[string]any) {
	stamp := time.Date(2026, 8, 28, 9, 30, 0, 0, time.UTC)
	groupChat := map[string]any{"id": int64(10), "chat_id": "chat-private", "group_name": "群名称", "owner_userid": "owner-private", "notice": "公告私密", "member_count": int32(9), "status": "active", "create_time": "2026-08-01", "dismissed_at": nil, "raw_payload": `{"secret":"raw-private"}`, "updated_at": stamp}
	snapshot := map[string]any{"chat_id": "chat-private", "group_name": "群名称", "owner_userid": "owner-private", "owner_name": "owner-name", "internal_member_count": int32(3), "external_member_count": int32(6), "synced_at": stamp, "status": "normal", "admin_userids": "admin-private"}
	plan := map[string]any{"id": int64(30), "plan_code": "plan-private", "plan_name": "计划私密", "plan_type": "sop", "owner_userid": "owner-private", "status": "active", "webhook_key": "webhook-private", "created_by": "creator-private", "updated_by": "editor-private", "created_at": stamp, "updated_at": stamp, "archived_at": nil, "default_action_type": "message", "allow_no_sop": false, "allow_external_recipients": true, "description": "说明私密", "signature_secret_hash": "signature-private", "last_rotated_at": nil}
	planGroup := map[string]any{"id": int64(40), "plan_id": int64(30), "chat_id": "chat-private", "group_name_snapshot": "群名称", "owner_userid_snapshot": "owner-private", "internal_member_count_snapshot": int32(3), "external_member_count_snapshot": int32(6), "status": "active", "created_at": stamp, "removed_at": nil}
	planNode := map[string]any{"id": int64(50), "plan_id": int64(30), "day_index": int32(2), "trigger_time_label": "09:30", "action_title": "标题私密", "text_content": "正文私密", "attachments_json": map[string]any{"url": "https://private.example"}, "sort_order": int32(1), "status": "active", "created_at": stamp, "updated_at": stamp, "content_package_json": map[string]any{"kind": "legacy_package", "reference": "opaque"}}
	return groupChat, snapshot, plan, planGroup, planNode
}

func raw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func copyMap(value map[string]any) map[string]any {
	copy := make(map[string]any, len(value))
	for key, item := range value {
		copy[key] = item
	}
	return copy
}
