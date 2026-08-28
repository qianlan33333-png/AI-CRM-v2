package v1membergridhistory

import (
	"context"
	"flag"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var memberGridHistoryArchiveRun = flag.String("membergrid-history-archive-run", "", "optional reconciled V2 archive run for read-only Member Grid history validation")

func TestReconciledMemberGridHistoryArchivePreflightWithoutWrites(t *testing.T) {
	if *memberGridHistoryArchiveRun == "" {
		t.Skip("supply -membergrid-history-archive-run and V2 archive environment for read-only validation")
	}
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	ctx := context.Background()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("membergrid_history_archive_open_failed")
	}
	defer archive.Close()
	report, err := Preflight(ctx, archive, *memberGridHistoryArchiveRun)
	if err != nil {
		t.Fatal("membergrid_history_preflight_failed")
	}
	if report.MemberViewRows != 2 || report.UsageSnapshotRows != 2534 || report.UsageSyncRunRows != 2 || report.CollaboratorRows != 1 || report.ShareRows != 2 {
		t.Fatalf("membergrid_history_archive_count_mismatch views=%d usage=%d sync=%d collaborators=%d shares=%d", report.MemberViewRows, report.UsageSnapshotRows, report.UsageSyncRunRows, report.CollaboratorRows, report.ShareRows)
	}
	if report.Candidates+report.Archived+report.Quarantined != 2541 {
		t.Fatal("membergrid_history_row_conservation_failed")
	}
	for _, reason := range report.SortedReasons() {
		t.Logf("reason=%s count=%d", reason, report.Reasons[reason])
	}
	t.Logf("read_only_preflight views=%d usage=%d sync=%d collaborators=%d shares=%d candidates=%d archived=%d quarantined=%d target_writes=0", report.MemberViewRows, report.UsageSnapshotRows, report.UsageSyncRunRows, report.CollaboratorRows, report.ShareRows, report.Candidates, report.Archived, report.Quarantined)
}
