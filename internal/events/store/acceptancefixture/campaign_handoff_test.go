package acceptancefixture

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

type campaignHandoffValidationTx struct{ pgx.Tx }

func TestCampaignHandoffFactFixtureRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	validTx := &campaignHandoffValidationTx{}
	appenders := []func(context.Context, pgx.Tx, int64, string) (int64, error){
		AppendCampaignHandoffAcceptedFact,
		AppendCampaignHandoffAcceptedFactWithForbiddenExtraKey,
	}
	for _, appendFact := range appenders {
		for _, testCase := range []struct {
			tx        pgx.Tx
			handoffID int64
			planID    string
		}{
			{handoffID: 1, planID: "ctp_valid"},
			{tx: validTx, planID: "ctp_valid"},
			{tx: validTx, handoffID: 1},
		} {
			if _, err := appendFact(context.Background(), testCase.tx, testCase.handoffID, testCase.planID); err == nil {
				t.Fatal("invalid Campaign handoff fact was accepted")
			}
		}
	}
}

func TestCampaignHandoffFactCleanupRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	if err := DeleteCampaignHandoffFacts(context.Background(), nil, []int64{1}); err == nil {
		t.Fatal("nil Events fixture pool was accepted")
	}
	for _, eventIDs := range [][]int64{{0}, {1, 1}} {
		if err := validCampaignHandoffEventIDs(eventIDs); err == nil {
			t.Fatalf("invalid event IDs accepted: %#v", eventIDs)
		}
	}
}
