package v1profilecatalog

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAdaptHistoryProducesTypedCandidateWithoutV2Resolution(t *testing.T) {
	templates, categories, mappings, rules := catalogueFixtures()
	history := AdaptHistory(raw(t, templates), raw(t, categories), raw(t, mappings), raw(t, rules))
	if history.Templates[0].Disposition != DispositionCandidate || history.Categories[0].Disposition != DispositionCandidate || history.OptionMappings[0].Disposition != DispositionCandidate || history.SignupTagRules[0].Disposition != DispositionCandidate {
		t.Fatalf("expected typed candidates: %+v", history)
	}
	template := history.Templates[0].Fact
	if template.QuestionnaireSourceID == nil || *template.QuestionnaireSourceID != 901 || template.SegmentationQuestionSourceID != nil || template.ProgramSourceID == nil || *template.ProgramSourceID != 88 || template.OriginalEnabled || template.Version != -3 {
		t.Fatalf("template source fields changed: %+v", template)
	}
	if template.CreatedByDigest == (ActorDigest{}) || template.UpdatedByDigest == (ActorDigest{}) {
		t.Fatal("sensitive actors were not summarized")
	}
	mapping := history.OptionMappings[0].Fact
	if mapping.QuestionSourceID != 777 || mapping.OptionSourceID != 778 || mapping.TemplateSourceID != 1 || mapping.CategorySourceID != 10 {
		t.Fatalf("external V1 source references changed: %+v", mapping)
	}
	if rule := history.SignupTagRules[0].Fact; rule.TagSourceID != "tag-v1-1" || rule.SignupStatus != "approved" || rule.OriginalActive {
		t.Fatalf("tag rule source status changed: %+v", rule)
	}
	encoded, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"creator-private", "updater-private"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("source actor leaked from candidate: %q", secret)
		}
	}
}

func TestAdaptHistoryQuarantinesMissingParentsAndSourceMismatches(t *testing.T) {
	templates, categories, mappings, rules := catalogueFixtures()
	categories[0]["template_id"] = int64(999)
	mappings[0]["category_id"] = int64(999)
	history := AdaptHistory(raw(t, templates), raw(t, categories), raw(t, mappings), raw(t, rules))
	if got := history.Categories[0]; got.Disposition != DispositionQuarantine || got.Reason != "profile_segment_category_template_unresolved" {
		t.Fatalf("missing template parent was accepted: %+v", got)
	}
	if got := history.OptionMappings[0]; got.Disposition != DispositionQuarantine || got.Reason != "profile_segment_option_mapping_category_unresolved" {
		t.Fatalf("missing category parent was accepted: %+v", got)
	}

	templates, categories, mappings, rules = catalogueFixtures()
	mappings[0]["template_id"] = int64(2)
	history = AdaptHistory(raw(t, templates), raw(t, categories), raw(t, mappings), raw(t, rules))
	if got := history.OptionMappings[0]; got.Disposition != DispositionQuarantine || got.Reason != "profile_segment_option_mapping_template_category_mismatch" {
		t.Fatalf("inconsistent V1 template/category relationship was accepted: %+v", got)
	}
}

func TestAdaptHistoryPreservesNullableSourceValuesAndOriginalTime(t *testing.T) {
	templates, categories, mappings, rules := catalogueFixtures()
	zoned := time.Date(2026, 8, 28, 9, 30, 0, 123456000, time.FixedZone("source", 8*60*60))
	templates[0]["questionnaire_id"] = nil
	templates[0]["segmentation_question_id"] = nil
	templates[0]["program_id"] = nil
	templates[0]["created_at"] = zoned
	templates[0]["updated_at"] = zoned
	categories[0]["sort_order"] = int64(-8)
	history := AdaptHistory(raw(t, templates), raw(t, categories), raw(t, mappings), raw(t, rules))
	if fact := history.Templates[0].Fact; fact == nil || fact.QuestionnaireSourceID != nil || fact.SegmentationQuestionSourceID != nil || fact.ProgramSourceID != nil || fact.CreatedAt.Format(time.RFC3339Nano) != zoned.Format(time.RFC3339Nano) || fact.UpdatedAt.Format(time.RFC3339Nano) != zoned.Format(time.RFC3339Nano) {
		t.Fatalf("nullable or source time changed: %+v", fact)
	}
	if fact := history.Categories[0].Fact; fact == nil || fact.SortOrder != -8 {
		t.Fatalf("source integer changed: %+v", fact)
	}
}

func TestAdaptHistoryQuarantinesManifestTypeViolations(t *testing.T) {
	templates, categories, mappings, rules := catalogueFixtures()
	templates[0]["id"] = "1"
	categories[0]["enabled"] = "false"
	mappings[0]["option_id"] = "778"
	rules[0]["active"] = "false"
	history := AdaptHistory(raw(t, templates), raw(t, categories), raw(t, mappings), raw(t, rules))
	checks := []struct {
		name   string
		reason string
		actual string
		got    Disposition
	}{
		{"template", "profile_segment_template_shape_invalid", history.Templates[0].Reason, history.Templates[0].Disposition},
		{"category", "profile_segment_category_shape_invalid", history.Categories[0].Reason, history.Categories[0].Disposition},
		{"mapping", "profile_segment_option_mapping_shape_invalid", history.OptionMappings[0].Reason, history.OptionMappings[0].Disposition},
		{"tag rule", "signup_tag_rule_shape_invalid", history.SignupTagRules[0].Reason, history.SignupTagRules[0].Disposition},
	}
	for _, check := range checks {
		if check.got != DispositionQuarantine || check.actual != check.reason {
			t.Errorf("%s type violation = (%s, %q), want (%s, %q)", check.name, check.got, check.actual, DispositionQuarantine, check.reason)
		}
	}
}

func TestAdaptHistoryConservesThirtySourceRows(t *testing.T) {
	templates, categories, mappings, rules := catalogueFixtures()
	for len(templates) < 4 {
		value := copyMap(templates[0])
		value["id"] = int64(len(templates) + 1)
		value["template_code"] = "template-" + string(rune('0'+len(templates)+1))
		templates = append(templates, value)
	}
	for len(categories) < 10 {
		value := copyMap(categories[0])
		value["id"] = int64(len(categories) + 10)
		value["template_id"] = int64((len(categories) % 4) + 1)
		categories = append(categories, value)
	}
	for len(mappings) < 6 {
		value := copyMap(mappings[0])
		value["id"] = int64(len(mappings) + 100)
		value["category_id"] = int64(10)
		value["template_id"] = int64(1)
		mappings = append(mappings, value)
	}
	for len(rules) < 10 {
		value := copyMap(rules[0])
		value["tag_id"] = "tag-v1-" + string(rune('0'+len(rules)+1))
		rules = append(rules, value)
	}
	history := AdaptHistory(raw(t, templates), raw(t, categories), raw(t, mappings), raw(t, rules))
	if history.SourceCount() != 30 || history.TerminalCount() != 30 {
		t.Fatalf("row conservation failed: source=%d terminal=%d", history.SourceCount(), history.TerminalCount())
	}
}

func catalogueFixtures() ([]map[string]any, []map[string]any, []map[string]any, []map[string]any) {
	stamp := time.Date(2026, 8, 28, 9, 30, 0, 0, time.UTC)
	templates := []map[string]any{{
		"id": int64(1), "template_code": "legacy-profile", "template_name": "历史分类", "questionnaire_id": int64(901), "segmentation_question_id": nil,
		"description": "原始说明", "enabled": false, "version": int64(-3), "created_by": "creator-private", "updated_by": "updater-private", "created_at": stamp, "updated_at": stamp, "program_id": int64(88),
	}, {
		"id": int64(2), "template_code": "legacy-profile-two", "template_name": "第二历史分类", "questionnaire_id": nil, "segmentation_question_id": nil,
		"description": "第二说明", "enabled": true, "version": int64(1), "created_by": "creator-two", "updated_by": "updater-two", "created_at": stamp, "updated_at": stamp, "program_id": nil,
	}}
	categories := []map[string]any{{
		"id": int64(10), "template_id": int64(1), "category_key": "journey", "category_name": "旅程", "description": "分类说明", "sort_order": int64(0), "enabled": true, "created_at": stamp, "updated_at": stamp,
	}}
	mappings := []map[string]any{{
		"id": int64(100), "template_id": int64(1), "category_id": int64(10), "question_id": int64(777), "option_id": int64(778), "created_at": stamp,
	}}
	rules := []map[string]any{{
		"tag_id": "tag-v1-1", "tag_name": "报名成功", "signup_status": "approved", "active": false, "updated_at": stamp,
	}}
	return templates, categories, mappings, rules
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
