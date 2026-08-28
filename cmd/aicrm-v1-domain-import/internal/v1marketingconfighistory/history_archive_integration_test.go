package v1marketingconfighistory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"sort"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var marketingConfigHistoryArchiveRun = flag.String("marketing-config-history-archive-run", "", "optional reconciled V2 archive run for read-only marketing configuration validation")

// TestReconciledMarketingConfigHistoryArchivePreflight is opt-in and read-only.
// It retains no raw configuration/answer JSON and does not construct a V2
// runtime automation, enrollment, event, queue, LLM call, or Provider effect.
func TestReconciledMarketingConfigHistoryArchivePreflight(t *testing.T) {
	if *marketingConfigHistoryArchiveRun == "" {
		t.Skip("supply -marketing-config-history-archive-run and V2 archive environment for read-only marketing configuration validation")
	}
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	archive, err := v1archive.OpenPostgresArchiveReader(context.Background(), environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("marketing_config_history_archive_open_failed")
	}
	defer archive.Close()
	configs, err := readMarketingHistoryTable(context.Background(), archive, *marketingConfigHistoryArchiveRun, ConfigTableID)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := readMarketingHistoryTable(context.Background(), archive, *marketingConfigHistoryArchiveRun, RulesTableID)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 1 || len(rules) != 3 {
		t.Fatalf("marketing_config_history_archive_count_mismatch configs=%d rules=%d", len(configs), len(rules))
	}
	history := AdaptHistory(configs, rules)
	candidates, quarantined, reasons, valid := countMarketingResults(history)
	if !valid || candidates+quarantined != len(configs)+len(rules) {
		t.Fatal("marketing_config_history_candidate_row_conservation_failed")
	}
	keys := make([]string, 0, len(reasons))
	for reason := range reasons {
		keys = append(keys, reason)
	}
	sort.Strings(keys)
	for _, reason := range keys {
		t.Logf("reason=%s count=%d", reason, reasons[reason])
	}
	t.Logf("source_rows=4 candidate_rows=%d quarantine_rows=%d target_writes=0 runtime_automations=0 enrollments=0 events=0 queue_jobs=0 llm_calls=0 provider_calls=0", candidates, quarantined)
	if quarantined > 0 {
		t.Errorf("marketing_config_history_rows_quarantined count=%d", quarantined)
	}
}

func readMarketingHistoryTable(ctx context.Context, archive *v1archive.PostgresArchiveReader, runID, table string) ([]json.RawMessage, error) {
	payloads := make([]json.RawMessage, 0)
	seen := make(map[[sha256.Size]byte]bool)
	reason := ""
	err := archive.EachTableRow(ctx, runID, table, func(row v1archive.ArchivedRow) error {
		reason = marketingArchiveRowReason(row, table, int64(len(payloads)+1))
		if reason == "" && seen[row.SourceKeyHMAC] {
			reason = "marketing_config_history_archive_duplicate_source_key"
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
		return nil, errors.New("marketing_config_history_archive_read_failed")
	}
	return payloads, nil
}

func marketingArchiveRowReason(row v1archive.ArchivedRow, table string, ordinal int64) string {
	if (table != ConfigTableID && table != RulesTableID) || row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != ordinal {
		return "marketing_config_history_archive_scope_or_ordinal_mismatch"
	}
	if row.SourceKeyHMAC == [sha256.Size]byte{} || row.PayloadHMAC == [sha256.Size]byte{} || row.FieldHMAC == [sha256.Size]byte{} || !json.Valid(row.Payload) {
		return "marketing_config_history_archive_integrity_invalid"
	}
	if len(row.RedactedFields) != 0 {
		return "marketing_config_history_archive_redacted"
	}
	return ""
}

func countMarketingResults(history History) (candidates, quarantined int, reasons map[string]int, valid bool) {
	reasons = make(map[string]int)
	countConfigs := func(rows []Result[ConfigFact]) bool {
		for _, row := range rows {
			switch row.Disposition {
			case DispositionCandidate:
				if row.Fact == nil || row.Reason != "" {
					return false
				}
				candidates++
			case DispositionQuarantine:
				if row.Fact != nil || row.Reason == "" {
					return false
				}
				quarantined++
				reasons[row.Reason]++
			default:
				return false
			}
		}
		return true
	}
	countRules := func(rows []Result[RuleFact]) bool {
		for _, row := range rows {
			switch row.Disposition {
			case DispositionCandidate:
				if row.Fact == nil || row.Reason != "" {
					return false
				}
				candidates++
			case DispositionQuarantine:
				if row.Fact != nil || row.Reason == "" {
					return false
				}
				quarantined++
				reasons[row.Reason]++
			default:
				return false
			}
		}
		return true
	}
	valid = countConfigs(history.Configs) && countRules(history.Rules)
	return candidates, quarantined, reasons, valid
}

func TestMarketingArchiveEnvelopeAndRedactionFailClosed(t *testing.T) {
	row := v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: ConfigTableID, SourceOrdinal: 1, SourceKeyHMAC: [sha256.Size]byte{1}, PayloadHMAC: [sha256.Size]byte{2}, FieldHMAC: [sha256.Size]byte{3}, Payload: []byte(`{"id":1}`)}
	if marketingArchiveRowReason(row, ConfigTableID, 1) != "" {
		t.Fatal("valid archive envelope rejected")
	}
	for _, mutate := range []func(*v1archive.ArchivedRow){
		func(value *v1archive.ArchivedRow) { value.AdapterID = "wrong" },
		func(value *v1archive.ArchivedRow) { value.TableID = RulesTableID },
		func(value *v1archive.ArchivedRow) { value.SourceOrdinal = 2 },
		func(value *v1archive.ArchivedRow) { value.SourceKeyHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.PayloadHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.FieldHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.Payload = []byte(`{`) },
		func(value *v1archive.ArchivedRow) { value.RedactedFields = []string{"config_payload_json.dynamic"} },
	} {
		changed := row
		mutate(&changed)
		if marketingArchiveRowReason(changed, ConfigTableID, 1) == "" {
			t.Fatal("invalid or redacted archive envelope accepted")
		}
	}
}
