package v1domain

import (
	"context"
	"flag"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

var wecomHistoryReconcileDSN = flag.String("wecom-history-reconcile-postgres-dsn", "", "isolated PostgreSQL containing imported WeCom history")
var wecomHistoryReconcileRun = flag.String("wecom-history-reconcile-run", "", "frozen archive run for full WeCom reconciliation")

func TestWeComHistoryPostgresReconciliation(t *testing.T) {
	if *wecomHistoryReconcileDSN == "" || *wecomHistoryReconcileRun == "" {
		t.Skip("supply isolated WeCom reconciliation DSN and run")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *wecomHistoryReconcileDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	result, err := ReconcileWeComContactHistory(ctx, pool, "v1-wecom-contact-history-a1", *wecomHistoryReconcileRun)
	if err != nil {
		t.Fatal(err)
	}
	if result.SelectedSourceCount == 0 || result.VerifiedCount != result.SelectedSourceCount || result.QuarantinedCount != 0 {
		t.Fatalf("incomplete WeCom reconciliation: %+v", result)
	}
}
