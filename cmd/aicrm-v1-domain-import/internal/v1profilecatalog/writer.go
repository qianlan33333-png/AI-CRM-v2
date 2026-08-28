package v1profilecatalog

import (
	"context"
	"encoding/hex"
	"reflect"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

const (
	ProfileTemplatesTableID      = "public/automation_profile_segment_template"
	ProfileCategoriesTableID     = "public/automation_profile_segment_category"
	ProfileOptionMappingsTableID = "public/automation_profile_segment_option_mapping"
	SignupTagRulesTableID        = "public/signup_tag_rules"

	ProfileTemplatesKind      = "templates"
	ProfileCategoriesKind     = "categories"
	ProfileOptionMappingsKind = "option_mappings"
	SignupTagRulesKind        = "signup_tag_rule"

	ProfileTemplatesTargetTable      = "segment_v1_profile_templates"
	ProfileCategoriesTargetTable     = "segment_v1_profile_categories"
	ProfileOptionMappingsTargetTable = "segment_v1_profile_option_mappings"
	SignupTagRulesTargetTable        = "contact_v1_signup_tag_rules"
)

// SourceBinding is the already-verified archive identity for one row. The
// importer owns ordinal, receipt, and archive-run handling; this adapter only
// rejects a mismatch before it can enter an owner service.
type SourceBinding struct {
	TableID, SourceIdentifier string
	SourceKeyDigest           [32]byte
	SourcePayloadDigest       [32]byte
	FieldDigest               [32]byte
	Redacted                  bool
}

// ProfileCatalogHistoryService is satisfied by segment/app's immutable
// history writer. It deliberately accepts the caller's transaction context.
type ProfileCatalogHistoryService interface {
	ImportTemplate(context.Context, string, [32]byte, segmentport.HistoricalProfileTemplate) (segmentport.ProfileCatalogHistoryReceipt, error)
	ImportCategory(context.Context, string, [32]byte, segmentport.HistoricalProfileCategory) (segmentport.ProfileCatalogHistoryReceipt, error)
	ImportOptionMapping(context.Context, string, [32]byte, segmentport.HistoricalProfileOptionMapping) (segmentport.ProfileCatalogHistoryReceipt, error)
}

// SignupTagHistoryService is satisfied by contact/app's immutable history
// writer. It has no current tag-catalogue or Provider operation.
type SignupTagHistoryService interface {
	ImportRule(context.Context, string, [32]byte, contactport.HistoricalSignupTagRule) (contactport.SignupTagHistoryReceipt, error)
}

// Writer maps typed, non-executable source facts to the owner-owned history
// services. It never starts a transaction itself.
type Writer struct {
	profiles ProfileCatalogHistoryService
	tags     SignupTagHistoryService
}

func NewWriter(profiles ProfileCatalogHistoryService, tags SignupTagHistoryService) (*Writer, error) {
	if nilDependency(profiles) || nilDependency(tags) {
		return nil, segmentport.ErrProfileCatalogHistoryUnavailable
	}
	return &Writer{profiles: profiles, tags: tags}, nil
}

func (writer *Writer) ApplyTemplate(ctx context.Context, binding SourceBinding, fact TemplateFact) (segmentport.ProfileCatalogHistoryReceipt, error) {
	if writer == nil || nilDependency(writer.profiles) || !validBinding(binding, ProfileTemplatesTableID) {
		return segmentport.ProfileCatalogHistoryReceipt{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	value := segmentport.HistoricalProfileTemplate{
		SourceID: fact.SourceID, SourceKeyDigest: binding.SourceKeyDigest, SourcePayloadDigest: binding.SourcePayloadDigest,
		TemplateCode: fact.TemplateCode, TemplateName: fact.TemplateName, QuestionnaireSourceID: copyInt64(fact.QuestionnaireSourceID),
		SegmentationQuestionSourceID: copyInt64(fact.SegmentationQuestionSourceID), ProgramSourceID: copyInt64(fact.ProgramSourceID),
		Description: fact.Description, OriginalEnabled: fact.OriginalEnabled, Version: fact.Version,
		CreatedByDigest: [32]byte(fact.CreatedByDigest), UpdatedByDigest: [32]byte(fact.UpdatedByDigest),
		CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt,
	}
	receipt, err := writer.profiles.ImportTemplate(ctx, binding.SourceIdentifier, binding.SourcePayloadDigest, value)
	if err != nil || !validProfileReceipt(receipt, ProfileTemplatesKind, binding) {
		if err != nil {
			return segmentport.ProfileCatalogHistoryReceipt{}, err
		}
		return segmentport.ProfileCatalogHistoryReceipt{}, segmentport.ErrProfileCatalogHistoryConflict
	}
	return receipt, nil
}

func (writer *Writer) ApplyCategory(ctx context.Context, binding SourceBinding, fact CategoryFact, parent segmentport.HistoricalProfileTemplate) (segmentport.ProfileCatalogHistoryReceipt, error) {
	if writer == nil || nilDependency(writer.profiles) || !validBinding(binding, ProfileCategoriesTableID) {
		return segmentport.ProfileCatalogHistoryReceipt{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	if parent.ID < 1 || parent.SourceID != fact.TemplateSourceID {
		return segmentport.ProfileCatalogHistoryReceipt{}, segmentport.ErrProfileCatalogHistoryConflict
	}
	value := segmentport.HistoricalProfileCategory{
		SourceID: fact.SourceID, SourceKeyDigest: binding.SourceKeyDigest, SourcePayloadDigest: binding.SourcePayloadDigest,
		TemplateSourceID: fact.TemplateSourceID, TemplateHistoryID: parent.ID, CategoryKey: fact.CategoryKey, CategoryName: fact.CategoryName,
		Description: fact.Description, SortOrder: fact.SortOrder, OriginalEnabled: fact.OriginalEnabled, CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt,
	}
	receipt, err := writer.profiles.ImportCategory(ctx, binding.SourceIdentifier, binding.SourcePayloadDigest, value)
	if err != nil || !validProfileReceipt(receipt, ProfileCategoriesKind, binding) {
		if err != nil {
			return segmentport.ProfileCatalogHistoryReceipt{}, err
		}
		return segmentport.ProfileCatalogHistoryReceipt{}, segmentport.ErrProfileCatalogHistoryConflict
	}
	return receipt, nil
}

func (writer *Writer) ApplyOptionMapping(ctx context.Context, binding SourceBinding, fact OptionMappingFact, template segmentport.HistoricalProfileTemplate, category segmentport.HistoricalProfileCategory) (segmentport.ProfileCatalogHistoryReceipt, error) {
	if writer == nil || nilDependency(writer.profiles) || !validBinding(binding, ProfileOptionMappingsTableID) {
		return segmentport.ProfileCatalogHistoryReceipt{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	if template.ID < 1 || category.ID < 1 || template.SourceID != fact.TemplateSourceID || category.SourceID != fact.CategorySourceID || category.TemplateHistoryID != template.ID || category.TemplateSourceID != fact.TemplateSourceID {
		return segmentport.ProfileCatalogHistoryReceipt{}, segmentport.ErrProfileCatalogHistoryConflict
	}
	value := segmentport.HistoricalProfileOptionMapping{
		SourceID: fact.SourceID, SourceKeyDigest: binding.SourceKeyDigest, SourcePayloadDigest: binding.SourcePayloadDigest,
		TemplateSourceID: fact.TemplateSourceID, CategorySourceID: fact.CategorySourceID, TemplateHistoryID: template.ID, CategoryHistoryID: category.ID,
		QuestionSourceID: fact.QuestionSourceID, OptionSourceID: fact.OptionSourceID, CreatedAt: fact.CreatedAt,
	}
	receipt, err := writer.profiles.ImportOptionMapping(ctx, binding.SourceIdentifier, binding.SourcePayloadDigest, value)
	if err != nil || !validProfileReceipt(receipt, ProfileOptionMappingsKind, binding) {
		if err != nil {
			return segmentport.ProfileCatalogHistoryReceipt{}, err
		}
		return segmentport.ProfileCatalogHistoryReceipt{}, segmentport.ErrProfileCatalogHistoryConflict
	}
	return receipt, nil
}

func (writer *Writer) ApplySignupTagRule(ctx context.Context, binding SourceBinding, fact SignupTagRuleFact) (contactport.SignupTagHistoryReceipt, error) {
	if writer == nil || nilDependency(writer.tags) || !validBinding(binding, SignupTagRulesTableID) {
		return contactport.SignupTagHistoryReceipt{}, contactport.ErrSignupTagHistoryInvalid
	}
	value := contactport.HistoricalSignupTagRule{
		SourceKeyDigest: binding.SourceKeyDigest, SourcePayloadDigest: binding.SourcePayloadDigest, TagSourceID: fact.TagSourceID,
		TagName: fact.TagName, SignupStatus: fact.SignupStatus, OriginalActive: fact.OriginalActive, UpdatedAt: fact.UpdatedAt,
	}
	receipt, err := writer.tags.ImportRule(ctx, binding.SourceIdentifier, binding.SourcePayloadDigest, value)
	if err != nil || !validSignupReceipt(receipt, binding) {
		if err != nil {
			return contactport.SignupTagHistoryReceipt{}, err
		}
		return contactport.SignupTagHistoryReceipt{}, contactport.ErrSignupTagHistoryConflict
	}
	return receipt, nil
}

// TargetReader is the small read-back opening used by central replay and
// reconciliation. It does not create a transaction or substitute parents.
type TargetReader struct {
	profiles segmentport.ProfileCatalogHistoryReader
	tags     contactport.SignupTagHistoryReader
}

func NewTargetReader(profiles segmentport.ProfileCatalogHistoryReader, tags contactport.SignupTagHistoryReader) (*TargetReader, error) {
	if nilDependency(profiles) || nilDependency(tags) {
		return nil, segmentport.ErrProfileCatalogHistoryUnavailable
	}
	return &TargetReader{profiles: profiles, tags: tags}, nil
}

func (reader *TargetReader) ReadTemplate(ctx context.Context, id int64) (segmentport.HistoricalProfileTemplate, error) {
	if reader == nil || nilDependency(reader.profiles) || id < 1 {
		return segmentport.HistoricalProfileTemplate{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	return reader.profiles.GetHistoricalProfileTemplate(ctx, id)
}

func (reader *TargetReader) ReadCategory(ctx context.Context, id int64) (segmentport.HistoricalProfileCategory, error) {
	if reader == nil || nilDependency(reader.profiles) || id < 1 {
		return segmentport.HistoricalProfileCategory{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	return reader.profiles.GetHistoricalProfileCategory(ctx, id)
}

func (reader *TargetReader) ReadOptionMapping(ctx context.Context, id int64) (segmentport.HistoricalProfileOptionMapping, error) {
	if reader == nil || nilDependency(reader.profiles) || id < 1 {
		return segmentport.HistoricalProfileOptionMapping{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	return reader.profiles.GetHistoricalProfileOptionMapping(ctx, id)
}

func (reader *TargetReader) ReadSignupTagRule(ctx context.Context, id int64) (contactport.HistoricalSignupTagRule, error) {
	if reader == nil || nilDependency(reader.tags) || id < 1 {
		return contactport.HistoricalSignupTagRule{}, contactport.ErrSignupTagHistoryInvalid
	}
	return reader.tags.GetHistoricalSignupTagRule(ctx, id)
}

func validBinding(value SourceBinding, tableID string) bool {
	return value.TableID == tableID && !value.Redacted && value.SourceKeyDigest != ([32]byte{}) && value.SourcePayloadDigest != ([32]byte{}) && value.FieldDigest != ([32]byte{}) && value.SourceIdentifier == hex.EncodeToString(value.SourceKeyDigest[:])
}

func validProfileReceipt(value segmentport.ProfileCatalogHistoryReceipt, kind string, binding SourceBinding) bool {
	return value.Kind == kind && value.SourceIdentifier == binding.SourceIdentifier && value.PayloadDigest == binding.SourcePayloadDigest && value.TargetID > 0 && value.TargetDigest != ([32]byte{})
}

func validSignupReceipt(value contactport.SignupTagHistoryReceipt, binding SourceBinding) bool {
	return value.SourceIdentifier == binding.SourceIdentifier && value.PayloadDigest == binding.SourcePayloadDigest && value.TargetID > 0 && value.TargetDigest != ([32]byte{})
}

func copyInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func nilDependency(value any) bool {
	if value == nil {
		return true
	}
	raw := reflect.ValueOf(value)
	return raw.Kind() == reflect.Ptr && raw.IsNil()
}
