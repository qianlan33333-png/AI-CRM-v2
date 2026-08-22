package legacyaudience

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

// SQLRow/SQLRows/SQLExecutor deliberately mirror the small database surface
// needed by this package without importing a concrete driver. Lane E supplies
// adapters for pgxpool reads and the platform transaction carried in context.
type SQLRow interface {
	Scan(...any) error
}

type SQLRows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}

type SQLExecutor interface {
	Exec(context.Context, string, ...any) (int64, error)
	Query(context.Context, string, ...any) (SQLRows, error)
	QueryRow(context.Context, string, ...any) SQLRow
}

type SQLProvider interface {
	Reader(context.Context) (SQLExecutor, error)
	Transaction(context.Context) (SQLExecutor, error)
	IsNoRows(error) bool
}

var _ Repository = (*SQLRepository)(nil)

type SQLRepository struct {
	provider SQLProvider
}

func NewSQLRepository(provider SQLProvider) (*SQLRepository, error) {
	if nilInterface(provider) {
		return nil, ErrUnavailable
	}
	return &SQLRepository{provider: provider}, nil
}

const groupColumns = `id, name, sort_order, version, created_by, created_at, updated_at`
const metadataColumns = `segment_id, group_id, lifecycle, version, created_by, updated_by, created_at, updated_at`

func (repository *SQLRepository) ListGroups(ctx context.Context) ([]Group, error) {
	database, err := repository.reader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := database.Query(ctx, `
SELECT `+groupColumns+`
FROM public.ai_audience_package_groups
ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, classifySQLError(err)
	}
	defer rows.Close()
	groups := make([]Group, 0)
	for rows.Next() {
		group, scanErr := scanGroup(rows)
		if scanErr != nil {
			return nil, classifySQLError(scanErr)
		}
		groups = append(groups, group)
	}
	if err = rows.Err(); err != nil {
		return nil, classifySQLError(err)
	}
	return groups, nil
}

func (repository *SQLRepository) LockGroup(ctx context.Context, groupID int64) (Group, error) {
	database, err := repository.transaction(ctx)
	if err != nil {
		return Group{}, err
	}
	group, err := scanGroup(database.QueryRow(ctx, `
SELECT `+groupColumns+`
FROM public.ai_audience_package_groups
WHERE id = $1
FOR UPDATE`, groupID))
	if err != nil {
		return Group{}, repository.notFound(err)
	}
	return group, nil
}

func (repository *SQLRepository) InsertGroup(ctx context.Context, name string, sortOrder int32, actorID int64, now time.Time) (Group, error) {
	database, err := repository.transaction(ctx)
	if err != nil {
		return Group{}, err
	}
	group, err := scanGroup(database.QueryRow(ctx, `
INSERT INTO public.ai_audience_package_groups
  (name, sort_order, version, created_by, created_at, updated_at)
VALUES ($1, $2, 1, $3, $4, $4)
RETURNING `+groupColumns, name, sortOrder, actorID, now))
	if err != nil {
		return Group{}, classifySQLError(err)
	}
	return group, nil
}

func (repository *SQLRepository) UpdateGroup(ctx context.Context, current Group, name string, sortOrder int32, _ int64, now time.Time) (Group, error) {
	database, err := repository.transaction(ctx)
	if err != nil {
		return Group{}, err
	}
	group, err := scanGroup(database.QueryRow(ctx, `
UPDATE public.ai_audience_package_groups
SET name = $1,
    sort_order = $2,
    version = version + 1,
    updated_at = $3
WHERE id = $4 AND version = $5
RETURNING `+groupColumns, name, sortOrder, now, current.ID, current.Version))
	if err != nil {
		if repository.provider.IsNoRows(err) {
			return Group{}, ErrVersionConflict
		}
		return Group{}, classifySQLError(err)
	}
	return group, nil
}

func (repository *SQLRepository) CountPackagesInGroup(ctx context.Context, groupID int64) (int64, error) {
	database, err := repository.transaction(ctx)
	if err != nil {
		return 0, err
	}
	var count int64
	if err = database.QueryRow(ctx, `
SELECT count(*)
FROM public.ai_audience_package_metadata
WHERE group_id = $1`, groupID).Scan(&count); err != nil {
		return 0, classifySQLError(err)
	}
	return count, nil
}

func (repository *SQLRepository) DeleteGroup(ctx context.Context, groupID int64, expectedVersion int64) error {
	database, err := repository.transaction(ctx)
	if err != nil {
		return err
	}
	rows, err := database.Exec(ctx, `
DELETE FROM public.ai_audience_package_groups
WHERE id = $1 AND version = $2`, groupID, expectedVersion)
	if err != nil {
		return classifySQLError(err)
	}
	if rows != 1 {
		return ErrVersionConflict
	}
	return nil
}

func (repository *SQLRepository) ListPackageMetadata(ctx context.Context, groupID *int64, limit, offset int) ([]PackageMetadata, int64, error) {
	database, err := repository.reader(ctx)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if groupID == nil {
		err = database.QueryRow(ctx, `SELECT count(*) FROM public.ai_audience_package_metadata`).Scan(&total)
	} else {
		err = database.QueryRow(ctx, `SELECT count(*) FROM public.ai_audience_package_metadata WHERE group_id = $1`, *groupID).Scan(&total)
	}
	if err != nil {
		return nil, 0, classifySQLError(err)
	}
	var rows SQLRows
	if groupID == nil {
		rows, err = database.Query(ctx, `
SELECT `+metadataColumns+`
FROM public.ai_audience_package_metadata
ORDER BY segment_id ASC
LIMIT $1 OFFSET $2`, limit, offset)
	} else {
		rows, err = database.Query(ctx, `
SELECT `+metadataColumns+`
FROM public.ai_audience_package_metadata
WHERE group_id = $1
ORDER BY segment_id ASC
LIMIT $2 OFFSET $3`, *groupID, limit, offset)
	}
	if err != nil {
		return nil, 0, classifySQLError(err)
	}
	defer rows.Close()
	items := make([]PackageMetadata, 0, limit)
	for rows.Next() {
		metadata, scanErr := scanMetadata(rows)
		if scanErr != nil {
			return nil, 0, classifySQLError(scanErr)
		}
		items = append(items, metadata)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, classifySQLError(err)
	}
	return items, total, nil
}

func (repository *SQLRepository) GetPackageMetadata(ctx context.Context, packageID int64) (PackageMetadata, error) {
	database, err := repository.reader(ctx)
	if err != nil {
		return PackageMetadata{}, err
	}
	metadata, err := scanMetadata(database.QueryRow(ctx, `
SELECT `+metadataColumns+`
FROM public.ai_audience_package_metadata
WHERE segment_id = $1`, packageID))
	if err != nil {
		return PackageMetadata{}, repository.notFound(err)
	}
	return metadata, nil
}

func (repository *SQLRepository) LockPackage(ctx context.Context, packageID int64) (PackageWriteModel, error) {
	database, err := repository.transaction(ctx)
	if err != nil {
		return PackageWriteModel{}, err
	}
	var model PackageWriteModel
	var definition []byte
	var refreshMode string
	var refreshCron sql.NullString
	var groupID sql.NullInt64
	var segmentLifecycle string
	err = database.QueryRow(ctx, `
SELECT s.id, s.name, s.definition, s.refresh_mode, s.refresh_cron, s.lifecycle_status,
       m.segment_id, m.group_id, m.lifecycle, m.version, m.created_by, m.updated_by, m.created_at, m.updated_at
FROM public.ai_audience_package_metadata AS m
JOIN public.segments AS s ON s.id = m.segment_id
WHERE m.segment_id = $1
FOR UPDATE OF s, m`, packageID).Scan(
		&model.SegmentID, &model.Name, &definition, &refreshMode, &refreshCron, &segmentLifecycle,
		&model.Metadata.SegmentID, &groupID, &model.Metadata.Lifecycle, &model.Metadata.Version,
		&model.Metadata.CreatedBy, &model.Metadata.UpdatedBy, &model.Metadata.CreatedAt, &model.Metadata.UpdatedAt,
	)
	if err != nil {
		return PackageWriteModel{}, repository.notFound(err)
	}
	model.Definition = segmentport.Definition(append([]byte(nil), definition...))
	model.RefreshMode = segmentport.RefreshMode(refreshMode)
	model.RefreshCron = nullableString(refreshCron)
	model.Metadata.GroupID = nullableInt64(groupID)
	model.SegmentLifecycle = segmentport.LifecycleStatus(segmentLifecycle)
	return model, nil
}

func (repository *SQLRepository) SavePackage(
	ctx context.Context,
	current PackageWriteModel,
	next PackageWriteModel,
	expectedVersion int64,
	actorID int64,
	now time.Time,
) (PackageWriteModel, error) {
	database, err := repository.transaction(ctx)
	if err != nil {
		return PackageWriteModel{}, err
	}
	archivedBy := "admin:" + strconv.FormatInt(actorID, 10)
	rows, err := database.Exec(ctx, `
UPDATE public.segments
SET name = $1,
    definition = $2,
    refresh_mode = $3,
    refresh_cron = $4,
    lifecycle_status = $5,
    archived_at = CASE WHEN $5 = 'archived' THEN COALESCE(archived_at, $6) ELSE archived_at END,
    archived_by = CASE WHEN $5 = 'archived' THEN COALESCE(archived_by, $7) ELSE archived_by END,
    updated_at = $6
WHERE id = $8 AND lifecycle_status = $9`,
		next.Name, []byte(next.Definition), string(next.RefreshMode), next.RefreshCron, string(next.SegmentLifecycle),
		now, archivedBy, current.SegmentID, string(current.SegmentLifecycle))
	if err != nil {
		return PackageWriteModel{}, classifySQLError(err)
	}
	if rows != 1 {
		return PackageWriteModel{}, ErrVersionConflict
	}
	metadata, err := scanMetadata(database.QueryRow(ctx, `
UPDATE public.ai_audience_package_metadata
SET group_id = $1,
    lifecycle = $2,
    version = version + 1,
    updated_by = $3,
    updated_at = $4
WHERE segment_id = $5 AND version = $6
RETURNING `+metadataColumns,
		next.Metadata.GroupID, next.Metadata.Lifecycle, actorID, now, current.SegmentID, expectedVersion))
	if err != nil {
		if repository.provider.IsNoRows(err) {
			return PackageWriteModel{}, ErrVersionConflict
		}
		return PackageWriteModel{}, classifySQLError(err)
	}
	result := cloneWriteModel(next)
	result.Metadata = metadata
	return result, nil
}

func (repository *SQLRepository) LockCopyNameNamespace(ctx context.Context, _ string) error {
	database, err := repository.transaction(ctx)
	if err != nil {
		return err
	}
	// All AI Audience copies share one transaction-scoped namespace lock. A
	// source-specific lock is insufficient because truncation can make distinct
	// source names produce the same deterministic copy candidate.
	_, err = database.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('ai_audience.package.copy.name.v1', 0))`)
	return classifySQLError(err)
}

func (repository *SQLRepository) PackageNameExists(ctx context.Context, name string) (bool, error) {
	database, err := repository.transaction(ctx)
	if err != nil {
		return false, err
	}
	var exists bool
	if err = database.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM public.segments WHERE lower(name) = lower($1)
)`, name).Scan(&exists); err != nil {
		return false, classifySQLError(err)
	}
	return exists, nil
}

func (repository *SQLRepository) InsertPackageCopy(ctx context.Context, source PackageWriteModel, name string, actorID int64, now time.Time) (PackageWriteModel, error) {
	database, err := repository.transaction(ctx)
	if err != nil {
		return PackageWriteModel{}, err
	}
	var result PackageWriteModel
	var definition []byte
	var refreshMode string
	var refreshCron sql.NullString
	var segmentLifecycle string
	err = database.QueryRow(ctx, `
INSERT INTO public.segments
  (name, definition, refresh_mode, refresh_cron, member_count, refreshed_at, refresh_status,
   created_by, created_at, updated_at, lifecycle_status, archived_at, archived_by)
SELECT $1, definition, refresh_mode, refresh_cron, 0, NULL, 'idle',
       $2, $3, $3, 'active', NULL, NULL
FROM public.segments
WHERE id = $4 AND lifecycle_status = 'active'
RETURNING id, name, definition, refresh_mode, refresh_cron, lifecycle_status`,
		name, actorID, now, source.SegmentID).Scan(
		&result.SegmentID, &result.Name, &definition, &refreshMode, &refreshCron, &segmentLifecycle,
	)
	if err != nil {
		if repository.provider.IsNoRows(err) {
			return PackageWriteModel{}, ErrArchived
		}
		return PackageWriteModel{}, classifySQLError(err)
	}
	result.Definition = segmentport.Definition(append([]byte(nil), definition...))
	result.RefreshMode = segmentport.RefreshMode(refreshMode)
	result.RefreshCron = nullableString(refreshCron)
	result.SegmentLifecycle = segmentport.LifecycleStatus(segmentLifecycle)
	metadata, metadataErr := scanMetadata(database.QueryRow(ctx, `
INSERT INTO public.ai_audience_package_metadata
  (segment_id, group_id, lifecycle, version, created_by, updated_by, created_at, updated_at)
VALUES ($1, $2, 'paused', 1, $3, $3, $4, $4)
RETURNING `+metadataColumns,
		result.SegmentID, source.Metadata.GroupID, actorID, now))
	if metadataErr != nil {
		return PackageWriteModel{}, classifySQLError(metadataErr)
	}
	result.Metadata = metadata
	return result, nil
}

func (repository *SQLRepository) ReserveReceipt(ctx context.Context, wanted ReceiptReservation) (Receipt, bool, error) {
	database, err := repository.transaction(ctx)
	if err != nil {
		return Receipt{}, false, err
	}
	receipt, err := scanReceipt(database.QueryRow(ctx, `
INSERT INTO public.ai_audience_operation_receipts
  (operation, actor_id, key_digest, payload_digest, state, created_at)
VALUES ($1, $2, $3, $4, 'in_progress', $5)
ON CONFLICT (operation, actor_id, key_digest) DO NOTHING
RETURNING id, operation, actor_id, key_digest, payload_digest, state, result_json`,
		wanted.Operation, wanted.ActorID, wanted.KeyDigest[:], wanted.PayloadDigest[:], wanted.CreatedAt))
	if err == nil {
		return receipt, true, nil
	}
	if !repository.provider.IsNoRows(err) {
		return Receipt{}, false, classifySQLError(err)
	}
	receipt, err = scanReceipt(database.QueryRow(ctx, `
SELECT id, operation, actor_id, key_digest, payload_digest, state, result_json
FROM public.ai_audience_operation_receipts
WHERE operation = $1 AND actor_id = $2 AND key_digest = $3
FOR UPDATE`, wanted.Operation, wanted.ActorID, wanted.KeyDigest[:]))
	if err != nil {
		return Receipt{}, false, classifySQLError(err)
	}
	return receipt, false, nil
}

func (repository *SQLRepository) CompleteReceipt(ctx context.Context, receiptID int64, result json.RawMessage, now time.Time) (Receipt, error) {
	database, err := repository.transaction(ctx)
	if err != nil {
		return Receipt{}, err
	}
	receipt, err := scanReceipt(database.QueryRow(ctx, `
UPDATE public.ai_audience_operation_receipts
SET state = 'completed', result_json = $1, completed_at = $2
WHERE id = $3 AND state = 'in_progress'
RETURNING id, operation, actor_id, key_digest, payload_digest, state, result_json`, []byte(result), now, receiptID))
	if err != nil {
		if repository.provider.IsNoRows(err) {
			return Receipt{}, ErrConflict
		}
		return Receipt{}, classifySQLError(err)
	}
	return receipt, nil
}

func (repository *SQLRepository) GetAutomationBinding(ctx context.Context, packageID int64) (*AutomationBinding, error) {
	database, err := repository.reader(ctx)
	if err != nil {
		return nil, err
	}
	binding, err := scanAutomationBinding(database.QueryRow(ctx, `
SELECT package_id, automation_agent_id, created_by, updated_by, created_at, updated_at
FROM public.ai_audience_package_automation_bindings
WHERE package_id = $1`, packageID))
	if err != nil {
		if repository.provider.IsNoRows(err) {
			return nil, nil
		}
		return nil, classifySQLError(err)
	}
	return &binding, nil
}

func (repository *SQLRepository) SaveAutomationBinding(ctx context.Context, value AutomationBinding, actorID int64, now time.Time) (AutomationBinding, error) {
	database, err := repository.transaction(ctx)
	if err != nil {
		return AutomationBinding{}, err
	}
	binding, err := scanAutomationBinding(database.QueryRow(ctx, `
INSERT INTO public.ai_audience_package_automation_bindings
  (package_id, automation_agent_id, created_by, updated_by, created_at, updated_at)
VALUES ($1, $2, $3, $3, $4, $4)
ON CONFLICT (package_id) DO UPDATE
SET automation_agent_id = EXCLUDED.automation_agent_id,
    updated_by = EXCLUDED.updated_by,
    updated_at = EXCLUDED.updated_at
RETURNING package_id, automation_agent_id, created_by, updated_by, created_at, updated_at`,
		value.PackageID, value.AutomationAgentID, actorID, now))
	if err != nil {
		return AutomationBinding{}, classifySQLError(err)
	}
	return binding, nil
}

func (repository *SQLRepository) DeleteAutomationBinding(ctx context.Context, packageID int64) (bool, error) {
	database, err := repository.transaction(ctx)
	if err != nil {
		return false, err
	}
	rows, err := database.Exec(ctx, `
DELETE FROM public.ai_audience_package_automation_bindings
WHERE package_id = $1`, packageID)
	if err != nil {
		return false, classifySQLError(err)
	}
	if rows > 1 {
		return false, ErrUnavailable
	}
	return rows == 1, nil
}

func (repository *SQLRepository) ListPackageSenders(ctx context.Context, packageID int64) ([]PackageSender, error) {
	database, err := repository.reader(ctx)
	if err != nil {
		return nil, err
	}
	return listPackageSenders(ctx, database, packageID, false)
}

func (repository *SQLRepository) ReplacePackageSenders(
	ctx context.Context,
	packageID int64,
	wanted []PackageSender,
	actorID int64,
	now time.Time,
) ([]PackageSender, bool, error) {
	database, err := repository.transaction(ctx)
	if err != nil {
		return nil, false, err
	}
	// There may be no sender rows yet, so row locks alone cannot serialize two
	// first writers. The package-scoped transaction advisory lock covers the
	// exact full-replacement set without blocking another package.
	if _, err = database.Exec(ctx, `
SELECT pg_advisory_xact_lock(hashtextextended('ai_audience.package.senders.v1:' || $1::text, 0))`, packageID); err != nil {
		return nil, false, classifySQLError(err)
	}
	current, err := listPackageSenders(ctx, database, packageID, true)
	if err != nil {
		return nil, false, err
	}
	if samePackageSenders(current, wanted) {
		return current, false, nil
	}
	if _, err = database.Exec(ctx, `DELETE FROM public.ai_audience_package_senders WHERE package_id = $1`, packageID); err != nil {
		return nil, false, classifySQLError(err)
	}
	for _, item := range wanted {
		if _, err = database.Exec(ctx, `
INSERT INTO public.ai_audience_package_senders
  (package_id, sender_userid, sort_order, is_enabled, created_by, updated_by, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5, $6, $6)`,
			packageID, item.SenderUserID, item.SortOrder, item.IsEnabled, actorID, now); err != nil {
			return nil, false, classifySQLError(err)
		}
	}
	stored, err := listPackageSenders(ctx, database, packageID, true)
	if err != nil {
		return nil, false, err
	}
	return stored, true, nil
}

func (repository *SQLRepository) ListEligibleSenderUserIDs(ctx context.Context, userIDs []string) ([]string, error) {
	database, err := repository.reader(ctx)
	if err != nil {
		return nil, err
	}
	return listEligibleSenderUserIDs(ctx, database, userIDs, false)
}

func (repository *SQLRepository) LockEligibleSenderUserIDs(ctx context.Context, userIDs []string) ([]string, error) {
	database, err := repository.transaction(ctx)
	if err != nil {
		return nil, err
	}
	return listEligibleSenderUserIDs(ctx, database, userIDs, true)
}

func (repository *SQLRepository) ReserveConfigurationReceipt(ctx context.Context, wanted ReceiptReservation) (Receipt, bool, error) {
	database, err := repository.transaction(ctx)
	if err != nil {
		return Receipt{}, false, err
	}
	receipt, err := scanReceipt(database.QueryRow(ctx, `
INSERT INTO public.ai_audience_local_configuration_receipts
  (operation, actor_id, key_digest, payload_digest, state, created_at)
VALUES ($1, $2, $3, $4, 'in_progress', $5)
ON CONFLICT (operation, actor_id, key_digest) DO NOTHING
RETURNING id, operation, actor_id, key_digest, payload_digest, state, result_json`,
		wanted.Operation, wanted.ActorID, wanted.KeyDigest[:], wanted.PayloadDigest[:], wanted.CreatedAt))
	if err == nil {
		return receipt, true, nil
	}
	if !repository.provider.IsNoRows(err) {
		return Receipt{}, false, classifySQLError(err)
	}
	receipt, err = scanReceipt(database.QueryRow(ctx, `
SELECT id, operation, actor_id, key_digest, payload_digest, state, result_json
FROM public.ai_audience_local_configuration_receipts
WHERE operation = $1 AND actor_id = $2 AND key_digest = $3
FOR UPDATE`, wanted.Operation, wanted.ActorID, wanted.KeyDigest[:]))
	if err != nil {
		return Receipt{}, false, classifySQLError(err)
	}
	return receipt, false, nil
}

func (repository *SQLRepository) CompleteConfigurationReceipt(ctx context.Context, receiptID int64, result json.RawMessage, now time.Time) (Receipt, error) {
	database, err := repository.transaction(ctx)
	if err != nil {
		return Receipt{}, err
	}
	receipt, err := scanReceipt(database.QueryRow(ctx, `
UPDATE public.ai_audience_local_configuration_receipts
SET state = 'completed', result_json = $1, completed_at = $2
WHERE id = $3 AND state = 'in_progress'
RETURNING id, operation, actor_id, key_digest, payload_digest, state, result_json`, []byte(result), now, receiptID))
	if err != nil {
		if repository.provider.IsNoRows(err) {
			return Receipt{}, ErrConflict
		}
		return Receipt{}, classifySQLError(err)
	}
	return receipt, nil
}

func listPackageSenders(ctx context.Context, database SQLExecutor, packageID int64, lock bool) ([]PackageSender, error) {
	query := `
SELECT sender_userid, sort_order, is_enabled
FROM public.ai_audience_package_senders
WHERE package_id = $1
ORDER BY sort_order ASC, sender_userid ASC`
	if lock {
		query += ` FOR UPDATE`
	}
	rows, err := database.Query(ctx, query, packageID)
	if err != nil {
		return nil, classifySQLError(err)
	}
	defer rows.Close()
	items := make([]PackageSender, 0, MaximumSenderCount)
	for rows.Next() {
		var item PackageSender
		if err = rows.Scan(&item.SenderUserID, &item.SortOrder, &item.IsEnabled); err != nil {
			return nil, classifySQLError(err)
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, classifySQLError(err)
	}
	if validateErr := validatePackageSenders(items); validateErr != nil {
		return nil, ErrUnavailable
	}
	return items, nil
}

func listEligibleSenderUserIDs(ctx context.Context, database SQLExecutor, userIDs []string, lock bool) ([]string, error) {
	if len(userIDs) == 0 {
		return []string{}, nil
	}
	query := `
SELECT wecom_userid
FROM public.staff
WHERE is_active
  AND btrim(wecom_userid) <> ''
  AND wecom_userid = ANY($1::text[])
ORDER BY wecom_userid ASC`
	if lock {
		// FOR SHARE conflicts with the row lock taken by UPDATE/DELETE, so an
		// in-flight staff deactivation or identifier change cannot pass between
		// validation and the sender replacement in this same transaction.
		query += ` FOR SHARE`
	}
	rows, err := database.Query(ctx, query, userIDs)
	if err != nil {
		return nil, classifySQLError(err)
	}
	defer rows.Close()
	items := make([]string, 0, len(userIDs))
	for rows.Next() {
		var userID string
		if err = rows.Scan(&userID); err != nil {
			return nil, classifySQLError(err)
		}
		items = append(items, userID)
	}
	if err = rows.Err(); err != nil {
		return nil, classifySQLError(err)
	}
	return items, nil
}

func scanAutomationBinding(row interface{ Scan(...any) error }) (AutomationBinding, error) {
	var binding AutomationBinding
	err := row.Scan(
		&binding.PackageID,
		&binding.AutomationAgentID,
		&binding.CreatedBy,
		&binding.UpdatedBy,
		&binding.CreatedAt,
		&binding.UpdatedAt,
	)
	return binding, err
}

func scanGroup(row interface{ Scan(...any) error }) (Group, error) {
	var group Group
	err := row.Scan(&group.ID, &group.Name, &group.SortOrder, &group.Version, &group.CreatedBy, &group.CreatedAt, &group.UpdatedAt)
	return group, err
}

func scanMetadata(row interface{ Scan(...any) error }) (PackageMetadata, error) {
	var metadata PackageMetadata
	var groupID sql.NullInt64
	err := row.Scan(
		&metadata.SegmentID, &groupID, &metadata.Lifecycle, &metadata.Version,
		&metadata.CreatedBy, &metadata.UpdatedBy, &metadata.CreatedAt, &metadata.UpdatedAt,
	)
	metadata.GroupID = nullableInt64(groupID)
	return metadata, err
}

func nullableInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	copy := value.Int64
	return &copy
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}

func scanReceipt(row interface{ Scan(...any) error }) (Receipt, error) {
	var receipt Receipt
	var keyDigest []byte
	var payloadDigest []byte
	var result []byte
	err := row.Scan(
		&receipt.ID, &receipt.Operation, &receipt.ActorID, &keyDigest, &payloadDigest, &receipt.State, &result,
	)
	if err != nil {
		return Receipt{}, err
	}
	if len(keyDigest) != sha256Size || len(payloadDigest) != sha256Size {
		return Receipt{}, ErrUnavailable
	}
	copy(receipt.KeyDigest[:], keyDigest)
	copy(receipt.PayloadDigest[:], payloadDigest)
	receipt.ResultJSON = append(json.RawMessage(nil), result...)
	return receipt, nil
}

const sha256Size = 32

func (repository *SQLRepository) reader(ctx context.Context) (SQLExecutor, error) {
	if repository == nil || nilInterface(repository.provider) || ctx == nil {
		return nil, ErrUnavailable
	}
	database, err := repository.provider.Reader(ctx)
	if err != nil || nilInterface(database) {
		return nil, errors.Join(ErrUnavailable, err)
	}
	return database, nil
}

func (repository *SQLRepository) transaction(ctx context.Context) (SQLExecutor, error) {
	if repository == nil || nilInterface(repository.provider) || ctx == nil {
		return nil, ErrUnavailable
	}
	database, err := repository.provider.Transaction(ctx)
	if err != nil || nilInterface(database) {
		return nil, errors.Join(ErrUnavailable, err)
	}
	return database, nil
}

func (repository *SQLRepository) notFound(err error) error {
	if repository != nil && !nilInterface(repository.provider) && repository.provider.IsNoRows(err) {
		return ErrNotFound
	}
	return classifySQLError(err)
}

type sqlStateError interface {
	SQLState() string
}

func classifySQLError(err error) error {
	if err == nil {
		return nil
	}
	var state sqlStateError
	if errors.As(err, &state) {
		switch state.SQLState() {
		case "23505", "23503":
			return errors.Join(ErrConflict, err)
		}
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrConflict) || errors.Is(err, ErrVersionConflict) || errors.Is(err, ErrArchived) {
		return err
	}
	return errors.Join(ErrUnavailable, err)
}
