package v1domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestBuildSurveyAggregateResolvesSourceRelationsAndDropsRedactedToken(t *testing.T) {
	stamp := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	questionnaire := archivedValue[surveyQuestionnaireJSON]{value: surveyQuestionnaireJSON{
		ID: 1, Slug: "survey", Name: "Survey", Title: "Survey", AnswerDisplayMode: "all_in_one",
		AssessmentConfig: json.RawMessage(`{}`), CreatedAt: stamp, UpdatedAt: stamp,
	}}
	questions := map[int64][]archivedValue[surveyQuestionJSON]{1: {{value: surveyQuestionJSON{
		ID: 2, QuestionnaireID: 1, Type: "single_choice", Title: "Q", SortOrder: 0, CreatedAt: stamp, UpdatedAt: stamp,
	}}}}
	options := map[int64][]archivedValue[surveyOptionJSON]{2: {{value: surveyOptionJSON{
		ID: 3, QuestionID: 2, OptionText: "A", TagCodes: json.RawMessage(`[]`), SortOrder: 0, CreatedAt: stamp, UpdatedAt: stamp,
	}}}}
	submissionArchive := v1archive.ArchivedRow{RedactedFields: []string{"result_token"}}
	submissions := map[int64][]archivedValue[surveySubmissionJSON]{1: {{archive: submissionArchive, value: surveySubmissionJSON{
		ID: 4, QuestionnaireID: 1, ResultToken: "[REDACTED]", FinalTags: json.RawMessage(`[]`), SubmittedAt: stamp, CreatedAt: stamp,
	}}}}
	answers := map[int64][]archivedValue[surveyAnswerJSON]{4: {{value: surveyAnswerJSON{
		ID: 5, SubmissionID: 4, QuestionID: 2, QuestionType: "single_choice", QuestionTitleSnapshot: "Q",
		SelectedOptionIDs: []int64{3}, SelectedOptionTexts: []string{"A"}, CreatedAt: stamp,
	}}}}
	aggregate, rows := buildSurveyAggregate(questionnaire, questions, options, submissions, answers, map[int64]bool{}, map[int64]bool{}, map[int64]bool{}, map[int64]bool{})
	if len(rows) != 5 || len(aggregate.Questions) != 1 || len(aggregate.Options) != 1 || len(aggregate.Submissions) != 1 || len(aggregate.Answers) != 1 {
		t.Fatalf("aggregate/rows = %#v/%d", aggregate, len(rows))
	}
	if aggregate.Submissions[0].ResultToken != "" {
		t.Fatalf("redacted result token retained: %q", aggregate.Submissions[0].ResultToken)
	}
	answer := aggregate.Answers[0]
	if answer.SortOrder != 0 || len(answer.SelectedOptions) != 1 || answer.SelectedOptions[0].SourceOptionID != 3 || answer.SelectedOptions[0].OptionText != "A" {
		t.Fatalf("answer = %#v", answer)
	}
}
