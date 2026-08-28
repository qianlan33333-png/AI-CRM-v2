package v1automationhistory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"sort"
	"strings"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var automationHistoryArchiveRun = flag.String("automation-history-archive-run", "", "optional reconciled V2 archive run for read-only automation history candidate validation")

// TestReconciledAutomationHistoryArchivePreflight is opt-in and read-only. It
// streams the reconciled archive without opening a target write transaction or
// logging source payloads, prompts, identities, or configuration material.
func TestReconciledAutomationHistoryArchivePreflight(t *testing.T) {
	if *automationHistoryArchiveRun == "" {
		t.Skip("supply -automation-history-archive-run and V2 archive environment for read-only automation history preflight")
	}
	ctx := context.Background()
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("automation_history_archive_open_failed")
	}
	defer archive.Close()

	tables := []struct {
		table string
		count int
	}{
		{SOPTemplateTableID, 16},
		{AgentConfigTableID, 12},
		{PromptRegistryTableID, 6},
		{AgentsTableID, 6},
	}
	payloads := make([][]json.RawMessage, len(tables))
	material := make([]automationMaterialCounts, len(tables))
	redactedColumns := make(map[string]int)
	for index, table := range tables {
		payloads[index], material[index], err = readAutomationHistoryTable(ctx, archive, *automationHistoryArchiveRun, table.table, redactedColumns)
		if err != nil {
			t.Fatal(err)
		}
		if len(payloads[index]) != table.count {
			t.Fatalf("automation_history_archive_count_mismatch table_index=%d count=%d", index, len(payloads[index]))
		}
	}

	history := AdaptHistory(payloads[0], payloads[1], payloads[2], payloads[3])
	if len(history.SOPTemplates) != len(payloads[0]) || len(history.AgentConfigs) != len(payloads[1]) || len(history.PromptRegistries) != len(payloads[2]) || len(history.Agents) != len(payloads[3]) {
		t.Fatal("automation_history_candidate_row_conservation_failed")
	}

	counts := make([]automationDispositionCounts, len(tables))
	var valid bool
	if counts[0], valid = countAutomationResults(history.SOPTemplates); !valid {
		t.Fatal("automation_history_candidate_result_invalid")
	}
	if counts[1], valid = countAutomationResults(history.AgentConfigs); !valid {
		t.Fatal("automation_history_candidate_result_invalid")
	}
	if counts[2], valid = countAutomationResults(history.PromptRegistries); !valid {
		t.Fatal("automation_history_candidate_result_invalid")
	}
	if counts[3], valid = countAutomationResults(history.Agents); !valid {
		t.Fatal("automation_history_candidate_result_invalid")
	}

	reasons := make(map[string]int)
	candidates, quarantined := 0, 0
	for index, table := range tables {
		candidates += counts[index].candidate
		quarantined += counts[index].quarantine
		for reason, count := range counts[index].reasons {
			reasons[reason] += count
		}
		t.Logf("table=%s source_rows=%d candidate_rows=%d quarantine_rows=%d original_opaque_summary_rows=%d redacted_placeholder_summary_rows=%d", table.table, len(payloads[index]), counts[index].candidate, counts[index].quarantine, material[index].original, material[index].redactedPlaceholder)
	}
	logAutomationCounts(t, "reason", reasons)
	logAutomationCounts(t, "redacted_column", redactedColumns)
	t.Logf("source_rows=40 candidate_rows=%d quarantine_rows=%d target_writes=0 current_objects=0 llm_calls=0 provider_calls=0", candidates, quarantined)
	if candidates+quarantined != 40 {
		t.Fatal("automation_history_candidate_terminal_count_mismatch")
	}
	if quarantined > 0 {
		t.Errorf("automation_history_rows_quarantined count=%d", quarantined)
	}
}

func readAutomationHistoryTable(ctx context.Context, archive *v1archive.PostgresArchiveReader, runID, table string, redactedColumns map[string]int) ([]json.RawMessage, automationMaterialCounts, error) {
	result := make([]json.RawMessage, 0)
	material := automationMaterialCounts{}
	seen := make(map[[sha256.Size]byte]bool)
	reason := ""
	err := archive.EachTableRow(ctx, runID, table, func(row v1archive.ArchivedRow) error {
		reason = automationArchiveRowReason(row, table, int64(len(result)+1))
		if reason == "" && seen[row.SourceKeyHMAC] {
			reason = "automation_history_archive_duplicate_source_key"
		}
		if reason != "" {
			return errors.New(reason)
		}
		seen[row.SourceKeyHMAC] = true
		if automationMaterialRedacted(table, row.RedactedFields) {
			material.redactedPlaceholder++
		} else {
			material.original++
		}
		for _, column := range row.RedactedFields {
			redactedColumns[table+"."+column]++
		}
		result = append(result, append(json.RawMessage(nil), row.Payload...))
		return nil
	})
	if err != nil {
		if reason != "" {
			return nil, automationMaterialCounts{}, errors.New(reason)
		}
		return nil, automationMaterialCounts{}, errors.New("automation_history_archive_read_failed")
	}
	return result, material, nil
}

func automationArchiveRowReason(row v1archive.ArchivedRow, table string, ordinal int64) string {
	if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != ordinal {
		return "automation_history_archive_scope_or_ordinal_mismatch"
	}
	if row.SourceKeyHMAC == [sha256.Size]byte{} || row.PayloadHMAC == [sha256.Size]byte{} || row.FieldHMAC == [sha256.Size]byte{} || !json.Valid(row.Payload) {
		return "automation_history_archive_integrity_invalid"
	}
	return ""
}

// automationMaterialRedacted marks a digest as a digest of archive-redacted
// material when any field that feeds that fact was replaced with [REDACTED].
// It never treats that digest as a summary of the original source value.
func automationMaterialRedacted(table string, paths []string) bool {
	var roots []string
	switch table {
	case SOPTemplateTableID:
		roots = []string{"images_json"}
	case AgentConfigTableID:
		roots = []string{"pool_keys_json", "draft_role_prompt", "draft_task_prompt", "draft_variables_json", "draft_output_schema_json", "published_role_prompt", "published_task_prompt", "published_variables_json", "published_output_schema_json", "last_change_summary"}
	case PromptRegistryTableID:
		roots = []string{"prompt_text"}
	case AgentsTableID:
		roots = []string{"metadata_json", "config_json"}
	default:
		return true
	}
	for _, path := range paths {
		for _, root := range roots {
			if path == root || strings.HasPrefix(path, root+".") || strings.HasPrefix(path, root+"[") {
				return true
			}
		}
	}
	return false
}

type automationMaterialCounts struct {
	original            int
	redactedPlaceholder int
}

type automationDispositionCounts struct {
	candidate  int
	quarantine int
	reasons    map[string]int
}

func countAutomationResults[T any](values []Result[T]) (automationDispositionCounts, bool) {
	counts := automationDispositionCounts{reasons: make(map[string]int)}
	for _, value := range values {
		switch value.Disposition {
		case DispositionCandidate:
			if value.Fact == nil || value.Reason != "" {
				return automationDispositionCounts{}, false
			}
			counts.candidate++
		case DispositionQuarantine:
			if value.Fact != nil || value.Reason == "" {
				return automationDispositionCounts{}, false
			}
			counts.quarantine++
			counts.reasons[value.Reason]++
		default:
			return automationDispositionCounts{}, false
		}
	}
	return counts, true
}

func logAutomationCounts(t *testing.T, kind string, counts map[string]int) {
	t.Helper()
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		t.Logf("%s=%s count=%d", kind, key, counts[key])
	}
}

func TestAutomationArchiveRowIdentityAndMaterialRedaction(t *testing.T) {
	row := v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: AgentConfigTableID, SourceOrdinal: 1, SourceKeyHMAC: [sha256.Size]byte{1}, PayloadHMAC: [sha256.Size]byte{2}, FieldHMAC: [sha256.Size]byte{3}, Payload: []byte(`{"id":1}`)}
	if reason := automationArchiveRowReason(row, AgentConfigTableID, 1); reason != "" {
		t.Fatal(reason)
	}
	if automationMaterialRedacted(AgentConfigTableID, []string{"draft_variables_json.api_key"}) == false {
		t.Fatal("redacted configuration placeholder was classified as original")
	}
	if automationMaterialRedacted(PromptRegistryTableID, nil) {
		t.Fatal("unredacted prompt summary was classified as a placeholder")
	}
	if !automationMaterialRedacted(PromptRegistryTableID, []string{"prompt_text"}) {
		t.Fatal("redacted prompt placeholder was classified as original")
	}
	for _, mutate := range []func(*v1archive.ArchivedRow){
		func(value *v1archive.ArchivedRow) { value.AdapterID = "wrong" },
		func(value *v1archive.ArchivedRow) { value.TableID = SOPTemplateTableID },
		func(value *v1archive.ArchivedRow) { value.SourceOrdinal++ },
		func(value *v1archive.ArchivedRow) { value.SourceKeyHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.PayloadHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.FieldHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.Payload = []byte(`{`) },
	} {
		changed := row
		mutate(&changed)
		if automationArchiveRowReason(changed, AgentConfigTableID, 1) == "" {
			t.Fatal("invalid archive row identity was accepted")
		}
	}
}

func TestAutomationDispositionCountsRejectsInconsistentRows(t *testing.T) {
	if _, ok := countAutomationResults([]Result[AgentFact]{{Disposition: DispositionCandidate}}); ok {
		t.Fatal("candidate without fact was accepted")
	}
	if _, ok := countAutomationResults([]Result[AgentFact]{{Disposition: DispositionQuarantine, Reason: "automation_agents_shape_invalid", Fact: &AgentFact{}}}); ok {
		t.Fatal("quarantine with fact was accepted")
	}
	counts, ok := countAutomationResults([]Result[AgentFact]{{Disposition: DispositionQuarantine, Reason: "automation_agents_shape_invalid"}})
	if !ok || counts.quarantine != 1 || counts.reasons["automation_agents_shape_invalid"] != 1 {
		t.Fatal("fixed quarantine reason was not counted")
	}
}
