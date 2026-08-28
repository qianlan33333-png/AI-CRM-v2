package v1statictail

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAdaptHistoryConserves54RowsAsReadOnlyFacts(t *testing.T) {
	invites, slices, strategies, versions, documents := validRows(t)
	history := AdaptHistory(invites, slices, strategies, versions, documents)
	if history.SourceCount() != 54 || history.TerminalCount() != 54 {
		t.Fatalf("row conservation source=%d terminal=%d", history.SourceCount(), history.TerminalCount())
	}
	for index, row := range history.GroupInvites {
		if row.Disposition != DispositionCandidate || row.Fact == nil || row.Fact.SealedSourceDigest != invites[index].PayloadHMAC {
			t.Fatalf("group invite=%#v", row)
		}
	}
	if history.GroupInvites[0].Fact.RoomBaseSourceID != nil {
		t.Fatalf("explicit source null room_base_id was not preserved: %#v", history.GroupInvites[0].Fact)
	}
	if fact := history.PageSlices[45].Fact; fact == nil || fact.ProductSourceID != 700 || fact.ImageSourceID != 845 || fact.OriginalEnabled || fact.SortOrder != 45 || fact.SealedSourceDigest != slices[45].PayloadHMAC {
		t.Fatalf("source image/product references or readonly fields lost: %#v", fact)
	}
	if fact := history.Strategies[0].Fact; fact == nil || fact.OriginalStatus != "paused" || fact.CurrentVersion != 2 || fact.SealedSourceDigest != strategies[0].PayloadHMAC {
		t.Fatalf("strategy fact=%#v", fact)
	}
	if fact := history.Versions[1].Fact; fact == nil || fact.EffectiveFrom != nil || fact.ConfirmedAt != nil || fact.OriginalGovernance != "unconfirmed" || fact.SealedSourceDigest != versions[1].PayloadHMAC {
		t.Fatalf("nullable version fields or original status lost: %#v", fact)
	}
	if fact := history.Documents[0].Fact; fact == nil || fact.ExecutionGuideGeneratedAt != nil || fact.CopyGuideGeneratedAt != nil || fact.MeasurementGuideGeneratedAt != nil || fact.SealedSourceDigest != documents[0].PayloadHMAC {
		t.Fatalf("nullable document timestamps lost: %#v", fact)
	}
}

func TestAdaptHistoryQuarantinesIllegalShapesAndAmbiguousSources(t *testing.T) {
	invites, slices, strategies, versions, documents := validRows(t)
	badInvite := sourceRecord(t, map[string]any{"id": "not-an-int"})
	badSlice := mapRow(t, slices[0])
	badSlice["enabled"] = "yes"
	badStrategy := mapRow(t, strategies[0])
	badStrategy["current_version"] = "two"
	badVersion := mapRow(t, versions[0])
	badVersion["definition_json"] = nil
	badDocument := mapRow(t, documents[0])
	badDocument["execution_contract_json"] = nil

	history := AdaptHistory(
		append(invites[:1], badInvite),
		[]SourceRecord{replacePayload(t, slices[0], badSlice)},
		[]SourceRecord{replacePayload(t, strategies[0], badStrategy)},
		[]SourceRecord{replacePayload(t, versions[0], badVersion)},
		[]SourceRecord{replacePayload(t, documents[0], badDocument)},
	)
	if history.GroupInvites[1].Reason != "group_invite_library_shape_invalid" || history.PageSlices[0].Reason != "wechat_pay_product_page_slice_shape_invalid" || history.Strategies[0].Reason != "operation_cycle_strategy_shape_invalid" || history.Versions[0].Reason != "operation_cycle_strategy_version_shape_invalid" || history.Documents[0].Reason != "operation_cycle_strategy_version_document_shape_invalid" {
		t.Fatalf("invalid source shape escaped quarantine: %#v", history)
	}

	duplicate := append(invites[:1], invites[0])
	history = AdaptHistory(duplicate, nil, nil, nil, nil)
	if history.GroupInvites[0].Reason != "group_invite_library_source_ambiguous" || history.GroupInvites[1].Reason != "group_invite_library_source_ambiguous" {
		t.Fatalf("duplicate source accepted: %#v", history.GroupInvites)
	}
}

func TestAdaptHistoryFailsClosedForBrokenOperationCycleParents(t *testing.T) {
	_, _, strategies, versions, documents := validRows(t)
	wrongParent := mapRow(t, versions[1])
	wrongParent["strategy_id"] = 999
	history := AdaptHistory(nil, nil, strategies, []SourceRecord{versions[0], replacePayload(t, versions[1], wrongParent)}, documents)
	if history.Versions[1].Reason != "operation_cycle_strategy_version_strategy_unresolved" {
		t.Fatalf("version with unknown strategy=%#v", history.Versions[1])
	}
	if history.Strategies[0].Reason != "operation_cycle_strategy_current_version_unresolved" || history.Versions[0].Reason != "operation_cycle_strategy_version_strategy_unresolved" {
		t.Fatalf("strategy did not close over invalid current-version chain: %#v %#v", history.Strategies[0], history.Versions[0])
	}
	if history.Documents[0].Reason != "operation_cycle_strategy_version_document_version_unresolved" {
		t.Fatalf("document survived broken version parent: %#v", history.Documents[0])
	}
}

func TestAdaptHistoryDoesNotExposeURLsActorsOrExecutableSourceMaterial(t *testing.T) {
	invites, slices, strategies, versions, documents := validRows(t)
	history := AdaptHistory(invites, slices, strategies, versions, documents)
	encoded, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{
		"https://private.invalid/group", "chat-private", "tenant-private", "actor-private", "confirmation-private",
		"definition-private", "skill-private", "# execution-private", "contract-private", "guide-source-private",
	} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("sealed source value escaped candidate boundary: %q", private)
		}
	}
}

func TestAdaptHistoryRequiresNullableFieldPresence(t *testing.T) {
	invites, _, _, versions, documents := validRows(t)
	missingRoomBase := mapRow(t, invites[0])
	delete(missingRoomBase, "room_base_id")
	missingEffectiveFrom := mapRow(t, versions[0])
	delete(missingEffectiveFrom, "effective_from")
	missingGuideTime := mapRow(t, documents[0])
	delete(missingGuideTime, "copy_guide_generated_at")
	history := AdaptHistory(
		[]SourceRecord{replacePayload(t, invites[0], missingRoomBase)},
		nil,
		nil,
		[]SourceRecord{replacePayload(t, versions[0], missingEffectiveFrom)},
		[]SourceRecord{replacePayload(t, documents[0], missingGuideTime)},
	)
	if history.GroupInvites[0].Reason != "group_invite_library_shape_invalid" || history.Versions[0].Reason != "operation_cycle_strategy_version_shape_invalid" || history.Documents[0].Reason != "operation_cycle_strategy_version_document_shape_invalid" {
		t.Fatalf("missing nullable field became source null: %#v", history)
	}
}

func TestAdaptHistoryRejectsZeroPayloadHMAC(t *testing.T) {
	_, slices, _, _, _ := validRows(t)
	slices[0].PayloadHMAC = OpaqueDigest{}
	history := AdaptHistory(nil, slices[:1], nil, nil, nil)
	if history.PageSlices[0].Reason != "wechat_pay_product_page_slice_shape_invalid" {
		t.Fatalf("zero payload digest accepted: %#v", history.PageSlices[0])
	}
}

func validRows(t *testing.T) ([]SourceRecord, []SourceRecord, []SourceRecord, []SourceRecord, []SourceRecord) {
	t.Helper()
	stamp := time.Date(2026, 8, 28, 9, 10, 11, 123456000, time.UTC)
	invite := map[string]any{
		"id": 1, "name": "历史群", "title": "历史入口", "description": "只读说明", "pic_url": "https://private.invalid/group", "join_url": "https://private.invalid/group", "config_id": "config-private", "state": "archived", "chat_id_list": []string{"chat-private"}, "auto_create_room": true, "room_base_name": "历史房间", "room_base_id": nil, "enabled": false, "created_at": stamp, "updated_at": stamp, "chat_id": "chat-private", "binding_status": "unbound",
	}
	invites := make([]SourceRecord, 4)
	for i := range invites {
		value := copyMap(invite)
		value["id"] = i + 1
		invites[i] = sourceRecord(t, value)
	}
	slices := make([]SourceRecord, 46)
	for i := range slices {
		slices[i] = sourceRecord(t, map[string]any{"id": 800 + i, "product_id": 700, "image_library_id": 800 + i, "sort_order": i, "enabled": i != 45, "created_at": stamp, "updated_at": stamp})
	}
	strategy := map[string]any{"id": 10, "tenant_id": "tenant-private", "strategy_key": "legacy-cycle", "title": "历史周期", "description": "只读", "cadence": "weekly", "timezone": "Asia/Shanghai", "status": "paused", "current_version": 2, "created_at": stamp, "updated_at": stamp}
	version := func(id, number int) SourceRecord {
		return sourceRecord(t, map[string]any{"id": id, "strategy_id": 10, "version": number, "label": "V1", "objective": "保留", "definition_json": map[string]any{"secret": "definition-private"}, "version_hash": "definition-hash", "effective_from": nil, "created_at": stamp, "governance_status": "unconfirmed", "confirmed_by": "actor-private", "confirmed_at": nil, "confirmation_note": "confirmation-private", "operation_skill_json": map[string]any{"secret": "skill-private"}, "operation_skill_hash": "skill-hash"})
	}
	document := map[string]any{"id": 300, "strategy_version_id": 102, "schema_version": "v1", "execution_guide_markdown": "# execution-private", "execution_guide_sha256": "execution-hash", "execution_guide_generated_at": nil, "execution_guide_source": "guide-source-private", "copy_guide_markdown": "# copy-private", "copy_guide_sha256": "copy-hash", "copy_guide_generated_at": nil, "copy_guide_source": "guide-source-private", "measurement_guide_markdown": "# measurement-private", "measurement_guide_sha256": "measurement-hash", "measurement_guide_generated_at": nil, "measurement_guide_source": "guide-source-private", "execution_contract_json": map[string]any{"secret": "contract-private"}, "document_pack_hash": "pack-hash", "created_at": stamp}
	return invites, slices, []SourceRecord{sourceRecord(t, strategy)}, []SourceRecord{version(101, 1), version(102, 2)}, []SourceRecord{sourceRecord(t, document)}
}

func sourceRecord(t *testing.T, value any) SourceRecord {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return SourceRecord{Payload: encoded, PayloadHMAC: OpaqueDigest{0x5a, byte(len(encoded))}}
}

func replacePayload(t *testing.T, record SourceRecord, value any) SourceRecord {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	record.Payload = encoded
	return record
}

func mapRow(t *testing.T, value SourceRecord) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(value.Payload, &result); err != nil {
		t.Fatal(err)
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
