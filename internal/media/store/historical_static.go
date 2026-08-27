package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	media "github.com/qianlan33333-png/AI-CRM-v2/internal/media"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type HistoricalStaticStore struct {
	tx         func(context.Context) (pgx.Tx, error)
	newQueries func(pgx.Tx) historicalStaticQueries
}

var _ media.HistoricalStaticStore = (*HistoricalStaticStore)(nil)

type historicalStaticQueries interface {
	InsertHistoricalStaticImage(context.Context, mediadb.InsertHistoricalStaticImageParams) (int64, error)
	InsertHistoricalStaticAttachment(context.Context, mediadb.InsertHistoricalStaticAttachmentParams) (int64, error)
}

func NewHistoricalStaticStore() *HistoricalStaticStore {
	return &HistoricalStaticStore{
		tx: platformstore.TxFromContext,
		newQueries: func(tx pgx.Tx) historicalStaticQueries {
			return mediadb.New(tx)
		},
	}
}

// Historical inserts use only the caller's transaction. They create disabled
// metadata and its verified blob atomically, without normal mutation receipts,
// events, variants, Provider caches, or jobs. The migration journal owns replay.
func (store *HistoricalStaticStore) InsertHistoricalImage(ctx context.Context, definition media.HistoricalImageDefinition) (int64, error) {
	if store == nil || store.tx == nil || store.newQueries == nil || ctx == nil || definition.Validate() != nil {
		return 0, media.ErrHistoricalStaticInvalid
	}
	tx, err := store.tx(ctx)
	if err != nil {
		return 0, err
	}
	queries := store.newQueries(tx)
	if queries == nil {
		return 0, media.ErrHistoricalStaticInvalid
	}
	item := definition.Image
	id, err := queries.InsertHistoricalStaticImage(ctx, mediadb.InsertHistoricalStaticImageParams{
		Content: definition.Content, Checksum: definition.Checksum[:], CreatedAt: stamp(item.CreatedAt.UTC()),
		Name: item.Name, FileName: item.FileName, MimeType: item.MimeType, FileSize: item.FileSize,
		Width: item.Width, Height: item.Height, Description: item.Description, Tags: item.Tags, Category: item.Category,
		CreatedBy: definition.Actor, UpdatedAt: stamp(item.UpdatedAt.UTC()),
	})
	return historicalStaticInsertResult(id, err)
}

func (store *HistoricalStaticStore) InsertHistoricalAttachment(ctx context.Context, definition media.HistoricalAttachmentDefinition) (int64, error) {
	if store == nil || store.tx == nil || store.newQueries == nil || ctx == nil || definition.Validate() != nil {
		return 0, media.ErrHistoricalStaticInvalid
	}
	tx, err := store.tx(ctx)
	if err != nil {
		return 0, err
	}
	queries := store.newQueries(tx)
	if queries == nil {
		return 0, media.ErrHistoricalStaticInvalid
	}
	item := definition.Attachment
	if item.Tags == nil {
		item.Tags = []string{}
	}
	tags, err := json.Marshal(item.Tags)
	if err != nil {
		return 0, err
	}
	id, err := queries.InsertHistoricalStaticAttachment(ctx, mediadb.InsertHistoricalStaticAttachmentParams{
		Content: definition.Content, Checksum: definition.Checksum[:], CreatedAt: stamp(item.CreatedAt.UTC()),
		Name: item.Name, FileName: item.FileName, MimeType: item.MimeType, FileSize: int32(item.FileSize),
		Description: item.Description, Tags: tags, Actor: item.CreatedBy, UpdatedAt: stamp(item.UpdatedAt.UTC()),
	})
	return historicalStaticInsertResult(id, err)
}

func historicalStaticInsertResult(id int64, err error) (int64, error) {
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return 0, media.ErrHistoricalStaticConflict
		}
		return 0, err
	}
	if id < 1 {
		return 0, media.ErrHistoricalStaticInvalid
	}
	return id, nil
}
