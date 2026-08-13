package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTaskQueryServiceListGetAndReconcile(t *testing.T) {
	now := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)
	owner := int64(42)
	task := TaskReadModel{
		TaskID: 71, CustomerID: 81, OwnerStaffID: &owner, BatchID: int64Pointer(91), BatchChunkIndex: int32Pointer(0),
		Status: TaskStatusSent, AttemptCount: 2, CurrentAttemptID: int64Pointer(111), ProviderMessageID: "provider-71",
		Job:       TaskJob{TaskID: 71, Generation: 1, RiverJobID: 101, JobKind: OutboundEnqueueBatchJobKind},
		CreatedAt: now.Add(-time.Hour), StatusUpdatedAt: now, SentAt: &now,
	}
	store := &taskQueryStoreStub{
		get: task,
		list: []TaskReadModel{task, {TaskID: 70, CustomerID: 80, OwnerStaffID: &owner,
			BatchID: int64Pointer(91), BatchChunkIndex: int32Pointer(0), Status: TaskStatusPending,
			Job:       TaskJob{TaskID: 70, Generation: 1, RiverJobID: 100, JobKind: OutboundEnqueueOneJobKind},
			CreatedAt: now.Add(-2 * time.Hour), StatusUpdatedAt: now.Add(-2 * time.Hour)}},
		attempts: []AttemptReadModel{{ID: 111, HistoryID: 112, TaskID: 71, Generation: 1,
			RiverJobID: 101, RiverAttempt: 2, RiverMaxAttempts: 3, State: SendAttemptSucceeded,
			ProviderMessageID: "provider-71", CompletedAt: &now}},
		receipts: []ControlReceiptReadModel{{ID: 121, TaskID: 71, Operation: "manual_retry",
			State: ControlReceiptCompleted, Job: TaskJob{TaskID: 71, Generation: 2, RiverJobID: 102,
				JobKind: OutboundEnqueueBatchJobKind}, EventID: 122, TaskStatus: TaskStatusPending,
			CreatedAt: now.Add(-time.Minute), CompletedAt: now}},
	}
	service := NewTaskQueryService(inlineTaskQueryUOW{}, store)

	listed, err := service.List(context.Background(), TaskListQuery{BatchID: int64Pointer(91), OwnerStaffID: &owner, Limit: 1})
	if err != nil || len(listed.Items) != 1 || !listed.HasMore || listed.Items[0].TaskID != 71 {
		t.Fatalf("List()=%+v err=%v", listed, err)
	}
	if store.lastList.Limit != 2 || store.lastList.OwnerStaffID == &owner {
		t.Fatalf("store list=%+v, want copied owner and limit+1", store.lastList)
	}

	got, err := service.Get(context.Background(), TaskGetQuery{TaskID: 71, OwnerStaffID: &owner})
	if err != nil || got.TaskID != 71 || got.OwnerStaffID == task.OwnerStaffID {
		t.Fatalf("Get()=%+v err=%v", got, err)
	}

	reconciled, err := service.Reconcile(context.Background(), TaskGetQuery{TaskID: 71, OwnerStaffID: &owner})
	if err != nil || reconciled.Task.TaskID != 71 || len(reconciled.Attempts) != 1 || len(reconciled.ControlReceipts) != 1 {
		t.Fatalf("Reconcile()=%+v err=%v", reconciled, err)
	}
}

func TestTaskQueryServiceFailsClosedOnInvalidScopedAndStoreResults(t *testing.T) {
	service := NewTaskQueryService(inlineTaskQueryUOW{}, &taskQueryStoreStub{})
	for _, query := range []TaskListQuery{
		{}, {Limit: 101}, {Limit: 1, Offset: -1}, {Limit: 1, BatchID: int64Pointer(0)},
		{Limit: 1, OwnerStaffID: int64Pointer(-1)}, {Limit: 1, Status: TaskStatus("delivered")},
	} {
		if _, err := service.List(context.Background(), query); !errors.Is(err, ErrInvalidTaskQuery) {
			t.Fatalf("List(%+v) err=%v, want invalid", query, err)
		}
	}
	if _, err := service.Get(context.Background(), TaskGetQuery{}); !errors.Is(err, ErrInvalidTaskQuery) {
		t.Fatalf("Get invalid err=%v", err)
	}

	now := time.Now().UTC()
	bad := &taskQueryStoreStub{get: TaskReadModel{TaskID: 7, CustomerID: 8, Status: TaskStatusSent,
		Job:       TaskJob{TaskID: 7, Generation: 1, RiverJobID: 9, JobKind: OutboundEnqueueOneJobKind},
		CreatedAt: now, StatusUpdatedAt: now}}
	if _, err := NewTaskQueryService(inlineTaskQueryUOW{}, bad).Get(context.Background(), TaskGetQuery{TaskID: 7}); !errors.Is(err, ErrTaskQueryUnavailable) {
		t.Fatalf("invalid sent projection err=%v", err)
	}

	notFound := &taskQueryStoreStub{err: ErrTaskNotFound}
	if _, err := NewTaskQueryService(inlineTaskQueryUOW{}, notFound).Get(context.Background(), TaskGetQuery{TaskID: 99}); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("not found err=%v", err)
	}
}

type inlineTaskQueryUOW struct{}

func (inlineTaskQueryUOW) Within(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type taskQueryStoreStub struct {
	get      TaskReadModel
	list     []TaskReadModel
	attempts []AttemptReadModel
	receipts []ControlReceiptReadModel
	err      error
	lastList TaskListQuery
}

func (stub *taskQueryStoreStub) GetTask(_ context.Context, query TaskGetQuery) (TaskReadModel, error) {
	if stub.err != nil {
		return TaskReadModel{}, stub.err
	}
	return cloneTaskReadModel(stub.get), nil
}

func (stub *taskQueryStoreStub) ListTasks(_ context.Context, query TaskListQuery) ([]TaskReadModel, error) {
	stub.lastList = cloneTaskListQuery(query)
	if stub.err != nil {
		return nil, stub.err
	}
	items := make([]TaskReadModel, len(stub.list))
	for index := range stub.list {
		items[index] = cloneTaskReadModel(stub.list[index])
	}
	return items, nil
}

func (stub *taskQueryStoreStub) ListAttempts(context.Context, TaskID) ([]AttemptReadModel, error) {
	if stub.err != nil {
		return nil, stub.err
	}
	return append([]AttemptReadModel(nil), stub.attempts...), nil
}

func (stub *taskQueryStoreStub) ListControlReceipts(context.Context, TaskID) ([]ControlReceiptReadModel, error) {
	if stub.err != nil {
		return nil, stub.err
	}
	return append([]ControlReceiptReadModel(nil), stub.receipts...), nil
}

func int64Pointer(value int64) *int64 { return &value }

func int32Pointer(value int32) *int32 { return &value }
