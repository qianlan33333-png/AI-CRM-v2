package worker

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/riverqueue/river"
)

const (
	EventPartitionMaintenanceJobKind       = "contact_event_partitions.ensure"
	eventPartitionFutureMonths       int32 = 3
	eventPartitionJobTimeout               = 2 * time.Minute
)

var ErrInvalidEventPartitionWorker = errors.New("invalid customer event partition worker")

type EventPartitionEnsurer interface {
	EnsureEventPartitions(context.Context, time.Time, int32) error
}

type EventPartitionMaintenanceArgs struct{}

func (EventPartitionMaintenanceArgs) Kind() string {
	return EventPartitionMaintenanceJobKind
}

type EventPartitionMaintenanceWorker struct {
	river.WorkerDefaults[EventPartitionMaintenanceArgs]
	ensurer EventPartitionEnsurer
	clock   func() time.Time
}

func NewEventPartitionMaintenanceWorker(
	ensurer EventPartitionEnsurer,
	clock func() time.Time,
) (*EventPartitionMaintenanceWorker, error) {
	if isNilWorkerDependency(ensurer) || clock == nil {
		return nil, ErrInvalidEventPartitionWorker
	}
	return &EventPartitionMaintenanceWorker{ensurer: ensurer, clock: clock}, nil
}

func (worker *EventPartitionMaintenanceWorker) Work(
	ctx context.Context,
	job *river.Job[EventPartitionMaintenanceArgs],
) error {
	if worker == nil || isNilWorkerDependency(worker.ensurer) || worker.clock == nil ||
		ctx == nil || job == nil {
		return ErrInvalidEventPartitionWorker
	}
	anchor := worker.clock()
	if anchor.IsZero() {
		return ErrInvalidEventPartitionWorker
	}
	return worker.ensurer.EnsureEventPartitions(ctx, anchor.UTC(), eventPartitionFutureMonths)
}

func (*EventPartitionMaintenanceWorker) Timeout(
	*river.Job[EventPartitionMaintenanceArgs],
) time.Duration {
	return eventPartitionJobTimeout
}

func isNilWorkerDependency(value any) bool {
	reflected := reflect.ValueOf(value)
	return !reflected.IsValid() ||
		((reflected.Kind() == reflect.Chan || reflected.Kind() == reflect.Func ||
			reflected.Kind() == reflect.Interface || reflected.Kind() == reflect.Map ||
			reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Slice) && reflected.IsNil())
}
