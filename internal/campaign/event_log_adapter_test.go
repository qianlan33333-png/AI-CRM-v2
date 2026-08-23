package campaign

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type eventAppenderSpy struct{ events []eventport.Event }

func (spy *eventAppenderSpy) Append(_ context.Context, event eventport.Event) (eventport.EventID, error) {
	spy.events = append(spy.events, event)
	return eventport.EventID(len(spy.events)), nil
}

func TestEventLogAdapterAppendsTouchPlanAsBoundLocalCampaignFact(t *testing.T) {
	spy := &eventAppenderSpy{}
	adapter, err := NewEventLogAdapter(spy)
	if err != nil {
		t.Fatal(err)
	}
	created, err := adapter.AppendTouchPlanCreated(context.Background(), TouchPlanCreatedAuditEvent{
		PlanID: DraftTouchPlanID(7, "spring-campaign", "touch-plan-key"), CampaignCode: "spring-campaign", OwnerActorID: 7,
		TargetDigest: strings.Repeat("1", 64), TargetCount: 1, ContentDigest: strings.Repeat("2", 64),
		OccurredAt: time.Date(2026, time.August, 23, 2, 3, 4, 0, time.UTC), IdempotencyKey: "touch-plan-event-key",
	})
	if err != nil || created != 1 || len(spy.events) != 1 || spy.events[0].Type != eventport.EvCloudCampaignFact {
		t.Fatalf("event id/error/events=%d/%v/%+v", created, err, spy.events)
	}
	var payload map[string]any
	if err = json.Unmarshal(spy.events[0].Payload, &payload); err != nil || payload["audit_type"] != "touch_plan_created" ||
		payload["plan_id"] == "" || payload["provider"] != nil || payload["outbound_task_id"] != nil {
		t.Fatalf("payload=%s parsed=%#v err=%v", spy.events[0].Payload, payload, err)
	}
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
