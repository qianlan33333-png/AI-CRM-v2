package v1radarhistory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var radarHistoryArchiveRun = flag.String("radar-history-archive-run", "", "optional reconciled V2 archive run for read-only Radar click candidate validation")

// TestReconciledRadarHistoryArchivePreflight is explicitly opt-in. It streams
// one reconciled archive table, opens no target write transaction, and logs
// only aggregate counts and fixed reason codes.
func TestReconciledRadarHistoryArchivePreflight(t *testing.T) {
	if *radarHistoryArchiveRun == "" {
		t.Skip("supply -radar-history-archive-run and V2 archive environment for read-only Radar validation")
	}
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	archive, err := v1archive.OpenPostgresArchiveReader(context.Background(), environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("radar_history_archive_open_failed")
	}
	defer archive.Close()

	payloads, err := readRadarHistoryTable(context.Background(), archive, *radarHistoryArchiveRun)
	if err != nil {
		t.Fatal(err)
	}
	if len(payloads) != 1735 {
		t.Fatalf("radar_history_archive_count_mismatch count=%d", len(payloads))
	}
	results := AdaptClicks(payloads)
	candidates, quarantined, reasons, valid := countRadarResults(results)
	if !valid || candidates+quarantined != len(payloads) {
		t.Fatal("radar_history_candidate_row_conservation_failed")
	}
	for reason, count := range reasons {
		t.Logf("reason=%s count=%d", reason, count)
	}
	t.Logf("table=%s source_rows=%d candidate_rows=%d quarantine_rows=%d target_writes=0 current_radar_events=0 outbox_events=0 provider_calls=0", ClickTableID, len(payloads), candidates, quarantined)
	if quarantined > 0 {
		t.Errorf("radar_history_rows_quarantined count=%d", quarantined)
	}
}

func readRadarHistoryTable(ctx context.Context, archive *v1archive.PostgresArchiveReader, runID string) ([]json.RawMessage, error) {
	payloads := make([]json.RawMessage, 0, 1735)
	seen := make(map[[sha256.Size]byte]bool)
	reason := ""
	err := archive.EachTableRow(ctx, runID, ClickTableID, func(row v1archive.ArchivedRow) error {
		reason = radarArchiveRowReason(row, int64(len(payloads)+1))
		if reason == "" && seen[row.SourceKeyHMAC] {
			reason = "radar_click_archive_duplicate_source_key"
		}
		if reason != "" {
			return errors.New(reason)
		}
		seen[row.SourceKeyHMAC] = true
		payloads = append(payloads, append(json.RawMessage(nil), row.Payload...))
		return nil
	})
	if err != nil {
		if reason != "" {
			return nil, errors.New(reason)
		}
		return nil, errors.New("radar_history_archive_read_failed")
	}
	return payloads, nil
}

func radarArchiveRowReason(row v1archive.ArchivedRow, ordinal int64) string {
	if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != ClickTableID || row.SourceOrdinal != ordinal {
		return "radar_click_archive_scope_or_ordinal_mismatch"
	}
	if row.SourceKeyHMAC == [sha256.Size]byte{} || row.PayloadHMAC == [sha256.Size]byte{} || row.FieldHMAC == [sha256.Size]byte{} || !json.Valid(row.Payload) {
		return "radar_click_archive_integrity_invalid"
	}
	// A digest of a redacted field would not represent the frozen source fact.
	// This private candidate therefore fails closed rather than treating an
	// archive placeholder as source identity or click telemetry.
	if len(row.RedactedFields) != 0 {
		return "radar_click_archive_redacted"
	}
	return ""
}

func countRadarResults(rows []Result) (candidates, quarantined int, reasons map[string]int, valid bool) {
	reasons = make(map[string]int)
	for _, row := range rows {
		switch row.Disposition {
		case DispositionCandidate:
			if row.Fact == nil || row.Reason != "" {
				return 0, 0, nil, false
			}
			candidates++
		case DispositionQuarantine:
			if row.Fact != nil || row.Reason == "" {
				return 0, 0, nil, false
			}
			quarantined++
			reasons[row.Reason]++
		default:
			return 0, 0, nil, false
		}
	}
	return candidates, quarantined, reasons, true
}

func TestRadarArchiveEnvelopeAndRedactionFailClosed(t *testing.T) {
	row := v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: ClickTableID, SourceOrdinal: 1, SourceKeyHMAC: [sha256.Size]byte{1}, PayloadHMAC: [sha256.Size]byte{2}, FieldHMAC: [sha256.Size]byte{3}, Payload: []byte(`{"id":1}`)}
	if radarArchiveRowReason(row, 1) != "" {
		t.Fatal("valid archive envelope rejected")
	}
	for _, mutate := range []func(*v1archive.ArchivedRow){
		func(value *v1archive.ArchivedRow) { value.AdapterID = "wrong" },
		func(value *v1archive.ArchivedRow) { value.TableID = "public/other" },
		func(value *v1archive.ArchivedRow) { value.SourceOrdinal = 2 },
		func(value *v1archive.ArchivedRow) { value.SourceKeyHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.PayloadHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.FieldHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.Payload = []byte(`{`) },
		func(value *v1archive.ArchivedRow) { value.RedactedFields = []string{"unknown.dynamic"} },
	} {
		changed := row
		mutate(&changed)
		if radarArchiveRowReason(changed, 1) == "" {
			t.Fatal("invalid or redacted archive envelope accepted")
		}
	}
}
