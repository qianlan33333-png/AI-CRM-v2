package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	media "github.com/qianlan33333-png/AI-CRM-v2/internal/media"
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
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO public.media_miniprograms
		(legacy_source_id,name,app_id,page_path,title,enabled,created_by,updated_by,version,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,FALSE,$6,$7,$8,$9,$10) RETURNING id`,
		definition.SourceID, item.Name, item.AppID, item.PagePath, item.Title, item.CreatedBy, item.UpdatedBy, item.Version, item.CreatedAt.UTC(), item.UpdatedAt.UTC()).Scan(&id)
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
