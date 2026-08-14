package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventdb "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store/generated"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type Appender struct{}

var _ eventport.Appender = (*Appender)(nil)

func NewAppender() *Appender {
	return &Appender{}
}

func (a *Appender) Append(ctx context.Context, event eventport.Event) (eventport.EventID, error) {
	if err := validate(event); err != nil {
		return 0, err
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}

	params := eventdb.AppendEventParams{
		EventType:      event.Type,
		Payload:        event.Payload,
		OccurredAt:     pgtype.Timestamptz{Time: event.OccurredAt, Valid: true},
		IdempotencyKey: event.IdempotencyKey,
	}
	if event.CustomerID > 0 {
		params.CustomerID = pgtype.Int8{Int64: int64(event.CustomerID), Valid: true}
	}
	id, err := eventdb.New(tx).AppendEvent(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, eventport.ErrIdempotencyConflict
	}
	if err != nil {
		return 0, err
	}
	return eventport.EventID(id), nil
}

func validate(event eventport.Event) error {
	if strings.TrimSpace(event.Type) == "" || event.CustomerID < 0 ||
		event.OccurredAt.IsZero() || strings.TrimSpace(event.IdempotencyKey) == "" ||
		len(event.Payload) == 0 || !json.Valid(event.Payload) {
		return eventport.ErrInvalidEvent
	}
	return nil
}

type transactionalEnqueuer interface {
	EnqueueTx(context.Context, pgx.Tx, platformjobqueue.Queue, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

type eventJobEnqueuer interface {
	Enqueue(context.Context, pgx.Tx, eventport.DeliveryJobArgs) (int64, error)
}

type insertOnlyEnqueuer struct {
	client *platformjobqueue.InsertOnlyClient
}

func (enqueuer insertOnlyEnqueuer) Enqueue(ctx context.Context, tx pgx.Tx, args eventport.DeliveryJobArgs) (int64, error) {
	return enqueuer.client.InsertTx(ctx, tx, args, string(platformjobqueue.QueueEvent))
}

type registeredEnqueuer struct{ client transactionalEnqueuer }

func (enqueuer registeredEnqueuer) Enqueue(ctx context.Context, tx pgx.Tx, args eventport.DeliveryJobArgs) (int64, error) {
	result, err := enqueuer.client.EnqueueTx(ctx, tx, platformjobqueue.QueueEvent, args, &river.InsertOpts{
		UniqueOpts: river.UniqueOpts{ByArgs: true},
	})
	if err != nil || result == nil || result.Job == nil || result.Job.ID <= 0 {
		return 0, errors.Join(eventport.ErrInvalidDelivery, err)
	}
	return result.Job.ID, nil
}

// DeliveryRepository is the only Events implementation allowed to mutate
// event_deliveries or to accept an Events River job in a caller's UoW.
type DeliveryRepository struct {
	pool       *pgxpool.Pool
	uow        platformport.UnitOfWork
	enqueuer   eventJobEnqueuer
	bindings   []eventport.DeliveryBinding
	boundTypes []string
	batchSize  int32
	now        func() time.Time
}

var (
	_ eventport.DeliveryAcceptor  = (*DeliveryRepository)(nil)
	_ eventport.DeliveryCompleter = (*DeliveryRepository)(nil)
	_ eventport.DeliveryRuntime   = (*DeliveryRepository)(nil)
)

func NewProducerDeliveryRepository(pool *pgxpool.Pool) (*DeliveryRepository, error) {
	client, err := platformjobqueue.NewInsertOnlyClient(pool)
	if err != nil {
		return nil, err
	}
	return newDeliveryRepository(pool, insertOnlyEnqueuer{client: client}, 0, nil)
}

func NewRuntimeDeliveryRepository(
	pool *pgxpool.Pool,
	enqueuer transactionalEnqueuer,
	batchSize int32,
	bindings []eventport.DeliveryBinding,
) (*DeliveryRepository, error) {
	if enqueuer == nil || batchSize <= 0 {
		return nil, eventport.ErrInvalidDelivery
	}
	return newDeliveryRepository(pool, registeredEnqueuer{client: enqueuer}, batchSize, bindings)
}

func newDeliveryRepository(pool *pgxpool.Pool, enqueuer eventJobEnqueuer, batchSize int32, bindings []eventport.DeliveryBinding) (*DeliveryRepository, error) {
	if pool == nil || enqueuer == nil {
		return nil, eventport.ErrInvalidDelivery
	}
	seen := make(map[string]struct{})
	seenTypes := make(map[string]struct{})
	boundTypes := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		if !validDeliveryName(binding.EventType) || !validDeliveryName(binding.Consumer) {
			return nil, eventport.ErrInvalidDelivery
		}
		key := binding.EventType + "\x00" + binding.Consumer
		if _, duplicate := seen[key]; duplicate {
			return nil, eventport.ErrInvalidDelivery
		}
		seen[key] = struct{}{}
		if _, exists := seenTypes[binding.EventType]; !exists {
			seenTypes[binding.EventType] = struct{}{}
			boundTypes = append(boundTypes, binding.EventType)
		}
	}
	return &DeliveryRepository{
		pool: pool, uow: platformstore.NewUnitOfWork(pool), enqueuer: enqueuer,
		bindings: append([]eventport.DeliveryBinding(nil), bindings...), boundTypes: boundTypes,
		batchSize: batchSize, now: time.Now,
	}, nil
}

func (repository *DeliveryRepository) Accept(ctx context.Context, eventID eventport.EventID, consumer string) error {
	if repository == nil || repository.enqueuer == nil || eventID <= 0 || !validDeliveryName(consumer) {
		return eventport.ErrInvalidDelivery
	}
	queries, err := deliveryQueries(ctx)
	if err != nil {
		return err
	}
	if err = repository.accept(ctx, queries, eventID, consumer); err != nil {
		return err
	}
	_, err = queries.MarkEventDispatched(ctx, int64(eventID))
	return err
}

func (repository *DeliveryRepository) accept(ctx context.Context, queries *eventdb.Queries, eventID eventport.EventID, consumer string) error {
	_, err := queries.ReserveEventDelivery(ctx, eventdb.ReserveEventDeliveryParams{EventID: int64(eventID), Consumer: consumer})
	if errors.Is(err, pgx.ErrNoRows) {
		existing, loadErr := queries.GetEventDelivery(ctx, eventdb.GetEventDeliveryParams{EventID: int64(eventID), Consumer: consumer})
		if loadErr != nil || !existing.RiverJobID.Valid || existing.RiverJobID.Int64 <= 0 {
			return errors.Join(eventport.ErrInvalidDelivery, loadErr)
		}
		return nil
	}
	if err != nil {
		return err
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	jobID, err := repository.enqueuer.Enqueue(ctx, tx, eventport.DeliveryJobArgs{EventID: int64(eventID), Consumer: consumer})
	if err != nil {
		return err
	}
	accepted, err := queries.AcceptEventDelivery(ctx, eventdb.AcceptEventDeliveryParams{
		RiverJobID: pgtype.Int8{Int64: jobID, Valid: true}, EventID: int64(eventID), Consumer: consumer,
	})
	if err != nil || !accepted.Valid || accepted.Int64 != jobID {
		return errors.Join(eventport.ErrInvalidDelivery, err)
	}
	return nil
}

func (repository *DeliveryRepository) Dispatch(ctx context.Context) (count int, err error) {
	if repository == nil || repository.uow == nil || repository.enqueuer == nil || repository.batchSize <= 0 {
		return 0, eventport.ErrInvalidDelivery
	}
	err = repository.uow.Within(ctx, func(txCtx context.Context) error {
		queries, queryErr := deliveryQueries(txCtx)
		if queryErr != nil {
			return queryErr
		}
		tx, queryErr := platformstore.TxFromContext(txCtx)
		if queryErr != nil {
			return queryErr
		}
		remaining := repository.batchSize
		for _, binding := range repository.bindings {
			if remaining == 0 {
				break
			}
			events, claimErr := queries.ClaimEventsMissingDelivery(txCtx, eventdb.ClaimEventsMissingDeliveryParams{
				EventType: binding.EventType, Consumer: binding.Consumer, BatchSize: remaining,
			})
			if claimErr != nil {
				return claimErr
			}
			for _, event := range events {
				if queryErr = repository.accept(txCtx, queries, eventport.EventID(event.ID), binding.Consumer); queryErr != nil {
					return queryErr
				}
				if _, queryErr = queries.MarkEventDispatched(txCtx, event.ID); queryErr != nil {
					return queryErr
				}
				count++
				remaining--
			}
		}
		if remaining == 0 {
			return nil
		}
		events, queryErr := queries.ClaimUndispatchedEvents(txCtx, eventdb.ClaimUndispatchedEventsParams{
			ExcludedEventTypes: repository.boundTypes, BatchSize: remaining,
		})
		if queryErr != nil {
			return queryErr
		}
		ids := make([]int64, 0, len(events))
		for _, event := range events {
			if _, queryErr = repository.enqueuer.Enqueue(txCtx, tx, eventport.DeliveryJobArgs{EventID: event.ID}); queryErr != nil {
				return queryErr
			}
			ids = append(ids, event.ID)
		}
		if len(ids) == 0 {
			return nil
		}
		updated, queryErr := queries.MarkEventsDispatched(txCtx, ids)
		if queryErr != nil || updated != int64(len(ids)) {
			return errors.Join(eventport.ErrInvalidDelivery, queryErr)
		}
		count += len(ids)
		return nil
	})
	return count, err
}

func (repository *DeliveryRepository) Load(ctx context.Context, eventID eventport.EventID) (eventport.Record, error) {
	if repository == nil || repository.pool == nil || eventID <= 0 {
		return eventport.Record{}, eventport.ErrInvalidDelivery
	}
	row, err := eventdb.New(repository.pool).GetEvent(ctx, int64(eventID))
	if err != nil {
		return eventport.Record{}, err
	}
	return deliveryRecord(row.ID, row.EventType, row.CustomerID, row.Payload, row.OccurredAt, row.IdempotencyKey)
}

func (repository *DeliveryRepository) Claim(ctx context.Context, eventID eventport.EventID, consumer, owner string, lease time.Duration) (claim eventport.DeliveryClaim, err error) {
	if repository == nil || repository.uow == nil || eventID <= 0 || !validDeliveryName(consumer) || !validDeliveryName(owner) || lease <= 0 || lease > time.Hour {
		return claim, eventport.ErrInvalidDelivery
	}
	err = repository.uow.Within(ctx, func(txCtx context.Context) error {
		queries, queryErr := deliveryQueries(txCtx)
		if queryErr != nil {
			return queryErr
		}
		now := repository.now().UTC()
		row, queryErr := queries.ClaimEventDelivery(txCtx, eventdb.ClaimEventDeliveryParams{
			LeaseOwner:     pgtype.Text{String: owner, Valid: true},
			LeaseExpiresAt: pgtype.Timestamptz{Time: now.Add(lease), Valid: true},
			EventID:        int64(eventID), Consumer: consumer,
			ClaimedAt: pgtype.Timestamptz{Time: now, Valid: true},
		})
		if errors.Is(queryErr, pgx.ErrNoRows) {
			delivery, loadErr := queries.GetEventDelivery(txCtx, eventdb.GetEventDeliveryParams{EventID: int64(eventID), Consumer: consumer})
			if loadErr != nil {
				return errors.Join(eventport.ErrInvalidDelivery, loadErr)
			}
			status := eventport.DeliveryStatus(delivery.Status)
			if status == eventport.DeliveryCompleted || status == eventport.DeliveryFinalFailed || status == eventport.DeliveryOutcomeUnknown {
				claim = eventport.DeliveryClaim{Consumer: consumer, Owner: owner, Status: status, Attempt: delivery.AttemptCount}
				return nil
			}
			return eventport.ErrDeliveryLeaseActive
		}
		if queryErr != nil {
			return queryErr
		}
		record, queryErr := deliveryRecord(row.ID, row.EventType, row.CustomerID, row.Payload, row.OccurredAt, row.IdempotencyKey)
		if queryErr != nil {
			return queryErr
		}
		claim = eventport.DeliveryClaim{Record: record, Consumer: row.Consumer, Owner: owner, Status: eventport.DeliveryProcessing, Attempt: row.AttemptCount}
		return nil
	})
	return claim, err
}

func (repository *DeliveryRepository) Complete(ctx context.Context, eventID eventport.EventID, consumer, owner string) error {
	if eventID <= 0 || !validDeliveryName(consumer) || !validDeliveryName(owner) {
		return eventport.ErrInvalidDelivery
	}
	queries, err := deliveryQueries(ctx)
	if err != nil {
		return err
	}
	status, err := queries.CompleteEventDelivery(ctx, eventdb.CompleteEventDeliveryParams{
		EventID: int64(eventID), Consumer: consumer, LeaseOwner: pgtype.Text{String: owner, Valid: true},
	})
	if err != nil || status != string(eventport.DeliveryCompleted) {
		return errors.Join(eventport.ErrInvalidDelivery, err)
	}
	return nil
}

func (repository *DeliveryRepository) Retry(ctx context.Context, eventID eventport.EventID, consumer, owner, code string) error {
	return repository.transition(ctx, eventID, consumer, owner, code, eventport.DeliveryPending)
}

func (repository *DeliveryRepository) FinalFail(ctx context.Context, eventID eventport.EventID, consumer, owner, code string) error {
	return repository.transition(ctx, eventID, consumer, owner, code, eventport.DeliveryFinalFailed)
}

func (repository *DeliveryRepository) OutcomeUnknown(ctx context.Context, eventID eventport.EventID, consumer, owner, code string) error {
	return repository.transition(ctx, eventID, consumer, owner, code, eventport.DeliveryOutcomeUnknown)
}

func (repository *DeliveryRepository) transition(ctx context.Context, eventID eventport.EventID, consumer, owner, code string, target eventport.DeliveryStatus) error {
	if repository == nil || repository.uow == nil || eventID <= 0 || !validDeliveryName(consumer) || !validDeliveryName(owner) || !validErrorCode(code) {
		return eventport.ErrInvalidDelivery
	}
	return repository.uow.Within(ctx, func(txCtx context.Context) error {
		queries, err := deliveryQueries(txCtx)
		if err != nil {
			return err
		}
		params := eventdb.RetryEventDeliveryParams{ErrorCode: pgtype.Text{String: code, Valid: true}, EventID: int64(eventID), Consumer: consumer, LeaseOwner: pgtype.Text{String: owner, Valid: true}}
		var updated int64
		switch target {
		case eventport.DeliveryPending:
			updated, err = queries.RetryEventDelivery(txCtx, params)
		case eventport.DeliveryFinalFailed:
			updated, err = queries.FinalFailEventDelivery(txCtx, eventdb.FinalFailEventDeliveryParams(params))
		case eventport.DeliveryOutcomeUnknown:
			updated, err = queries.OutcomeUnknownEventDelivery(txCtx, eventdb.OutcomeUnknownEventDeliveryParams(params))
		default:
			return eventport.ErrInvalidDelivery
		}
		if err != nil || updated != 1 {
			return errors.Join(eventport.ErrInvalidDelivery, err)
		}
		return nil
	})
}

func deliveryQueries(ctx context.Context) (*eventdb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return eventdb.New(tx), nil
}

func deliveryRecord(id int64, eventType string, customerID pgtype.Int8, payload []byte, occurredAt pgtype.Timestamptz, key string) (eventport.Record, error) {
	if id <= 0 || !validDeliveryName(eventType) || !occurredAt.Valid || occurredAt.Time.IsZero() || key == "" || !json.Valid(payload) {
		return eventport.Record{}, eventport.ErrInvalidDelivery
	}
	record := eventport.Record{ID: eventport.EventID(id), Event: eventport.Event{Type: eventType, Payload: append([]byte(nil), payload...), OccurredAt: occurredAt.Time, IdempotencyKey: key}}
	if customerID.Valid {
		record.CustomerID = eventport.CustomerID(customerID.Int64)
	}
	return record, nil
}

func validDeliveryName(value string) bool {
	return value != "" && len(value) <= 200 && strings.TrimSpace(value) == value
}

func validErrorCode(value string) bool {
	return value != "" && len(value) <= 100 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\r\n\t")
}
