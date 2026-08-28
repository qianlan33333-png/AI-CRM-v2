package v1broadcastjobhistory

import (
	"context"
	"flag"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var broadcastJobHistoryArchiveRun = flag.String("broadcast-job-history-archive-run", "", "optional reconciled V2 archive run for read-only broadcast job history validation")

// TestReconciledBroadcastJobHistoryArchivePreflight is opt-in and reads only
// the sealed V2 archive. It explicitly rejects a V1 source DSN and reports
// aggregate counts/root names only, never a target, source payload, ID, or
// execution state value.
func TestReconciledBroadcastJobHistoryArchivePreflight(t *testing.T) {
	if *broadcastJobHistoryArchiveRun == "" {
		t.Skip("supply -broadcast-job-history-archive-run and V2 archive environment for read-only source validation")
	}
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	if environment.SourceDatabaseURL != "" {
		t.Fatal("v1_source_database_url_forbidden")
	}
	ctx := context.Background()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("broadcast_job_history_archive_open_failed")
	}
	defer archive.Close()
	report, err := Preflight(ctx, archive, *broadcastJobHistoryArchiveRun)
	if err != nil {
		t.Fatal("broadcast_job_history_archive_read_failed")
	}
	if report.SourceRows != 10853 || report.Candidates+report.Quarantined != 10853 {
		t.Fatalf("broadcast_job_history_count_or_conservation_mismatch source_rows=%d candidates=%d quarantined=%d", report.SourceRows, report.Candidates, report.Quarantined)
	}
	t.Logf("broadcast_job_history_preflight source_rows=%d candidates=%d quarantined=%d reasons=%v redacted_roots=%v target_writes=0 tasks_created=0 customer_links=0 provider_calls=0", report.SourceRows, report.Candidates, report.Quarantined, report.Reasons, report.RedactedRoots)
	if report.Quarantined > 0 {
		t.Errorf("broadcast_job_history_rows_quarantined count=%d", report.Quarantined)
	}
}
