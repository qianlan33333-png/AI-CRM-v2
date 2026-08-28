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

type InvalidSourceHistoryStore struct{}
type InvalidSourceHistoryReader struct{ db mediadb.DBTX }

var _ mediaport.InvalidSourceHistoryStore = (*InvalidSourceHistoryStore)(nil)
var _ mediaport.InvalidSourceHistoryReader = (*InvalidSourceHistoryReader)(nil)

func NewInvalidSourceHistoryStore() *InvalidSourceHistoryStore { return &InvalidSourceHistoryStore{} }
func NewInvalidSourceHistoryReader(db mediadb.DBTX) *InvalidSourceHistoryReader {
	return &InvalidSourceHistoryReader{db: db}
}

func (store *InvalidSourceHistoryStore) queries(ctx context.Context) (*mediadb.Queries, error) {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return nil, mediaport.ErrInvalidSourceHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, mediaport.ErrInvalidSourceHistoryUnavailable
	}
	return mediadb.New(tx), nil
}
func (reader *InvalidSourceHistoryReader) queries(ctx context.Context) (*mediadb.Queries, error) {
	if reader == nil || ctx == nil || ctx.Err() != nil {
		return nil, mediaport.ErrInvalidSourceHistoryUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return mediadb.New(tx), nil
	}
	if nilInvalidAssetHistoryDB(reader.db) {
		return nil, mediaport.ErrInvalidSourceHistoryUnavailable
	}
	return mediadb.New(reader.db), nil
}

func (store *InvalidSourceHistoryStore) CreateHistoricalInvalidAsset(ctx context.Context, value mediaport.HistoricalInvalidAsset) (mediaport.HistoricalInvalidAsset, error) {
	if value.ID != 0 {
		return mediaport.HistoricalInvalidAsset{}, mediaport.ErrInvalidSourceHistoryInvalid
	}
	check := value
	check.ID = 1
	if _, err := mediaapp.DigestHistoricalInvalidAsset(check); err != nil {
		return mediaport.HistoricalInvalidAsset{}, mediaport.ErrInvalidSourceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return mediaport.HistoricalInvalidAsset{}, err
	}
	row, err := queries.CreateHistoricalInvalidAsset(ctx, invalidAssetHistoryParams(value))
	if err != nil {
		return mediaport.HistoricalInvalidAsset{}, invalidAssetHistoryStoreError(err)
	}
	return invalidAssetHistoryValue(row)
}
func (store *InvalidSourceHistoryStore) GetHistoricalInvalidAsset(ctx context.Context, id int64) (mediaport.HistoricalInvalidAsset, error) {
	if id < 1 {
		return mediaport.HistoricalInvalidAsset{}, mediaport.ErrInvalidSourceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return mediaport.HistoricalInvalidAsset{}, err
	}
	row, err := queries.GetHistoricalInvalidAsset(ctx, id)
	if err != nil {
		return mediaport.HistoricalInvalidAsset{}, invalidAssetHistoryStoreError(err)
	}
	return invalidAssetHistoryValue(row)
}
func (reader *InvalidSourceHistoryReader) GetHistoricalInvalidAsset(ctx context.Context, id int64) (mediaport.HistoricalInvalidAsset, error) {
	if id < 1 {
		return mediaport.HistoricalInvalidAsset{}, mediaport.ErrInvalidSourceHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return mediaport.HistoricalInvalidAsset{}, err
	}
	row, err := queries.GetHistoricalInvalidAsset(ctx, id)
	if err != nil {
		return mediaport.HistoricalInvalidAsset{}, invalidAssetHistoryStoreError(err)
	}
	return invalidAssetHistoryValue(row)
}
func (reader *InvalidSourceHistoryReader) ListHistoricalInvalidAsset(ctx context.Context, page mediaport.InvalidSourceHistoryQuery) ([]mediaport.HistoricalInvalidAsset, int64, error) {
	if page.Limit < 1 || page.Limit > 200 || page.Offset < 0 {
		return nil, 0, mediaport.ErrInvalidSourceHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountHistoricalInvalidAsset(ctx)
	if err != nil || total < 0 {
		return nil, 0, invalidAssetHistoryStoreError(err)
	}
	rows, err := queries.ListHistoricalInvalidAsset(ctx, mediadb.ListHistoricalInvalidAssetParams{RowLimit: page.Limit, RowOffset: page.Offset})
	if err != nil {
		return nil, 0, invalidAssetHistoryStoreError(err)
	}
	items := make([]mediaport.HistoricalInvalidAsset, 0, len(rows))
	for _, row := range rows {
		value, err := invalidAssetHistoryValue(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
	}
	return items, total, nil
}

func invalidAssetHistoryParams(value mediaport.HistoricalInvalidAsset) mediadb.CreateHistoricalInvalidAssetParams {
	return mediadb.CreateHistoricalInvalidAssetParams{SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], PrivateDigest: value.PrivateDigest[:], RedactedRoots: append([]string{}, value.RedactedRoots...), Kind: value.Kind, SourceID: value.SourceID, Name: value.Name, FileName: value.FileName, MimeType: value.MIMEType, FileSize: value.FileSize, OriginalEnabled: value.OriginalEnabled, ContentDigest: value.ContentDigest[:], CreatedAt: invalidAssetHistoryTime(value.CreatedAt), UpdatedAt: invalidAssetHistoryTime(value.UpdatedAt), QuarantineReason: value.QuarantineReason}
}
func invalidAssetHistoryValue(row mediadb.MediaV1InvalidAssetHistory) (mediaport.HistoricalInvalidAsset, error) {
	if row.ID < 1 || !invalidAssetHistoryFinite(row.CreatedAt) || !invalidAssetHistoryFinite(row.UpdatedAt) {
		return mediaport.HistoricalInvalidAsset{}, mediaport.ErrInvalidSourceHistoryUnavailable
	}
	value := mediaport.HistoricalInvalidAsset{ID: row.ID, RedactedRoots: append([]string{}, row.RedactedRoots...), Kind: row.Kind, SourceID: row.SourceID, Name: row.Name, FileName: row.FileName, MIMEType: row.MimeType, FileSize: row.FileSize, OriginalEnabled: row.OriginalEnabled, CreatedAt: row.CreatedAt.Time.UTC().Truncate(time.Microsecond), UpdatedAt: row.UpdatedAt.Time.UTC().Truncate(time.Microsecond), QuarantineReason: row.QuarantineReason}
	for _, pair := range []struct {
		target *[32]byte
		source []byte
	}{{&value.SourceKeyDigest, row.SourceKeyDigest}, {&value.SourcePayloadDigest, row.SourcePayloadDigest}, {&value.SourceFieldDigest, row.SourceFieldDigest}, {&value.PrivateDigest, row.PrivateDigest}, {&value.ContentDigest, row.ContentDigest}} {
		if len(pair.source) != 32 {
			return mediaport.HistoricalInvalidAsset{}, mediaport.ErrInvalidSourceHistoryUnavailable
		}
		copy(pair.target[:], pair.source)
	}
	if _, err := mediaapp.DigestHistoricalInvalidAsset(value); err != nil {
		return mediaport.HistoricalInvalidAsset{}, mediaport.ErrInvalidSourceHistoryUnavailable
	}
	return value, nil
}
func invalidAssetHistoryTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC().Truncate(time.Microsecond), Valid: true}
}
func invalidAssetHistoryFinite(value pgtype.Timestamptz) bool {
	return value.Valid && value.InfinityModifier == pgtype.Finite
}
func invalidAssetHistoryStoreError(err error) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		return mediaport.ErrInvalidSourceHistoryConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return mediaport.ErrInvalidSourceHistoryConflict
	}
	return mediaport.ErrInvalidSourceHistoryUnavailable
}
func nilInvalidAssetHistoryDB(value mediadb.DBTX) bool {
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
