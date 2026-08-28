package v1profilecatalog

import (
	"context"
	"errors"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

type profileWriterContextKey struct{}

func TestWriterAppliesFactsWithVerifiedBindingsAndCallerContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), profileWriterContextKey{}, "caller-tx")
	profiles := &profileWriterFake{ctx: ctx}
	tags := &signupWriterFake{ctx: ctx}
	writer, err := NewWriter(profiles, tags)
	if err != nil {
		t.Fatal(err)
	}
	templateFact, categoryFact, mappingFact, ruleFact := writerFacts()
	templateBinding := writerBinding(ProfileTemplatesTableID, 1)
	templateReceipt, err := writer.ApplyTemplate(ctx, templateBinding, templateFact)
	if err != nil || templateReceipt.TargetID != 11 || profiles.template.SourcePayloadDigest != templateBinding.SourcePayloadDigest || profiles.template.QuestionnaireSourceID == templateFact.QuestionnaireSourceID {
		t.Fatalf("template was not copied through the caller transaction: receipt=%+v err=%v value=%+v", templateReceipt, err, profiles.template)
	}
	if profiles.template.CreatedByDigest != [32]byte(templateFact.CreatedByDigest) || !profiles.template.CreatedAt.Equal(templateFact.CreatedAt) {
		t.Fatalf("template fields changed: %+v", profiles.template)
	}

	parent := segmentport.HistoricalProfileTemplate{ID: templateReceipt.TargetID, SourceID: templateFact.SourceID}
	categoryReceipt, err := writer.ApplyCategory(ctx, writerBinding(ProfileCategoriesTableID, 2), categoryFact, parent)
	if err != nil || categoryReceipt.TargetID != 12 || profiles.category.TemplateHistoryID != parent.ID {
		t.Fatalf("category parent was not passed: receipt=%+v err=%v value=%+v", categoryReceipt, err, profiles.category)
	}
	category := segmentport.HistoricalProfileCategory{ID: categoryReceipt.TargetID, SourceID: categoryFact.SourceID, TemplateSourceID: templateFact.SourceID, TemplateHistoryID: parent.ID}
	if _, err := writer.ApplyOptionMapping(ctx, writerBinding(ProfileOptionMappingsTableID, 3), mappingFact, parent, category); err != nil {
		t.Fatalf("mapping was not applied: %v", err)
	}
	if profiles.mapping.TemplateHistoryID != parent.ID || profiles.mapping.CategoryHistoryID != category.ID || profiles.mapping.OptionSourceID != mappingFact.OptionSourceID {
		t.Fatalf("mapping fields changed: %+v", profiles.mapping)
	}
	if _, err := writer.ApplySignupTagRule(ctx, writerBinding(SignupTagRulesTableID, 4), ruleFact); err != nil {
		t.Fatalf("signup tag rule was not applied: %v", err)
	}
	if tags.rule.TagName != ruleFact.TagName || tags.rule.SourceKeyDigest == ([32]byte{}) {
		t.Fatalf("signup tag fields changed: %+v", tags.rule)
	}
	if profiles.calls != 3 || tags.calls != 1 {
		t.Fatalf("unexpected owner calls: profiles=%d tags=%d", profiles.calls, tags.calls)
	}
}

func TestWriterRejectsRedactionParentMismatchAndReceiptMismatch(t *testing.T) {
	ctx := context.WithValue(context.Background(), profileWriterContextKey{}, "caller-tx")
	profiles := &profileWriterFake{ctx: ctx}
	tags := &signupWriterFake{ctx: ctx}
	writer, err := NewWriter(profiles, tags)
	if err != nil {
		t.Fatal(err)
	}
	template, category, mapping, rule := writerFacts()
	redacted := writerBinding(ProfileTemplatesTableID, 1)
	redacted.Redacted = true
	if _, err := writer.ApplyTemplate(ctx, redacted, template); !errors.Is(err, segmentport.ErrProfileCatalogHistoryInvalid) || profiles.calls != 0 {
		t.Fatalf("redacted template reached owner: %v calls=%d", err, profiles.calls)
	}
	wrongTemplate := segmentport.HistoricalProfileTemplate{ID: 11, SourceID: template.SourceID + 1}
	if _, err := writer.ApplyCategory(ctx, writerBinding(ProfileCategoriesTableID, 2), category, wrongTemplate); !errors.Is(err, segmentport.ErrProfileCatalogHistoryConflict) || profiles.calls != 0 {
		t.Fatalf("wrong category parent was accepted: %v calls=%d", err, profiles.calls)
	}
	rightTemplate := segmentport.HistoricalProfileTemplate{ID: 11, SourceID: template.SourceID}
	wrongCategory := segmentport.HistoricalProfileCategory{ID: 12, SourceID: category.SourceID, TemplateSourceID: template.SourceID + 1, TemplateHistoryID: 11}
	if _, err := writer.ApplyOptionMapping(ctx, writerBinding(ProfileOptionMappingsTableID, 3), mapping, rightTemplate, wrongCategory); !errors.Is(err, segmentport.ErrProfileCatalogHistoryConflict) || profiles.calls != 0 {
		t.Fatalf("cross-template mapping was accepted: %v calls=%d", err, profiles.calls)
	}
	profiles.templateReceipt = segmentport.ProfileCatalogHistoryReceipt{Kind: "wrong", TargetID: 11, TargetDigest: digest(8)}
	if _, err := writer.ApplyTemplate(ctx, writerBinding(ProfileTemplatesTableID, 1), template); !errors.Is(err, segmentport.ErrProfileCatalogHistoryConflict) {
		t.Fatalf("invalid owner receipt was accepted: %v", err)
	}
	tags.receipt = contactport.SignupTagHistoryReceipt{TargetID: 14, TargetDigest: digest(9)}
	if _, err := writer.ApplySignupTagRule(ctx, writerBinding(SignupTagRulesTableID, 4), rule); !errors.Is(err, contactport.ErrSignupTagHistoryConflict) {
		t.Fatalf("invalid signup receipt was accepted: %v", err)
	}
}

func TestWriterPropagatesOwnerFailureWithoutStartingAnotherTransaction(t *testing.T) {
	ctx := context.WithValue(context.Background(), profileWriterContextKey{}, "caller-tx")
	profiles := &profileWriterFake{ctx: ctx, templateErr: segmentport.ErrProfileCatalogHistoryUnavailable}
	writer, err := NewWriter(profiles, &signupWriterFake{ctx: ctx})
	if err != nil {
		t.Fatal(err)
	}
	template, _, _, _ := writerFacts()
	if _, err := writer.ApplyTemplate(ctx, writerBinding(ProfileTemplatesTableID, 1), template); !errors.Is(err, segmentport.ErrProfileCatalogHistoryUnavailable) || profiles.calls != 1 {
		t.Fatalf("owner failure was not returned unchanged: %v calls=%d", err, profiles.calls)
	}
}

func TestTargetReaderReadsTypedTargetsAndRejectsMissingDependencies(t *testing.T) {
	ctx := context.Background()
	profiles := &profileTargetReaderFake{template: segmentport.HistoricalProfileTemplate{ID: 11}, category: segmentport.HistoricalProfileCategory{ID: 12}, mapping: segmentport.HistoricalProfileOptionMapping{ID: 13}}
	tags := &signupTargetReaderFake{rule: contactport.HistoricalSignupTagRule{ID: 14}}
	reader, err := NewTargetReader(profiles, tags)
	if err != nil {
		t.Fatal(err)
	}
	if value, err := reader.ReadTemplate(ctx, 11); err != nil || value.ID != 11 {
		t.Fatalf("template read=%+v err=%v", value, err)
	}
	if value, err := reader.ReadCategory(ctx, 12); err != nil || value.ID != 12 {
		t.Fatalf("category read=%+v err=%v", value, err)
	}
	if value, err := reader.ReadOptionMapping(ctx, 13); err != nil || value.ID != 13 {
		t.Fatalf("mapping read=%+v err=%v", value, err)
	}
	if value, err := reader.ReadSignupTagRule(ctx, 14); err != nil || value.ID != 14 {
		t.Fatalf("rule read=%+v err=%v", value, err)
	}
	if _, err := reader.ReadTemplate(ctx, 0); !errors.Is(err, segmentport.ErrProfileCatalogHistoryInvalid) {
		t.Fatalf("zero target id was accepted: %v", err)
	}
	if _, err := NewTargetReader(nil, tags); !errors.Is(err, segmentport.ErrProfileCatalogHistoryUnavailable) {
		t.Fatalf("nil profile reader was accepted: %v", err)
	}
}

func writerFacts() (TemplateFact, CategoryFact, OptionMappingFact, SignupTagRuleFact) {
	stamp := time.Date(2026, 8, 28, 9, 30, 0, 123456789, time.FixedZone("source", 8*60*60))
	questionnaire, program := int64(901), int64(88)
	template := TemplateFact{SourceID: -7, TemplateCode: "legacy", TemplateName: "历史", QuestionnaireSourceID: &questionnaire, ProgramSourceID: &program, Description: "原说明", OriginalEnabled: false, Version: -3, CreatedByDigest: ActorDigest(digest(1)), UpdatedByDigest: ActorDigest(digest(2)), CreatedAt: stamp, UpdatedAt: stamp}
	category := CategoryFact{SourceID: 10, TemplateSourceID: -7, CategoryKey: "journey", CategoryName: "旅程", Description: "分类", SortOrder: -2, OriginalEnabled: false, CreatedAt: stamp, UpdatedAt: stamp}
	mapping := OptionMappingFact{SourceID: 100, TemplateSourceID: -7, CategorySourceID: 10, QuestionSourceID: 777, OptionSourceID: 778, CreatedAt: stamp}
	rule := SignupTagRuleFact{TagSourceID: "tag-v1", TagName: "报名", SignupStatus: "approved", OriginalActive: false, UpdatedAt: stamp}
	return template, category, mapping, rule
}

func writerBinding(table string, n byte) SourceBinding {
	return SourceBinding{TableID: table, SourceKeyDigest: digest(n), SourcePayloadDigest: digest(n + 20), FieldDigest: digest(n + 40), SourceIdentifier: hexDigest(digest(n))}
}

func digest(n byte) [32]byte {
	var value [32]byte
	value[0] = n
	return value
}

func hexDigest(value [32]byte) string {
	const alphabet = "0123456789abcdef"
	result := make([]byte, len(value)*2)
	for index, item := range value {
		result[index*2], result[index*2+1] = alphabet[item>>4], alphabet[item&15]
	}
	return string(result)
}

type profileWriterFake struct {
	ctx                                              context.Context
	templateErr                                      error
	templateReceipt, categoryReceipt, mappingReceipt segmentport.ProfileCatalogHistoryReceipt
	template                                         segmentport.HistoricalProfileTemplate
	category                                         segmentport.HistoricalProfileCategory
	mapping                                          segmentport.HistoricalProfileOptionMapping
	calls                                            int
}

func (fake *profileWriterFake) ImportTemplate(ctx context.Context, source string, payload [32]byte, value segmentport.HistoricalProfileTemplate) (segmentport.ProfileCatalogHistoryReceipt, error) {
	fake.calls++
	if ctx != fake.ctx {
		return segmentport.ProfileCatalogHistoryReceipt{}, errors.New("wrong caller transaction")
	}
	if fake.templateErr != nil {
		return segmentport.ProfileCatalogHistoryReceipt{}, fake.templateErr
	}
	fake.template = value
	if fake.templateReceipt.TargetID != 0 {
		return fake.templateReceipt, nil
	}
	return segmentport.ProfileCatalogHistoryReceipt{Kind: ProfileTemplatesKind, SourceIdentifier: source, PayloadDigest: payload, TargetDigest: digest(11), TargetID: 11}, nil
}

func (fake *profileWriterFake) ImportCategory(ctx context.Context, source string, payload [32]byte, value segmentport.HistoricalProfileCategory) (segmentport.ProfileCatalogHistoryReceipt, error) {
	fake.calls++
	if ctx != fake.ctx {
		return segmentport.ProfileCatalogHistoryReceipt{}, errors.New("wrong caller transaction")
	}
	fake.category = value
	if fake.categoryReceipt.TargetID != 0 {
		return fake.categoryReceipt, nil
	}
	return segmentport.ProfileCatalogHistoryReceipt{Kind: ProfileCategoriesKind, SourceIdentifier: source, PayloadDigest: payload, TargetDigest: digest(12), TargetID: 12}, nil
}

func (fake *profileWriterFake) ImportOptionMapping(ctx context.Context, source string, payload [32]byte, value segmentport.HistoricalProfileOptionMapping) (segmentport.ProfileCatalogHistoryReceipt, error) {
	fake.calls++
	if ctx != fake.ctx {
		return segmentport.ProfileCatalogHistoryReceipt{}, errors.New("wrong caller transaction")
	}
	fake.mapping = value
	if fake.mappingReceipt.TargetID != 0 {
		return fake.mappingReceipt, nil
	}
	return segmentport.ProfileCatalogHistoryReceipt{Kind: ProfileOptionMappingsKind, SourceIdentifier: source, PayloadDigest: payload, TargetDigest: digest(13), TargetID: 13}, nil
}

type signupWriterFake struct {
	ctx     context.Context
	receipt contactport.SignupTagHistoryReceipt
	rule    contactport.HistoricalSignupTagRule
	calls   int
}

func (fake *signupWriterFake) ImportRule(ctx context.Context, source string, payload [32]byte, value contactport.HistoricalSignupTagRule) (contactport.SignupTagHistoryReceipt, error) {
	fake.calls++
	if ctx != fake.ctx {
		return contactport.SignupTagHistoryReceipt{}, errors.New("wrong caller transaction")
	}
	fake.rule = value
	if fake.receipt.TargetID != 0 {
		return fake.receipt, nil
	}
	return contactport.SignupTagHistoryReceipt{SourceIdentifier: source, PayloadDigest: payload, TargetDigest: digest(14), TargetID: 14}, nil
}

type profileTargetReaderFake struct {
	template segmentport.HistoricalProfileTemplate
	category segmentport.HistoricalProfileCategory
	mapping  segmentport.HistoricalProfileOptionMapping
}

func (fake *profileTargetReaderFake) GetHistoricalProfileTemplate(_ context.Context, _ int64) (segmentport.HistoricalProfileTemplate, error) {
	return fake.template, nil
}
func (fake *profileTargetReaderFake) ListHistoricalProfileTemplates(context.Context, segmentport.ProfileCatalogHistoryQuery) ([]segmentport.HistoricalProfileTemplate, int64, error) {
	return nil, 0, nil
}
func (fake *profileTargetReaderFake) GetHistoricalProfileCategory(_ context.Context, _ int64) (segmentport.HistoricalProfileCategory, error) {
	return fake.category, nil
}
func (fake *profileTargetReaderFake) ListHistoricalProfileCategories(context.Context, segmentport.ProfileCatalogHistoryQuery) ([]segmentport.HistoricalProfileCategory, int64, error) {
	return nil, 0, nil
}
func (fake *profileTargetReaderFake) GetHistoricalProfileOptionMapping(_ context.Context, _ int64) (segmentport.HistoricalProfileOptionMapping, error) {
	return fake.mapping, nil
}
func (fake *profileTargetReaderFake) ListHistoricalProfileOptionMappings(context.Context, segmentport.ProfileCatalogHistoryQuery) ([]segmentport.HistoricalProfileOptionMapping, int64, error) {
	return nil, 0, nil
}

type signupTargetReaderFake struct {
	rule contactport.HistoricalSignupTagRule
}

func (fake *signupTargetReaderFake) GetHistoricalSignupTagRule(_ context.Context, _ int64) (contactport.HistoricalSignupTagRule, error) {
	return fake.rule, nil
}
func (fake *signupTargetReaderFake) ListHistoricalSignupTagRules(context.Context, int32, int32) ([]contactport.HistoricalSignupTagRule, int64, error) {
	return nil, 0, nil
}
