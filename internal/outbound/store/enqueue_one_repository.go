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

// EnqueueOneRepository owns the receipt that binds an outbound command to its
// accepted task, immutable event fact, and insert-only River job.
type EnqueueOneRepository struct {
	client *platformjobqueue.InsertOnlyClient
}

var (
	_ outboundapp.EnqueueReceiptStore = (*EnqueueOneRepository)(nil)
	_ outboundapp.EnqueueOneEnqueuer  = (*EnqueueOneRepository)(nil)
)

func NewEnqueueOneRepository(pool *pgxpool.Pool) (*EnqueueOneRepository, error) {
	if pool == nil {
		return nil, outboundapp.ErrEnqueueOneFailed
	}
	client, err := platformjobqueue.NewInsertOnlyClient(pool)
	if err != nil {
		return nil, errors.Join(outboundapp.ErrEnqueueOneFailed, err)
	}
	return &EnqueueOneRepository{client: client}, nil
}

func (repository *EnqueueOneRepository) ReserveEnqueueReceipt(ctx context.Context, command outboundapp.EnqueueOneCommand) (outboundapp.EnqueueReceipt, error) {
	if repository == nil || !validStoredCommand(command) {
		return outboundapp.EnqueueReceipt{}, outboundapp.ErrInvalidEnqueueOneCommand
	}
	queries, err := enqueueOneQueries(ctx)
	if err != nil {
		return outboundapp.EnqueueReceipt{}, err
	}
	receipt, err := queries.ReserveOutboundEnqueueReceipt(ctx, outbounddb.ReserveOutboundEnqueueReceiptParams{
		IdempotencyScope: command.IdempotencyScope,
		IdempotencyKey:   command.IdempotencyKey,
		CustomerID:       command.CustomerID,
		TemplateKey:      command.TemplateKey,
		Payload:          command.Payload,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return outboundapp.EnqueueReceipt{}, outboundapp.ErrInvalidEnqueueOneCommand
	}
	if err != nil {
		return outboundapp.EnqueueReceipt{}, err
	}
	return enqueueReceipt(receipt.ID, receipt.IdempotencyScope, receipt.IdempotencyKey, receipt.CustomerID, receipt.TemplateKey, receipt.Payload, receipt.State, receipt.TaskID, receipt.EventID, receipt.RiverJobID)
}

func (repository *EnqueueOneRepository) AcceptEnqueueReceipt(ctx context.Context, receiptID int64, taskID outboundapp.TaskID, eventID eventport.EventID, riverJobID int64) (outboundapp.EnqueueReceipt, error) {
	if repository == nil || receiptID <= 0 || taskID <= 0 || eventID <= 0 || riverJobID <= 0 {
		return outboundapp.EnqueueReceipt{}, outboundapp.ErrEnqueueOneFailed
	}
	queries, err := enqueueOneQueries(ctx)
	if err != nil {
		return outboundapp.EnqueueReceipt{}, err
	}
	receipt, err := queries.AcceptOutboundEnqueueReceipt(ctx, outbounddb.AcceptOutboundEnqueueReceiptParams{
		TaskID:     int64(taskID),
		EventID:    int64(eventID),
		RiverJobID: riverJobID,
		ID:         receiptID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return outboundapp.EnqueueReceipt{}, outboundapp.ErrEnqueueOneFailed
	}
	if err != nil {
		return outboundapp.EnqueueReceipt{}, err
	}
	return enqueueReceipt(receipt.ID, receipt.IdempotencyScope, receipt.IdempotencyKey, receipt.CustomerID, receipt.TemplateKey, receipt.Payload, receipt.State, receipt.TaskID, receipt.EventID, receipt.RiverJobID)
}

func (repository *EnqueueOneRepository) EnqueueOne(ctx context.Context, args outboundapp.EnqueueOneArgs) (int64, error) {
	if repository == nil || repository.client == nil || args.TaskID <= 0 || args.ReceiptID <= 0 {
		return 0, outboundapp.ErrEnqueueOneFailed
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	jobID, err := repository.client.InsertTx(ctx, tx, args, "outbound")
	if err != nil || jobID <= 0 {
		return 0, errors.Join(outboundapp.ErrEnqueueOneFailed, err)
	}
	return jobID, nil
}

func enqueueOneQueries(ctx context.Context) (*outbounddb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return outbounddb.New(tx), nil
}

func validStoredCommand(command outboundapp.EnqueueOneCommand) bool {
	return command.CustomerID > 0 && command.TemplateKey == outboundapp.TemplateTextNoticeV1 &&
		command.IdempotencyScope != "" && command.IdempotencyKey != ""
}

func enqueueReceipt(
	id int64,
	scope string,
	key string,
	customerID int64,
	templateKey string,
	payload []byte,
	state string,
	taskID pgtype.Int8,
	eventID pgtype.Int8,
	riverJobID pgtype.Int8,
) (outboundapp.EnqueueReceipt, error) {
	if id <= 0 || customerID <= 0 {
		return outboundapp.EnqueueReceipt{}, outboundapp.ErrEnqueueOneFailed
	}
	receipt := outboundapp.EnqueueReceipt{
		ID: id,
		Command: outboundapp.EnqueueOneCommand{
			OneCommand:       outboundapp.OneCommand{CustomerID: customerID, TemplateKey: templateKey, Payload: payload},
			IdempotencyScope: scope,
			IdempotencyKey:   key,
		},
		State: outboundapp.EnqueueReceiptState(state),
	}
	if taskID.Valid {
		receipt.TaskID = outboundapp.TaskID(taskID.Int64)
	}
	if eventID.Valid {
		receipt.EventID = eventport.EventID(eventID.Int64)
	}
	if riverJobID.Valid {
		receipt.RiverJobID = riverJobID.Int64
	}
	return receipt, nil
}
