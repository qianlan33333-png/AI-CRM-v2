// Package store owns only Push Center's read projection. It deliberately has
// no dependency on any sending domain, worker, or provider.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	pushcenterapp "github.com/qianlan33333-png/AI-CRM-v2/internal/pushcenter/app"
	pushcenterdb "github.com/qianlan33333-png/AI-CRM-v2/internal/pushcenter/store/generated"
)

type Repository struct{}

var _ pushcenterapp.Repository = (*Repository)(nil)

func NewRepository() *Repository { return &Repository{} }

func (*Repository) ReadSummary(ctx context.Context, filter pushcenterapp.Filter) (pushcenterapp.Summary, error) {
	transaction, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return pushcenterapp.Summary{}, err
	}
	queries := pushcenterdb.New(transaction)
	state, err := queries.GetPushCenterReadModelState(ctx)
	if errors.Is(err, pgx.ErrNoRows) || err != nil || !state.ProductionDataReady {
		return pushcenterapp.Summary{}, pushcenterapp.ErrReadModelUnavailable
	}
	normalized := filter.Normalized()
	total, err := queries.CountPushCenterProjection(ctx, summaryParameters(normalized))
	if err != nil {
		return pushcenterapp.Summary{}, err
	}
	byStatus, err := queries.CountPushCenterProjectionByStatus(ctx, statusParameters(normalized))
	if err != nil {
		return pushcenterapp.Summary{}, err
	}
	byEffectiveStatus, err := queries.CountPushCenterProjectionByEffectiveStatus(ctx, effectiveStatusParameters(normalized))
	if err != nil {
		return pushcenterapp.Summary{}, err
	}
	bySection, err := queries.CountPushCenterProjectionBySection(ctx, sectionParameters(normalized))
	if err != nil {
		return pushcenterapp.Summary{}, err
	}
	result := pushcenterapp.Summary{AppliedFilter: normalized, Total: total,
		ByStatus: make(map[string]int64, len(byStatus)), ByEffectiveStatus: make(map[string]int64, len(byEffectiveStatus)),
		BySection: make(map[string]int64, len(bySection))}
	for _, row := range byStatus {
		result.ByStatus[row.Status] = row.Count
	}
	for _, row := range byEffectiveStatus {
		result.ByEffectiveStatus[row.EffectiveStatus] = row.Count
	}
	for _, row := range bySection {
		result.BySection[row.Section] = row.Count
	}
	return result, nil
}

func summaryParameters(filter pushcenterapp.Filter) pushcenterdb.CountPushCenterProjectionParams {
	return pushcenterdb.CountPushCenterProjectionParams{Section: filter.Section, EffectType: filter.EffectType, Status: filter.Status,
		BusinessType: filter.BusinessType, BusinessID: filter.BusinessID, TargetType: filter.TargetType, TargetID: filter.TargetID,
		ExternalUserid: filter.ExternalUserID, OwnerUserid: filter.OwnerUserID, TraceID: filter.TraceID,
		IdempotencyKey: filter.IdempotencyKey, SourceModule: filter.SourceModule, SourceRoute: filter.SourceRoute,
		CreatedFrom: filter.CreatedFrom, CreatedTo: filter.CreatedTo}
}

func statusParameters(filter pushcenterapp.Filter) pushcenterdb.CountPushCenterProjectionByStatusParams {
	return pushcenterdb.CountPushCenterProjectionByStatusParams{Section: filter.Section, EffectType: filter.EffectType, Status: filter.Status,
		BusinessType: filter.BusinessType, BusinessID: filter.BusinessID, TargetType: filter.TargetType, TargetID: filter.TargetID,
		ExternalUserid: filter.ExternalUserID, OwnerUserid: filter.OwnerUserID, TraceID: filter.TraceID,
		IdempotencyKey: filter.IdempotencyKey, SourceModule: filter.SourceModule, SourceRoute: filter.SourceRoute,
		CreatedFrom: filter.CreatedFrom, CreatedTo: filter.CreatedTo}
}

func effectiveStatusParameters(filter pushcenterapp.Filter) pushcenterdb.CountPushCenterProjectionByEffectiveStatusParams {
	return pushcenterdb.CountPushCenterProjectionByEffectiveStatusParams{Section: filter.Section, EffectType: filter.EffectType, Status: filter.Status,
		BusinessType: filter.BusinessType, BusinessID: filter.BusinessID, TargetType: filter.TargetType, TargetID: filter.TargetID,
		ExternalUserid: filter.ExternalUserID, OwnerUserid: filter.OwnerUserID, TraceID: filter.TraceID,
		IdempotencyKey: filter.IdempotencyKey, SourceModule: filter.SourceModule, SourceRoute: filter.SourceRoute,
		CreatedFrom: filter.CreatedFrom, CreatedTo: filter.CreatedTo}
}

func sectionParameters(filter pushcenterapp.Filter) pushcenterdb.CountPushCenterProjectionBySectionParams {
	return pushcenterdb.CountPushCenterProjectionBySectionParams{Section: filter.Section, EffectType: filter.EffectType, Status: filter.Status,
		BusinessType: filter.BusinessType, BusinessID: filter.BusinessID, TargetType: filter.TargetType, TargetID: filter.TargetID,
		ExternalUserid: filter.ExternalUserID, OwnerUserid: filter.OwnerUserID, TraceID: filter.TraceID,
		IdempotencyKey: filter.IdempotencyKey, SourceModule: filter.SourceModule, SourceRoute: filter.SourceRoute,
		CreatedFrom: filter.CreatedFrom, CreatedTo: filter.CreatedTo}
}
