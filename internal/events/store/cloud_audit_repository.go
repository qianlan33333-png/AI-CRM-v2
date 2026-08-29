package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventdb "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store/generated"
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
	rows, err := eventdb.New(tx).ListCloudAuditFacts(ctx, eventdb.ListCloudAuditFactsParams{
		TraceID: filter.TraceID, SessionID: filter.SessionID, RowLimit: filter.Limit,
	})
	if err != nil {
		return nil, err
	}
	result := make([]eventport.CloudAuditFact, len(rows))
	for index, row := range rows {
		if !row.OccurredAt.Valid {
			return nil, errors.New("cloud audit event timestamp unavailable")
		}
		result[index] = eventport.CloudAuditFact{
			EventID: eventport.EventID(row.ID), EventType: row.EventType, OccurredAt: row.OccurredAt.Time.UTC(), Dispatched: row.Dispatched,
			Pending: row.Pending, Processing: row.Processing, Completed: row.Completed,
			FinalFailed: row.FinalFailed, OutcomeUnknown: row.OutcomeUnknown,
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	committed = true
	return result, nil
}

var _ eventport.CloudAuditRepository = (*CloudAuditRepository)(nil)
