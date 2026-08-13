package app

import (
	"context"
	"errors"
	"reflect"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const TaskQueryMaximumLimit int32 = 100

var (
	ErrInvalidTaskQuery     = errors.New("invalid outbound task query")
	ErrTaskNotFound         = errors.New("outbound task not found")
	ErrTaskQueryUnavailable = errors.New("outbound task query unavailable")
)

// TaskReadModel exposes only delivery/control facts. Message payloads and
// recipient identifiers remain private to the Outbound storage boundary.
type TaskReadModel struct {
	TaskID            TaskID
	CustomerID        int64
	OwnerStaffID      *int64
	BatchID           *int64
	BatchChunkIndex   *int32
	Status            TaskStatus
	AttemptCount      int32
	CurrentAttemptID  *int64
	LastFailureKind   ProviderFailureKind
	LastError         string
	ProviderMessageID string
	Job               TaskJob
	CreatedAt         time.Time
	StatusUpdatedAt   time.Time
	SentAt            *time.Time
}

type AttemptReadModel struct {
	ID                int64
	HistoryID         int64
	TaskID            TaskID
	Generation        int32
	RiverJobID        int64
	RiverAttempt      int32
	RiverMaxAttempts  int32
	State             SendAttemptState
	FailureKind       ProviderFailureKind
	ProviderCode      string
	ProviderMessageID string
	DispatchStartedAt *time.Time
	CompletedAt       *time.Time
}

type ControlReceiptReadModel struct {
	ID          int64
	TaskID      TaskID
	Operation   string
	State       ControlReceiptState
	Job         TaskJob
	EventID     eventport.EventID
	TaskStatus  TaskStatus
	CreatedAt   time.Time
	CompletedAt time.Time
}

type TaskGetQuery struct {
	TaskID       TaskID
	OwnerStaffID *int64
}

type TaskListQuery struct {
	BatchID      *int64
	Status       TaskStatus
	OwnerStaffID *int64
	Limit        int32
	Offset       int32
}

type TaskListResult struct {
	Items   []TaskReadModel
	HasMore bool
}

type TaskReconciliation struct {
	Task            TaskReadModel
	Attempts        []AttemptReadModel
	ControlReceipts []ControlReceiptReadModel
}

type TaskQueryStore interface {
	GetTask(context.Context, TaskGetQuery) (TaskReadModel, error)
	ListTasks(context.Context, TaskListQuery) ([]TaskReadModel, error)
	ListAttempts(context.Context, TaskID) ([]AttemptReadModel, error)
	ListControlReceipts(context.Context, TaskID) ([]ControlReceiptReadModel, error)
}

type TaskQueryService struct {
	uow   platformport.UnitOfWork
	store TaskQueryStore
}

func NewTaskQueryService(uow platformport.UnitOfWork, store TaskQueryStore) *TaskQueryService {
	return &TaskQueryService{uow: uow, store: store}
}

func (service *TaskQueryService) Get(ctx context.Context, query TaskGetQuery) (TaskReadModel, error) {
	if err := validateTaskGetQuery(ctx, query); err != nil {
		return TaskReadModel{}, err
	}
	if service == nil || nilTaskQueryDependency(service.uow) || nilTaskQueryDependency(service.store) {
		return TaskReadModel{}, ErrTaskQueryUnavailable
	}
	var result TaskReadModel
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var storeErr error
		result, storeErr = service.store.GetTask(txCtx, cloneTaskGetQuery(query))
		return storeErr
	})
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return TaskReadModel{}, ErrTaskNotFound
		}
		return TaskReadModel{}, errors.Join(ErrTaskQueryUnavailable, err)
	}
	if err = validateTaskReadModel(result, query.TaskID, query.OwnerStaffID); err != nil {
		return TaskReadModel{}, errors.Join(ErrTaskQueryUnavailable, err)
	}
	return cloneTaskReadModel(result), nil
}

func (service *TaskQueryService) List(ctx context.Context, query TaskListQuery) (TaskListResult, error) {
	if err := validateTaskListQuery(ctx, query); err != nil {
		return TaskListResult{}, err
	}
	if service == nil || nilTaskQueryDependency(service.uow) || nilTaskQueryDependency(service.store) {
		return TaskListResult{}, ErrTaskQueryUnavailable
	}
	storeQuery := cloneTaskListQuery(query)
	storeQuery.Limit++
	var stored []TaskReadModel
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var storeErr error
		stored, storeErr = service.store.ListTasks(txCtx, storeQuery)
		return storeErr
	})
	if err != nil {
		return TaskListResult{}, errors.Join(ErrTaskQueryUnavailable, err)
	}
	if len(stored) > int(storeQuery.Limit) {
		return TaskListResult{}, errors.Join(ErrTaskQueryUnavailable, errors.New("outbound task store exceeded its limit"))
	}
	for _, item := range stored {
		if err = validateTaskReadModel(item, 0, query.OwnerStaffID); err != nil ||
			(query.BatchID != nil && (item.BatchID == nil || *item.BatchID != *query.BatchID)) ||
			(query.Status != "" && item.Status != query.Status) {
			return TaskListResult{}, errors.Join(ErrTaskQueryUnavailable, err)
		}
	}
	result := TaskListResult{HasMore: len(stored) > int(query.Limit)}
	if result.HasMore {
		stored = stored[:query.Limit]
	}
	result.Items = make([]TaskReadModel, len(stored))
	for index := range stored {
		result.Items[index] = cloneTaskReadModel(stored[index])
	}
	return result, nil
}

func (service *TaskQueryService) Reconcile(ctx context.Context, query TaskGetQuery) (TaskReconciliation, error) {
	if err := validateTaskGetQuery(ctx, query); err != nil {
		return TaskReconciliation{}, err
	}
	if service == nil || nilTaskQueryDependency(service.uow) || nilTaskQueryDependency(service.store) {
		return TaskReconciliation{}, ErrTaskQueryUnavailable
	}
	var result TaskReconciliation
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var storeErr error
		result.Task, storeErr = service.store.GetTask(txCtx, cloneTaskGetQuery(query))
		if storeErr != nil {
			return storeErr
		}
		result.Attempts, storeErr = service.store.ListAttempts(txCtx, query.TaskID)
		if storeErr != nil {
			return storeErr
		}
		result.ControlReceipts, storeErr = service.store.ListControlReceipts(txCtx, query.TaskID)
		return storeErr
	})
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return TaskReconciliation{}, ErrTaskNotFound
		}
		return TaskReconciliation{}, errors.Join(ErrTaskQueryUnavailable, err)
	}
	if err = validateTaskReadModel(result.Task, query.TaskID, query.OwnerStaffID); err != nil {
		return TaskReconciliation{}, errors.Join(ErrTaskQueryUnavailable, err)
	}
	for _, attempt := range result.Attempts {
		if !validAttemptReadModel(attempt, query.TaskID) {
			return TaskReconciliation{}, errors.Join(ErrTaskQueryUnavailable, errors.New("invalid outbound attempt read model"))
		}
	}
	for _, receipt := range result.ControlReceipts {
		if !validControlReceiptReadModel(receipt, query.TaskID) {
			return TaskReconciliation{}, errors.Join(ErrTaskQueryUnavailable, errors.New("invalid outbound control receipt read model"))
		}
	}
	return cloneTaskReconciliation(result), nil
}

func validateTaskGetQuery(ctx context.Context, query TaskGetQuery) error {
	if ctx == nil || ctx.Err() != nil || query.TaskID <= 0 || invalidPositivePointer(query.OwnerStaffID) {
		return ErrInvalidTaskQuery
	}
	return nil
}

func validateTaskListQuery(ctx context.Context, query TaskListQuery) error {
	if ctx == nil || ctx.Err() != nil || query.Limit < 1 || query.Limit > TaskQueryMaximumLimit || query.Offset < 0 ||
		invalidPositivePointer(query.BatchID) || invalidPositivePointer(query.OwnerStaffID) ||
		(query.Status != "" && !validTaskStatus(query.Status)) {
		return ErrInvalidTaskQuery
	}
	return nil
}

func validateTaskReadModel(model TaskReadModel, expected TaskID, owner *int64) error {
	if model.TaskID <= 0 || (expected > 0 && model.TaskID != expected) || model.CustomerID <= 0 ||
		invalidPositivePointer(model.OwnerStaffID) || invalidPositivePointer(model.BatchID) ||
		(model.BatchID == nil) != (model.BatchChunkIndex == nil) ||
		(model.BatchChunkIndex != nil && *model.BatchChunkIndex < 0) || !validTaskStatus(model.Status) ||
		model.AttemptCount < 0 || !validTaskJob(model.Job, model.TaskID) || model.CreatedAt.IsZero() ||
		model.StatusUpdatedAt.IsZero() || model.StatusUpdatedAt.Before(model.CreatedAt) ||
		(owner != nil && (model.OwnerStaffID == nil || *model.OwnerStaffID != *owner)) {
		return errors.New("invalid outbound task read model")
	}
	if model.CurrentAttemptID != nil && *model.CurrentAttemptID <= 0 {
		return errors.New("invalid outbound current attempt")
	}
	switch model.Status {
	case TaskStatusPending, TaskStatusCancelled:
		if model.AttemptCount != 0 || model.CurrentAttemptID != nil || model.LastFailureKind != "" ||
			model.LastError != "" || model.ProviderMessageID != "" || model.SentAt != nil {
			return errors.New("invalid outbound pending or cancelled task")
		}
	case TaskStatusSending:
		if model.AttemptCount < 1 || model.CurrentAttemptID == nil || model.LastFailureKind != "" ||
			model.LastError != "" || model.ProviderMessageID != "" || model.SentAt != nil {
			return errors.New("invalid outbound sending task")
		}
	case TaskStatusSent:
		if model.AttemptCount < 1 || model.CurrentAttemptID == nil || model.LastFailureKind != "" ||
			model.LastError != "" || model.ProviderMessageID == "" || model.SentAt == nil || model.SentAt.IsZero() {
			return errors.New("invalid outbound sent task")
		}
	case TaskStatusRetryableFailed, TaskStatusFinalFailed, TaskStatusOutcomeUnknown:
		if model.AttemptCount < 1 || model.CurrentAttemptID == nil || model.LastFailureKind == "" ||
			model.LastError == "" || model.ProviderMessageID != "" || model.SentAt != nil {
			return errors.New("invalid outbound failed task")
		}
	}
	return nil
}

func validAttemptReadModel(model AttemptReadModel, taskID TaskID) bool {
	if model.ID <= 0 || model.HistoryID <= 0 || model.TaskID != taskID || model.Generation <= 0 ||
		model.RiverJobID <= 0 || model.RiverAttempt <= 0 || model.RiverMaxAttempts < model.RiverAttempt {
		return false
	}
	switch model.State {
	case SendAttemptReserved:
		return model.DispatchStartedAt == nil && model.CompletedAt == nil && model.FailureKind == "" &&
			model.ProviderCode == "" && model.ProviderMessageID == ""
	case SendAttemptDispatching:
		return model.DispatchStartedAt != nil && !model.DispatchStartedAt.IsZero() && model.CompletedAt == nil
	case SendAttemptSucceeded:
		return model.CompletedAt != nil && !model.CompletedAt.IsZero() && model.FailureKind == "" &&
			model.ProviderCode == "" && model.ProviderMessageID != ""
	case SendAttemptRetryableFailed, SendAttemptFinalFailed, SendAttemptOutcomeUnknown:
		return model.CompletedAt != nil && !model.CompletedAt.IsZero() && model.FailureKind != "" &&
			model.ProviderCode != "" && model.ProviderMessageID == ""
	default:
		return false
	}
}

func validControlReceiptReadModel(model ControlReceiptReadModel, taskID TaskID) bool {
	return model.ID > 0 && model.TaskID == taskID &&
		(model.Operation == "cancel" || model.Operation == "manual_retry") &&
		model.State == ControlReceiptCompleted && validTaskJob(model.Job, taskID) && model.EventID > 0 &&
		((model.Operation == "cancel" && model.TaskStatus == TaskStatusCancelled) ||
			(model.Operation == "manual_retry" && model.TaskStatus == TaskStatusPending)) &&
		!model.CreatedAt.IsZero() && !model.CompletedAt.IsZero() && !model.CompletedAt.Before(model.CreatedAt)
}

func validTaskStatus(status TaskStatus) bool {
	switch status {
	case TaskStatusPending, TaskStatusSending, TaskStatusSent, TaskStatusRetryableFailed,
		TaskStatusFinalFailed, TaskStatusOutcomeUnknown, TaskStatusCancelled:
		return true
	default:
		return false
	}
}

func invalidPositivePointer(value *int64) bool { return value != nil && *value <= 0 }

func cloneTaskGetQuery(query TaskGetQuery) TaskGetQuery {
	query.OwnerStaffID = cloneTaskQueryInt64(query.OwnerStaffID)
	return query
}

func cloneTaskListQuery(query TaskListQuery) TaskListQuery {
	query.BatchID = cloneTaskQueryInt64(query.BatchID)
	query.OwnerStaffID = cloneTaskQueryInt64(query.OwnerStaffID)
	return query
}

func cloneTaskReadModel(model TaskReadModel) TaskReadModel {
	model.OwnerStaffID = cloneTaskQueryInt64(model.OwnerStaffID)
	model.BatchID = cloneTaskQueryInt64(model.BatchID)
	model.BatchChunkIndex = cloneTaskQueryInt32(model.BatchChunkIndex)
	model.CurrentAttemptID = cloneTaskQueryInt64(model.CurrentAttemptID)
	model.SentAt = cloneTaskQueryTime(model.SentAt)
	return model
}

func cloneTaskReconciliation(result TaskReconciliation) TaskReconciliation {
	result.Task = cloneTaskReadModel(result.Task)
	result.Attempts = append([]AttemptReadModel(nil), result.Attempts...)
	for index := range result.Attempts {
		result.Attempts[index].DispatchStartedAt = cloneTaskQueryTime(result.Attempts[index].DispatchStartedAt)
		result.Attempts[index].CompletedAt = cloneTaskQueryTime(result.Attempts[index].CompletedAt)
	}
	result.ControlReceipts = append([]ControlReceiptReadModel(nil), result.ControlReceipts...)
	return result
}

func cloneTaskQueryInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTaskQueryInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTaskQueryTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func nilTaskQueryDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Pointer && reflected.IsNil()
}
