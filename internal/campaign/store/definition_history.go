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
	campaignapp "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/app"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	campaigndb "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type CampaignDefinitionHistoryStore struct{}
type CampaignDefinitionHistoryReader struct{ db campaigndb.DBTX }

var _ campaignport.CampaignDefinitionHistoryStore = (*CampaignDefinitionHistoryStore)(nil)
var _ campaignport.CampaignDefinitionHistoryReader = (*CampaignDefinitionHistoryReader)(nil)
var _ campaignport.CampaignDefinitionCurrentReader = (*CampaignDefinitionHistoryReader)(nil)

func (reader *CampaignDefinitionHistoryReader) GetCurrentCampaignDefinitionHistoryParent(ctx context.Context, code string) (campaignport.CampaignDefinitionCurrentParent, error) {
	if code == "" {
		return campaignport.CampaignDefinitionCurrentParent{}, campaignport.ErrCampaignHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return campaignport.CampaignDefinitionCurrentParent{}, err
	}
	row, err := queries.GetCurrentCampaignDefinitionHistoryParent(ctx, code)
	if err != nil {
		return campaignport.CampaignDefinitionCurrentParent{}, campaignDefinitionHistoryError(err)
	}
	return campaignport.CampaignDefinitionCurrentParent{Code: row.CampaignCode, ApprovalStatus: row.ApprovalStatus, RuntimeStatus: row.RuntimeStatus, Version: row.Version}, nil
}

func NewCampaignDefinitionHistoryStore() *CampaignDefinitionHistoryStore {
	return &CampaignDefinitionHistoryStore{}
}

func NewCampaignDefinitionHistoryReader(db campaigndb.DBTX) *CampaignDefinitionHistoryReader {
	return &CampaignDefinitionHistoryReader{db: db}
}

func (store *CampaignDefinitionHistoryStore) queries(ctx context.Context) (*campaigndb.Queries, error) {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return nil, campaignport.ErrCampaignHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, campaignport.ErrCampaignHistoryUnavailable
	}
	return campaigndb.New(tx), nil
}

func (reader *CampaignDefinitionHistoryReader) queries(ctx context.Context) (*campaigndb.Queries, error) {
	if reader == nil || ctx == nil || ctx.Err() != nil {
		return nil, campaignport.ErrCampaignHistoryUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return campaigndb.New(tx), nil
	}
	if reader.db == nil {
		return nil, campaignport.ErrCampaignHistoryUnavailable
	}
	value := reflect.ValueOf(reader.db)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return nil, campaignport.ErrCampaignHistoryUnavailable
	}
	return campaigndb.New(reader.db), nil
}

func (store *CampaignDefinitionHistoryStore) CreateHistoricalCampaignDefinition(ctx context.Context, value campaignport.HistoricalCampaignDefinition) (campaignport.HistoricalCampaignDefinition, error) {
	validation := value
	validation.ID = 1
	if value.ID != 0 || !campaignDefinitionHistoryDefinitionValid(validation) {
		return campaignport.HistoricalCampaignDefinition{}, campaignport.ErrCampaignHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return campaignport.HistoricalCampaignDefinition{}, err
	}
	row, err := queries.CreateHistoricalCampaignDefinition(ctx, campaignDefinitionHistoryDefinitionParams(value))
	if err != nil {
		return campaignport.HistoricalCampaignDefinition{}, campaignDefinitionHistoryError(err)
	}
	return campaignDefinitionHistoryDefinitionValue(row)
}

func (store *CampaignDefinitionHistoryStore) GetHistoricalCampaignDefinition(ctx context.Context, id int64) (campaignport.HistoricalCampaignDefinition, error) {
	if id < 1 {
		return campaignport.HistoricalCampaignDefinition{}, campaignport.ErrCampaignHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return campaignport.HistoricalCampaignDefinition{}, err
	}
	row, err := queries.GetHistoricalCampaignDefinition(ctx, id)
	if err != nil {
		return campaignport.HistoricalCampaignDefinition{}, campaignDefinitionHistoryError(err)
	}
	return campaignDefinitionHistoryDefinitionValue(row)
}

func (reader *CampaignDefinitionHistoryReader) GetHistoricalCampaignDefinition(ctx context.Context, id int64) (campaignport.HistoricalCampaignDefinition, error) {
	if id < 1 {
		return campaignport.HistoricalCampaignDefinition{}, campaignport.ErrCampaignHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return campaignport.HistoricalCampaignDefinition{}, err
	}
	row, err := queries.GetHistoricalCampaignDefinition(ctx, id)
	if err != nil {
		return campaignport.HistoricalCampaignDefinition{}, campaignDefinitionHistoryError(err)
	}
	return campaignDefinitionHistoryDefinitionValue(row)
}

func (store *CampaignDefinitionHistoryStore) CreateHistoricalCampaignDefinitionStep(ctx context.Context, value campaignport.HistoricalCampaignDefinitionStep) (campaignport.HistoricalCampaignDefinitionStep, error) {
	validation := value
	validation.ID = 1
	if value.ID != 0 || !campaignDefinitionHistoryStepValid(validation) {
		return campaignport.HistoricalCampaignDefinitionStep{}, campaignport.ErrCampaignHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return campaignport.HistoricalCampaignDefinitionStep{}, err
	}
	row, err := queries.CreateHistoricalCampaignDefinitionStep(ctx, campaignDefinitionHistoryStepParams(value))
	if err != nil {
		return campaignport.HistoricalCampaignDefinitionStep{}, campaignDefinitionHistoryError(err)
	}
	return campaignDefinitionHistoryStepValue(row)
}

func (store *CampaignDefinitionHistoryStore) GetHistoricalCampaignDefinitionStep(ctx context.Context, id int64) (campaignport.HistoricalCampaignDefinitionStep, error) {
	if id < 1 {
		return campaignport.HistoricalCampaignDefinitionStep{}, campaignport.ErrCampaignHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return campaignport.HistoricalCampaignDefinitionStep{}, err
	}
	row, err := queries.GetHistoricalCampaignDefinitionStep(ctx, id)
	if err != nil {
		return campaignport.HistoricalCampaignDefinitionStep{}, campaignDefinitionHistoryError(err)
	}
	return campaignDefinitionHistoryStepValue(row)
}

func (reader *CampaignDefinitionHistoryReader) GetHistoricalCampaignDefinitionStep(ctx context.Context, id int64) (campaignport.HistoricalCampaignDefinitionStep, error) {
	if id < 1 {
		return campaignport.HistoricalCampaignDefinitionStep{}, campaignport.ErrCampaignHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return campaignport.HistoricalCampaignDefinitionStep{}, err
	}
	row, err := queries.GetHistoricalCampaignDefinitionStep(ctx, id)
	if err != nil {
		return campaignport.HistoricalCampaignDefinitionStep{}, campaignDefinitionHistoryError(err)
	}
	return campaignDefinitionHistoryStepValue(row)
}

func (reader *CampaignDefinitionHistoryReader) ListHistoricalCampaignDefinitions(ctx context.Context, limit, offset int32) ([]campaignport.HistoricalCampaignDefinition, int64, error) {
	if !campaignDefinitionHistoryPage(limit, offset) {
		return nil, 0, campaignport.ErrCampaignHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountHistoricalCampaignDefinitions(ctx)
	if err != nil {
		return nil, 0, campaignDefinitionHistoryError(err)
	}
	rows, err := queries.ListHistoricalCampaignDefinitions(ctx, campaigndb.ListHistoricalCampaignDefinitionsParams{PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, campaignDefinitionHistoryError(err)
	}
	items := make([]campaignport.HistoricalCampaignDefinition, 0, len(rows))
	for _, row := range rows {
		value, err := campaignDefinitionHistoryDefinitionValue(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
	}
	return items, total, nil
}

func (reader *CampaignDefinitionHistoryReader) ListHistoricalCampaignDefinitionSteps(ctx context.Context, campaignSourceID *int64, limit, offset int32) ([]campaignport.HistoricalCampaignDefinitionStep, int64, error) {
	if !campaignDefinitionHistoryPage(limit, offset) {
		return nil, 0, campaignport.ErrCampaignHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	filter := campaignDefinitionHistoryInt(campaignSourceID)
	total, err := queries.CountHistoricalCampaignDefinitionSteps(ctx, filter)
	if err != nil {
		return nil, 0, campaignDefinitionHistoryError(err)
	}
	rows, err := queries.ListHistoricalCampaignDefinitionSteps(ctx, campaigndb.ListHistoricalCampaignDefinitionStepsParams{CampaignSourceID: filter, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, campaignDefinitionHistoryError(err)
	}
	items := make([]campaignport.HistoricalCampaignDefinitionStep, 0, len(rows))
	for _, row := range rows {
		value, err := campaignDefinitionHistoryStepValue(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
	}
	return items, total, nil
}

func campaignDefinitionHistoryDefinitionParams(value campaignport.HistoricalCampaignDefinition) campaigndb.CreateHistoricalCampaignDefinitionParams {
	return campaigndb.CreateHistoricalCampaignDefinitionParams{
		SourceID: value.SourceID, Code: value.Code, DisplayName: value.DisplayName, Intent: value.Intent, AnchorMode: value.AnchorMode,
		AnchorDate: value.AnchorDate, ReviewStatus: value.ReviewStatus, RunStatus: value.RunStatus,
		ApprovedAt: campaignDefinitionHistoryOptionalTimestamp(value.ApprovedAt), StartedAt: campaignDefinitionHistoryOptionalTimestamp(value.StartedAt),
		FinishedAt: campaignDefinitionHistoryOptionalTimestamp(value.FinishedAt), PausedAt: campaignDefinitionHistoryOptionalTimestamp(value.PausedAt),
		PausedReason: value.PausedReason, CreatedAt: campaignDefinitionHistoryTimestamp(value.CreatedAt), UpdatedAt: campaignDefinitionHistoryTimestamp(value.UpdatedAt),
		OriginalDisposition: value.OriginalDisposition, OriginalReason: value.OriginalReason, PrivateDigest: value.PrivateDigest[:],
		SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:],
		RedactedRoots: append([]string{}, value.RedactedRoots...),
	}
}

func campaignDefinitionHistoryStepParams(value campaignport.HistoricalCampaignDefinitionStep) campaigndb.CreateHistoricalCampaignDefinitionStepParams {
	return campaigndb.CreateHistoricalCampaignDefinitionStepParams{
		SourceID: value.SourceID, CampaignSourceID: value.CampaignSourceID, SegmentSourceID: value.SegmentSourceID,
		HistoryDefinitionID: campaignDefinitionHistoryInt(value.HistoryDefinitionID), CurrentCampaignCode: campaignDefinitionHistoryText(value.CurrentCampaignCode),
		SourceParentState: value.SourceParentState, StepIndex: value.StepIndex, DayOffset: value.DayOffset, SendTime: value.SendTime,
		Timezone: value.Timezone, ContentMasked: value.ContentMasked, StopOnReply: value.StopOnReply, SkipRecentDays: value.SkipRecentDays,
		CreatedAt: campaignDefinitionHistoryTimestamp(value.CreatedAt), UpdatedAt: campaignDefinitionHistoryTimestamp(value.UpdatedAt),
		OriginalDisposition: value.OriginalDisposition, OriginalReason: value.OriginalReason, ContentDigest: value.ContentDigest[:],
		PrivateDigest: value.PrivateDigest[:], SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:],
		SourceFieldDigest: value.SourceFieldDigest[:], RedactedRoots: append([]string{}, value.RedactedRoots...),
	}
}

func campaignDefinitionHistoryDefinitionValue(row campaigndb.CampaignV1DefinitionHistory) (campaignport.HistoricalCampaignDefinition, error) {
	created, err := campaignDefinitionHistoryRequiredTime(row.CreatedAt)
	if err != nil {
		return campaignport.HistoricalCampaignDefinition{}, campaignport.ErrCampaignHistoryUnavailable
	}
	updated, err := campaignDefinitionHistoryRequiredTime(row.UpdatedAt)
	if err != nil {
		return campaignport.HistoricalCampaignDefinition{}, campaignport.ErrCampaignHistoryUnavailable
	}
	value := campaignport.HistoricalCampaignDefinition{
		ID: row.ID, SourceID: row.SourceID, Code: row.Code, DisplayName: row.DisplayName, Intent: row.Intent, AnchorMode: row.AnchorMode,
		AnchorDate: row.AnchorDate, ReviewStatus: row.ReviewStatus, RunStatus: row.RunStatus, PausedReason: row.PausedReason,
		CreatedAt: created, UpdatedAt: updated, OriginalDisposition: row.OriginalDisposition, OriginalReason: row.OriginalReason,
		RedactedRoots: append([]string{}, row.RedactedRoots...),
	}
	if value.ApprovedAt, err = campaignDefinitionHistoryOptionalTime(row.ApprovedAt); err != nil {
		return campaignport.HistoricalCampaignDefinition{}, campaignport.ErrCampaignHistoryUnavailable
	}
	if value.StartedAt, err = campaignDefinitionHistoryOptionalTime(row.StartedAt); err != nil {
		return campaignport.HistoricalCampaignDefinition{}, campaignport.ErrCampaignHistoryUnavailable
	}
	if value.FinishedAt, err = campaignDefinitionHistoryOptionalTime(row.FinishedAt); err != nil {
		return campaignport.HistoricalCampaignDefinition{}, campaignport.ErrCampaignHistoryUnavailable
	}
	if value.PausedAt, err = campaignDefinitionHistoryOptionalTime(row.PausedAt); err != nil {
		return campaignport.HistoricalCampaignDefinition{}, campaignport.ErrCampaignHistoryUnavailable
	}
	if !campaignDefinitionHistoryCopyDigests([][]byte{row.PrivateDigest, row.SourceKeyDigest, row.SourcePayloadDigest, row.SourceFieldDigest}, []*[32]byte{&value.PrivateDigest, &value.SourceKeyDigest, &value.SourcePayloadDigest, &value.SourceFieldDigest}) || !campaignDefinitionHistoryDefinitionValid(value) {
		return campaignport.HistoricalCampaignDefinition{}, campaignport.ErrCampaignHistoryUnavailable
	}
	return value, nil
}

func campaignDefinitionHistoryStepValue(row campaigndb.CampaignV1DefinitionStepHistory) (campaignport.HistoricalCampaignDefinitionStep, error) {
	created, err := campaignDefinitionHistoryRequiredTime(row.CreatedAt)
	if err != nil {
		return campaignport.HistoricalCampaignDefinitionStep{}, campaignport.ErrCampaignHistoryUnavailable
	}
	updated, err := campaignDefinitionHistoryRequiredTime(row.UpdatedAt)
	if err != nil {
		return campaignport.HistoricalCampaignDefinitionStep{}, campaignport.ErrCampaignHistoryUnavailable
	}
	value := campaignport.HistoricalCampaignDefinitionStep{
		ID: row.ID, SourceID: row.SourceID, CampaignSourceID: row.CampaignSourceID, SegmentSourceID: row.SegmentSourceID,
		SourceParentState: row.SourceParentState, StepIndex: row.StepIndex, DayOffset: row.DayOffset, SendTime: row.SendTime,
		Timezone: row.Timezone, ContentMasked: row.ContentMasked, StopOnReply: row.StopOnReply, SkipRecentDays: row.SkipRecentDays,
		CreatedAt: created, UpdatedAt: updated, OriginalDisposition: row.OriginalDisposition, OriginalReason: row.OriginalReason,
		RedactedRoots: append([]string{}, row.RedactedRoots...),
	}
	if value.HistoryDefinitionID, err = campaignDefinitionHistoryOptionalID(row.HistoryDefinitionID); err != nil {
		return campaignport.HistoricalCampaignDefinitionStep{}, campaignport.ErrCampaignHistoryUnavailable
	}
	if value.CurrentCampaignCode, err = campaignDefinitionHistoryOptionalCode(row.CurrentCampaignCode); err != nil {
		return campaignport.HistoricalCampaignDefinitionStep{}, campaignport.ErrCampaignHistoryUnavailable
	}
	if !campaignDefinitionHistoryCopyDigests([][]byte{row.ContentDigest, row.PrivateDigest, row.SourceKeyDigest, row.SourcePayloadDigest, row.SourceFieldDigest}, []*[32]byte{&value.ContentDigest, &value.PrivateDigest, &value.SourceKeyDigest, &value.SourcePayloadDigest, &value.SourceFieldDigest}) || !campaignDefinitionHistoryStepValid(value) {
		return campaignport.HistoricalCampaignDefinitionStep{}, campaignport.ErrCampaignHistoryUnavailable
	}
	return value, nil
}

func campaignDefinitionHistoryDefinitionValid(value campaignport.HistoricalCampaignDefinition) bool {
	_, err := campaignapp.HistoricalCampaignDefinitionDigest(value)
	return err == nil
}

func campaignDefinitionHistoryStepValid(value campaignport.HistoricalCampaignDefinitionStep) bool {
	_, err := campaignapp.HistoricalCampaignDefinitionStepDigest(value)
	return err == nil
}

func campaignDefinitionHistoryPage(limit, offset int32) bool {
	return limit >= 1 && limit <= 100 && offset >= 0
}

func campaignDefinitionHistoryCopyDigests(sources [][]byte, targets []*[32]byte) bool {
	if len(sources) != len(targets) {
		return false
	}
	for index, source := range sources {
		if len(source) != 32 {
			return false
		}
		copy(targets[index][:], source)
	}
	return true
}

func campaignDefinitionHistoryTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC().Truncate(time.Microsecond), Valid: true}
}

func campaignDefinitionHistoryOptionalTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return campaignDefinitionHistoryTimestamp(*value)
}

func campaignDefinitionHistoryInt(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func campaignDefinitionHistoryText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func campaignDefinitionHistoryRequiredTime(value pgtype.Timestamptz) (time.Time, error) {
	if !value.Valid || value.InfinityModifier != pgtype.Finite {
		return time.Time{}, errors.New("invalid required historical timestamp")
	}
	return value.Time.UTC().Truncate(time.Microsecond), nil
}

func campaignDefinitionHistoryOptionalTime(value pgtype.Timestamptz) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	result, err := campaignDefinitionHistoryRequiredTime(value)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func campaignDefinitionHistoryOptionalID(value pgtype.Int8) (*int64, error) {
	if !value.Valid {
		return nil, nil
	}
	if value.Int64 < 1 {
		return nil, errors.New("invalid historical parent ID")
	}
	result := value.Int64
	return &result, nil
}

func campaignDefinitionHistoryOptionalCode(value pgtype.Text) (*string, error) {
	if !value.Valid {
		return nil, nil
	}
	if strings.TrimSpace(value.String) == "" {
		return nil, errors.New("invalid historical current campaign code")
	}
	result := value.String
	return &result, nil
}

func campaignDefinitionHistoryError(err error) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && (pgError.Code == "23505" || pgError.Code == "23503") {
		return campaignport.ErrCampaignHistoryConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return campaignport.ErrCampaignHistoryConflict
	}
	return campaignport.ErrCampaignHistoryUnavailable
}
