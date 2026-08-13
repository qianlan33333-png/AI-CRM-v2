package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outbounddb "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store/generated"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var errTaskJobLink = errors.New("record outbound task River job link")

type ControlRepository struct {
	client *platformjobqueue.OutboundControlClient
}

var _ outboundapp.CancelRepository = (*ControlRepository)(nil)

func NewControlRepository(pool *pgxpool.Pool) (*ControlRepository, error) {
	if pool == nil {
		return nil, outboundapp.ErrCancelFailed
	}
	client, err := platformjobqueue.NewOutboundControlClient(pool)
	if err != nil {
		return nil, errors.Join(outboundapp.ErrCancelFailed, err)
	}
	return &ControlRepository{client: client}, nil
}

func (repository *ControlRepository) ReserveCancelReceipt(ctx context.Context, command outboundapp.CancelCommand) (outboundapp.CancelReceipt, error) {
	queries, err := controlQueries(ctx)
	if repository == nil || repository.client == nil || command.TaskID <= 0 || err != nil {
		return outboundapp.CancelReceipt{}, errors.Join(outboundapp.ErrCancelFailed, err)
	}
	row, err := queries.ReserveOutboundCancelReceipt(ctx, outbounddb.ReserveOutboundCancelReceiptParams{
		IdempotencyScope: command.IdempotencyScope,
		IdempotencyKey:   command.IdempotencyKey,
		TaskID:           int64(command.TaskID),
	})
	if err != nil {
		return outboundapp.CancelReceipt{}, errors.Join(outboundapp.ErrCancelFailed, err)
	}
	return storedCancelReceipt(
		row.ID, row.IdempotencyScope, row.IdempotencyKey, row.Operation, row.TaskID, row.State,
		row.CustomerID, row.JobGeneration, row.RiverJobID, row.JobKind, row.EventID, row.TaskStatus, row.CompletedAt,
	)
}

func (repository *ControlRepository) LockCancelTarget(ctx context.Context, taskID outboundapp.TaskID) (outboundapp.CancelTarget, error) {
	queries, err := controlQueries(ctx)
	if repository == nil || repository.client == nil || taskID <= 0 || err != nil {
		return outboundapp.CancelTarget{}, errors.Join(outboundapp.ErrCancelFailed, err)
	}
	task, err := queries.LockOutboundTaskForCancel(ctx, int64(taskID))
	if errors.Is(err, pgx.ErrNoRows) {
		return outboundapp.CancelTarget{}, outboundapp.ErrCancelTaskNotFound
	}
	if err != nil || task.ID != int64(taskID) || task.CustomerID <= 0 {
		return outboundapp.CancelTarget{}, errors.Join(outboundapp.ErrCancelFailed, err)
	}
	target := outboundapp.CancelTarget{TaskID: taskID, CustomerID: task.CustomerID, Status: outboundapp.TaskStatus(task.Status)}
	link, err := queries.LoadLatestOutboundTaskJobLink(ctx, int64(taskID))
	if errors.Is(err, pgx.ErrNoRows) {
		return target, nil
	}
	if err != nil || link.TaskID != int64(taskID) || link.Generation <= 0 || link.RiverJobID <= 0 || link.JobKind == "" {
		return outboundapp.CancelTarget{}, errors.Join(outboundapp.ErrCancelFailed, err)
	}
	target.Job = outboundapp.TaskJob{
		TaskID: taskID, Generation: link.Generation, RiverJobID: link.RiverJobID, JobKind: link.JobKind,
	}
	return target, nil
}

func (repository *ControlRepository) DeletePendingTaskJob(ctx context.Context, target outboundapp.CancelTarget) (outboundapp.TaskJob, error) {
	if repository == nil || repository.client == nil || target.TaskID <= 0 || target.Status != outboundapp.TaskStatusPending {
		return outboundapp.TaskJob{}, outboundapp.ErrCancelFailed
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return outboundapp.TaskJob{}, errors.Join(outboundapp.ErrCancelFailed, err)
	}
	deleted, err := repository.client.DeletePendingTaskTx(
		ctx, tx, int64(target.TaskID), target.Job.RiverJobID, target.Job.JobKind,
	)
	if errors.Is(err, platformjobqueue.ErrOutboundTaskJobRunning) ||
		errors.Is(err, platformjobqueue.ErrOutboundTaskJobUnavailable) {
		return outboundapp.TaskJob{}, outboundapp.ErrCancelWorkerWon
	}
	if err != nil {
		return outboundapp.TaskJob{}, errors.Join(outboundapp.ErrCancelFailed, err)
	}
	generation := target.Job.Generation
	if generation == 0 {
		generation = 1
	}
	job := outboundapp.TaskJob{TaskID: target.TaskID, Generation: generation, RiverJobID: deleted.ID, JobKind: deleted.Kind}
	if job.RiverJobID <= 0 || job.JobKind == "" ||
		(target.Job.RiverJobID > 0 && (job.RiverJobID != target.Job.RiverJobID || job.JobKind != target.Job.JobKind)) {
		return outboundapp.TaskJob{}, outboundapp.ErrCancelFailed
	}
	return job, nil
}

func (repository *ControlRepository) CompleteCancel(
	ctx context.Context,
	receiptID int64,
	target outboundapp.CancelTarget,
	job outboundapp.TaskJob,
	eventID eventport.EventID,
) (outboundapp.CancelReceipt, error) {
	queries, err := controlQueries(ctx)
	if repository == nil || repository.client == nil || receiptID <= 0 || target.TaskID <= 0 ||
		job.TaskID != target.TaskID || job.Generation <= 0 || job.RiverJobID <= 0 || job.JobKind == "" || eventID <= 0 || err != nil {
		return outboundapp.CancelReceipt{}, errors.Join(outboundapp.ErrCancelFailed, err)
	}
	linked, err := recordTaskJobLink(ctx, job)
	if err != nil || linked != job {
		return outboundapp.CancelReceipt{}, errors.Join(outboundapp.ErrCancelFailed, err)
	}
	cancelledLink, err := queries.MarkOutboundTaskJobCancelled(ctx, outbounddb.MarkOutboundTaskJobCancelledParams{
		TaskID: int64(job.TaskID), Generation: job.Generation, RiverJobID: job.RiverJobID, JobKind: job.JobKind,
	})
	if err != nil || cancelledLink.TaskID != int64(job.TaskID) || cancelledLink.Generation != job.Generation ||
		cancelledLink.RiverJobID != job.RiverJobID || cancelledLink.JobKind != job.JobKind || !cancelledLink.CancelledAt.Valid {
		return outboundapp.CancelReceipt{}, errors.Join(outboundapp.ErrCancelFailed, err)
	}
	cancelledTask, err := queries.MarkOutboundTaskCancelled(ctx, int64(target.TaskID))
	if err != nil || cancelledTask.ID != int64(target.TaskID) || cancelledTask.CustomerID != target.CustomerID ||
		cancelledTask.Status != string(outboundapp.TaskStatusCancelled) || !cancelledTask.StatusUpdatedAt.Valid {
		return outboundapp.CancelReceipt{}, errors.Join(outboundapp.ErrCancelFailed, err)
	}
	row, err := queries.CompleteOutboundCancelReceipt(ctx, outbounddb.CompleteOutboundCancelReceiptParams{
		CustomerID: target.CustomerID, JobGeneration: job.Generation, RiverJobID: job.RiverJobID,
		JobKind: job.JobKind, EventID: int64(eventID),
		ID: receiptID, TaskID: int64(target.TaskID),
	})
	if err != nil {
		return outboundapp.CancelReceipt{}, errors.Join(outboundapp.ErrCancelFailed, err)
	}
	return storedCancelReceipt(
		row.ID, row.IdempotencyScope, row.IdempotencyKey, row.Operation, row.TaskID, row.State,
		row.CustomerID, row.JobGeneration, row.RiverJobID, row.JobKind, row.EventID, row.TaskStatus, row.CompletedAt,
	)
}

func recordTaskJobLink(ctx context.Context, job outboundapp.TaskJob) (outboundapp.TaskJob, error) {
	queries, err := controlQueries(ctx)
	if err != nil || job.TaskID <= 0 || job.Generation <= 0 || job.RiverJobID <= 0 || job.JobKind == "" {
		return outboundapp.TaskJob{}, errors.Join(errTaskJobLink, err)
	}
	row, err := queries.RecordOutboundTaskJobLink(ctx, outbounddb.RecordOutboundTaskJobLinkParams{
		TaskID: int64(job.TaskID), Generation: job.Generation, RiverJobID: job.RiverJobID, JobKind: job.JobKind,
	})
	if err != nil || row.TaskID != int64(job.TaskID) || row.Generation != job.Generation ||
		row.RiverJobID != job.RiverJobID || row.JobKind != job.JobKind || row.CancelledAt.Valid {
		return outboundapp.TaskJob{}, errors.Join(errTaskJobLink, err)
	}
	return outboundapp.TaskJob{TaskID: outboundapp.TaskID(row.TaskID), Generation: row.Generation, RiverJobID: row.RiverJobID, JobKind: row.JobKind}, nil
}

func controlQueries(ctx context.Context) (*outbounddb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return outbounddb.New(tx), nil
}

func storedCancelReceipt(
	id int64,
	scope, key, operation string,
	taskID int64,
	state string,
	customerID pgtype.Int8,
	jobGeneration pgtype.Int4,
	riverJobID pgtype.Int8,
	jobKind pgtype.Text,
	eventID pgtype.Int8,
	taskStatus pgtype.Text,
	completedAt pgtype.Timestamptz,
) (outboundapp.CancelReceipt, error) {
	if id <= 0 || operation != "cancel" || taskID <= 0 {
		return outboundapp.CancelReceipt{}, outboundapp.ErrCancelFailed
	}
	receipt := outboundapp.CancelReceipt{
		ID: id,
		Command: outboundapp.CancelCommand{
			TaskID: outboundapp.TaskID(taskID), IdempotencyScope: scope, IdempotencyKey: key,
		},
		State: outboundapp.ControlReceiptState(state),
	}
	if customerID.Valid {
		receipt.CustomerID = customerID.Int64
	}
	if jobGeneration.Valid {
		receipt.Job.Generation = jobGeneration.Int32
	}
	if riverJobID.Valid {
		receipt.Job.RiverJobID = riverJobID.Int64
	}
	if jobGeneration.Valid || riverJobID.Valid {
		receipt.Job.TaskID = outboundapp.TaskID(taskID)
	}
	if jobKind.Valid {
		receipt.Job.JobKind = jobKind.String
	}
	if eventID.Valid {
		receipt.EventID = eventport.EventID(eventID.Int64)
	}
	if taskStatus.Valid {
		receipt.TaskStatus = outboundapp.TaskStatus(taskStatus.String)
	}
	if completedAt.Valid {
		receipt.CompletedAt = completedAt.Time
	}
	return receipt, nil
}
