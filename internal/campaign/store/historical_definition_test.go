package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
)

func TestHistoricalDefinitionStoreInsertsOnlyHeaderAndStepsOnCallerTransaction(t *testing.T) {
	tx := &historicalDefinitionTx{}
	store := &HistoricalDefinitionStore{tx: func(context.Context) (pgx.Tx, error) { return tx, nil }}
	stamp := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	definition := campaign.HistoricalDefinition{
		Campaign: campaign.Campaign{Code: "history-campaign", Name: "history", ApprovalStatus: campaign.ApprovalRejected, RuntimeStatus: campaign.RuntimePaused, Version: 1, CreatedBy: 9, UpdatedBy: 9, CreatedAt: stamp, UpdatedAt: stamp},
		Steps:    []campaign.Step{{Index: 1, DelayMinutes: 0, Content: "one"}, {Index: 2, DelayMinutes: 10, Content: "two"}},
	}
	if err := store.InsertHistoricalDefinition(context.Background(), definition); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(tx.calls, ","), "header,step,step"; got != want {
		t.Fatalf("calls = %q, want %q", got, want)
	}
}

func TestHistoricalDefinitionStoreConflictsInsteadOfOverwritingExistingCampaign(t *testing.T) {
	tx := &historicalDefinitionTx{execErr: &pgconn.PgError{Code: "23505"}}
	store := &HistoricalDefinitionStore{tx: func(context.Context) (pgx.Tx, error) { return tx, nil }}
	definition := campaign.HistoricalDefinition{Campaign: campaign.Campaign{Code: "history-campaign", ApprovalStatus: campaign.ApprovalRejected, RuntimeStatus: campaign.RuntimePaused, Version: 1}}
	if err := store.InsertHistoricalDefinition(context.Background(), definition); !errors.Is(err, campaign.ErrHistoricalDefinitionConflict) {
		t.Fatalf("InsertHistoricalDefinition() error = %v", err)
	}
	if got, want := strings.Join(tx.calls, ","), "header"; got != want {
		t.Fatalf("calls = %q, want %q", got, want)
	}
}

type historicalDefinitionTx struct {
	pgx.Tx
	calls   []string
	execErr error
}

func (tx *historicalDefinitionTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	if strings.Contains(sql, "cloud_campaigns") {
		tx.calls = append(tx.calls, "header")
	} else if strings.Contains(sql, "cloud_campaign_steps") {
		tx.calls = append(tx.calls, "step")
	} else {
		tx.calls = append(tx.calls, "forbidden")
	}
	if tx.execErr != nil {
		return pgconn.CommandTag{}, tx.execErr
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}
