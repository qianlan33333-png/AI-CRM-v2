package store

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type InvalidSourceHistoryStore struct{}
type InvalidSourceHistoryReader struct{ db contactdb.DBTX }

var _ contact.InvalidSourceHistoryStore = (*InvalidSourceHistoryStore)(nil)
var _ contact.InvalidSourceHistoryReader = (*InvalidSourceHistoryReader)(nil)

func NewInvalidSourceHistoryStore() *InvalidSourceHistoryStore { return &InvalidSourceHistoryStore{} }
func NewInvalidSourceHistoryReader(db contactdb.DBTX) *InvalidSourceHistoryReader {
	return &InvalidSourceHistoryReader{db: db}
}

func (store *InvalidSourceHistoryStore) queries(ctx context.Context) (*contactdb.Queries, error) {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return nil, contact.ErrInvalidSourceHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, contact.ErrInvalidSourceHistoryUnavailable
	}
	return contactdb.New(tx), nil
}
func (reader *InvalidSourceHistoryReader) queries(ctx context.Context) (*contactdb.Queries, error) {
	if reader == nil || ctx == nil || ctx.Err() != nil {
		return nil, contact.ErrInvalidSourceHistoryUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return contactdb.New(tx), nil
	}
	if invalidSourceHistoryNilDB(reader.db) {
		return nil, contact.ErrInvalidSourceHistoryUnavailable
	}
	return contactdb.New(reader.db), nil
}

func (store *InvalidSourceHistoryStore) CreateHistoricalUnboundTag(ctx context.Context, value contact.HistoricalUnboundTag) (contact.HistoricalUnboundTag, error) {
	if value.ID != 0 || !invalidSourceHistoryStoreValidUnboundTag(value) {
		return contact.HistoricalUnboundTag{}, contact.ErrInvalidSourceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return contact.HistoricalUnboundTag{}, err
	}
	row, err := queries.CreateHistoricalUnboundTag(ctx, contactdb.CreateHistoricalUnboundTagParams{SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], PrivateDigest: value.PrivateDigest[:], RedactedRoots: value.RedactedRoots, TagSourceID: value.TagSourceID, UnionIDDigest: value.UnionIDDigest[:], CreatedAt: invalidSourceHistoryTimestamp(value.CreatedAt), QuarantineReason: value.QuarantineReason})
	if err != nil {
		return contact.HistoricalUnboundTag{}, invalidSourceHistoryStoreError(err)
	}
	return invalidSourceHistoryUnboundTagValue(row)
}
func (store *InvalidSourceHistoryStore) GetHistoricalUnboundTag(ctx context.Context, id int64) (contact.HistoricalUnboundTag, error) {
	if id < 1 {
		return contact.HistoricalUnboundTag{}, contact.ErrInvalidSourceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return contact.HistoricalUnboundTag{}, err
	}
	row, err := queries.GetHistoricalUnboundTag(ctx, id)
	if err != nil {
		return contact.HistoricalUnboundTag{}, invalidSourceHistoryStoreError(err)
	}
	return invalidSourceHistoryUnboundTagValue(row)
}
func (reader *InvalidSourceHistoryReader) GetHistoricalUnboundTag(ctx context.Context, id int64) (contact.HistoricalUnboundTag, error) {
	if id < 1 {
		return contact.HistoricalUnboundTag{}, contact.ErrInvalidSourceHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return contact.HistoricalUnboundTag{}, err
	}
	row, err := queries.GetHistoricalUnboundTag(ctx, id)
	if err != nil {
		return contact.HistoricalUnboundTag{}, invalidSourceHistoryStoreError(err)
	}
	return invalidSourceHistoryUnboundTagValue(row)
}
func (reader *InvalidSourceHistoryReader) ListHistoricalUnboundTag(ctx context.Context, query contact.InvalidSourceHistoryQuery) ([]contact.HistoricalUnboundTag, int64, error) {
	if invalidSourceHistoryBadQuery(query) {
		return nil, 0, contact.ErrInvalidSourceHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountHistoricalUnboundTag(ctx)
	if err != nil {
		return nil, 0, invalidSourceHistoryStoreError(err)
	}
	rows, err := queries.ListHistoricalUnboundTag(ctx, contactdb.ListHistoricalUnboundTagParams{RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, invalidSourceHistoryStoreError(err)
	}
	items := make([]contact.HistoricalUnboundTag, 0, len(rows))
	for _, row := range rows {
		value, valueErr := invalidSourceHistoryUnboundTagValue(row)
		if valueErr != nil {
			return nil, 0, valueErr
		}
		items = append(items, value)
	}
	return items, total, nil
}

func (store *InvalidSourceHistoryStore) CreateHistoricalInvalidChannel(ctx context.Context, value contact.HistoricalInvalidChannel) (contact.HistoricalInvalidChannel, error) {
	if value.ID != 0 || !invalidSourceHistoryStoreValidInvalidChannel(value) {
		return contact.HistoricalInvalidChannel{}, contact.ErrInvalidSourceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return contact.HistoricalInvalidChannel{}, err
	}
	row, err := queries.CreateHistoricalInvalidChannel(ctx, contactdb.CreateHistoricalInvalidChannelParams{SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], PrivateDigest: value.PrivateDigest[:], RedactedRoots: value.RedactedRoots, SourceID: value.SourceID, Code: value.Code, Name: value.Name, ChannelType: value.ChannelType, CarrierType: value.CarrierType, CreatedAt: invalidSourceHistoryTimestamp(value.CreatedAt), UpdatedAt: invalidSourceHistoryTimestamp(value.UpdatedAt), QuarantineReason: value.QuarantineReason})
	if err != nil {
		return contact.HistoricalInvalidChannel{}, invalidSourceHistoryStoreError(err)
	}
	return invalidSourceHistoryInvalidChannelValue(row)
}
func (store *InvalidSourceHistoryStore) GetHistoricalInvalidChannel(ctx context.Context, id int64) (contact.HistoricalInvalidChannel, error) {
	if id < 1 {
		return contact.HistoricalInvalidChannel{}, contact.ErrInvalidSourceHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return contact.HistoricalInvalidChannel{}, err
	}
	row, err := queries.GetHistoricalInvalidChannel(ctx, id)
	if err != nil {
		return contact.HistoricalInvalidChannel{}, invalidSourceHistoryStoreError(err)
	}
	return invalidSourceHistoryInvalidChannelValue(row)
}
func (reader *InvalidSourceHistoryReader) GetHistoricalInvalidChannel(ctx context.Context, id int64) (contact.HistoricalInvalidChannel, error) {
	if id < 1 {
		return contact.HistoricalInvalidChannel{}, contact.ErrInvalidSourceHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return contact.HistoricalInvalidChannel{}, err
	}
	row, err := queries.GetHistoricalInvalidChannel(ctx, id)
	if err != nil {
		return contact.HistoricalInvalidChannel{}, invalidSourceHistoryStoreError(err)
	}
	return invalidSourceHistoryInvalidChannelValue(row)
}
func (reader *InvalidSourceHistoryReader) ListHistoricalInvalidChannel(ctx context.Context, query contact.InvalidSourceHistoryQuery) ([]contact.HistoricalInvalidChannel, int64, error) {
	if invalidSourceHistoryBadQuery(query) {
		return nil, 0, contact.ErrInvalidSourceHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountHistoricalInvalidChannel(ctx)
	if err != nil {
		return nil, 0, invalidSourceHistoryStoreError(err)
	}
	rows, err := queries.ListHistoricalInvalidChannel(ctx, contactdb.ListHistoricalInvalidChannelParams{RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, invalidSourceHistoryStoreError(err)
	}
	items := make([]contact.HistoricalInvalidChannel, 0, len(rows))
	for _, row := range rows {
		value, valueErr := invalidSourceHistoryInvalidChannelValue(row)
		if valueErr != nil {
			return nil, 0, valueErr
		}
		items = append(items, value)
	}
	return items, total, nil
}

func invalidSourceHistoryUnboundTagValue(row contactdb.ContactV1UnboundTagHistory) (contact.HistoricalUnboundTag, error) {
	key, payload, field, private, union, ok := invalidSourceHistoryDigests(row.SourceKeyDigest, row.SourcePayloadDigest, row.SourceFieldDigest, row.PrivateDigest, row.UnionIDDigest)
	created, validTime := invalidSourceHistoryStoredTime(row.CreatedAt)
	value := contact.HistoricalUnboundTag{ID: row.ID, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, PrivateDigest: private, RedactedRoots: invalidSourceHistoryStoredRoots(row.RedactedRoots), TagSourceID: row.TagSourceID, UnionIDDigest: union, CreatedAt: created, QuarantineReason: row.QuarantineReason}
	if !ok || !validTime {
		return contact.HistoricalUnboundTag{}, contact.ErrInvalidSourceHistoryUnavailable
	}
	if _, err := contactapp.DigestHistoricalUnboundTag(value); err != nil {
		return contact.HistoricalUnboundTag{}, contact.ErrInvalidSourceHistoryUnavailable
	}
	return value, nil
}
func invalidSourceHistoryInvalidChannelValue(row contactdb.ContactV1InvalidChannelHistory) (contact.HistoricalInvalidChannel, error) {
	key, payload, field, private, ok := invalidSourceHistoryDigests4(row.SourceKeyDigest, row.SourcePayloadDigest, row.SourceFieldDigest, row.PrivateDigest)
	created, createdOK := invalidSourceHistoryStoredTime(row.CreatedAt)
	updated, updatedOK := invalidSourceHistoryStoredTime(row.UpdatedAt)
	value := contact.HistoricalInvalidChannel{ID: row.ID, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, PrivateDigest: private, RedactedRoots: invalidSourceHistoryStoredRoots(row.RedactedRoots), SourceID: row.SourceID, Code: row.Code, Name: row.Name, ChannelType: row.ChannelType, CarrierType: row.CarrierType, CreatedAt: created, UpdatedAt: updated, QuarantineReason: row.QuarantineReason}
	if !ok || !createdOK || !updatedOK {
		return contact.HistoricalInvalidChannel{}, contact.ErrInvalidSourceHistoryUnavailable
	}
	if _, err := contactapp.DigestHistoricalInvalidChannel(value); err != nil {
		return contact.HistoricalInvalidChannel{}, contact.ErrInvalidSourceHistoryUnavailable
	}
	return value, nil
}
func invalidSourceHistoryStoreValidUnboundTag(value contact.HistoricalUnboundTag) bool {
	value.ID = 1
	_, err := contactapp.DigestHistoricalUnboundTag(value)
	return err == nil
}
func invalidSourceHistoryStoreValidInvalidChannel(value contact.HistoricalInvalidChannel) bool {
	value.ID = 1
	_, err := contactapp.DigestHistoricalInvalidChannel(value)
	return err == nil
}
func invalidSourceHistoryTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func invalidSourceHistoryStoredTime(value pgtype.Timestamptz) (time.Time, bool) {
	return value.Time.UTC().Truncate(time.Microsecond), value.Valid && value.InfinityModifier == pgtype.Finite
}
func invalidSourceHistoryStoredRoots(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string(nil), values...)
}
func invalidSourceHistoryBadQuery(query contact.InvalidSourceHistoryQuery) bool {
	return query.Limit < 1 || query.Limit > 200 || query.Offset < 0
}
func invalidSourceHistoryNilDB(value contactdb.DBTX) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return (v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface) && v.IsNil()
}
func invalidSourceHistoryStoreError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return contact.ErrInvalidSourceHistoryConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return contact.ErrInvalidSourceHistoryConflict
	}
	return contact.ErrInvalidSourceHistoryUnavailable
}
func invalidSourceHistoryDigests(values ...[]byte) ([32]byte, [32]byte, [32]byte, [32]byte, [32]byte, bool) {
	var one, two, three, four, five [32]byte
	if len(values) != 5 {
		return one, two, three, four, five, false
	}
	all := [][32]byte{one, two, three, four, five}
	for index, value := range values {
		if len(value) != 32 {
			return one, two, three, four, five, false
		}
		copy(all[index][:], value)
		if all[index] == ([32]byte{}) {
			return one, two, three, four, five, false
		}
	}
	return all[0], all[1], all[2], all[3], all[4], true
}
func invalidSourceHistoryDigests4(values ...[]byte) ([32]byte, [32]byte, [32]byte, [32]byte, bool) {
	var one, two, three, four [32]byte
	if len(values) != 4 {
		return one, two, three, four, false
	}
	all := [][32]byte{one, two, three, four}
	for index, value := range values {
		if len(value) != 32 {
			return one, two, three, four, false
		}
		copy(all[index][:], value)
		if all[index] == ([32]byte{}) {
			return one, two, three, four, false
		}
	}
	return all[0], all[1], all[2], all[3], true
}
