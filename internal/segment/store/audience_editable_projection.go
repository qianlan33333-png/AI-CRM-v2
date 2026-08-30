package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
)

type AudienceEditableProjectionStore struct{}

func NewAudienceEditableProjectionStore() *AudienceEditableProjectionStore {
	return &AudienceEditableProjectionStore{}
}

type editableAudienceGroup struct {
	historyID int64
	name      string
	createdAt time.Time
	updatedAt time.Time
}

type editableAudiencePackage struct {
	historyID     int64
	sourceID      int64
	groupHistory  sql.NullInt64
	name          string
	createdAt     time.Time
	updatedAt     time.Time
	refreshedAt   time.Time
	sourceMembers int64
	mappedMembers int64
}

func (store *AudienceEditableProjectionStore) ProjectActiveAudienceHistory(ctx context.Context, actorID int64, at time.Time) (segmentapp.AudienceEditableProjectionResult, error) {
	if store == nil || ctx == nil || actorID < 1 || at.IsZero() || at.Location() != time.UTC {
		return segmentapp.AudienceEditableProjectionResult{}, segmentapp.ErrAudienceEditableProjection
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return segmentapp.AudienceEditableProjectionResult{}, err
	}
	if _, err = tx.Exec(ctx, `LOCK TABLE public.ai_audience_v1_editable_group_projections, public.ai_audience_v1_editable_package_projections IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return segmentapp.AudienceEditableProjectionResult{}, err
	}
	result := segmentapp.AudienceEditableProjectionResult{}
	deferredKeys := segmentapp.DeferredRedesignAudiencePackageKeys()
	if err = tx.QueryRow(ctx, `
SELECT count(*)
FROM public.segment_v1_audience_packages AS package
WHERE original_status <> $1 OR package_key = ANY($2::text[])
   OR (SELECT count(*) FROM public.segment_v1_audience_members AS member WHERE member.package_history_id=package.id)
      <> (SELECT count(DISTINCT member.customer_id) FROM public.segment_v1_audience_members AS member WHERE member.package_history_id=package.id AND member.customer_id IS NOT NULL)`, "active", deferredKeys).Scan(&result.HistoryOnlyPreserved); err != nil {
		return segmentapp.AudienceEditableProjectionResult{}, err
	}
	groups, err := loadEditableAudienceGroups(ctx, tx, deferredKeys)
	if err != nil {
		return segmentapp.AudienceEditableProjectionResult{}, err
	}
	groupIDs := make(map[int64]int64, len(groups))
	for _, group := range groups {
		groupID, replayed, projectErr := projectAudienceGroup(ctx, tx, group, actorID, at)
		if projectErr != nil {
			return segmentapp.AudienceEditableProjectionResult{}, projectErr
		}
		groupIDs[group.historyID] = groupID
		if replayed {
			result.GroupsReplayed++
		} else {
			result.GroupsCreated++
		}
	}
	packages, err := loadEditableAudiencePackages(ctx, tx, deferredKeys)
	if err != nil {
		return segmentapp.AudienceEditableProjectionResult{}, err
	}
	for _, item := range packages {
		var groupID *int64
		if item.groupHistory.Valid {
			mapped, ok := groupIDs[item.groupHistory.Int64]
			if !ok {
				return segmentapp.AudienceEditableProjectionResult{}, segmentapp.ErrAudienceEditableProjection
			}
			groupID = &mapped
		}
		replayed, projectErr := projectAudiencePackage(ctx, tx, item, groupID, actorID, at)
		if projectErr != nil {
			return segmentapp.AudienceEditableProjectionResult{}, projectErr
		}
		if replayed {
			result.PackagesReplayed++
		} else {
			result.PackagesCreated++
			result.MembersProjected += int(item.mappedMembers)
		}
	}
	return result, nil
}

func loadEditableAudienceGroups(ctx context.Context, tx pgx.Tx, deferredKeys []string) ([]editableAudienceGroup, error) {
	rows, err := tx.Query(ctx, `
SELECT DISTINCT history.id, history.name, history.created_at, history.updated_at
FROM public.segment_v1_audience_groups AS history
JOIN public.segment_v1_audience_packages AS package ON package.group_history_id = history.id
WHERE package.original_status = $1
  AND package.package_key <> ALL($2::text[])
  AND (SELECT count(*) FROM public.segment_v1_audience_members AS member WHERE member.package_history_id=package.id)
      = (SELECT count(DISTINCT member.customer_id) FROM public.segment_v1_audience_members AS member WHERE member.package_history_id=package.id AND member.customer_id IS NOT NULL)
ORDER BY history.id`, "active", deferredKeys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []editableAudienceGroup{}
	for rows.Next() {
		var item editableAudienceGroup
		if err = rows.Scan(&item.historyID, &item.name, &item.createdAt, &item.updatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func projectAudienceGroup(ctx context.Context, tx pgx.Tx, item editableAudienceGroup, actorID int64, at time.Time) (int64, bool, error) {
	var groupID int64
	err := tx.QueryRow(ctx, `
SELECT projection.group_id
FROM public.ai_audience_v1_editable_group_projections AS projection
JOIN public.ai_audience_package_groups AS current_group ON current_group.id = projection.group_id
WHERE projection.group_history_id = $1`, item.historyID).Scan(&groupID)
	if err == nil {
		return groupID, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, err
	}
	if item.name == "" || item.createdAt.IsZero() || item.updatedAt.Before(item.createdAt) {
		return 0, false, segmentapp.ErrAudienceEditableProjection
	}
	err = tx.QueryRow(ctx, `
INSERT INTO public.ai_audience_package_groups(name, sort_order, version, created_by, created_at, updated_at)
VALUES ($1, 0, 1, $2, $3, $4)
RETURNING id`, item.name, actorID, item.createdAt, item.updatedAt).Scan(&groupID)
	if err != nil {
		return 0, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO public.ai_audience_v1_editable_group_projections(group_history_id, group_id, created_at) VALUES ($1, $2, $3)`, item.historyID, groupID, at); err != nil {
		return 0, false, err
	}
	return groupID, false, nil
}

func loadEditableAudiencePackages(ctx context.Context, tx pgx.Tx, deferredKeys []string) ([]editableAudiencePackage, error) {
	rows, err := tx.Query(ctx, `
SELECT package.id, package.source_id, package.group_history_id, package.name,
       package.created_at, package.updated_at,
       COALESCE(package.last_daily_refreshed_at, package.last_incremental_at, package.updated_at),
       count(member.id), count(DISTINCT member.customer_id)
FROM public.segment_v1_audience_packages AS package
LEFT JOIN public.segment_v1_audience_members AS member ON member.package_history_id = package.id
WHERE package.original_status = $1
  AND package.package_key <> ALL($2::text[])
GROUP BY package.id
HAVING count(member.id) = count(DISTINCT member.customer_id)
ORDER BY package.id`, "active", deferredKeys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []editableAudiencePackage{}
	for rows.Next() {
		var item editableAudiencePackage
		if err = rows.Scan(&item.historyID, &item.sourceID, &item.groupHistory, &item.name, &item.createdAt, &item.updatedAt, &item.refreshedAt, &item.sourceMembers, &item.mappedMembers); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func projectAudiencePackage(ctx context.Context, tx pgx.Tx, item editableAudiencePackage, groupID *int64, actorID int64, at time.Time) (bool, error) {
	if item.sourceID < 1 || item.name == "" || item.createdAt.IsZero() || item.updatedAt.Before(item.createdAt) || item.sourceMembers != item.mappedMembers {
		return false, segmentapp.ErrAudienceEditableProjection
	}
	var segmentID int64
	err := tx.QueryRow(ctx, `
SELECT projection.segment_id
FROM public.ai_audience_v1_editable_package_projections AS projection
JOIN public.ai_audience_package_metadata AS metadata ON metadata.segment_id = projection.segment_id
WHERE projection.package_history_id = $1`, item.historyID).Scan(&segmentID)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	definition, err := json.Marshal(map[string]any{"field": "legacy_audience_package_source_id", "op": "eq", "value": item.sourceID})
	if err != nil {
		return false, err
	}
	err = tx.QueryRow(ctx, `
INSERT INTO public.segments
  (name, definition, refresh_mode, refresh_cron, member_count, refreshed_at, refresh_status,
   created_by, created_at, updated_at, lifecycle_status, archived_at, archived_by)
VALUES ($1, $2, 'manual', NULL, $3, $4, 'idle', $5, $6, $7, 'active', NULL, NULL)
RETURNING id`, item.name, definition, item.mappedMembers, item.refreshedAt, actorID, item.createdAt, item.updatedAt).Scan(&segmentID)
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO public.ai_audience_package_metadata
  (segment_id, group_id, lifecycle, version, created_by, updated_by, created_at, updated_at)
VALUES ($1, $2, 'paused', 1, $3, $3, $4, $5)`, segmentID, groupID, actorID, item.createdAt, item.updatedAt); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO public.segment_members(segment_id, customer_id, computed_at)
SELECT $1, member.customer_id, max(member.last_updated_at)
FROM public.segment_v1_audience_members AS member
WHERE member.package_history_id = $2 AND member.customer_id IS NOT NULL
GROUP BY member.customer_id
ORDER BY member.customer_id`, segmentID, item.historyID); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO public.ai_audience_v1_editable_package_projections
  (package_history_id, segment_id, source_member_count, mapped_member_count, created_at)
VALUES ($1, $2, $3, $4, $5)`, item.historyID, segmentID, item.sourceMembers, item.mappedMembers, at); err != nil {
		return false, err
	}
	return false, nil
}
