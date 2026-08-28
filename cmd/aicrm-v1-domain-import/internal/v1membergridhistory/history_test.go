package v1membergridhistory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestHistoryCandidatesPreserveSourceWithoutV2Claims(t *testing.T) {
	view := AdaptMemberView(archiveRow(MemberViewsTableID, 1, memberViewPayload(1)))
	if view.Disposition != DispositionCandidate || view.Record == nil || !view.Record.RequiresProductCrosswalk || !view.Record.RequiresSchemaCrosswalk ||
		view.Reason != ReasonSchemaCrosswalkRequired || !json.Valid(view.Record.ConfigJSON) || !json.Valid(view.Record.SourcePayload) {
		t.Fatal("member_view_history_candidate_missing")
	}
	usage := AdaptUsageSnapshot(archiveRow(UsageSnapshotsTableID, 1, usagePayload()))
	if usage.Disposition != DispositionCandidate || usage.Record == nil || usage.Record.UnionID != "" || usage.Record.LearningPlanCurrent != nil ||
		usage.Record.LastOpenAt != nil || !json.Valid(usage.Record.SourcePayload) {
		t.Fatal("usage_snapshot_not_preserved")
	}
}

func TestPermissionSharesAndSyncAreArchiveOnly(t *testing.T) {
	for _, decision := range []struct {
		disposition Disposition
		reason      string
		record      bool
	}{
		{AdaptMemberCollaborator(archiveRow(MemberCollaboratorsTableID, 1, collaboratorPayload())).Disposition, AdaptMemberCollaborator(archiveRow(MemberCollaboratorsTableID, 1, collaboratorPayload())).Reason, AdaptMemberCollaborator(archiveRow(MemberCollaboratorsTableID, 1, collaboratorPayload())).Record != nil},
		{AdaptMemberShare(archiveRow(MemberSharesTableID, 1, sharePayload())).Disposition, AdaptMemberShare(archiveRow(MemberSharesTableID, 1, sharePayload())).Reason, AdaptMemberShare(archiveRow(MemberSharesTableID, 1, sharePayload())).Record != nil},
		{AdaptUsageSyncRun(archiveRow(UsageSyncRunsTableID, 1, syncRunPayload())).Disposition, AdaptUsageSyncRun(archiveRow(UsageSyncRunsTableID, 1, syncRunPayload())).Reason, AdaptUsageSyncRun(archiveRow(UsageSyncRunsTableID, 1, syncRunPayload())).Record != nil},
	} {
		if decision.disposition != DispositionArchive || decision.reason == "" || !decision.record {
			t.Fatal("legacy_runtime_or_authorization_not_archived")
		}
	}
}

func TestMemberGridHistoryQuarantinesUnsafeInput(t *testing.T) {
	view := archiveRow(MemberViewsTableID, 1, memberViewPayload(1))
	for _, mutate := range []func(*v1archive.ArchivedRow){
		func(row *v1archive.ArchivedRow) { row.RedactedFields = []string{"config_json"} },
		func(row *v1archive.ArchivedRow) { row.Payload = []byte(`{`) },
		func(row *v1archive.ArchivedRow) { row.FieldHMAC = [sha256.Size]byte{} },
		func(row *v1archive.ArchivedRow) { row.Payload = []byte(memberViewPayload(0)) },
	} {
		row := view
		mutate(&row)
		if AdaptMemberView(row).Disposition != DispositionQuarantine {
			t.Fatal("unsafe_member_view_not_quarantined")
		}
	}
	usage := archiveRow(UsageSnapshotsTableID, 1, usagePayload())
	usage.Payload = []byte(`{"huangyoucan_user_id":"x","learning_plan_current":-1,"open_count_7d":0,"refreshed_at":"2026-08-01T01:00:00Z"}`)
	if AdaptUsageSnapshot(usage).Disposition != DispositionQuarantine {
		t.Fatal("invalid_usage_not_quarantined")
	}
}

func TestPreflightClassifiesAllFiveTablesWithoutWrites(t *testing.T) {
	archive := &fakeArchive{rows: map[string][]v1archive.ArchivedRow{
		MemberViewsTableID:         {archiveRow(MemberViewsTableID, 1, memberViewPayload(1))},
		UsageSnapshotsTableID:      {archiveRow(UsageSnapshotsTableID, 1, usagePayload())},
		UsageSyncRunsTableID:       {archiveRow(UsageSyncRunsTableID, 1, syncRunPayload())},
		MemberCollaboratorsTableID: {archiveRow(MemberCollaboratorsTableID, 1, collaboratorPayload())},
		MemberSharesTableID:        {archiveRow(MemberSharesTableID, 1, sharePayload())},
	}}
	report, err := Preflight(context.Background(), archive, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.Candidates != 2 || report.Archived != 3 || report.Quarantined != 0 || archive.calls != 5 {
		t.Fatalf("unexpected report: %#v calls=%d", report, archive.calls)
	}
	if report.Candidates+report.Archived+report.Quarantined != 5 || report.Reasons[ReasonSchemaCrosswalkRequired] != 1 {
		t.Fatal("preflight_row_conservation_or_reason_missing")
	}
}

func TestPreflightFailsClosedForArchiveScope(t *testing.T) {
	archive := &fakeArchive{rows: map[string][]v1archive.ArchivedRow{
		MemberViewsTableID:         {archiveRow(MemberViewsTableID, 2, memberViewPayload(1))},
		UsageSnapshotsTableID:      {},
		UsageSyncRunsTableID:       {},
		MemberCollaboratorsTableID: {},
		MemberSharesTableID:        {},
	}}
	if _, err := Preflight(context.Background(), archive, "run-1"); !errors.Is(err, ErrInvalidArchiveRow) {
		t.Fatalf("err=%v want invalid archive row", err)
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

func archiveRow(table string, ordinal int64, payload string) v1archive.ArchivedRow {
	value := []byte(payload)
	return v1archive.ArchivedRow{
		AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal,
		SourceKeyHMAC: sha256.Sum256([]byte(fmt.Sprintf("%s/%d", table, ordinal))),
		PayloadHMAC:   sha256.Sum256(value), FieldHMAC: sha256.Sum256([]byte("field/" + table)), Payload: value,
	}
}

func memberViewPayload(productID int64) string {
	return fmt.Sprintf(`{"id":1,"tenant_id":"tenant","service_product_id":%d,"name":"private view","position":0,"is_default":false,"schema_version":99,"config_json":{"unknown_filter":"preserve_only"},"version":1,"created_by":"old","updated_by":"old","created_at":"2026-08-01T01:00:00Z","updated_at":"2026-08-01T01:00:00Z"}`, productID)
}

func usagePayload() string {
	return `{"huangyoucan_user_id":"private-user","unionid":"","mobile_md5":"private-md5","formally_logged_in":true,"has_token_usage":true,"learning_plan_id":"plan","learning_plan_current":null,"learning_plan_total":null,"open_count_7d":0,"last_open_at":null,"refreshed_at":"2026-08-01T01:00:00Z"}`
}

func syncRunPayload() string {
	return `{"id":1,"trigger_source":"cron","status":"done","source_row_count":1,"snapshot_row_count":1,"started_at":"2026-08-01T01:00:00Z","finished_at":"2026-08-01T01:01:00Z","error_summary":""}`
}

func collaboratorPayload() string {
	return `{"id":1,"tenant_id":"tenant","service_product_id":1,"admin_user_id":3,"wecom_userid":"old-user","display_name":"private","avatar_url":"https://example.invalid/a","permission":"editor","version":1,"created_by":"old","updated_by":"old","created_at":"2026-08-01T01:00:00Z","updated_at":"2026-08-01T01:00:00Z"}`
}

func sharePayload() string {
	return `{"id":1,"tenant_id":"tenant","service_product_id":1,"enabled":true,"public_id":"secret-token","generation":1,"version":1,"created_by":"old","updated_by":"old","created_at":"2026-08-01T01:00:00Z","updated_at":"2026-08-01T01:00:00Z"}`
}
