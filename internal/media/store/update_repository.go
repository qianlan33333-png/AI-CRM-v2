package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
)

var _ mediaapp.ImageMetadataUpdateStore = (*UploadRepository)(nil)

func (repository *UploadRepository) LockImageMetadata(ctx context.Context, imageID int64) (mediaapp.ImageMetadata, error) {
	if repository == nil || ctx == nil || imageID < 1 {
		return mediaapp.ImageMetadata{}, mediaapp.ErrImageMetadataUnavailable
	}
	query, err := queries(ctx)
	if err != nil {
		return mediaapp.ImageMetadata{}, err
	}
	row, err := query.LockMediaImageMetadata(ctx, imageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return mediaapp.ImageMetadata{}, mediaapp.ErrImageMetadataNotFound
	}
	if err != nil {
		return mediaapp.ImageMetadata{}, err
	}
	return imageMetadataFromLockRow(row), nil
}

func (repository *UploadRepository) UpdateImageMetadata(ctx context.Context, image mediaapp.ImageMetadata) (mediaapp.ImageMetadata, error) {
	if repository == nil || ctx == nil || image.ID < 1 {
		return mediaapp.ImageMetadata{}, mediaapp.ErrImageMetadataUnavailable
	}
	query, err := queries(ctx)
	if err != nil {
		return mediaapp.ImageMetadata{}, err
	}
	row, err := query.UpdateMediaImageMetadata(ctx, mediadb.UpdateMediaImageMetadataParams{
		ImageID: image.ID, Name: image.Name, Description: image.Description, Tags: image.Tags,
		Category: image.Category, Enabled: image.Enabled, UpdatedAt: stamp(image.UpdatedAt),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return mediaapp.ImageMetadata{}, mediaapp.ErrImageMetadataNotFound
	}
	if err != nil {
		return mediaapp.ImageMetadata{}, err
	}
	return imageMetadataFromUpdateRow(row), nil
}

func imageMetadataFromLockRow(row mediadb.LockMediaImageMetadataRow) mediaapp.ImageMetadata {
	return mediaapp.ImageMetadata{
		ID: row.ID, Name: row.Name, FileName: row.FileName, MimeType: row.MimeType, FileSize: int64(row.FileSize),
		Enabled: row.Enabled, Description: row.Description, Tags: row.Tags, Category: row.Category,
		Width: int(row.Width), Height: int(row.Height), CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func imageMetadataFromUpdateRow(row mediadb.UpdateMediaImageMetadataRow) mediaapp.ImageMetadata {
	return mediaapp.ImageMetadata{
		ID: row.ID, Name: row.Name, FileName: row.FileName, MimeType: row.MimeType, FileSize: int64(row.FileSize),
		Enabled: row.Enabled, Description: row.Description, Tags: row.Tags, Category: row.Category,
		Width: int(row.Width), Height: int(row.Height), CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}
