package store

import (
	"context"
	"errors"

	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
)

var errInvalidFacetsRepository = errors.New("invalid media facets repository")

var _ mediaapp.FacetStore = (*UploadRepository)(nil)

func (repository *UploadRepository) ListFacetRows(ctx context.Context) ([]mediaapp.FacetRow, error) {
	if repository == nil {
		return nil, errInvalidFacetsRepository
	}
	query, err := queries(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := query.ListMediaImageFacetRows(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]mediaapp.FacetRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, mediaapp.FacetRow{Category: row.Category, Tags: row.Tags})
	}
	return result, nil
}
