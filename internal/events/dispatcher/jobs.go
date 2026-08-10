package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventdb "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store/generated"
	"github.com/riverqueue/river"
)

const (
	DispatchJobKind = "events_dispatch"
	DeliverJobKind  = "events_deliver"
)

var (
	ErrInvalidSubscriber = errors.New("invalid event subscriber")
	ErrNoSubscriber      = errors.New("no subscriber for event type")
)

type DispatchArgs struct{}

func (DispatchArgs) Kind() string { return DispatchJobKind }

type DeliverArgs struct {
	EventID int64 `json:"event_id"`
}

func (DeliverArgs) Kind() string { return DeliverJobKind }

type DispatchWorker struct {
	river.WorkerDefaults[DispatchArgs]
	dispatcher *Dispatcher
}

func NewDispatchWorker(dispatcher *Dispatcher) (*DispatchWorker, error) {
	if dispatcher == nil {
		return nil, ErrInvalidDispatcher
	}
	return &DispatchWorker{dispatcher: dispatcher}, nil
}

func (worker *DispatchWorker) Work(ctx context.Context, _ *river.Job[DispatchArgs]) error {
	_, err := worker.dispatcher.Dispatch(ctx)
	return err
}

type Router struct {
	subscribers map[string][]eventport.Subscriber
}

func NewRouter(subscribers ...eventport.Subscriber) (*Router, error) {
	routes := make(map[string][]eventport.Subscriber)
	for _, subscriber := range subscribers {
		if isNil(subscriber) {
			return nil, ErrInvalidSubscriber
		}
		types := subscriber.EventTypes()
		if len(types) == 0 {
			return nil, ErrInvalidSubscriber
		}
		seen := make(map[string]struct{}, len(types))
		for _, eventType := range types {
			eventType = strings.TrimSpace(eventType)
			if eventType == "" {
				return nil, ErrInvalidSubscriber
			}
			if _, duplicate := seen[eventType]; duplicate {
				return nil, ErrInvalidSubscriber
			}
			seen[eventType] = struct{}{}
			routes[eventType] = append(routes[eventType], subscriber)
		}
	}
	return &Router{subscribers: routes}, nil
}

func (router *Router) Consume(ctx context.Context, event eventport.Record) error {
	if router == nil {
		return ErrInvalidSubscriber
	}
	subscribers := router.subscribers[event.Type]
	if len(subscribers) == 0 {
		return fmt.Errorf("%w: %s", ErrNoSubscriber, event.Type)
	}
	var consumeErr error
	for _, subscriber := range subscribers {
		if err := subscriber.Consume(ctx, event); err != nil {
			consumeErr = errors.Join(consumeErr, err)
		}
	}
	return consumeErr
}

type DeliveryWorker struct {
	river.WorkerDefaults[DeliverArgs]
	pool   *pgxpool.Pool
	router *Router
}

func NewDeliveryWorker(pool *pgxpool.Pool, router *Router) (*DeliveryWorker, error) {
	if pool == nil || router == nil {
		return nil, ErrInvalidDispatcher
	}
	return &DeliveryWorker{pool: pool, router: router}, nil
}

func (worker *DeliveryWorker) Work(ctx context.Context, job *river.Job[DeliverArgs]) error {
	if worker == nil || worker.pool == nil || worker.router == nil || job == nil || job.Args.EventID <= 0 {
		return ErrInvalidDispatcher
	}
	row, err := eventdb.New(worker.pool).GetEvent(ctx, job.Args.EventID)
	if err != nil {
		return fmt.Errorf("load event %d: %w", job.Args.EventID, err)
	}
	record := eventport.Record{
		ID: eventport.EventID(row.ID),
		Event: eventport.Event{
			Type:           row.EventType,
			Payload:        append([]byte(nil), row.Payload...),
			OccurredAt:     row.OccurredAt.Time,
			IdempotencyKey: row.IdempotencyKey,
		},
	}
	if row.CustomerID.Valid {
		record.CustomerID = eventport.CustomerID(row.CustomerID.Int64)
	}
	return worker.router.Consume(ctx, record)
}

func (worker *DeliveryWorker) Timeout(*river.Job[DeliverArgs]) time.Duration {
	return 30 * time.Second
}
