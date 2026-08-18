package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
)

var _ mediaapp.ImageVariantStore = (*UploadRepository)(nil)

func (repository *UploadRepository) ReadImageVariant(ctx context.Context, imageID int64) (mediaapp.ImageVariantRow, error) {
	if repository == nil || ctx == nil || imageID < 1 {
		return mediaapp.ImageVariantRow{}, mediaapp.ErrImageVariantUnavailable
	}
	query, err := queries(ctx)
	if err != nil {
		return mediaapp.ImageVariantRow{}, err
	}
	row, err := query.GetMediaImageVariant(ctx, imageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return mediaapp.ImageVariantRow{}, mediaapp.ErrImageVariantNotFound
	}
	if err != nil {
		return mediaapp.ImageVariantRow{}, err
	}
	return imageVariantRowFromGenerated(row), nil
}

func imageVariantRowFromGenerated(row mediadb.GetMediaImageVariantRow) mediaapp.ImageVariantRow {
	return mediaapp.ImageVariantRow{
		ID: row.ID, FileName: row.FileName, MimeType: row.MimeType, FileSize: row.FileSize,
		Width: row.Width, Height: row.Height, ImageChecksum: append([]byte(nil), row.ImageChecksum...),
		BlobChecksum: append([]byte(nil), row.BlobChecksum...), Content: append([]byte(nil), row.Content...),
	}
}
