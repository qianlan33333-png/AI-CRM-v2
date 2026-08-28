package store

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
	wecomdb "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store/generated"
)

type MessageHistoryStore struct{}
type MessageHistoryReader struct{ db wecomdb.DBTX }

var _ wecomport.MessageHistoryStore = (*MessageHistoryStore)(nil)
var _ wecomport.MessageHistoryReader = (*MessageHistoryReader)(nil)

func NewMessageHistoryStore() *MessageHistoryStore { return &MessageHistoryStore{} }
func NewMessageHistoryReader(db wecomdb.DBTX) *MessageHistoryReader {
	return &MessageHistoryReader{db: db}
}

func (store *MessageHistoryStore) queries(ctx context.Context) (*wecomdb.Queries, error) {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return nil, wecomport.ErrMessageHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, wecomport.ErrMessageHistoryUnavailable
	}
	return wecomdb.New(tx), nil
}

func (reader *MessageHistoryReader) queries(ctx context.Context) (*wecomdb.Queries, error) {
	if reader == nil || reader.db == nil || ctx == nil || ctx.Err() != nil {
		return nil, wecomport.ErrMessageHistoryUnavailable
	}
	v := reflect.ValueOf(reader.db)
	if v.Kind() == reflect.Pointer && v.IsNil() {
		return nil, wecomport.ErrMessageHistoryUnavailable
	}
	return wecomdb.New(reader.db), nil
}

func (store *MessageHistoryStore) CreateHistoricalMessage(ctx context.Context, value wecomport.HistoricalMessage) (wecomport.HistoricalMessage, error) {
	if value.ID != 0 {
		return wecomport.HistoricalMessage{}, wecomport.ErrMessageHistoryInvalid
	}
	check := value
	check.ID = 1
	if _, err := wecomapp.HistoricalMessageDigest(check); err != nil {
		return wecomport.HistoricalMessage{}, err
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return wecomport.HistoricalMessage{}, err
	}
	content := pgtype.Text{}
	if value.ContentMasked != nil {
		content = pgtype.Text{String: *value.ContentMasked, Valid: true}
	}
	row, err := queries.CreateHistoricalMessage(ctx, wecomdb.CreateHistoricalMessageParams{
		SourceID: value.SourceID, Sequence: messageHistoryInt(value.Sequence), CustomerID: messageHistoryInt(value.CustomerID),
		ChatType: value.ChatType, MessageType: value.MessageType, ContentMasked: content,
		OriginalSendTime: value.OriginalSendTime, SendTimeBasis: value.SendTimeBasis, SentAt: pgTimestamp(value.SentAt),
		CreatedAt: pgTimestamp(&value.CreatedAt), SourcePayloadDigest: value.SourcePayloadDigest[:],
	})
	if err != nil {
		return wecomport.HistoricalMessage{}, messageHistoryError(err)
	}
	return messageHistoryValue(row)
}

func (store *MessageHistoryStore) GetHistoricalMessage(ctx context.Context, id int64) (wecomport.HistoricalMessage, error) {
	if id < 1 {
		return wecomport.HistoricalMessage{}, wecomport.ErrMessageHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return wecomport.HistoricalMessage{}, err
	}
	row, err := q.GetHistoricalMessage(ctx, id)
	if err != nil {
		return wecomport.HistoricalMessage{}, messageHistoryError(err)
	}
	return messageHistoryValue(row)
}

func (reader *MessageHistoryReader) GetHistoricalMessage(ctx context.Context, id int64) (wecomport.HistoricalMessage, error) {
	if id < 1 {
		return wecomport.HistoricalMessage{}, wecomport.ErrMessageHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return wecomport.HistoricalMessage{}, err
	}
	row, err := q.GetHistoricalMessage(ctx, id)
	if err != nil {
		return wecomport.HistoricalMessage{}, messageHistoryError(err)
	}
	return messageHistoryValue(row)
}

func (reader *MessageHistoryReader) ListHistoricalMessages(ctx context.Context, query wecomport.MessageHistoryQuery) ([]wecomport.HistoricalMessage, int64, error) {
	if query.Limit < 1 || query.Limit > 100 || query.Offset < 0 || (query.CustomerID != nil && *query.CustomerID < 1) || (query.ChatType != "" && query.ChatType != "private" && query.ChatType != "group") {
		return nil, 0, wecomport.ErrMessageHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	customer := messageHistoryInt(query.CustomerID)
	total, err := q.CountHistoricalMessages(ctx, wecomdb.CountHistoricalMessagesParams{CustomerID: customer, ChatType: query.ChatType})
	if err != nil {
		return nil, 0, messageHistoryError(err)
	}
	rows, err := q.ListHistoricalMessages(ctx, wecomdb.ListHistoricalMessagesParams{CustomerID: customer, ChatType: query.ChatType, RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, messageHistoryError(err)
	}
	items := make([]wecomport.HistoricalMessage, 0, len(rows))
	for _, row := range rows {
		value, err := messageHistoryValue(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
	}
	return items, total, nil
}

func messageHistoryInt(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func messageHistoryValue(row wecomdb.WecomV1MessageHistory) (wecomport.HistoricalMessage, error) {
	if !row.CreatedAt.Valid || row.CreatedAt.InfinityModifier != pgtype.Finite || len(row.SourcePayloadDigest) != 32 || (row.SentAt.Valid && row.SentAt.InfinityModifier != pgtype.Finite) {
		return wecomport.HistoricalMessage{}, wecomport.ErrMessageHistoryUnavailable
	}
	value := wecomport.HistoricalMessage{ID: row.ID, SourceID: row.SourceID, ChatType: row.ChatType, MessageType: row.MessageType,
		OriginalSendTime: row.OriginalSendTime, SendTimeBasis: row.SendTimeBasis, CreatedAt: row.CreatedAt.Time.UTC().Truncate(time.Microsecond)}
	copy(value.SourcePayloadDigest[:], row.SourcePayloadDigest)
	if row.Sequence.Valid {
		value.Sequence = &row.Sequence.Int64
	}
	if row.CustomerID.Valid {
		value.CustomerID = &row.CustomerID.Int64
	}
	if row.ContentMasked.Valid {
		value.ContentMasked = &row.ContentMasked.String
	}
	if row.SentAt.Valid {
		at := row.SentAt.Time.UTC().Truncate(time.Microsecond)
		value.SentAt = &at
	}
	if _, err := wecomapp.HistoricalMessageDigest(value); err != nil {
		return wecomport.HistoricalMessage{}, wecomport.ErrMessageHistoryUnavailable
	}
	return value, nil
}

func messageHistoryError(err error) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		return wecomport.ErrMessageHistoryConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return wecomport.ErrMessageHistoryConflict
	}
	return wecomport.ErrMessageHistoryUnavailable
}
