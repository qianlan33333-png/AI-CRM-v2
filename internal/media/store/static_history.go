package store

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type StaticMediaHistoryStore struct{}
type StaticMediaHistoryReader struct{ db mediadb.DBTX }

var _ mediaport.StaticMediaHistoryStore = (*StaticMediaHistoryStore)(nil)
var _ mediaport.StaticMediaHistoryReader = (*StaticMediaHistoryReader)(nil)

func NewStaticMediaHistoryStore() *StaticMediaHistoryStore { return &StaticMediaHistoryStore{} }
func NewStaticMediaHistoryReader(db mediadb.DBTX) *StaticMediaHistoryReader {
	return &StaticMediaHistoryReader{db: db}
}

func (store *StaticMediaHistoryStore) queries(ctx context.Context) (*mediadb.Queries, error) {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return nil, mediaport.ErrStaticMediaHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, mediaport.ErrStaticMediaHistoryUnavailable
	}
	return mediadb.New(tx), nil
}

func (reader *StaticMediaHistoryReader) queries(ctx context.Context) (*mediadb.Queries, error) {
	if reader == nil || ctx == nil || ctx.Err() != nil {
		return nil, mediaport.ErrStaticMediaHistoryUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil && tx != nil {
		return mediadb.New(tx), nil
	}
	if reader.db == nil {
		return nil, mediaport.ErrStaticMediaHistoryUnavailable
	}
	v := reflect.ValueOf(reader.db)
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return nil, mediaport.ErrStaticMediaHistoryUnavailable
	}
	return mediadb.New(reader.db), nil
}

func (store *StaticMediaHistoryStore) CreateHistoricalGroupInvite(ctx context.Context, value mediaport.HistoricalGroupInvite) (mediaport.HistoricalGroupInvite, error) {
	if value.ID != 0 {
		return mediaport.HistoricalGroupInvite{}, mediaport.ErrStaticMediaHistoryInvalid
	}
	check := value
	check.ID = 1
	if _, err := mediaapp.HistoricalGroupInviteDigest(check); err != nil {
		return mediaport.HistoricalGroupInvite{}, mediaport.ErrStaticMediaHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return mediaport.HistoricalGroupInvite{}, err
	}
	row, err := queries.CreateHistoricalGroupInvite(ctx, mediadb.CreateHistoricalGroupInviteParams{
		SourceID: value.SourceID, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:],
		Name: value.Name, Title: value.Title, Description: value.Description, OriginalState: value.OriginalState,
		OriginalAutoCreate: value.OriginalAutoCreate, RoomBaseName: value.RoomBaseName, RoomBaseSourceID: staticMediaHistoryInt(value.RoomBaseSourceID),
		OriginalEnabled: value.OriginalEnabled, OriginalBindingState: value.OriginalBindingState,
		CreatedAt: staticMediaHistoryTimestamp(value.CreatedAt), UpdatedAt: staticMediaHistoryTimestamp(value.UpdatedAt),
	})
	if err != nil {
		return mediaport.HistoricalGroupInvite{}, staticMediaHistoryStoreError(err)
	}
	return staticMediaHistoryValue(row)
}

func (store *StaticMediaHistoryStore) GetHistoricalGroupInvite(ctx context.Context, id int64) (mediaport.HistoricalGroupInvite, error) {
	if id < 1 {
		return mediaport.HistoricalGroupInvite{}, mediaport.ErrStaticMediaHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return mediaport.HistoricalGroupInvite{}, err
	}
	row, err := queries.GetHistoricalGroupInvite(ctx, id)
	if err != nil {
		return mediaport.HistoricalGroupInvite{}, staticMediaHistoryStoreError(err)
	}
	return staticMediaHistoryValue(row)
}

func (reader *StaticMediaHistoryReader) GetHistoricalGroupInvite(ctx context.Context, id int64) (mediaport.HistoricalGroupInvite, error) {
	if id < 1 {
		return mediaport.HistoricalGroupInvite{}, mediaport.ErrStaticMediaHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return mediaport.HistoricalGroupInvite{}, err
	}
	row, err := queries.GetHistoricalGroupInvite(ctx, id)
	if err != nil {
		return mediaport.HistoricalGroupInvite{}, staticMediaHistoryStoreError(err)
	}
	return staticMediaHistoryValue(row)
}

func (reader *StaticMediaHistoryReader) ListHistoricalGroupInvite(ctx context.Context, query mediaport.StaticMediaHistoryQuery) ([]mediaport.HistoricalGroupInvite, int64, error) {
	if query.Limit < 1 || query.Limit > 100 || query.Offset < 0 {
		return nil, 0, mediaport.ErrStaticMediaHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountHistoricalGroupInvite(ctx)
	if err != nil || total < 0 {
		return nil, 0, staticMediaHistoryStoreError(err)
	}
	rows, err := queries.ListHistoricalGroupInvite(ctx, mediadb.ListHistoricalGroupInviteParams{RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, staticMediaHistoryStoreError(err)
	}
	items := make([]mediaport.HistoricalGroupInvite, 0, len(rows))
	for _, row := range rows {
		item, err := staticMediaHistoryValue(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, nil
}

func staticMediaHistoryValue(row mediadb.MediaV1GroupInviteHistory) (mediaport.HistoricalGroupInvite, error) {
	if row.ID < 1 || len(row.SourceKeyDigest) != 32 || len(row.SourcePayloadDigest) != 32 || !staticMediaHistoryFinite(row.CreatedAt) || !staticMediaHistoryFinite(row.UpdatedAt) {
		return mediaport.HistoricalGroupInvite{}, mediaport.ErrStaticMediaHistoryUnavailable
	}
	value := mediaport.HistoricalGroupInvite{ID: row.ID, SourceID: row.SourceID, Name: row.Name, Title: row.Title, Description: row.Description,
		OriginalState: row.OriginalState, OriginalAutoCreate: row.OriginalAutoCreate, RoomBaseName: row.RoomBaseName, OriginalEnabled: row.OriginalEnabled,
		OriginalBindingState: row.OriginalBindingState, CreatedAt: row.CreatedAt.Time.UTC().Truncate(time.Microsecond), UpdatedAt: row.UpdatedAt.Time.UTC().Truncate(time.Microsecond)}
	copy(value.SourceKeyDigest[:], row.SourceKeyDigest)
	copy(value.SourcePayloadDigest[:], row.SourcePayloadDigest)
	if row.RoomBaseSourceID.Valid {
		id := row.RoomBaseSourceID.Int64
		value.RoomBaseSourceID = &id
	}
	if _, err := mediaapp.HistoricalGroupInviteDigest(value); err != nil {
		return mediaport.HistoricalGroupInvite{}, mediaport.ErrStaticMediaHistoryUnavailable
	}
	return value, nil
}

func staticMediaHistoryInt(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}
func staticMediaHistoryTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func staticMediaHistoryFinite(value pgtype.Timestamptz) bool {
	return value.Valid && value.InfinityModifier == pgtype.Finite
}
func staticMediaHistoryStoreError(err error) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		return mediaport.ErrStaticMediaHistoryConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return mediaport.ErrStaticMediaHistoryConflict
	}
	return mediaport.ErrStaticMediaHistoryUnavailable
}
