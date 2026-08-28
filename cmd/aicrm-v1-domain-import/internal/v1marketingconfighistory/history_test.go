package v1marketingconfighistory

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAdaptHistoryPreservesInertConfigAndRuleFacts(t *testing.T) {
	config, rule := marketingFixtures()
	history := AdaptHistory([]json.RawMessage{rawMarketing(t, config)}, []json.RawMessage{rawMarketing(t, rule)})
	configFact := mustConfigCandidate(t, history.Configs[0])
	if configFact.SourceID != 9 || configFact.AutomationKey != "onboard-v1" || configFact.OriginalStatus != "active" || configFact.DoNotStartAfterHour != 0 || configFact.ConfigPayloadDigest == (OpaqueDigest{}) {
		t.Fatal("config source fact was changed")
	}
	if configFact.CreatedAt.Format(time.RFC3339Nano) != "2026-08-28T09:30:00.123456+08:00" || configFact.UpdatedAt.Format(time.RFC3339Nano) != "2026-08-27T09:30:00.123456+08:00" {
		t.Fatal("config source times were normalized")
	}
	ruleFact := mustRuleCandidate(t, history.Rules[0])
	if ruleFact.SourceID != 10 || ruleFact.ConfigSourceID != 9 || ruleFact.QuestionnaireSourceID == nil || *ruleFact.QuestionnaireSourceID != 0 || ruleFact.QuestionSourceID != nil || ruleFact.ScoreDelta != -7 || ruleFact.SortOrder != -2 || ruleFact.OriginalActive || ruleFact.AnswerMatchValueDigest == (OpaqueDigest{}) || ruleFact.RulePayloadDigest == (OpaqueDigest{}) {
		t.Fatal("rule source values were changed")
	}
	encoded, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"config-secret", "answer-secret", "rule-secret"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("opaque JSON source material escaped candidate")
		}
	}
}

func TestAdaptHistoryQuarantinesUnresolvedAndAmbiguousSources(t *testing.T) {
	config, rule := marketingFixtures()
	missingParent := rule
	missingParent["automation_config_id"] = int64(999)
	result := AdaptHistory([]json.RawMessage{rawMarketing(t, config)}, []json.RawMessage{rawMarketing(t, missingParent)})
	if decision := result.Rules[0]; decision.Disposition != DispositionQuarantine || decision.Reason != "marketing_automation_question_rule_config_unresolved" || decision.SourceID != 10 || decision.Fact != nil {
		t.Fatal("rule with absent source config was accepted")
	}
	duplicate := AdaptHistory([]json.RawMessage{rawMarketing(t, config), rawMarketing(t, config)}, []json.RawMessage{rawMarketing(t, rule)})
	for _, decision := range duplicate.Configs {
		if decision.Disposition != DispositionQuarantine || decision.Reason != "marketing_automation_config_source_ambiguous" {
			t.Fatal("ambiguous configs were accepted")
		}
	}
	if decision := duplicate.Rules[0]; decision.Disposition != DispositionQuarantine || decision.Reason != "marketing_automation_question_rule_config_unresolved" {
		t.Fatal("rule did not fail closed after ambiguous config")
	}
	invalid := rule
	invalid["is_active"] = "false"
	if decision := AdaptRule(rawMarketing(t, invalid)); decision.Disposition != DispositionQuarantine || decision.Reason != "marketing_automation_question_rule_shape_invalid" {
		t.Fatal("invalid source scalar type was accepted")
	}
}

func TestCountMarketingResultsCountsTypedRows(t *testing.T) {
	config, rule := marketingFixtures()
	candidates, quarantined, _, valid := countMarketingResults(AdaptHistory([]json.RawMessage{rawMarketing(t, config)}, []json.RawMessage{rawMarketing(t, rule)}))
	if !valid || candidates != 2 || quarantined != 0 {
		t.Fatal("typed candidate counts were not preserved")
	}
}

func marketingFixtures() (map[string]any, map[string]any) {
	created := time.Date(2026, 8, 28, 9, 30, 0, 123456000, time.FixedZone("v1-source", 8*60*60))
	updated := created.Add(-24 * time.Hour)
	config := map[string]any{
		"id": int64(9), "automation_key": "onboard-v1", "automation_name": "历史欢迎", "target_event": "signup_success", "channel_type": "text_message", "status": "active", "do_not_start_after_hour": int32(0), "config_payload_json": map[string]any{"secret": "config-secret"}, "created_at": created, "updated_at": updated,
	}
	rule := map[string]any{
		"id": int64(10), "automation_config_id": int64(9), "questionnaire_id": int64(0), "question_id": nil, "rule_code": "source-rule", "rule_name": "来源规则", "answer_match_type": "any_of", "answer_match_value_json": []any{"answer-secret"}, "score_delta": int32(-7), "segment_hint": "legacy", "stage_hint": "history", "is_active": false, "sort_order": int32(-2), "rule_payload_json": map[string]any{"secret": "rule-secret"}, "created_at": created, "updated_at": updated,
	}
	return config, rule
}

func mustConfigCandidate(t *testing.T, value Result[ConfigFact]) *ConfigFact {
	t.Helper()
	if value.Disposition != DispositionCandidate || value.Fact == nil || value.Reason != "" {
		t.Fatal("expected config candidate")
	}
	return value.Fact
}

func mustRuleCandidate(t *testing.T, value Result[RuleFact]) *RuleFact {
	t.Helper()
	if value.Disposition != DispositionCandidate || value.Fact == nil || value.Reason != "" {
		t.Fatal("expected rule candidate")
	}
	return value.Fact
}

func rawMarketing(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
