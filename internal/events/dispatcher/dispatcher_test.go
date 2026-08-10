package dispatcher

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type recordingSubscriber struct {
	types    []string
	err      error
	received []eventport.Record
}

func (subscriber *recordingSubscriber) EventTypes() []string {
	return append([]string(nil), subscriber.types...)
}

func (subscriber *recordingSubscriber) Consume(_ context.Context, event eventport.Record) error {
	subscriber.received = append(subscriber.received, event)
	return subscriber.err
}

type testUnitOfWork struct{}

func (*testUnitOfWork) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type testEnqueuer struct{}

func (*testEnqueuer) EnqueueTx(context.Context, pgx.Tx, platformjobqueue.Queue, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	return nil, nil
}

func TestJobArgsKindsAreStable(t *testing.T) {
	if got := (DispatchArgs{}).Kind(); got != "events_dispatch" {
		t.Fatalf("DispatchArgs.Kind() = %q, want %q", got, "events_dispatch")
	}
	if got := (DeliverArgs{}).Kind(); got != "events_deliver" {
		t.Fatalf("DeliverArgs.Kind() = %q, want %q", got, "events_deliver")
	}
}

func TestNewRouterRejectsInvalidSubscribers(t *testing.T) {
	typedNil := (*recordingSubscriber)(nil)
	tests := []struct {
		name       string
		subscriber eventport.Subscriber
	}{
		{name: "nil subscriber"},
		{name: "typed nil subscriber", subscriber: typedNil},
		{name: "no event types", subscriber: &recordingSubscriber{}},
		{name: "empty event type", subscriber: &recordingSubscriber{types: []string{""}}},
		{name: "blank event type", subscriber: &recordingSubscriber{types: []string{" \t\n "}}},
		{name: "duplicate event type after trim", subscriber: &recordingSubscriber{types: []string{"customer.updated", " customer.updated "}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, err := NewRouter(test.subscriber)
			if router != nil || !errors.Is(err, ErrInvalidSubscriber) {
				t.Fatalf("NewRouter() = %v, %v; want nil, ErrInvalidSubscriber", router, err)
			}
		})
	}
}

func TestRouterDeliversOriginalRecordToAllSubscribers(t *testing.T) {
	first := &recordingSubscriber{types: []string{"customer.updated"}}
	second := &recordingSubscriber{types: []string{"customer.updated"}}
	router, err := NewRouter(first, second)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}

	record := eventport.Record{
		ID: 42,
		Event: eventport.Event{
			Type:           "customer.updated",
			CustomerID:     84,
			Payload:        []byte(`{"source":"dispatcher"}`),
			OccurredAt:     time.Date(2026, time.August, 10, 8, 0, 0, 0, time.UTC),
			IdempotencyKey: "event-42",
		},
	}
	if err := router.Consume(context.Background(), record); err != nil {
		t.Fatalf("Consume() error = %v", err)
	}

	for _, subscriber := range []*recordingSubscriber{first, second} {
		if len(subscriber.received) != 1 {
			t.Fatalf("subscriber received %d records, want 1", len(subscriber.received))
		}
		if !reflect.DeepEqual(subscriber.received[0], record) {
			t.Fatalf("subscriber received %#v, want %#v", subscriber.received[0], record)
		}
	}
}

func TestRouterAggregatesSubscriberErrorsWithoutShortCircuiting(t *testing.T) {
	firstErr := errors.New("first subscriber failed")
	secondErr := errors.New("second subscriber failed")
	first := &recordingSubscriber{types: []string{"customer.updated"}, err: firstErr}
	second := &recordingSubscriber{types: []string{"customer.updated"}, err: secondErr}
	router, err := NewRouter(first, second)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}

	err = router.Consume(context.Background(), eventport.Record{Event: eventport.Event{Type: "customer.updated"}})
	if !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("Consume() error = %v, want both subscriber errors", err)
	}
	if len(first.received) != 1 || len(second.received) != 1 {
		t.Fatalf("subscriber calls = %d, %d; want 1, 1", len(first.received), len(second.received))
	}
}

func TestRouterFailsClosedWhenNoSubscriberMatches(t *testing.T) {
	router, err := NewRouter()
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}

	err = router.Consume(context.Background(), eventport.Record{Event: eventport.Event{Type: "customer.updated"}})
	if !errors.Is(err, ErrNoSubscriber) {
		t.Fatalf("Consume() error = %v, want ErrNoSubscriber", err)
	}
}

func TestDeliveryInsertOptionsAreUniqueByArgsAndLeaveQueueUnset(t *testing.T) {
	options := deliveryInsertOptions()
	if options == nil {
		t.Fatal("deliveryInsertOptions() = nil")
	}
	if !options.UniqueOpts.ByArgs {
		t.Fatal("deliveryInsertOptions().UniqueOpts.ByArgs = false, want true")
	}
	if options.UniqueOpts.ByQueue || options.UniqueOpts.ByPeriod != 0 || options.UniqueOpts.ByState != nil || options.UniqueOpts.ExcludeKind {
		t.Fatalf("deliveryInsertOptions().UniqueOpts = %#v, want only ByArgs enabled", options.UniqueOpts)
	}
	if options.Queue != "" || options.Queue == river.QueueDefault {
		t.Fatalf("deliveryInsertOptions().Queue = %q, want unset and not River default %q", options.Queue, river.QueueDefault)
	}
}

func TestConstructorsFailClosedForInvalidInputs(t *testing.T) {
	validUOW := &testUnitOfWork{}
	validEnqueuer := &testEnqueuer{}
	var typedNilUOW *testUnitOfWork
	var typedNilEnqueuer *testEnqueuer

	for _, test := range []struct {
		name      string
		uow       platformport.UnitOfWork
		enqueuer  transactionalEnqueuer
		batchSize int32
	}{
		{name: "nil unit of work", enqueuer: validEnqueuer, batchSize: 1},
		{name: "typed nil unit of work", uow: typedNilUOW, enqueuer: validEnqueuer, batchSize: 1},
		{name: "nil enqueuer", uow: validUOW, batchSize: 1},
		{name: "typed nil enqueuer", uow: validUOW, enqueuer: typedNilEnqueuer, batchSize: 1},
		{name: "zero batch size", uow: validUOW, enqueuer: validEnqueuer},
		{name: "negative batch size", uow: validUOW, enqueuer: validEnqueuer, batchSize: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			dispatcher, err := New(test.uow, test.enqueuer, test.batchSize)
			if dispatcher != nil || !errors.Is(err, ErrInvalidDispatcher) {
				t.Fatalf("New() = %v, %v; want nil, ErrInvalidDispatcher", dispatcher, err)
			}
		})
	}

	dispatcher, err := New(validUOW, validEnqueuer, 1)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if worker, err := NewDispatchWorker(nil); worker != nil || !errors.Is(err, ErrInvalidDispatcher) {
		t.Fatalf("NewDispatchWorker(nil) = %v, %v; want nil, ErrInvalidDispatcher", worker, err)
	}
	if worker, err := NewDispatchWorker(dispatcher); err != nil || worker == nil {
		t.Fatalf("NewDispatchWorker(valid) = %v, %v; want non-nil, nil", worker, err)
	}

	router, err := NewRouter()
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	pool := &pgxpool.Pool{}
	if worker, err := NewDeliveryWorker(nil, router); worker != nil || !errors.Is(err, ErrInvalidDispatcher) {
		t.Fatalf("NewDeliveryWorker(nil, valid) = %v, %v; want nil, ErrInvalidDispatcher", worker, err)
	}
	if worker, err := NewDeliveryWorker(pool, nil); worker != nil || !errors.Is(err, ErrInvalidDispatcher) {
		t.Fatalf("NewDeliveryWorker(valid, nil) = %v, %v; want nil, ErrInvalidDispatcher", worker, err)
	}
	if worker, err := NewDeliveryWorker(pool, router); err != nil || worker == nil {
		t.Fatalf("NewDeliveryWorker(valid) = %v, %v; want non-nil, nil", worker, err)
	}
}
