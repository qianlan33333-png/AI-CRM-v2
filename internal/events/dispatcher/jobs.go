package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	"github.com/riverqueue/river"
)

const (
	DispatchJobKind = "events_dispatch"
	DeliverJobKind  = eventport.DeliveryJobKind
)

var (
	ErrInvalidSubscriber = errors.New("invalid event subscriber")
	ErrNoSubscriber      = errors.New("no subscriber for event type")
)

type DispatchArgs struct{}

func (DispatchArgs) Kind() string { return DispatchJobKind }

type DeliverArgs = eventport.DeliveryJobArgs

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
	subscribers       map[string][]eventport.Subscriber
	deliveryConsumers map[string]eventport.DeliverySubscriber
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
	return &Router{subscribers: routes, deliveryConsumers: make(map[string]eventport.DeliverySubscriber)}, nil
}

func (router *Router) RegisterDelivery(subscriber eventport.DeliverySubscriber) error {
	if router == nil || isNil(subscriber) {
		return ErrInvalidSubscriber
	}
	consumer := strings.TrimSpace(subscriber.Consumer())
	types := subscriber.EventTypes()
	if consumer == "" || len(types) == 0 || router.deliveryConsumers[consumer] != nil {
		return ErrInvalidSubscriber
	}
	seen := make(map[string]struct{}, len(types))
	for _, eventType := range types {
		if eventType = strings.TrimSpace(eventType); eventType == "" {
			return ErrInvalidSubscriber
		}
		if _, duplicate := seen[eventType]; duplicate {
			return ErrInvalidSubscriber
		}
		seen[eventType] = struct{}{}
	}
	router.deliveryConsumers[consumer] = subscriber
	return nil
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

func (router *Router) ConsumeDelivery(ctx context.Context, claim eventport.DeliveryClaim) error {
	if router == nil || claim.Record.ID <= 0 || claim.Consumer == "" {
		return ErrInvalidSubscriber
	}
	subscriber := router.deliveryConsumers[claim.Consumer]
	if subscriber == nil {
		return fmt.Errorf("%w: %s", ErrNoSubscriber, claim.Consumer)
	}
	allowed := false
	for _, eventType := range subscriber.EventTypes() {
		allowed = allowed || eventType == claim.Record.Type
	}
	if !allowed {
		return eventport.PoisonDelivery(fmt.Errorf("consumer %s rejects event type %s", claim.Consumer, claim.Record.Type))
	}
	return subscriber.ConsumeDelivery(ctx, claim)
}

type DeliveryWorker struct {
	river.WorkerDefaults[DeliverArgs]
	deliveries eventport.DeliveryRuntime
	router     *Router
}

func NewDeliveryWorker(deliveries eventport.DeliveryRuntime, router *Router) (*DeliveryWorker, error) {
	if isNil(deliveries) || router == nil {
		return nil, ErrInvalidDispatcher
	}
	return &DeliveryWorker{deliveries: deliveries, router: router}, nil
}

func (worker *DeliveryWorker) Work(ctx context.Context, job *river.Job[DeliverArgs]) error {
	if worker == nil || isNil(worker.deliveries) || worker.router == nil || job == nil || job.Args.EventID <= 0 {
		return ErrInvalidDispatcher
	}
	if job.Args.Consumer == "" {
		record, err := worker.deliveries.Load(ctx, eventport.EventID(job.Args.EventID))
		if err != nil {
			return fmt.Errorf("load event %d: %w", job.Args.EventID, err)
		}
		return worker.router.Consume(ctx, record)
	}
	owner := "river:" + strconv.FormatInt(job.ID, 10)
	claim, err := worker.deliveries.Claim(ctx, eventport.EventID(job.Args.EventID), job.Args.Consumer, owner, time.Minute)
	if err != nil {
		return err
	}
	if claim.Status == eventport.DeliveryCompleted || claim.Status == eventport.DeliveryFinalFailed || claim.Status == eventport.DeliveryOutcomeUnknown {
		return nil
	}
	consumeErr := worker.router.ConsumeDelivery(ctx, claim)
	if consumeErr == nil {
		return nil
	}
	if errors.Is(consumeErr, eventport.ErrDeliveryOutcomeUnknown) {
		return worker.deliveries.OutcomeUnknown(ctx, claim.Record.ID, claim.Consumer, claim.Owner, "outcome_unknown")
	}
	if errors.Is(consumeErr, eventport.ErrDeliveryPoison) || errors.Is(consumeErr, ErrNoSubscriber) {
		return worker.deliveries.FinalFail(ctx, claim.Record.ID, claim.Consumer, claim.Owner, "poison")
	}
	if job.Attempt >= job.MaxAttempts {
		return worker.deliveries.FinalFail(ctx, claim.Record.ID, claim.Consumer, claim.Owner, "retry_exhausted")
	}
	if retryErr := worker.deliveries.Retry(ctx, claim.Record.ID, claim.Consumer, claim.Owner, "transient"); retryErr != nil {
		return errors.Join(consumeErr, retryErr)
	}
	return consumeErr
}

func (worker *DeliveryWorker) Timeout(*river.Job[DeliverArgs]) time.Duration {
	return 30 * time.Second
}
