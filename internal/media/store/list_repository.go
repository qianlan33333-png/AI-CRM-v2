package store

import (
	"context"
	"encoding/json"
	"errors"

	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var errInvalidImageListRepository = errors.New("invalid media image list repository")

const imageListTrimSpaceCharacters = "\t\n\v\f\r \u0085\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000"

var _ mediaapp.ImageListStore = (*UploadRepository)(nil)

func (repository *UploadRepository) ListImageRows(
	ctx context.Context,
	filter mediaapp.ImageListFilter,
	limit int64,
	offset int64,
) (mediaapp.ImageListRead, error) {
	if repository == nil || ctx == nil || limit < 1 || offset < 0 {
		return mediaapp.ImageListRead{}, errInvalidImageListRepository
	}
	tags := filter.Tags
	if tags == nil {
		tags = []string{}
	}
	tagGroups := filter.TagGroups
	if tagGroups == nil {
		tagGroups = [][]string{}
	}
	encodedGroups, err := json.Marshal(tagGroups)
	if err != nil {
		return mediaapp.ImageListRead{}, errInvalidImageListRepository
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return mediaapp.ImageListRead{}, err
	}
	rows, err := mediadb.New(tx).ListMediaImagePage(ctx, mediadb.ListMediaImagePageParams{
		Column1: filter.Search,
		Column2: filter.Category,
		Column3: tags,
		Column4: encodedGroups,
		Column5: filter.OnlyUnlabeled,
		Column6: limit,
		Column7: offset,
		Column8: imageListTrimSpaceCharacters,
		Column9: filter.EnabledOnly,
	})
	if err != nil {
		return mediaapp.ImageListRead{}, err
	}
	return imageListReadFromGeneratedRows(rows)
}

func (repository *UploadRepository) CountEnabledImages(ctx context.Context) (int64, error) {
	if repository == nil || ctx == nil {
		return 0, errInvalidImageListRepository
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	count, err := mediadb.New(tx).CountEnabledMediaImages(ctx)
	if err != nil || count < 0 {
		return 0, errInvalidImageListRepository
	}
	return count, nil
}

func imageListReadFromGeneratedRows(rows []mediadb.ListMediaImagePageRow) (mediaapp.ImageListRead, error) {
	result := mediaapp.ImageListRead{Rows: []mediaapp.ImageListRow{}}
	seen := false
	for _, row := range rows {
		if row.Total < 0 || (seen && result.Total != row.Total) {
			return mediaapp.ImageListRead{}, errInvalidImageListRepository
		}
		seen = true
		result.Total = row.Total
		if !row.ID.Valid {
			if row.Name.Valid || row.FileName.Valid || row.MimeType.Valid || row.FileSize.Valid || row.Enabled.Valid || row.Description.Valid || row.Tags.Valid ||
				row.Category.Valid || row.Width.Valid || row.Height.Valid || row.CreatedAt.Valid || row.UpdatedAt.Valid {
				return mediaapp.ImageListRead{}, errInvalidImageListRepository
			}
			continue
		}
		if !row.Name.Valid || !row.FileName.Valid || !row.MimeType.Valid || !row.FileSize.Valid || !row.Enabled.Valid || !row.Description.Valid || !row.Tags.Valid ||
			!row.Category.Valid || !row.Width.Valid || !row.Height.Valid || !row.CreatedAt.Valid || !row.UpdatedAt.Valid {
			return mediaapp.ImageListRead{}, errInvalidImageListRepository
		}
		result.Rows = append(result.Rows, mediaapp.ImageListRow{
			ID: row.ID.Int64, Name: row.Name.String, FileName: row.FileName.String, MimeType: row.MimeType.String,
			FileSize: row.FileSize.Int32, Enabled: row.Enabled.Bool, Description: row.Description.String, Tags: row.Tags.String, Category: row.Category.String,
			Width: row.Width.Int32, Height: row.Height.Int32, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		})
	}
	if !seen {
		return mediaapp.ImageListRead{}, errInvalidImageListRepository
	}
	return result, nil
}
