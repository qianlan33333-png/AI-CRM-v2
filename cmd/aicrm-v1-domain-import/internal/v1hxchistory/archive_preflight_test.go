package v1hxchistory

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

var hxcHistoryArchiveRun = flag.String("hxc-history-archive-run", "", "optional reconciled V2 archive run for read-only HXC history validation")

const (
	reasonArchiveScope       = "hxc_history_archive_scope_invalid"
	reasonArchiveHMAC        = "hxc_history_archive_hmac_missing"
	reasonArchiveDuplicate   = "hxc_history_archive_duplicate_source"
	reasonRequiredRedaction  = "hxc_history_required_field_redacted"
	reasonCandidateInvariant = "hxc_history_candidate_invariant_invalid"
)

type hxcArchiveSpec struct {
	table     string
	expected  int
	candidate bool
}

type hxcArchiveTable struct {
	payloads []json.RawMessage
	forced   map[int]bool
}

// TestReconciledHXCArchivePreflight is explicitly opt-in. EachTableRow opens
// only a repeatable-read/read-only transaction against the already reconciled
// V2 archive; it has no V1 URL, target writer, queue, or Provider path.
func TestReconciledHXCArchivePreflight(t *testing.T) {
	if *hxcHistoryArchiveRun == "" {
		t.Skip("supply -hxc-history-archive-run and V2 archive environment for read-only HXC history validation")
	}
	ctx := context.Background()
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("hxc_history_archive_open_failed")
	}
	defer archive.Close()

	specs := []hxcArchiveSpec{
		{DashboardMetaTableID, 816, true}, {DashboardSnapshotTableID, 1326, true},
		{ActivationStatusTableID, 149, true}, {HuangxiaocanActivationID, 142, true},
		{ExperienceLeadsTableID, 13, true}, {ImportBatchesTableID, 27, true},
		{SendRecordsTableID, 2, false}, {SendConfigTableID, 1, false},
	}
	rows := make(map[string]hxcArchiveTable, len(specs))
	reasons, roots := map[string]int{}, map[string]int{}
	for _, spec := range specs {
		if !spec.candidate {
			decision := ClassifyArchiveOnlyTable(spec.table)
			if decision.Disposition != DispositionArchive || decision.Fact != nil {
				t.Fatal("hxc_history_archive_only_policy_invalid")
			}
		}
		value, tableReasons, tableRoots, readErr := readHXCArchiveTable(ctx, archive, *hxcHistoryArchiveRun, spec)
		if readErr != nil {
			t.Fatal(readErr.Error())
		}
		if len(value.payloads) != spec.expected {
			t.Fatal("hxc_history_archive_count_mismatch")
		}
		rows[spec.table] = value
		mergeCounts(reasons, tableReasons)
		mergeCounts(roots, tableRoots)
	}

	history := AdaptHistory(
		rows[DashboardMetaTableID].payloads,
		rows[DashboardSnapshotTableID].payloads,
		rows[ActivationStatusTableID].payloads,
		rows[HuangxiaocanActivationID].payloads,
		rows[ExperienceLeadsTableID].payloads,
		rows[ImportBatchesTableID].payloads,
	)
	if len(history.DashboardMeta) != 816 || len(history.DashboardSnapshot) != 1326 || len(history.ActivationStatus) != 149 || len(history.Huangxiaocan) != 142 || len(history.ExperienceLeads) != 13 || len(history.ImportBatches) != 27 {
		t.Fatal("hxc_history_candidate_row_conservation_failed")
	}
	candidates, quarantined, countErr := countHXCResults(history, rows, reasons)
	if countErr != nil {
		t.Fatal(countErr.Error())
	}
	archived := len(rows[SendRecordsTableID].payloads) + len(rows[SendConfigTableID].payloads)
	if candidates+quarantined != 2473 || archived != 3 {
		t.Fatal("hxc_history_terminal_count_mismatch")
	}
	logHXCPreflightCounts(t, specs, candidates, quarantined, archived, reasons, roots)
}

func readHXCArchiveTable(ctx context.Context, archive *v1archive.PostgresArchiveReader, runID string, spec hxcArchiveSpec) (hxcArchiveTable, map[string]int, map[string]int, error) {
	result, reasons, roots := hxcArchiveTable{payloads: make([]json.RawMessage, 0, spec.expected), forced: map[int]bool{}}, map[string]int{}, map[string]int{}
	seen := map[[sha256.Size]byte]struct{}{}
	ordinal := int64(1)
	err := archive.EachTableRow(ctx, runID, spec.table, func(row v1archive.ArchivedRow) error {
		if reason := hxcArchiveRowReason(row, spec.table, ordinal); reason != "" {
			return errors.New(reason)
		}
		ordinal++
		if _, found := seen[row.SourceKeyHMAC]; found {
			return errors.New(reasonArchiveDuplicate)
		}
		seen[row.SourceKeyHMAC] = struct{}{}
		for _, field := range row.RedactedFields {
			roots[hxcRedactionRoot(spec.table, field)]++
		}
		index := len(result.payloads)
		if spec.candidate && hxcRequiredFieldRedacted(spec.table, row) {
			result.payloads = append(result.payloads, json.RawMessage(`{}`))
			result.forced[index] = true
			reasons[reasonRequiredRedaction]++
			return nil
		}
		result.payloads = append(result.payloads, append(json.RawMessage(nil), row.Payload...))
		return nil
	})
	if err != nil {
		// Reader/decrypt errors can contain source values; only fixed callback
		// codes are returned to the test output.
		switch err.Error() {
		case reasonArchiveScope, reasonArchiveHMAC, reasonArchiveDuplicate:
			return hxcArchiveTable{}, nil, nil, err
		default:
			return hxcArchiveTable{}, nil, nil, errors.New("hxc_history_archive_read_failed")
		}
	}
	return result, reasons, roots, nil
}

func hxcArchiveRowReason(row v1archive.ArchivedRow, table string, ordinal int64) string {
	if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != ordinal || !json.Valid(row.Payload) {
		return reasonArchiveScope
	}
	if row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) {
		return reasonArchiveHMAC
	}
	return ""
}

func hxcRequiredFieldRedacted(table string, row v1archive.ArchivedRow) bool {
	for _, field := range hxcRequiredFields[table] {
		if v1archive.IsRedacted(row, field) {
			return true
		}
	}
	return false
}

func countHXCResults(history History, rows map[string]hxcArchiveTable, reasons map[string]int) (int, int, error) {
	candidates, quarantined := 0, 0
	count := func(table string, values []Disposition, facts []bool) error {
		forced := rows[table].forced
		for index, disposition := range values {
			if forced[index] {
				if disposition != DispositionQuarantine || facts[index] {
					return errors.New(reasonCandidateInvariant)
				}
				quarantined++
				continue
			}
			switch disposition {
			case DispositionCandidate:
				if !facts[index] {
					return errors.New(reasonCandidateInvariant)
				}
				candidates++
			case DispositionQuarantine:
				if facts[index] {
					return errors.New(reasonCandidateInvariant)
				}
				quarantined++
				reasons[reasonInvalidSource]++
			default:
				return errors.New(reasonCandidateInvariant)
			}
		}
		return nil
	}
	if err := count(DashboardMetaTableID, metaStates(history.DashboardMeta), metaFacts(history.DashboardMeta)); err != nil {
		return 0, 0, err
	}
	if err := count(DashboardSnapshotTableID, snapshotStates(history.DashboardSnapshot), snapshotFacts(history.DashboardSnapshot)); err != nil {
		return 0, 0, err
	}
	if err := count(ActivationStatusTableID, activationStates(history.ActivationStatus), activationFacts(history.ActivationStatus)); err != nil {
		return 0, 0, err
	}
	if err := count(HuangxiaocanActivationID, activationStates(history.Huangxiaocan), activationFacts(history.Huangxiaocan)); err != nil {
		return 0, 0, err
	}
	if err := count(ExperienceLeadsTableID, leadStates(history.ExperienceLeads), leadFacts(history.ExperienceLeads)); err != nil {
		return 0, 0, err
	}
	if err := count(ImportBatchesTableID, batchStates(history.ImportBatches), batchFacts(history.ImportBatches)); err != nil {
		return 0, 0, err
	}
	return candidates, quarantined, nil
}

func logHXCPreflightCounts(t *testing.T, specs []hxcArchiveSpec, candidates, quarantined, archived int, reasons, roots map[string]int) {
	for _, spec := range specs {
		t.Logf("table=%s rows=%d", spec.table, spec.expected)
	}
	t.Logf("candidate=%d quarantine=%d archive_only=%d target_writes=0", candidates, quarantined, archived)
	logHXCCountMap(t, "quarantine_reason", reasons)
	logHXCCountMap(t, "redacted_root", roots)
}

func logHXCCountMap(t *testing.T, label string, values map[string]int) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		t.Logf("%s=none count=0", label)
		return
	}
	for _, key := range keys {
		t.Logf("%s=%s count=%d", label, key, values[key])
	}
}

func mergeCounts(target, source map[string]int) {
	for key, value := range source {
		target[key] += value
	}
}

func metaStates(values []Decision[DashboardMetaFact]) []Disposition {
	result := make([]Disposition, len(values))
	for i := range values {
		result[i] = values[i].Disposition
	}
	return result
}
func metaFacts(values []Decision[DashboardMetaFact]) []bool {
	result := make([]bool, len(values))
	for i := range values {
		result[i] = values[i].Fact != nil
	}
	return result
}
func snapshotStates(values []Decision[DashboardSnapshotFact]) []Disposition {
	result := make([]Disposition, len(values))
	for i := range values {
		result[i] = values[i].Disposition
	}
	return result
}
func snapshotFacts(values []Decision[DashboardSnapshotFact]) []bool {
	result := make([]bool, len(values))
	for i := range values {
		result[i] = values[i].Fact != nil
	}
	return result
}
func activationStates(values []Decision[ActivationFact]) []Disposition {
	result := make([]Disposition, len(values))
	for i := range values {
		result[i] = values[i].Disposition
	}
	return result
}
func activationFacts(values []Decision[ActivationFact]) []bool {
	result := make([]bool, len(values))
	for i := range values {
		result[i] = values[i].Fact != nil
	}
	return result
}
func leadStates(values []Decision[ExperienceLeadFact]) []Disposition {
	result := make([]Disposition, len(values))
	for i := range values {
		result[i] = values[i].Disposition
	}
	return result
}
func leadFacts(values []Decision[ExperienceLeadFact]) []bool {
	result := make([]bool, len(values))
	for i := range values {
		result[i] = values[i].Fact != nil
	}
	return result
}
func batchStates(values []Decision[ImportBatchFact]) []Disposition {
	result := make([]Disposition, len(values))
	for i := range values {
		result[i] = values[i].Disposition
	}
	return result
}
func batchFacts(values []Decision[ImportBatchFact]) []bool {
	result := make([]bool, len(values))
	for i := range values {
		result[i] = values[i].Fact != nil
	}
	return result
}

func hxcRedactionRoot(table, path string) string {
	if strings.Contains(path, ".") || strings.Contains(path, "[") {
		return "unknown"
	}
	if _, found := hxcManifestRoots[table][path]; found {
		return path
	}
	return "unknown"
}

var hxcRequiredFields = map[string][]string{
	DashboardMetaTableID:     {"id", "started_at", "finished_at", "status", "row_count", "member_hit", "user_hit", "only_member", "trigger_source"},
	DashboardSnapshotTableID: {"id", "unionid", "refreshed_at", "in_lead_pool", "in_people", "in_questionnaire", "class_term_no", "class_term_label", "crm_hxc_state", "hxc_member_hit", "hxc_user_hit", "funnel_state", "hxc_member_status", "hxc_registered_at", "hxc_last_login_at", "membership_type", "membership_status", "membership_end_at", "membership_days_left", "consultation_used", "consultation_limit", "conv_chat", "conv_consult", "conv_lesson", "msg_user", "msg_ai", "consult_completed", "last_msg_at", "subscription_tier", "subscription_expires_at", "subscription_quota", "subscription_used", "crm_created_at", "last_questionnaire_at", "subscription_period_start"},
	ActivationStatusTableID:  {"id", "mobile", "activation_status", "import_batch_id", "is_active", "created_at", "updated_at"},
	HuangxiaocanActivationID: {"id", "mobile", "activation_state", "import_batch_id", "is_active", "created_at", "updated_at"},
	ExperienceLeadsTableID:   {"id", "mobile", "source_type", "import_batch_id", "is_active", "created_at", "updated_at"},
	ImportBatchesTableID:     {"id", "import_type", "total_rows", "success_rows", "failed_rows", "created_at"},
}

var hxcManifestRoots = map[string]map[string]struct{}{
	DashboardMetaTableID:     roots("id started_at finished_at status row_count member_hit user_hit only_member error_message trigger_source"),
	DashboardSnapshotTableID: roots("id mobile phone_match_key in_lead_pool in_people in_questionnaire customer_name owner_userid is_wecom_added is_mobile_bound class_term_no class_term_label first_entry_source last_entry_source crm_hxc_state crm_created_at questionnaires questionnaire_count last_questionnaire_at hxc_member_hit hxc_user_hit funnel_state hxc_user_id hxc_nickname hxc_member_status hxc_registered_at hxc_last_login_at hxc_silent_days membership_type membership_status membership_end_at membership_days_left membership_source consultation_used consultation_limit conv_chat conv_consult conv_lesson msg_user msg_ai consult_completed consult_avg_turn last_msg_at refreshed_at hxc_member_level hxc_member_expires_at hxc_onboard_status hxc_assessment_status hxc_growth_onboard_status hxc_first_login_at identity_stage monthly_income_range business_focus ai_usage_status main_pain_points ai_pain_points core_painful_scenario focus_topics persona_sketch interaction_style communication_style background_confidence main_line_type main_line_stage main_line_tier main_line_confirmed_at main_line_desc main_line_issue assessment_count latest_assessment_status latest_assessment_score latest_assessment_phase latest_assessment_sub_type latest_assessment_completed_at assessment_dimension_scores subscription_tier subscription_expires_at subscription_quota subscription_used subscription_period_start last_activation_sku_code last_activation_new_tier last_activation_source last_activation_at active_goals_count active_paths_count current_milestone_max active_tasks_count completed_tasks_count task_checkin_count last_task_checkin_at last_task_checkin_mood last_task_checkin_state_score next_review_at last_reviewed_at review_schedule_status last_recent_event_at last_recent_event_type recommended_topic_status recommended_topic_generated_at topic_summary_count last_topic_summary_at last_topic_summary_title primary_role biz_score inner_score trust_score trust_tier clarity_score role_mode growth_credit_balance growth_credit_period_granted growth_credit_period_used growth_credit_period_ends_at webhook_questionnaire_count last_webhook_questionnaire_at last_webhook_questionnaire_status crm_chat_job_count crm_chat_done_count crm_chat_failed_count last_crm_chat_job_status last_crm_chat_job_at last_crm_chat_callback_status unionid"),
	ActivationStatusTableID:  roots("id mobile activation_status activation_remark import_batch_id created_by is_active created_at updated_at"),
	HuangxiaocanActivationID: roots("id mobile activation_state import_batch_id created_by is_active created_at updated_at"),
	ExperienceLeadsTableID:   roots("id mobile source_type import_batch_id created_by is_active created_at updated_at"),
	ImportBatchesTableID:     roots("id import_type file_name total_rows success_rows failed_rows error_summary created_by created_at"),
	SendRecordsTableID:       roots("id record_key task_type outbound_task_ids_json task_results_json selected_count eligible_count sent_count skipped_count skipped_reasons_json include_do_not_disturb content_preview image_count sender_userids_json filter_snapshot_json operator status status_label last_status_sync_at created_at target_unionids_json idempotency_key execution_backend external_effect_job_ids_json external_effect_status_summary_json planned_count queued_count dispatching_count succeeded_count failed_count blocked_count cancelled_count last_refreshed_at target_source target_source_id"),
	SendConfigTableID:        roots("id sender_userid display_name priority is_active created_at updated_at"),
}

func roots(fields string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, field := range strings.Fields(fields) {
		result[field] = struct{}{}
	}
	return result
}

func TestHXCArchiveRowChecksAndRedactionAccounting(t *testing.T) {
	row := v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: DashboardSnapshotTableID, SourceOrdinal: 1, SourceKeyHMAC: [sha256.Size]byte{1}, PayloadHMAC: [sha256.Size]byte{2}, FieldHMAC: [sha256.Size]byte{3}, Payload: []byte(`{}`)}
	if reason := hxcArchiveRowReason(row, DashboardSnapshotTableID, 1); reason != "" {
		t.Fatal(reason)
	}
	if !hxcRequiredFieldRedacted(DashboardSnapshotTableID, v1archive.ArchivedRow{RedactedFields: []string{"unionid"}}) {
		t.Fatal("redacted private resolver input accepted")
	}
	if !hxcRequiredFieldRedacted(DashboardSnapshotTableID, v1archive.ArchivedRow{RedactedFields: []string{"crm_created_at"}}) || !hxcRequiredFieldRedacted(ActivationStatusTableID, v1archive.ArchivedRow{RedactedFields: []string{"import_batch_id"}}) {
		t.Fatal("redacted typed history field accepted")
	}
	if hxcRequiredFieldRedacted(DashboardSnapshotTableID, v1archive.ArchivedRow{RedactedFields: []string{"customer_name"}}) {
		t.Fatal("opaque private display field blocked observation")
	}
	if hxcRedactionRoot(DashboardSnapshotTableID, "membership_status") != "membership_status" || hxcRedactionRoot(DashboardSnapshotTableID, "unknown.nested") != "unknown" || hxcRedactionRoot(DashboardSnapshotTableID, "not_in_manifest") != "unknown" {
		t.Fatal("redaction root accounting changed")
	}
	for _, mutate := range []func(*v1archive.ArchivedRow){func(value *v1archive.ArchivedRow) { value.SourceOrdinal = 2 }, func(value *v1archive.ArchivedRow) { value.SourceKeyHMAC = [sha256.Size]byte{} }} {
		changed := row
		mutate(&changed)
		if hxcArchiveRowReason(changed, DashboardSnapshotTableID, 1) == "" {
			t.Fatal("invalid archive identity accepted")
		}
	}
}
