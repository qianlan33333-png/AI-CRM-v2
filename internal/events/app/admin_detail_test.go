package app

import (
	"context"
	"errors"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type adminDetailRepositoryStub struct {
	snapshot eventport.AdminDetailSnapshot
	err      error
	calls    int
}

func (stub *adminDetailRepositoryStub) Read(context.Context, eventport.EventID) (eventport.AdminDetailSnapshot, error) {
	stub.calls++
	return stub.snapshot, stub.err
}

func TestAdminDetailReturnsAllDeliveriesInRegistryOrder(t *testing.T) {
	stamp := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	completed := stamp.Add(time.Minute)
	repository := &adminDetailRepositoryStub{snapshot: eventport.AdminDetailSnapshot{
		Found: true,
		Event: eventport.AdminReadEvent{EventID: 42, EventType: eventport.EvTagApplied, OccurredAt: stamp},
		Deliveries: []eventport.AdminReadDelivery{
			{EventID: 42, Consumer: eventport.ConsumerStatsTagApplied, Status: string(eventport.DeliveryCompleted), AttemptCount: 1, CompletedAt: &completed},
			{EventID: 42, Consumer: eventport.ConsumerAutomationTagTrigger, Status: string(eventport.DeliveryPending), AttemptCount: 0},
		},
	}}
	service := NewAdminDetailService(repository, func() time.Time { return stamp.Add(time.Hour) })
	result, err := service.Get(context.Background(), 42)
	if err != nil || repository.calls != 1 {
		t.Fatalf("result=%+v err=%v calls=%d", result, err, repository.calls)
	}
	if result.Item.EventID != 42 || result.Item.OccurredAt != stamp || len(result.Item.Deliveries) != 2 || result.Item.Deliveries[0].Consumer != eventport.ConsumerAutomationTagTrigger || result.Item.Deliveries[1].Consumer != eventport.ConsumerStatsTagApplied {
		t.Fatalf("item=%+v", result.Item)
	}
	if result.ObservedAt != stamp.Add(time.Hour) || result.Item.Deliveries[1].CompletedAt == nil || !result.Item.Deliveries[1].CompletedAt.Equal(completed) {
		t.Fatalf("timestamps result=%+v", result)
	}
}

func TestAdminDetailNoDeliveryAndNotFoundAreDistinct(t *testing.T) {
	stamp := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	service := NewAdminDetailService(&adminDetailRepositoryStub{snapshot: eventport.AdminDetailSnapshot{
		Found: true, Event: eventport.AdminReadEvent{EventID: 7, EventType: "custom.local_fact", OccurredAt: stamp},
	}}, func() time.Time { return stamp })
	result, err := service.Get(context.Background(), 7)
	if err != nil || result.Item.Deliveries == nil || len(result.Item.Deliveries) != 0 {
		t.Fatalf("no-delivery result=%+v err=%v", result, err)
	}
	service = NewAdminDetailService(&adminDetailRepositoryStub{snapshot: eventport.AdminDetailSnapshot{}}, func() time.Time { return stamp })
	if _, err := service.Get(context.Background(), 7); !errors.Is(err, ErrAdminDetailNotFound) {
		t.Fatalf("not-found err=%v", err)
	}
}

func TestAdminDetailRejectsCorruptRowsAndRepositoryFailures(t *testing.T) {
	stamp := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	cases := []eventport.AdminDetailSnapshot{
		{Found: true, Event: eventport.AdminReadEvent{EventID: 1, EventType: eventport.EvTagApplied, OccurredAt: stamp}, Deliveries: []eventport.AdminReadDelivery{{EventID: 1, Consumer: "unknown", Status: string(eventport.DeliveryPending)}}},
		{Found: true, Event: eventport.AdminReadEvent{EventID: 1, EventType: eventport.EvTagApplied, OccurredAt: stamp}, Deliveries: []eventport.AdminReadDelivery{{EventID: 1, Consumer: eventport.ConsumerAutomationTagTrigger, Status: string(eventport.DeliveryCompleted)}}},
		{Found: true, Event: eventport.AdminReadEvent{EventID: 1, EventType: eventport.EvTagApplied, OccurredAt: stamp}, Deliveries: []eventport.AdminReadDelivery{{EventID: 2, Consumer: eventport.ConsumerAutomationTagTrigger, Status: string(eventport.DeliveryPending)}}},
	}
	for index, snapshot := range cases {
		service := NewAdminDetailService(&adminDetailRepositoryStub{snapshot: snapshot}, func() time.Time { return stamp })
		if _, err := service.Get(context.Background(), 1); !errors.Is(err, ErrAdminDetailUnavailable) {
			t.Fatalf("case=%d err=%v", index, err)
		}
	}
	service := NewAdminDetailService(&adminDetailRepositoryStub{err: errors.New("db failed")}, time.Now)
	if _, err := service.Get(context.Background(), 1); !errors.Is(err, ErrAdminDetailUnavailable) {
		t.Fatalf("repository err=%v", err)
	}
}
