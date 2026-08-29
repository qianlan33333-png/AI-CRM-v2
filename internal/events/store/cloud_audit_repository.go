package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type CloudAuditRepository struct{ db adminReadTxBeginner }

func NewCloudAuditRepository(db adminReadTxBeginner) *CloudAuditRepository {
	return &CloudAuditRepository{db: db}
}

func (repository *CloudAuditRepository) ListCloudAudit(ctx context.Context, filter eventport.CloudAuditFilter) ([]eventport.CloudAuditFact, error) {
	if repository == nil || repository.db == nil || ctx == nil || ctx.Err() != nil || filter.Limit < 1 || filter.Limit > 100 || filter.TraceID == "" && filter.SessionID == "" {
		return nil, errors.New("cloud audit repository unavailable")
	}
	tx, err := repository.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	rows, err := tx.Query(ctx, `
SELECT event.id, event.event_type, event.occurred_at, event.dispatched,
       count(delivery.event_id) FILTER (WHERE delivery.status = 'pending')::bigint,
       count(delivery.event_id) FILTER (WHERE delivery.status = 'processing')::bigint,
       count(delivery.event_id) FILTER (WHERE delivery.status = 'completed')::bigint,
       count(delivery.event_id) FILTER (WHERE delivery.status = 'final_failed')::bigint,
       count(delivery.event_id) FILTER (WHERE delivery.status = 'outcome_unknown')::bigint
FROM public.event_log AS event
LEFT JOIN public.event_deliveries AS delivery ON delivery.event_id = event.id
WHERE ($1::text = '' OR event.payload ->> 'trace_id' = $1::text)
  AND ($2::text = '' OR event.payload ->> 'session_id' = $2::text)
GROUP BY event.id, event.event_type, event.occurred_at, event.dispatched
ORDER BY event.occurred_at DESC, event.id DESC
LIMIT $3`, filter.TraceID, filter.SessionID, filter.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]eventport.CloudAuditFact, 0)
	for rows.Next() {
		var item eventport.CloudAuditFact
		if err = rows.Scan(&item.EventID, &item.EventType, &item.OccurredAt, &item.Dispatched, &item.Pending, &item.Processing, &item.Completed, &item.FinalFailed, &item.OutcomeUnknown); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	committed = true
	return result, nil
}

var _ eventport.CloudAuditRepository = (*CloudAuditRepository)(nil)
