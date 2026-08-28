package v1legacymarketingcurrent

import (
	"bytes"
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

var legacyMarketingCurrentArchiveRun = flag.String("legacy-marketing-current-archive-run", "", "optional reconciled V2 archive run for read-only legacy marketing current preflight")

type legacyMarketingArchiveTable struct {
	table    string
	expected int
}

type legacyMarketingArchiveSummary struct {
	Table          string         `json:"table"`
	Rows           int            `json:"rows"`
	Candidates     int            `json:"candidates"`
	Quarantined    int            `json:"quarantined"`
	Reasons        map[string]int `json:"reasons"`
	RedactionRoots map[string]int `json:"redaction_roots"`
	FieldShapes    map[string]int `json:"field_shapes"`
}

// TestReconciledLegacyMarketingCurrentArchivePreflight streams only the
// sealed V2 archive. A configured V1 source DSN is rejected before opening
// anything, and this test never resolves an external user or writes a target.
func TestReconciledLegacyMarketingCurrentArchivePreflight(t *testing.T) {
	if *legacyMarketingCurrentArchiveRun == "" {
		t.Skip("supply -legacy-marketing-current-archive-run and V2 archive environment for read-only legacy marketing preflight")
	}
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	if environment.SourceDatabaseURL != "" {
		t.Fatal("legacy_marketing_current_v1_source_dsn_configured")
	}
	sourceHMACKey := []byte(environment.SourceHMACKey)
	if len(sourceHMACKey) < sha256.Size {
		t.Fatal("legacy_marketing_current_source_hmac_key_invalid")
	}
	archive, err := v1archive.OpenPostgresArchiveReader(context.Background(), environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("legacy_marketing_current_archive_open_failed")
	}
	defer archive.Close()

	tables := []legacyMarketingArchiveTable{
		{table: MarketingStateCurrentTableID, expected: 13},
		{table: MarketingValueSegmentCurrentTableID, expected: 11},
	}
	rows := make(map[string][]v1archive.ArchivedRow, len(tables))
	summaries := make(map[string]legacyMarketingArchiveSummary, len(tables))
	for _, table := range tables {
		values, summary, readErr := readLegacyMarketingArchiveTable(context.Background(), archive, *legacyMarketingCurrentArchiveRun, sourceHMACKey, table)
		if readErr != nil {
			t.Fatal("legacy_marketing_current_archive_preflight_failed")
		}
		if summary.Rows != table.expected {
			t.Fatal("legacy_marketing_current_archive_count_mismatch")
		}
		rows[table.table], summaries[table.table] = values, summary
	}

	history := AdaptHistory(rows[MarketingStateCurrentTableID], rows[MarketingValueSegmentCurrentTableID])
	stateSummary := summaries[MarketingStateCurrentTableID]
	addLegacyMarketingResults(&stateSummary, history.MarketingStateCurrent)
	summaries[MarketingStateCurrentTableID] = stateSummary
	valueSummary := summaries[MarketingValueSegmentCurrentTableID]
	addLegacyMarketingResults(&valueSummary, history.MarketingValueSegmentCurrent)
	summaries[MarketingValueSegmentCurrentTableID] = valueSummary

	totalRows, totalCandidates, totalQuarantined := 0, 0, 0
	for _, table := range tables {
		summary := summaries[table.table]
		if summary.Rows != summary.Candidates+summary.Quarantined {
			t.Fatal("legacy_marketing_current_archive_conservation_failed")
		}
		encoded, marshalErr := json.Marshal(summary)
		if marshalErr != nil {
			t.Fatal("legacy_marketing_current_archive_summary_encode_failed")
		}
		t.Logf("legacy_marketing_current_archive_preflight=%s", encoded)
		totalRows += summary.Rows
		totalCandidates += summary.Candidates
		totalQuarantined += summary.Quarantined
	}
	if totalRows != 24 || totalRows != totalCandidates+totalQuarantined || history.SourceCount() != 24 || history.TerminalCount() != 24 {
		t.Fatal("legacy_marketing_current_archive_total_conservation_failed")
	}
	t.Logf("legacy_marketing_current_archive_total rows=%d candidates=%d quarantined=%d customer_mappings=0 target_writes=0 active_states=0 events=0 queue_jobs=0 provider_calls=0", totalRows, totalCandidates, totalQuarantined)
}

func readLegacyMarketingArchiveTable(ctx context.Context, archive *v1archive.PostgresArchiveReader, runID string, sourceHMACKey []byte, table legacyMarketingArchiveTable) ([]v1archive.ArchivedRow, legacyMarketingArchiveSummary, error) {
	rows := make([]v1archive.ArchivedRow, 0, table.expected)
	summary := legacyMarketingArchiveSummary{Table: table.table, Reasons: map[string]int{}, RedactionRoots: map[string]int{}, FieldShapes: map[string]int{}}
	seenSourceKeys := map[[sha256.Size]byte]struct{}{}
	var reason string
	err := archive.EachTableRow(ctx, runID, table.table, func(row v1archive.ArchivedRow) error {
		reason = legacyMarketingArchiveRowReason(row, table.table, int64(summary.Rows+1), sourceHMACKey)
		if reason == "" {
			if _, duplicate := seenSourceKeys[row.SourceKeyHMAC]; duplicate {
				reason = "legacy_marketing_current_archive_duplicate_source_key"
			}
		}
		if reason != "" {
			return errors.New(reason)
		}
		seenSourceKeys[row.SourceKeyHMAC] = struct{}{}
		summary.Rows++
		for _, path := range row.RedactedFields {
			summary.RedactionRoots[legacyMarketingRedactionRoot(table.table, path)]++
		}
		addLegacyMarketingFieldShapes(summary.FieldShapes, table.table, row.Payload)
		rows = append(rows, row)
		return nil
	})
	if err != nil {
		return nil, legacyMarketingArchiveSummary{}, err
	}
	return rows, summary, nil
}

func legacyMarketingArchiveRowReason(row v1archive.ArchivedRow, table string, ordinal int64, sourceHMACKey []byte) string {
	if (table != MarketingStateCurrentTableID && table != MarketingValueSegmentCurrentTableID) || row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != ordinal ||
		row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) || !json.Valid(row.Payload) {
		return "legacy_marketing_current_archive_identity_invalid"
	}
	payloadDigest, err := v1archive.PayloadHMAC(sourceHMACKey, strings.TrimPrefix(table, "public/"), row.Payload)
	if err != nil || payloadDigest != row.PayloadHMAC {
		return "legacy_marketing_current_archive_payload_hmac_invalid"
	}
	fieldDigest, err := v1archive.FieldHMAC(sourceHMACKey, strings.TrimPrefix(table, "public/"), row.RedactedFields)
	if err != nil || fieldDigest != row.FieldHMAC {
		return "legacy_marketing_current_archive_field_hmac_invalid"
	}
	return ""
}

func addLegacyMarketingResults[T any](summary *legacyMarketingArchiveSummary, rows []Result[T]) {
	for _, row := range rows {
		switch row.Disposition {
		case DispositionCandidate:
			if row.Fact == nil || row.Reason != "" {
				summary.Reasons["legacy_marketing_current_result_invalid"]++
				continue
			}
			summary.Candidates++
		case DispositionQuarantine:
			if row.Fact != nil || row.Reason == "" {
				summary.Reasons["legacy_marketing_current_result_invalid"]++
				continue
			}
			summary.Quarantined++
			summary.Reasons[row.Reason]++
		default:
			summary.Reasons["legacy_marketing_current_result_invalid"]++
		}
	}
}

func addLegacyMarketingFieldShapes(counts map[string]int, table string, payload []byte) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(payload, &fields) != nil || fields == nil {
		counts["row:not_object"]++
		return
	}
	if table == MarketingStateCurrentTableID {
		if len(fields) == len(marketingStateCurrentFields) {
			counts["row:expected_columns"]++
		} else {
			counts["row:unexpected_columns"]++
		}
		counts["source_payload_json:"+legacyMarketingJSONShape(fields["source_payload_json"])]++
		return
	}
	if len(fields) == len(marketingValueSegmentCurrentFields) {
		counts["row:expected_columns"]++
	} else {
		counts["row:unexpected_columns"]++
	}
	counts["score_breakdown_json:"+legacyMarketingJSONShape(fields["score_breakdown_json"])]++
	counts["source_payload_json:"+legacyMarketingJSONShape(fields["source_payload_json"])]++
}

func legacyMarketingJSONShape(value json.RawMessage) string {
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 {
		return "missing"
	}
	switch trimmed[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "bool"
	case 'n':
		return "null"
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return "number"
	default:
		return "invalid"
	}
}

func legacyMarketingRedactionRoot(table, path string) string {
	root := path
	if index := strings.IndexAny(root, ".["); index >= 0 {
		root = root[:index]
	}
	for _, field := range legacyMarketingFields(table) {
		if root == field {
			return root
		}
	}
	return "unknown"
}

func legacyMarketingFields(table string) []string {
	if table == MarketingStateCurrentTableID {
		return marketingStateCurrentFields
	}
	return marketingValueSegmentCurrentFields
}

func TestLegacyMarketingRedactionRootAndJSONShapeNeverUseSourceValues(t *testing.T) {
	for _, test := range []struct {
		table, path, want string
	}{
		{MarketingStateCurrentTableID, "source_payload_json.secret", "source_payload_json"},
		{MarketingValueSegmentCurrentTableID, "score_breakdown_json[0]", "score_breakdown_json"},
		{MarketingStateCurrentTableID, "unknown.dynamic", "unknown"},
	} {
		if got := legacyMarketingRedactionRoot(test.table, test.path); got != test.want {
			t.Fatalf("redaction root mismatch got=%q want=%q", got, test.want)
		}
	}
	for value, want := range map[string]string{
		`{}`: "object", `[]`: "array", `"x"`: "string", `false`: "bool", `-1`: "number", `null`: "null", ``: "missing",
	} {
		if got := legacyMarketingJSONShape([]byte(value)); got != want {
			t.Fatalf("shape mismatch got=%q want=%q", got, want)
		}
	}
}

func TestLegacyMarketingArchiveRowReasonValidatesArchiveHMACs(t *testing.T) {
	key := bytes.Repeat([]byte{8}, sha256.Size)
	payload := []byte(`{"id":1}`)
	payloadHMAC, err := v1archive.PayloadHMAC(key, "marketing_state_current", payload)
	if err != nil {
		t.Fatal(err)
	}
	fieldHMAC, err := v1archive.FieldHMAC(key, "marketing_state_current", nil)
	if err != nil {
		t.Fatal(err)
	}
	row := v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: MarketingStateCurrentTableID, SourceOrdinal: 1,
		SourceKeyHMAC: sha256.Sum256([]byte("source")), PayloadHMAC: payloadHMAC, FieldHMAC: fieldHMAC, Payload: payload}
	if got := legacyMarketingArchiveRowReason(row, MarketingStateCurrentTableID, 1, key); got != "" {
		t.Fatalf("valid archive HMAC rejected: %s", got)
	}
	row.FieldHMAC = [sha256.Size]byte{}
	if legacyMarketingArchiveRowReason(row, MarketingStateCurrentTableID, 1, key) == "" {
		t.Fatal("invalid field HMAC accepted")
	}
}
