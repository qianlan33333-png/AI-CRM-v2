// Package dispatcher moves committed event_log rows into durable River jobs.
package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/jackc/pgx/v5"
	eventdb "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store/generated"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

const DefaultBatchSize int32 = 100

var (
	ErrInvalidDispatcher = errors.New("invalid event dispatcher")
	ErrDispatchProgress  = errors.New("event dispatch progress mismatch")
)

type transactionalEnqueuer interface {
	EnqueueTx(context.Context, pgx.Tx, platformjobqueue.Queue, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// Dispatcher atomically inserts River delivery jobs and advances event_log
// dispatch progress. It does not run subscribers or call external services.
type Dispatcher struct {
	uow       platformport.UnitOfWork
	enqueuer  transactionalEnqueuer
	batchSize int32
}

func New(uow platformport.UnitOfWork, enqueuer transactionalEnqueuer, batchSize int32) (*Dispatcher, error) {
	if isNil(uow) || isNil(enqueuer) || batchSize <= 0 {
		return nil, ErrInvalidDispatcher
	}
	return &Dispatcher{uow: uow, enqueuer: enqueuer, batchSize: batchSize}, nil
}

func (dispatcher *Dispatcher) Dispatch(ctx context.Context) (int, error) {
	if dispatcher == nil || isNil(dispatcher.uow) || isNil(dispatcher.enqueuer) || dispatcher.batchSize <= 0 {
		return 0, ErrInvalidDispatcher
	}
	var dispatched int
	err := dispatcher.uow.Within(ctx, func(txCtx context.Context) error {
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		queries := eventdb.New(tx)
		events, err := queries.ClaimUndispatchedEvents(txCtx, dispatcher.batchSize)
		if err != nil {
			return fmt.Errorf("claim undispatched events: %w", err)
		}
		if len(events) == 0 {
			dispatched = 0
			return nil
		}
		ids := make([]int64, 0, len(events))
		for _, event := range events {
			if _, err = dispatcher.enqueuer.EnqueueTx(txCtx, tx, platformjobqueue.QueueEvent,
				DeliverArgs{EventID: event.ID}, deliveryInsertOptions()); err != nil {
				return fmt.Errorf("enqueue event %d: %w", event.ID, err)
			}
			ids = append(ids, event.ID)
		}
		updated, err := queries.MarkEventsDispatched(txCtx, ids)
		if err != nil {
			return fmt.Errorf("mark events dispatched: %w", err)
		}
		if updated != int64(len(ids)) {
			return ErrDispatchProgress
		}
		dispatched = len(ids)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return dispatched, nil
}

func deliveryInsertOptions() *river.InsertOpts {
	return &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

func isNil(value any) bool {
	reflected := reflect.ValueOf(value)
	return !reflected.IsValid() ||
		((reflected.Kind() == reflect.Chan || reflected.Kind() == reflect.Func ||
			reflected.Kind() == reflect.Interface || reflected.Kind() == reflect.Map ||
			reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Slice) && reflected.IsNil())
}
