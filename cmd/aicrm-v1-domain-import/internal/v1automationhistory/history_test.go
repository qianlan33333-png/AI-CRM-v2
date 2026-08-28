package v1automationhistory

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAdaptHistoryPreservesInertAutomationFacts(t *testing.T) {
	sop, config, prompt, agent := fixtures()
	history := AdaptHistory([]json.RawMessage{raw(t, sop)}, []json.RawMessage{raw(t, config)}, []json.RawMessage{raw(t, prompt)}, []json.RawMessage{raw(t, agent)})

	sopFact := mustCandidate(t, history.SOPTemplates[0]).Fact
	if sopFact.SourceID != 10 || sopFact.PoolKey != "pool-source-1" || sopFact.DayIndex != 2 || sopFact.OriginalEnabled || sopFact.ImagesDigest == (OpaqueDigest{}) {
		t.Fatal("SOP source state or reference was changed")
	}
	if sopFact.CreatedAt.Format(time.RFC3339Nano) != "2026-08-28T09:30:00.123456+08:00" || sopFact.UpdatedAt.Format(time.RFC3339Nano) != "2026-08-28T09:30:00.123456+08:00" {
		t.Fatal("SOP source timestamps were changed")
	}
	if sopFact.ContentMasked != "联系 [masked-phone]\n保留原换行" || strings.Contains(sopFact.ContentMasked, "13800138000") {
		t.Fatal("SOP content did not use the historical phone masking policy")
	}

	configFact := mustCandidate(t, history.AgentConfigs[0]).Fact
	if configFact.SourceID != 20 || configFact.AgentCode != "audience-draft" || configFact.OriginalEnabled || configFact.DraftVersion != 4 || configFact.PublishedVersion != 3 || configFact.ConfigDigest == (OpaqueDigest{}) {
		t.Fatal("agent configuration source state/version was changed or secret summary missing")
	}
	if configFact.PublishedAt != "2026-08-01 09:30:00" || configFact.LastModifiedSource != "legacy" || configFact.SubmittedAt != "" {
		t.Fatal("agent configuration historical text references were not preserved")
	}

	promptFact := mustCandidate(t, history.PromptRegistries[0]).Fact
	if promptFact.SourceID != 30 || promptFact.AgentCode != "audience-draft" || promptFact.OriginalEnabled || promptFact.Version != 7 || promptFact.PromptDigest == (OpaqueDigest{}) {
		t.Fatal("prompt registry source state/version was changed")
	}

	agentFact := mustCandidate(t, history.Agents[0]).Fact
	if agentFact.SourceID != 40 || agentFact.ProgramSourceID != 0 || agentFact.WorkflowSourceID != 0 || agentFact.NodeSourceID != 0 || agentFact.TaskSourceID != 0 || agentFact.OriginalType != "agent" || agentFact.OriginalStatus != "active" || !agentFact.OriginalEnabled || agentFact.ConfigurationDigest == (OpaqueDigest{}) {
		t.Fatal("agent source references or original state were not preserved")
	}
}

func TestAdaptHistoryPreservesEmptyHistoricalTextWithoutActivation(t *testing.T) {
	_, config, _, agent := fixtures()
	config["published_at"] = ""
	config["published_by"] = ""
	config["last_modified_at"] = ""
	config["last_modified_by"] = ""
	config["last_modified_source"] = ""
	config["last_change_summary"] = ""
	config["submitted_at"] = ""
	config["submitted_by"] = ""
	agent["archived_at"] = ""
	history := AdaptHistory(nil, []json.RawMessage{raw(t, config)}, nil, []json.RawMessage{raw(t, agent)})
	if fact := mustCandidate(t, history.AgentConfigs[0]).Fact; fact.PublishedAt != "" || fact.SubmittedBy != "" || fact.ConfigDigest == (OpaqueDigest{}) {
		t.Fatal("empty V1 config text was not preserved as source history")
	}
	if fact := mustCandidate(t, history.Agents[0]).Fact; fact.ArchivedAt != "" || fact.OriginalStatus != "active" {
		t.Fatal("empty V1 archived_at or original status was changed")
	}
}

func TestAdaptHistoryQuarantinesInvalidShapeAndDuplicateSource(t *testing.T) {
	_, config, prompt, _ := fixtures()
	config["enabled"] = "false"
	invalid := AdaptHistory(nil, []json.RawMessage{raw(t, config)}, nil, nil).AgentConfigs[0]
	if invalid.Disposition != DispositionQuarantine || invalid.Reason != "automation_agent_config_shape_invalid" || invalid.SourceID != 20 {
		t.Fatal("invalid source type was not quarantined with its source identifier")
	}

	first := raw(t, prompt)
	second := raw(t, prompt)
	duplicate := AdaptHistory(nil, nil, []json.RawMessage{first, second}, nil).PromptRegistries
	for _, value := range duplicate {
		if value.Disposition != DispositionQuarantine || value.Reason != "automation_agent_prompt_registry_source_ambiguous" || value.SourceID != 30 || value.Fact != nil {
			t.Fatal("ambiguous source rows were not quarantined")
		}
	}

	prompt["prompt_text"] = nil
	null := AdaptHistory(nil, nil, []json.RawMessage{raw(t, prompt)}, nil).PromptRegistries[0]
	if null.Disposition != DispositionQuarantine || null.Reason != "automation_agent_prompt_registry_shape_invalid" {
		t.Fatal("manifest non-null prompt value was accepted as null")
	}
}

func TestCandidateSerializationDoesNotExposePromptsOrConfig(t *testing.T) {
	sop, config, prompt, agent := fixtures()
	history := AdaptHistory([]json.RawMessage{raw(t, sop)}, []json.RawMessage{raw(t, config)}, []json.RawMessage{raw(t, prompt)}, []json.RawMessage{raw(t, agent)})
	encoded, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"draft-role-secret", "published-task-secret", "prompt-registry-secret", "config-secret", "image-secret", "change-secret"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("sealed automation material escaped a candidate")
		}
	}
	if !strings.Contains(string(encoded), "[masked-phone]") || strings.Contains(string(encoded), "13800138000") {
		t.Fatal("SOP display text did not retain its masked-only policy")
	}
}

func fixtures() (map[string]any, map[string]any, map[string]any, map[string]any) {
	stamp := time.Date(2026, 8, 28, 9, 30, 0, 123456000, time.FixedZone("v1-source", 8*60*60))
	sop := map[string]any{
		"id": int64(10), "pool_key": "pool-source-1", "day_index": int32(2), "content": "联系 13800138000\n保留原换行", "images_json": map[string]any{"asset": "image-secret"}, "enabled": false, "created_at": stamp, "updated_at": stamp,
	}
	config := map[string]any{
		"id": int64(20), "agent_code": "audience-draft", "display_name": "人群草稿", "pool_keys_json": []any{"pool-source-1"}, "enabled": false,
		"draft_role_prompt": "draft-role-secret", "draft_task_prompt": "draft-task-secret", "draft_variables_json": map[string]any{"key": "config-secret"}, "draft_output_schema_json": map[string]any{"type": "object"},
		"published_role_prompt": "published-role-secret", "published_task_prompt": "published-task-secret", "published_variables_json": map[string]any{}, "published_output_schema_json": map[string]any{},
		"draft_version": int32(4), "published_version": int32(3), "published_at": "2026-08-01 09:30:00", "published_by": "legacy-user", "last_modified_at": "2026-08-02 09:30:00", "last_modified_by": "legacy-user", "last_modified_source": "legacy", "last_change_summary": "change-secret",
		"created_at": stamp, "updated_at": stamp, "submitted_for_publish": false, "submitted_at": "", "submitted_by": "", "scenario_code": "audience",
	}
	prompt := map[string]any{"id": int64(30), "agent_code": "audience-draft", "display_name": "人群草稿", "prompt_text": "prompt-registry-secret", "enabled": false, "version": int32(7), "created_at": stamp, "updated_at": stamp}
	agent := map[string]any{
		"id": int64(40), "program_id": int64(0), "workflow_id": int64(0), "node_id": int64(0), "task_id": int64(0), "agent_code": "audience-draft", "agent_name": "人群草稿", "agent_type": "agent", "status": "active", "sort_order": int32(0),
		"metadata_json": map[string]any{"secret": "config-secret"}, "config_json": map[string]any{"image": "image-secret"}, "enabled": true, "created_by": "legacy-user", "updated_by": "legacy-user", "created_at": stamp, "updated_at": stamp, "archived_at": "",
	}
	return sop, config, prompt, agent
}

func raw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func mustCandidate[T any](t *testing.T, value Result[T]) Result[T] {
	t.Helper()
	if value.Disposition != DispositionCandidate || value.Fact == nil {
		t.Fatal("expected historical candidate")
	}
	return value
}
