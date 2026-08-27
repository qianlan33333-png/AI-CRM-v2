package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type HistoricalDefinitionStore struct {
	tx func(context.Context) (pgx.Tx, error)
}

var _ campaign.HistoricalDefinitionStore = (*HistoricalDefinitionStore)(nil)

func NewHistoricalDefinitionStore() *HistoricalDefinitionStore {
	return &HistoricalDefinitionStore{tx: platformstore.TxFromContext}
}

// InsertHistoricalDefinition only inserts a rejected+paused historical
// definition and its steps through the caller's transaction. It has no path
// to Campaign plans, commands, events, queues, or Providers.
func (store *HistoricalDefinitionStore) InsertHistoricalDefinition(ctx context.Context, definition campaign.HistoricalDefinition) error {
	if store == nil || store.tx == nil || definition.Campaign.ApprovalStatus != campaign.ApprovalRejected || definition.Campaign.RuntimeStatus != campaign.RuntimePaused || definition.Campaign.Version != 1 {
		return campaign.ErrUnavailable
	}
	tx, err := store.tx(ctx)
	if err != nil {
		return err
	}
	value := definition.Campaign
	if _, err = tx.Exec(ctx, `INSERT INTO public.cloud_campaigns (campaign_code,name,approval_status,runtime_status,version,created_by,updated_by,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, value.Code, value.Name, value.ApprovalStatus, value.RuntimeStatus, value.Version, value.CreatedBy, value.UpdatedBy, value.CreatedAt.UTC(), value.UpdatedAt.UTC()); err != nil {
		return historicalConflict(err)
	}
	for _, step := range definition.Steps {
		if _, err = tx.Exec(ctx, `INSERT INTO public.cloud_campaign_steps (campaign_code,step_index,delay_minutes,content) VALUES ($1,$2,$3,$4)`, value.Code, step.Index, step.DelayMinutes, step.Content); err != nil {
			return historicalConflict(err)
		}
	}
	return nil
}

func historicalConflict(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return campaign.ErrHistoricalDefinitionConflict
	}
	return err
}
