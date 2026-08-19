package internaleventsacceptance_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	eventapp "github.com/qianlan33333-png/AI-CRM-v2/internal/events/app"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
)

func TestInternalEventDetail0371IsPointReadOnlyAndBounded(t *testing.T) {
	p, ctx := openPool(t)
	repository := eventstore.NewAdminDetailRepository(p)
	observedAt := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	service := eventapp.NewAdminDetailService(repository, func() time.Time { return observedAt })
	marker := fmt.Sprintf("p4-0371-%d", time.Now().UnixNano())
	stamp := time.Date(2099, 1, 2, 3, 4, 5, 0, time.UTC)
	eventIDs := make([]int64, 0, 7)
	seedEvent := func(eventType string) int64 {
		var id int64
		if err := p.QueryRow(ctx, `INSERT INTO event_log (event_type, occurred_at, idempotency_key, dispatched)
VALUES ($1, $2, $3, FALSE) RETURNING id`, eventType, stamp, fmt.Sprintf("%s-event-%d", marker, len(eventIDs))).Scan(&id); err != nil {
			t.Fatalf("seed event type=%q: %v", eventType, err)
		}
		eventIDs = append(eventIDs, id)
		return id
	}
	seedDelivery := func(eventID int64, consumer, status string, attempt int32, completed *time.Time, processing bool) {
		var leaseOwner any
		var leaseExpires any
		if processing {
			leaseOwner = marker + "-lease"
			leaseExpires = stamp.Add(time.Hour)
		}
		if _, err := p.Exec(ctx, `INSERT INTO event_deliveries
 (event_id, consumer, status, attempt_count, lease_owner, lease_expires_at, completed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)`, eventID, consumer, status, attempt, leaseOwner, leaseExpires, completed); err != nil {
			t.Fatalf("seed delivery event=%d consumer=%q status=%q: %v", eventID, consumer, status, err)
		}
	}

	pending := seedEvent(eventport.EvTagApplied)
	processing := seedEvent(eventport.EvTagApplied)
	completed := seedEvent(eventport.EvOperationCycleFact)
	finalFailed := seedEvent(eventport.EvTagApplied)
	unknown := seedEvent(eventport.EvTagApplied)
	allRows := seedEvent(eventport.EvTagApplied)
	noDelivery := seedEvent(marker + ".arbitrary_valid_type")
	unknownConsumer := seedEvent(eventport.EvTagApplied)
	wrongBinding := seedEvent(eventport.EvTagApplied)
	completion := stamp.Add(time.Minute)
	seedDelivery(pending, eventport.ConsumerAutomationTagTrigger, string(eventport.DeliveryPending), 0, nil, false)
	seedDelivery(processing, eventport.ConsumerStatsTagApplied, string(eventport.DeliveryProcessing), 2, nil, true)
	seedDelivery(completed, eventport.ConsumerOperationCycleFact, string(eventport.DeliveryCompleted), 1, &completion, false)
	seedDelivery(finalFailed, eventport.ConsumerAutomationTagTrigger, string(eventport.DeliveryFinalFailed), 3, &completion, false)
	seedDelivery(unknown, eventport.ConsumerStatsTagApplied, string(eventport.DeliveryOutcomeUnknown), 4, &completion, false)
	seedDelivery(allRows, eventport.ConsumerStatsTagApplied, string(eventport.DeliveryCompleted), 1, &completion, false)
	seedDelivery(allRows, eventport.ConsumerAutomationTagTrigger, string(eventport.DeliveryPending), 0, nil, false)
	seedDelivery(unknownConsumer, "unknown.consumer.v1", string(eventport.DeliveryPending), 0, nil, false)
	seedDelivery(wrongBinding, eventport.ConsumerOperationCycleFact, string(eventport.DeliveryPending), 0, nil, false)
	// Snapshot after fixture creation: the read-only assertion must ignore the
	// deliberately seeded rows and detect only mutations caused by the detail
	// read itself.
	before := sourceFacts(t, ctx, p)
	t.Cleanup(func() {
		if _, err := p.Exec(ctx, `DELETE FROM event_deliveries WHERE event_id = ANY($1::bigint[])`, eventIDs); err != nil {
			t.Errorf("cleanup detail deliveries: %v", err)
		}
		if _, err := p.Exec(ctx, `DELETE FROM event_log WHERE id = ANY($1::bigint[])`, eventIDs); err != nil {
			t.Errorf("cleanup detail events: %v", err)
		}
	})

	for _, test := range []struct {
		name     string
		eventID  int64
		consumer string
		status   string
		attempt  int32
	}{
		{name: "pending", eventID: pending, consumer: eventport.ConsumerAutomationTagTrigger, status: string(eventport.DeliveryPending), attempt: 0},
		{name: "processing", eventID: processing, consumer: eventport.ConsumerStatsTagApplied, status: string(eventport.DeliveryProcessing), attempt: 2},
		{name: "completed", eventID: completed, consumer: eventport.ConsumerOperationCycleFact, status: string(eventport.DeliveryCompleted), attempt: 1},
		{name: "final_failed", eventID: finalFailed, consumer: eventport.ConsumerAutomationTagTrigger, status: string(eventport.DeliveryFinalFailed), attempt: 3},
		{name: "outcome_unknown", eventID: unknown, consumer: eventport.ConsumerStatsTagApplied, status: string(eventport.DeliveryOutcomeUnknown), attempt: 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := service.Get(ctx, eventport.EventID(test.eventID))
			if err != nil {
				t.Fatalf("detail: %v", err)
			}
			if result.Item.EventID != eventport.EventID(test.eventID) || result.Item.OccurredAt.IsZero() || result.Item.EventType == "" || len(result.Item.Deliveries) != 1 {
				t.Fatalf("item=%+v", result.Item)
			}
			delivery := result.Item.Deliveries[0]
			if delivery.Consumer != test.consumer || delivery.Status != test.status || delivery.AttemptCount != test.attempt {
				t.Fatalf("delivery=%+v", delivery)
			}
			terminal := test.status == string(eventport.DeliveryCompleted) || test.status == string(eventport.DeliveryFinalFailed) || test.status == string(eventport.DeliveryOutcomeUnknown)
			if terminal && delivery.CompletedAt == nil {
				t.Fatal("terminal delivery lost completed_at")
			}
			if !terminal && delivery.CompletedAt != nil {
				t.Fatal("non-terminal delivery unexpectedly has completed_at")
			}
		})
	}
	all, err := service.Get(ctx, eventport.EventID(allRows))
	if err != nil || len(all.Item.Deliveries) != 2 || all.Item.Deliveries[0].Consumer != eventport.ConsumerAutomationTagTrigger || all.Item.Deliveries[1].Consumer != eventport.ConsumerStatsTagApplied {
		t.Fatalf("all deliveries=%+v err=%v", all, err)
	}
	empty, err := service.Get(ctx, eventport.EventID(noDelivery))
	if err != nil || empty.Item.Deliveries == nil || len(empty.Item.Deliveries) != 0 {
		t.Fatalf("no-delivery=%+v err=%v", empty, err)
	}
	for _, test := range []struct {
		name    string
		eventID int64
	}{
		{name: "unknown consumer", eventID: unknownConsumer},
		{name: "wrong event binding", eventID: wrongBinding},
	} {
		t.Run("malformed/"+test.name, func(t *testing.T) {
			if _, err := service.Get(ctx, eventport.EventID(test.eventID)); !errors.Is(err, eventapp.ErrAdminDetailUnavailable) {
				t.Fatalf("malformed point read err=%v, want ErrAdminDetailUnavailable", err)
			}
		})
	}
	missing, err := service.Get(ctx, eventport.EventID(9223372036854775807))
	if !errors.Is(err, eventapp.ErrAdminDetailNotFound) || missing.Item.EventID != 0 {
		t.Fatalf("missing=%+v err=%v", missing, err)
	}
	if after := sourceFacts(t, ctx, p); after != before {
		t.Fatalf("detail read changed source facts: before=%+v after=%+v", before, after)
	}
}

type malformedAdminDetailRepository struct {
	snapshot eventport.AdminDetailSnapshot
}

func (repository *malformedAdminDetailRepository) Read(context.Context, eventport.EventID) (eventport.AdminDetailSnapshot, error) {
	return repository.snapshot, nil
}

func TestInternalEventDetail0371MalformedPointReadRowsFailClosed(t *testing.T) {
	stamp := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	completed := stamp.Add(time.Minute)
	event := eventport.AdminReadEvent{EventID: 42, EventType: eventport.EvTagApplied, OccurredAt: stamp}
	validCompleted := eventport.AdminReadDelivery{EventID: 42, Consumer: eventport.ConsumerStatsTagApplied, Status: string(eventport.DeliveryCompleted), AttemptCount: 1, CompletedAt: &completed}
	malformed := []struct {
		name       string
		deliveries []eventport.AdminReadDelivery
	}{
		{name: "unknown consumer", deliveries: []eventport.AdminReadDelivery{{EventID: 42, Consumer: "unknown.consumer.v1", Status: string(eventport.DeliveryPending)}}},
		{name: "wrong event binding", deliveries: []eventport.AdminReadDelivery{{EventID: 42, Consumer: eventport.ConsumerOperationCycleFact, Status: string(eventport.DeliveryPending)}}},
		{name: "duplicate consumer", deliveries: []eventport.AdminReadDelivery{validCompleted, validCompleted}},
		{name: "negative attempt", deliveries: []eventport.AdminReadDelivery{{EventID: 42, Consumer: eventport.ConsumerStatsTagApplied, Status: string(eventport.DeliveryCompleted), AttemptCount: -1, CompletedAt: &completed}}},
		{name: "invalid status", deliveries: []eventport.AdminReadDelivery{{EventID: 42, Consumer: eventport.ConsumerStatsTagApplied, Status: "invalid", AttemptCount: 1, CompletedAt: &completed}}},
		{name: "pending with completion", deliveries: []eventport.AdminReadDelivery{{EventID: 42, Consumer: eventport.ConsumerStatsTagApplied, Status: string(eventport.DeliveryPending), CompletedAt: &completed}}},
		{name: "completed without completion", deliveries: []eventport.AdminReadDelivery{{EventID: 42, Consumer: eventport.ConsumerStatsTagApplied, Status: string(eventport.DeliveryCompleted), AttemptCount: 1}}},
		{name: "processing with completion", deliveries: []eventport.AdminReadDelivery{{EventID: 42, Consumer: eventport.ConsumerStatsTagApplied, Status: string(eventport.DeliveryProcessing), AttemptCount: 1, CompletedAt: &completed}}},
	}
	for _, test := range malformed {
		t.Run(test.name, func(t *testing.T) {
			repository := &malformedAdminDetailRepository{snapshot: eventport.AdminDetailSnapshot{Found: true, Event: event, Deliveries: test.deliveries}}
			service := eventapp.NewAdminDetailService(repository, func() time.Time { return stamp })
			if _, err := service.Get(context.Background(), eventport.EventID(42)); !errors.Is(err, eventapp.ErrAdminDetailUnavailable) {
				t.Fatalf("malformed point-read row err=%v, want ErrAdminDetailUnavailable", err)
			}
		})
	}
}

func TestInternalEventDetail0371RepositoryFailureIsUnavailableAndReadOnly(t *testing.T) {
	p, ctx := openPool(t)
	before := sourceFacts(t, ctx, p)
	failedPool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		t.Fatalf("open independent detail pool: %v", err)
	}
	failedPool.Close()
	repository := eventstore.NewAdminDetailRepository(failedPool)
	service := eventapp.NewAdminDetailService(repository, time.Now)
	if _, err := repository.Read(ctx, 1); err == nil {
		t.Fatal("closed detail pool unexpectedly read")
	}
	if _, err := service.Get(ctx, 1); !errors.Is(err, eventapp.ErrAdminDetailUnavailable) {
		t.Fatalf("closed detail source error=%v", err)
	}
	if after := sourceFacts(t, ctx, p); after != before {
		t.Fatalf("failed detail read changed source facts: before=%+v after=%+v", before, after)
	}
}
