package store

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type HistoricalChannelStore struct {
	tx func(context.Context) (pgx.Tx, error)
}

var _ contactport.HistoricalChannelStore = (*HistoricalChannelStore)(nil)

func NewHistoricalChannelStore() *HistoricalChannelStore {
	return &HistoricalChannelStore{tx: platformstore.TxFromContext}
}

func (store *HistoricalChannelStore) CreateHistoricalChannel(ctx context.Context, record contactport.HistoricalChannelRecord) (contactport.HistoricalChannelRecord, error) {
	if !validHistoricalChannelCreate(record) {
		return contactport.HistoricalChannelRecord{}, contactport.ErrHistoricalChannelInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return contactport.HistoricalChannelRecord{}, err
	}
	row, err := queries.CreateHistoricalChannel(ctx, contactdb.CreateHistoricalChannelParams{
		Code: record.Code, Name: record.Name, Projection: record.Projection, Actor: record.CreatedBy,
		ConfigDigest: record.LegacyConfigDigest,
		CreatedAt:    pgtype.Timestamptz{Time: record.CreatedAt, Valid: true},
		UpdatedAt:    pgtype.Timestamptz{Time: record.UpdatedAt, Valid: true},
	})
	if err != nil {
		return contactport.HistoricalChannelRecord{}, historicalChannelError(err)
	}
	return historicalChannelRecord(contactdb.GetHistoricalChannelRow(row)), nil
}

func (store *HistoricalChannelStore) GetHistoricalChannel(ctx context.Context, id int64) (contactport.HistoricalChannelRecord, error) {
	if id < 1 {
		return contactport.HistoricalChannelRecord{}, contactport.ErrHistoricalChannelInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return contactport.HistoricalChannelRecord{}, err
	}
	row, err := queries.GetHistoricalChannel(ctx, id)
	if err != nil {
		return contactport.HistoricalChannelRecord{}, historicalChannelError(err)
	}
	return historicalChannelRecord(row), nil
}

func (store *HistoricalChannelStore) queries(ctx context.Context) (*contactdb.Queries, error) {
	if store == nil || store.tx == nil || ctx == nil || ctx.Err() != nil {
		return nil, contactport.ErrHistoricalChannelUnavailable
	}
	tx, err := store.tx(ctx)
	if err != nil || tx == nil {
		return nil, contactport.ErrHistoricalChannelUnavailable
	}
	return contactdb.New(tx), nil
}

func historicalChannelRecord(row contactdb.GetHistoricalChannelRow) contactport.HistoricalChannelRecord {
	return contactport.HistoricalChannelRecord{
		ID: row.ID, Code: row.Code, Name: row.Name, Status: row.Status, Projection: row.Config,
		LegacyConfigDigest: row.ConfigDigest, CreatedBy: row.CreatedBy, UpdatedBy: row.UpdatedBy,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func validHistoricalChannelCreate(record contactport.HistoricalChannelRecord) bool {
	if record.ID != 0 || record.Status != "inactive" || record.CreatedBy < 1 || record.UpdatedBy != record.CreatedBy ||
		record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() || record.UpdatedAt.Before(record.CreatedAt) {
		return false
	}
	for _, value := range []string{record.Code, record.Name} {
		if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 200 {
			return false
		}
	}
	if len(record.LegacyConfigDigest) != 71 || !strings.HasPrefix(record.LegacyConfigDigest, "sha256:") {
		return false
	}
	digest, err := hex.DecodeString(record.LegacyConfigDigest[7:])
	if err != nil || hex.EncodeToString(digest) != record.LegacyConfigDigest[7:] {
		return false
	}
	var projection map[string]json.RawMessage
	if json.Unmarshal(record.Projection, &projection) != nil || projection == nil {
		return false
	}
	_, found := projection["schema_version"]
	return found
}

func historicalChannelError(err error) error {
	var postgresError *pgconn.PgError
	if errors.Is(err, pgx.ErrNoRows) || errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return contactport.ErrHistoricalChannelConflict
	}
	return contactport.ErrHistoricalChannelUnavailable
}
