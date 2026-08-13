package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type enqueueTestUoW struct{}

func (enqueueTestUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type enqueueTestTasks struct {
	calls int
	id    TaskID
}

func (tasks *enqueueTestTasks) CreateAcceptedTask(context.Context, OneCommand) (TaskID, error) {
	tasks.calls++
	return tasks.id, nil
}

type enqueueTestEvents struct {
	calls int
	id    eventport.EventID
}

func (events *enqueueTestEvents) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	events.calls++
	return events.id, nil
}

type enqueueTestReceipts struct {
	reserved EnqueueReceipt
	accepted EnqueueReceipt
}

func (receipts *enqueueTestReceipts) ReserveEnqueueReceipt(context.Context, EnqueueOneCommand) (EnqueueReceipt, error) {
	return receipts.reserved, nil
}

func (receipts *enqueueTestReceipts) AcceptEnqueueReceipt(context.Context, int64, TaskID, eventport.EventID, int64) (EnqueueReceipt, error) {
	return receipts.accepted, nil
}

type enqueueTestJobs struct {
	calls int
	id    int64
}

func (jobs *enqueueTestJobs) EnqueueOne(context.Context, EnqueueOneArgs) (int64, error) {
	jobs.calls++
	return jobs.id, nil
}

func TestEnqueueOnePersistsTaskEventAndOutboundJob(t *testing.T) {
	command := testEnqueueOneCommand()
	tasks := &enqueueTestTasks{id: 41}
	events := &enqueueTestEvents{id: 42}
	receipts := &enqueueTestReceipts{
		reserved: EnqueueReceipt{ID: 43, Command: command, State: EnqueueReceiptReserved},
		accepted: EnqueueReceipt{ID: 43, Command: command, State: EnqueueReceiptAccepted, TaskID: 41, EventID: 42, RiverJobID: 44},
	}
	jobs := &enqueueTestJobs{id: 44}
	service := NewEnqueueOneService(enqueueTestUoW{}, tasks, events, receipts, jobs)

	got, err := service.Enqueue(context.Background(), command)
	if err != nil || got != (EnqueuedTask{TaskID: 41, EventID: 42, RiverJobID: 44}) {
		t.Fatalf("Enqueue() = %#v, %v", got, err)
	}
	if tasks.calls != 1 || events.calls != 1 || jobs.calls != 1 {
		t.Fatalf("task/event/job calls = %d/%d/%d, want 1/1/1", tasks.calls, events.calls, jobs.calls)
	}
}

func TestEnqueueOneReturnsOriginalFactsForSameCommand(t *testing.T) {
	command := testEnqueueOneCommand()
	tasks := &enqueueTestTasks{id: 99}
	events := &enqueueTestEvents{id: 98}
	receipts := &enqueueTestReceipts{reserved: EnqueueReceipt{
		ID: 43, Command: command, State: EnqueueReceiptAccepted, TaskID: 41, EventID: 42, RiverJobID: 44,
	}}
	jobs := &enqueueTestJobs{id: 97}
	service := NewEnqueueOneService(enqueueTestUoW{}, tasks, events, receipts, jobs)

	got, err := service.Enqueue(context.Background(), command)
	if err != nil || got != (EnqueuedTask{TaskID: 41, EventID: 42, RiverJobID: 44}) {
		t.Fatalf("Enqueue() = %#v, %v", got, err)
	}
	if tasks.calls != 0 || events.calls != 0 || jobs.calls != 0 {
		t.Fatalf("replay task/event/job calls = %d/%d/%d, want 0/0/0", tasks.calls, events.calls, jobs.calls)
	}
}

func TestEnqueueOneRejectsDifferentCommandForSameKey(t *testing.T) {
	command := testEnqueueOneCommand()
	stored := command
	stored.Payload = json.RawMessage(`{"text":"other"}`)
	service := NewEnqueueOneService(
		enqueueTestUoW{}, &enqueueTestTasks{id: 41}, &enqueueTestEvents{id: 42},
		&enqueueTestReceipts{reserved: EnqueueReceipt{ID: 43, Command: stored, State: EnqueueReceiptAccepted, TaskID: 41, EventID: 42, RiverJobID: 44}},
		&enqueueTestJobs{id: 44},
	)

	_, err := service.Enqueue(context.Background(), command)
	if !errors.Is(err, ErrEnqueueOneConflict) {
		t.Fatalf("Enqueue() error = %v, want %v", err, ErrEnqueueOneConflict)
	}
}

func testEnqueueOneCommand() EnqueueOneCommand {
	return EnqueueOneCommand{
		OneCommand:       OneCommand{CustomerID: 7, TemplateKey: TemplateTextNoticeV1, Payload: json.RawMessage(`{"text":"one"}`)},
		IdempotencyScope: "operator:7",
		IdempotencyKey:   "outbound-enqueue-one-command",
	}
}
