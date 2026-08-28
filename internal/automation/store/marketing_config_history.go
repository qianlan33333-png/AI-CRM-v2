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
	automationapp "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/app"
	automation "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	automationdb "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

// MarketingConfigHistoryStore uses only the caller transaction for writes.
type MarketingConfigHistoryStore struct{}

// MarketingConfigHistoryReader accepts a pool or transaction. A transaction
// carried by context takes precedence so replay checks see uncommitted rows.
type MarketingConfigHistoryReader struct{ db automationdb.DBTX }

var _ automation.MarketingConfigHistoryStore = (*MarketingConfigHistoryStore)(nil)
var _ automation.MarketingConfigHistoryReader = (*MarketingConfigHistoryReader)(nil)

func NewMarketingConfigHistoryStore() *MarketingConfigHistoryStore {
	return &MarketingConfigHistoryStore{}
}
func NewMarketingConfigHistoryReader(db automationdb.DBTX) *MarketingConfigHistoryReader {
	return &MarketingConfigHistoryReader{db: db}
}

func (s *MarketingConfigHistoryStore) q(ctx context.Context) (*automationdb.Queries, error) {
	if s == nil || ctx == nil || ctx.Err() != nil {
		return nil, automation.ErrMarketingConfigHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, automation.ErrMarketingConfigHistoryUnavailable
	}
	return automationdb.New(tx), nil
}

func (r *MarketingConfigHistoryReader) q(ctx context.Context) (*automationdb.Queries, error) {
	if r == nil || ctx == nil || ctx.Err() != nil {
		return nil, automation.ErrMarketingConfigHistoryUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return automationdb.New(tx), nil
	}
	if nilMarketingConfigHistoryDB(r.db) {
		return nil, automation.ErrMarketingConfigHistoryUnavailable
	}
	return automationdb.New(r.db), nil
}

func (s *MarketingConfigHistoryStore) CreateHistoricalMarketingAutomationConfig(ctx context.Context, value automation.HistoricalMarketingAutomationConfig) (automation.HistoricalMarketingAutomationConfig, error) {
	value = normalizeStoredMarketingConfig(value)
	if value.ID != 0 || invalidStoredMarketingConfig(value) {
		return automation.HistoricalMarketingAutomationConfig{}, automation.ErrMarketingConfigHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return automation.HistoricalMarketingAutomationConfig{}, err
	}
	row, err := q.CreateHistoricalMarketingAutomationConfig(ctx, automationdb.CreateHistoricalMarketingAutomationConfigParams{
		SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], SourceID: value.SourceID,
		AutomationKey: value.AutomationKey, AutomationName: value.AutomationName, TargetEvent: value.TargetEvent, ChannelType: value.ChannelType, OriginalStatus: value.OriginalStatus,
		DoNotStartAfterHour: value.DoNotStartAfterHour, CreatedAt: marketingConfigTimestamp(value.CreatedAt), UpdatedAt: marketingConfigTimestamp(value.UpdatedAt), ConfigPayloadDigest: value.ConfigPayloadDigest[:],
	})
	if err != nil {
		return automation.HistoricalMarketingAutomationConfig{}, marketingConfigHistoryDBError(err)
	}
	return marketingConfigValue(row)
}

func (s *MarketingConfigHistoryStore) GetHistoricalMarketingAutomationConfig(ctx context.Context, id int64) (automation.HistoricalMarketingAutomationConfig, error) {
	return getMarketingConfig(ctx, s.q, id)
}
func (r *MarketingConfigHistoryReader) GetHistoricalMarketingAutomationConfig(ctx context.Context, id int64) (automation.HistoricalMarketingAutomationConfig, error) {
	return getMarketingConfig(ctx, r.q, id)
}
func getMarketingConfig(ctx context.Context, query func(context.Context) (*automationdb.Queries, error), id int64) (automation.HistoricalMarketingAutomationConfig, error) {
	if id < 1 {
		return automation.HistoricalMarketingAutomationConfig{}, automation.ErrMarketingConfigHistoryInvalid
	}
	q, err := query(ctx)
	if err != nil {
		return automation.HistoricalMarketingAutomationConfig{}, err
	}
	row, err := q.GetHistoricalMarketingAutomationConfig(ctx, id)
	if err != nil {
		return automation.HistoricalMarketingAutomationConfig{}, marketingConfigHistoryDBError(err)
	}
	return marketingConfigValue(row)
}

func (r *MarketingConfigHistoryReader) ListHistoricalMarketingAutomationConfig(ctx context.Context, query automation.MarketingConfigHistoryQuery) ([]automation.HistoricalMarketingAutomationConfig, int64, error) {
	if invalidMarketingConfigQuery(query) {
		return nil, 0, automation.ErrMarketingConfigHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalMarketingAutomationConfig(ctx)
	if err != nil {
		return nil, 0, marketingConfigHistoryDBError(err)
	}
	rows, err := q.ListHistoricalMarketingAutomationConfig(ctx, automationdb.ListHistoricalMarketingAutomationConfigParams{RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, marketingConfigHistoryDBError(err)
	}
	values := make([]automation.HistoricalMarketingAutomationConfig, 0, len(rows))
	for _, row := range rows {
		value, err := marketingConfigValue(row)
		if err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	return values, total, nil
}

func (s *MarketingConfigHistoryStore) CreateHistoricalMarketingAutomationRule(ctx context.Context, value automation.HistoricalMarketingAutomationRule) (automation.HistoricalMarketingAutomationRule, error) {
	value = normalizeStoredMarketingRule(value)
	if value.ID != 0 || invalidStoredMarketingRule(value) {
		return automation.HistoricalMarketingAutomationRule{}, automation.ErrMarketingConfigHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return automation.HistoricalMarketingAutomationRule{}, err
	}
	row, err := q.CreateHistoricalMarketingAutomationRule(ctx, automationdb.CreateHistoricalMarketingAutomationRuleParams{
		SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], SourceID: value.SourceID,
		ConfigID: value.ConfigID, ConfigSourceID: value.ConfigSourceID, QuestionnaireSourceID: marketingConfigInt8(value.QuestionnaireSourceID), QuestionSourceID: marketingConfigInt8(value.QuestionSourceID),
		RuleCode: value.RuleCode, RuleName: value.RuleName, AnswerMatchType: value.AnswerMatchType, ScoreDelta: value.ScoreDelta, SegmentHint: value.SegmentHint, StageHint: value.StageHint,
		OriginalActive: value.OriginalActive, SortOrder: value.SortOrder, CreatedAt: marketingConfigTimestamp(value.CreatedAt), UpdatedAt: marketingConfigTimestamp(value.UpdatedAt),
		AnswerMatchValueDigest: value.AnswerMatchValueDigest[:], RulePayloadDigest: value.RulePayloadDigest[:],
	})
	if err != nil {
		return automation.HistoricalMarketingAutomationRule{}, marketingConfigHistoryDBError(err)
	}
	return marketingRuleValue(row)
}

func (s *MarketingConfigHistoryStore) GetHistoricalMarketingAutomationRule(ctx context.Context, id int64) (automation.HistoricalMarketingAutomationRule, error) {
	return getMarketingRule(ctx, s.q, id)
}
func (r *MarketingConfigHistoryReader) GetHistoricalMarketingAutomationRule(ctx context.Context, id int64) (automation.HistoricalMarketingAutomationRule, error) {
	return getMarketingRule(ctx, r.q, id)
}
func getMarketingRule(ctx context.Context, query func(context.Context) (*automationdb.Queries, error), id int64) (automation.HistoricalMarketingAutomationRule, error) {
	if id < 1 {
		return automation.HistoricalMarketingAutomationRule{}, automation.ErrMarketingConfigHistoryInvalid
	}
	q, err := query(ctx)
	if err != nil {
		return automation.HistoricalMarketingAutomationRule{}, err
	}
	row, err := q.GetHistoricalMarketingAutomationRule(ctx, id)
	if err != nil {
		return automation.HistoricalMarketingAutomationRule{}, marketingConfigHistoryDBError(err)
	}
	return marketingRuleValue(row)
}

func (r *MarketingConfigHistoryReader) ListHistoricalMarketingAutomationRule(ctx context.Context, query automation.MarketingConfigHistoryQuery) ([]automation.HistoricalMarketingAutomationRule, int64, error) {
	if invalidMarketingConfigQuery(query) {
		return nil, 0, automation.ErrMarketingConfigHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalMarketingAutomationRule(ctx)
	if err != nil {
		return nil, 0, marketingConfigHistoryDBError(err)
	}
	rows, err := q.ListHistoricalMarketingAutomationRule(ctx, automationdb.ListHistoricalMarketingAutomationRuleParams{RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, marketingConfigHistoryDBError(err)
	}
	values := make([]automation.HistoricalMarketingAutomationRule, 0, len(rows))
	for _, row := range rows {
		value, err := marketingRuleValue(row)
		if err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	return values, total, nil
}

func marketingConfigValue(row automationdb.AutomationV1MarketingConfigHistory) (automation.HistoricalMarketingAutomationConfig, error) {
	key, ok := marketingConfigDigestValue(row.SourceKeyDigest)
	if !ok {
		return automation.HistoricalMarketingAutomationConfig{}, automation.ErrMarketingConfigHistoryUnavailable
	}
	payload, ok := marketingConfigDigestValue(row.SourcePayloadDigest)
	if !ok {
		return automation.HistoricalMarketingAutomationConfig{}, automation.ErrMarketingConfigHistoryUnavailable
	}
	field, ok := marketingConfigDigestValue(row.SourceFieldDigest)
	if !ok {
		return automation.HistoricalMarketingAutomationConfig{}, automation.ErrMarketingConfigHistoryUnavailable
	}
	configPayload, ok := marketingConfigDigestValue(row.ConfigPayloadDigest)
	if !ok {
		return automation.HistoricalMarketingAutomationConfig{}, automation.ErrMarketingConfigHistoryUnavailable
	}
	created, createdOK := marketingConfigTimeValue(row.CreatedAt)
	updated, updatedOK := marketingConfigTimeValue(row.UpdatedAt)
	value := automation.HistoricalMarketingAutomationConfig{ID: row.ID, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, SourceID: row.SourceID, AutomationKey: row.AutomationKey, AutomationName: row.AutomationName, TargetEvent: row.TargetEvent, ChannelType: row.ChannelType, OriginalStatus: row.OriginalStatus, DoNotStartAfterHour: row.DoNotStartAfterHour, CreatedAt: created, UpdatedAt: updated, ConfigPayloadDigest: configPayload}
	if !createdOK || !updatedOK || invalidStoredMarketingConfig(value) {
		return automation.HistoricalMarketingAutomationConfig{}, automation.ErrMarketingConfigHistoryUnavailable
	}
	return value, nil
}

func marketingRuleValue(row automationdb.AutomationV1MarketingRuleHistory) (automation.HistoricalMarketingAutomationRule, error) {
	key, ok := marketingConfigDigestValue(row.SourceKeyDigest)
	if !ok {
		return automation.HistoricalMarketingAutomationRule{}, automation.ErrMarketingConfigHistoryUnavailable
	}
	payload, ok := marketingConfigDigestValue(row.SourcePayloadDigest)
	if !ok {
		return automation.HistoricalMarketingAutomationRule{}, automation.ErrMarketingConfigHistoryUnavailable
	}
	field, ok := marketingConfigDigestValue(row.SourceFieldDigest)
	if !ok {
		return automation.HistoricalMarketingAutomationRule{}, automation.ErrMarketingConfigHistoryUnavailable
	}
	answer, ok := marketingConfigDigestValue(row.AnswerMatchValueDigest)
	if !ok {
		return automation.HistoricalMarketingAutomationRule{}, automation.ErrMarketingConfigHistoryUnavailable
	}
	rulePayload, ok := marketingConfigDigestValue(row.RulePayloadDigest)
	if !ok {
		return automation.HistoricalMarketingAutomationRule{}, automation.ErrMarketingConfigHistoryUnavailable
	}
	created, createdOK := marketingConfigTimeValue(row.CreatedAt)
	updated, updatedOK := marketingConfigTimeValue(row.UpdatedAt)
	value := automation.HistoricalMarketingAutomationRule{ID: row.ID, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, SourceID: row.SourceID, ConfigID: row.ConfigID, ConfigSourceID: row.ConfigSourceID, QuestionnaireSourceID: marketingConfigInt64Value(row.QuestionnaireSourceID), QuestionSourceID: marketingConfigInt64Value(row.QuestionSourceID), RuleCode: row.RuleCode, RuleName: row.RuleName, AnswerMatchType: row.AnswerMatchType, ScoreDelta: row.ScoreDelta, SegmentHint: row.SegmentHint, StageHint: row.StageHint, OriginalActive: row.OriginalActive, SortOrder: row.SortOrder, CreatedAt: created, UpdatedAt: updated, AnswerMatchValueDigest: answer, RulePayloadDigest: rulePayload}
	if !createdOK || !updatedOK || invalidStoredMarketingRule(value) {
		return automation.HistoricalMarketingAutomationRule{}, automation.ErrMarketingConfigHistoryUnavailable
	}
	return value, nil
}

func normalizeStoredMarketingConfig(value automation.HistoricalMarketingAutomationConfig) automation.HistoricalMarketingAutomationConfig {
	value.CreatedAt, value.UpdatedAt = marketingConfigStoreTime(value.CreatedAt), marketingConfigStoreTime(value.UpdatedAt)
	return value
}
func normalizeStoredMarketingRule(value automation.HistoricalMarketingAutomationRule) automation.HistoricalMarketingAutomationRule {
	value.CreatedAt, value.UpdatedAt = marketingConfigStoreTime(value.CreatedAt), marketingConfigStoreTime(value.UpdatedAt)
	value.QuestionnaireSourceID = copyMarketingConfigStoreInt64(value.QuestionnaireSourceID)
	value.QuestionSourceID = copyMarketingConfigStoreInt64(value.QuestionSourceID)
	return value
}
func invalidStoredMarketingConfig(value automation.HistoricalMarketingAutomationConfig) bool {
	if value.ID == 0 {
		value.ID = 1
	}
	_, err := automationapp.HistoricalMarketingAutomationConfigDigest(value)
	return err != nil
}
func invalidStoredMarketingRule(value automation.HistoricalMarketingAutomationRule) bool {
	if value.ID == 0 {
		value.ID = 1
	}
	_, err := automationapp.HistoricalMarketingAutomationRuleDigest(value)
	return err != nil
}
func invalidMarketingConfigQuery(query automation.MarketingConfigHistoryQuery) bool {
	return query.Limit < 1 || query.Limit > 100 || query.Offset < 0
}

func marketingConfigDigestValue(value []byte) ([32]byte, bool) {
	var result [32]byte
	if len(value) != len(result) {
		return result, false
	}
	copy(result[:], value)
	return result, result != ([32]byte{})
}
func marketingConfigTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func marketingConfigTimeValue(value pgtype.Timestamptz) (time.Time, bool) {
	if !value.Valid || value.InfinityModifier != pgtype.Finite {
		return time.Time{}, false
	}
	return marketingConfigStoreTime(value.Time), true
}
func marketingConfigInt8(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}
func marketingConfigInt64Value(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}
func marketingConfigStoreTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}
func copyMarketingConfigStoreInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
func nilMarketingConfigHistoryDB(value automationdb.DBTX) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}
func marketingConfigHistoryDBError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return automation.ErrMarketingConfigHistoryUnavailable
	}
	var postgres *pgconn.PgError
	if errors.As(err, &postgres) && strings.HasPrefix(postgres.Code, "23") {
		return automation.ErrMarketingConfigHistoryConflict
	}
	return automation.ErrMarketingConfigHistoryUnavailable
}
