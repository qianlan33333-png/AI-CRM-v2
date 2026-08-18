package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
)

var _ mediaapp.ImageDetailStore = (*UploadRepository)(nil)

func (repository *UploadRepository) ReadImageDetail(ctx context.Context, imageID int64) (mediaapp.ImageDetailRow, error) {
	if repository == nil || ctx == nil || imageID < 1 {
		return mediaapp.ImageDetailRow{}, mediaapp.ErrImageDetailUnavailable
	}
	query, err := queries(ctx)
	if err != nil {
		return mediaapp.ImageDetailRow{}, err
	}
	row, err := query.GetMediaImageDetail(ctx, imageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return mediaapp.ImageDetailRow{}, mediaapp.ErrImageDetailNotFound
	}
	if err != nil {
		return mediaapp.ImageDetailRow{}, err
	}
	return imageDetailRowFromGenerated(row), nil
}

func imageDetailRowFromGenerated(row mediadb.GetMediaImageDetailRow) mediaapp.ImageDetailRow {
	return mediaapp.ImageDetailRow{
		ID: row.ID, Name: row.Name, FileName: row.FileName, MimeType: row.MimeType,
		FileSize: row.FileSize, Description: row.Description, Tags: row.Tags, Category: row.Category,
		Width: row.Width, Height: row.Height, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		ImageChecksum: append([]byte(nil), row.ImageChecksum...), BlobChecksum: append([]byte(nil), row.BlobChecksum...),
		Content: append([]byte(nil), row.Content...),
	}
}
