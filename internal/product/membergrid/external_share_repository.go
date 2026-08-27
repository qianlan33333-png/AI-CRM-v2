package membergrid

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

var _ ExternalShareStore = (*Repository)(nil)

// CurrentExternalShare returns the explicit disabled/version-zero baseline for
// products that have not yet had public sharing configured. The caller's
// management path remains responsible for proving the product exists.
func (repository *Repository) CurrentExternalShare(ctx context.Context, serviceProductID int64) (ExternalShare, error) {
	if repository == nil || repository.shareQueries == nil || ctx == nil || serviceProductID < 1 {
		return ExternalShare{}, ErrUnavailable
	}
	queries, err := repository.shareQueries(ctx)
	if err != nil {
		return ExternalShare{}, errors.Join(ErrUnavailable, err)
	}
	row, err := queries.CurrentMemberGridExternalShare(ctx, serviceProductID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ExternalShare{ServiceProductID: serviceProductID, Version: 0}, nil
	}
	if err != nil {
		return ExternalShare{}, externalShareRepositoryError(err, false)
	}
	return mapExternalShare(row.ServiceProductID, row.ShareID, row.Enabled, row.Version)
}

func (repository *Repository) SetExternalShare(ctx context.Context, record SetExternalShareRecord) (ExternalShare, error) {
	if repository == nil || repository.shareQueries == nil || ctx == nil || !validSetExternalShareRecord(record) {
		return ExternalShare{}, ErrUnavailable
	}
	queries, err := repository.shareQueries(ctx)
	if err != nil {
		return ExternalShare{}, errors.Join(ErrUnavailable, err)
	}
	row, err := queries.SetMemberGridExternalShare(ctx, productdb.SetMemberGridExternalShareParams{
		ServiceProductID: record.ServiceProductID,
		ShareID:          record.ShareID,
		Enabled:          record.Enabled,
		UpdatedBy:        record.ActorID,
		ExpectedVersion:  record.ExpectedVersion,
	})
	if err != nil {
		return ExternalShare{}, externalShareRepositoryError(err, false)
	}
	share, err := mapExternalShare(row.ServiceProductID, row.ShareID, row.Enabled, row.Version)
	if err != nil {
		return ExternalShare{}, err
	}
	if share.ServiceProductID != record.ServiceProductID || share.Enabled != record.Enabled || share.ShareID != record.ShareID || share.Version != record.ExpectedVersion+1 {
		return ExternalShare{}, ErrUnavailable
	}
	return share, nil
}

func (repository *Repository) LookupEnabledExternalShare(ctx context.Context, shareID string) (ExternalShare, error) {
	if repository == nil || repository.shareQueries == nil || ctx == nil || !validExternalShareID(shareID) {
		return ExternalShare{}, ErrUnavailable
	}
	queries, err := repository.shareQueries(ctx)
	if err != nil {
		return ExternalShare{}, errors.Join(ErrUnavailable, err)
	}
	row, err := queries.LookupEnabledMemberGridExternalShare(ctx, shareID)
	if err != nil {
		return ExternalShare{}, externalShareRepositoryError(err, true)
	}
	share, err := mapExternalShare(row.ServiceProductID, row.ShareID, row.Enabled, row.Version)
	if err != nil {
		return ExternalShare{}, err
	}
	if !share.Enabled || share.ShareID != shareID {
		return ExternalShare{}, ErrUnavailable
	}
	return share, nil
}

func mapExternalShare(serviceProductID int64, shareID string, enabled bool, version int64) (ExternalShare, error) {
	share := ExternalShare{ServiceProductID: serviceProductID, ShareID: shareID, Enabled: enabled, Version: version}
	if !validExternalShare(share) {
		return ExternalShare{}, ErrUnavailable
	}
	return cloneExternalShare(share), nil
}

func validSetExternalShareRecord(record SetExternalShareRecord) bool {
	return record.ServiceProductID > 0 && record.ExpectedVersion >= 0 && record.ActorID > 0 && validIdempotencyKey(record.IdempotencyKey) &&
		(!record.Enabled && record.ShareID == "" || record.Enabled && validExternalShareID(record.ShareID))
}

func externalShareRepositoryError(err error, notFound bool) error {
	if errors.Is(err, pgx.ErrNoRows) {
		if notFound {
			return ErrNotFound
		}
		return ErrConflict
	}
	return managementRepositoryError(err, false)
}
