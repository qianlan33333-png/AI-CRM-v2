// Package v1profilecatalog classifies frozen V1 profile catalogue rows without
// writing a V2 record, resolving a V2 foreign key, or enabling a legacy rule.
package v1profilecatalog

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"time"
)

type Disposition string

const (
	DispositionCandidate  Disposition = "historical_candidate"
	DispositionQuarantine Disposition = "quarantine"
)

// ActorDigest retains only a stable summary of a sensitive V1 actor value.
// The original value stays in the sealed source archive.
type ActorDigest [sha256.Size]byte

// TemplateFact is a typed, non-executable V1 template record. Every *SourceID
// field is a V1 reference, never a V2 foreign key.
type TemplateFact struct {
	SourceID                     int64
	TemplateCode                 string
	TemplateName                 string
	QuestionnaireSourceID        *int64
	SegmentationQuestionSourceID *int64
	Description                  string
	OriginalEnabled              bool
	Version                      int64
	CreatedByDigest              ActorDigest
	UpdatedByDigest              ActorDigest
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
	ProgramSourceID              *int64
}

type CategoryFact struct {
	SourceID         int64
	TemplateSourceID int64
	CategoryKey      string
	CategoryName     string
	Description      string
	SortOrder        int64
	OriginalEnabled  bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type OptionMappingFact struct {
	SourceID         int64
	TemplateSourceID int64
	CategorySourceID int64
	QuestionSourceID int64
	OptionSourceID   int64
	CreatedAt        time.Time
}

type SignupTagRuleFact struct {
	TagSourceID    string
	TagName        string
	SignupStatus   string
	OriginalActive bool
	UpdatedAt      time.Time
}

type TemplateResult struct {
	Disposition Disposition
	Reason      string
	Fact        *TemplateFact
}

type CategoryResult struct {
	Disposition Disposition
	Reason      string
	Fact        *CategoryFact
}

type OptionMappingResult struct {
	Disposition Disposition
	Reason      string
	Fact        *OptionMappingFact
}

type SignupTagRuleResult struct {
	Disposition Disposition
	Reason      string
	Fact        *SignupTagRuleFact
}

// History preserves input order and row count for all four frozen source
// tables. It is deliberately not an import command or target persistence DTO.
type History struct {
	Templates      []TemplateResult
	Categories     []CategoryResult
	OptionMappings []OptionMappingResult
	SignupTagRules []SignupTagRuleResult
}

// SourceCount and TerminalCount make row conservation explicit. Every source
// row receives exactly one terminal candidate or quarantine result.
func (value History) SourceCount() int {
	return len(value.Templates) + len(value.Categories) + len(value.OptionMappings) + len(value.SignupTagRules)
}

func (value History) TerminalCount() int {
	count := 0
	for _, result := range value.Templates {
		if result.Disposition == DispositionCandidate || result.Disposition == DispositionQuarantine {
			count++
		}
	}
	for _, result := range value.Categories {
		if result.Disposition == DispositionCandidate || result.Disposition == DispositionQuarantine {
			count++
		}
	}
	for _, result := range value.OptionMappings {
		if result.Disposition == DispositionCandidate || result.Disposition == DispositionQuarantine {
			count++
		}
	}
	for _, result := range value.SignupTagRules {
		if result.Disposition == DispositionCandidate || result.Disposition == DispositionQuarantine {
			count++
		}
	}
	return count
}

// AdaptHistory decodes only the four frozen manifest shapes. It does not
// validate references outside this small source graph: questionnaire, question,
// option, tag, and program IDs remain historical references for a later owner.
func AdaptHistory(templates, categories, optionMappings, signupTagRules []json.RawMessage) History {
	history := History{
		Templates:      make([]TemplateResult, len(templates)),
		Categories:     make([]CategoryResult, len(categories)),
		OptionMappings: make([]OptionMappingResult, len(optionMappings)),
		SignupTagRules: make([]SignupTagRuleResult, len(signupTagRules)),
	}
	for index, row := range templates {
		history.Templates[index] = adaptTemplate(row)
	}
	templateIDs := uniqueTemplateIDs(history.Templates)

	for index, row := range categories {
		history.Categories[index] = adaptCategory(row)
	}
	for index := range history.Categories {
		fact := history.Categories[index].Fact
		if fact != nil {
			if _, found := templateIDs[fact.TemplateSourceID]; !found {
				quarantineCategory(&history.Categories[index], "profile_segment_category_template_unresolved")
			}
		}
	}
	categoryIDs := uniqueCategoryIDs(history.Categories)

	for index, row := range optionMappings {
		history.OptionMappings[index] = adaptOptionMapping(row)
	}
	for index := range history.OptionMappings {
		fact := history.OptionMappings[index].Fact
		if fact == nil {
			continue
		}
		if _, found := templateIDs[fact.TemplateSourceID]; !found {
			quarantineOptionMapping(&history.OptionMappings[index], "profile_segment_option_mapping_template_unresolved")
			continue
		}
		category, found := categoryIDs[fact.CategorySourceID]
		if !found {
			quarantineOptionMapping(&history.OptionMappings[index], "profile_segment_option_mapping_category_unresolved")
			continue
		}
		if category.TemplateSourceID != fact.TemplateSourceID {
			quarantineOptionMapping(&history.OptionMappings[index], "profile_segment_option_mapping_template_category_mismatch")
		}
	}
	uniqueOptionMappingIDs(history.OptionMappings)

	for index, row := range signupTagRules {
		history.SignupTagRules[index] = adaptSignupTagRule(row)
	}
	uniqueSignupTagRuleIDs(history.SignupTagRules)
	return history
}

func adaptTemplate(value json.RawMessage) TemplateResult {
	fields, ok := object(value)
	id, idOK := required[int64](fields, "id")
	code, codeOK := required[string](fields, "template_code")
	name, nameOK := required[string](fields, "template_name")
	questionnaireID, questionnaireOK := optional[int64](fields, "questionnaire_id")
	segmentationQuestionID, questionOK := optional[int64](fields, "segmentation_question_id")
	description, descriptionOK := required[string](fields, "description")
	enabled, enabledOK := required[bool](fields, "enabled")
	version, versionOK := required[int64](fields, "version")
	createdBy, createdByOK := required[string](fields, "created_by")
	updatedBy, updatedByOK := required[string](fields, "updated_by")
	createdAt, createdAtOK := required[time.Time](fields, "created_at")
	updatedAt, updatedAtOK := required[time.Time](fields, "updated_at")
	programID, programOK := optional[int64](fields, "program_id")
	if !ok || !idOK || !codeOK || !nameOK || !questionnaireOK || !questionOK || !descriptionOK || !enabledOK || !versionOK || !createdByOK || !updatedByOK || !createdAtOK || !updatedAtOK || !programOK {
		return TemplateResult{Disposition: DispositionQuarantine, Reason: "profile_segment_template_shape_invalid"}
	}
	return TemplateResult{Disposition: DispositionCandidate, Fact: &TemplateFact{
		SourceID: id, TemplateCode: code, TemplateName: name, QuestionnaireSourceID: questionnaireID, SegmentationQuestionSourceID: segmentationQuestionID,
		Description: description, OriginalEnabled: enabled, Version: version, CreatedByDigest: actorDigest("created_by", createdBy), UpdatedByDigest: actorDigest("updated_by", updatedBy),
		CreatedAt: createdAt, UpdatedAt: updatedAt, ProgramSourceID: programID,
	}}
}

func adaptCategory(value json.RawMessage) CategoryResult {
	fields, ok := object(value)
	id, idOK := required[int64](fields, "id")
	templateID, templateOK := required[int64](fields, "template_id")
	key, keyOK := required[string](fields, "category_key")
	name, nameOK := required[string](fields, "category_name")
	description, descriptionOK := required[string](fields, "description")
	sortOrder, sortOK := required[int64](fields, "sort_order")
	enabled, enabledOK := required[bool](fields, "enabled")
	createdAt, createdAtOK := required[time.Time](fields, "created_at")
	updatedAt, updatedAtOK := required[time.Time](fields, "updated_at")
	if !ok || !idOK || !templateOK || !keyOK || !nameOK || !descriptionOK || !sortOK || !enabledOK || !createdAtOK || !updatedAtOK {
		return CategoryResult{Disposition: DispositionQuarantine, Reason: "profile_segment_category_shape_invalid"}
	}
	return CategoryResult{Disposition: DispositionCandidate, Fact: &CategoryFact{
		SourceID: id, TemplateSourceID: templateID, CategoryKey: key, CategoryName: name, Description: description, SortOrder: sortOrder,
		OriginalEnabled: enabled, CreatedAt: createdAt, UpdatedAt: updatedAt,
	}}
}

func adaptOptionMapping(value json.RawMessage) OptionMappingResult {
	fields, ok := object(value)
	id, idOK := required[int64](fields, "id")
	templateID, templateOK := required[int64](fields, "template_id")
	categoryID, categoryOK := required[int64](fields, "category_id")
	questionID, questionOK := required[int64](fields, "question_id")
	optionID, optionOK := required[int64](fields, "option_id")
	createdAt, createdAtOK := required[time.Time](fields, "created_at")
	if !ok || !idOK || !templateOK || !categoryOK || !questionOK || !optionOK || !createdAtOK {
		return OptionMappingResult{Disposition: DispositionQuarantine, Reason: "profile_segment_option_mapping_shape_invalid"}
	}
	return OptionMappingResult{Disposition: DispositionCandidate, Fact: &OptionMappingFact{
		SourceID: id, TemplateSourceID: templateID, CategorySourceID: categoryID, QuestionSourceID: questionID, OptionSourceID: optionID, CreatedAt: createdAt,
	}}
}

func adaptSignupTagRule(value json.RawMessage) SignupTagRuleResult {
	fields, ok := object(value)
	tagID, tagIDOK := required[string](fields, "tag_id")
	tagName, tagNameOK := required[string](fields, "tag_name")
	status, statusOK := required[string](fields, "signup_status")
	active, activeOK := required[bool](fields, "active")
	updatedAt, updatedAtOK := required[time.Time](fields, "updated_at")
	if !ok || !tagIDOK || !tagNameOK || !statusOK || !activeOK || !updatedAtOK {
		return SignupTagRuleResult{Disposition: DispositionQuarantine, Reason: "signup_tag_rule_shape_invalid"}
	}
	return SignupTagRuleResult{Disposition: DispositionCandidate, Fact: &SignupTagRuleFact{TagSourceID: tagID, TagName: tagName, SignupStatus: status, OriginalActive: active, UpdatedAt: updatedAt}}
}

func uniqueTemplateIDs(values []TemplateResult) map[int64]struct{} {
	counts := make(map[int64]int)
	for _, value := range values {
		if value.Disposition == DispositionCandidate && value.Fact != nil {
			counts[value.Fact.SourceID]++
		}
	}
	for index := range values {
		if fact := values[index].Fact; fact != nil && counts[fact.SourceID] != 1 {
			quarantineTemplate(&values[index], "profile_segment_template_source_id_ambiguous")
		}
	}
	ids := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value.Disposition == DispositionCandidate && value.Fact != nil {
			ids[value.Fact.SourceID] = struct{}{}
		}
	}
	return ids
}

func uniqueCategoryIDs(values []CategoryResult) map[int64]CategoryFact {
	counts := make(map[int64]int)
	for _, value := range values {
		if value.Disposition == DispositionCandidate && value.Fact != nil {
			counts[value.Fact.SourceID]++
		}
	}
	for index := range values {
		if fact := values[index].Fact; fact != nil && counts[fact.SourceID] != 1 {
			quarantineCategory(&values[index], "profile_segment_category_source_id_ambiguous")
		}
	}
	ids := make(map[int64]CategoryFact, len(values))
	for _, value := range values {
		if value.Disposition == DispositionCandidate && value.Fact != nil {
			ids[value.Fact.SourceID] = *value.Fact
		}
	}
	return ids
}

func uniqueOptionMappingIDs(values []OptionMappingResult) {
	counts := make(map[int64]int)
	for _, value := range values {
		if value.Disposition == DispositionCandidate && value.Fact != nil {
			counts[value.Fact.SourceID]++
		}
	}
	for index := range values {
		if fact := values[index].Fact; fact != nil && counts[fact.SourceID] != 1 {
			quarantineOptionMapping(&values[index], "profile_segment_option_mapping_source_id_ambiguous")
		}
	}
}

func uniqueSignupTagRuleIDs(values []SignupTagRuleResult) {
	counts := make(map[string]int)
	for _, value := range values {
		if value.Disposition == DispositionCandidate && value.Fact != nil {
			counts[value.Fact.TagSourceID]++
		}
	}
	for index := range values {
		if fact := values[index].Fact; fact != nil && counts[fact.TagSourceID] != 1 {
			values[index] = SignupTagRuleResult{Disposition: DispositionQuarantine, Reason: "signup_tag_rule_source_id_ambiguous"}
		}
	}
}

func quarantineTemplate(value *TemplateResult, reason string) {
	*value = TemplateResult{Disposition: DispositionQuarantine, Reason: reason}
}

func quarantineCategory(value *CategoryResult, reason string) {
	*value = CategoryResult{Disposition: DispositionQuarantine, Reason: reason}
}

func quarantineOptionMapping(value *OptionMappingResult, reason string) {
	*value = OptionMappingResult{Disposition: DispositionQuarantine, Reason: reason}
}

func actorDigest(role, value string) ActorDigest {
	return ActorDigest(sha256.Sum256([]byte("v1-profile-catalog-actor-v1\x00" + role + "\x00" + value)))
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

func required[T any](source fields, name string) (T, bool) {
	var zero T
	raw, found := source[name]
	if !found || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &zero) != nil {
		return zero, false
	}
	return zero, true
}

func optional[T any](source fields, name string) (*T, bool) {
	raw, found := source[name]
	if !found {
		return nil, false
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, true
	}
	var value T
	if json.Unmarshal(raw, &value) != nil {
		return nil, false
	}
	return &value, true
}
