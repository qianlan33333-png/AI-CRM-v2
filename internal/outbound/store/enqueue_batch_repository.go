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

type EnqueueBatchRepository struct {
	client *platformjobqueue.InsertOnlyClient
}

var _ outboundapp.EnqueueBatchRepository = (*EnqueueBatchRepository)(nil)

func NewEnqueueBatchRepository(pool *pgxpool.Pool) (*EnqueueBatchRepository, error) {
	if pool == nil {
		return nil, outboundapp.ErrEnqueueBatchFailed
	}
	client, err := platformjobqueue.NewInsertOnlyClient(pool)
	if err != nil {
		return nil, errors.Join(outboundapp.ErrEnqueueBatchFailed, err)
	}
	return &EnqueueBatchRepository{client: client}, nil
}

func (repository *EnqueueBatchRepository) ReserveBatch(ctx context.Context, definition outboundapp.BatchDefinition) (outboundapp.BatchReceipt, error) {
	queries, err := enqueueBatchQueries(ctx)
	if repository == nil || err != nil {
		return outboundapp.BatchReceipt{}, errors.Join(outboundapp.ErrEnqueueBatchFailed, err)
	}
	row, err := queries.ReserveOutboundBatch(ctx, outbounddb.ReserveOutboundBatchParams{
		IdempotencyScope: definition.IdempotencyScope, IdempotencyKey: definition.IdempotencyKey,
		Tier: string(definition.Tier), RecipientDigest: definition.RecipientDigest[:], RecipientCount: int32(definition.RecipientCount),
		TemplateKey: definition.TemplateKey, Payload: definition.Payload,
	})
	if err != nil {
		return outboundapp.BatchReceipt{}, errors.Join(outboundapp.ErrEnqueueBatchFailed, err)
	}
	return batchReceipt(row.ID, row.IdempotencyScope, row.IdempotencyKey, row.Tier, row.RecipientDigest, row.RecipientCount, row.TemplateKey, row.Payload, row.AcceptedEventID)
}

func (repository *EnqueueBatchRepository) AcceptBatch(ctx context.Context, batchID int64, eventID eventport.EventID) (outboundapp.BatchReceipt, error) {
	queries, err := enqueueBatchQueries(ctx)
	if repository == nil || batchID <= 0 || eventID <= 0 || err != nil {
		return outboundapp.BatchReceipt{}, errors.Join(outboundapp.ErrEnqueueBatchFailed, err)
	}
	row, err := queries.AcceptOutboundBatch(ctx, outbounddb.AcceptOutboundBatchParams{AcceptedEventID: int64(eventID), ID: batchID})
	if err != nil {
		return outboundapp.BatchReceipt{}, errors.Join(outboundapp.ErrEnqueueBatchFailed, err)
	}
	return batchReceipt(row.ID, row.IdempotencyScope, row.IdempotencyKey, row.Tier, row.RecipientDigest, row.RecipientCount, row.TemplateKey, row.Payload, row.AcceptedEventID)
}

func (repository *EnqueueBatchRepository) ReserveBatchChunk(ctx context.Context, batchID int64, index, start, count int) (outboundapp.BatchChunk, error) {
	queries, err := enqueueBatchQueries(ctx)
	if repository == nil || batchID <= 0 || index < 0 || start < 0 || count <= 0 || err != nil {
		return outboundapp.BatchChunk{}, errors.Join(outboundapp.ErrEnqueueBatchFailed, err)
	}
	row, err := queries.ReserveOutboundBatchChunk(ctx, outbounddb.ReserveOutboundBatchChunkParams{BatchID: batchID, ChunkIndex: int32(index), RecipientStart: int32(start), RecipientCount: int32(count)})
	if err != nil {
		return outboundapp.BatchChunk{}, errors.Join(outboundapp.ErrEnqueueBatchFailed, err)
	}
	return outboundapp.BatchChunk{ID: row.ID, BatchID: row.BatchID, Index: int(row.ChunkIndex), RecipientStart: int(row.RecipientStart), RecipientCount: int(row.RecipientCount), State: outboundapp.BatchChunkState(row.State)}, nil
}

func (repository *EnqueueBatchRepository) CreateBatchTask(ctx context.Context, command outboundapp.BatchTaskCommand) (outboundapp.TaskID, error) {
	queries, err := enqueueBatchQueries(ctx)
	if repository == nil || command.BatchID <= 0 || command.ChunkIndex < 0 || command.CustomerID <= 0 || err != nil {
		return 0, errors.Join(outboundapp.ErrEnqueueBatchFailed, err)
	}
	id, err := queries.CreateOutboundBatchTask(ctx, outbounddb.CreateOutboundBatchTaskParams{
		CustomerID: command.CustomerID, TemplateKey: command.TemplateKey, Payload: command.Payload,
		BatchID: command.BatchID, BatchChunkIndex: int32(command.ChunkIndex),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, outboundapp.ErrInvalidEnqueueBatchCommand
	}
	if err != nil || id <= 0 {
		return 0, errors.Join(outboundapp.ErrEnqueueBatchFailed, err)
	}
	return outboundapp.TaskID(id), nil
}

func (repository *EnqueueBatchRepository) EnqueueBatchTask(ctx context.Context, args outboundapp.EnqueueBatchTaskArgs) (int64, error) {
	if repository == nil || repository.client == nil || args.BatchID <= 0 || args.ChunkIndex < 0 || args.TaskID <= 0 {
		return 0, outboundapp.ErrEnqueueBatchFailed
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	jobID, err := repository.client.InsertTx(ctx, tx, args, "outbound")
	if err != nil || jobID <= 0 {
		return 0, errors.Join(outboundapp.ErrEnqueueBatchFailed, err)
	}
	return jobID, nil
}

func (repository *EnqueueBatchRepository) AcceptBatchChunk(ctx context.Context, chunkID int64) (outboundapp.BatchChunk, error) {
	queries, err := enqueueBatchQueries(ctx)
	if repository == nil || chunkID <= 0 || err != nil {
		return outboundapp.BatchChunk{}, errors.Join(outboundapp.ErrEnqueueBatchFailed, err)
	}
	row, err := queries.AcceptOutboundBatchChunk(ctx, chunkID)
	if err != nil {
		return outboundapp.BatchChunk{}, errors.Join(outboundapp.ErrEnqueueBatchFailed, err)
	}
	return outboundapp.BatchChunk{ID: row.ID, BatchID: row.BatchID, Index: int(row.ChunkIndex), RecipientStart: int(row.RecipientStart), RecipientCount: int(row.RecipientCount), State: outboundapp.BatchChunkState(row.State)}, nil
}

func enqueueBatchQueries(ctx context.Context) (*outbounddb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return outbounddb.New(tx), nil
}

func batchReceipt(id int64, scope, key, tier string, digest []byte, count int32, template string, payload []byte, eventID pgtype.Int8) (outboundapp.BatchReceipt, error) {
	if id <= 0 || len(digest) != 32 || count <= 0 {
		return outboundapp.BatchReceipt{}, outboundapp.ErrEnqueueBatchFailed
	}
	var fixed [32]byte
	copy(fixed[:], digest)
	receipt := outboundapp.BatchReceipt{ID: id, Definition: outboundapp.BatchDefinition{
		IdempotencyScope: scope, IdempotencyKey: key, Tier: outboundapp.BatchTier(tier), RecipientDigest: fixed,
		RecipientCount: int(count), TemplateKey: template, Payload: payload,
	}}
	if eventID.Valid {
		receipt.AcceptedEventID = eventport.EventID(eventID.Int64)
	}
	return receipt, nil
}
