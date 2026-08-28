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
	radarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/app"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
	radardb "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/store/generated"
)

type InvalidSourceHistoryStore struct{}
type InvalidSourceHistoryReader struct{ db radardb.DBTX }

var _ radarport.InvalidSourceHistoryStore = (*InvalidSourceHistoryStore)(nil)
var _ radarport.InvalidSourceHistoryReader = (*InvalidSourceHistoryReader)(nil)

func NewInvalidSourceHistoryStore() *InvalidSourceHistoryStore { return &InvalidSourceHistoryStore{} }
func NewInvalidSourceHistoryReader(db radardb.DBTX) *InvalidSourceHistoryReader {
	return &InvalidSourceHistoryReader{db: db}
}
func (store *InvalidSourceHistoryStore) queries(ctx context.Context) (*radardb.Queries, error) {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return nil, radarport.ErrInvalidSourceHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, radarport.ErrInvalidSourceHistoryUnavailable
	}
	return radardb.New(tx), nil
}
func (reader *InvalidSourceHistoryReader) queries(ctx context.Context) (*radardb.Queries, error) {
	if reader == nil || ctx == nil || ctx.Err() != nil {
		return nil, radarport.ErrInvalidSourceHistoryUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return radardb.New(tx), nil
	}
	if nilInvalidRadarLinkHistoryDB(reader.db) {
		return nil, radarport.ErrInvalidSourceHistoryUnavailable
	}
	return radardb.New(reader.db), nil
}
func (store *InvalidSourceHistoryStore) CreateHistoricalInvalidRadarLink(ctx context.Context, value radarport.HistoricalInvalidRadarLink) (radarport.HistoricalInvalidRadarLink, error) {
	if value.ID != 0 {
		return radarport.HistoricalInvalidRadarLink{}, radarport.ErrInvalidSourceHistoryInvalid
	}
	check := value
	check.ID = 1
	if _, err := radarapp.DigestHistoricalInvalidRadarLink(check); err != nil {
		return radarport.HistoricalInvalidRadarLink{}, radarport.ErrInvalidSourceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return radarport.HistoricalInvalidRadarLink{}, err
	}
	row, err := queries.CreateHistoricalInvalidRadarLink(ctx, invalidRadarLinkHistoryParams(value))
	if err != nil {
		return radarport.HistoricalInvalidRadarLink{}, invalidRadarLinkHistoryStoreError(err)
	}
	return invalidRadarLinkHistoryValue(row)
}
func (store *InvalidSourceHistoryStore) GetHistoricalInvalidRadarLink(ctx context.Context, id int64) (radarport.HistoricalInvalidRadarLink, error) {
	if id < 1 {
		return radarport.HistoricalInvalidRadarLink{}, radarport.ErrInvalidSourceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return radarport.HistoricalInvalidRadarLink{}, err
	}
	row, err := queries.GetHistoricalInvalidRadarLink(ctx, id)
	if err != nil {
		return radarport.HistoricalInvalidRadarLink{}, invalidRadarLinkHistoryStoreError(err)
	}
	return invalidRadarLinkHistoryValue(row)
}
func (reader *InvalidSourceHistoryReader) GetHistoricalInvalidRadarLink(ctx context.Context, id int64) (radarport.HistoricalInvalidRadarLink, error) {
	if id < 1 {
		return radarport.HistoricalInvalidRadarLink{}, radarport.ErrInvalidSourceHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return radarport.HistoricalInvalidRadarLink{}, err
	}
	row, err := queries.GetHistoricalInvalidRadarLink(ctx, id)
	if err != nil {
		return radarport.HistoricalInvalidRadarLink{}, invalidRadarLinkHistoryStoreError(err)
	}
	return invalidRadarLinkHistoryValue(row)
}
func (reader *InvalidSourceHistoryReader) ListHistoricalInvalidRadarLink(ctx context.Context, page radarport.InvalidSourceHistoryQuery) ([]radarport.HistoricalInvalidRadarLink, int64, error) {
	if page.Limit < 1 || page.Limit > 200 || page.Offset < 0 {
		return nil, 0, radarport.ErrInvalidSourceHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountHistoricalInvalidRadarLink(ctx)
	if err != nil || total < 0 {
		return nil, 0, invalidRadarLinkHistoryStoreError(err)
	}
	rows, err := queries.ListHistoricalInvalidRadarLink(ctx, radardb.ListHistoricalInvalidRadarLinkParams{RowLimit: page.Limit, RowOffset: page.Offset})
	if err != nil {
		return nil, 0, invalidRadarLinkHistoryStoreError(err)
	}
	items := make([]radarport.HistoricalInvalidRadarLink, 0, len(rows))
	for _, row := range rows {
		value, err := invalidRadarLinkHistoryValue(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
	}
	return items, total, nil
}
func invalidRadarLinkHistoryParams(value radarport.HistoricalInvalidRadarLink) radardb.CreateHistoricalInvalidRadarLinkParams {
	return radardb.CreateHistoricalInvalidRadarLinkParams{SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], PrivateDigest: value.PrivateDigest[:], RedactedRoots: append([]string{}, value.RedactedRoots...), SourceID: value.SourceID, Code: value.Code, Title: value.Title, DestinationUrlDigest: value.DestinationURLDigest[:], CreatedAt: invalidRadarLinkHistoryTime(value.CreatedAt), UpdatedAt: invalidRadarLinkHistoryTime(value.UpdatedAt), QuarantineReason: value.QuarantineReason}
}
func invalidRadarLinkHistoryValue(row radardb.RadarV1InvalidLinkHistory) (radarport.HistoricalInvalidRadarLink, error) {
	if row.ID < 1 || !invalidRadarLinkHistoryFinite(row.CreatedAt) || !invalidRadarLinkHistoryFinite(row.UpdatedAt) {
		return radarport.HistoricalInvalidRadarLink{}, radarport.ErrInvalidSourceHistoryUnavailable
	}
	value := radarport.HistoricalInvalidRadarLink{ID: row.ID, RedactedRoots: append([]string{}, row.RedactedRoots...), SourceID: row.SourceID, Code: row.Code, Title: row.Title, CreatedAt: row.CreatedAt.Time.UTC().Truncate(time.Microsecond), UpdatedAt: row.UpdatedAt.Time.UTC().Truncate(time.Microsecond), QuarantineReason: row.QuarantineReason}
	for _, pair := range []struct {
		target *[32]byte
		source []byte
	}{{&value.SourceKeyDigest, row.SourceKeyDigest}, {&value.SourcePayloadDigest, row.SourcePayloadDigest}, {&value.SourceFieldDigest, row.SourceFieldDigest}, {&value.PrivateDigest, row.PrivateDigest}, {&value.DestinationURLDigest, row.DestinationUrlDigest}} {
		if len(pair.source) != 32 {
			return radarport.HistoricalInvalidRadarLink{}, radarport.ErrInvalidSourceHistoryUnavailable
		}
		copy(pair.target[:], pair.source)
	}
	if _, err := radarapp.DigestHistoricalInvalidRadarLink(value); err != nil {
		return radarport.HistoricalInvalidRadarLink{}, radarport.ErrInvalidSourceHistoryUnavailable
	}
	return value, nil
}
func invalidRadarLinkHistoryTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC().Truncate(time.Microsecond), Valid: true}
}
func invalidRadarLinkHistoryFinite(value pgtype.Timestamptz) bool {
	return value.Valid && value.InfinityModifier == pgtype.Finite
}
func invalidRadarLinkHistoryStoreError(err error) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		return radarport.ErrInvalidSourceHistoryConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return radarport.ErrInvalidSourceHistoryConflict
	}
	return radarport.ErrInvalidSourceHistoryUnavailable
}
func nilInvalidRadarLinkHistoryDB(value radardb.DBTX) bool {
	if value == nil {
		return true
	}
	current := reflect.ValueOf(value)
	switch current.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return current.IsNil()
	}
	return false
}
