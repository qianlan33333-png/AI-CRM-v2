package v1broadcasthistory

import (
	"context"
	"errors"
	"flag"
	"sort"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var broadcastHistoryArchivePreflight = flag.Bool("broadcast-history-archive-preflight", false, "read sealed V1 broadcast history from V2 archive without writes")

var errBroadcastHistoryArchiveRow = errors.New("invalid broadcast history archive row")

func TestBroadcastHistoryArchivePreflight(t *testing.T) {
	if !*broadcastHistoryArchivePreflight {
		t.Skip("set -broadcast-history-archive-preflight for sealed archive read-only preflight")
	}
	ctx := context.Background()
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("broadcast_history_archive_open_failed")
	}
	defer archive.Close()
	plans := readBroadcastHistoryRows(t, ctx, archive, PlansTableID, 551)
	recipients := readBroadcastHistoryRows(t, ctx, archive, RecipientsTableID, 5442)
	messages := readBroadcastHistoryRows(t, ctx, archive, MessagesTableID, 5442)
	history := AdaptHistory(plans, recipients, messages)
	report := broadcastHistoryReport(history)
	if report.Candidates+report.Quarantined != 11435 {
		t.Fatal("broadcast_history_preflight_row_conservation_failed")
	}
	for _, reason := range report.SortedReasons() {
		t.Logf("broadcast_history_preflight reason=%s count=%d", reason, report.Reasons[reason])
	}
	t.Logf("broadcast_history_preflight plans=%d recipients=%d messages=%d candidates=%d quarantined=%d writes=0 provider_effects=0", len(plans), len(recipients), len(messages), report.Candidates, report.Quarantined)
}

func readBroadcastHistoryRows(t *testing.T, ctx context.Context, archive *v1archive.PostgresArchiveReader, tableID string, expected int) []v1archive.ArchivedRow {
	t.Helper()
	rows := make([]v1archive.ArchivedRow, 0, expected)
	err := archive.EachTableRow(ctx, FixedBroadcastHistoryArchiveRunID, tableID, func(row v1archive.ArchivedRow) error {
		if row.TableID != tableID || row.SourceOrdinal != int64(len(rows)+1) {
			return errBroadcastHistoryArchiveRow
		}
		rows = append(rows, row)
		return nil
	})
	if err != nil || len(rows) != expected {
		t.Fatal("broadcast_history_archive_count_or_ordinal_invalid")
	}
	return rows
}

const FixedBroadcastHistoryArchiveRunID = "v1-full-archive-20260827"

type PreflightReport struct {
	Candidates, Quarantined int
	Reasons                 map[string]int
}

func (report PreflightReport) SortedReasons() []string {
	keys := make([]string, 0, len(report.Reasons))
	for reason := range report.Reasons {
		keys = append(keys, reason)
	}
	sort.Strings(keys)
	return keys
}

func broadcastHistoryReport(history History) PreflightReport {
	report := PreflightReport{Reasons: map[string]int{}}
	for _, result := range history.Plans {
		report.record(result.Disposition, result.Reason)
	}
	for _, result := range history.Recipients {
		report.record(result.Disposition, result.Reason)
	}
	for _, result := range history.Messages {
		report.record(result.Disposition, result.Reason)
	}
	return report
}

func (report *PreflightReport) record(disposition Disposition, reason string) {
	switch disposition {
	case DispositionCandidate:
		report.Candidates++
	case DispositionQuarantine:
		report.Quarantined++
	}
	if reason != "" {
		report.Reasons[reason]++
	}
}
