package store

import (
	"context"
	"errors"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

func TestAppendFailsClosedWithoutTransaction(t *testing.T) {
	event := validEvent()
	_, err := NewAppender().Append(context.Background(), event)
	if !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("Append() error = %v, want ErrTransactionRequired", err)
	}
}

func TestAppendRejectsInvalidEventBeforeTransactionLookup(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*eventport.Event)
	}{
		{name: "type", mutate: func(event *eventport.Event) { event.Type = " " }},
		{name: "customer", mutate: func(event *eventport.Event) { event.CustomerID = -1 }},
		{name: "payload", mutate: func(event *eventport.Event) { event.Payload = []byte("{") }},
		{name: "occurred at", mutate: func(event *eventport.Event) { event.OccurredAt = time.Time{} }},
		{name: "idempotency key", mutate: func(event *eventport.Event) { event.IdempotencyKey = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := validEvent()
			test.mutate(&event)
			_, err := NewAppender().Append(context.Background(), event)
			if !errors.Is(err, eventport.ErrInvalidEvent) {
				t.Fatalf("Append() error = %v, want ErrInvalidEvent", err)
			}
		})
	}
}

func validEvent() eventport.Event {
	return eventport.Event{
		Type:           "stage.changed",
		CustomerID:     42,
		Payload:        []byte(`{"stage_id":7}`),
		OccurredAt:     time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
		IdempotencyKey: "stage.changed:42:7",
	}
}
