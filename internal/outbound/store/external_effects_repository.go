package store

import (
	"context"
	"errors"
	"math"
	"reflect"

	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
)

// ExternalEffectsRepository is a safe adapter over the existing closed
// Outbound task read port. It does not query tables directly and deliberately
// drops every identity, payload, provider, receipt, error, and queue field
// before returning data to the External Effects application service.
type ExternalEffectsRepository struct {
	tasks outboundapp.TaskQueryStore
}

var _ outboundapp.ExternalEffectsReadStore = (*ExternalEffectsRepository)(nil)

func NewExternalEffectsRepository(tasks outboundapp.TaskQueryStore) (*ExternalEffectsRepository, error) {
	if nilExternalEffectsTaskStore(tasks) {
		return nil, outboundapp.ErrInvalidExternalEffectsConfiguration
	}
	return &ExternalEffectsRepository{tasks: tasks}, nil
}

func (repository *ExternalEffectsRepository) ListExternalEffectSources(
	ctx context.Context,
	query outboundapp.ExternalEffectStoreQuery,
) ([]outboundapp.ExternalEffectSource, error) {
	if ctx == nil || ctx.Err() != nil || repository == nil || nilExternalEffectsTaskStore(repository.tasks) ||
		query.Offset < 0 || query.Limit < 1 || query.Limit > outboundapp.TaskQueryMaximumLimit ||
		(query.Status != "" && !outboundapp.ExternalEffectStatusKnown(query.Status)) {
		return nil, outboundapp.ErrInvalidExternalEffectsQuery
	}

	models, err := repository.tasks.ListTasks(ctx, outboundapp.TaskListQuery{
		Status: query.Status,
		Limit:  query.Limit,
		Offset: query.Offset,
	})
	if err != nil {
		return nil, err
	}
	if len(models) > int(query.Limit) {
		return nil, errors.New("outbound task read port exceeded the external effects limit")
	}

	result := make([]outboundapp.ExternalEffectSource, len(models))
	for index, model := range models {
		result[index] = outboundapp.ExternalEffectSource{
			TaskID:          model.TaskID,
			Status:          model.Status,
			AttemptCount:    model.AttemptCount,
			CreatedAt:       model.CreatedAt.UTC(),
			StatusUpdatedAt: model.StatusUpdatedAt.UTC(),
		}
	}
	return result, nil
}

func (repository *ExternalEffectsRepository) CountExternalEffectStatuses(
	ctx context.Context,
) (outboundapp.ExternalEffectStatusCounts, error) {
	if ctx == nil || ctx.Err() != nil || repository == nil || nilExternalEffectsTaskStore(repository.tasks) {
		return outboundapp.ExternalEffectStatusCounts{}, outboundapp.ErrInvalidExternalEffectsQuery
	}

	var counts outboundapp.ExternalEffectStatusCounts
	var offset int32
	var previousTaskID outboundapp.TaskID
	for {
		sources, err := repository.ListExternalEffectSources(ctx, outboundapp.ExternalEffectStoreQuery{
			Offset: offset,
			Limit:  outboundapp.TaskQueryMaximumLimit,
		})
		if err != nil {
			return outboundapp.ExternalEffectStatusCounts{}, err
		}
		for _, source := range sources {
			if source.TaskID < 1 || !outboundapp.ExternalEffectStatusKnown(source.Status) ||
				source.CreatedAt.IsZero() || source.StatusUpdatedAt.IsZero() ||
				source.StatusUpdatedAt.Before(source.CreatedAt) ||
				(previousTaskID > 0 && source.TaskID >= previousTaskID) {
				return outboundapp.ExternalEffectStatusCounts{}, errors.New("invalid outbound task read facts while counting external effects")
			}
			if err = incrementExternalEffectStatusCount(&counts, source.Status); err != nil {
				return outboundapp.ExternalEffectStatusCounts{}, err
			}
			previousTaskID = source.TaskID
		}
		if len(sources) < int(outboundapp.TaskQueryMaximumLimit) {
			return counts, nil
		}
		if offset > math.MaxInt32-int32(len(sources)) {
			return outboundapp.ExternalEffectStatusCounts{}, errors.New("external effects count offset overflow")
		}
		offset += int32(len(sources))
	}
}

func incrementExternalEffectStatusCount(
	counts *outboundapp.ExternalEffectStatusCounts,
	status outboundapp.TaskStatus,
) error {
	if counts == nil {
		return errors.New("external effects status counts are required")
	}
	var target *int64
	switch status {
	case outboundapp.TaskStatusPending:
		target = &counts.Pending
	case outboundapp.TaskStatusSending:
		target = &counts.Sending
	case outboundapp.TaskStatusSent:
		target = &counts.Sent
	case outboundapp.TaskStatusRetryableFailed:
		target = &counts.RetryableFailed
	case outboundapp.TaskStatusFinalFailed:
		target = &counts.FinalFailed
	case outboundapp.TaskStatusOutcomeUnknown:
		target = &counts.OutcomeUnknown
	case outboundapp.TaskStatusCancelled:
		target = &counts.Cancelled
	default:
		return errors.New("unknown outbound task status in external effects count")
	}
	if *target == math.MaxInt64 {
		return errors.New("external effects status count overflow")
	}
	(*target)++
	return nil
}

func nilExternalEffectsTaskStore(value outboundapp.TaskQueryStore) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Pointer && reflected.IsNil()
}
