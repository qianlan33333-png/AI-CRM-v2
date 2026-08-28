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

type RadarClickHistoryStore struct{}
type RadarClickHistoryReader struct{ db radardb.DBTX }

var _ radarport.RadarClickHistoryStore = (*RadarClickHistoryStore)(nil)
var _ radarport.RadarClickHistoryReader = (*RadarClickHistoryReader)(nil)

func NewRadarClickHistoryStore() *RadarClickHistoryStore { return &RadarClickHistoryStore{} }
func NewRadarClickHistoryReader(db radardb.DBTX) *RadarClickHistoryReader {
	return &RadarClickHistoryReader{db: db}
}

func (store *RadarClickHistoryStore) queries(ctx context.Context) (*radardb.Queries, error) {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return nil, radarport.ErrRadarClickHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, radarport.ErrRadarClickHistoryUnavailable
	}
	return radardb.New(tx), nil
}

func (reader *RadarClickHistoryReader) queries(ctx context.Context) (*radardb.Queries, error) {
	if reader == nil || ctx == nil || ctx.Err() != nil {
		return nil, radarport.ErrRadarClickHistoryUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return radardb.New(tx), nil
	}
	if nilRadarClickHistoryDB(reader.db) {
		return nil, radarport.ErrRadarClickHistoryUnavailable
	}
	return radardb.New(reader.db), nil
}

func (store *RadarClickHistoryStore) CreateHistoricalRadarClick(ctx context.Context, value radarport.HistoricalRadarClick) (radarport.HistoricalRadarClick, error) {
	if value.ID != 0 {
		return radarport.HistoricalRadarClick{}, radarport.ErrRadarClickHistoryInvalid
	}
	check := value
	check.ID = 1
	if _, err := radarapp.HistoricalRadarClickDigest(check); err != nil {
		return radarport.HistoricalRadarClick{}, radarport.ErrRadarClickHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return radarport.HistoricalRadarClick{}, err
	}
	row, err := queries.CreateHistoricalRadarClick(ctx, radarClickHistoryParams(value))
	if err != nil {
		return radarport.HistoricalRadarClick{}, radarClickHistoryStoreError(err)
	}
	return radarClickHistoryValue(row)
}

func (store *RadarClickHistoryStore) GetHistoricalRadarClick(ctx context.Context, id int64) (radarport.HistoricalRadarClick, error) {
	if id < 1 {
		return radarport.HistoricalRadarClick{}, radarport.ErrRadarClickHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return radarport.HistoricalRadarClick{}, err
	}
	row, err := queries.GetHistoricalRadarClick(ctx, id)
	if err != nil {
		return radarport.HistoricalRadarClick{}, radarClickHistoryStoreError(err)
	}
	return radarClickHistoryValue(row)
}

func (reader *RadarClickHistoryReader) GetHistoricalRadarClick(ctx context.Context, id int64) (radarport.HistoricalRadarClick, error) {
	if id < 1 {
		return radarport.HistoricalRadarClick{}, radarport.ErrRadarClickHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return radarport.HistoricalRadarClick{}, err
	}
	row, err := queries.GetHistoricalRadarClick(ctx, id)
	if err != nil {
		return radarport.HistoricalRadarClick{}, radarClickHistoryStoreError(err)
	}
	return radarClickHistoryValue(row)
}

func (reader *RadarClickHistoryReader) ListHistoricalRadarClick(ctx context.Context, page radarport.RadarClickHistoryQuery) ([]radarport.HistoricalRadarClick, int64, error) {
	if page.Limit < 1 || page.Limit > 100 || page.Offset < 0 {
		return nil, 0, radarport.ErrRadarClickHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountHistoricalRadarClick(ctx)
	if err != nil {
		return nil, 0, radarClickHistoryStoreError(err)
	}
	rows, err := queries.ListHistoricalRadarClick(ctx, radardb.ListHistoricalRadarClickParams{RowLimit: page.Limit, RowOffset: page.Offset})
	if err != nil {
		return nil, 0, radarClickHistoryStoreError(err)
	}
	items := make([]radarport.HistoricalRadarClick, 0, len(rows))
	for _, row := range rows {
		value, err := radarClickHistoryValue(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
	}
	return items, total, nil
}

func radarClickHistoryParams(value radarport.HistoricalRadarClick) radardb.CreateHistoricalRadarClickParams {
	return radardb.CreateHistoricalRadarClickParams{SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:],
		SourceID: value.SourceID, LinkSourceID: value.LinkSourceID, RadarLinkID: radarClickHistoryInt(value.RadarLinkID), CustomerID: radarClickHistoryInt(value.CustomerID),
		Code: value.Code, RawStage: value.RawStage, SourceChannel: value.SourceChannel, TargetTypeSnapshot: value.TargetTypeSnapshot, SourceChannelSnapshot: value.SourceChannelSnapshot,
		ErrorCode: value.ErrorCode, CreatedAt: radarClickHistoryTime(value.CreatedAt), OpenIDDigest: value.OpenIDDigest[:], UnionIDDigest: value.UnionIDDigest[:],
		ExternalUserIDDigest: value.ExternalUserIDDigest[:], CampaignIDDigest: value.CampaignIDDigest[:], StaffIDDigest: value.StaffIDDigest[:], UserAgentDigest: value.UserAgentDigest[:],
		IpDigest: value.IPDigest[:], PersonIDDigest: value.PersonIDDigest[:], IpHashDigest: value.IPHashDigest[:], CampaignSnapshotDigest: value.CampaignSnapshotDigest[:],
		StaffSnapshotDigest: value.StaffSnapshotDigest[:], RefererDigest: value.RefererDigest[:], QueryParamsDigest: value.QueryParamsDigest[:]}
}

func radarClickHistoryValue(row radardb.RadarV1ClickHistory) (radarport.HistoricalRadarClick, error) {
	if row.ID < 1 || !row.CreatedAt.Valid || row.CreatedAt.InfinityModifier != pgtype.Finite {
		return radarport.HistoricalRadarClick{}, radarport.ErrRadarClickHistoryUnavailable
	}
	value := radarport.HistoricalRadarClick{ID: row.ID, SourceID: row.SourceID, LinkSourceID: row.LinkSourceID, Code: row.Code, RawStage: row.RawStage,
		SourceChannel: row.SourceChannel, TargetTypeSnapshot: row.TargetTypeSnapshot, SourceChannelSnapshot: row.SourceChannelSnapshot, ErrorCode: row.ErrorCode,
		CreatedAt: row.CreatedAt.Time.UTC().Truncate(time.Microsecond)}
	for _, pair := range []struct {
		target *[32]byte
		source []byte
	}{
		{&value.SourceKeyDigest, row.SourceKeyDigest}, {&value.SourcePayloadDigest, row.SourcePayloadDigest}, {&value.SourceFieldDigest, row.SourceFieldDigest},
		{&value.OpenIDDigest, row.OpenIDDigest}, {&value.UnionIDDigest, row.UnionIDDigest}, {&value.ExternalUserIDDigest, row.ExternalUserIDDigest},
		{&value.CampaignIDDigest, row.CampaignIDDigest}, {&value.StaffIDDigest, row.StaffIDDigest}, {&value.UserAgentDigest, row.UserAgentDigest},
		{&value.IPDigest, row.IpDigest}, {&value.PersonIDDigest, row.PersonIDDigest}, {&value.IPHashDigest, row.IpHashDigest},
		{&value.CampaignSnapshotDigest, row.CampaignSnapshotDigest}, {&value.StaffSnapshotDigest, row.StaffSnapshotDigest}, {&value.RefererDigest, row.RefererDigest}, {&value.QueryParamsDigest, row.QueryParamsDigest},
	} {
		if len(pair.source) != 32 {
			return radarport.HistoricalRadarClick{}, radarport.ErrRadarClickHistoryUnavailable
		}
		copy(pair.target[:], pair.source)
	}
	if row.RadarLinkID.Valid {
		id := row.RadarLinkID.Int64
		value.RadarLinkID = &id
	}
	if row.CustomerID.Valid {
		id := row.CustomerID.Int64
		value.CustomerID = &id
	}
	if _, err := radarapp.HistoricalRadarClickDigest(value); err != nil {
		return radarport.HistoricalRadarClick{}, radarport.ErrRadarClickHistoryUnavailable
	}
	return value, nil
}

func radarClickHistoryInt(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func radarClickHistoryTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC().Truncate(time.Microsecond), Valid: true}
}

func radarClickHistoryStoreError(err error) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		return radarport.ErrRadarClickHistoryConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return radarport.ErrRadarClickHistoryConflict
	}
	return radarport.ErrRadarClickHistoryUnavailable
}

func nilRadarClickHistoryDB(value radardb.DBTX) bool {
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
