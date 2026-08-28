package v1hxchistory

import (
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var candidateTime = time.Date(2026, 8, 27, 12, 34, 56, 123456000, time.UTC)

func TestAdaptHistoryProducesOnlyObservedFactsAndConservesRows(t *testing.T) {
	meta := raw(t, map[string]any{"id": 1, "started_at": candidateTime, "finished_at": nil, "status": "completed", "row_count": 1326, "member_hit": 11, "user_hit": 12, "only_member": 3, "error_message": "private", "trigger_source": "scheduled"})
	snapshot := validSnapshot(t)
	activation := raw(t, map[string]any{"id": 3, "mobile": "private", "activation_status": "activated", "activation_remark": "private", "import_batch_id": 9, "created_by": "private", "is_active": true, "created_at": candidateTime, "updated_at": candidateTime.Add(time.Hour)})
	huangxiaocan := raw(t, map[string]any{"id": 4, "mobile": "private", "activation_state": "not_activated", "import_batch_id": "legacy", "created_by": "private", "is_active": false, "created_at": candidateTime, "updated_at": candidateTime.Add(time.Hour)})
	lead := raw(t, map[string]any{"id": 5, "mobile": "private", "source_type": "manual_import", "import_batch_id": nil, "created_by": "private", "is_active": true, "created_at": candidateTime, "updated_at": candidateTime.Add(time.Hour)})
	batch := raw(t, map[string]any{"id": 6, "import_type": "lead", "file_name": "private", "total_rows": 4, "success_rows": 3, "failed_rows": 1, "error_summary": "private", "created_by": "private", "created_at": candidateTime})

	history := AdaptHistory([]json.RawMessage{meta}, []json.RawMessage{snapshot}, []json.RawMessage{activation}, []json.RawMessage{huangxiaocan}, []json.RawMessage{lead}, []json.RawMessage{batch})
	if len(history.DashboardMeta) != 1 || len(history.DashboardSnapshot) != 1 || len(history.ActivationStatus) != 1 || len(history.Huangxiaocan) != 1 || len(history.ExperienceLeads) != 1 || len(history.ImportBatches) != 1 {
		t.Fatal("input rows were not conserved")
	}
	if got := history.DashboardSnapshot[0]; got.Disposition != DispositionCandidate || got.Fact == nil || got.Fact.Observation != ObservedSnapshot || !got.Fact.ObservedAt.Equal(candidateTime) || got.Fact.FunnelState != "activated" || got.Fact.MembershipStatus != "active" || got.Fact.ConsultationLimit == nil || *got.Fact.ConsultationLimit != 10 || got.Fact.CRMCreatedAt == nil || *got.Fact.CRMCreatedAt != "2024-02-29" || got.Fact.LastQuestionnaireAt != nil || got.Fact.SubscriptionPeriodStart == nil || *got.Fact.SubscriptionPeriodStart != "2026-08-01" {
		t.Fatalf("snapshot=%#v", got)
	}
	if got := history.ActivationStatus[0]; got.Disposition != DispositionCandidate || got.Fact == nil || got.Fact.SourceTable != ActivationStatusTableID || got.Fact.OriginalState != "activated" || got.Fact.LegacyImportBatchRef == nil || *got.Fact.LegacyImportBatchRef != "9" {
		t.Fatalf("activation=%#v", got)
	}
	if got := history.Huangxiaocan[0]; got.Disposition != DispositionCandidate || got.Fact == nil || got.Fact.SourceTable != HuangxiaocanActivationID || got.Fact.OriginalState != "not_activated" || got.Fact.LegacyImportBatchRef == nil || *got.Fact.LegacyImportBatchRef != "legacy" {
		t.Fatalf("huangxiaocan=%#v", got)
	}
	if got := history.ExperienceLeads[0]; got.Disposition != DispositionCandidate || got.Fact == nil || got.Fact.OriginalType != "manual_import" || got.Fact.LegacyImportBatchRef != nil {
		t.Fatalf("lead=%#v", got)
	}
	if got := history.ImportBatches[0]; got.Disposition != DispositionCandidate || got.Fact == nil || got.Fact.TotalRows != 4 || got.Fact.SuccessRows != 3 || got.Fact.FailedRows != 1 {
		t.Fatalf("batch=%#v", got)
	}
	if got, want := history.DashboardSnapshot[0].Fact.PayloadDigest, OpaqueDigest(sha256.Sum256(snapshot)); got != want {
		t.Fatalf("snapshot digest=%x want=%x", got, want)
	}
	encoded, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private", "unionid", "mobile", "error_message", "file_name", "created_by"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("candidate leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestAdaptHistoryPreservesDateAndBatchReferenceSourceSemantics(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"zero_source_id":     func(value map[string]any) { value["id"] = 0 },
		"negative_source_id": func(value map[string]any) { value["id"] = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			value := validSnapshotMap()
			mutate(value)
			got := AdaptHistory(nil, []json.RawMessage{raw(t, value)}, nil, nil, nil, nil).DashboardSnapshot[0]
			if got.Disposition != DispositionCandidate || got.Fact == nil {
				t.Fatalf("snapshot=%#v", got)
			}
		})
	}

	for name, mutate := range map[string]func(map[string]any){
		"invalid_calendar": func(value map[string]any) { value["crm_created_at"] = "2026-02-29" },
		"empty_date":       func(value map[string]any) { value["last_questionnaire_at"] = "" },
		"missing_date":     func(value map[string]any) { delete(value, "subscription_period_start") },
	} {
		t.Run(name, func(t *testing.T) {
			value := validSnapshotMap()
			mutate(value)
			got := AdaptHistory(nil, []json.RawMessage{raw(t, value)}, nil, nil, nil, nil).DashboardSnapshot[0]
			if got.Disposition != DispositionQuarantine || got.Fact != nil {
				t.Fatalf("snapshot=%#v", got)
			}
		})
	}

	activation := func(id, batch any) json.RawMessage {
		return raw(t, map[string]any{"id": id, "mobile": "private", "activation_status": "activated", "activation_remark": "private", "import_batch_id": batch, "created_by": "private", "is_active": true, "created_at": candidateTime, "updated_at": candidateTime})
	}
	for name, input := range map[string]struct {
		row  json.RawMessage
		want *string
	}{
		"nullable_numeric_null": {activation(0, nil), nil},
		"numeric_negative":      {activation(-1, -7), ptr("-7")},
	} {
		t.Run(name, func(t *testing.T) {
			got := AdaptHistory(nil, nil, []json.RawMessage{input.row}, nil, nil, nil).ActivationStatus[0]
			if got.Disposition != DispositionCandidate || got.Fact == nil || !sameString(got.Fact.LegacyImportBatchRef, input.want) {
				t.Fatalf("activation=%#v", got)
			}
		})
	}

	huang := raw(t, map[string]any{"id": 0, "mobile": "private", "activation_state": "legacy", "import_batch_id": "", "created_by": "private", "is_active": false, "created_at": candidateTime, "updated_at": candidateTime})
	got := AdaptHistory(nil, nil, nil, []json.RawMessage{huang}, nil, nil).Huangxiaocan[0]
	if got.Disposition != DispositionCandidate || got.Fact == nil || got.Fact.LegacyImportBatchRef == nil || *got.Fact.LegacyImportBatchRef != "" {
		t.Fatalf("huangxiaocan=%#v", got)
	}

	meta := raw(t, map[string]any{"id": 0, "started_at": candidateTime, "finished_at": nil, "status": "completed", "row_count": 1, "member_hit": 0, "user_hit": 0, "only_member": 0, "error_message": "private", "trigger_source": "manual"})
	lead := raw(t, map[string]any{"id": -1, "mobile": "private", "source_type": "manual", "import_batch_id": 0, "created_by": "private", "is_active": true, "created_at": candidateTime, "updated_at": candidateTime})
	batch := raw(t, map[string]any{"id": 0, "import_type": "lead", "file_name": "private", "total_rows": 1, "success_rows": 1, "failed_rows": 0, "error_summary": "", "created_by": "private", "created_at": candidateTime})
	history := AdaptHistory([]json.RawMessage{meta}, nil, nil, nil, []json.RawMessage{lead}, []json.RawMessage{batch})
	if history.DashboardMeta[0].Fact == nil || history.ExperienceLeads[0].Fact == nil || history.ImportBatches[0].Fact == nil || history.ExperienceLeads[0].Fact.LegacyImportBatchRef == nil || *history.ExperienceLeads[0].Fact.LegacyImportBatchRef != "0" {
		t.Fatalf("non-positive source ids were rejected: %#v", history)
	}
}

func TestAdaptHistoryQuarantinesMissingOrInvalidFrozenFields(t *testing.T) {
	missingOptional := validSnapshotMap()
	delete(missingOptional, "membership_end_at")
	badMeta := raw(t, map[string]any{"id": 1, "started_at": candidateTime, "finished_at": nil, "status": "completed", "row_count": "bad", "member_hit": 0, "user_hit": 0, "only_member": 0, "trigger_source": "scheduled"})
	badActivation := raw(t, map[string]any{"id": 3, "mobile": "private", "activation_status": "bad\u0000state", "activation_remark": "private", "import_batch_id": nil, "created_by": "private", "is_active": true, "created_at": candidateTime, "updated_at": candidateTime})
	history := AdaptHistory([]json.RawMessage{badMeta}, []json.RawMessage{raw(t, missingOptional)}, []json.RawMessage{badActivation}, nil, nil, nil)
	if got := history.DashboardMeta[0]; got.Disposition != DispositionQuarantine || got.Reason != reasonInvalidSource || got.Fact != nil {
		t.Fatalf("meta=%#v", got)
	}
	if got := history.DashboardSnapshot[0]; got.Disposition != DispositionQuarantine || got.Reason != reasonInvalidSource || got.Fact != nil {
		t.Fatalf("snapshot=%#v", got)
	}
	if got := history.ActivationStatus[0]; got.Disposition != DispositionQuarantine || got.Reason != reasonInvalidSource || got.Fact != nil {
		t.Fatalf("activation=%#v", got)
	}
}

func TestDashboardSnapshotIsReplacementObservationNotCurrentState(t *testing.T) {
	first, second := validSnapshotMap(), validSnapshotMap()
	first["id"], first["refreshed_at"], first["membership_status"] = 7, candidateTime, "expired"
	second["id"], second["refreshed_at"], second["membership_status"] = 8, candidateTime.Add(time.Hour), "active"
	history := AdaptHistory(nil, []json.RawMessage{raw(t, first), raw(t, second)}, nil, nil, nil, nil)
	if history.DashboardSnapshot[0].Fact == nil || history.DashboardSnapshot[1].Fact == nil || history.DashboardSnapshot[0].Fact.Observation != ObservedSnapshot || history.DashboardSnapshot[1].Fact.Observation != ObservedSnapshot || history.DashboardSnapshot[0].Fact.MembershipStatus != "expired" || history.DashboardSnapshot[1].Fact.MembershipStatus != "active" {
		t.Fatalf("snapshots=%#v", history.DashboardSnapshot)
	}
}

func TestClassifyArchiveOnlyTablesNeverCreatesCandidate(t *testing.T) {
	for table, reason := range map[string]string{SendRecordsTableID: reasonArchiveSendHistory, SendConfigTableID: reasonArchiveSenderConfig} {
		got := ClassifyArchiveOnlyTable(table)
		if got.Disposition != DispositionArchive || got.Reason != reason || got.Fact != nil {
			t.Fatalf("%s=%#v", table, got)
		}
	}
	if got := ClassifyArchiveOnlyTable("public/unknown"); got.Disposition != DispositionQuarantine || got.Reason != reasonUnknownArchiveSource {
		t.Fatalf("unknown=%#v", got)
	}
}

func validSnapshot(t *testing.T) json.RawMessage { t.Helper(); return raw(t, validSnapshotMap()) }
func validSnapshotMap() map[string]any {
	return map[string]any{
		"id": 2, "unionid": "private-union", "refreshed_at": candidateTime,
		"in_lead_pool": true, "in_people": false, "in_questionnaire": true,
		"class_term_no": 3, "class_term_label": "term", "crm_hxc_state": "activated",
		"hxc_member_hit": true, "hxc_user_hit": true, "funnel_state": "activated", "hxc_member_status": "active", "hxc_registered_at": candidateTime, "hxc_last_login_at": nil,
		"membership_type": "annual", "membership_status": "active", "membership_end_at": candidateTime.Add(24 * time.Hour), "membership_days_left": 1,
		"consultation_used": 2, "consultation_limit": 10,
		"conv_chat": 1, "conv_consult": 2, "conv_lesson": 3, "msg_user": 4, "msg_ai": 5, "consult_completed": 6, "last_msg_at": nil,
		"subscription_tier": "annual", "subscription_expires_at": candidateTime.Add(24 * time.Hour), "subscription_quota": 20, "subscription_used": 3,
		"crm_created_at": "2024-02-29", "last_questionnaire_at": nil, "subscription_period_start": "2026-08-01",
	}
}
func ptr(value string) *string { return &value }
func sameString(got, want *string) bool {
	if got == nil || want == nil {
		return got == want
	}
	return *got == *want
}
func raw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	result, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
