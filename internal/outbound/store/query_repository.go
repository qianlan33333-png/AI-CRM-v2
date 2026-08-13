package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outbounddb "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type TaskQueryRepository struct{}

var _ outboundapp.TaskQueryStore = (*TaskQueryRepository)(nil)

func NewTaskQueryRepository() *TaskQueryRepository { return &TaskQueryRepository{} }

func (*TaskQueryRepository) GetTask(ctx context.Context, query outboundapp.TaskGetQuery) (outboundapp.TaskReadModel, error) {
	queries, err := taskReadQueries(ctx)
	if err != nil {
		return outboundapp.TaskReadModel{}, err
	}
	row, err := queries.GetOutboundTaskReadModel(ctx, outbounddb.GetOutboundTaskReadModelParams{
		TaskID: int64(query.TaskID), OwnerStaffID: outboundNullableInt64(query.OwnerStaffID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return outboundapp.TaskReadModel{}, outboundapp.ErrTaskNotFound
	}
	if err != nil {
		return outboundapp.TaskReadModel{}, err
	}
	return outboundTaskReadModel(taskReadRow{
		id: row.ID, customerID: row.CustomerID, ownerStaffID: row.OwnerStaffID,
		batchID: row.BatchID, batchChunkIndex: row.BatchChunkIndex, status: row.Status,
		attemptCount: row.AttemptCount, currentAttemptID: row.CurrentAttemptID,
		lastFailureKind: row.LastFailureKind, lastError: row.LastError,
		providerMessageID: row.ProviderMessageID, createdAt: row.CreatedAt,
		statusUpdatedAt: row.StatusUpdatedAt, sentAt: row.SentAt, generation: row.Generation,
		riverJobID: row.RiverJobID, jobKind: row.JobKind,
	}), nil
}

func (*TaskQueryRepository) ListTasks(ctx context.Context, query outboundapp.TaskListQuery) ([]outboundapp.TaskReadModel, error) {
	queries, err := taskReadQueries(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListOutboundTaskReadModels(ctx, outbounddb.ListOutboundTaskReadModelsParams{
		BatchID: outboundNullableInt64(query.BatchID), TaskStatus: outboundNullableText(string(query.Status)),
		OwnerStaffID: outboundNullableInt64(query.OwnerStaffID), ResultLimit: query.Limit, ResultOffset: query.Offset,
	})
	if err != nil {
		return nil, err
	}
	result := make([]outboundapp.TaskReadModel, len(rows))
	for index, row := range rows {
		result[index] = outboundTaskReadModel(taskReadRow{
			id: row.ID, customerID: row.CustomerID, ownerStaffID: row.OwnerStaffID,
			batchID: row.BatchID, batchChunkIndex: row.BatchChunkIndex, status: row.Status,
			attemptCount: row.AttemptCount, currentAttemptID: row.CurrentAttemptID,
			lastFailureKind: row.LastFailureKind, lastError: row.LastError,
			providerMessageID: row.ProviderMessageID, createdAt: row.CreatedAt,
			statusUpdatedAt: row.StatusUpdatedAt, sentAt: row.SentAt, generation: row.Generation,
			riverJobID: row.RiverJobID, jobKind: row.JobKind,
		})
	}
	return result, nil
}

func (*TaskQueryRepository) ListAttempts(ctx context.Context, taskID outboundapp.TaskID) ([]outboundapp.AttemptReadModel, error) {
	queries, err := taskReadQueries(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListOutboundAttemptReadModels(ctx, int64(taskID))
	if err != nil {
		return nil, err
	}
	result := make([]outboundapp.AttemptReadModel, len(rows))
	for index, row := range rows {
		result[index] = outboundapp.AttemptReadModel{
			ID: row.ID, HistoryID: row.HistoryID, TaskID: outboundapp.TaskID(row.TaskID),
			Generation: row.Generation, RiverJobID: row.RiverJobID, RiverAttempt: row.RiverAttempt,
			RiverMaxAttempts: row.RiverMaxAttempts, State: outboundapp.SendAttemptState(row.State),
			FailureKind:  outboundapp.ProviderFailureKind(row.FailureKind.String),
			ProviderCode: row.ProviderCode.String, ProviderMessageID: row.ProviderMessageID.String,
			DispatchStartedAt: outboundTimePointer(row.DispatchStartedAt), CompletedAt: outboundTimePointer(row.CompletedAt),
		}
	}
	return result, nil
}

func (*TaskQueryRepository) ListControlReceipts(ctx context.Context, taskID outboundapp.TaskID) ([]outboundapp.ControlReceiptReadModel, error) {
	queries, err := taskReadQueries(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListOutboundControlReceiptReadModels(ctx, int64(taskID))
	if err != nil {
		return nil, err
	}
	result := make([]outboundapp.ControlReceiptReadModel, len(rows))
	for index, row := range rows {
		result[index] = outboundapp.ControlReceiptReadModel{
			ID: row.ID, TaskID: outboundapp.TaskID(row.TaskID), Operation: row.Operation,
			State: outboundapp.ControlReceiptState(row.State),
			Job: outboundapp.TaskJob{TaskID: outboundapp.TaskID(row.TaskID), Generation: row.JobGeneration.Int32,
				RiverJobID: row.RiverJobID.Int64, JobKind: row.JobKind.String},
			EventID: eventID(row.EventID), TaskStatus: outboundapp.TaskStatus(row.TaskStatus.String),
			CreatedAt: outboundTime(row.CreatedAt), CompletedAt: outboundTime(row.CompletedAt),
		}
	}
	return result, nil
}

type taskReadRow struct {
	id                int64
	customerID        int64
	ownerStaffID      pgtype.Int8
	batchID           pgtype.Int8
	batchChunkIndex   pgtype.Int4
	status            string
	attemptCount      int32
	currentAttemptID  pgtype.Int8
	lastFailureKind   pgtype.Text
	lastError         pgtype.Text
	providerMessageID pgtype.Text
	createdAt         pgtype.Timestamptz
	statusUpdatedAt   pgtype.Timestamptz
	sentAt            pgtype.Timestamptz
	generation        int32
	riverJobID        int64
	jobKind           string
}

func outboundTaskReadModel(row taskReadRow) outboundapp.TaskReadModel {
	return outboundapp.TaskReadModel{
		TaskID: outboundapp.TaskID(row.id), CustomerID: row.customerID,
		OwnerStaffID: outboundInt64Pointer(row.ownerStaffID), BatchID: outboundInt64Pointer(row.batchID),
		BatchChunkIndex: outboundInt32Pointer(row.batchChunkIndex), Status: outboundapp.TaskStatus(row.status),
		AttemptCount: row.attemptCount, CurrentAttemptID: outboundInt64Pointer(row.currentAttemptID),
		LastFailureKind: outboundapp.ProviderFailureKind(row.lastFailureKind.String), LastError: row.lastError.String,
		ProviderMessageID: row.providerMessageID.String,
		Job: outboundapp.TaskJob{TaskID: outboundapp.TaskID(row.id), Generation: row.generation,
			RiverJobID: row.riverJobID, JobKind: row.jobKind},
		CreatedAt: outboundTime(row.createdAt), StatusUpdatedAt: outboundTime(row.statusUpdatedAt),
		SentAt: outboundTimePointer(row.sentAt),
	}
}

func taskReadQueries(ctx context.Context) (*outbounddb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return outbounddb.New(tx), nil
}

func outboundNullableInt64(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func outboundNullableText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func outboundInt64Pointer(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	copy := value.Int64
	return &copy
}

func outboundInt32Pointer(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	copy := value.Int32
	return &copy
}

func outboundTime(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

func outboundTimePointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	copy := value.Time.UTC()
	return &copy
}

func eventID(value pgtype.Int8) eventport.EventID {
	if !value.Valid {
		return 0
	}
	return eventport.EventID(value.Int64)
}
