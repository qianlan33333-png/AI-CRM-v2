package v1marketingstatehistory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"strings"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var marketingStateHistoryArchiveRun = flag.String("marketing-state-history-archive-run", "", "optional reconciled V2 archive run for read-only marketing-state history preflight")

type marketingStateArchiveSummary struct {
	Table          string         `json:"table"`
	Rows           int            `json:"rows"`
	Candidates     int            `json:"candidates"`
	Quarantined    int            `json:"quarantined"`
	Reasons        map[string]int `json:"reasons"`
	RedactionRoots map[string]int `json:"redaction_roots"`
}

type marketingStateArchiveTable struct {
	table    string
	expected int
}

// TestReconciledMarketingStateHistoryArchivePreflight is opt-in and read-only.
// It streams the sealed V2 archive only; it does not resolve a customer or
// write any current state, journal receipt, queue, or Provider effect.
func TestReconciledMarketingStateHistoryArchivePreflight(t *testing.T) {
	if *marketingStateHistoryArchiveRun == "" {
		t.Skip("supply -marketing-state-history-archive-run and V2 archive environment for read-only marketing-state history preflight")
	}
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	sourceHMACKey := []byte(environment.SourceHMACKey)
	if len(sourceHMACKey) < sha256.Size {
		t.Fatal("marketing_state_history_source_hmac_key_invalid")
	}
	ctx := context.Background()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("marketing_state_history_archive_open_failed")
	}
	defer archive.Close()

	tables := []marketingStateArchiveTable{
		{table: MarketingStateCurrentTableID, expected: 77},
		{table: MarketingStateHistoryTableID, expected: 224},
		{table: ValueSegmentCurrentTableID, expected: 342},
		{table: ValueSegmentHistoryTableID, expected: 344},
	}
	rows := make(map[string][]v1archive.ArchivedRow, len(tables))
	summaries := make(map[string]marketingStateArchiveSummary, len(tables))
	for _, table := range tables {
		values, summary, readErr := readMarketingStateHistoryArchiveTable(ctx, archive, *marketingStateHistoryArchiveRun, sourceHMACKey, table)
		if readErr != nil {
			t.Fatal("marketing_state_history_archive_preflight_failed")
		}
		if summary.Rows != table.expected {
			t.Fatal("marketing_state_history_archive_count_mismatch")
		}
		rows[table.table], summaries[table.table] = values, summary
	}

	history := AdaptHistory(rows[MarketingStateCurrentTableID], rows[MarketingStateHistoryTableID], rows[ValueSegmentCurrentTableID], rows[ValueSegmentHistoryTableID])
	currentSummary := summaries[MarketingStateCurrentTableID]
	addMarketingStateResults(&currentSummary, history.MarketingStateCurrent)
	summaries[MarketingStateCurrentTableID] = currentSummary
	historySummary := summaries[MarketingStateHistoryTableID]
	addMarketingStateResults(&historySummary, history.MarketingStateHistory)
	summaries[MarketingStateHistoryTableID] = historySummary
	valueCurrentSummary := summaries[ValueSegmentCurrentTableID]
	addMarketingStateResults(&valueCurrentSummary, history.ValueSegmentCurrent)
	summaries[ValueSegmentCurrentTableID] = valueCurrentSummary
	valueHistorySummary := summaries[ValueSegmentHistoryTableID]
	addMarketingStateResults(&valueHistorySummary, history.ValueSegmentHistory)
	summaries[ValueSegmentHistoryTableID] = valueHistorySummary

	totalRows, totalCandidates, totalQuarantined := 0, 0, 0
	for _, table := range tables {
		summary := summaries[table.table]
		if summary.Rows != summary.Candidates+summary.Quarantined {
			t.Fatal("marketing_state_history_archive_conservation_failed")
		}
		encoded, marshalErr := json.Marshal(summary)
		if marshalErr != nil {
			t.Fatal("marketing_state_history_archive_summary_encode_failed")
		}
		t.Logf("marketing_state_history_archive_preflight=%s", encoded)
		totalRows += summary.Rows
		totalCandidates += summary.Candidates
		totalQuarantined += summary.Quarantined
	}
	if totalRows != 987 || totalRows != totalCandidates+totalQuarantined || history.SourceCount() != 987 || history.TerminalCount() != 987 {
		t.Fatal("marketing_state_history_archive_total_conservation_failed")
	}
	t.Logf("marketing_state_history_archive_total rows=%d candidates=%d quarantined=%d customer_mappings=0 target_writes=0 current_state_writes=0 journal_writes=0", totalRows, totalCandidates, totalQuarantined)
}

func readMarketingStateHistoryArchiveTable(ctx context.Context, archive *v1archive.PostgresArchiveReader, runID string, sourceHMACKey []byte, table marketingStateArchiveTable) ([]v1archive.ArchivedRow, marketingStateArchiveSummary, error) {
	rows := make([]v1archive.ArchivedRow, 0, table.expected)
	summary := marketingStateArchiveSummary{Table: table.table, Reasons: map[string]int{}, RedactionRoots: map[string]int{}}
	seenSourceKeys := map[[sha256.Size]byte]struct{}{}
	reason := ""
	err := archive.EachTableRow(ctx, runID, table.table, func(row v1archive.ArchivedRow) error {
		reason = marketingStateHistoryArchiveRowReason(row, table.table, int64(summary.Rows+1), sourceHMACKey)
		if reason == "" {
			if _, found := seenSourceKeys[row.SourceKeyHMAC]; found {
				reason = "marketing_state_history_archive_duplicate_source_key"
			}
		}
		if reason != "" {
			return errors.New(reason)
		}
		seenSourceKeys[row.SourceKeyHMAC] = struct{}{}
		summary.Rows++
		for _, path := range row.RedactedFields {
			summary.RedactionRoots[marketingStateHistoryRedactionRoot(path)]++
		}
		rows = append(rows, row)
		return nil
	})
	if err != nil {
		return nil, marketingStateArchiveSummary{}, err
	}
	return rows, summary, nil
}

func marketingStateHistoryArchiveRowReason(row v1archive.ArchivedRow, table string, ordinal int64, sourceHMACKey []byte) string {
	if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != ordinal || row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) || !json.Valid(row.Payload) {
		return "marketing_state_history_archive_identity_invalid"
	}
	payloadDigest, err := v1archive.PayloadHMAC(sourceHMACKey, strings.TrimPrefix(table, "public/"), row.Payload)
	if err != nil || payloadDigest != row.PayloadHMAC {
		return "marketing_state_history_archive_payload_hmac_invalid"
	}
	fieldDigest, err := v1archive.FieldHMAC(sourceHMACKey, strings.TrimPrefix(table, "public/"), row.RedactedFields)
	if err != nil || fieldDigest != row.FieldHMAC {
		return "marketing_state_history_archive_field_hmac_invalid"
	}
	// ArchivedRow intentionally exposes no source-key JSON. Its non-zero,
	// unique HMAC is authenticated by record AAD during archive decryption; a
	// preflight must not fabricate a V1 customer or source identifier to repeat it.
	return ""
}

func addMarketingStateResults[T any](summary *marketingStateArchiveSummary, values []Result[T]) {
	for _, value := range values {
		switch value.Disposition {
		case DispositionCandidate:
			if value.Fact == nil || value.Reason != "" {
				summary.Reasons["marketing_state_history_result_invalid"]++
				continue
			}
			summary.Candidates++
		case DispositionQuarantine:
			if value.Fact != nil || value.Reason == "" {
				summary.Reasons["marketing_state_history_result_invalid"]++
				continue
			}
			summary.Quarantined++
			summary.Reasons[value.Reason]++
		default:
			summary.Reasons["marketing_state_history_result_invalid"]++
		}
	}
}

func marketingStateHistoryRedactionRoot(path string) string {
	root := path
	if index := strings.IndexAny(root, ".["); index >= 0 {
		root = root[:index]
	}
	if root == "" {
		return "invalid"
	}
	return root
}

func TestMarketingStateHistoryRedactionRoot(t *testing.T) {
	for path, want := range map[string]string{
		"token": "token", "state_payload_json.items[0]": "state_payload_json", "nested.secret": "nested", "": "invalid",
	} {
		if got := marketingStateHistoryRedactionRoot(path); got != want {
			t.Fatalf("redaction root mismatch path=%q got=%q", path, got)
		}
	}
}
