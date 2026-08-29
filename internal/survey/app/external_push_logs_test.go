package app

import (
	"context"
	"errors"
	"testing"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

func TestExternalPushLogsDistinguishAcceptanceDeliveryAndUnknown(t *testing.T) {
	now := time.Date(2026, 8, 29, 8, 0, 0, 0, time.UTC)
	store := &externalPushLogStoreStub{items: []ExternalPushLog{
		{BindingID: 1, QuestionnaireID: 7, SubmissionID: 11, CustomerID: 21, EffectID: "eer_31", State: eer.StateAccepted, CreatedAt: now, UpdatedAt: now},
		{BindingID: 2, QuestionnaireID: 7, SubmissionID: 12, CustomerID: 22, EffectID: "eer_32", State: eer.StateExecuted, AttemptCount: 1, ProviderAccepted: true, DeliveryProven: true, CreatedAt: now, UpdatedAt: now},
		{BindingID: 3, QuestionnaireID: 7, SubmissionID: 13, CustomerID: 23, EffectID: "eer_33", State: eer.StateOutcomeUnknown, AttemptCount: 1, CreatedAt: now, UpdatedAt: now},
	}, total: 3}
	questionnaireID := surveyport.ID(7)
	page, err := NewExternalPushLogService(testUOW{}, store).List(context.Background(), &questionnaireID, 50, 0)
	if err != nil || len(page.Items) != 3 || page.Total != 3 || page.HasMore || !page.Items[0].Accepted || page.Items[0].ProviderAccepted || !page.Items[1].DeliveryProven || !page.Items[2].OutcomeUnknown || page.Items[2].DeliveryProven {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	if store.questionnaireID == nil || *store.questionnaireID != questionnaireID {
		t.Fatalf("filter=%v", store.questionnaireID)
	}
	if _, err = NewExternalPushLogService(testUOW{}, store).List(context.Background(), nil, 50, 0); err != nil || store.questionnaireID != nil {
		t.Fatalf("global filter=%v err=%v", store.questionnaireID, err)
	}
}

func TestExternalPushLogsFailClosedOnInvalidProjection(t *testing.T) {
	now := time.Now().UTC()
	store := &externalPushLogStoreStub{items: []ExternalPushLog{{BindingID: 1, QuestionnaireID: 7, SubmissionID: 11, CustomerID: 21, EffectID: "eer_31", State: eer.StateExecuted, ProviderAccepted: false, DeliveryProven: true, CreatedAt: now, UpdatedAt: now}}, total: 1}
	_, err := NewExternalPushLogService(testUOW{}, store).List(context.Background(), nil, 50, 0)
	if !errors.Is(err, ErrExternalPushUnavailable) {
		t.Fatalf("err=%v", err)
	}
}

type externalPushLogStoreStub struct {
	items           []ExternalPushLog
	total           int64
	questionnaireID *surveyport.ID
}

func (store *externalPushLogStoreStub) ListExternalPushLogs(_ context.Context, questionnaireID *surveyport.ID, _, _ int32) ([]ExternalPushLog, int64, error) {
	store.questionnaireID = questionnaireID
	return append([]ExternalPushLog(nil), store.items...), store.total, nil
}
