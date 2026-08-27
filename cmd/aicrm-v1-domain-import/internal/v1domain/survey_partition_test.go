package v1domain

import (
	"testing"

	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

func TestPartitionSurveyAggregateQuarantinesOnlyUnresolvedSubmissionGroup(t *testing.T) {
	aggregate := partitionTestAggregate(
		[]surveyport.ImportSubmission{{SourceID: 200, SourceQuestionnaireID: 1}, {SourceID: 201, SourceQuestionnaireID: 1}},
		[]surveyport.ImportAnswer{
			{SourceID: 300, SourceSubmissionID: 200, SourceQuestionID: 10, QuestionType: surveyport.SingleChoice, SelectedOptions: []surveyport.ImportAnswerOption{{SourceOptionID: 100}}},
			{SourceID: 301, SourceSubmissionID: 201, SourceQuestionID: 999, QuestionType: surveyport.SingleChoice},
			{SourceID: 302, SourceSubmissionID: 201, SourceQuestionID: 10, QuestionType: surveyport.SingleChoice, SelectedOptions: []surveyport.ImportAnswerOption{{SourceOptionID: 100}}},
		},
	)
	rows := partitionTestRows(1, []int64{10}, []int64{100}, []int64{200, 201}, []int64{300, 301, 302})

	partition := partitionSurveyAggregate(aggregate, rows)
	if got := surveySubmissionSources(partition.Importable.Submissions); !sameInt64s(got, []int64{200}) {
		t.Fatalf("importable submissions = %v, want [200]", got)
	}
	if got := surveyAnswerSources(partition.Importable.Answers); !sameInt64s(got, []int64{300}) {
		t.Fatalf("importable answers = %v, want [300]", got)
	}
	if got := surveyRowSources(partition.QuarantinedRows); !sameSurveyRowSources(got, []surveyRowSource{
		{"public/questionnaire_submissions", 201},
		{"public/questionnaire_submission_answers", 301},
		{"public/questionnaire_submission_answers", 302},
	}) {
		t.Fatalf("quarantined rows = %v", got)
	}
	assertSurveyPartitionConservesRows(t, rows, partition)
}

func TestPartitionSurveyAggregateQuarantinesAllAnswersWhenOneSelectedOptionIsMissing(t *testing.T) {
	aggregate := partitionTestAggregate(
		[]surveyport.ImportSubmission{{SourceID: 200, SourceQuestionnaireID: 1}},
		[]surveyport.ImportAnswer{
			{SourceID: 300, SourceSubmissionID: 200, SourceQuestionID: 10, QuestionType: surveyport.SingleChoice, SelectedOptions: []surveyport.ImportAnswerOption{{SourceOptionID: 100}, {SourceOptionID: 101}, {SourceOptionID: 102}}},
			{SourceID: 301, SourceSubmissionID: 200, SourceQuestionID: 10, QuestionType: surveyport.SingleChoice, SelectedOptions: []surveyport.ImportAnswerOption{{SourceOptionID: 100}}},
		},
	)
	rows := partitionTestRows(1, []int64{10}, []int64{100}, []int64{200}, []int64{300, 301})

	partition := partitionSurveyAggregate(aggregate, rows)
	if len(partition.Importable.Submissions) != 0 || len(partition.Importable.Answers) != 0 {
		t.Fatalf("partial submission was imported: %#v", partition.Importable)
	}
	if got := surveyRowSources(partition.QuarantinedRows); !sameSurveyRowSources(got, []surveyRowSource{
		{"public/questionnaire_submissions", 200},
		{"public/questionnaire_submission_answers", 300},
		{"public/questionnaire_submission_answers", 301},
	}) {
		t.Fatalf("quarantined rows = %v", got)
	}
	assertSurveyPartitionConservesRows(t, rows, partition)
}

func TestPartitionSurveyAggregateKeepsDefinitionWhenAllSubmissionsAreUnresolved(t *testing.T) {
	aggregate := partitionTestAggregate(
		[]surveyport.ImportSubmission{{SourceID: 200, SourceQuestionnaireID: 1}},
		[]surveyport.ImportAnswer{{SourceID: 300, SourceSubmissionID: 200, SourceQuestionID: 10, QuestionType: surveyport.MultiChoice}},
	)
	rows := partitionTestRows(1, []int64{10}, []int64{100}, []int64{200}, []int64{300})

	partition := partitionSurveyAggregate(aggregate, rows)
	if len(partition.Importable.Questions) != 1 || len(partition.Importable.Options) != 1 || len(partition.ImportRows) != 3 {
		t.Fatalf("definition was not retained: aggregate=%#v rows=%v", partition.Importable, surveyRowSources(partition.ImportRows))
	}
	if len(partition.Importable.Submissions) != 0 || len(partition.Importable.Answers) != 0 {
		t.Fatalf("unresolved submission survived: %#v", partition.Importable)
	}
	assertSurveyPartitionConservesRows(t, rows, partition)
}

func partitionTestAggregate(submissions []surveyport.ImportSubmission, answers []surveyport.ImportAnswer) surveyport.ValidatedImportAggregate {
	return surveyport.ValidatedImportAggregate{
		Questionnaire: surveyport.ImportQuestionnaire{SourceID: 1},
		Questions:     []surveyport.ImportQuestion{{SourceID: 10, SourceQuestionnaireID: 1, Type: surveyport.SingleChoice}},
		Options:       []surveyport.ImportOption{{SourceID: 100, SourceQuestionID: 10}},
		Submissions:   submissions,
		Answers:       answers,
	}
}

func partitionTestRows(questionnaire int64, questions, options, submissions, answers []int64) []surveyArchiveRef {
	rows := []surveyArchiveRef{{table: "public/questionnaires", source: questionnaire}}
	for _, source := range questions {
		rows = append(rows, surveyArchiveRef{table: "public/questionnaire_questions", source: source})
	}
	for _, source := range options {
		rows = append(rows, surveyArchiveRef{table: "public/questionnaire_options", source: source})
	}
	for _, source := range submissions {
		rows = append(rows, surveyArchiveRef{table: "public/questionnaire_submissions", source: source})
	}
	for _, source := range answers {
		rows = append(rows, surveyArchiveRef{table: "public/questionnaire_submission_answers", source: source})
	}
	return rows
}

func surveySubmissionSources(values []surveyport.ImportSubmission) []int64 {
	result := make([]int64, len(values))
	for index, value := range values {
		result[index] = value.SourceID
	}
	return result
}

func surveyAnswerSources(values []surveyport.ImportAnswer) []int64 {
	result := make([]int64, len(values))
	for index, value := range values {
		result[index] = value.SourceID
	}
	return result
}

type surveyRowSource struct {
	table  string
	source int64
}

func surveyRowSources(rows []surveyArchiveRef) []surveyRowSource {
	result := make([]surveyRowSource, len(rows))
	for index, row := range rows {
		result[index] = surveyRowSource{row.table, row.source}
	}
	return result
}

func assertSurveyPartitionConservesRows(t *testing.T, rows []surveyArchiveRef, partition surveyAggregatePartition) {
	t.Helper()
	seen := make(map[surveyRowSource]bool, len(rows))
	for _, row := range append(append([]surveyArchiveRef{}, partition.ImportRows...), partition.QuarantinedRows...) {
		key := surveyRowSource{row.table, row.source}
		if seen[key] {
			t.Fatalf("source row assigned more than once: %v", key)
		}
		seen[key] = true
	}
	if len(seen) != len(rows) {
		t.Fatalf("assigned rows=%d, source rows=%d", len(seen), len(rows))
	}
	for _, row := range rows {
		if !seen[surveyRowSource{row.table, row.source}] {
			t.Fatalf("source row lost: %s/%d", row.table, row.source)
		}
	}
}

func sameInt64s(got, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func sameSurveyRowSources(got, want []surveyRowSource) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
