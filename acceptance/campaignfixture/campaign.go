// Package campaignfixture creates Campaign-owned rows for acceptance tests.
package campaignfixture

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidDraftCampaignFixture = errors.New("invalid draft campaign fixture")

func CreateDraftCampaign(ctx context.Context, pool *pgxpool.Pool, code, content string, actorID int64, at time.Time) error {
	if ctx == nil || pool == nil || code == "" || content == "" || actorID < 1 || at.IsZero() {
		return ErrInvalidDraftCampaignFixture
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `INSERT INTO public.cloud_campaigns (campaign_code,name,approval_status,runtime_status,version,created_by,updated_by,created_at,updated_at)
VALUES ($1,$1,'draft','idle',1,$2,$2,$3,$3)`, code, actorID, at); err != nil {
		return fmt.Errorf("create campaign-owned acceptance draft: %w", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO public.cloud_campaign_steps (campaign_code,step_index,delay_minutes,content) VALUES ($1,1,0,$2)`, code, content); err != nil {
		return fmt.Errorf("create campaign-owned acceptance draft step: %w", err)
	}
	return tx.Commit(ctx)
}
