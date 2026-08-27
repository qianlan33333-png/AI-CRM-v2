package membergrid

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var _ ExternalShareStore = (*Repository)(nil)

const currentExternalShareSQL = `SELECT
  s.service_product_id,
  COALESCE(s.share_id, ''),
  s.enabled,
  s.version
FROM public.service_period_member_grid_external_shares AS s
WHERE s.service_product_id = $1`

// setExternalShareSQL is one PostgreSQL CAS statement. It creates version one
// for a disabled, absent row and otherwise advances the current version only
// when the caller's ExpectedVersion still matches. A disabled state stores no
// share ID, so an old public token cannot resolve after the transaction.
const setExternalShareSQL = `INSERT INTO public.service_period_member_grid_external_shares AS s (
  service_product_id,
  share_id,
  enabled,
  version,
  updated_by,
  created_at,
  updated_at
) VALUES ($1, NULLIF($2, ''), $3, 1, $4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT (service_product_id) DO UPDATE
SET share_id = EXCLUDED.share_id,
    enabled = EXCLUDED.enabled,
    version = s.version + 1,
    updated_by = EXCLUDED.updated_by,
    updated_at = CURRENT_TIMESTAMP
WHERE s.version = $5
RETURNING service_product_id, COALESCE(share_id, ''), enabled, version`

const lookupEnabledExternalShareSQL = `SELECT
  s.service_product_id,
  COALESCE(s.share_id, ''),
  s.enabled,
  s.version
FROM public.service_period_member_grid_external_shares AS s
WHERE s.share_id = $1
  AND s.enabled = TRUE`

// CurrentExternalShare returns the explicit disabled/version-zero baseline for
// products that have not yet had public sharing configured. The caller's
// management path remains responsible for proving the product exists.
func (repository *Repository) CurrentExternalShare(ctx context.Context, serviceProductID int64) (ExternalShare, error) {
	if repository == nil || repository.executor == nil || ctx == nil || serviceProductID < 1 {
		return ExternalShare{}, ErrUnavailable
	}
	executor, err := repository.executor(ctx)
	if err != nil {
		return ExternalShare{}, errors.Join(ErrUnavailable, err)
	}
	share, err := scanExternalShare(executor.QueryRow(ctx, currentExternalShareSQL, serviceProductID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ExternalShare{ServiceProductID: serviceProductID, Version: 0}, nil
	}
	if err != nil {
		return ExternalShare{}, externalShareRepositoryError(err, false)
	}
	return share, nil
}

func (repository *Repository) SetExternalShare(ctx context.Context, record SetExternalShareRecord) (ExternalShare, error) {
	if repository == nil || repository.executor == nil || ctx == nil || !validSetExternalShareRecord(record) {
		return ExternalShare{}, ErrUnavailable
	}
	executor, err := repository.executor(ctx)
	if err != nil {
		return ExternalShare{}, errors.Join(ErrUnavailable, err)
	}
	share, err := scanExternalShare(executor.QueryRow(ctx, setExternalShareSQL,
		record.ServiceProductID, record.ShareID, record.Enabled, record.ActorID, record.ExpectedVersion))
	if err != nil {
		return ExternalShare{}, externalShareRepositoryError(err, false)
	}
	if share.ServiceProductID != record.ServiceProductID || share.Enabled != record.Enabled || share.ShareID != record.ShareID || share.Version != record.ExpectedVersion+1 {
		return ExternalShare{}, ErrUnavailable
	}
	return share, nil
}

func (repository *Repository) LookupEnabledExternalShare(ctx context.Context, shareID string) (ExternalShare, error) {
	if repository == nil || repository.executor == nil || ctx == nil || !validExternalShareID(shareID) {
		return ExternalShare{}, ErrUnavailable
	}
	executor, err := repository.executor(ctx)
	if err != nil {
		return ExternalShare{}, errors.Join(ErrUnavailable, err)
	}
	share, err := scanExternalShare(executor.QueryRow(ctx, lookupEnabledExternalShareSQL, shareID))
	if err != nil {
		return ExternalShare{}, externalShareRepositoryError(err, true)
	}
	if !share.Enabled || share.ShareID != shareID {
		return ExternalShare{}, ErrUnavailable
	}
	return share, nil
}

func scanExternalShare(row sqlRow) (ExternalShare, error) {
	var share ExternalShare
	if err := row.Scan(&share.ServiceProductID, &share.ShareID, &share.Enabled, &share.Version); err != nil {
		return ExternalShare{}, err
	}
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
