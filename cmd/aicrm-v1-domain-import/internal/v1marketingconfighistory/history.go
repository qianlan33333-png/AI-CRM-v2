// Package v1marketingconfighistory parses V1 marketing configuration as inert
// history. It has no V2 automation runtime, enrollment, event, queue, LLM, or
// Provider dependency.
package v1marketingconfighistory

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"time"
)

const (
	ConfigTableID = "public/marketing_automation_configs"
	RulesTableID  = "public/marketing_automation_question_rules"
)

type Disposition string

const (
	DispositionCandidate  Disposition = "historical_candidate"
	DispositionQuarantine Disposition = "quarantine"
)

type OpaqueDigest [sha256.Size]byte

// ConfigFact retains source configuration only. OriginalStatus is never
// converted into a live automation status.
type ConfigFact struct {
	SourceID            int64        `json:"source_id"`
	AutomationKey       string       `json:"automation_key"`
	AutomationName      string       `json:"automation_name"`
	TargetEvent         string       `json:"target_event"`
	ChannelType         string       `json:"channel_type"`
	OriginalStatus      string       `json:"original_status"`
	DoNotStartAfterHour int32        `json:"do_not_start_after_hour"`
	CreatedAt           time.Time    `json:"created_at"`
	UpdatedAt           time.Time    `json:"updated_at"`
	ConfigPayloadDigest OpaqueDigest `json:"config_payload_digest"`
}

// RuleFact retains a source-only rule relation. QuestionnaireSourceID and
// QuestionSourceID are not V2 IDs and must be crosswalked later if needed.
type RuleFact struct {
	SourceID               int64        `json:"source_id"`
	ConfigSourceID         int64        `json:"config_source_id"`
	QuestionnaireSourceID  *int64       `json:"questionnaire_source_id,omitempty"`
	QuestionSourceID       *int64       `json:"question_source_id,omitempty"`
	RuleCode               string       `json:"rule_code"`
	RuleName               string       `json:"rule_name"`
	AnswerMatchType        string       `json:"answer_match_type"`
	ScoreDelta             int32        `json:"score_delta"`
	SegmentHint            string       `json:"segment_hint"`
	StageHint              string       `json:"stage_hint"`
	OriginalActive         bool         `json:"original_active"`
	SortOrder              int32        `json:"sort_order"`
	CreatedAt              time.Time    `json:"created_at"`
	UpdatedAt              time.Time    `json:"updated_at"`
	AnswerMatchValueDigest OpaqueDigest `json:"answer_match_value_digest"`
	RulePayloadDigest      OpaqueDigest `json:"rule_payload_digest"`
}

type Result[T any] struct {
	SourceID    int64       `json:"source_id"`
	Disposition Disposition `json:"disposition"`
	Reason      string      `json:"reason,omitempty"`
	Fact        *T          `json:"fact,omitempty"`
}

type History struct {
	Configs []Result[ConfigFact]
	Rules   []Result[RuleFact]
}

type configJSON struct {
	ID                  int64           `json:"id"`
	AutomationKey       string          `json:"automation_key"`
	AutomationName      string          `json:"automation_name"`
	TargetEvent         string          `json:"target_event"`
	ChannelType         string          `json:"channel_type"`
	Status              string          `json:"status"`
	DoNotStartAfterHour int32           `json:"do_not_start_after_hour"`
	ConfigPayload       json.RawMessage `json:"config_payload_json"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

type ruleJSON struct {
	ID               int64           `json:"id"`
	ConfigID         int64           `json:"automation_config_id"`
	QuestionnaireID  *int64          `json:"questionnaire_id"`
	QuestionID       *int64          `json:"question_id"`
	RuleCode         string          `json:"rule_code"`
	RuleName         string          `json:"rule_name"`
	AnswerMatchType  string          `json:"answer_match_type"`
	AnswerMatchValue json.RawMessage `json:"answer_match_value_json"`
	ScoreDelta       int32           `json:"score_delta"`
	SegmentHint      string          `json:"segment_hint"`
	StageHint        string          `json:"stage_hint"`
	IsActive         bool            `json:"is_active"`
	SortOrder        int32           `json:"sort_order"`
	RulePayload      json.RawMessage `json:"rule_payload_json"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

const configRequiredFields = "id automation_key automation_name target_event channel_type status do_not_start_after_hour config_payload_json created_at updated_at"
const ruleRequiredFields = "id automation_config_id rule_code rule_name answer_match_type answer_match_value_json score_delta segment_hint stage_hint is_active sort_order rule_payload_json created_at updated_at"
const ruleNullableFields = "questionnaire_id question_id"

// AdaptHistory preserves all input rows. Rules are candidates only if exactly
// one visible source config parent exists; neither status nor is_active can
// activate a V2 automation.
func AdaptHistory(configs, rules []json.RawMessage) History {
	history := History{Configs: make([]Result[ConfigFact], len(configs)), Rules: make([]Result[RuleFact], len(rules))}
	for index, row := range configs {
		history.Configs[index] = AdaptConfig(row)
	}
	quarantineDuplicateConfigs(history.Configs)
	parents := configParents(history.Configs)
	for index, row := range rules {
		history.Rules[index] = AdaptRule(row)
		if fact := history.Rules[index].Fact; fact != nil && parents[fact.ConfigSourceID] == nil {
			history.Rules[index] = quarantine[RuleFact](fact.SourceID, "marketing_automation_question_rule_config_unresolved")
		}
	}
	quarantineDuplicateRules(history.Rules)
	return history
}

func AdaptConfig(payload json.RawMessage) Result[ConfigFact] {
	fields, ok := marketingObject(payload)
	if !ok {
		return quarantine[ConfigFact](0, "marketing_automation_config_json_invalid")
	}
	sourceID := marketingSourceID(fields)
	var source configJSON
	if !decodeMarketing(fields, payload, &source, configRequiredFields, "") || source.ID < 1 || source.CreatedAt.IsZero() || source.UpdatedAt.IsZero() {
		return quarantine[ConfigFact](sourceID, "marketing_automation_config_shape_invalid")
	}
	digest, ok := marketingFieldDigest(fields, "config_payload_json")
	if !ok {
		return quarantine[ConfigFact](source.ID, "marketing_automation_config_shape_invalid")
	}
	fact := ConfigFact{SourceID: source.ID, AutomationKey: source.AutomationKey, AutomationName: source.AutomationName, TargetEvent: source.TargetEvent, ChannelType: source.ChannelType, OriginalStatus: source.Status, DoNotStartAfterHour: source.DoNotStartAfterHour, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt, ConfigPayloadDigest: digest}
	return Result[ConfigFact]{SourceID: fact.SourceID, Disposition: DispositionCandidate, Fact: &fact}
}

func AdaptRule(payload json.RawMessage) Result[RuleFact] {
	fields, ok := marketingObject(payload)
	if !ok {
		return quarantine[RuleFact](0, "marketing_automation_question_rule_json_invalid")
	}
	sourceID := marketingSourceID(fields)
	var source ruleJSON
	if !decodeMarketing(fields, payload, &source, ruleRequiredFields, ruleNullableFields) || source.ID < 1 || source.ConfigID < 1 || source.CreatedAt.IsZero() || source.UpdatedAt.IsZero() {
		return quarantine[RuleFact](sourceID, "marketing_automation_question_rule_shape_invalid")
	}
	answer, answerOK := marketingFieldDigest(fields, "answer_match_value_json")
	payloadDigest, payloadOK := marketingFieldDigest(fields, "rule_payload_json")
	if !answerOK || !payloadOK {
		return quarantine[RuleFact](source.ID, "marketing_automation_question_rule_shape_invalid")
	}
	fact := RuleFact{SourceID: source.ID, ConfigSourceID: source.ConfigID, QuestionnaireSourceID: source.QuestionnaireID, QuestionSourceID: source.QuestionID, RuleCode: source.RuleCode, RuleName: source.RuleName, AnswerMatchType: source.AnswerMatchType, ScoreDelta: source.ScoreDelta, SegmentHint: source.SegmentHint, StageHint: source.StageHint, OriginalActive: source.IsActive, SortOrder: source.SortOrder, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt, AnswerMatchValueDigest: answer, RulePayloadDigest: payloadDigest}
	return Result[RuleFact]{SourceID: fact.SourceID, Disposition: DispositionCandidate, Fact: &fact}
}

func quarantine[T any](sourceID int64, reason string) Result[T] {
	return Result[T]{SourceID: sourceID, Disposition: DispositionQuarantine, Reason: reason}
}

func quarantineDuplicateConfigs(rows []Result[ConfigFact]) {
	counts := make(map[int64]int, len(rows))
	for _, row := range rows {
		if row.Fact != nil && row.Disposition == DispositionCandidate {
			counts[row.SourceID]++
		}
	}
	for index := range rows {
		if rows[index].Fact != nil && counts[rows[index].SourceID] > 1 {
			rows[index] = quarantine[ConfigFact](rows[index].SourceID, "marketing_automation_config_source_ambiguous")
		}
	}
}

func configParents(rows []Result[ConfigFact]) map[int64]*ConfigFact {
	parents := make(map[int64]*ConfigFact, len(rows))
	for index := range rows {
		if rows[index].Disposition == DispositionCandidate && rows[index].Fact != nil {
			parents[rows[index].SourceID] = rows[index].Fact
		}
	}
	return parents
}

func quarantineDuplicateRules(rows []Result[RuleFact]) {
	counts := make(map[int64]int, len(rows))
	for _, row := range rows {
		if row.Fact != nil && row.Disposition == DispositionCandidate {
			counts[row.SourceID]++
		}
	}
	for index := range rows {
		if rows[index].Fact != nil && counts[rows[index].SourceID] > 1 {
			rows[index] = quarantine[RuleFact](rows[index].SourceID, "marketing_automation_question_rule_source_ambiguous")
		}
	}
}

func marketingFieldDigest(source marketingFields, name string) (OpaqueDigest, bool) {
	raw, found := source[name]
	if !found || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || !json.Valid(raw) {
		return OpaqueDigest{}, false
	}
	sum := sha256.Sum256(append(append([]byte("v1-marketing-config-history-field-v1\x00"), name...), raw...))
	return OpaqueDigest(sum), true
}

type marketingFields map[string]json.RawMessage

func marketingObject(value json.RawMessage) (marketingFields, bool) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	fields := make(marketingFields)
	if decoder.Decode(&fields) != nil || fields == nil {
		return nil, false
	}
	var extra any
	return fields, errors.Is(decoder.Decode(&extra), io.EOF)
}

func decodeMarketing(fields marketingFields, payload json.RawMessage, target any, required, nullable string) bool {
	for _, names := range []string{required, nullable} {
		for _, name := range bytes.Fields([]byte(names)) {
			raw, found := fields[string(name)]
			if !found || (names == required && bytes.Equal(bytes.TrimSpace(raw), []byte("null"))) {
				return false
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if decoder.Decode(target) != nil {
		return false
	}
	var extra any
	return errors.Is(decoder.Decode(&extra), io.EOF)
}

func marketingSourceID(fields marketingFields) int64 {
	raw, found := fields["id"]
	if !found || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return 0
	}
	var id int64
	if json.Unmarshal(raw, &id) != nil || id < 1 {
		return 0
	}
	return id
}
