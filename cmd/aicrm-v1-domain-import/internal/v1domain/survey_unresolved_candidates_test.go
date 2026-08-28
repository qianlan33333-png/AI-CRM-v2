package v1domain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"strconv"
	"testing"
	"time"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var surveyUnresolvedHistoryArchiveRun = flag.String("survey-unresolved-history-archive-run", "", "optional reconciled V2 archive run for Survey unresolved-history candidate preflight")

type surveyUnresolvedArchiveFake struct {
	tables map[string][]v1archive.ArchivedRow
}

func (fake surveyUnresolvedArchiveFake) EachTableRow(ctx context.Context, runID, table string, callback func(v1archive.ArchivedRow) error) error {
	if ctx == nil || runID != "archive-run" {
		return errSurveyUnresolvedCandidateArchive
	}
	for _, row := range fake.tables[table] {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

func TestBuildSurveyUnresolvedCandidatesKeepsWholeSubmissionAndAnswerSnapshots(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 9, 0, 0, 123456000, time.UTC)
	archive := surveyUnresolvedFixture(t, stamp, 999, []int64{100})
	result, err := BuildSurveyUnresolvedCandidates(context.Background(), archive, "archive-run")
	if err != nil {
		t.Fatal("build unresolved candidates")
	}
	if len(result.Submissions) != 1 || len(result.Answers) != 2 {
		t.Fatalf("candidate counts submissions=%d answers=%d", len(result.Submissions), len(result.Answers))
	}
	if result.Submissions[0].Source.SourceID != 201 || !sameSurveyUnresolvedReasons(result.Submissions[0].Reasons, []SurveyUnresolvedReason{SurveyUnresolvedMissingQuestion}) {
		t.Fatal("missing-question submission was not selected exactly")
	}
	if result.SubmissionReasons[SurveyUnresolvedMissingQuestion] != 1 {
		t.Fatal("missing-question count was not preserved")
	}
	first := result.Answers[0]
	if first.Source.SourceID != 301 || first.Source.SubmissionSourceID != 201 || first.Source.QuestionSourceID != 999 ||
		first.Source.QuestionType != "single_choice" || first.Source.QuestionTitleSnapshot != "historical question" ||
		!sameInt64s(first.Source.SelectedOptionIDs, []int64{100}) || !sameStrings(first.Source.SelectedOptionTexts, []string{"historical option"}) ||
		!bytes.Equal(first.Source.SelectedOptionScores, []byte(`[7]`)) || !bytes.Equal(first.Source.SelectedOptionTags, []byte(`[["tag-a"]]`)) ||
		first.Source.TextValue != "historical answer\n" || first.Source.ScoreContribution != 7 || !first.Source.CreatedAt.Equal(stamp) {
		t.Fatal("answer snapshot was changed")
	}
	if first.ArchivedRow.PayloadHMAC != surveyUnresolvedRowBySource(t, archive.tables["public/questionnaire_submission_answers"], 301).PayloadHMAC {
		t.Fatal("answer payload binding changed")
	}
	if result.Answers[1].Source.SourceID != 302 || !sameSurveyUnresolvedReasons(result.Answers[1].SubmissionReasons, []SurveyUnresolvedReason{SurveyUnresolvedMissingQuestion}) {
		t.Fatal("submission group was partially selected")
	}
}

func TestBuildSurveyUnresolvedCandidatesReportsOptionDefinitionGap(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
	archive := surveyUnresolvedFixture(t, stamp, 10, []int64{999})
	result, err := BuildSurveyUnresolvedCandidates(context.Background(), archive, "archive-run")
	if err != nil {
		t.Fatal("build unresolved candidates")
	}
	if len(result.Submissions) != 1 || len(result.Answers) != 2 || result.SubmissionReasons[SurveyUnresolvedMissingOption] != 1 {
		t.Fatal("missing option was not selected as a complete submission group")
	}
	if !sameSurveyUnresolvedReasons(result.Submissions[0].Reasons, []SurveyUnresolvedReason{SurveyUnresolvedMissingOption}) {
		t.Fatal("missing option reason changed")
	}
}

func TestBuildSurveyUnresolvedCandidatesRejectsInvalidEnvelopeAndRetainedRedaction(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
	archive := surveyUnresolvedFixture(t, stamp, 999, []int64{100})
	broken := archive.tables["public/questionnaire_submission_answers"][0]
	broken.FieldHMAC = [sha256.Size]byte{}
	archive.tables["public/questionnaire_submission_answers"][0] = broken
	if _, err := BuildSurveyUnresolvedCandidates(context.Background(), archive, "archive-run"); err == nil {
		t.Fatal("zero field digest was accepted")
	}

	archive = surveyUnresolvedFixture(t, stamp, 999, []int64{100})
	broken = archive.tables["public/questionnaire_submission_answers"][0]
	broken.RedactedFields = []string{"question_id"}
	archive.tables["public/questionnaire_submission_answers"][0] = broken
	if _, err := BuildSurveyUnresolvedCandidates(context.Background(), archive, "archive-run"); err == nil {
		t.Fatal("retained answer field redaction was accepted")
	}

	archive = surveyUnresolvedFixture(t, stamp, 999, []int64{100})
	broken = archive.tables["public/questionnaire_submissions"][1]
	broken.RedactedFields = []string{"result_token"}
	archive.tables["public/questionnaire_submissions"][1] = broken
	if _, err := BuildSurveyUnresolvedCandidates(context.Background(), archive, "archive-run"); err != nil {
		t.Fatal("unused result token redaction changed unresolved selection")
	}
}

func TestReconciledSurveyUnresolvedCandidatesArchive(t *testing.T) {
	if *surveyUnresolvedHistoryArchiveRun == "" {
		t.Skip("supply -survey-unresolved-history-archive-run and V2 archive environment for read-only candidate preflight")
	}
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	archive, err := v1archive.OpenPostgresArchiveReader(context.Background(), environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("cannot open V2 archive for unresolved-history preflight")
	}
	defer archive.Close()
	result, err := BuildSurveyUnresolvedCandidates(context.Background(), archive, *surveyUnresolvedHistoryArchiveRun)
	if err != nil {
		t.Fatal("cannot build Survey unresolved-history candidates")
	}
	if len(result.Submissions) != 1144 || len(result.Answers) != 4327 || len(result.Submissions)+len(result.Answers) != 5471 {
		t.Fatalf("unexpected Survey unresolved-history counts submissions=%d answers=%d total=%d", len(result.Submissions), len(result.Answers), len(result.Submissions)+len(result.Answers))
	}
	t.Logf("survey unresolved-history preflight: submissions=%d answers=%d total=%d missing_question=%d question_type_mismatch=%d missing_option=%d option_parent_mismatch=%d target_writes=0",
		len(result.Submissions), len(result.Answers), len(result.Submissions)+len(result.Answers),
		result.SubmissionReasons[SurveyUnresolvedMissingQuestion], result.SubmissionReasons[SurveyUnresolvedQuestionTypeMismatch],
		result.SubmissionReasons[SurveyUnresolvedMissingOption], result.SubmissionReasons[SurveyUnresolvedOptionParentMismatch])
}

func surveyUnresolvedFixture(t *testing.T, stamp time.Time, badQuestionID int64, selectedOptionIDs []int64) surveyUnresolvedArchiveFake {
	t.Helper()
	tables := map[string][]v1archive.ArchivedRow{}
	tables["public/questionnaires"] = []v1archive.ArchivedRow{surveyUnresolvedRow(t, "public/questionnaires", 1, map[string]any{
		"id": 1, "slug": "survey", "name": "Survey", "title": "Survey", "description": "", "is_disabled": false, "redirect_url": "",
		"created_at": stamp, "updated_at": stamp, "external_push_enabled": false, "external_push_url": "", "external_push_day": nil,
		"external_push_frequency": nil, "external_push_remark": "", "external_push_custom_params": "", "assessment_enabled": false,
		"assessment_config": map[string]any{}, "answer_display_mode": "all_in_one", "external_push_type": "", "external_push_expires_at_ts": nil,
		"completion_target_json": nil, "lead_channel_id": nil, "lead_qr_title": "", "lead_qr_subtitle": "",
	})}
	tables["public/questionnaire_questions"] = []v1archive.ArchivedRow{surveyUnresolvedRow(t, "public/questionnaire_questions", 1, map[string]any{
		"id": 10, "questionnaire_id": 1, "type": "single_choice", "title": "current question", "required": false, "sort_order": 1,
		"created_at": stamp, "updated_at": stamp, "placeholder_text": "", "assessment_dimension_key": "", "sidebar_profile_field": "",
	})}
	tables["public/questionnaire_options"] = []v1archive.ArchivedRow{surveyUnresolvedRow(t, "public/questionnaire_options", 1, map[string]any{
		"id": 100, "question_id": 10, "option_text": "current option", "score": 7, "tag_codes": []string{"tag-a"}, "sort_order": 1,
		"created_at": stamp, "updated_at": stamp, "assessment_type_key": "", "is_other": false, "other_placeholder": "", "other_max_length": 80,
	})}
	tables["public/questionnaire_submissions"] = []v1archive.ArchivedRow{
		surveyUnresolvedSubmissionRow(t, 1, 200, stamp), surveyUnresolvedSubmissionRow(t, 2, 201, stamp),
	}
	tables["public/questionnaire_submission_answers"] = []v1archive.ArchivedRow{
		surveyUnresolvedAnswerRow(t, 1, 300, 200, 10, []int64{100}, stamp),
		surveyUnresolvedAnswerRow(t, 2, 301, 201, badQuestionID, selectedOptionIDs, stamp),
		surveyUnresolvedAnswerRow(t, 3, 302, 201, 10, []int64{100}, stamp),
	}
	return surveyUnresolvedArchiveFake{tables: tables}
}

func surveyUnresolvedSubmissionRow(t *testing.T, ordinal, id int64, stamp time.Time) v1archive.ArchivedRow {
	t.Helper()
	return surveyUnresolvedRow(t, "public/questionnaire_submissions", ordinal, map[string]any{
		"id": id, "questionnaire_id": 1, "unionid": "union", "follow_user_userid": "staff", "matched_by": "unionid", "source_channel": "", "campaign_id": "", "staff_id": "",
		"total_score": 7, "final_tags": []string{"tag-a"}, "redirect_url_snapshot": "", "submitted_at": stamp, "assessment_result_snapshot": map[string]any{}, "result_token": "secret", "created_at": stamp,
	})
}

func surveyUnresolvedAnswerRow(t *testing.T, ordinal, id, submissionID, questionID int64, selected []int64, stamp time.Time) v1archive.ArchivedRow {
	t.Helper()
	return surveyUnresolvedRow(t, "public/questionnaire_submission_answers", ordinal, map[string]any{
		"id": id, "submission_id": submissionID, "question_id": questionID, "question_type": "single_choice", "question_title_snapshot": "historical question",
		"selected_option_ids": selected, "selected_option_texts_snapshot": []string{"historical option"}, "selected_option_scores_snapshot": []int{7}, "selected_option_tags_snapshot": [][]string{{"tag-a"}},
		"text_value": "historical answer\n", "score_contribution": 7, "created_at": stamp,
	})
}

func surveyUnresolvedRow(t *testing.T, table string, ordinal int64, value any) v1archive.ArchivedRow {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal("marshal test archive row")
	}
	seed := table + "/" + strconv.FormatInt(ordinal, 10)
	return v1archive.ArchivedRow{
		AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal,
		SourceKeyHMAC: sha256.Sum256([]byte("source/" + seed)), PayloadHMAC: sha256.Sum256(payload), FieldHMAC: sha256.Sum256([]byte("fields/" + seed)), Payload: payload,
	}
}

func surveyUnresolvedRowBySource(t *testing.T, rows []v1archive.ArchivedRow, sourceID int64) v1archive.ArchivedRow {
	t.Helper()
	for _, row := range rows {
		var value struct {
			ID int64 `json:"id"`
		}
		if json.Unmarshal(row.Payload, &value) == nil && value.ID == sourceID {
			return row
		}
	}
	t.Fatal("test source row missing")
	return v1archive.ArchivedRow{}
}

func sameSurveyUnresolvedReasons(actual, expected []SurveyUnresolvedReason) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index := range actual {
		if actual[index] != expected[index] {
			return false
		}
	}
	return true
}

func sameStrings(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index := range actual {
		if actual[index] != expected[index] {
			return false
		}
	}
	return true
}
