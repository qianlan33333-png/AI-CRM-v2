package app

import (
	"context"
	"errors"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

type ExternalPushLog struct {
	BindingID        int64         `json:"binding_id,string"`
	QuestionnaireID  surveyport.ID `json:"questionnaire_id,string"`
	SubmissionID     int64         `json:"submission_id,string"`
	CustomerID       int64         `json:"customer_id,string"`
	EffectID         string        `json:"external_effect_id"`
	State            eer.State     `json:"state"`
	AttemptCount     int32         `json:"attempt_count"`
	Accepted         bool          `json:"accepted"`
	ProviderAccepted bool          `json:"provider_accepted"`
	DeliveryProven   bool          `json:"delivery_proven"`
	OutcomeUnknown   bool          `json:"outcome_unknown"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
}

type ExternalPushLogPage struct {
	Items   []ExternalPushLog `json:"items"`
	Total   int64             `json:"total"`
	Limit   int32             `json:"limit"`
	Offset  int32             `json:"offset"`
	HasMore bool              `json:"has_more"`
}

type ExternalPushLogReader interface {
	ListExternalPushLogs(context.Context, *surveyport.ID, int32, int32) ([]ExternalPushLog, int64, error)
}

type ExternalPushLogService struct {
	uow   platformport.UnitOfWork
	store ExternalPushLogReader
}

func NewExternalPushLogService(uow platformport.UnitOfWork, store ExternalPushLogReader) *ExternalPushLogService {
	return &ExternalPushLogService{uow: uow, store: store}
}

// List reads persisted survey bindings joined to their canonical EER state.
// It never substitutes the local external-push test queue for delivery facts.
func (service *ExternalPushLogService) List(ctx context.Context, questionnaireID *surveyport.ID, limit, offset int32) (ExternalPushLogPage, error) {
	if ctx == nil || service == nil || service.uow == nil || service.store == nil || limit < 1 || limit > 100 || offset < 0 || (questionnaireID != nil && *questionnaireID < 1) {
		return ExternalPushLogPage{}, ErrExternalPushUnavailable
	}
	result := ExternalPushLogPage{Items: []ExternalPushLog{}, Limit: limit, Offset: offset}
	err := service.within(ctx, func(tx context.Context) error {
		var err error
		result.Items, result.Total, err = service.store.ListExternalPushLogs(tx, questionnaireID, limit, offset)
		return err
	})
	if err != nil {
		return ExternalPushLogPage{}, errors.Join(ErrExternalPushUnavailable, err)
	}
	for index := range result.Items {
		item := &result.Items[index]
		if !validExternalPushLog(*item, questionnaireID) {
			return ExternalPushLogPage{}, ErrExternalPushUnavailable
		}
		item.Accepted = true
		item.OutcomeUnknown = item.State == eer.StateOutcomeUnknown
	}
	result.HasMore = int64(offset)+int64(len(result.Items)) < result.Total
	return result, nil
}

func (service *ExternalPushLogService) within(ctx context.Context, callback func(context.Context) error) error {
	if _, err := platformstore.TxFromContext(ctx); err == nil {
		return callback(ctx)
	}
	return service.uow.Within(ctx, callback)
}

func validExternalPushLog(item ExternalPushLog, questionnaireID *surveyport.ID) bool {
	if item.BindingID < 1 || item.QuestionnaireID < 1 || item.SubmissionID < 1 || item.CustomerID < 1 || item.EffectID == "" || item.AttemptCount < 0 || item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() || item.DeliveryProven && !item.ProviderAccepted || questionnaireID != nil && item.QuestionnaireID != *questionnaireID {
		return false
	}
	switch item.State {
	case eer.StateAccepted, eer.StateQueued, eer.StateAttempted, eer.StateExecuted, eer.StateOutcomeUnknown, eer.StateReconciled, eer.StateRetryableFailed, eer.StateFinalFailed, eer.State("cancelled"):
		return true
	default:
		return false
	}
}
