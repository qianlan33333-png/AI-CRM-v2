// Package store owns Automation's durable tag-trigger receipt and its
// transaction-scoped consumer. It never writes Events-owned tables directly.
package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	automationdb "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store/generated"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var (
	ErrInvalidTagTrigger  = errors.New("invalid automation tag trigger")
	ErrTagTriggerConflict = errors.New("automation tag trigger conflict")
)

type TriggerReceipt struct {
	ID               int64
	EventID          eventport.EventID
	CustomerID       eventport.CustomerID
	TagID            int64
	Actor            string
	TriggeredEventID eventport.EventID
	TriggeredAt      time.Time
	CompletedAt      time.Time
}

type TriggerListInput struct {
	Page, PageSize     int32
	ReceiptID, EventID *int64
	StartedAfter       *time.Time
	StartedBefore      *time.Time
}

type TriggerListResult struct {
	Items []TriggerReceipt
	Total int64
}

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (repository *Repository) List(ctx context.Context, input TriggerListInput) (TriggerListResult, error) {
	if repository == nil || repository.pool == nil || input.Page < 1 || input.Page > 10000 || input.PageSize < 1 || input.PageSize > 100 ||
		(input.ReceiptID != nil && *input.ReceiptID <= 0) || (input.EventID != nil && *input.EventID <= 0) ||
		(input.StartedAfter != nil && input.StartedBefore != nil && !input.StartedAfter.Before(*input.StartedBefore)) {
		return TriggerListResult{}, ErrInvalidTagTrigger
	}
	filter := triggerFilter(input)
	queries := automationdb.New(repository.pool)
	total, err := queries.CountAutomationTriggerReceipts(ctx, automationdb.CountAutomationTriggerReceiptsParams(filter))
	if err != nil {
		return TriggerListResult{}, err
	}
	rows, err := queries.ListAutomationTriggerReceipts(ctx, automationdb.ListAutomationTriggerReceiptsParams{
		ReceiptID: filter.ReceiptID, EventID: filter.EventID,
		StartedAfter: filter.StartedAfter, StartedBefore: filter.StartedBefore,
		PageOffset: (input.Page - 1) * input.PageSize, PageSize: input.PageSize,
	})
	if err != nil {
		return TriggerListResult{}, err
	}
	result := TriggerListResult{Items: make([]TriggerReceipt, 0, len(rows)), Total: total}
	for _, row := range rows {
		receipt, mapErr := mapReceipt(row)
		if mapErr != nil {
			return TriggerListResult{}, mapErr
		}
		result.Items = append(result.Items, receipt)
	}
	return result, nil
}

type tagTriggerConsumer struct {
	uow        platformport.UnitOfWork
	repository *Repository
	events     eventport.Appender
	deliveries eventport.DeliveryCompleter
}

var _ eventport.DeliverySubscriber = (*tagTriggerConsumer)(nil)

func NewTagTriggerConsumer(uow platformport.UnitOfWork, repository *Repository, events eventport.Appender, deliveries eventport.DeliveryCompleter) (eventport.DeliverySubscriber, error) {
	if uow == nil || repository == nil || repository.pool == nil || events == nil || deliveries == nil {
		return nil, ErrInvalidTagTrigger
	}
	return &tagTriggerConsumer{uow: uow, repository: repository, events: events, deliveries: deliveries}, nil
}

func (*tagTriggerConsumer) Consumer() string     { return eventport.ConsumerAutomationTagTrigger }
func (*tagTriggerConsumer) EventTypes() []string { return []string{eventport.EvTagApplied} }

func (consumer *tagTriggerConsumer) ConsumeDelivery(ctx context.Context, claim eventport.DeliveryClaim) error {
	payload, err := decodeTagApplied(claim)
	if err != nil {
		return eventport.PoisonDelivery(err)
	}
	err = consumer.uow.Within(ctx, func(txCtx context.Context) error {
		queries, queryErr := automationQueries(txCtx)
		if queryErr != nil {
			return queryErr
		}
		receipt, queryErr := queries.ReserveAutomationTriggerReceipt(txCtx, automationdb.ReserveAutomationTriggerReceiptParams{
			EventID: int64(claim.Record.ID), Consumer: claim.Consumer, CustomerID: payload.CustomerID,
			TagID: payload.TagID, Actor: payload.Actor,
		})
		if errors.Is(queryErr, pgx.ErrNoRows) {
			return eventport.PoisonDelivery(ErrTagTriggerConflict)
		}
		if queryErr != nil {
			return queryErr
		}
		if receipt.State == "triggered" {
			if !receipt.TriggeredEventID.Valid || receipt.TriggeredEventID.Int64 <= 0 {
				return eventport.PoisonDelivery(ErrTagTriggerConflict)
			}
			return consumer.deliveries.Complete(txCtx, claim.Record.ID, claim.Consumer, claim.Owner)
		}
		triggerPayload, marshalErr := json.Marshal(struct {
			SourceEventID int64  `json:"source_event_id"`
			CustomerID    int64  `json:"customer_id"`
			TagID         int64  `json:"tag_id"`
			Actor         string `json:"actor"`
			Consumer      string `json:"consumer"`
		}{int64(claim.Record.ID), payload.CustomerID, payload.TagID, payload.Actor, claim.Consumer})
		if marshalErr != nil || !receipt.TriggeredAt.Valid {
			return errors.Join(ErrInvalidTagTrigger, marshalErr)
		}
		triggeredEventID, appendErr := consumer.events.Append(txCtx, eventport.Event{
			Type: eventport.EvAutomationTriggered, CustomerID: claim.Record.CustomerID,
			Payload: triggerPayload, OccurredAt: receipt.TriggeredAt.Time.UTC(),
			IdempotencyKey: "automation.triggered:" + claim.Record.IdempotencyKey + ":" + claim.Consumer,
		})
		if errors.Is(appendErr, eventport.ErrIdempotencyConflict) {
			return eventport.PoisonDelivery(appendErr)
		}
		if appendErr != nil {
			return appendErr
		}
		completed, queryErr := queries.CompleteAutomationTriggerReceipt(txCtx, automationdb.CompleteAutomationTriggerReceiptParams{
			TriggeredEventID: pgtype.Int8{Int64: int64(triggeredEventID), Valid: true}, ID: receipt.ID,
		})
		if queryErr != nil || completed.State != "triggered" || !completed.TriggeredEventID.Valid || completed.TriggeredEventID.Int64 != int64(triggeredEventID) {
			return errors.Join(ErrTagTriggerConflict, queryErr)
		}
		return consumer.deliveries.Complete(txCtx, claim.Record.ID, claim.Consumer, claim.Owner)
	})
	return err
}

type tagAppliedPayload struct {
	CustomerID int64  `json:"customer_id"`
	TagID      int64  `json:"tag_id"`
	Actor      string `json:"actor"`
}

func decodeTagApplied(claim eventport.DeliveryClaim) (tagAppliedPayload, error) {
	if claim.Record.ID <= 0 || claim.Record.Type != eventport.EvTagApplied || claim.Record.CustomerID <= 0 ||
		claim.Consumer != eventport.ConsumerAutomationTagTrigger || claim.Owner == "" || claim.Status != eventport.DeliveryProcessing {
		return tagAppliedPayload{}, ErrInvalidTagTrigger
	}
	decoder := json.NewDecoder(bytes.NewReader(claim.Record.Payload))
	decoder.DisallowUnknownFields()
	var payload tagAppliedPayload
	if err := decoder.Decode(&payload); err != nil {
		return payload, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return payload, ErrInvalidTagTrigger
	}
	if payload.CustomerID != int64(claim.Record.CustomerID) || payload.TagID <= 0 || payload.Actor == "" ||
		len(payload.Actor) > 200 || strings.TrimSpace(payload.Actor) != payload.Actor {
		return payload, ErrInvalidTagTrigger
	}
	return payload, nil
}

func automationQueries(ctx context.Context) (*automationdb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return automationdb.New(tx), nil
}

func triggerFilter(input TriggerListInput) automationdb.CountAutomationTriggerReceiptsParams {
	filter := automationdb.CountAutomationTriggerReceiptsParams{}
	if input.ReceiptID != nil {
		filter.ReceiptID = pgtype.Int8{Int64: *input.ReceiptID, Valid: true}
	}
	if input.EventID != nil {
		filter.EventID = pgtype.Int8{Int64: *input.EventID, Valid: true}
	}
	if input.StartedAfter != nil {
		filter.StartedAfter = pgtype.Timestamptz{Time: input.StartedAfter.UTC(), Valid: true}
	}
	if input.StartedBefore != nil {
		filter.StartedBefore = pgtype.Timestamptz{Time: input.StartedBefore.UTC(), Valid: true}
	}
	return filter
}

func mapReceipt(row automationdb.AutomationTriggerReceipt) (TriggerReceipt, error) {
	if row.ID <= 0 || row.EventID <= 0 || row.CustomerID <= 0 || row.TagID <= 0 || row.State != "triggered" ||
		!row.TriggeredEventID.Valid || row.TriggeredEventID.Int64 <= 0 || !row.TriggeredAt.Valid || !row.CompletedAt.Valid {
		return TriggerReceipt{}, ErrTagTriggerConflict
	}
	return TriggerReceipt{
		ID: row.ID, EventID: eventport.EventID(row.EventID), CustomerID: eventport.CustomerID(row.CustomerID),
		TagID: row.TagID, Actor: row.Actor, TriggeredEventID: eventport.EventID(row.TriggeredEventID.Int64),
		TriggeredAt: row.TriggeredAt.Time.UTC(), CompletedAt: row.CompletedAt.Time.UTC(),
	}, nil
}
