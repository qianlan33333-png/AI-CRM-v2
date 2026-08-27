package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	media "github.com/qianlan33333-png/AI-CRM-v2/internal/media"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type HistoricalStaticStore struct {
	tx func(context.Context) (pgx.Tx, error)
}

var _ media.HistoricalStaticStore = (*HistoricalStaticStore)(nil)

func NewHistoricalStaticStore() *HistoricalStaticStore {
	return &HistoricalStaticStore{tx: platformstore.TxFromContext}
}

// Historical inserts use only the caller's transaction. They create disabled
// metadata and its verified blob atomically, without normal mutation receipts,
// events, variants, Provider caches, or jobs. The migration journal owns replay.
func (store *HistoricalStaticStore) InsertHistoricalImage(ctx context.Context, definition media.HistoricalImageDefinition) (int64, error) {
	if store == nil || store.tx == nil || ctx == nil || definition.Validate() != nil {
		return 0, media.ErrHistoricalStaticInvalid
	}
	tx, err := store.tx(ctx)
	if err != nil {
		return 0, err
	}
	item := definition.Image
	var id int64
	err = tx.QueryRow(ctx, `WITH inserted AS (
		INSERT INTO public.media_images
		(name,file_name,mime_type,file_size,width,height,checksum,description,tags,category,enabled,created_by,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,FALSE,$11,$12,$13) RETURNING id
	)
	INSERT INTO public.media_image_blobs (image_id,content,checksum,created_at)
	SELECT id,$14,$7,$12 FROM inserted RETURNING image_id`,
		item.Name, item.FileName, item.MimeType, item.FileSize, item.Width, item.Height, definition.Checksum[:],
		item.Description, item.Tags, item.Category, definition.Actor, item.CreatedAt.UTC(), item.UpdatedAt.UTC(), definition.Content).Scan(&id)
	return historicalStaticInsertResult(id, err)
}

func (store *HistoricalStaticStore) InsertHistoricalAttachment(ctx context.Context, definition media.HistoricalAttachmentDefinition) (int64, error) {
	if store == nil || store.tx == nil || ctx == nil || definition.Validate() != nil {
		return 0, media.ErrHistoricalStaticInvalid
	}
	tx, err := store.tx(ctx)
	if err != nil {
		return 0, err
	}
	item := definition.Attachment
	if item.Tags == nil {
		item.Tags = []string{}
	}
	tags, err := json.Marshal(item.Tags)
	if err != nil {
		return 0, err
	}
	var id int64
	err = tx.QueryRow(ctx, `WITH inserted AS (
		INSERT INTO public.media_attachments
		(name,file_name,mime_type,file_size,checksum,description,tags,enabled,version,created_by,updated_by,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,FALSE,1,$8,$8,$9,$10) RETURNING id
	)
	INSERT INTO public.media_attachment_blobs (attachment_id,content,checksum,created_at)
	SELECT id,$11,$5,$9 FROM inserted RETURNING attachment_id`,
		item.Name, item.FileName, item.MimeType, item.FileSize, definition.Checksum[:], item.Description, tags,
		item.CreatedBy, item.CreatedAt.UTC(), item.UpdatedAt.UTC(), definition.Content).Scan(&id)
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
