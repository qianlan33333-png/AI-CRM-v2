package v1contacthistory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestAdaptSidebarProfilePreservesHistoryWithoutCustomerMapping(t *testing.T) {
	row := archiveRow(SidebarProfileFieldsTableID, 1, `{"source":"sidebar","industry":"education","industry_description":"private","needs_blockers_followup":"yes","updated_by":"staff-1","updated_at":"2026-08-01T01:02:03Z","unionid":"union-1","future_custom_field":{"private":true}}`)
	decision := AdaptSidebarProfile(row)
	if decision.Disposition != DispositionCandidate || decision.Candidate == nil {
		t.Fatal("sidebar_candidate_missing")
	}
	if decision.Candidate.UnionID != "union-1" || decision.Candidate.Industry != "education" || !json.Valid(decision.Candidate.SourcePayload) {
		t.Fatal("sidebar_history_not_preserved")
	}
	if decision.Candidate.UpdatedAt.IsZero() || len(decision.Candidate.SourcePayload) != len(row.Payload) {
		t.Fatal("sidebar_time_or_payload_not_preserved")
	}
}

func TestAdaptContactHistoryQuarantinesUnsafeInputs(t *testing.T) {
	sidebar := archiveRow(SidebarProfileFieldsTableID, 1, sidebarPayload("union-1"))
	for _, mutate := range []func(*v1archive.ArchivedRow){
		func(row *v1archive.ArchivedRow) { row.RedactedFields = []string{"industry"} },
		func(row *v1archive.ArchivedRow) { row.Payload = []byte(`{`) },
		func(row *v1archive.ArchivedRow) { row.AdapterID = "wrong-adapter" },
		func(row *v1archive.ArchivedRow) { row.FieldHMAC = [sha256.Size]byte{} },
	} {
		row := sidebar
		mutate(&row)
		if AdaptSidebarProfile(row).Disposition != DispositionQuarantine {
			t.Fatal("unsafe_sidebar_not_quarantined")
		}
	}

	result := redactedResultRow(1, "result-1", "session-1")
	for _, mutate := range []func(*v1archive.ArchivedRow){
		func(row *v1archive.ArchivedRow) {
			row.RedactedFields = []string{"preview_token", "rows_json", "stats_json.preview_token"}
		},
		func(row *v1archive.ArchivedRow) { row.Payload = []byte(`{"result_id":"result-1"}`) },
		func(row *v1archive.ArchivedRow) { row.Payload = []byte(`{`) },
	} {
		row := result
		mutate(&row)
		if AdaptOwnerMigrationResult(row).Disposition != DispositionQuarantine {
			t.Fatal("unsafe_result_not_quarantined")
		}
	}
}

func TestPreflightChecksRelationsAndNeverWrites(t *testing.T) {
	archive := fakeArchive{rows: map[string][]v1archive.ArchivedRow{
		SidebarProfileFieldsTableID:   {archiveRow(SidebarProfileFieldsTableID, 1, sidebarPayload("union-1"))},
		OwnerMigrationSessionsTableID: {archiveRow(OwnerMigrationSessionsTableID, 1, ownerSessionPayload("session-1"))},
		OwnerMigrationPreviewsTableID: {redactedPreviewRow(1, "session-1", "result-1")},
		OwnerMigrationResultsTableID:  {redactedResultRow(1, "result-1", "session-1")},
	}}
	report, err := Preflight(context.Background(), &archive, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.SidebarCandidates != 1 || report.OwnerResultCandidates != 1 || report.Quarantined != 0 ||
		report.SidebarRows != 1 || report.OwnerResultRows != 1 || report.OwnerSessionRows != 1 || report.OwnerPreviewRows != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
	if report.PreviewSessionResolved != 1 || report.PreviewSessionUnresolved != 0 || report.ResultPreviewResolved != 1 || report.ResultPreviewUnresolved != 0 {
		t.Fatalf("unexpected relation report: %#v", report)
	}
	if archive.calls != 4 {
		t.Fatalf("archive calls=%d want=4", archive.calls)
	}
}

func TestPreflightPreservesRedactedTokenHistoryWithoutGuessingRelation(t *testing.T) {
	archive := fakeArchive{rows: map[string][]v1archive.ArchivedRow{
		SidebarProfileFieldsTableID:   {},
		OwnerMigrationSessionsTableID: {archiveRow(OwnerMigrationSessionsTableID, 1, ownerSessionPayload("session-1"))},
		OwnerMigrationPreviewsTableID: {redactedPreviewRow(1, "", "result-1")},
		OwnerMigrationResultsTableID:  {redactedResultRow(1, "result-1", "session-1")},
	}}
	report, err := Preflight(context.Background(), &archive, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.OwnerResultCandidates != 1 || report.Quarantined != 0 || report.ResultPreviewResolved != 0 || report.ResultPreviewUnresolved != 1 {
		t.Fatalf("redacted_token_history_not_preserved: %#v", report)
	}
	if report.PreviewSessionResolved != 0 || report.PreviewSessionUnresolved != 1 {
		t.Fatalf("session_relation_not_verified: %#v", report)
	}

	row := redactedResultRow(1, "result-1", "session-1")
	decision := AdaptOwnerMigrationResult(row)
	if decision.Candidate == nil || decision.Candidate.PreviewRelation != OwnerPreviewRelationUnresolved || decision.Candidate.SourceKeyHMAC != row.SourceKeyHMAC || string(decision.Candidate.SourcePayload) != string(row.Payload) {
		t.Fatal("redacted_result_source_evidence_not_preserved")
	}
}

func TestPreflightRetainsThirtyThreeEmptyPreviewSessionsAsUnresolved(t *testing.T) {
	previews := make([]v1archive.ArchivedRow, 0, 33)
	for ordinal := int64(1); ordinal <= 33; ordinal++ {
		previews = append(previews, redactedPreviewRow(ordinal, "", ""))
	}
	archive := fakeArchive{rows: map[string][]v1archive.ArchivedRow{
		SidebarProfileFieldsTableID: {},
		OwnerMigrationSessionsTableID: {
			archiveRow(OwnerMigrationSessionsTableID, 1, ownerSessionPayload("session-1")),
			archiveRow(OwnerMigrationSessionsTableID, 2, ownerSessionPayload("session-2")),
		},
		OwnerMigrationPreviewsTableID: previews,
		OwnerMigrationResultsTableID:  {},
	}}
	report, err := Preflight(context.Background(), &archive, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.OwnerPreviewRows != 33 || report.PreviewSessionResolved != 0 || report.PreviewSessionUnresolved != 33 || report.Quarantined != 0 {
		t.Fatalf("empty_preview_sessions_not_preserved: %#v", report)
	}
}

func TestPreflightLeavesUnknownNonEmptyPreviewSessionUnresolved(t *testing.T) {
	archive := fakeArchive{rows: map[string][]v1archive.ArchivedRow{
		SidebarProfileFieldsTableID:   {},
		OwnerMigrationSessionsTableID: {archiveRow(OwnerMigrationSessionsTableID, 1, ownerSessionPayload("session-1"))},
		OwnerMigrationPreviewsTableID: {redactedPreviewRow(1, "session-unknown", "")},
		OwnerMigrationResultsTableID:  {},
	}}
	report, err := Preflight(context.Background(), &archive, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.PreviewSessionResolved != 0 || report.PreviewSessionUnresolved != 1 || report.Quarantined != 0 {
		t.Fatalf("unknown_preview_session_not_unresolved: %#v", report)
	}
}

func TestPreflightRetainsEmptyResultSessionWithoutGuessingPreviewRelation(t *testing.T) {
	archive := fakeArchive{rows: map[string][]v1archive.ArchivedRow{
		SidebarProfileFieldsTableID:   {},
		OwnerMigrationSessionsTableID: {archiveRow(OwnerMigrationSessionsTableID, 1, ownerSessionPayload("session-1"))},
		OwnerMigrationPreviewsTableID: {redactedPreviewRow(1, "session-1", "result-1")},
		OwnerMigrationResultsTableID:  {redactedResultRow(1, "result-1", "")},
	}}
	report, err := Preflight(context.Background(), &archive, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.OwnerResultCandidates != 1 || report.Quarantined != 0 || report.ResultPreviewResolved != 0 || report.ResultPreviewUnresolved != 1 {
		t.Fatalf("empty_result_session_not_preserved_as_unresolved: %#v", report)
	}
}

func TestParsePreviewsRejectsNonEmptyInvalidSessionID(t *testing.T) {
	if _, err := parsePreviews([]v1archive.ArchivedRow{redactedPreviewRow(1, " session-1 ", "")}); !errors.Is(err, ErrInvalidArchiveRow) {
		t.Fatalf("err=%v want invalid archive row", err)
	}
}

func TestPreflightKeepsExistingPreviewSessionAssociations(t *testing.T) {
	archive := fakeArchive{rows: map[string][]v1archive.ArchivedRow{
		SidebarProfileFieldsTableID: {},
		OwnerMigrationSessionsTableID: {
			archiveRow(OwnerMigrationSessionsTableID, 1, ownerSessionPayload("session-1")),
			archiveRow(OwnerMigrationSessionsTableID, 2, ownerSessionPayload("session-2")),
		},
		OwnerMigrationPreviewsTableID: {
			redactedPreviewRow(1, "session-1", ""),
			redactedPreviewRow(2, "session-2", ""),
		},
		OwnerMigrationResultsTableID: {},
	}}
	report, err := Preflight(context.Background(), &archive, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.PreviewSessionResolved != 2 || report.PreviewSessionUnresolved != 0 || report.Quarantined != 0 {
		t.Fatalf("existing_preview_sessions_regressed: %#v", report)
	}
}

func TestOwnerMigrationContextExposesOnlyVerifiedRelations(t *testing.T) {
	context, err := BuildOwnerMigrationContext(
		[]v1archive.ArchivedRow{archiveRow(OwnerMigrationSessionsTableID, 1, ownerSessionPayload("session-1"))},
		[]v1archive.ArchivedRow{redactedPreviewRow(1, "session-1", "result-1")},
	)
	if err != nil {
		t.Fatal("context_build_failed")
	}
	decision := AdaptOwnerMigrationResult(redactedResultRow(1, "result-1", "session-1"))
	if decision.Candidate == nil {
		t.Fatal("result_candidate_missing")
	}
	relations, reason := context.SessionRelation(*decision.Candidate)
	if reason != "" || relations != (OwnerMigrationRelations{SessionRelation: OwnerSessionRelationResolved, PreviewRelation: OwnerPreviewRelationResolved}) {
		t.Fatalf("verified_relations=%#v reason=%q", relations, reason)
	}
	decision = AdaptOwnerMigrationResult(redactedResultRow(1, "result-1", ""))
	relations, reason = context.SessionRelation(*decision.Candidate)
	if reason != "" || relations != (OwnerMigrationRelations{SessionRelation: OwnerSessionRelationUnresolved, PreviewRelation: OwnerPreviewRelationUnresolved}) {
		t.Fatalf("empty_session_was_guessed relations=%#v reason=%q", relations, reason)
	}
}

func TestPreflightQuarantinesMissingSessionOrMismatchedOwnerRelations(t *testing.T) {
	for name, scenario := range map[string]struct {
		resultSession  string
		previewSession string
		wantCandidate  int
		wantQuarantine int
		wantUnresolved int
		wantReason     string
	}{
		"preview_unresolved": {resultSession: "session-1", wantCandidate: 1, wantUnresolved: 1},
		"mismatch":           {resultSession: "session-1", previewSession: "session-2", wantQuarantine: 1, wantReason: ReasonOwnerRelationMismatched},
		"session_missing":    {resultSession: "session-2", previewSession: "session-2", wantQuarantine: 1, wantReason: ReasonOwnerRelationMissing},
	} {
		t.Run(name, func(t *testing.T) {
			previews := []v1archive.ArchivedRow{}
			if scenario.previewSession != "" {
				previews = append(previews, redactedPreviewRow(1, scenario.previewSession, "result-1"))
			}
			archive := fakeArchive{rows: map[string][]v1archive.ArchivedRow{
				SidebarProfileFieldsTableID:   {},
				OwnerMigrationSessionsTableID: {archiveRow(OwnerMigrationSessionsTableID, 1, ownerSessionPayload("session-1"))},
				OwnerMigrationPreviewsTableID: previews,
				OwnerMigrationResultsTableID:  {redactedResultRow(1, "result-1", scenario.resultSession)},
			}}
			report, err := Preflight(context.Background(), &archive, "run-1")
			if err != nil {
				t.Fatal(err)
			}
			if report.OwnerResultCandidates != scenario.wantCandidate || report.Quarantined != scenario.wantQuarantine || report.ResultPreviewUnresolved != scenario.wantUnresolved {
				t.Fatalf("unexpected report: %#v", report)
			}
			if scenario.wantReason != "" && report.Reasons[scenario.wantReason] != 1 {
				t.Fatalf("relation_reason_absent report=%#v", report)
			}
		})
	}
}

func TestPreflightRejectsArchiveIntegrityOrParentShape(t *testing.T) {
	valid := fakeArchive{rows: map[string][]v1archive.ArchivedRow{
		SidebarProfileFieldsTableID:   {},
		OwnerMigrationSessionsTableID: {archiveRow(OwnerMigrationSessionsTableID, 1, ownerSessionPayload("session-1"))},
		OwnerMigrationPreviewsTableID: {redactedPreviewRow(1, "session-1", "")},
		OwnerMigrationResultsTableID:  {},
	}}
	for _, mutate := range []func(*fakeArchive){
		func(archive *fakeArchive) { archive.rows[OwnerMigrationSessionsTableID][0].SourceOrdinal = 2 },
		func(archive *fakeArchive) {
			archive.rows[OwnerMigrationPreviewsTableID][0].RedactedFields = []string{"preview_token", "rows_json"}
		},
		func(archive *fakeArchive) { archive.rows[OwnerMigrationSessionsTableID][0].Payload = []byte(`{`) },
	} {
		archive := cloneArchive(valid)
		mutate(&archive)
		if _, err := Preflight(context.Background(), &archive, "run-1"); !errors.Is(err, ErrInvalidArchiveRow) {
			t.Fatalf("err=%v want invalid archive row", err)
		}
	}
}

func TestContactHistoryPreflightDiagnosticsUsesFixedPreviewParseBucket(t *testing.T) {
	preview := redactedPreviewRow(1, "session-1", "")
	preview.RedactedFields = []string{"preview_token", "rows_json"}
	archive := fakeArchive{rows: map[string][]v1archive.ArchivedRow{
		SidebarProfileFieldsTableID:   {},
		OwnerMigrationSessionsTableID: {archiveRow(OwnerMigrationSessionsTableID, 1, ownerSessionPayload("session-1"))},
		OwnerMigrationPreviewsTableID: {preview},
		OwnerMigrationResultsTableID:  {},
	}}
	diagnostics := contactHistoryPreflightDiagnostics(context.Background(), &archive, "run-1")
	if len(diagnostics) != 1 || diagnostics[0] != (contactHistoryPreflightDiagnostic{Source: "owner_previews", Stage: "parse_row", Reason: "owner_preview_redaction_profile_invalid", Count: 1}) {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
}

func TestPreviewDiagnosticsUseFixedFieldReasons(t *testing.T) {
	payload := strings.Replace(ownerPreviewPayload("[REDACTED]", " session-1 ", ""), `"rows_json":[]`, `"rows_json":{}`, 1)
	row := archiveRow(OwnerMigrationPreviewsTableID, 1, payload)
	row.RedactedFields = []string{"preview_token"}
	diagnostics := previewParseDiagnostics([]v1archive.ArchivedRow{row})
	want := []contactHistoryPreflightDiagnostic{
		{Source: "owner_previews", Stage: "parse_row", Reason: "owner_preview_rows_json_type", Count: 1},
		{Source: "owner_previews", Stage: "parse_row", Reason: "owner_preview_session_id_invalid", Count: 1},
	}
	if len(diagnostics) != len(want) {
		t.Fatalf("diagnostic count=%d want=%d", len(diagnostics), len(want))
	}
	for index := range want {
		if diagnostics[index] != want[index] {
			t.Fatalf("diagnostic[%d]=%#v want=%#v", index, diagnostics[index], want[index])
		}
	}
}

func TestResultDiagnosticsUseFixedFieldReasons(t *testing.T) {
	payload := strings.Replace(ownerResultPayload("result-1", "session-1"), `"eligible_count":1`, `"eligible_count":-1`, 1)
	payload = strings.Replace(payload, `"session_id":"session-1"`, `"session_id":" session-1 "`, 1)
	diagnostics := resultParseDiagnostics([]v1archive.ArchivedRow{redactedResultRowWithPayload(1, payload)})
	want := []contactHistoryPreflightDiagnostic{
		{Source: "owner_results", Stage: "parse_row", Reason: "owner_result_eligible_count_negative", Count: 1},
		{Source: "owner_results", Stage: "parse_row", Reason: "owner_result_session_id_invalid", Count: 1},
	}
	if len(diagnostics) != len(want) {
		t.Fatalf("diagnostic count=%d want=%d", len(diagnostics), len(want))
	}
	for index := range want {
		if diagnostics[index] != want[index] {
			t.Fatalf("diagnostic[%d]=%#v want=%#v", index, diagnostics[index], want[index])
		}
	}
}

type fakeArchive struct {
	rows  map[string][]v1archive.ArchivedRow
	calls int
}

func (archive *fakeArchive) EachTableRow(ctx context.Context, runID, tableID string, callback func(v1archive.ArchivedRow) error) error {
	archive.calls++
	if runID != "run-1" || callback == nil {
		return errors.New("invalid fake archive call")
	}
	for _, row := range archive.rows[tableID] {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

func cloneArchive(source fakeArchive) fakeArchive {
	values := make(map[string][]v1archive.ArchivedRow, len(source.rows))
	for table, rows := range source.rows {
		values[table] = append([]v1archive.ArchivedRow(nil), rows...)
	}
	return fakeArchive{rows: values}
}

func archiveRow(table string, ordinal int64, payload string) v1archive.ArchivedRow {
	value := []byte(payload)
	return v1archive.ArchivedRow{
		AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal,
		SourceKeyHMAC: sha256.Sum256([]byte(fmt.Sprintf("%s/%d", table, ordinal))),
		PayloadHMAC:   sha256.Sum256(value),
		FieldHMAC:     sha256.Sum256([]byte("field/" + table)),
		Payload:       value,
	}
}

func redactedPreviewRow(ordinal int64, sessionID, executedResultID string) v1archive.ArchivedRow {
	row := archiveRow(OwnerMigrationPreviewsTableID, ordinal, ownerPreviewPayload("[REDACTED]", sessionID, executedResultID))
	row.RedactedFields = []string{"preview_token"}
	return row
}

func redactedResultRow(ordinal int64, resultID, sessionID string) v1archive.ArchivedRow {
	return redactedResultRowWithPayload(ordinal, ownerResultPayload(resultID, sessionID))
}

func redactedResultRowWithPayload(ordinal int64, payload string) v1archive.ArchivedRow {
	row := archiveRow(OwnerMigrationResultsTableID, ordinal, payload)
	row.RedactedFields = []string{"preview_token", "stats_json.preview_token"}
	return row
}

func sidebarPayload(unionID string) string {
	return fmt.Sprintf(`{"source":"sidebar","industry":"education","industry_description":"private","needs_blockers_followup":"yes","updated_by":"staff-1","updated_at":"2026-08-01T01:02:03Z","unionid":%q}`, unionID)
}

func ownerSessionPayload(sessionID string) string {
	return fmt.Sprintf(`{"session_id":%q,"file_name":"private.csv","file_hash":"hash","source_owner_userid":"old","target_owner_userid":"new","include_wecom_transfer":false,"transfer_welcome_msg":"","rows_json":[],"row_stats_json":{},"operator":"operator","created_at":"2026-08-01T01:02:03Z"}`, sessionID)
}

func ownerPreviewPayload(token, sessionID, executedResultID string) string {
	return fmt.Sprintf(`{"preview_token":%q,"preview_hash":"hash","scope_type":"all","session_id":%q,"file_hash":"hash","source_owner_userid":"old","target_owner_userid":"new","source_owner_display_name":"old","target_owner_display_name":"new","include_wecom_transfer":false,"transfer_welcome_msg":"","eligible_external_userids_json":[],"rows_json":[],"row_stats_json":{},"surface_counts_json":{},"pending_review_json":{},"confirm_phrase":"confirm","operator":"operator","created_at":"2026-08-01T01:02:03Z","expires_at":"2026-08-02T01:02:03Z","executed_result_id":%q}`, token, sessionID, executedResultID)
}

func ownerResultPayload(resultID, sessionID string) string {
	return fmt.Sprintf(`{"result_id":%q,"job_id":"job-1","preview_token":"[REDACTED]","scope_type":"all","session_id":%q,"file_hash":"hash","source_owner_userid":"old","target_owner_userid":"new","source_owner_display_name":"old","target_owner_display_name":"new","operator":"operator","preview_hash":"hash","total_rows":1,"eligible_count":1,"wecom_success":0,"wecom_failed":0,"crm_updated":1,"include_wecom_transfer":false,"transfer_welcome_msg":"","rows_json":[],"stats_json":{"preview_token":"[REDACTED]"},"created_at":"2026-08-01T01:02:03Z","executed_at":"2026-08-01T01:03:03Z"}`, resultID, sessionID)
}

func TestCandidateTimesRemainSourceInstants(t *testing.T) {
	row := archiveRow(SidebarProfileFieldsTableID, 1, `{"source":"sidebar","industry":"","industry_description":"","needs_blockers_followup":"","updated_by":"","updated_at":"2026-08-01T09:02:03+08:00","unionid":"union-1"}`)
	decision := AdaptSidebarProfile(row)
	if decision.Candidate == nil || !decision.Candidate.UpdatedAt.Equal(time.Date(2026, 8, 1, 1, 2, 3, 0, time.UTC)) {
		t.Fatal("source_time_changed")
	}
}
