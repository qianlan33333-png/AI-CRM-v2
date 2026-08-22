package campaign

import (
	"context"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type eventAppenderSpy struct{ events []eventport.Event }

func (spy *eventAppenderSpy) Append(_ context.Context, event eventport.Event) (eventport.EventID, error) {
	spy.events = append(spy.events, event)
	return eventport.EventID(len(spy.events)), nil
}

func TestEventLogAdapterSeparatesBatchCampaignReceiptKeys(t *testing.T) {
	spy := &eventAppenderSpy{}
	adapter, err := NewEventLogAdapter(spy)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 22, 1, 0, 0, 0, time.UTC)
	for _, code := range []string{"one", "two"} {
		if err = adapter.Append(context.Background(), AuditEvent{Type: "cloud_campaign.batch_started", CampaignCode: code, ActorID: 7, IdempotencyKey: "batch-key", OccurredAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	if len(spy.events) != 2 || spy.events[0].IdempotencyKey == spy.events[1].IdempotencyKey {
		t.Fatalf("events=%+v", spy.events)
	}
}

func TestEventLogAdapterSeparatesActorsWithSameReceiptKey(t *testing.T) {
	spy := &eventAppenderSpy{}
	adapter, err := NewEventLogAdapter(spy)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 22, 1, 0, 0, 0, time.UTC)
	for _, actorID := range []int64{7, 8} {
		if err = adapter.Append(context.Background(), AuditEvent{Type: "cloud_campaign.approved", CampaignCode: "one", ActorID: actorID, IdempotencyKey: "same-key", OccurredAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	if len(spy.events) != 2 || spy.events[0].IdempotencyKey == spy.events[1].IdempotencyKey {
		t.Fatalf("events=%+v", spy.events)
	}
}
