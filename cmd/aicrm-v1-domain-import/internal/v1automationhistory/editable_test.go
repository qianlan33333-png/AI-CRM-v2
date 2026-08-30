package v1automationhistory

import (
	"encoding/json"
	"testing"
)

func TestAdaptEditableAgentsUsesCurrentAgentsAndKeepsThemPausedCandidates(t *testing.T) {
	_, config, prompt, agent := fixtures()
	smoke := cloneFixture(t, config)
	smoke["id"], smoke["agent_code"], smoke["display_name"] = int64(21), "smoke_runtime_only", "smoke"

	items, err := AdaptEditableAgents(
		[]json.RawMessage{raw(t, config), raw(t, smoke)},
		[]json.RawMessage{raw(t, prompt)},
		[]json.RawMessage{raw(t, agent)},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].SourceAgentID != 40 || items[0].SourceConfigID != 20 || items[0].SourcePromptID != 30 || items[0].AgentCode != "audience-draft" {
		t.Fatalf("editable agents = %#v", items)
	}
	if items[0].DraftRole != "draft-role-secret" || items[0].PublishedTask != "published-task-secret" || items[0].DraftVersion != 4 || items[0].PublishedVersion != 3 {
		t.Fatal("editable prompt/version state was not preserved")
	}
	var legacy map[string]any
	if json.Unmarshal(items[0].LegacyConfiguration, &legacy) != nil || legacy["scenario_code"] != "audience" || legacy["last_change_summary"] != "change-secret" {
		t.Fatalf("legacy configuration = %s", items[0].LegacyConfiguration)
	}
}

func TestAdaptEditableAgentsUsesRegistryForAgentWithoutConfiguration(t *testing.T) {
	_, _, prompt, agent := fixtures()
	prompt["agent_code"], prompt["display_name"], prompt["version"] = "central_router_agent", "中枢", int32(2)
	agent["agent_code"], agent["agent_name"] = "central_router_agent", "中枢"
	items, err := AdaptEditableAgents(nil, []json.RawMessage{raw(t, prompt)}, []json.RawMessage{raw(t, agent)})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].SourceConfigID != 0 || items[0].DraftTask != "prompt-registry-secret" || items[0].PublishedTask != "prompt-registry-secret" || items[0].DraftVersion != 2 {
		t.Fatalf("router projection = %#v", items)
	}
}

func TestAdaptEditableAgentsExcludesCurrentTestAgent(t *testing.T) {
	_, config, prompt, agent := fixtures()
	testPrompt := cloneFixture(t, prompt)
	testPrompt["id"], testPrompt["agent_code"], testPrompt["display_name"] = int64(31), "test_agent", "测试 Agent"
	testAgent := cloneFixture(t, agent)
	testAgent["id"], testAgent["agent_code"], testAgent["agent_name"] = int64(41), "test_agent", "测试 Agent"
	items, err := AdaptEditableAgents(
		[]json.RawMessage{raw(t, config)},
		[]json.RawMessage{raw(t, prompt), raw(t, testPrompt)},
		[]json.RawMessage{raw(t, agent), raw(t, testAgent)},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].AgentCode != "audience-draft" {
		t.Fatalf("test Agent was projected: %#v", items)
	}
}

func cloneFixture(t *testing.T, source map[string]any) map[string]any {
	t.Helper()
	rawValue := raw(t, source)
	var result map[string]any
	if err := json.Unmarshal(rawValue, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
