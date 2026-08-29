package main

import (
	"context"
	"flag"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var customerTimelineHistoryPostgresDSN = flag.String("customer-timeline-history-postgres-dsn", "", "isolated PostgreSQL DSN containing imported customer timeline history")
var customerTimelineHistoryPostgresArchiveRun = flag.String("customer-timeline-history-postgres-archive-run", "", "reconciled archive run for isolated customer timeline reconciliation")
var customerTimelineHistoryPostgresDM01Run = flag.Int64("customer-timeline-history-postgres-dm01-run", 0, "verified DM01 full-import run for isolated customer timeline reconciliation")

func TestCustomerTimelineHistoryPostgresReconciliation(t *testing.T) {
	if *customerTimelineHistoryPostgresDSN == "" || *customerTimelineHistoryPostgresArchiveRun == "" || *customerTimelineHistoryPostgresDM01Run < 1 {
		t.Skip("supply isolated customer timeline PostgreSQL, archive run, and DM01 run")
	}
	archiveRuntime := appconfig.LoadV1ArchiveRuntimeEnvironment()
	dm01Runtime := appconfig.LoadDM01RuntimeEnvironment()
	if len(archiveRuntime.ArchiveKey) != 32 || len(archiveRuntime.SourceHMACKey) < 32 || len(dm01Runtime.SourceHMACKey) < 32 {
		t.Fatal("invalid isolated customer timeline key inputs")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *customerTimelineHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, *customerTimelineHistoryPostgresDSN, []byte(archiveRuntime.ArchiveKey))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	value, err := importCustomerTimelineHistory(ctx, archive, platformstore.NewUnitOfWork(pool), *customerTimelineHistoryPostgresArchiveRun, *customerTimelineHistoryPostgresDM01Run, []byte(dm01Runtime.SourceHMACKey), []byte(archiveRuntime.SourceHMACKey), true)
	if err != nil {
		t.Fatal(err)
	}
	result, ok := value.(v1domain.CustomerTimelineHistoryReconciliationResult)
	if !ok || result.SelectedSourceCount == 0 || result.ReceiptCount != result.SelectedSourceCount || result.VerifiedCount != result.SelectedSourceCount || result.ImportedCount+result.QuarantinedCount != result.SelectedSourceCount || result.ComparisonDigest == ([32]byte{}) {
		t.Fatalf("incomplete isolated customer timeline reconciliation: %#v", value)
	}
}
