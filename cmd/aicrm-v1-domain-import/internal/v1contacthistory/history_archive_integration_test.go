package v1contacthistory

import (
	"context"
	"encoding/json"
	"flag"
	"sort"
	"strings"
	"testing"
	"time"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var contactHistoryArchiveRun = flag.String("contact-history-archive-run", "", "optional reconciled V2 archive run for read-only Contact history validation")

func TestReconciledContactHistoryArchivePreflightWithoutWrites(t *testing.T) {
	if *contactHistoryArchiveRun == "" {
		t.Skip("supply -contact-history-archive-run and V2 archive environment for read-only validation")
	}
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	ctx := context.Background()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("contact_history_archive_open_failed")
	}
	defer archive.Close()
	report, err := Preflight(ctx, archive, *contactHistoryArchiveRun)
	if err != nil {
		for _, diagnostic := range contactHistoryPreflightDiagnostics(ctx, archive, *contactHistoryArchiveRun) {
			t.Logf("contact_history_preflight_diagnostic source=%s stage=%s reason=%s count=%d", diagnostic.Source, diagnostic.Stage, diagnostic.Reason, diagnostic.Count)
		}
		t.Fatal("contact_history_preflight_failed")
	}
	if report.SidebarRows != 59 || report.OwnerResultRows != 34 || report.OwnerSessionRows != 2 || report.OwnerPreviewRows != 35 {
		t.Fatalf("contact_history_archive_count_mismatch sidebar=%d results=%d sessions=%d previews=%d", report.SidebarRows, report.OwnerResultRows, report.OwnerSessionRows, report.OwnerPreviewRows)
	}
	if report.SidebarCandidates+report.OwnerResultCandidates+report.Quarantined != report.SidebarRows+report.OwnerResultRows {
		t.Fatal("contact_history_row_conservation_failed")
	}
	if report.PreviewSessionResolved+report.PreviewSessionUnresolved != report.OwnerPreviewRows ||
		report.ResultPreviewResolved+report.ResultPreviewUnresolved != report.OwnerResultCandidates {
		t.Fatal("contact_history_relation_count_mismatch")
	}
	for _, reason := range report.SortedReasons() {
		t.Logf("reason=%s count=%d", reason, report.Reasons[reason])
	}
	if report.Reasons[ReasonInvalidOwnerResult] != 0 {
		for _, diagnostic := range contactHistoryResultDiagnostics(ctx, archive, *contactHistoryArchiveRun) {
			t.Logf("contact_history_result_diagnostic source=%s stage=%s reason=%s count=%d", diagnostic.Source, diagnostic.Stage, diagnostic.Reason, diagnostic.Count)
		}
	}
	t.Logf("read_only_preflight sidebar=%d owner_results=%d sessions=%d previews=%d candidates=%d quarantined=%d preview_session_resolved=%d preview_session_unresolved=%d result_preview_resolved=%d result_preview_unresolved=%d target_writes=0", report.SidebarRows, report.OwnerResultRows, report.OwnerSessionRows, report.OwnerPreviewRows, report.SidebarCandidates+report.OwnerResultCandidates, report.Quarantined, report.PreviewSessionResolved, report.PreviewSessionUnresolved, report.ResultPreviewResolved, report.ResultPreviewUnresolved)
}

func contactHistoryResultDiagnostics(ctx context.Context, archive ArchiveReader, runID string) []contactHistoryPreflightDiagnostic {
	rows, err := readRows(ctx, archive, runID, OwnerMigrationResultsTableID)
	if err != nil {
		return []contactHistoryPreflightDiagnostic{{Source: "owner_results", Stage: "read", Reason: "read_failed", Count: 1}}
	}
	return resultParseDiagnostics(rows)
}

type contactHistoryPreflightDiagnostic struct {
	Source string
	Stage  string
	Reason string
	Count  int
}

func contactHistoryPreflightDiagnostics(ctx context.Context, archive ArchiveReader, runID string) []contactHistoryPreflightDiagnostic {
	tables := []struct {
		source  string
		tableID string
	}{
		{source: "sidebar", tableID: SidebarProfileFieldsTableID},
		{source: "owner_results", tableID: OwnerMigrationResultsTableID},
		{source: "owner_sessions", tableID: OwnerMigrationSessionsTableID},
		{source: "owner_previews", tableID: OwnerMigrationPreviewsTableID},
	}
	rows := make(map[string][]v1archive.ArchivedRow, len(tables))
	for _, table := range tables {
		value, err := readRows(ctx, archive, runID, table.tableID)
		if err != nil {
			return []contactHistoryPreflightDiagnostic{{Source: table.source, Stage: "read", Reason: "read_failed", Count: 1}}
		}
		rows[table.tableID] = value
	}
	if count := invalidSessionRows(rows[OwnerMigrationSessionsTableID]); count != 0 {
		return []contactHistoryPreflightDiagnostic{{Source: "owner_sessions", Stage: "parse_row", Reason: ReasonInvalidArchiveRow, Count: count}}
	}
	if _, err := parseSessions(rows[OwnerMigrationSessionsTableID]); err != nil {
		return []contactHistoryPreflightDiagnostic{{Source: "owner_sessions", Stage: "parse_set", Reason: ReasonInvalidArchiveRow, Count: 1}}
	}
	if diagnostics := previewParseDiagnostics(rows[OwnerMigrationPreviewsTableID]); len(diagnostics) != 0 {
		return diagnostics
	}
	if _, err := parsePreviews(rows[OwnerMigrationPreviewsTableID]); err != nil {
		return []contactHistoryPreflightDiagnostic{{Source: "owner_previews", Stage: "parse_set", Reason: ReasonInvalidArchiveRow, Count: 1}}
	}
	return []contactHistoryPreflightDiagnostic{{Source: "contact_history", Stage: "preflight", Reason: "unknown_failure", Count: 1}}
}

func invalidSessionRows(rows []v1archive.ArchivedRow) int {
	count := 0
	for _, row := range rows {
		if _, err := parseSessions([]v1archive.ArchivedRow{row}); err != nil {
			count++
		}
	}
	return count
}

func previewParseDiagnostics(rows []v1archive.ArchivedRow) []contactHistoryPreflightDiagnostic {
	counts := make(map[string]int)
	for _, row := range rows {
		for _, reason := range previewValidationReasons(row) {
			counts[reason]++
		}
	}
	if len(counts) == 0 {
		return nil
	}
	reasons := make([]string, 0, len(counts))
	for reason := range counts {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	result := make([]contactHistoryPreflightDiagnostic, 0, len(reasons))
	for _, reason := range reasons {
		result = append(result, contactHistoryPreflightDiagnostic{Source: "owner_previews", Stage: "parse_row", Reason: reason, Count: counts[reason]})
	}
	return result
}

func resultParseDiagnostics(rows []v1archive.ArchivedRow) []contactHistoryPreflightDiagnostic {
	counts := make(map[string]int)
	for _, row := range rows {
		for _, reason := range resultValidationReasons(row) {
			counts[reason]++
		}
	}
	if len(counts) == 0 {
		return nil
	}
	reasons := make([]string, 0, len(counts))
	for reason := range counts {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	result := make([]contactHistoryPreflightDiagnostic, 0, len(reasons))
	for _, reason := range reasons {
		result = append(result, contactHistoryPreflightDiagnostic{Source: "owner_results", Stage: "parse_row", Reason: reason, Count: counts[reason]})
	}
	return result
}

func resultValidationReasons(row v1archive.ArchivedRow) []string {
	if !validRow(row, OwnerMigrationResultsTableID) {
		return []string{ReasonInvalidArchiveRow}
	}
	if !hasExactRedactions(row, "preview_token", "stats_json.preview_token") {
		return []string{"owner_result_redaction_profile_invalid"}
	}
	var value OwnerMigrationResultHistory
	if json.Unmarshal(row.Payload, &value) != nil {
		return []string{ReasonInvalidSourcePayload}
	}
	reasons := make([]string, 0)
	if !validIdentifier(value.ResultID) {
		reasons = append(reasons, "owner_result_result_id_missing_or_invalid")
	}
	if !validOptionalIdentifier(value.SessionID) {
		reasons = append(reasons, "owner_result_session_id_invalid")
	}
	if value.CreatedAt.IsZero() {
		reasons = append(reasons, "owner_result_created_at_missing_or_zero")
	}
	if value.ExecutedAt.IsZero() {
		reasons = append(reasons, "owner_result_executed_at_missing_or_zero")
	}
	if !value.CreatedAt.IsZero() && !value.ExecutedAt.IsZero() && value.ExecutedAt.Before(value.CreatedAt) {
		reasons = append(reasons, "owner_result_executed_before_created")
	}
	for _, field := range []struct {
		name  string
		value int
	}{
		{name: "total_rows", value: value.TotalRows},
		{name: "eligible_count", value: value.EligibleCount},
		{name: "wecom_success", value: value.WeComSuccess},
		{name: "wecom_failed", value: value.WeComFailed},
		{name: "crm_updated", value: value.CRMUpdated},
	} {
		if field.value < 0 {
			reasons = append(reasons, "owner_result_"+field.name+"_negative")
		}
	}
	if !validJSON(value.RowsJSON) {
		reasons = append(reasons, "owner_result_rows_json_missing_or_invalid")
	}
	if !validJSON(value.StatsJSON) {
		reasons = append(reasons, "owner_result_stats_json_missing_or_invalid")
	}
	return reasons
}

func previewValidationReasons(row v1archive.ArchivedRow) []string {
	if !validRow(row, OwnerMigrationPreviewsTableID) {
		return []string{ReasonInvalidArchiveRow}
	}
	if !hasExactRedactions(row, "preview_token") {
		return []string{"owner_preview_redaction_profile_invalid"}
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(row.Payload, &fields) != nil || fields == nil {
		return []string{ReasonInvalidSourcePayload}
	}
	reasons := make([]string, 0)
	if reason := previewSessionIDReason(fields["session_id"]); reason != "" {
		reasons = append(reasons, reason)
	}
	createdAt, createdReason := previewTimeReason("created_at", fields["created_at"])
	if createdReason != "" {
		reasons = append(reasons, createdReason)
	}
	expiresAt, expiresReason := previewTimeReason("expires_at", fields["expires_at"])
	if expiresReason != "" {
		reasons = append(reasons, expiresReason)
	}
	if createdReason == "" && expiresReason == "" && expiresAt.Before(createdAt) {
		reasons = append(reasons, "owner_preview_expires_before_created")
	}
	for _, field := range []struct {
		name string
		kind byte
	}{
		{name: "eligible_external_userids_json", kind: '['},
		{name: "rows_json", kind: '['},
		{name: "row_stats_json", kind: '{'},
		{name: "surface_counts_json", kind: '{'},
		{name: "pending_review_json", kind: '{'},
	} {
		if reason := previewJSONReason(field.name, fields[field.name], field.kind); reason != "" {
			reasons = append(reasons, reason)
		}
	}
	if reason := previewExecutedResultIDReason(fields["executed_result_id"]); reason != "" {
		reasons = append(reasons, reason)
	}
	return reasons
}

func previewSessionIDReason(raw json.RawMessage) string {
	var value string
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if json.Unmarshal(raw, &value) != nil {
		return "owner_preview_session_id_invalid"
	}
	if strings.TrimSpace(value) == "" {
		return ""
	}
	if !validIdentifier(value) {
		return "owner_preview_session_id_invalid"
	}
	return ""
}

func previewTimeReason(name string, raw json.RawMessage) (time.Time, string) {
	var value time.Time
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return time.Time{}, "owner_preview_" + name + "_parse"
	}
	if value.IsZero() {
		return time.Time{}, "owner_preview_" + name + "_zero"
	}
	return value, ""
}

func previewJSONReason(name string, raw json.RawMessage, kind byte) string {
	if len(raw) == 0 {
		return "owner_preview_" + name + "_missing"
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "null" {
		return "owner_preview_" + name + "_null"
	}
	if !json.Valid(raw) || len(trimmed) == 0 || trimmed[0] != kind {
		return "owner_preview_" + name + "_type"
	}
	return ""
}

func previewExecutedResultIDReason(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return "owner_preview_executed_result_id_missing"
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || !validOptionalIdentifier(value) {
		return "owner_preview_executed_result_id_invalid"
	}
	return ""
}
