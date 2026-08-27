package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	media "github.com/qianlan33333-png/AI-CRM-v2/internal/media"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type HistoricalMiniProgramStore struct {
	tx func(context.Context) (pgx.Tx, error)
}

var _ media.HistoricalMiniProgramStore = (*HistoricalMiniProgramStore)(nil)

func NewHistoricalMiniProgramStore() *HistoricalMiniProgramStore {
	return &HistoricalMiniProgramStore{tx: platformstore.TxFromContext}
}

// InsertHistoricalMiniProgram has no path to the normal Media mutation
// receipts, event log, thumbnail cache, queue, or Provider. Historical rows
// are permanently inserted disabled with all provider-backed fields at schema
// defaults (empty/null).
func (store *HistoricalMiniProgramStore) InsertHistoricalMiniProgram(ctx context.Context, definition media.HistoricalMiniProgramDefinition) (int64, error) {
	if store == nil || store.tx == nil || definition.Item.Enabled || definition.Item.ThumbnailImageURL != "" || definition.Item.ThumbnailImageBase64 != "" ||
		definition.Item.ThumbnailImageID != nil || definition.Item.ThumbnailMediaID != "" || definition.Item.ThumbnailMediaExpiresAt != nil {
		return 0, media.ErrHistoricalMiniProgramInvalid
	}
	tx, err := store.tx(ctx)
	if err != nil {
		return 0, err
	}
	item := definition.Item
	id, err := mediadb.New(tx).InsertHistoricalMiniProgram(ctx, mediadb.InsertHistoricalMiniProgramParams{
		LegacySourceID: pgtype.Int8{Int64: definition.SourceID, Valid: true},
		Name:           item.Name, AppID: item.AppID, PagePath: item.PagePath, Title: item.Title,
		CreatedBy: item.CreatedBy, UpdatedBy: item.UpdatedBy, Version: item.Version,
		CreatedAt: pgtype.Timestamptz{Time: item.CreatedAt.UTC(), Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: item.UpdatedAt.UTC(), Valid: true},
	})
	if err != nil {
		return 0, historicalMiniProgramConflict(err)
	}
	if id < 1 {
		return 0, media.ErrHistoricalMiniProgramInvalid
	}
	return id, nil
}

func historicalMiniProgramConflict(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return media.ErrHistoricalMiniProgramConflict
	}
	return err
}
