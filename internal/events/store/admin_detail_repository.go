package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventdb "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store/generated"
)

// AdminDetailRepository reads the minimal local projection needed by the
// point-detail route. The transaction is explicitly read-only and never
// touches queue, payload, or external-effect state.
type AdminDetailRepository struct {
	db adminReadTxBeginner
}

func NewAdminDetailRepository(db adminReadTxBeginner) *AdminDetailRepository {
	return &AdminDetailRepository{db: db}
}

func (repository *AdminDetailRepository) Read(ctx context.Context, eventID eventport.EventID) (eventport.AdminDetailSnapshot, error) {
	if repository == nil || repository.db == nil || ctx == nil || eventID <= 0 {
		return eventport.AdminDetailSnapshot{}, errors.New("admin detail repository unavailable")
	}
	tx, err := repository.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return eventport.AdminDetailSnapshot{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()

	queries := eventdb.New(tx)
	event, err := queries.GetAdminDetailEvent(ctx, int64(eventID))
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil {
			return eventport.AdminDetailSnapshot{}, err
		}
		committed = true
		return eventport.AdminDetailSnapshot{Found: false, Deliveries: make([]eventport.AdminReadDelivery, 0)}, nil
	}
	if err != nil {
		return eventport.AdminDetailSnapshot{}, err
	}
	if !event.OccurredAt.Valid {
		return eventport.AdminDetailSnapshot{}, errors.New("invalid event timestamp")
	}
	deliveries, err := queries.ListAdminDetailDeliveries(ctx, int64(eventID))
	if err != nil {
		return eventport.AdminDetailSnapshot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return eventport.AdminDetailSnapshot{}, err
	}
	committed = true

	snapshot := eventport.AdminDetailSnapshot{
		Found: true,
		Event: eventport.AdminReadEvent{
			EventID: eventport.EventID(event.ID), EventType: event.EventType,
			OccurredAt: event.OccurredAt.Time, Dispatched: event.Dispatched,
		},
		Deliveries: make([]eventport.AdminReadDelivery, 0, len(deliveries)),
	}
	for _, delivery := range deliveries {
		var completedAt *time.Time
		if delivery.CompletedAt.Valid {
			value := delivery.CompletedAt.Time
			completedAt = &value
		}
		snapshot.Deliveries = append(snapshot.Deliveries, eventport.AdminReadDelivery{
			EventID: eventport.EventID(delivery.EventID), Consumer: delivery.Consumer,
			Status: delivery.Status, AttemptCount: delivery.AttemptCount, CompletedAt: completedAt,
		})
	}
	return snapshot, nil
}

var _ eventport.AdminDetailRepository = (*AdminDetailRepository)(nil)
