package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

// ProfileCatalogHistoryService writes inert V1 catalogue facts and their
// replay receipts in the caller transaction. It never changes current Segment
// templates, members, rules, or external effects.
type ProfileCatalogHistoryService struct {
	store   segmentport.ProfileCatalogHistoryStore
	journal segmentport.ProfileCatalogHistoryJournal
}

func NewProfileCatalogHistoryService(store segmentport.ProfileCatalogHistoryStore, journal segmentport.ProfileCatalogHistoryJournal) *ProfileCatalogHistoryService {
	return &ProfileCatalogHistoryService{store: store, journal: journal}
}

func (service *ProfileCatalogHistoryService) ready(ctx context.Context) bool {
	return service != nil && ctx != nil && ctx.Err() == nil && !nilCRUD(service.store) && !nilCRUD(service.journal)
}

func (service *ProfileCatalogHistoryService) ImportTemplate(ctx context.Context, sourceIdentifier string, payloadDigest [32]byte, value segmentport.HistoricalProfileTemplate) (segmentport.ProfileCatalogHistoryReceipt, error) {
	if !service.ready(ctx) {
		return segmentport.ProfileCatalogHistoryReceipt{}, segmentport.ErrProfileCatalogHistoryUnavailable
	}
	if value.ID != 0 || sourceIdentifier != hex.EncodeToString(value.SourceKeyDigest[:]) || payloadDigest == ([32]byte{}) || payloadDigest != value.SourcePayloadDigest {
		return segmentport.ProfileCatalogHistoryReceipt{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	value = normalizeProfileTemplate(value)
	return writeProfileCatalogHistory(ctx, service.journal, "templates", sourceIdentifier, payloadDigest, value,
		func(value segmentport.HistoricalProfileTemplate) int64 { return value.ID },
		func(value segmentport.HistoricalProfileTemplate, id int64) segmentport.HistoricalProfileTemplate {
			value.ID = id
			return normalizeProfileTemplate(value)
		},
		HistoricalProfileTemplateDigest, service.store.CreateHistoricalProfileTemplate, service.store.GetHistoricalProfileTemplate)
}

func (service *ProfileCatalogHistoryService) ImportCategory(ctx context.Context, sourceIdentifier string, payloadDigest [32]byte, value segmentport.HistoricalProfileCategory) (segmentport.ProfileCatalogHistoryReceipt, error) {
	if !service.ready(ctx) {
		return segmentport.ProfileCatalogHistoryReceipt{}, segmentport.ErrProfileCatalogHistoryUnavailable
	}
	if value.ID != 0 || sourceIdentifier != hex.EncodeToString(value.SourceKeyDigest[:]) || payloadDigest == ([32]byte{}) || payloadDigest != value.SourcePayloadDigest {
		return segmentport.ProfileCatalogHistoryReceipt{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	value = normalizeProfileCategory(value)
	parent, err := service.store.GetHistoricalProfileTemplate(ctx, value.TemplateHistoryID)
	if err != nil {
		return segmentport.ProfileCatalogHistoryReceipt{}, profileCatalogHistoryError(err)
	}
	if parent.ID != value.TemplateHistoryID || parent.SourceID != value.TemplateSourceID {
		return segmentport.ProfileCatalogHistoryReceipt{}, segmentport.ErrProfileCatalogHistoryConflict
	}
	return writeProfileCatalogHistory(ctx, service.journal, "categories", sourceIdentifier, payloadDigest, value,
		func(value segmentport.HistoricalProfileCategory) int64 { return value.ID },
		func(value segmentport.HistoricalProfileCategory, id int64) segmentport.HistoricalProfileCategory {
			value.ID = id
			return normalizeProfileCategory(value)
		},
		HistoricalProfileCategoryDigest, service.store.CreateHistoricalProfileCategory, service.store.GetHistoricalProfileCategory)
}

func (service *ProfileCatalogHistoryService) ImportOptionMapping(ctx context.Context, sourceIdentifier string, payloadDigest [32]byte, value segmentport.HistoricalProfileOptionMapping) (segmentport.ProfileCatalogHistoryReceipt, error) {
	if !service.ready(ctx) {
		return segmentport.ProfileCatalogHistoryReceipt{}, segmentport.ErrProfileCatalogHistoryUnavailable
	}
	if value.ID != 0 || sourceIdentifier != hex.EncodeToString(value.SourceKeyDigest[:]) || payloadDigest == ([32]byte{}) || payloadDigest != value.SourcePayloadDigest {
		return segmentport.ProfileCatalogHistoryReceipt{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	value = normalizeProfileOptionMapping(value)
	template, err := service.store.GetHistoricalProfileTemplate(ctx, value.TemplateHistoryID)
	if err != nil {
		return segmentport.ProfileCatalogHistoryReceipt{}, profileCatalogHistoryError(err)
	}
	category, err := service.store.GetHistoricalProfileCategory(ctx, value.CategoryHistoryID)
	if err != nil {
		return segmentport.ProfileCatalogHistoryReceipt{}, profileCatalogHistoryError(err)
	}
	if template.ID != value.TemplateHistoryID || template.SourceID != value.TemplateSourceID || category.ID != value.CategoryHistoryID || category.TemplateHistoryID != value.TemplateHistoryID || category.TemplateSourceID != value.TemplateSourceID || category.SourceID != value.CategorySourceID {
		return segmentport.ProfileCatalogHistoryReceipt{}, segmentport.ErrProfileCatalogHistoryConflict
	}
	return writeProfileCatalogHistory(ctx, service.journal, "option_mappings", sourceIdentifier, payloadDigest, value,
		func(value segmentport.HistoricalProfileOptionMapping) int64 { return value.ID },
		func(value segmentport.HistoricalProfileOptionMapping, id int64) segmentport.HistoricalProfileOptionMapping {
			value.ID = id
			return normalizeProfileOptionMapping(value)
		},
		HistoricalProfileOptionMappingDigest, service.store.CreateHistoricalProfileOptionMapping, service.store.GetHistoricalProfileOptionMapping)
}

func HistoricalProfileTemplateDigest(value segmentport.HistoricalProfileTemplate) ([32]byte, error) {
	if value.ID < 1 || value.SourceKeyDigest == ([32]byte{}) || value.SourcePayloadDigest == ([32]byte{}) || value.CreatedByDigest == ([32]byte{}) || value.UpdatedByDigest == ([32]byte{}) || value.CreatedAt.IsZero() || value.UpdatedAt.IsZero() {
		return [32]byte{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	return profileCatalogDigest("templates", normalizeProfileTemplate(value))
}

func HistoricalProfileCategoryDigest(value segmentport.HistoricalProfileCategory) ([32]byte, error) {
	if value.ID < 1 || value.SourceKeyDigest == ([32]byte{}) || value.SourcePayloadDigest == ([32]byte{}) || value.TemplateHistoryID < 1 || value.CreatedAt.IsZero() || value.UpdatedAt.IsZero() {
		return [32]byte{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	return profileCatalogDigest("categories", normalizeProfileCategory(value))
}

func HistoricalProfileOptionMappingDigest(value segmentport.HistoricalProfileOptionMapping) ([32]byte, error) {
	if value.ID < 1 || value.SourceKeyDigest == ([32]byte{}) || value.SourcePayloadDigest == ([32]byte{}) || value.TemplateHistoryID < 1 || value.CategoryHistoryID < 1 || value.CreatedAt.IsZero() {
		return [32]byte{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	return profileCatalogDigest("option_mappings", normalizeProfileOptionMapping(value))
}

func normalizeProfileTemplate(value segmentport.HistoricalProfileTemplate) segmentport.HistoricalProfileTemplate {
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
	value.QuestionnaireSourceID = profileCatalogInt(value.QuestionnaireSourceID)
	value.SegmentationQuestionSourceID = profileCatalogInt(value.SegmentationQuestionSourceID)
	value.ProgramSourceID = profileCatalogInt(value.ProgramSourceID)
	return value
}
func normalizeProfileCategory(value segmentport.HistoricalProfileCategory) segmentport.HistoricalProfileCategory {
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
	return value
}
func normalizeProfileOptionMapping(value segmentport.HistoricalProfileOptionMapping) segmentport.HistoricalProfileOptionMapping {
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	return value
}
func profileCatalogInt(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func writeProfileCatalogHistory[T any](ctx context.Context, journal segmentport.ProfileCatalogHistoryJournal, kind, source string, payload [32]byte, value T, id func(T) int64, withID func(T, int64) T, digest func(T) ([32]byte, error), create func(context.Context, T) (T, error), get func(context.Context, int64) (T, error)) (segmentport.ProfileCatalogHistoryReceipt, error) {
	empty := segmentport.ProfileCatalogHistoryReceipt{}
	if source == "" || payload == ([32]byte{}) {
		return empty, segmentport.ErrProfileCatalogHistoryInvalid
	}
	if _, err := digest(withID(value, 1)); err != nil {
		return empty, segmentport.ErrProfileCatalogHistoryInvalid
	}
	receipt, found, err := journal.LoadProfileCatalogHistory(ctx, kind, source)
	if err != nil {
		return empty, profileCatalogHistoryError(err)
	}
	if found {
		if receipt.Kind != kind || receipt.SourceIdentifier != source || receipt.PayloadDigest != payload || receipt.TargetID < 1 || receipt.TargetDigest == ([32]byte{}) {
			return empty, segmentport.ErrProfileCatalogHistoryConflict
		}
		actual, err := get(ctx, receipt.TargetID)
		if err != nil {
			return empty, profileCatalogHistoryError(err)
		}
		actualDigest, err := digest(actual)
		expected, expectedErr := digest(withID(value, receipt.TargetID))
		if err != nil || expectedErr != nil || id(actual) != receipt.TargetID || actualDigest != expected || actualDigest != receipt.TargetDigest {
			return empty, segmentport.ErrProfileCatalogHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := create(ctx, withID(value, 0))
	if err != nil {
		return empty, profileCatalogHistoryError(err)
	}
	actualDigest, err := digest(actual)
	expected, expectedErr := digest(withID(value, id(actual)))
	if err != nil || expectedErr != nil || actualDigest != expected {
		return empty, segmentport.ErrProfileCatalogHistoryConflict
	}
	receipt = segmentport.ProfileCatalogHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetDigest: actualDigest, TargetID: id(actual)}
	if err := journal.RecordProfileCatalogHistory(ctx, receipt); err != nil {
		return empty, profileCatalogHistoryError(err)
	}
	return receipt, nil
}

func profileCatalogDigest(kind string, value any) ([32]byte, error) {
	encoded, err := json.Marshal(struct {
		Kind  string
		Value any
	}{Kind: kind, Value: value})
	if err != nil {
		return [32]byte{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}
func profileCatalogHistoryError(err error) error {
	switch {
	case errors.Is(err, segmentport.ErrProfileCatalogHistoryInvalid):
		return segmentport.ErrProfileCatalogHistoryInvalid
	case errors.Is(err, segmentport.ErrProfileCatalogHistoryConflict):
		return segmentport.ErrProfileCatalogHistoryConflict
	default:
		return segmentport.ErrProfileCatalogHistoryUnavailable
	}
}
