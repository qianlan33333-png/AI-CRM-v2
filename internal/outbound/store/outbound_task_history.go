package store

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
	outbounddb "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type OutboundTaskHistoryStore struct{}
type OutboundTaskHistoryReader struct{ db outbounddb.DBTX }

var _ outboundport.OutboundTaskHistoryStore = (*OutboundTaskHistoryStore)(nil)
var _ outboundport.OutboundTaskHistoryReader = (*OutboundTaskHistoryReader)(nil)

func NewOutboundTaskHistoryStore() *OutboundTaskHistoryStore { return &OutboundTaskHistoryStore{} }
func NewOutboundTaskHistoryReader(db outbounddb.DBTX) *OutboundTaskHistoryReader {
	return &OutboundTaskHistoryReader{db: db}
}

func (store *OutboundTaskHistoryStore) queries(ctx context.Context) (*outbounddb.Queries, error) {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return nil, outboundport.ErrOutboundTaskHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, outboundport.ErrOutboundTaskHistoryUnavailable
	}
	return outbounddb.New(tx), nil
}

func (reader *OutboundTaskHistoryReader) queries(ctx context.Context) (*outbounddb.Queries, error) {
	if reader == nil || ctx == nil || ctx.Err() != nil {
		return nil, outboundport.ErrOutboundTaskHistoryUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return outbounddb.New(tx), nil
	}
	if reader.db == nil {
		return nil, outboundport.ErrOutboundTaskHistoryUnavailable
	}
	value := reflect.ValueOf(reader.db)
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return nil, outboundport.ErrOutboundTaskHistoryUnavailable
	}
	return outbounddb.New(reader.db), nil
}

func (store *OutboundTaskHistoryStore) CreateHistoricalOutboundTask(ctx context.Context, value outboundport.HistoricalOutboundTask) (outboundport.HistoricalOutboundTask, error) {
	if value.ID != 0 {
		return outboundport.HistoricalOutboundTask{}, outboundport.ErrOutboundTaskHistoryInvalid
	}
	check := value
	check.ID = 1
	if _, err := outboundapp.HistoricalOutboundTaskDigest(check); err != nil {
		return outboundport.HistoricalOutboundTask{}, outboundport.ErrOutboundTaskHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return outboundport.HistoricalOutboundTask{}, err
	}
	row, err := q.CreateHistoricalOutboundTask(ctx, outboundTaskHistoryParams(value))
	if err != nil {
		return outboundport.HistoricalOutboundTask{}, outboundTaskHistoryStoreError(err)
	}
	return outboundTaskHistoryValue(row)
}

func (store *OutboundTaskHistoryStore) GetHistoricalOutboundTask(ctx context.Context, id int64) (outboundport.HistoricalOutboundTask, error) {
	if id < 1 {
		return outboundport.HistoricalOutboundTask{}, outboundport.ErrOutboundTaskHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return outboundport.HistoricalOutboundTask{}, err
	}
	row, err := q.GetHistoricalOutboundTask(ctx, id)
	if err != nil {
		return outboundport.HistoricalOutboundTask{}, outboundTaskHistoryStoreError(err)
	}
	return outboundTaskHistoryValue(row)
}

func (store *OutboundTaskHistoryStore) LookupOutboundTaskHistoryParents(ctx context.Context, sourceID int64) ([]outboundport.OutboundTaskHistoryParent, error) {
	q, err := store.queries(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.LookupOutboundTaskHistoryParents(ctx, sourceID)
	if err != nil {
		return nil, outboundTaskHistoryStoreError(err)
	}
	values := make([]outboundport.OutboundTaskHistoryParent, 0, len(rows))
	for _, row := range rows {
		values = append(values, outboundport.OutboundTaskHistoryParent{ID: row.ID, SourceID: row.SourceID, LegacyOutboundTaskID: outboundTaskHistoryOptionalInt(row.LegacyOutboundTaskID)})
	}
	return values, nil
}

func (reader *OutboundTaskHistoryReader) GetHistoricalOutboundTask(ctx context.Context, id int64) (outboundport.HistoricalOutboundTask, error) {
	if id < 1 {
		return outboundport.HistoricalOutboundTask{}, outboundport.ErrOutboundTaskHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return outboundport.HistoricalOutboundTask{}, err
	}
	row, err := q.GetHistoricalOutboundTask(ctx, id)
	if err != nil {
		return outboundport.HistoricalOutboundTask{}, outboundTaskHistoryStoreError(err)
	}
	return outboundTaskHistoryValue(row)
}

func (reader *OutboundTaskHistoryReader) ListHistoricalOutboundTasks(ctx context.Context, query outboundport.OutboundTaskHistoryQuery) ([]outboundport.HistoricalOutboundTask, int64, error) {
	if query.Limit == 0 {
		query.Limit = 20
	}
	if query.Limit < 1 || query.Limit > 100 || query.Offset < 0 {
		return nil, 0, outboundport.ErrOutboundTaskHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalOutboundTasks(ctx)
	if err != nil {
		return nil, 0, outboundTaskHistoryStoreError(err)
	}
	rows, err := q.ListHistoricalOutboundTasks(ctx, outbounddb.ListHistoricalOutboundTasksParams{Limit: query.Limit, Offset: query.Offset})
	if err != nil {
		return nil, 0, outboundTaskHistoryStoreError(err)
	}
	values := make([]outboundport.HistoricalOutboundTask, 0, len(rows))
	for _, row := range rows {
		value, err := outboundTaskHistoryValue(row)
		if err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	return values, total, nil
}

func outboundTaskHistoryParams(value outboundport.HistoricalOutboundTask) outbounddb.CreateHistoricalOutboundTaskParams {
	return outbounddb.CreateHistoricalOutboundTaskParams{
		SourceID: value.SourceID, TaskType: value.TaskType, Status: value.Status, CreatedAt: outboundTaskHistoryTimestamp(value.CreatedAt),
		BroadcastJobHistoryID: outboundTaskHistoryInt(value.BroadcastJobHistoryID), RequestPayloadDigest: value.RequestPayloadDigest[:], ResponsePayloadDigest: value.ResponsePayloadDigest[:],
		WecomTaskIDDigest: outboundTaskHistoryDigest(value.WeComTaskIDDigest), TraceIDDigest: value.TraceIDDigest[:], LegacyBroadcastJobID: outboundTaskHistoryInt(value.LegacyBroadcastJobID),
		SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], RedactedRoots: append([]string{}, value.RedactedRoots...),
	}
}

func outboundTaskHistoryValue(row outbounddb.OutboundV1TaskHistory) (outboundport.HistoricalOutboundTask, error) {
	created, err := outboundTaskHistoryRequiredTime(row.CreatedAt)
	if err != nil {
		return outboundport.HistoricalOutboundTask{}, outboundport.ErrOutboundTaskHistoryUnavailable
	}
	if row.WecomTaskIDDigest != nil && len(row.WecomTaskIDDigest) != 32 {
		return outboundport.HistoricalOutboundTask{}, outboundport.ErrOutboundTaskHistoryUnavailable
	}
	value := outboundport.HistoricalOutboundTask{
		ID: row.ID, SourceID: row.SourceID, TaskType: row.TaskType, Status: row.Status, CreatedAt: created,
		BroadcastJobHistoryID: outboundTaskHistoryOptionalInt(row.BroadcastJobHistoryID), LegacyBroadcastJobID: outboundTaskHistoryOptionalInt(row.LegacyBroadcastJobID),
		WeComTaskIDDigest: outboundTaskHistoryOptionalDigest(row.WecomTaskIDDigest), RedactedRoots: append([]string{}, row.RedactedRoots...),
	}
	for _, digest := range []struct {
		source []byte
		target *[32]byte
	}{
		{row.RequestPayloadDigest, &value.RequestPayloadDigest}, {row.ResponsePayloadDigest, &value.ResponsePayloadDigest}, {row.TraceIDDigest, &value.TraceIDDigest},
		{row.SourceKeyDigest, &value.SourceKeyDigest}, {row.SourcePayloadDigest, &value.SourcePayloadDigest}, {row.SourceFieldDigest, &value.SourceFieldDigest},
	} {
		if len(digest.source) != 32 {
			return outboundport.HistoricalOutboundTask{}, outboundport.ErrOutboundTaskHistoryUnavailable
		}
		copy(digest.target[:], digest.source)
	}
	if _, err := outboundapp.HistoricalOutboundTaskDigest(value); err != nil {
		return outboundport.HistoricalOutboundTask{}, outboundport.ErrOutboundTaskHistoryUnavailable
	}
	return value, nil
}

func outboundTaskHistoryTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC().Truncate(time.Microsecond), Valid: true}
}

func outboundTaskHistoryInt(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func outboundTaskHistoryDigest(value *[32]byte) []byte {
	if value == nil {
		return nil
	}
	return value[:]
}

func outboundTaskHistoryOptionalInt(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	copy := value.Int64
	return &copy
}

func outboundTaskHistoryOptionalDigest(value []byte) *[32]byte {
	if value == nil {
		return nil
	}
	var digest [32]byte
	copy(digest[:], value)
	return &digest
}

func outboundTaskHistoryRequiredTime(value pgtype.Timestamptz) (time.Time, error) {
	if !value.Valid || value.InfinityModifier != pgtype.Finite {
		return time.Time{}, errors.New("invalid required time")
	}
	return value.Time.UTC().Truncate(time.Microsecond), nil
}

func outboundTaskHistoryStoreError(err error) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && (pgError.Code == "23505" || pgError.Code == "23503") {
		return outboundport.ErrOutboundTaskHistoryConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return outboundport.ErrOutboundTaskHistoryConflict
	}
	return outboundport.ErrOutboundTaskHistoryUnavailable
}
