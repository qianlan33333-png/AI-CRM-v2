package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaigndb "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/store/generated"
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
	queries := campaigndb.New(tx)
	if err = queries.InsertHistoricalCampaignDefinition(ctx, campaigndb.InsertHistoricalCampaignDefinitionParams{
		CampaignCode: value.Code, Name: value.Name, CreatedBy: value.CreatedBy, UpdatedBy: value.UpdatedBy,
		CreatedAt: pgtype.Timestamptz{Time: value.CreatedAt.UTC(), Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: value.UpdatedAt.UTC(), Valid: true},
	}); err != nil {
		return historicalConflict(err)
	}
	for _, step := range definition.Steps {
		if err = queries.InsertHistoricalCampaignStep(ctx, campaigndb.InsertHistoricalCampaignStepParams{
			CampaignCode: value.Code, StepIndex: step.Index, DelayMinutes: step.DelayMinutes, Content: step.Content,
		}); err != nil {
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
