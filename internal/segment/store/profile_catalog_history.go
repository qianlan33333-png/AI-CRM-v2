package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentdb "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store/generated"
)

type ProfileCatalogHistoryStore struct {
	tx func(context.Context) (pgx.Tx, error)
}
type ProfileCatalogHistoryReader struct{ db segmentdb.DBTX }

var _ segmentport.ProfileCatalogHistoryStore = (*ProfileCatalogHistoryStore)(nil)
var _ segmentport.ProfileCatalogHistoryReader = (*ProfileCatalogHistoryReader)(nil)

func NewProfileCatalogHistoryStore() *ProfileCatalogHistoryStore {
	return &ProfileCatalogHistoryStore{tx: platformstore.TxFromContext}
}
func NewProfileCatalogHistoryReader(db segmentdb.DBTX) *ProfileCatalogHistoryReader {
	return &ProfileCatalogHistoryReader{db: db}
}

func (store *ProfileCatalogHistoryStore) queries(ctx context.Context) (*segmentdb.Queries, error) {
	if store == nil || store.tx == nil || ctx == nil || ctx.Err() != nil {
		return nil, segmentport.ErrProfileCatalogHistoryUnavailable
	}
	tx, err := store.tx(ctx)
	if err != nil || tx == nil {
		return nil, segmentport.ErrProfileCatalogHistoryUnavailable
	}
	return segmentdb.New(tx), nil
}
func (reader *ProfileCatalogHistoryReader) queries(ctx context.Context) (*segmentdb.Queries, error) {
	if reader == nil || profileCatalogNil(reader.db) || ctx == nil || ctx.Err() != nil {
		return nil, segmentport.ErrProfileCatalogHistoryUnavailable
	}
	return segmentdb.New(reader.db), nil
}

func (store *ProfileCatalogHistoryStore) CreateHistoricalProfileTemplate(ctx context.Context, value segmentport.HistoricalProfileTemplate) (segmentport.HistoricalProfileTemplate, error) {
	if !validProfileTemplateCreate(value) {
		return segmentport.HistoricalProfileTemplate{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalProfileTemplate{}, err
	}
	row, err := queries.CreateHistoricalProfileTemplate(ctx, segmentdb.CreateHistoricalProfileTemplateParams{SourceID: value.SourceID, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], TemplateCode: value.TemplateCode, TemplateName: value.TemplateName, QuestionnaireSourceID: profileCatalogNullableInt(value.QuestionnaireSourceID), SegmentationQuestionSourceID: profileCatalogNullableInt(value.SegmentationQuestionSourceID), ProgramSourceID: profileCatalogNullableInt(value.ProgramSourceID), Description: value.Description, OriginalEnabled: value.OriginalEnabled, Version: value.Version, CreatedByDigest: value.CreatedByDigest[:], UpdatedByDigest: value.UpdatedByDigest[:], CreatedAt: profileCatalogTime(value.CreatedAt), UpdatedAt: profileCatalogTime(value.UpdatedAt)})
	if err != nil {
		return segmentport.HistoricalProfileTemplate{}, profileCatalogStoreError(err)
	}
	return profileCatalogTemplate(row)
}
func (store *ProfileCatalogHistoryStore) GetHistoricalProfileTemplate(ctx context.Context, id int64) (segmentport.HistoricalProfileTemplate, error) {
	queries, err := store.queries(ctx)
	if id < 1 {
		return segmentport.HistoricalProfileTemplate{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	if err != nil {
		return segmentport.HistoricalProfileTemplate{}, err
	}
	row, err := queries.GetHistoricalProfileTemplate(ctx, id)
	if err != nil {
		return segmentport.HistoricalProfileTemplate{}, profileCatalogStoreError(err)
	}
	return profileCatalogTemplate(row)
}
func (store *ProfileCatalogHistoryStore) CreateHistoricalProfileCategory(ctx context.Context, value segmentport.HistoricalProfileCategory) (segmentport.HistoricalProfileCategory, error) {
	if !validProfileCategoryCreate(value) {
		return segmentport.HistoricalProfileCategory{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalProfileCategory{}, err
	}
	row, err := queries.CreateHistoricalProfileCategory(ctx, segmentdb.CreateHistoricalProfileCategoryParams{SourceID: value.SourceID, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], TemplateSourceID: value.TemplateSourceID, TemplateHistoryID: value.TemplateHistoryID, CategoryKey: value.CategoryKey, CategoryName: value.CategoryName, Description: value.Description, SortOrder: value.SortOrder, OriginalEnabled: value.OriginalEnabled, CreatedAt: profileCatalogTime(value.CreatedAt), UpdatedAt: profileCatalogTime(value.UpdatedAt)})
	if err != nil {
		return segmentport.HistoricalProfileCategory{}, profileCatalogStoreError(err)
	}
	return profileCatalogCategory(row)
}
func (store *ProfileCatalogHistoryStore) GetHistoricalProfileCategory(ctx context.Context, id int64) (segmentport.HistoricalProfileCategory, error) {
	queries, err := store.queries(ctx)
	if id < 1 {
		return segmentport.HistoricalProfileCategory{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	if err != nil {
		return segmentport.HistoricalProfileCategory{}, err
	}
	row, err := queries.GetHistoricalProfileCategory(ctx, id)
	if err != nil {
		return segmentport.HistoricalProfileCategory{}, profileCatalogStoreError(err)
	}
	return profileCatalogCategory(row)
}
func (store *ProfileCatalogHistoryStore) CreateHistoricalProfileOptionMapping(ctx context.Context, value segmentport.HistoricalProfileOptionMapping) (segmentport.HistoricalProfileOptionMapping, error) {
	if !validProfileMappingCreate(value) {
		return segmentport.HistoricalProfileOptionMapping{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalProfileOptionMapping{}, err
	}
	row, err := queries.CreateHistoricalProfileOptionMapping(ctx, segmentdb.CreateHistoricalProfileOptionMappingParams{SourceID: value.SourceID, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], TemplateSourceID: value.TemplateSourceID, CategorySourceID: value.CategorySourceID, TemplateHistoryID: value.TemplateHistoryID, CategoryHistoryID: value.CategoryHistoryID, QuestionSourceID: value.QuestionSourceID, OptionSourceID: value.OptionSourceID, CreatedAt: profileCatalogTime(value.CreatedAt)})
	if err != nil {
		return segmentport.HistoricalProfileOptionMapping{}, profileCatalogStoreError(err)
	}
	return profileCatalogMapping(row)
}
func (store *ProfileCatalogHistoryStore) GetHistoricalProfileOptionMapping(ctx context.Context, id int64) (segmentport.HistoricalProfileOptionMapping, error) {
	queries, err := store.queries(ctx)
	if id < 1 {
		return segmentport.HistoricalProfileOptionMapping{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	if err != nil {
		return segmentport.HistoricalProfileOptionMapping{}, err
	}
	row, err := queries.GetHistoricalProfileOptionMapping(ctx, id)
	if err != nil {
		return segmentport.HistoricalProfileOptionMapping{}, profileCatalogStoreError(err)
	}
	return profileCatalogMapping(row)
}

func (reader *ProfileCatalogHistoryReader) GetHistoricalProfileTemplate(ctx context.Context, id int64) (segmentport.HistoricalProfileTemplate, error) {
	if id < 1 {
		return segmentport.HistoricalProfileTemplate{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return segmentport.HistoricalProfileTemplate{}, err
	}
	row, err := queries.GetHistoricalProfileTemplate(ctx, id)
	if err != nil {
		return segmentport.HistoricalProfileTemplate{}, profileCatalogStoreError(err)
	}
	return profileCatalogTemplate(row)
}
func (reader *ProfileCatalogHistoryReader) GetHistoricalProfileCategory(ctx context.Context, id int64) (segmentport.HistoricalProfileCategory, error) {
	if id < 1 {
		return segmentport.HistoricalProfileCategory{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return segmentport.HistoricalProfileCategory{}, err
	}
	row, err := queries.GetHistoricalProfileCategory(ctx, id)
	if err != nil {
		return segmentport.HistoricalProfileCategory{}, profileCatalogStoreError(err)
	}
	return profileCatalogCategory(row)
}
func (reader *ProfileCatalogHistoryReader) GetHistoricalProfileOptionMapping(ctx context.Context, id int64) (segmentport.HistoricalProfileOptionMapping, error) {
	if id < 1 {
		return segmentport.HistoricalProfileOptionMapping{}, segmentport.ErrProfileCatalogHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return segmentport.HistoricalProfileOptionMapping{}, err
	}
	row, err := queries.GetHistoricalProfileOptionMapping(ctx, id)
	if err != nil {
		return segmentport.HistoricalProfileOptionMapping{}, profileCatalogStoreError(err)
	}
	return profileCatalogMapping(row)
}

func (reader *ProfileCatalogHistoryReader) ListHistoricalProfileTemplates(ctx context.Context, q segmentport.ProfileCatalogHistoryQuery) ([]segmentport.HistoricalProfileTemplate, int64, error) {
	if q.TemplateHistoryID != nil || q.CategoryHistoryID != nil {
		return nil, 0, segmentport.ErrProfileCatalogHistoryInvalid
	}
	queries, err := reader.page(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	count, err := queries.CountHistoricalProfileTemplates(ctx)
	if err != nil {
		return nil, 0, profileCatalogStoreError(err)
	}
	rows, err := queries.ListHistoricalProfileTemplates(ctx, segmentdb.ListHistoricalProfileTemplatesParams{Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		return nil, 0, profileCatalogStoreError(err)
	}
	result := make([]segmentport.HistoricalProfileTemplate, 0, len(rows))
	for _, row := range rows {
		value, err := profileCatalogTemplate(row)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, value)
	}
	return result, count, nil
}
func (reader *ProfileCatalogHistoryReader) ListHistoricalProfileCategories(ctx context.Context, q segmentport.ProfileCatalogHistoryQuery) ([]segmentport.HistoricalProfileCategory, int64, error) {
	if q.TemplateHistoryID == nil || *q.TemplateHistoryID < 1 || q.CategoryHistoryID != nil {
		return nil, 0, segmentport.ErrProfileCatalogHistoryInvalid
	}
	queries, err := reader.page(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	count, err := queries.CountHistoricalProfileCategories(ctx, *q.TemplateHistoryID)
	if err != nil {
		return nil, 0, profileCatalogStoreError(err)
	}
	rows, err := queries.ListHistoricalProfileCategories(ctx, segmentdb.ListHistoricalProfileCategoriesParams{TemplateHistoryID: *q.TemplateHistoryID, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		return nil, 0, profileCatalogStoreError(err)
	}
	result := make([]segmentport.HistoricalProfileCategory, 0, len(rows))
	for _, row := range rows {
		value, err := profileCatalogCategory(row)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, value)
	}
	return result, count, nil
}
func (reader *ProfileCatalogHistoryReader) ListHistoricalProfileOptionMappings(ctx context.Context, q segmentport.ProfileCatalogHistoryQuery) ([]segmentport.HistoricalProfileOptionMapping, int64, error) {
	if q.TemplateHistoryID == nil || q.CategoryHistoryID == nil || *q.TemplateHistoryID < 1 || *q.CategoryHistoryID < 1 {
		return nil, 0, segmentport.ErrProfileCatalogHistoryInvalid
	}
	queries, err := reader.page(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	count, err := queries.CountHistoricalProfileOptionMappings(ctx, segmentdb.CountHistoricalProfileOptionMappingsParams{TemplateHistoryID: *q.TemplateHistoryID, CategoryHistoryID: *q.CategoryHistoryID})
	if err != nil {
		return nil, 0, profileCatalogStoreError(err)
	}
	rows, err := queries.ListHistoricalProfileOptionMappings(ctx, segmentdb.ListHistoricalProfileOptionMappingsParams{TemplateHistoryID: *q.TemplateHistoryID, CategoryHistoryID: *q.CategoryHistoryID, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		return nil, 0, profileCatalogStoreError(err)
	}
	result := make([]segmentport.HistoricalProfileOptionMapping, 0, len(rows))
	for _, row := range rows {
		value, err := profileCatalogMapping(row)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, value)
	}
	return result, count, nil
}

func (reader *ProfileCatalogHistoryReader) page(ctx context.Context, q segmentport.ProfileCatalogHistoryQuery) (*segmentdb.Queries, error) {
	if q.Limit < 1 || q.Limit > 100 || q.Offset < 0 {
		return nil, segmentport.ErrProfileCatalogHistoryInvalid
	}
	return reader.queries(ctx)
}
func profileCatalogNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
func profileCatalogTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func profileCatalogNullableInt(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}
func profileCatalogOptionalInt(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	copy := value.Int64
	return &copy
}
func profileCatalogRequiredTime(value pgtype.Timestamptz) (time.Time, error) {
	if !value.Valid || value.InfinityModifier != pgtype.Finite || value.Time.IsZero() {
		return time.Time{}, segmentport.ErrProfileCatalogHistoryUnavailable
	}
	return value.Time, nil
}
func profileCatalogDigest(value []byte) ([32]byte, error) {
	var result [32]byte
	if len(value) != len(result) {
		return result, segmentport.ErrProfileCatalogHistoryUnavailable
	}
	copy(result[:], value)
	return result, nil
}
func profileCatalogStoreError(err error) error {
	var pgErr *pgconn.PgError
	if errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &pgErr) && strings.HasPrefix(pgErr.Code, "23")) {
		return segmentport.ErrProfileCatalogHistoryConflict
	}
	return segmentport.ErrProfileCatalogHistoryUnavailable
}
func validProfileTemplateCreate(value segmentport.HistoricalProfileTemplate) bool {
	return value.ID == 0 && value.SourceKeyDigest != ([32]byte{}) && value.SourcePayloadDigest != ([32]byte{}) && value.CreatedByDigest != ([32]byte{}) && value.UpdatedByDigest != ([32]byte{}) && !value.CreatedAt.IsZero() && !value.UpdatedAt.IsZero()
}
func validProfileCategoryCreate(value segmentport.HistoricalProfileCategory) bool {
	return value.ID == 0 && value.SourceKeyDigest != ([32]byte{}) && value.SourcePayloadDigest != ([32]byte{}) && value.TemplateHistoryID > 0 && !value.CreatedAt.IsZero() && !value.UpdatedAt.IsZero()
}
func validProfileMappingCreate(value segmentport.HistoricalProfileOptionMapping) bool {
	return value.ID == 0 && value.SourceKeyDigest != ([32]byte{}) && value.SourcePayloadDigest != ([32]byte{}) && value.TemplateHistoryID > 0 && value.CategoryHistoryID > 0 && !value.CreatedAt.IsZero()
}
func profileCatalogTemplate(row segmentdb.SegmentV1ProfileTemplate) (segmentport.HistoricalProfileTemplate, error) {
	created, err := profileCatalogRequiredTime(row.CreatedAt)
	if err != nil {
		return segmentport.HistoricalProfileTemplate{}, err
	}
	updated, err := profileCatalogRequiredTime(row.UpdatedAt)
	if err != nil {
		return segmentport.HistoricalProfileTemplate{}, err
	}
	key, err := profileCatalogDigest(row.SourceKeyDigest)
	if err != nil {
		return segmentport.HistoricalProfileTemplate{}, err
	}
	payload, err := profileCatalogDigest(row.SourcePayloadDigest)
	if err != nil {
		return segmentport.HistoricalProfileTemplate{}, err
	}
	createdBy, err := profileCatalogDigest(row.CreatedByDigest)
	if err != nil {
		return segmentport.HistoricalProfileTemplate{}, err
	}
	updatedBy, err := profileCatalogDigest(row.UpdatedByDigest)
	if err != nil {
		return segmentport.HistoricalProfileTemplate{}, err
	}
	if row.ID < 1 {
		return segmentport.HistoricalProfileTemplate{}, segmentport.ErrProfileCatalogHistoryUnavailable
	}
	return segmentport.HistoricalProfileTemplate{ID: row.ID, SourceID: row.SourceID, SourceKeyDigest: key, SourcePayloadDigest: payload, TemplateCode: row.TemplateCode, TemplateName: row.TemplateName, QuestionnaireSourceID: profileCatalogOptionalInt(row.QuestionnaireSourceID), SegmentationQuestionSourceID: profileCatalogOptionalInt(row.SegmentationQuestionSourceID), ProgramSourceID: profileCatalogOptionalInt(row.ProgramSourceID), Description: row.Description, OriginalEnabled: row.OriginalEnabled, Version: row.Version, CreatedByDigest: createdBy, UpdatedByDigest: updatedBy, CreatedAt: created, UpdatedAt: updated}, nil
}
func profileCatalogCategory(row segmentdb.SegmentV1ProfileCategory) (segmentport.HistoricalProfileCategory, error) {
	created, err := profileCatalogRequiredTime(row.CreatedAt)
	if err != nil {
		return segmentport.HistoricalProfileCategory{}, err
	}
	updated, err := profileCatalogRequiredTime(row.UpdatedAt)
	if err != nil {
		return segmentport.HistoricalProfileCategory{}, err
	}
	key, err := profileCatalogDigest(row.SourceKeyDigest)
	if err != nil {
		return segmentport.HistoricalProfileCategory{}, err
	}
	payload, err := profileCatalogDigest(row.SourcePayloadDigest)
	if err != nil {
		return segmentport.HistoricalProfileCategory{}, err
	}
	if row.ID < 1 || row.TemplateHistoryID < 1 {
		return segmentport.HistoricalProfileCategory{}, segmentport.ErrProfileCatalogHistoryUnavailable
	}
	return segmentport.HistoricalProfileCategory{ID: row.ID, SourceID: row.SourceID, SourceKeyDigest: key, SourcePayloadDigest: payload, TemplateSourceID: row.TemplateSourceID, TemplateHistoryID: row.TemplateHistoryID, CategoryKey: row.CategoryKey, CategoryName: row.CategoryName, Description: row.Description, SortOrder: row.SortOrder, OriginalEnabled: row.OriginalEnabled, CreatedAt: created, UpdatedAt: updated}, nil
}
func profileCatalogMapping(row segmentdb.SegmentV1ProfileOptionMapping) (segmentport.HistoricalProfileOptionMapping, error) {
	created, err := profileCatalogRequiredTime(row.CreatedAt)
	if err != nil {
		return segmentport.HistoricalProfileOptionMapping{}, err
	}
	key, err := profileCatalogDigest(row.SourceKeyDigest)
	if err != nil {
		return segmentport.HistoricalProfileOptionMapping{}, err
	}
	payload, err := profileCatalogDigest(row.SourcePayloadDigest)
	if err != nil {
		return segmentport.HistoricalProfileOptionMapping{}, err
	}
	if row.ID < 1 || row.TemplateHistoryID < 1 || row.CategoryHistoryID < 1 {
		return segmentport.HistoricalProfileOptionMapping{}, segmentport.ErrProfileCatalogHistoryUnavailable
	}
	return segmentport.HistoricalProfileOptionMapping{ID: row.ID, SourceID: row.SourceID, SourceKeyDigest: key, SourcePayloadDigest: payload, TemplateSourceID: row.TemplateSourceID, CategorySourceID: row.CategorySourceID, TemplateHistoryID: row.TemplateHistoryID, CategoryHistoryID: row.CategoryHistoryID, QuestionSourceID: row.QuestionSourceID, OptionSourceID: row.OptionSourceID, CreatedAt: created}, nil
}
