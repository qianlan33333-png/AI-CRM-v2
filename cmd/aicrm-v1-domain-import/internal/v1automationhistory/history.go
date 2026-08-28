// Package v1automationhistory classifies archived V1 automation rows as
// non-executable historical facts. It has no V2 store, command, queue, LLM,
// or Provider dependency.
package v1automationhistory

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	SOPTemplateTableID    = "public/automation_sop_template"
	AgentConfigTableID    = "public/automation_agent_config"
	PromptRegistryTableID = "public/automation_agent_prompt_registry"
	AgentsTableID         = "public/automation_agents"
)

type Disposition string

const (
	DispositionCandidate  Disposition = "historical_candidate"
	DispositionQuarantine Disposition = "quarantine"
)

// OpaqueDigest binds sensitive source material to its sealed archive without
// making it recoverable from this candidate.
type OpaqueDigest [sha256.Size]byte

// SOPTemplateFact is a display-only V1 SOP definition. It cannot schedule or
// send anything. Image source material is represented only by ImagesDigest.
type SOPTemplateFact struct {
	SourceID        int64
	PoolKey         string
	DayIndex        int32
	ContentMasked   string
	ImagesDigest    OpaqueDigest
	OriginalEnabled bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// AgentConfigFact preserves configuration provenance only. Prompt and JSON
// fields remain in the encrypted archive and are represented by ConfigDigest.
type AgentConfigFact struct {
	SourceID            int64
	AgentCode           string
	DisplayName         string
	ScenarioCode        string
	OriginalEnabled     bool
	DraftVersion        int32
	PublishedVersion    int32
	PublishedAt         string
	PublishedBy         string
	LastModifiedAt      string
	LastModifiedBy      string
	LastModifiedSource  string
	SubmittedForPublish bool
	SubmittedAt         string
	SubmittedBy         string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	ConfigDigest        OpaqueDigest
}

// PromptRegistryFact retains an inert prompt reference. PromptText itself is
// deliberately not available to a caller of this package.
type PromptRegistryFact struct {
	SourceID        int64
	AgentCode       string
	DisplayName     string
	OriginalEnabled bool
	Version         int32
	CreatedAt       time.Time
	UpdatedAt       time.Time
	PromptDigest    OpaqueDigest
}

// AgentFact records V1 workflow source references only. Its original type and
// status are never translated to an enabled V2 agent.
type AgentFact struct {
	SourceID            int64
	ProgramSourceID     int64
	WorkflowSourceID    int64
	NodeSourceID        int64
	TaskSourceID        int64
	AgentCode           string
	AgentName           string
	OriginalType        string
	OriginalStatus      string
	SortOrder           int32
	OriginalEnabled     bool
	CreatedBySource     string
	UpdatedBySource     string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	ArchivedAt          string
	ConfigurationDigest OpaqueDigest
}

type Result[T any] struct {
	SourceID    int64
	Disposition Disposition
	Reason      string
	Fact        *T
}

// History preserves input order and row count separately for all four source
// tables. No source reference is treated as a V2 ID or activation authority.
type History struct {
	SOPTemplates     []Result[SOPTemplateFact]
	AgentConfigs     []Result[AgentConfigFact]
	PromptRegistries []Result[PromptRegistryFact]
	Agents           []Result[AgentFact]
}

type sopTemplateJSON struct {
	ID        int64           `json:"id"`
	PoolKey   string          `json:"pool_key"`
	DayIndex  int32           `json:"day_index"`
	Content   string          `json:"content"`
	Images    json.RawMessage `json:"images_json"`
	Enabled   bool            `json:"enabled"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type agentConfigJSON struct {
	ID                    int64           `json:"id"`
	AgentCode             string          `json:"agent_code"`
	DisplayName           string          `json:"display_name"`
	PoolKeys              json.RawMessage `json:"pool_keys_json"`
	Enabled               bool            `json:"enabled"`
	DraftRolePrompt       string          `json:"draft_role_prompt"`
	DraftTaskPrompt       string          `json:"draft_task_prompt"`
	DraftVariables        json.RawMessage `json:"draft_variables_json"`
	DraftOutputSchema     json.RawMessage `json:"draft_output_schema_json"`
	PublishedRolePrompt   string          `json:"published_role_prompt"`
	PublishedTaskPrompt   string          `json:"published_task_prompt"`
	PublishedVariables    json.RawMessage `json:"published_variables_json"`
	PublishedOutputSchema json.RawMessage `json:"published_output_schema_json"`
	DraftVersion          int32           `json:"draft_version"`
	PublishedVersion      int32           `json:"published_version"`
	PublishedAt           string          `json:"published_at"`
	PublishedBy           string          `json:"published_by"`
	LastModifiedAt        string          `json:"last_modified_at"`
	LastModifiedBy        string          `json:"last_modified_by"`
	LastModifiedSource    string          `json:"last_modified_source"`
	LastChangeSummary     string          `json:"last_change_summary"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
	SubmittedForPublish   bool            `json:"submitted_for_publish"`
	SubmittedAt           string          `json:"submitted_at"`
	SubmittedBy           string          `json:"submitted_by"`
	ScenarioCode          string          `json:"scenario_code"`
}

type promptRegistryJSON struct {
	ID          int64     `json:"id"`
	AgentCode   string    `json:"agent_code"`
	DisplayName string    `json:"display_name"`
	PromptText  string    `json:"prompt_text"`
	Enabled     bool      `json:"enabled"`
	Version     int32     `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type agentJSON struct {
	ID         int64           `json:"id"`
	ProgramID  int64           `json:"program_id"`
	WorkflowID int64           `json:"workflow_id"`
	NodeID     int64           `json:"node_id"`
	TaskID     int64           `json:"task_id"`
	AgentCode  string          `json:"agent_code"`
	AgentName  string          `json:"agent_name"`
	AgentType  string          `json:"agent_type"`
	Status     string          `json:"status"`
	SortOrder  int32           `json:"sort_order"`
	Metadata   json.RawMessage `json:"metadata_json"`
	Config     json.RawMessage `json:"config_json"`
	Enabled    bool            `json:"enabled"`
	CreatedBy  string          `json:"created_by"`
	UpdatedBy  string          `json:"updated_by"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	ArchivedAt string          `json:"archived_at"`
}

// AdaptHistory parses the four frozen V1 automation tables. It is deliberately
// pure: a candidate cannot create an active agent, call an LLM, or emit work.
func AdaptHistory(sopTemplates, agentConfigs, promptRegistries, agents []json.RawMessage) History {
	history := History{
		SOPTemplates:     make([]Result[SOPTemplateFact], len(sopTemplates)),
		AgentConfigs:     make([]Result[AgentConfigFact], len(agentConfigs)),
		PromptRegistries: make([]Result[PromptRegistryFact], len(promptRegistries)),
		Agents:           make([]Result[AgentFact], len(agents)),
	}
	for index, row := range sopTemplates {
		history.SOPTemplates[index] = adaptSOPTemplate(row)
	}
	for index, row := range agentConfigs {
		history.AgentConfigs[index] = adaptAgentConfig(row)
	}
	for index, row := range promptRegistries {
		history.PromptRegistries[index] = adaptPromptRegistry(row)
	}
	for index, row := range agents {
		history.Agents[index] = adaptAgent(row)
	}
	quarantineDuplicateIDs(history.SOPTemplates, "automation_sop_template_source_ambiguous")
	quarantineDuplicateIDs(history.AgentConfigs, "automation_agent_config_source_ambiguous")
	quarantineDuplicateIDs(history.PromptRegistries, "automation_agent_prompt_registry_source_ambiguous")
	quarantineDuplicateIDs(history.Agents, "automation_agents_source_ambiguous")
	return history
}

func adaptSOPTemplate(value json.RawMessage) Result[SOPTemplateFact] {
	fields, ok := object(value)
	if !ok {
		return quarantine[SOPTemplateFact](0, "automation_sop_template_json_invalid")
	}
	sourceID := sourceID(fields)
	var source sopTemplateJSON
	if !decode(fields, value, &source, "id", "pool_key", "day_index", "content", "images_json", "enabled", "created_at", "updated_at") || source.ID < 1 || source.CreatedAt.IsZero() || source.UpdatedAt.IsZero() {
		return quarantine[SOPTemplateFact](sourceID, "automation_sop_template_shape_invalid")
	}
	content, masked := maskText(source.Content)
	images, digested := opaque(fields, "images_json")
	if !masked || !digested {
		return quarantine[SOPTemplateFact](source.ID, "automation_sop_template_shape_invalid")
	}
	return candidate(SOPTemplateFact{SourceID: source.ID, PoolKey: source.PoolKey, DayIndex: source.DayIndex, ContentMasked: content, ImagesDigest: images, OriginalEnabled: source.Enabled, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt})
}

func adaptAgentConfig(value json.RawMessage) Result[AgentConfigFact] {
	fields, ok := object(value)
	if !ok {
		return quarantine[AgentConfigFact](0, "automation_agent_config_json_invalid")
	}
	sourceID := sourceID(fields)
	var source agentConfigJSON
	if !decode(fields, value, &source,
		"id", "agent_code", "display_name", "pool_keys_json", "enabled", "draft_role_prompt", "draft_task_prompt", "draft_variables_json", "draft_output_schema_json",
		"published_role_prompt", "published_task_prompt", "published_variables_json", "published_output_schema_json", "draft_version", "published_version", "published_at", "published_by",
		"last_modified_at", "last_modified_by", "last_modified_source", "last_change_summary", "created_at", "updated_at", "submitted_for_publish", "submitted_at", "submitted_by", "scenario_code") || source.ID < 1 || source.CreatedAt.IsZero() || source.UpdatedAt.IsZero() {
		return quarantine[AgentConfigFact](sourceID, "automation_agent_config_shape_invalid")
	}
	digest, digested := opaque(fields,
		"pool_keys_json", "draft_role_prompt", "draft_task_prompt", "draft_variables_json", "draft_output_schema_json", "published_role_prompt", "published_task_prompt",
		"published_variables_json", "published_output_schema_json", "last_change_summary")
	if !digested {
		return quarantine[AgentConfigFact](source.ID, "automation_agent_config_shape_invalid")
	}
	return candidate(AgentConfigFact{SourceID: source.ID, AgentCode: source.AgentCode, DisplayName: source.DisplayName, ScenarioCode: source.ScenarioCode,
		OriginalEnabled: source.Enabled, DraftVersion: source.DraftVersion, PublishedVersion: source.PublishedVersion, PublishedAt: source.PublishedAt, PublishedBy: source.PublishedBy,
		LastModifiedAt: source.LastModifiedAt, LastModifiedBy: source.LastModifiedBy, LastModifiedSource: source.LastModifiedSource, SubmittedForPublish: source.SubmittedForPublish,
		SubmittedAt: source.SubmittedAt, SubmittedBy: source.SubmittedBy, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt, ConfigDigest: digest})
}

func adaptPromptRegistry(value json.RawMessage) Result[PromptRegistryFact] {
	fields, ok := object(value)
	if !ok {
		return quarantine[PromptRegistryFact](0, "automation_agent_prompt_registry_json_invalid")
	}
	sourceID := sourceID(fields)
	var source promptRegistryJSON
	if !decode(fields, value, &source, "id", "agent_code", "display_name", "prompt_text", "enabled", "version", "created_at", "updated_at") || source.ID < 1 || source.CreatedAt.IsZero() || source.UpdatedAt.IsZero() {
		return quarantine[PromptRegistryFact](sourceID, "automation_agent_prompt_registry_shape_invalid")
	}
	digest, digested := opaque(fields, "prompt_text")
	if !digested {
		return quarantine[PromptRegistryFact](source.ID, "automation_agent_prompt_registry_shape_invalid")
	}
	return candidate(PromptRegistryFact{SourceID: source.ID, AgentCode: source.AgentCode, DisplayName: source.DisplayName, OriginalEnabled: source.Enabled, Version: source.Version, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt, PromptDigest: digest})
}

func adaptAgent(value json.RawMessage) Result[AgentFact] {
	fields, ok := object(value)
	if !ok {
		return quarantine[AgentFact](0, "automation_agents_json_invalid")
	}
	sourceID := sourceID(fields)
	var source agentJSON
	if !decode(fields, value, &source,
		"id", "program_id", "workflow_id", "node_id", "task_id", "agent_code", "agent_name", "agent_type", "status", "sort_order", "metadata_json", "config_json",
		"enabled", "created_by", "updated_by", "created_at", "updated_at", "archived_at") || source.ID < 1 || source.CreatedAt.IsZero() || source.UpdatedAt.IsZero() {
		return quarantine[AgentFact](sourceID, "automation_agents_shape_invalid")
	}
	digest, digested := opaque(fields, "metadata_json", "config_json")
	if !digested {
		return quarantine[AgentFact](source.ID, "automation_agents_shape_invalid")
	}
	return candidate(AgentFact{SourceID: source.ID, ProgramSourceID: source.ProgramID, WorkflowSourceID: source.WorkflowID, NodeSourceID: source.NodeID, TaskSourceID: source.TaskID,
		AgentCode: source.AgentCode, AgentName: source.AgentName, OriginalType: source.AgentType, OriginalStatus: source.Status, SortOrder: source.SortOrder,
		OriginalEnabled: source.Enabled, CreatedBySource: source.CreatedBy, UpdatedBySource: source.UpdatedBy, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt,
		ArchivedAt: source.ArchivedAt, ConfigurationDigest: digest})
}

func candidate[T any](fact T) Result[T] {
	sourceID := sourceIDFromFact(fact)
	return Result[T]{SourceID: sourceID, Disposition: DispositionCandidate, Fact: &fact}
}

func quarantine[T any](sourceID int64, reason string) Result[T] {
	return Result[T]{SourceID: sourceID, Disposition: DispositionQuarantine, Reason: reason}
}

func quarantineDuplicateIDs[T any](values []Result[T], reason string) {
	counts := make(map[int64]int, len(values))
	for _, value := range values {
		if value.Disposition == DispositionCandidate && value.Fact != nil {
			counts[value.SourceID]++
		}
	}
	for index := range values {
		if values[index].Disposition == DispositionCandidate && values[index].Fact != nil && counts[values[index].SourceID] > 1 {
			values[index] = quarantine[T](values[index].SourceID, reason)
		}
	}
}

func sourceIDFromFact(value any) int64 {
	switch value := value.(type) {
	case SOPTemplateFact:
		return value.SourceID
	case AgentConfigFact:
		return value.SourceID
	case PromptRegistryFact:
		return value.SourceID
	case AgentFact:
		return value.SourceID
	default:
		return 0
	}
}

type fields map[string]json.RawMessage

func object(value json.RawMessage) (fields, bool) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	result := make(fields)
	if decoder.Decode(&result) != nil || result == nil {
		return nil, false
	}
	var extra any
	return result, errors.Is(decoder.Decode(&extra), io.EOF)
}

func decode(source fields, value json.RawMessage, target any, names ...string) bool {
	for _, name := range names {
		raw, found := source[name]
		if !found || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return false
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	if decoder.Decode(target) != nil {
		return false
	}
	var extra any
	return errors.Is(decoder.Decode(&extra), io.EOF)
}

func sourceID(source fields) int64 {
	raw, found := source["id"]
	if !found || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return 0
	}
	var id int64
	if json.Unmarshal(raw, &id) != nil || id < 1 {
		return 0
	}
	return id
}

func opaque(source fields, names ...string) (OpaqueDigest, bool) {
	values := make(map[string]json.RawMessage, len(names))
	for _, name := range names {
		raw, found := source[name]
		if !found || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || !json.Valid(raw) {
			return OpaqueDigest{}, false
		}
		values[name] = append(json.RawMessage(nil), raw...)
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return OpaqueDigest{}, false
	}
	sum := sha256.Sum256(append([]byte("v1-automation-history-opaque-v1\x00"), encoded...))
	return OpaqueDigest(sum), true
}

// maskText matches the existing historical message display policy: only a
// continuous Chinese mobile number is replaced, and all other text is kept.
func maskText(value string) (string, bool) {
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return "", false
	}
	var masked strings.Builder
	masked.Grow(len(value))
	for offset := 0; offset < len(value); {
		end := offset
		if value[offset] == '+' {
			end++
		}
		for end < len(value) && value[end] >= '0' && value[end] <= '9' {
			end++
		}
		if phoneLike(value[offset:end]) {
			masked.WriteString("[masked-phone]")
			offset = end
			continue
		}
		_, width := utf8.DecodeRuneInString(value[offset:])
		masked.WriteString(value[offset : offset+width])
		offset += width
	}
	return masked.String(), true
}

func phoneLike(value string) bool {
	digits := strings.TrimPrefix(value, "+86")
	return len(digits) == 11 && digits[0] == '1' && digits[1] >= '3' && digits[1] <= '9'
}
