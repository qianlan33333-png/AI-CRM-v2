// Package dispatcher moves committed event_log rows into durable River jobs.
package dispatcher

import (
	"context"
	"errors"
	"reflect"
	"sync"

	"github.com/jackc/pgx/v5"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

const DefaultBatchSize int32 = 100

var (
	ErrInvalidDispatcher = errors.New("invalid event dispatcher")
	ErrEnqueuerBound     = errors.New("event dispatcher enqueuer is already bound")
)

type transactionalEnqueuer interface {
	EnqueueTx(context.Context, pgx.Tx, platformjobqueue.Queue, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// DeferredEnqueuer breaks the worker/client construction cycle with a single,
// fail-closed bind before the River runtime starts. It cannot be rebound.
type DeferredEnqueuer struct {
	mu       sync.RWMutex
	enqueuer transactionalEnqueuer
}

func NewDeferredEnqueuer() *DeferredEnqueuer {
	return &DeferredEnqueuer{}
}

func (reference *DeferredEnqueuer) Bind(enqueuer transactionalEnqueuer) error {
	if reference == nil || isNil(enqueuer) {
		return ErrInvalidDispatcher
	}
	reference.mu.Lock()
	defer reference.mu.Unlock()
	if reference.enqueuer != nil {
		return ErrEnqueuerBound
	}
	reference.enqueuer = enqueuer
	return nil
}

func (reference *DeferredEnqueuer) EnqueueTx(ctx context.Context, tx pgx.Tx, queue platformjobqueue.Queue, args river.JobArgs, options *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	if reference == nil {
		return nil, ErrInvalidDispatcher
	}
	reference.mu.RLock()
	enqueuer := reference.enqueuer
	reference.mu.RUnlock()
	if isNil(enqueuer) {
		return nil, platformjobqueue.ErrClientUnavailable
	}
	return enqueuer.EnqueueTx(ctx, tx, queue, args, options)
}

type dispatchStore interface {
	Dispatch(context.Context) (int, error)
}

// Dispatcher is a SQL-free runtime wrapper around the Events-owned store.
type Dispatcher struct {
	store dispatchStore
}

func New(store dispatchStore) (*Dispatcher, error) {
	if isNil(store) {
		return nil, ErrInvalidDispatcher
	}
	return &Dispatcher{store: store}, nil
}

func (dispatcher *Dispatcher) Dispatch(ctx context.Context) (int, error) {
	if dispatcher == nil || isNil(dispatcher.store) {
		return 0, ErrInvalidDispatcher
	}
	return dispatcher.store.Dispatch(ctx)
}

func isNil(value any) bool {
	reflected := reflect.ValueOf(value)
	return !reflected.IsValid() ||
		((reflected.Kind() == reflect.Chan || reflected.Kind() == reflect.Func ||
			reflected.Kind() == reflect.Interface || reflected.Kind() == reflect.Map ||
			reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Slice) && reflected.IsNil())
}
