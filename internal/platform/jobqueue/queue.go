package jobqueue

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	queueriver "github.com/riverqueue/river"
)

type Queue string

const (
	QueueCritical Queue = "critical"
	QueueEvent    Queue = "event"
	QueueOutbound Queue = "outbound"
	QueueSync     Queue = "sync"
	QueueHeavy    Queue = "heavy"
	QueueAI       Queue = "ai"
)

const CriticalJobTimeout = 30 * time.Second

var (
	ErrInvalidQueue     = errors.New("invalid River queue")
	ErrInvalidQueuePlan = errors.New("invalid River queue plan")
	ErrUnregisteredJob  = errors.New("River job kind is not registered")
	ErrQueueMismatch    = errors.New("River job queue does not match its registration")
	ErrInvalidWorker    = errors.New("invalid River worker")
)

type QueueConcurrency struct {
	Critical int32
	Event    int32
	Outbound int32
	Sync     int32
	Heavy    int32
	AI       int32
}

func (concurrency QueueConcurrency) Total() int32 {
	return concurrency.Critical + concurrency.Event + concurrency.Outbound +
		concurrency.Sync + concurrency.Heavy + concurrency.AI
}

func (concurrency QueueConcurrency) validate() error {
	if concurrency.Critical <= 0 || concurrency.Event <= 0 || concurrency.Outbound <= 0 ||
		concurrency.Sync <= 0 || concurrency.Heavy <= 0 || concurrency.AI <= 0 {
		return ErrInvalidQueuePlan
	}
	return nil
}

func (concurrency QueueConcurrency) riverQueues() map[string]queueriver.QueueConfig {
	return map[string]queueriver.QueueConfig{
		string(QueueCritical): {MaxWorkers: int(concurrency.Critical)},
		string(QueueEvent):    {MaxWorkers: int(concurrency.Event)},
		string(QueueOutbound): {MaxWorkers: int(concurrency.Outbound)},
		string(QueueSync):     {MaxWorkers: int(concurrency.Sync)},
		string(QueueHeavy):    {MaxWorkers: int(concurrency.Heavy)},
		string(QueueAI):       {MaxWorkers: int(concurrency.AI)},
	}
}

func validQueue(queue Queue) bool {
	switch queue {
	case QueueCritical, QueueEvent, QueueOutbound, QueueSync, QueueHeavy, QueueAI:
		return true
	default:
		return false
	}
}

type WorkerRegistry struct {
	workers     *queueriver.Workers
	assignments map[string]Queue
}

func NewWorkerRegistry() *WorkerRegistry {
	return &WorkerRegistry{workers: queueriver.NewWorkers(), assignments: make(map[string]Queue)}
}

func AddWorker[T queueriver.JobArgs](registry *WorkerRegistry, queue Queue, worker queueriver.Worker[T]) error {
	if registry == nil || registry.workers == nil || registry.assignments == nil || !validQueue(queue) || isNil(worker) {
		return ErrInvalidWorker
	}
	var args T
	kind := args.Kind()
	if kind == "" {
		return ErrInvalidWorker
	}
	if _, exists := registry.assignments[kind]; exists {
		return fmt.Errorf("%w: duplicate job kind", ErrInvalidWorker)
	}
	wrapped := &queueWorker[T]{Worker: worker, queue: queue}
	if err := queueriver.AddWorkerSafely(registry.workers, wrapped); err != nil {
		return fmt.Errorf("%w: registration failed", ErrInvalidWorker)
	}
	registry.assignments[kind] = queue
	return nil
}

func isNil(value any) bool {
	reflected := reflect.ValueOf(value)
	return !reflected.IsValid() ||
		((reflected.Kind() == reflect.Chan || reflected.Kind() == reflect.Func ||
			reflected.Kind() == reflect.Interface || reflected.Kind() == reflect.Map ||
			reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Slice) && reflected.IsNil())
}

type queueWorker[T queueriver.JobArgs] struct {
	queueriver.Worker[T]
	queue Queue
}

// ExplicitOptions validates that an enqueue request uses the worker's frozen
// queue and returns a defensive copy with that queue set explicitly.
func (registry *WorkerRegistry) ExplicitOptions(queue Queue, args queueriver.JobArgs, options *queueriver.InsertOpts) (*queueriver.InsertOpts, error) {
	if registry == nil || registry.assignments == nil || !validQueue(queue) || isNil(args) {
		return nil, ErrInvalidQueue
	}
	registeredQueue, ok := registry.assignments[args.Kind()]
	if !ok {
		return nil, ErrUnregisteredJob
	}
	if registeredQueue != queue {
		return nil, ErrQueueMismatch
	}
	cloned := queueriver.InsertOpts{}
	if options != nil {
		cloned = *options
	}
	if cloned.Queue != "" && cloned.Queue != string(queue) {
		return nil, ErrQueueMismatch
	}
	cloned.Queue = string(queue)
	return &cloned, nil
}

func (worker *queueWorker[T]) Timeout(job *queueriver.Job[T]) time.Duration {
	timeout := worker.Worker.Timeout(job)
	if worker.queue == QueueCritical && (timeout <= 0 || timeout > CriticalJobTimeout) {
		return CriticalJobTimeout
	}
	return timeout
}
