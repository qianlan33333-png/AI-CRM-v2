package app

import (
	"context"
	"errors"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type adminReadRepositoryStub struct {
	snapshot eventport.AdminReadSnapshot
	err      error
	calls    int
}

func (stub *adminReadRepositoryStub) Read(context.Context, string) (eventport.AdminReadSnapshot, error) {
	stub.calls++
	return stub.snapshot, stub.err
}

func TestAdminReadListFiltersEventsButReturnsAllDeliveries(t *testing.T) {
	stamp := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	completed := stamp.Add(time.Minute)
	repository := &adminReadRepositoryStub{snapshot: eventport.AdminReadSnapshot{
		Events: []eventport.AdminReadEvent{
			{EventID: 1, EventType: eventport.EvTagApplied, OccurredAt: stamp, Dispatched: false},
			{EventID: 2, EventType: eventport.EvTagApplied, OccurredAt: stamp, Dispatched: true},
		},
		Deliveries: []eventport.AdminReadDelivery{
			{EventID: 1, Consumer: eventport.ConsumerAutomationTagTrigger, Status: string(eventport.DeliveryPending), AttemptCount: 0},
			{EventID: 1, Consumer: eventport.ConsumerStatsTagApplied, Status: string(eventport.DeliveryCompleted), AttemptCount: 1, CompletedAt: &completed},
			{EventID: 2, Consumer: eventport.ConsumerStatsTagApplied, Status: string(eventport.DeliveryPending), AttemptCount: 0},
		},
	}}
	service := NewAdminReadService(repository, func() time.Time { return stamp.Add(time.Hour) })
	result, err := service.List(context.Background(), eventport.AdminReadQuery{Consumer: eventport.ConsumerStatsTagApplied, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if repository.calls != 1 || result.Total != 2 || len(result.Items) != 2 || result.Items[0].EventID != 2 || result.Items[1].EventID != 1 {
		t.Fatalf("calls/total/items=%d/%d/%+v", repository.calls, result.Total, result.Items)
	}
	if len(result.Items[1].Deliveries) != 2 || result.Items[1].Deliveries[0].Consumer != eventport.ConsumerAutomationTagTrigger {
		t.Fatalf("deliveries=%+v", result.Items[1].Deliveries)
	}
}

func TestAdminReadDiagnosticsCountsMatchingDeliveries(t *testing.T) {
	stamp := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	repository := &adminReadRepositoryStub{snapshot: eventport.AdminReadSnapshot{
		Events: []eventport.AdminReadEvent{
			{EventID: 1, EventType: eventport.EvTagApplied, OccurredAt: stamp, Dispatched: false},
		},
		Deliveries: []eventport.AdminReadDelivery{
			{EventID: 1, Consumer: eventport.ConsumerAutomationTagTrigger, Status: string(eventport.DeliveryPending), AttemptCount: 0},
			{EventID: 1, Consumer: eventport.ConsumerStatsTagApplied, Status: string(eventport.DeliveryCompleted), AttemptCount: 1, CompletedAt: func() *time.Time { v := stamp; return &v }()},
		},
	}}
	service := NewAdminReadService(repository, func() time.Time { return stamp })
	result, err := service.Diagnostics(context.Background(), eventport.AdminReadQuery{Status: string(eventport.DeliveryPending)})
	if err != nil {
		t.Fatal(err)
	}
	if result.EventCount != 1 || result.UndispatchedEventCount != 1 || result.DeliveryCounts.Pending != 1 || result.DeliveryCounts.Completed != 0 {
		t.Fatalf("result=%+v", result)
	}
	if len(result.ConsumerRegistry) != 5 || result.ConsumerRegistry[0].Consumer != eventport.ConsumerAutomationTagTrigger || result.ConsumerRegistry[3].Consumer != eventport.ConsumerCloudCampaignFact || result.ConsumerRegistry[4].Consumer != eventport.ConsumerOutboundCampaignHandoffFact {
		t.Fatalf("registry=%+v", result.ConsumerRegistry)
	}
}

func TestAdminReadRejectsUnknownConsumerAndBindingCorruption(t *testing.T) {
	stamp := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	service := NewAdminReadService(&adminReadRepositoryStub{snapshot: eventport.AdminReadSnapshot{
		Events:     []eventport.AdminReadEvent{{EventID: 1, EventType: eventport.EvTagApplied, OccurredAt: stamp}},
		Deliveries: []eventport.AdminReadDelivery{{EventID: 1, Consumer: "unknown.consumer", Status: string(eventport.DeliveryPending)}},
	}}, time.Now)
	if _, err := service.List(context.Background(), eventport.AdminReadQuery{Limit: 50}); !errors.Is(err, ErrAdminReadUnavailable) {
		t.Fatalf("err=%v", err)
	}

	service = NewAdminReadService(&adminReadRepositoryStub{snapshot: eventport.AdminReadSnapshot{
		Events:     []eventport.AdminReadEvent{{EventID: 1, EventType: eventport.EvTagApplied, OccurredAt: stamp}},
		Deliveries: []eventport.AdminReadDelivery{{EventID: 1, Consumer: eventport.ConsumerOperationCycleFact, Status: string(eventport.DeliveryPending)}},
	}}, time.Now)
	if _, err := service.List(context.Background(), eventport.AdminReadQuery{Limit: 50}); !errors.Is(err, ErrAdminReadUnavailable) {
		t.Fatalf("binding err=%v", err)
	}
}

func TestAdminReadAcceptsCanonicalUnicodeWhitespaceInEventType(t *testing.T) {
	stamp := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	eventType := "custom.event\u00a0"
	service := NewAdminReadService(&adminReadRepositoryStub{snapshot: eventport.AdminReadSnapshot{
		Events: []eventport.AdminReadEvent{{EventID: 1, EventType: eventType, OccurredAt: stamp}},
	}}, func() time.Time { return stamp })
	result, err := service.List(context.Background(), eventport.AdminReadQuery{Limit: 50})
	if err != nil || result.Total != 1 || len(result.Items) != 1 || result.Items[0].EventType != eventType {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
