package v1automationhistory

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrEditableAgentInvalid = errors.New("V1 editable automation agent invalid")

// EditableAgent is a paused V2 configuration candidate. It contains no
// execution authority and cannot enqueue work or call an external Provider.
type EditableAgent struct {
	SourceAgentID       int64
	SourceConfigID      int64
	SourcePromptID      int64
	AgentCode           string
	AgentName           string
	DraftRole           string
	DraftTask           string
	PublishedRole       string
	PublishedTask       string
	DraftVersion        int64
	PublishedVersion    int64
	LegacyConfiguration json.RawMessage
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// AdaptEditableAgents projects only current rows from automation_agents. V1
// smoke-only configuration rows have no current agent row and are ignored.
func AdaptEditableAgents(agentConfigs, promptRegistries, agents []json.RawMessage) ([]EditableAgent, error) {
	history := AdaptHistory(nil, agentConfigs, promptRegistries, agents)
	configs := make(map[string]agentConfigJSON, len(agentConfigs))
	configIDs := make(map[string]int64, len(agentConfigs))
	for index, result := range history.AgentConfigs {
		if result.Disposition != DispositionCandidate || result.Fact == nil {
			return nil, ErrEditableAgentInvalid
		}
		var source agentConfigJSON
		if json.Unmarshal(agentConfigs[index], &source) != nil || !validEditableCode(source.AgentCode) {
			return nil, ErrEditableAgentInvalid
		}
		if _, duplicate := configs[source.AgentCode]; duplicate {
			return nil, ErrEditableAgentInvalid
		}
		configs[source.AgentCode], configIDs[source.AgentCode] = source, source.ID
	}
	prompts := make(map[string]promptRegistryJSON, len(promptRegistries))
	for index, result := range history.PromptRegistries {
		if result.Disposition != DispositionCandidate || result.Fact == nil {
			return nil, ErrEditableAgentInvalid
		}
		var source promptRegistryJSON
		if json.Unmarshal(promptRegistries[index], &source) != nil || !validEditableCode(source.AgentCode) {
			return nil, ErrEditableAgentInvalid
		}
		if _, duplicate := prompts[source.AgentCode]; duplicate {
			return nil, ErrEditableAgentInvalid
		}
		prompts[source.AgentCode] = source
	}
	result := make([]EditableAgent, 0, len(agents))
	seen := make(map[string]struct{}, len(agents))
	for index, decision := range history.Agents {
		if decision.Disposition != DispositionCandidate || decision.Fact == nil {
			return nil, ErrEditableAgentInvalid
		}
		var source agentJSON
		if json.Unmarshal(agents[index], &source) != nil || !source.Enabled || source.ArchivedAt != "" || !validEditableCode(source.AgentCode) || !validEditableText(source.AgentName, 120) || testEditableAgent(source.AgentCode, source.AgentName) {
			continue
		}
		if _, duplicate := seen[source.AgentCode]; duplicate {
			return nil, ErrEditableAgentInvalid
		}
		seen[source.AgentCode] = struct{}{}
		prompt, promptFound := prompts[source.AgentCode]
		config, configFound := configs[source.AgentCode]
		if !promptFound {
			return nil, ErrEditableAgentInvalid
		}
		item, err := editableAgent(source, config, configFound, prompt)
		if err != nil {
			return nil, err
		}
		item.SourceConfigID = configIDs[source.AgentCode]
		result = append(result, item)
	}
	if len(result) == 0 {
		return nil, ErrEditableAgentInvalid
	}
	return result, nil
}

func testEditableAgent(code, name string) bool {
	code = strings.ToLower(code)
	name = strings.ToLower(name)
	return strings.Contains(code, "smoke") || strings.Contains(code, "realtest") || strings.HasPrefix(code, "test_") || strings.HasPrefix(code, "test-") ||
		strings.HasSuffix(code, "_test") || strings.HasSuffix(code, "-test") || strings.Contains(name, "smoke") || strings.Contains(name, "realtest") || strings.Contains(name, "测试")
}

func editableAgent(source agentJSON, config agentConfigJSON, configFound bool, prompt promptRegistryJSON) (EditableAgent, error) {
	item := EditableAgent{SourceAgentID: source.ID, SourcePromptID: prompt.ID, AgentCode: source.AgentCode, AgentName: source.AgentName, CreatedAt: source.CreatedAt.UTC(), UpdatedAt: source.UpdatedAt.UTC()}
	legacy := map[string]any{
		"source":                  "v1_automation",
		"prompt_registry_text":    prompt.PromptText,
		"prompt_registry_version": prompt.Version,
	}
	if configFound {
		item.DraftRole, item.DraftTask = config.DraftRolePrompt, config.DraftTaskPrompt
		item.PublishedRole, item.PublishedTask = config.PublishedRolePrompt, config.PublishedTaskPrompt
		item.DraftVersion, item.PublishedVersion = int64(config.DraftVersion), int64(config.PublishedVersion)
		item.CreatedAt, item.UpdatedAt = earlier(item.CreatedAt, config.CreatedAt.UTC()), later(item.UpdatedAt, config.UpdatedAt.UTC())
		legacy["scenario_code"] = config.ScenarioCode
		legacy["pool_keys"] = config.PoolKeys
		legacy["draft_variables"] = config.DraftVariables
		legacy["draft_output_schema"] = config.DraftOutputSchema
		legacy["published_variables"] = config.PublishedVariables
		legacy["published_output_schema"] = config.PublishedOutputSchema
		legacy["published_at"] = config.PublishedAt
		legacy["published_by"] = config.PublishedBy
		legacy["last_modified_at"] = config.LastModifiedAt
		legacy["last_modified_by"] = config.LastModifiedBy
		legacy["last_modified_source"] = config.LastModifiedSource
		legacy["last_change_summary"] = config.LastChangeSummary
		legacy["submitted_for_publish"] = config.SubmittedForPublish
		legacy["submitted_at"] = config.SubmittedAt
		legacy["submitted_by"] = config.SubmittedBy
	} else {
		item.DraftTask, item.PublishedTask = prompt.PromptText, prompt.PromptText
		item.DraftVersion, item.PublishedVersion = int64(prompt.Version), int64(prompt.Version)
	}
	if item.DraftVersion < 1 || item.PublishedVersion < 1 || !validEditableText(item.DraftRole, 20_000) || !validEditableText(item.DraftTask, 20_000) || !validEditableText(item.PublishedRole, 20_000) || !validEditableText(item.PublishedTask, 20_000) {
		return EditableAgent{}, ErrEditableAgentInvalid
	}
	raw, err := json.Marshal(legacy)
	if err != nil || len(raw) > 100_000 {
		return EditableAgent{}, ErrEditableAgentInvalid
	}
	item.LegacyConfiguration = raw
	return item, nil
}

func validEditableCode(value string) bool {
	if value == "" || len(value) > 120 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func validEditableText(value string, limit int) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, '\x00') && len(value) <= limit
}

func earlier(left, right time.Time) time.Time {
	if right.Before(left) {
		return right
	}
	return left
}

func later(left, right time.Time) time.Time {
	if right.After(left) {
		return right
	}
	return left
}
