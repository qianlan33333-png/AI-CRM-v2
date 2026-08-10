package jobqueue

import (
	"context"
	"errors"
	"testing"
	"time"

	queueriver "github.com/riverqueue/river"
)

type testQueueArgs struct{}

func (testQueueArgs) Kind() string { return "p2s04_test_queue" }

type testQueueWorker struct {
	queueriver.WorkerDefaults[testQueueArgs]
	timeout time.Duration
}

func (worker *testQueueWorker) Work(context.Context, *queueriver.Job[testQueueArgs]) error {
	return nil
}

func (worker *testQueueWorker) Timeout(*queueriver.Job[testQueueArgs]) time.Duration {
	return worker.timeout
}

func TestCriticalWorkerTimeoutIsCapped(t *testing.T) {
	for _, test := range []struct {
		name    string
		timeout time.Duration
		want    time.Duration
	}{
		{name: "default", want: CriticalJobTimeout},
		{name: "shorter", timeout: 5 * time.Second, want: 5 * time.Second},
		{name: "longer", timeout: time.Minute, want: CriticalJobTimeout},
		{name: "infinite", timeout: -1, want: CriticalJobTimeout},
	} {
		t.Run(test.name, func(t *testing.T) {
			wrapped := queueWorker[testQueueArgs]{Worker: &testQueueWorker{timeout: test.timeout}, queue: QueueCritical}
			if got := wrapped.Timeout(nil); got != test.want {
				t.Fatalf("Timeout() = %s, want %s", got, test.want)
			}
		})
	}
	heavy := queueWorker[testQueueArgs]{Worker: &testQueueWorker{timeout: -1}, queue: QueueHeavy}
	if got := heavy.Timeout(nil); got != -1 {
		t.Fatalf("heavy Timeout() = %s, want worker override", got)
	}
}

func TestQueuePlanAndWorkerRegistrationFailClosed(t *testing.T) {
	valid := QueueConcurrency{Critical: 2, Event: 1, Outbound: 1, Sync: 1, Heavy: 1, AI: 1}
	if err := valid.validate(); err != nil || valid.Total() != 7 || len(valid.riverQueues()) != 6 {
		t.Fatalf("valid queue plan = total %d, queues %d, error %v", valid.Total(), len(valid.riverQueues()), err)
	}
	invalid := valid
	invalid.AI = 0
	if err := invalid.validate(); !errors.Is(err, ErrInvalidQueuePlan) {
		t.Fatalf("invalid queue plan error = %v", err)
	}
	registry := NewWorkerRegistry()
	if err := AddWorker(registry, QueueCritical, &testQueueWorker{}); err != nil {
		t.Fatalf("AddWorker() error = %v", err)
	}
	if got := registry.assignments[testQueueArgs{}.Kind()]; got != QueueCritical {
		t.Fatalf("registered queue = %q", got)
	}
	if err := AddWorker(registry, Queue("default"), &testQueueWorker{}); !errors.Is(err, ErrInvalidWorker) {
		t.Fatalf("default queue registration error = %v", err)
	}
}

func TestExplicitOptionsRejectDefaultMismatchAndUnknownKind(t *testing.T) {
	registry := NewWorkerRegistry()
	if err := AddWorker(registry, QueueCritical, &testQueueWorker{}); err != nil {
		t.Fatal(err)
	}
	client := &Client{registry: registry}
	options, err := client.explicitOptions(QueueCritical, testQueueArgs{}, nil)
	if err != nil || options.Queue != string(QueueCritical) {
		t.Fatalf("explicitOptions() = %#v, %v", options, err)
	}
	_, err = client.explicitOptions(QueueCritical, testQueueArgs{}, &queueriver.InsertOpts{Queue: queueriver.QueueDefault})
	if !errors.Is(err, ErrQueueMismatch) {
		t.Fatalf("default options error = %v", err)
	}
	_, err = client.explicitOptions(QueueHeavy, testQueueArgs{}, nil)
	if !errors.Is(err, ErrQueueMismatch) {
		t.Fatalf("wrong queue error = %v", err)
	}
	_, err = client.explicitOptions(QueueCritical, unknownQueueArgs{}, nil)
	if !errors.Is(err, ErrUnregisteredJob) {
		t.Fatalf("unknown job error = %v", err)
	}
	var typedNil *pointerQueueArgs
	_, err = client.explicitOptions(QueueCritical, typedNil, nil)
	if !errors.Is(err, ErrInvalidQueue) {
		t.Fatalf("typed nil args error = %v", err)
	}
}

type unknownQueueArgs struct{}

func (unknownQueueArgs) Kind() string { return "p2s04_unknown_queue" }

type pointerQueueArgs struct{}

func (*pointerQueueArgs) Kind() string { return "p2s05_pointer_queue" }
