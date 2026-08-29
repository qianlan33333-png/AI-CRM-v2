package app

import (
	"context"
	"errors"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type cloudAuditRepositoryStub struct {
	filter eventport.CloudAuditFilter
	items  []eventport.CloudAuditFact
	err    error
}

func (repository *cloudAuditRepositoryStub) ListCloudAudit(_ context.Context, filter eventport.CloudAuditFilter) ([]eventport.CloudAuditFact, error) {
	repository.filter = filter
	return append([]eventport.CloudAuditFact(nil), repository.items...), repository.err
}

func TestCloudAuditRequiresTraceOrSessionAndReturnsOnlyLocalFacts(t *testing.T) {
	now := time.Date(2026, 8, 29, 4, 5, 6, 0, time.UTC)
	repository := &cloudAuditRepositoryStub{items: []eventport.CloudAuditFact{{EventID: 9, EventType: "cloud_campaign.fact", OccurredAt: now.Add(-time.Minute), Dispatched: true, Completed: 1, OutcomeUnknown: 1}}}
	service := NewCloudAuditService(repository, func() time.Time { return now })
	result, err := service.List(context.Background(), eventport.CloudAuditFilter{TraceID: " trace-1 ", SessionID: "session-1", Limit: 25})
	if err != nil || repository.filter.TraceID != "trace-1" || len(result.Items) != 1 || !result.LocalFactsOnly || result.RealExternalCallExecuted || result.DeliveryProven || result.Items[0].OutcomeUnknown != 1 {
		t.Fatalf("result=%+v filter=%+v err=%v", result, repository.filter, err)
	}
	if _, err = service.List(context.Background(), eventport.CloudAuditFilter{Limit: 25}); !errors.Is(err, ErrCloudAuditInvalid) {
		t.Fatalf("missing identifiers err=%v", err)
	}
}

func TestCloudAuditFailsClosedOnMalformedRepositoryFacts(t *testing.T) {
	now := time.Now().UTC()
	repository := &cloudAuditRepositoryStub{items: []eventport.CloudAuditFact{{EventID: 9, EventType: "cloud_campaign.fact", OccurredAt: now, OutcomeUnknown: -1}}}
	service := NewCloudAuditService(repository, func() time.Time { return now })
	if _, err := service.List(context.Background(), eventport.CloudAuditFilter{TraceID: "trace-1", Limit: 25}); !errors.Is(err, ErrCloudAuditUnavailable) {
		t.Fatalf("err=%v", err)
	}
}
