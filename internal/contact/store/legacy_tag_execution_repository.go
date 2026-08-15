package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

// LegacyTagExecutionRepository is the Contact-owned persistence adapter for
// local tag-command acceptance. InsertOnlyClient accepts an unregistered River
// job transactionally; it never starts a worker or invokes WeCom.
type LegacyTagExecutionRepository struct {
	client *platformjobqueue.InsertOnlyClient
}

var (
	_ contactapp.LegacyTagSyncReceiptStore         = (*LegacyTagExecutionRepository)(nil)
	_ contactapp.LegacyTagSyncEnqueuer             = (*LegacyTagExecutionRepository)(nil)
	_ contactapp.LegacyTagLiveMutationReceiptStore = (*LegacyTagExecutionRepository)(nil)
	_ contactapp.LegacyTagLiveMutationEnqueuer     = (*LegacyTagExecutionRepository)(nil)
	_ contactapp.LegacyTagExecutionStatusReader    = (*LegacyTagExecutionRepository)(nil)
)

func NewLegacyTagExecutionRepository(pool *pgxpool.Pool) (*LegacyTagExecutionRepository, error) {
	if pool == nil {
		return nil, contactapp.ErrLegacyTagExecutionUnavailable
	}
	client, err := platformjobqueue.NewInsertOnlyClient(pool)
	if err != nil {
		return nil, errors.Join(contactapp.ErrLegacyTagExecutionUnavailable, err)
	}
	return &LegacyTagExecutionRepository{client: client}, nil
}

func (repository *LegacyTagExecutionRepository) ReserveLegacyTagSync(ctx context.Context, command contactapp.LegacyTagSyncCommand) (contactapp.LegacyTagSyncReceipt, error) {
	if repository == nil {
		return contactapp.LegacyTagSyncReceipt{}, contactapp.ErrInvalidLegacyTagSync
	}
	queries, err := legacyTagQueries(ctx)
	if err != nil {
		return contactapp.LegacyTagSyncReceipt{}, err
	}
	digest := legacyTagKeyDigest(command.IdempotencyKey)
	row, err := queries.ReserveLegacyTagSyncReceipt(ctx, contactdb.ReserveLegacyTagSyncReceiptParams{
		ActorID: command.Actor, IdempotencyKey: command.IdempotencyKey, KeyDigest: digest[:], Kind: string(command.Kind), TraceID: command.TraceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		loaded, loadErr := queries.GetLegacyTagSyncReceipt(ctx, contactdb.GetLegacyTagSyncReceiptParams{ActorID: command.Actor, KeyDigest: digest[:]})
		if loadErr != nil {
			return contactapp.LegacyTagSyncReceipt{}, loadErr
		}
		return legacyTagSyncReceipt(loaded.ID, loaded.ActorID, loaded.IdempotencyKey, loaded.Kind, loaded.TraceID, loaded.State, loaded.EventID, loaded.RiverJobID)
	}
	if err != nil {
		return contactapp.LegacyTagSyncReceipt{}, err
	}
	return legacyTagSyncReceipt(row.ID, row.ActorID, row.IdempotencyKey, row.Kind, row.TraceID, row.State, row.EventID, row.RiverJobID)
}

func (repository *LegacyTagExecutionRepository) AcceptLegacyTagSync(ctx context.Context, receiptID int64, eventID eventport.EventID, riverJobID int64) (contactapp.LegacyTagSyncReceipt, error) {
	if repository == nil || receiptID <= 0 || eventID <= 0 || riverJobID <= 0 {
		return contactapp.LegacyTagSyncReceipt{}, contactapp.ErrLegacyTagSyncFailed
	}
	queries, err := legacyTagQueries(ctx)
	if err != nil {
		return contactapp.LegacyTagSyncReceipt{}, err
	}
	row, err := queries.AcceptLegacyTagSyncReceipt(ctx, contactdb.AcceptLegacyTagSyncReceiptParams{
		ID: receiptID, EventID: pgtype.Int8{Int64: int64(eventID), Valid: true}, RiverJobID: pgtype.Int8{Int64: riverJobID, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.LegacyTagSyncReceipt{}, contactapp.ErrLegacyTagSyncFailed
	}
	if err != nil {
		return contactapp.LegacyTagSyncReceipt{}, err
	}
	return legacyTagSyncReceipt(row.ID, row.ActorID, row.IdempotencyKey, row.Kind, row.TraceID, row.State, row.EventID, row.RiverJobID)
}

func (repository *LegacyTagExecutionRepository) EnqueueLegacyTagSync(ctx context.Context, job contactapp.LegacyTagSyncJob) (int64, error) {
	if repository == nil || repository.client == nil || job.ReceiptID <= 0 {
		return 0, contactapp.ErrLegacyTagSyncFailed
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	jobID, err := repository.client.InsertTx(ctx, tx, job, string(platformjobqueue.QueueSync))
	if err != nil || jobID <= 0 {
		return 0, errors.Join(contactapp.ErrLegacyTagSyncFailed, err)
	}
	return jobID, nil
}

func (repository *LegacyTagExecutionRepository) ReserveLegacyTagLiveMutation(ctx context.Context, command contactapp.LegacyTagLiveMutationCommand) (contactapp.LegacyTagLiveMutationReceipt, error) {
	if repository == nil {
		return contactapp.LegacyTagLiveMutationReceipt{}, contactapp.ErrInvalidLegacyTagLiveMutation
	}
	queries, err := legacyTagQueries(ctx)
	if err != nil {
		return contactapp.LegacyTagLiveMutationReceipt{}, err
	}
	digest := legacyTagKeyDigest(command.IdempotencyKey)
	row, err := queries.ReserveLegacyTagLiveMutationReceipt(ctx, contactdb.ReserveLegacyTagLiveMutationReceiptParams{
		ActorID: command.Actor, IdempotencyKey: command.IdempotencyKey, KeyDigest: digest[:], Operation: string(command.Operation), Payload: command.Payload, TraceID: command.TraceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		loaded, loadErr := queries.GetLegacyTagLiveMutationReceipt(ctx, contactdb.GetLegacyTagLiveMutationReceiptParams{ActorID: command.Actor, KeyDigest: digest[:]})
		if loadErr != nil {
			return contactapp.LegacyTagLiveMutationReceipt{}, loadErr
		}
		return legacyTagLiveMutationReceipt(loaded.ID, loaded.ActorID, loaded.IdempotencyKey, loaded.Operation, loaded.Payload, loaded.TraceID, loaded.State, loaded.EventID, loaded.RiverJobID)
	}
	if err != nil {
		return contactapp.LegacyTagLiveMutationReceipt{}, err
	}
	return legacyTagLiveMutationReceipt(row.ID, row.ActorID, row.IdempotencyKey, row.Operation, row.Payload, row.TraceID, row.State, row.EventID, row.RiverJobID)
}

func (repository *LegacyTagExecutionRepository) AcceptLegacyTagLiveMutation(ctx context.Context, receiptID int64, eventID eventport.EventID, riverJobID int64) (contactapp.LegacyTagLiveMutationReceipt, error) {
	if repository == nil || receiptID <= 0 || eventID <= 0 || riverJobID <= 0 {
		return contactapp.LegacyTagLiveMutationReceipt{}, contactapp.ErrLegacyTagLiveMutationFailed
	}
	queries, err := legacyTagQueries(ctx)
	if err != nil {
		return contactapp.LegacyTagLiveMutationReceipt{}, err
	}
	row, err := queries.AcceptLegacyTagLiveMutationReceipt(ctx, contactdb.AcceptLegacyTagLiveMutationReceiptParams{
		ID: receiptID, EventID: pgtype.Int8{Int64: int64(eventID), Valid: true}, RiverJobID: pgtype.Int8{Int64: riverJobID, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.LegacyTagLiveMutationReceipt{}, contactapp.ErrLegacyTagLiveMutationFailed
	}
	if err != nil {
		return contactapp.LegacyTagLiveMutationReceipt{}, err
	}
	return legacyTagLiveMutationReceipt(row.ID, row.ActorID, row.IdempotencyKey, row.Operation, row.Payload, row.TraceID, row.State, row.EventID, row.RiverJobID)
}

func (repository *LegacyTagExecutionRepository) EnqueueLegacyTagLiveMutation(ctx context.Context, job contactapp.LegacyTagLiveMutationJob) (int64, error) {
	if repository == nil || repository.client == nil || job.ReceiptID <= 0 {
		return 0, contactapp.ErrLegacyTagLiveMutationFailed
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	jobID, err := repository.client.InsertTx(ctx, tx, job, string(platformjobqueue.QueueSync))
	if err != nil || jobID <= 0 {
		return 0, errors.Join(contactapp.ErrLegacyTagLiveMutationFailed, err)
	}
	return jobID, nil
}

func (repository *LegacyTagExecutionRepository) ReadLegacyTagExecutionStatus(ctx context.Context) (contactapp.LegacyTagExecutionStatus, error) {
	if repository == nil {
		return contactapp.LegacyTagExecutionStatus{}, contactapp.ErrLegacyTagExecutionUnavailable
	}
	queries, err := legacyTagQueries(ctx)
	if err != nil {
		return contactapp.LegacyTagExecutionStatus{}, err
	}
	payload, err := queries.GetLegacyTagExecutionStatus(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.LegacyTagExecutionStatus{}, contactapp.ErrLegacyTagExecutionUnavailable
	}
	if err != nil {
		return contactapp.LegacyTagExecutionStatus{}, err
	}
	return contactapp.LegacyTagExecutionStatus{Payload: append([]byte(nil), payload...)}, nil
}

func legacyTagKeyDigest(key string) [sha256.Size]byte { return sha256.Sum256([]byte(key)) }

func legacyTagSyncReceipt(id, actor int64, key, kind, trace, state string, eventID, riverJobID pgtype.Int8) (contactapp.LegacyTagSyncReceipt, error) {
	if id <= 0 || actor <= 0 || strings.TrimSpace(key) != key || strings.TrimSpace(trace) != trace || !eventID.Valid && eventID.Int64 != 0 || !riverJobID.Valid && riverJobID.Int64 != 0 {
		return contactapp.LegacyTagSyncReceipt{}, contactapp.ErrLegacyTagSyncFailed
	}
	receipt := contactapp.LegacyTagSyncReceipt{ID: id, Command: contactapp.LegacyTagSyncCommand{Actor: actor, IdempotencyKey: key, TraceID: trace, Kind: contactapp.LegacyTagSyncKind(kind)}, State: contactapp.LegacyTagSyncReceiptState(state)}
	if eventID.Valid {
		receipt.EventID = eventport.EventID(eventID.Int64)
	}
	if riverJobID.Valid {
		receipt.RiverJobID = riverJobID.Int64
	}
	return receipt, nil
}

func legacyTagLiveMutationReceipt(id, actor int64, key, operation string, payload []byte, trace, state string, eventID, riverJobID pgtype.Int8) (contactapp.LegacyTagLiveMutationReceipt, error) {
	if id <= 0 || actor <= 0 || strings.TrimSpace(key) != key || strings.TrimSpace(trace) != trace || len(payload) == 0 || !eventID.Valid && eventID.Int64 != 0 || !riverJobID.Valid && riverJobID.Int64 != 0 {
		return contactapp.LegacyTagLiveMutationReceipt{}, contactapp.ErrLegacyTagLiveMutationFailed
	}
	receipt := contactapp.LegacyTagLiveMutationReceipt{ID: id, Command: contactapp.LegacyTagLiveMutationCommand{Actor: actor, IdempotencyKey: key, TraceID: trace, Operation: contactapp.LegacyTagLiveMutationOperation(operation), Payload: append([]byte(nil), payload...)}, State: contactapp.LegacyTagLiveMutationReceiptState(state)}
	if eventID.Valid {
		receipt.EventID = eventport.EventID(eventID.Int64)
	}
	if riverJobID.Valid {
		receipt.RiverJobID = riverJobID.Int64
	}
	return receipt, nil
}
