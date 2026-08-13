package jobqueue

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	queueriver "github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
)

const (
	outboundQueueName        = "outbound"
	outboundEnqueueOneKind   = "outbound_enqueue_one"
	outboundEnqueueBatchKind = "outbound_enqueue_batch_task"
	outboundJobListPageSize  = 10000
)

var (
	ErrOutboundTaskJobUnavailable = errors.New("outbound task River job unavailable")
	ErrOutboundTaskJobRunning     = errors.New("outbound task River job is running")
)

type OutboundTaskJob struct {
	ID   int64
	Kind string
}

type OutboundManualRetryTask struct {
	TaskID          int64
	JobKind         string
	ReceiptID       int64
	BatchID         int64
	BatchChunkIndex int32
}

type outboundManualRetryOneArgs struct {
	TaskID    int64 `json:"task_id"`
	ReceiptID int64 `json:"receipt_id"`
}

func (outboundManualRetryOneArgs) Kind() string { return outboundEnqueueOneKind }

type outboundManualRetryBatchArgs struct {
	BatchID    int64 `json:"batch_id"`
	ChunkIndex int32 `json:"chunk_index"`
	TaskID     int64 `json:"task_id"`
}

func (outboundManualRetryBatchArgs) Kind() string { return outboundEnqueueBatchKind }

// OutboundControlClient is the narrow platform-owned River catalog boundary
// needed by Outbound cancellation. It uses only River's typed transaction APIs.
type OutboundControlClient struct {
	client *queueriver.Client[pgx.Tx]
}

func NewOutboundControlClient(pool *pgxpool.Pool) (*OutboundControlClient, error) {
	if pool == nil {
		return nil, ErrClientUnavailable
	}
	client, err := queueriver.NewClient(riverpgxv5.New(pool), &queueriver.Config{SkipUnknownJobCheck: true})
	if err != nil {
		return nil, err
	}
	return &OutboundControlClient{client: client}, nil
}

// DeletePendingTaskTx deletes exactly one queued Outbound job in the caller's
// transaction. linkedJobID is the normal path. A zero link activates a bounded
// compatibility scan of official typed JobListTx results for pre-link jobs.
func (client *OutboundControlClient) DeletePendingTaskTx(
	ctx context.Context,
	tx pgx.Tx,
	taskID int64,
	linkedJobID int64,
	linkedJobKind string,
) (OutboundTaskJob, error) {
	if client == nil || client.client == nil || tx == nil || taskID <= 0 ||
		(linkedJobID > 0) != (linkedJobKind != "") ||
		(linkedJobKind != "" && !isOutboundJobKind(linkedJobKind)) {
		return OutboundTaskJob{}, ErrOutboundTaskJobUnavailable
	}

	var job *rivertype.JobRow
	var err error
	if linkedJobID > 0 {
		job, err = client.client.JobGetTx(ctx, tx, linkedJobID)
		if err != nil || !matchesPendingOutboundTask(job, taskID, linkedJobKind) {
			return OutboundTaskJob{}, classifyOutboundJobError(err, job)
		}
	} else {
		job, err = client.findPendingTaskJobTx(ctx, tx, taskID)
		if err != nil {
			return OutboundTaskJob{}, err
		}
	}

	deleted, err := client.client.JobDeleteTx(ctx, tx, job.ID)
	if err != nil || deleted == nil || deleted.ID != job.ID || deleted.Kind != job.Kind ||
		deleted.Queue != outboundQueueName || outboundTaskID(deleted) != taskID {
		return OutboundTaskJob{}, classifyOutboundJobError(err, deleted)
	}
	return OutboundTaskJob{ID: deleted.ID, Kind: deleted.Kind}, nil
}

// InsertManualRetryTaskTx inserts only the two frozen Outbound job shapes.
func (client *OutboundControlClient) InsertManualRetryTaskTx(
	ctx context.Context,
	tx pgx.Tx,
	task OutboundManualRetryTask,
) (OutboundTaskJob, error) {
	if client == nil || client.client == nil || tx == nil || task.TaskID <= 0 {
		return OutboundTaskJob{}, ErrOutboundTaskJobUnavailable
	}
	var args queueriver.JobArgs
	switch task.JobKind {
	case outboundEnqueueOneKind:
		if task.ReceiptID <= 0 || task.BatchID != 0 || task.BatchChunkIndex != 0 {
			return OutboundTaskJob{}, ErrOutboundTaskJobUnavailable
		}
		args = outboundManualRetryOneArgs{TaskID: task.TaskID, ReceiptID: task.ReceiptID}
	case outboundEnqueueBatchKind:
		if task.BatchID <= 0 || task.BatchChunkIndex < 0 || task.ReceiptID != 0 {
			return OutboundTaskJob{}, ErrOutboundTaskJobUnavailable
		}
		args = outboundManualRetryBatchArgs{TaskID: task.TaskID, BatchID: task.BatchID, ChunkIndex: task.BatchChunkIndex}
	default:
		return OutboundTaskJob{}, ErrOutboundTaskJobUnavailable
	}
	inserted, err := client.client.InsertTx(ctx, tx, args, &queueriver.InsertOpts{Queue: outboundQueueName})
	if err != nil || inserted == nil || inserted.Job == nil || inserted.Job.ID <= 0 ||
		inserted.Job.Queue != outboundQueueName || inserted.Job.Kind != task.JobKind || outboundTaskID(inserted.Job) != task.TaskID {
		return OutboundTaskJob{}, errors.Join(ErrOutboundTaskJobUnavailable, err)
	}
	return OutboundTaskJob{ID: inserted.Job.ID, Kind: inserted.Job.Kind}, nil
}

func (client *OutboundControlClient) findPendingTaskJobTx(ctx context.Context, tx pgx.Tx, taskID int64) (*rivertype.JobRow, error) {
	params := queueriver.NewJobListParams().
		First(outboundJobListPageSize).
		Queues(outboundQueueName).
		Kinds(outboundEnqueueOneKind, outboundEnqueueBatchKind).
		States(
			rivertype.JobStateAvailable,
			rivertype.JobStatePending,
			rivertype.JobStateRetryable,
			rivertype.JobStateScheduled,
		)
	var match *rivertype.JobRow
	for {
		result, err := client.client.JobListTx(ctx, tx, params)
		if err != nil || result == nil {
			return nil, errors.Join(ErrOutboundTaskJobUnavailable, err)
		}
		for _, candidate := range result.Jobs {
			if outboundTaskID(candidate) != taskID {
				continue
			}
			if match != nil {
				return nil, ErrOutboundTaskJobUnavailable
			}
			match = candidate
		}
		if len(result.Jobs) < outboundJobListPageSize {
			break
		}
		if result.LastCursor == nil {
			return nil, ErrOutboundTaskJobUnavailable
		}
		params = params.After(result.LastCursor)
	}
	if match == nil {
		return nil, ErrOutboundTaskJobUnavailable
	}
	return match, nil
}

func matchesPendingOutboundTask(job *rivertype.JobRow, taskID int64, kind string) bool {
	return job != nil && job.ID > 0 && job.Queue == outboundQueueName && job.Kind == kind &&
		isOutboundJobKind(job.Kind) && isPendingRiverState(job.State) && outboundTaskID(job) == taskID
}

func outboundTaskID(job *rivertype.JobRow) int64 {
	if job == nil || len(job.EncodedArgs) == 0 {
		return 0
	}
	var args struct {
		TaskID int64 `json:"task_id"`
	}
	if json.Unmarshal(job.EncodedArgs, &args) != nil || args.TaskID <= 0 {
		return 0
	}
	return args.TaskID
}

func isOutboundJobKind(kind string) bool {
	return kind == outboundEnqueueOneKind || kind == outboundEnqueueBatchKind
}

func isPendingRiverState(state rivertype.JobState) bool {
	switch state {
	case rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable, rivertype.JobStateScheduled:
		return true
	default:
		return false
	}
}

func classifyOutboundJobError(err error, job *rivertype.JobRow) error {
	if errors.Is(err, rivertype.ErrJobRunning) || job != nil && job.State == rivertype.JobStateRunning {
		return errors.Join(ErrOutboundTaskJobRunning, err)
	}
	return errors.Join(ErrOutboundTaskJobUnavailable, err)
}
