package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventdb "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store/generated"
)

type adminReadTxBeginner interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

type AdminReadRepository struct {
	db adminReadTxBeginner
}

func NewAdminReadRepository(db adminReadTxBeginner) *AdminReadRepository {
	return &AdminReadRepository{db: db}
}

func (repository *AdminReadRepository) Read(ctx context.Context, eventType string) (eventport.AdminReadSnapshot, error) {
	if repository == nil || repository.db == nil || ctx == nil {
		return eventport.AdminReadSnapshot{}, errors.New("admin read repository unavailable")
	}
	tx, err := repository.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return eventport.AdminReadSnapshot{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()

	queries := eventdb.New(tx)
	events, err := queries.ListAdminReadEvents(ctx, eventType)
	if err != nil {
		return eventport.AdminReadSnapshot{}, err
	}
	ids := make([]int64, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	deliveries := make([]eventdb.ListAdminReadDeliveriesRow, 0)
	if len(ids) > 0 {
		deliveries, err = queries.ListAdminReadDeliveries(ctx, ids)
		if err != nil {
			return eventport.AdminReadSnapshot{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return eventport.AdminReadSnapshot{}, err
	}
	committed = true

	snapshot := eventport.AdminReadSnapshot{
		Events:     make([]eventport.AdminReadEvent, 0, len(events)),
		Deliveries: make([]eventport.AdminReadDelivery, 0, len(deliveries)),
	}
	for _, event := range events {
		if !event.OccurredAt.Valid {
			return eventport.AdminReadSnapshot{}, errors.New("invalid event timestamp")
		}
		snapshot.Events = append(snapshot.Events, eventport.AdminReadEvent{
			EventID: eventport.EventID(event.ID), EventType: event.EventType,
			OccurredAt: event.OccurredAt.Time, Dispatched: event.Dispatched,
		})
	}
	for _, delivery := range deliveries {
		var completedAt *time.Time
		if delivery.CompletedAt.Valid {
			value := delivery.CompletedAt.Time
			completedAt = &value
		}
		snapshot.Deliveries = append(snapshot.Deliveries, eventport.AdminReadDelivery{
			EventID: eventport.EventID(delivery.EventID), Consumer: delivery.Consumer, Status: delivery.Status,
			AttemptCount: delivery.AttemptCount, CompletedAt: completedAt,
		})
	}
	return snapshot, nil
}

var _ eventport.AdminReadRepository = (*AdminReadRepository)(nil)
