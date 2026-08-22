package membergrid

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const activeStaffExistsSQL = `SELECT COALESCE((
  SELECT TRUE
  FROM public.staff AS s
  WHERE s.id = $1
    AND s.is_active = TRUE
  FOR SHARE
), FALSE)`

var _ ManagementStore = (*Repository)(nil)

const listSavedViewsSQL = `SELECT
  v.id,
  v.service_product_id,
  v.name,
  v.state,
  v.sort,
  v.columns,
  v.source_view_id,
  v.version,
  v.created_by,
  v.created_at,
  v.updated_at
FROM public.service_period_member_views AS v
WHERE v.service_product_id = $1
ORDER BY v.id ASC`

const getSavedViewForUpdateSQL = `SELECT
  v.id,
  v.service_product_id,
  v.name,
  v.state,
  v.sort,
  v.columns,
  v.source_view_id,
  v.version,
  v.created_by,
  v.created_at,
  v.updated_at
FROM public.service_period_member_views AS v
WHERE v.service_product_id = $1
  AND v.id = $2
FOR UPDATE`

const createSavedViewSQL = `INSERT INTO public.service_period_member_views (
  service_product_id,
  name,
  state,
  sort,
  columns,
  source_view_id,
  version,
  created_by,
  created_at,
  updated_at
) VALUES ($1, $2, $3, $4, $5, $6, 1, $7, $8, $8)
RETURNING id, service_product_id, name, state, sort, columns, source_view_id,
  version, created_by, created_at, updated_at`

const updateSavedViewSQL = `UPDATE public.service_period_member_views
SET name = $4,
    state = $5,
    sort = $6,
    columns = $7,
    version = version + 1,
    updated_at = $8
WHERE service_product_id = $1
  AND id = $2
  AND version = $3
RETURNING id, service_product_id, name, state, sort, columns, source_view_id,
  version, created_by, created_at, updated_at`

const deleteSavedViewSQL = `DELETE FROM public.service_period_member_views
WHERE service_product_id = $1
  AND id = $2
  AND version = $3
RETURNING id, service_product_id, name, state, sort, columns, source_view_id,
  version, created_by, created_at, updated_at`

const listCollaboratorsSQL = `SELECT
  c.id,
  c.service_product_id,
  c.staff_id,
  c.permission,
  c.version,
  c.invited_by,
  c.created_at,
  c.updated_at
FROM public.service_period_member_grid_collaborators AS c
WHERE c.service_product_id = $1
ORDER BY c.id ASC`

const getCollaboratorForUpdateSQL = `SELECT
  c.id,
  c.service_product_id,
  c.staff_id,
  c.permission,
  c.version,
  c.invited_by,
  c.created_at,
  c.updated_at
FROM public.service_period_member_grid_collaborators AS c
WHERE c.service_product_id = $1
  AND c.id = $2
FOR UPDATE`

const createCollaboratorSQL = `INSERT INTO public.service_period_member_grid_collaborators (
  service_product_id,
  staff_id,
  permission,
  version,
  invited_by,
  created_at,
  updated_at
) VALUES ($1, $2, $3, 1, $4, $5, $5)
RETURNING id, service_product_id, staff_id, permission, version, invited_by,
  created_at, updated_at`

const updateCollaboratorSQL = `UPDATE public.service_period_member_grid_collaborators
SET permission = $4,
    version = version + 1,
    updated_at = $5
WHERE service_product_id = $1
  AND id = $2
  AND version = $3
RETURNING id, service_product_id, staff_id, permission, version, invited_by,
  created_at, updated_at`

const deleteCollaboratorSQL = `DELETE FROM public.service_period_member_grid_collaborators
WHERE service_product_id = $1
  AND id = $2
  AND version = $3
RETURNING id, service_product_id, staff_id, permission, version, invited_by,
  created_at, updated_at`

const reserveManagementReceiptSQL = `INSERT INTO public.product_operation_receipts (
  operation,
  actor_scope,
  key_digest,
  payload_digest,
  created_at
) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (operation, actor_scope, key_digest) DO NOTHING
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot`

const getManagementReceiptSQL = `SELECT
  id,
  operation,
  actor_scope,
  key_digest,
  payload_digest,
  state,
  result_snapshot
FROM public.product_operation_receipts
WHERE operation = $1
  AND actor_scope = $2
  AND key_digest = $3`

const completeManagementReceiptSQL = `UPDATE public.product_operation_receipts
SET state = 'completed',
    result_snapshot = $2::jsonb,
    completed_at = $3
WHERE id = $1
  AND state = 'in_progress'
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot`

func (repository *Repository) ActiveStaffExists(ctx context.Context, staffID int64) (bool, error) {
	if repository == nil || repository.executor == nil || ctx == nil || staffID < 1 {
		return false, ErrUnavailable
	}
	executor, err := repository.executor(ctx)
	if err != nil {
		return false, errors.Join(ErrUnavailable, err)
	}
	var exists bool
	if err = executor.QueryRow(ctx, activeStaffExistsSQL, staffID).Scan(&exists); err != nil {
		return false, managementRepositoryError(err, false)
	}
	return exists, nil
}

func (repository *Repository) ListSavedViews(ctx context.Context, serviceProductID int64) ([]SavedView, error) {
	executor, err := repository.managementExecutor(ctx, serviceProductID)
	if err != nil {
		return nil, err
	}
	rows, err := executor.Query(ctx, listSavedViewsSQL, serviceProductID)
	if err != nil {
		return nil, managementRepositoryError(err, false)
	}
	defer rows.Close()
	views := make([]SavedView, 0)
	for rows.Next() {
		view, scanErr := scanSavedView(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		views = append(views, view)
	}
	if err = rows.Err(); err != nil {
		return nil, managementRepositoryError(err, false)
	}
	return views, nil
}

func (repository *Repository) GetSavedViewForUpdate(ctx context.Context, serviceProductID, viewID int64) (SavedView, error) {
	if viewID < 1 {
		return SavedView{}, ErrNotFound
	}
	executor, err := repository.managementExecutor(ctx, serviceProductID)
	if err != nil {
		return SavedView{}, err
	}
	view, err := scanSavedView(executor.QueryRow(ctx, getSavedViewForUpdateSQL, serviceProductID, viewID))
	if err != nil {
		return SavedView{}, managementRepositoryError(err, true)
	}
	return view, nil
}

func (repository *Repository) CreateSavedView(ctx context.Context, record CreateSavedViewRecord) (SavedView, error) {
	if !validCreateSavedViewRecord(record) {
		return SavedView{}, ErrUnavailable
	}
	executor, err := repository.managementExecutor(ctx, record.ServiceProductID)
	if err != nil {
		return SavedView{}, err
	}
	view, err := scanSavedView(executor.QueryRow(ctx, createSavedViewSQL,
		record.ServiceProductID, record.Name, string(record.State), string(record.Sort), cloneColumnsSelection(record.Columns),
		cloneOptionalID(record.SourceViewID), record.CreatedBy, record.CreatedAt.UTC()))
	if err != nil {
		return SavedView{}, managementRepositoryError(err, false)
	}
	return view, nil
}

func (repository *Repository) UpdateSavedView(ctx context.Context, record UpdateSavedViewRecord) (SavedView, error) {
	if !validUpdateSavedViewRecord(record) {
		return SavedView{}, ErrUnavailable
	}
	executor, err := repository.managementExecutor(ctx, record.ServiceProductID)
	if err != nil {
		return SavedView{}, err
	}
	view, err := scanSavedView(executor.QueryRow(ctx, updateSavedViewSQL,
		record.ServiceProductID, record.ViewID, record.ExpectedVersion, record.Name, string(record.State), string(record.Sort),
		cloneColumnsSelection(record.Columns), record.UpdatedAt.UTC()))
	if err != nil {
		return SavedView{}, managementRepositoryError(err, false)
	}
	return view, nil
}

func (repository *Repository) DeleteSavedView(ctx context.Context, serviceProductID, viewID, expectedVersion int64) (SavedView, error) {
	if viewID < 1 || expectedVersion < 1 {
		return SavedView{}, ErrUnavailable
	}
	executor, err := repository.managementExecutor(ctx, serviceProductID)
	if err != nil {
		return SavedView{}, err
	}
	view, err := scanSavedView(executor.QueryRow(ctx, deleteSavedViewSQL, serviceProductID, viewID, expectedVersion))
	if err != nil {
		return SavedView{}, managementRepositoryError(err, false)
	}
	return view, nil
}

func (repository *Repository) ListCollaborators(ctx context.Context, serviceProductID int64) ([]Collaborator, error) {
	executor, err := repository.managementExecutor(ctx, serviceProductID)
	if err != nil {
		return nil, err
	}
	rows, err := executor.Query(ctx, listCollaboratorsSQL, serviceProductID)
	if err != nil {
		return nil, managementRepositoryError(err, false)
	}
	defer rows.Close()
	collaborators := make([]Collaborator, 0)
	for rows.Next() {
		collaborator, scanErr := scanCollaborator(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		collaborators = append(collaborators, collaborator)
	}
	if err = rows.Err(); err != nil {
		return nil, managementRepositoryError(err, false)
	}
	return collaborators, nil
}

func (repository *Repository) GetCollaboratorForUpdate(ctx context.Context, serviceProductID, collaboratorID int64) (Collaborator, error) {
	if collaboratorID < 1 {
		return Collaborator{}, ErrNotFound
	}
	executor, err := repository.managementExecutor(ctx, serviceProductID)
	if err != nil {
		return Collaborator{}, err
	}
	collaborator, err := scanCollaborator(executor.QueryRow(ctx, getCollaboratorForUpdateSQL, serviceProductID, collaboratorID))
	if err != nil {
		return Collaborator{}, managementRepositoryError(err, true)
	}
	return collaborator, nil
}

func (repository *Repository) CreateCollaborator(ctx context.Context, record CreateCollaboratorRecord) (Collaborator, error) {
	if !validCreateCollaboratorRecord(record) {
		return Collaborator{}, ErrUnavailable
	}
	executor, err := repository.managementExecutor(ctx, record.ServiceProductID)
	if err != nil {
		return Collaborator{}, err
	}
	collaborator, err := scanCollaborator(executor.QueryRow(ctx, createCollaboratorSQL,
		record.ServiceProductID, record.StaffID, string(record.Permission), record.InvitedBy, record.CreatedAt.UTC()))
	if err != nil {
		return Collaborator{}, managementRepositoryError(err, false)
	}
	return collaborator, nil
}

func (repository *Repository) UpdateCollaborator(ctx context.Context, record UpdateCollaboratorRecord) (Collaborator, error) {
	if !validUpdateCollaboratorRecord(record) {
		return Collaborator{}, ErrUnavailable
	}
	executor, err := repository.managementExecutor(ctx, record.ServiceProductID)
	if err != nil {
		return Collaborator{}, err
	}
	collaborator, err := scanCollaborator(executor.QueryRow(ctx, updateCollaboratorSQL,
		record.ServiceProductID, record.CollaboratorID, record.ExpectedVersion, string(record.Permission), record.UpdatedAt.UTC()))
	if err != nil {
		return Collaborator{}, managementRepositoryError(err, false)
	}
	return collaborator, nil
}

func (repository *Repository) DeleteCollaborator(ctx context.Context, serviceProductID, collaboratorID, expectedVersion int64) (Collaborator, error) {
	if collaboratorID < 1 || expectedVersion < 1 {
		return Collaborator{}, ErrUnavailable
	}
	executor, err := repository.managementExecutor(ctx, serviceProductID)
	if err != nil {
		return Collaborator{}, err
	}
	collaborator, err := scanCollaborator(executor.QueryRow(ctx, deleteCollaboratorSQL, serviceProductID, collaboratorID, expectedVersion))
	if err != nil {
		return Collaborator{}, managementRepositoryError(err, false)
	}
	return collaborator, nil
}

func (repository *Repository) ReserveMutationReceipt(ctx context.Context, reservation MutationReceiptReservation) (MutationReceipt, bool, error) {
	if repository == nil || repository.executor == nil || ctx == nil ||
		(reservation.Operation != mutationOperationCreate && reservation.Operation != mutationOperationUpdate) ||
		reservation.ActorScope == "" || len(reservation.ActorScope) > 200 || reservation.CreatedAt.IsZero() {
		return MutationReceipt{}, false, ErrUnavailable
	}
	executor, err := repository.executor(ctx)
	if err != nil {
		return MutationReceipt{}, false, errors.Join(ErrUnavailable, err)
	}
	receipt, err := scanMutationReceipt(executor.QueryRow(ctx, reserveManagementReceiptSQL,
		reservation.Operation, reservation.ActorScope, reservation.KeyDigest[:], reservation.PayloadDigest[:], reservation.CreatedAt.UTC()))
	if err == nil {
		return receipt, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return MutationReceipt{}, false, managementRepositoryError(err, false)
	}
	receipt, err = scanMutationReceipt(executor.QueryRow(ctx, getManagementReceiptSQL,
		reservation.Operation, reservation.ActorScope, reservation.KeyDigest[:]))
	if err != nil {
		return MutationReceipt{}, false, managementRepositoryError(err, false)
	}
	return receipt, false, nil
}

func (repository *Repository) CompleteMutationReceipt(ctx context.Context, receiptID int64, snapshot json.RawMessage, now time.Time) (MutationReceipt, error) {
	if repository == nil || repository.executor == nil || ctx == nil || receiptID < 1 || now.IsZero() || !json.Valid(snapshot) {
		return MutationReceipt{}, ErrUnavailable
	}
	executor, err := repository.executor(ctx)
	if err != nil {
		return MutationReceipt{}, errors.Join(ErrUnavailable, err)
	}
	receipt, err := scanMutationReceipt(executor.QueryRow(ctx, completeManagementReceiptSQL, receiptID, snapshot, now.UTC()))
	if err != nil {
		return MutationReceipt{}, managementRepositoryError(err, false)
	}
	return receipt, nil
}

func (repository *Repository) managementExecutor(ctx context.Context, serviceProductID int64) (sqlExecutor, error) {
	if repository == nil || repository.executor == nil || ctx == nil || serviceProductID < 1 {
		return nil, ErrUnavailable
	}
	executor, err := repository.executor(ctx)
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	return executor, nil
}

func scanSavedView(row sqlRow) (SavedView, error) {
	var (
		view       SavedView
		state      string
		sort       string
		sourceView *int64
	)
	if err := row.Scan(&view.ID, &view.ServiceProductID, &view.Name, &state, &sort, &view.Columns, &sourceView,
		&view.Version, &view.CreatedBy, &view.CreatedAt, &view.UpdatedAt); err != nil {
		return SavedView{}, err
	}
	view.State = StateFilter(state)
	view.Sort = ViewSort(sort)
	view.SourceViewID = cloneOptionalID(sourceView)
	view = cloneSavedView(view)
	if !validSavedView(view) {
		return SavedView{}, ErrUnavailable
	}
	return view, nil
}

func scanCollaborator(row sqlRow) (Collaborator, error) {
	var (
		collaborator Collaborator
		permission   string
	)
	if err := row.Scan(&collaborator.ID, &collaborator.ServiceProductID, &collaborator.StaffID, &permission,
		&collaborator.Version, &collaborator.InvitedBy, &collaborator.CreatedAt, &collaborator.UpdatedAt); err != nil {
		return Collaborator{}, err
	}
	collaborator.Permission = CollaboratorPermission(permission)
	collaborator = cloneCollaborator(collaborator)
	if !validCollaborator(collaborator) {
		return Collaborator{}, ErrUnavailable
	}
	return collaborator, nil
}

func scanMutationReceipt(row sqlRow) (MutationReceipt, error) {
	var (
		receipt  MutationReceipt
		key      []byte
		payload  []byte
		snapshot []byte
	)
	if err := row.Scan(&receipt.ID, &receipt.Operation, &receipt.ActorScope, &key, &payload, &receipt.State, &snapshot); err != nil {
		return MutationReceipt{}, err
	}
	if len(key) != sha256DigestSize || len(payload) != sha256DigestSize {
		return MutationReceipt{}, ErrUnavailable
	}
	copy(receipt.KeyDigest[:], key)
	copy(receipt.PayloadDigest[:], payload)
	receipt.ResultSnapshot = append(json.RawMessage(nil), snapshot...)
	return receipt, nil
}

const sha256DigestSize = 32

func validCreateSavedViewRecord(record CreateSavedViewRecord) bool {
	return record.ServiceProductID > 0 && validViewName(record.Name) && record.State.valid() && record.Sort.valid() &&
		validColumnSelection(record.Columns) && (record.SourceViewID == nil || *record.SourceViewID > 0) &&
		record.CreatedBy > 0 && !record.CreatedAt.IsZero()
}

func validUpdateSavedViewRecord(record UpdateSavedViewRecord) bool {
	return record.ServiceProductID > 0 && record.ViewID > 0 && record.ExpectedVersion > 0 && validViewName(record.Name) &&
		record.State.valid() && record.Sort.valid() && validColumnSelection(record.Columns) && !record.UpdatedAt.IsZero()
}

func validCreateCollaboratorRecord(record CreateCollaboratorRecord) bool {
	return record.ServiceProductID > 0 && record.StaffID > 0 && record.Permission.valid() && record.InvitedBy > 0 && !record.CreatedAt.IsZero()
}

func validUpdateCollaboratorRecord(record UpdateCollaboratorRecord) bool {
	return record.ServiceProductID > 0 && record.CollaboratorID > 0 && record.ExpectedVersion > 0 &&
		record.Permission.valid() && !record.UpdatedAt.IsZero()
}

func managementRepositoryError(err error, notFound bool) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		if notFound {
			return ErrNotFound
		}
		return ErrConflict
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505", "23503", "23514":
			return errors.Join(ErrConflict, err)
		}
	}
	return errors.Join(ErrUnavailable, err)
}
