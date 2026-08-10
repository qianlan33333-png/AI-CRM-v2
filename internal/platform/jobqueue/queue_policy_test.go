package jobqueue

import (
	"context"
	"errors"
	"testing"
	"time"

	queueriver "github.com/riverqueue/river"
)

type queuePolicyArgs struct{}

func (queuePolicyArgs) Kind() string { return "p2_04_queue_policy" }

type queuePolicyEmptyKindArgs struct{}

func (queuePolicyEmptyKindArgs) Kind() string { return "" }

type queuePolicyUnregisteredArgs struct{}

func (queuePolicyUnregisteredArgs) Kind() string { return "p2_04_queue_policy_unregistered" }

type queuePolicyWorker[T queueriver.JobArgs] struct {
	queueriver.WorkerDefaults[T]
	timeout time.Duration
}

func (*queuePolicyWorker[T]) Work(context.Context, *queueriver.Job[T]) error {
	return nil
}

func (worker *queuePolicyWorker[T]) Timeout(*queueriver.Job[T]) time.Duration {
	return worker.timeout
}

func TestQueuePolicyConcurrency(t *testing.T) {
	valid := QueueConcurrency{Critical: 2, Event: 3, Outbound: 5, Sync: 7, Heavy: 11, AI: 13}
	criticalZero := valid
	criticalZero.Critical = 0
	eventNegative := valid
	eventNegative.Event = -1
	outboundZero := valid
	outboundZero.Outbound = 0
	syncNegative := valid
	syncNegative.Sync = -1
	heavyZero := valid
	heavyZero.Heavy = 0
	aiNegative := valid
	aiNegative.AI = -1

	tests := []struct {
		name        string
		concurrency QueueConcurrency
		wantErr     error
		wantTotal   int32
	}{
		{name: "all six role queues are positive", concurrency: valid, wantTotal: 41},
		{name: "critical must be positive", concurrency: criticalZero, wantErr: ErrInvalidQueuePlan},
		{name: "event must be positive", concurrency: eventNegative, wantErr: ErrInvalidQueuePlan},
		{name: "outbound must be positive", concurrency: outboundZero, wantErr: ErrInvalidQueuePlan},
		{name: "sync must be positive", concurrency: syncNegative, wantErr: ErrInvalidQueuePlan},
		{name: "heavy must be positive", concurrency: heavyZero, wantErr: ErrInvalidQueuePlan},
		{name: "ai must be positive", concurrency: aiNegative, wantErr: ErrInvalidQueuePlan},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.concurrency.validate()
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("validate() error = %v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validate() error = %v", err)
			}
			if got := test.concurrency.Total(); got != test.wantTotal {
				t.Fatalf("Total() = %d, want %d", got, test.wantTotal)
			}

			wantQueues := map[string]int{
				string(QueueCritical): int(test.concurrency.Critical),
				string(QueueEvent):    int(test.concurrency.Event),
				string(QueueOutbound): int(test.concurrency.Outbound),
				string(QueueSync):     int(test.concurrency.Sync),
				string(QueueHeavy):    int(test.concurrency.Heavy),
				string(QueueAI):       int(test.concurrency.AI),
			}
			gotQueues := test.concurrency.riverQueues()
			if len(gotQueues) != len(wantQueues) {
				t.Fatalf("riverQueues() count = %d, want %d: %#v", len(gotQueues), len(wantQueues), gotQueues)
			}
			for queue, wantWorkers := range wantQueues {
				got, ok := gotQueues[queue]
				if !ok || got.MaxWorkers != wantWorkers {
					t.Fatalf("riverQueues()[%q] = %#v, want MaxWorkers %d", queue, got, wantWorkers)
				}
			}
		})
	}
}

func TestQueuePolicyWhitelist(t *testing.T) {
	tests := []struct {
		name      string
		queue     Queue
		wantValid bool
	}{
		{name: "critical", queue: QueueCritical, wantValid: true},
		{name: "event", queue: QueueEvent, wantValid: true},
		{name: "outbound", queue: QueueOutbound, wantValid: true},
		{name: "sync", queue: QueueSync, wantValid: true},
		{name: "heavy", queue: QueueHeavy, wantValid: true},
		{name: "ai", queue: QueueAI, wantValid: true},
		{name: "default is rejected", queue: Queue(queueriver.QueueDefault)},
		{name: "empty is rejected", queue: ""},
		{name: "unknown is rejected", queue: "p2_04_unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validQueue(test.queue); got != test.wantValid {
				t.Fatalf("validQueue(%q) = %t, want %t", test.queue, got, test.wantValid)
			}
		})
	}
}

func TestQueuePolicyWorkerRegistration(t *testing.T) {
	tests := []struct {
		name            string
		setup           func(*WorkerRegistry) error
		register        func(*WorkerRegistry) error
		wantErr         error
		wantAssignments int
		wantQueue       Queue
	}{
		{
			name: "registers a worker on its explicit queue",
			register: func(registry *WorkerRegistry) error {
				return AddWorker(registry, QueueCritical, &queuePolicyWorker[queuePolicyArgs]{})
			},
			wantAssignments: 1,
			wantQueue:       QueueCritical,
		},
		{
			name: "rejects duplicate kind",
			setup: func(registry *WorkerRegistry) error {
				return AddWorker(registry, QueueCritical, &queuePolicyWorker[queuePolicyArgs]{})
			},
			register: func(registry *WorkerRegistry) error {
				return AddWorker(registry, QueueEvent, &queuePolicyWorker[queuePolicyArgs]{})
			},
			wantErr:         ErrInvalidWorker,
			wantAssignments: 1,
			wantQueue:       QueueCritical,
		},
		{
			name: "rejects nil registry",
			register: func(*WorkerRegistry) error {
				return AddWorker[queuePolicyArgs](nil, QueueCritical, &queuePolicyWorker[queuePolicyArgs]{})
			},
			wantErr: ErrInvalidWorker,
		},
		{
			name: "rejects typed nil worker",
			register: func(registry *WorkerRegistry) error {
				var worker *queuePolicyWorker[queuePolicyArgs]
				return AddWorker(registry, QueueCritical, worker)
			},
			wantErr: ErrInvalidWorker,
		},
		{
			name: "rejects empty kind",
			register: func(registry *WorkerRegistry) error {
				return AddWorker(registry, QueueCritical, &queuePolicyWorker[queuePolicyEmptyKindArgs]{})
			},
			wantErr: ErrInvalidWorker,
		},
		{
			name: "rejects default queue",
			register: func(registry *WorkerRegistry) error {
				return AddWorker(registry, Queue(queueriver.QueueDefault), &queuePolicyWorker[queuePolicyArgs]{})
			},
			wantErr: ErrInvalidWorker,
		},
		{
			name: "rejects unknown queue",
			register: func(registry *WorkerRegistry) error {
				return AddWorker(registry, Queue("p2_04_unknown"), &queuePolicyWorker[queuePolicyArgs]{})
			},
			wantErr: ErrInvalidWorker,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := NewWorkerRegistry()
			if test.setup != nil {
				if err := test.setup(registry); err != nil {
					t.Fatalf("setup worker registration error = %v", err)
				}
			}

			err := test.register(registry)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("AddWorker() error = %v, want %v", err, test.wantErr)
				}
			} else if err != nil {
				t.Fatalf("AddWorker() error = %v", err)
			}
			if got := len(registry.assignments); got != test.wantAssignments {
				t.Fatalf("assignment count = %d, want %d", got, test.wantAssignments)
			}
			if test.wantAssignments > 0 {
				if got := registry.assignments[queuePolicyArgs{}.Kind()]; got != test.wantQueue {
					t.Fatalf("assignment queue = %q, want %q", got, test.wantQueue)
				}
			}
		})
	}
}

func TestQueuePolicyWorkerTimeouts(t *testing.T) {
	tests := []struct {
		name    string
		queue   Queue
		timeout time.Duration
		want    time.Duration
	}{
		{name: "critical default is capped", queue: QueueCritical, want: CriticalJobTimeout},
		{name: "critical shorter timeout is retained", queue: QueueCritical, timeout: 5 * time.Second, want: 5 * time.Second},
		{name: "critical timeout at cap is retained", queue: QueueCritical, timeout: CriticalJobTimeout, want: CriticalJobTimeout},
		{name: "critical timeout above cap is reduced", queue: QueueCritical, timeout: CriticalJobTimeout + time.Nanosecond, want: CriticalJobTimeout},
		{name: "critical infinite timeout is capped", queue: QueueCritical, timeout: -1, want: CriticalJobTimeout},
		{name: "event default timeout is retained", queue: QueueEvent, want: 0},
		{name: "outbound infinite timeout is retained", queue: QueueOutbound, timeout: -1, want: -1},
		{name: "sync long timeout is retained", queue: QueueSync, timeout: time.Minute, want: time.Minute},
		{name: "heavy short timeout is retained", queue: QueueHeavy, timeout: 5 * time.Second, want: 5 * time.Second},
		{name: "ai negative timeout is retained", queue: QueueAI, timeout: -2, want: -2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			worker := &queueWorker[queuePolicyArgs]{
				Worker: &queuePolicyWorker[queuePolicyArgs]{timeout: test.timeout},
				queue:  test.queue,
			}
			if got := worker.Timeout(nil); got != test.want {
				t.Fatalf("Timeout() = %s, want %s", got, test.want)
			}
		})
	}
}

func TestQueuePolicyExplicitOptions(t *testing.T) {
	registry := NewWorkerRegistry()
	if err := AddWorker(registry, QueueCritical, &queuePolicyWorker[queuePolicyArgs]{}); err != nil {
		t.Fatalf("AddWorker() error = %v", err)
	}
	client := &Client{registry: registry}

	tests := []struct {
		name      string
		queue     Queue
		args      queueriver.JobArgs
		options   *queueriver.InsertOpts
		wantErr   error
		wantQueue string
	}{
		{name: "injects queue for nil options", queue: QueueCritical, args: queuePolicyArgs{}, wantQueue: string(QueueCritical)},
		{name: "injects queue for empty options", queue: QueueCritical, args: queuePolicyArgs{}, options: &queueriver.InsertOpts{}, wantQueue: string(QueueCritical)},
		{name: "accepts matching explicit queue", queue: QueueCritical, args: queuePolicyArgs{}, options: &queueriver.InsertOpts{Queue: string(QueueCritical)}, wantQueue: string(QueueCritical)},
		{name: "rejects caller default queue", queue: QueueCritical, args: queuePolicyArgs{}, options: &queueriver.InsertOpts{Queue: queueriver.QueueDefault}, wantErr: ErrQueueMismatch},
		{name: "rejects caller wrong queue", queue: QueueCritical, args: queuePolicyArgs{}, options: &queueriver.InsertOpts{Queue: string(QueueHeavy)}, wantErr: ErrQueueMismatch},
		{name: "rejects requested queue mismatch", queue: QueueHeavy, args: queuePolicyArgs{}, wantErr: ErrQueueMismatch},
		{name: "rejects unregistered kind", queue: QueueCritical, args: queuePolicyUnregisteredArgs{}, wantErr: ErrUnregisteredJob},
		{name: "rejects default requested queue", queue: Queue(queueriver.QueueDefault), args: queuePolicyArgs{}, wantErr: ErrInvalidQueue},
		{name: "rejects unknown requested queue", queue: Queue("p2_04_unknown"), args: queuePolicyArgs{}, wantErr: ErrInvalidQueue},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := client.explicitOptions(test.queue, test.args, test.options)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("explicitOptions() error = %v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("explicitOptions() error = %v", err)
			}
			if got == nil || got.Queue != test.wantQueue {
				t.Fatalf("explicitOptions() = %#v, want Queue %q", got, test.wantQueue)
			}
		})
	}
}
