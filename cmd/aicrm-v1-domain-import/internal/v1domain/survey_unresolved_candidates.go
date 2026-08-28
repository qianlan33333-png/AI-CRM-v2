package v1domain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

var errSurveyUnresolvedCandidateArchive = errors.New("invalid survey unresolved history archive")

// SurveyUnresolvedReason describes why a submission cannot be related to the
// current V1 questionnaire definition. It is source-definition evidence, not
// a V2 validation result.
type SurveyUnresolvedReason string

const (
	SurveyUnresolvedMissingQuestion      SurveyUnresolvedReason = "missing_question"
	SurveyUnresolvedQuestionTypeMismatch SurveyUnresolvedReason = "question_type_mismatch"
	SurveyUnresolvedMissingOption        SurveyUnresolvedReason = "missing_option"
	SurveyUnresolvedOptionParentMismatch SurveyUnresolvedReason = "option_parent_mismatch"
)

// SurveyUnresolvedSubmissionSource is a private-domain source snapshot. Its
// identity and authorization material is intentionally not rendered by this
// package; the encrypted ArchivedRow remains the authoritative preservation.
type SurveyUnresolvedSubmissionSource struct {
	SourceID              int64
	QuestionnaireSourceID int64
	UnionID               string
	FollowUserUserID      string
	MatchedBy             string
	SourceChannel         string
	CampaignID            string
	StaffID               string
	TotalScore            float64
	FinalTags             json.RawMessage
	RedirectURLSnapshot   string
	AssessmentResult      json.RawMessage
	SubmittedAt           time.Time
	CreatedAt             time.Time
}

// SurveyUnresolvedAnswerSource retains answer snapshots without making their
// question/option IDs look like current V2 definition IDs.
type SurveyUnresolvedAnswerSource struct {
	SourceID              int64
	SubmissionSourceID    int64
	QuestionSourceID      int64
	QuestionType          string
	QuestionTitleSnapshot string
	SelectedOptionIDs     []int64
	SelectedOptionTexts   []string
	SelectedOptionScores  json.RawMessage
	SelectedOptionTags    json.RawMessage
	TextValue             string
	ScoreContribution     float64
	CreatedAt             time.Time
}

type SurveyUnresolvedSubmissionCandidate struct {
	Source      SurveyUnresolvedSubmissionSource
	ArchivedRow v1archive.ArchivedRow
	Reasons     []SurveyUnresolvedReason
}

type SurveyUnresolvedAnswerCandidate struct {
	Source            SurveyUnresolvedAnswerSource
	ArchivedRow       v1archive.ArchivedRow
	SubmissionReasons []SurveyUnresolvedReason
}

// SurveyUnresolvedCandidates contains only the previously isolated submission
// groups. It never writes receipts or alters the original quarantine.
type SurveyUnresolvedCandidates struct {
	Submissions       []SurveyUnresolvedSubmissionCandidate
	Answers           []SurveyUnresolvedAnswerCandidate
	SubmissionReasons map[SurveyUnresolvedReason]int
}

type surveyUnresolvedSubmissionJSON struct {
	surveySubmissionJSON
	AssessmentResult json.RawMessage `json:"assessment_result_snapshot"`
}

type surveyUnresolvedAnswerJSON struct {
	surveyAnswerJSON
	SelectedOptionScores json.RawMessage `json:"selected_option_scores_snapshot"`
	SelectedOptionTags   json.RawMessage `json:"selected_option_tags_snapshot"`
	ScoreContribution    float64         `json:"score_contribution"`
}

type surveyUnresolvedSchema struct {
	columns  []string
	nullable map[string]bool
	retained map[string]bool
}

func surveyUnresolvedSchemas() map[string]surveyUnresolvedSchema {
	return map[string]surveyUnresolvedSchema{
		"public/questionnaires": {
			columns:  []string{"id", "slug", "name", "title", "description", "is_disabled", "redirect_url", "created_at", "updated_at", "external_push_enabled", "external_push_url", "external_push_day", "external_push_frequency", "external_push_remark", "external_push_custom_params", "assessment_enabled", "assessment_config", "answer_display_mode", "external_push_type", "external_push_expires_at_ts", "completion_target_json", "lead_channel_id", "lead_qr_title", "lead_qr_subtitle"},
			nullable: map[string]bool{"external_push_day": true, "external_push_frequency": true, "external_push_expires_at_ts": true, "completion_target_json": true, "lead_channel_id": true},
			retained: surveyUnresolvedFieldSet("id", "slug", "name", "title", "description", "is_disabled", "created_at", "updated_at", "assessment_enabled", "assessment_config", "answer_display_mode"),
		},
		"public/questionnaire_questions": {
			columns:  []string{"id", "questionnaire_id", "type", "title", "required", "sort_order", "created_at", "updated_at", "placeholder_text", "assessment_dimension_key", "sidebar_profile_field"},
			retained: surveyUnresolvedFieldSet("id", "questionnaire_id", "type", "title", "required", "sort_order", "created_at", "updated_at", "placeholder_text", "assessment_dimension_key", "sidebar_profile_field"),
		},
		"public/questionnaire_options": {
			columns:  []string{"id", "question_id", "option_text", "score", "tag_codes", "sort_order", "created_at", "updated_at", "assessment_type_key", "is_other", "other_placeholder", "other_max_length"},
			retained: surveyUnresolvedFieldSet("id", "question_id", "option_text", "score", "tag_codes", "sort_order", "created_at", "updated_at", "assessment_type_key", "is_other", "other_placeholder", "other_max_length"),
		},
		"public/questionnaire_submissions": {
			columns:  []string{"id", "questionnaire_id", "unionid", "follow_user_userid", "matched_by", "source_channel", "campaign_id", "staff_id", "total_score", "final_tags", "redirect_url_snapshot", "submitted_at", "assessment_result_snapshot", "result_token", "created_at"},
			retained: surveyUnresolvedFieldSet("id", "questionnaire_id", "unionid", "follow_user_userid", "matched_by", "source_channel", "campaign_id", "staff_id", "total_score", "final_tags", "redirect_url_snapshot", "submitted_at", "assessment_result_snapshot", "created_at"),
		},
		"public/questionnaire_submission_answers": {
			columns:  []string{"id", "submission_id", "question_id", "question_type", "question_title_snapshot", "selected_option_ids", "selected_option_texts_snapshot", "selected_option_scores_snapshot", "selected_option_tags_snapshot", "text_value", "score_contribution", "created_at"},
			retained: surveyUnresolvedFieldSet("id", "submission_id", "question_id", "question_type", "question_title_snapshot", "selected_option_ids", "selected_option_texts_snapshot", "selected_option_scores_snapshot", "selected_option_tags_snapshot", "text_value", "score_contribution", "created_at"),
		},
	}
}

func surveyUnresolvedFieldSet(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

// BuildSurveyUnresolvedCandidates repeats the existing submission-group
// partition and returns only its quarantined source rows. It is deliberately
// read-only: no V2 IDs, receipts, or current Survey objects are created.
func BuildSurveyUnresolvedCandidates(ctx context.Context, archive ArchiveSource, archiveRunID string) (SurveyUnresolvedCandidates, error) {
	if archive == nil || archiveRunID == "" {
		return SurveyUnresolvedCandidates{}, errSurveyUnresolvedCandidateArchive
	}
	schemas := surveyUnresolvedSchemas()
	questionnaires, err := readSurveyUnresolvedRows[surveyQuestionnaireJSON](ctx, archive, archiveRunID, "public/questionnaires", schemas["public/questionnaires"])
	if err != nil {
		return SurveyUnresolvedCandidates{}, err
	}
	questions, err := readSurveyUnresolvedRows[surveyQuestionJSON](ctx, archive, archiveRunID, "public/questionnaire_questions", schemas["public/questionnaire_questions"])
	if err != nil {
		return SurveyUnresolvedCandidates{}, err
	}
	options, err := readSurveyUnresolvedRows[surveyOptionJSON](ctx, archive, archiveRunID, "public/questionnaire_options", schemas["public/questionnaire_options"])
	if err != nil {
		return SurveyUnresolvedCandidates{}, err
	}
	submissions, err := readSurveyUnresolvedRows[surveyUnresolvedSubmissionJSON](ctx, archive, archiveRunID, "public/questionnaire_submissions", schemas["public/questionnaire_submissions"])
	if err != nil {
		return SurveyUnresolvedCandidates{}, err
	}
	answers, err := readSurveyUnresolvedRows[surveyUnresolvedAnswerJSON](ctx, archive, archiveRunID, "public/questionnaire_submission_answers", schemas["public/questionnaire_submission_answers"])
	if err != nil {
		return SurveyUnresolvedCandidates{}, err
	}
	if !uniqueSurveySourceIDs(questionnaires, func(value surveyQuestionnaireJSON) int64 { return value.ID }) ||
		!uniqueSurveySourceIDs(questions, func(value surveyQuestionJSON) int64 { return value.ID }) ||
		!uniqueSurveySourceIDs(options, func(value surveyOptionJSON) int64 { return value.ID }) ||
		!uniqueSurveySourceIDs(submissions, func(value surveyUnresolvedSubmissionJSON) int64 { return value.ID }) ||
		!uniqueSurveySourceIDs(answers, func(value surveyUnresolvedAnswerJSON) int64 { return value.ID }) {
		return SurveyUnresolvedCandidates{}, errSurveyUnresolvedCandidateArchive
	}

	baseSubmissions := make([]archivedValue[surveySubmissionJSON], len(submissions))
	submissionSource := make(map[int64]archivedValue[surveyUnresolvedSubmissionJSON], len(submissions))
	for index, row := range submissions {
		if _, exists := submissionSource[row.value.ID]; exists {
			return SurveyUnresolvedCandidates{}, errSurveyUnresolvedCandidateArchive
		}
		baseSubmissions[index] = archivedValue[surveySubmissionJSON]{archive: row.archive, value: row.value.surveySubmissionJSON}
		submissionSource[row.value.ID] = row
	}
	baseAnswers := make([]archivedValue[surveyAnswerJSON], len(answers))
	answerSource := make(map[int64]archivedValue[surveyUnresolvedAnswerJSON], len(answers))
	for index, row := range answers {
		if _, exists := answerSource[row.value.ID]; exists {
			return SurveyUnresolvedCandidates{}, errSurveyUnresolvedCandidateArchive
		}
		baseAnswers[index] = archivedValue[surveyAnswerJSON]{archive: row.archive, value: row.value.surveyAnswerJSON}
		answerSource[row.value.ID] = row
	}

	questionsByParent := groupSurveyQuestions(questions)
	optionsByParent := groupSurveyOptions(options)
	submissionsByParent := groupSurveySubmissions(baseSubmissions)
	answersByParent := groupSurveyAnswers(baseAnswers)
	consumedQuestions, consumedOptions := map[int64]bool{}, map[int64]bool{}
	consumedSubmissions, consumedAnswers := map[int64]bool{}, map[int64]bool{}
	selectedSubmissions := make(map[int64][]SurveyUnresolvedReason)
	selectedAnswers := make(map[int64][]SurveyUnresolvedReason)
	for _, questionnaire := range questionnaires {
		aggregate, rows := buildSurveyAggregate(questionnaire, questionsByParent, optionsByParent, submissionsByParent, answersByParent,
			consumedQuestions, consumedOptions, consumedSubmissions, consumedAnswers)
		partition := partitionSurveyAggregate(aggregate, rows)
		reasons := surveyUnresolvedReasons(aggregate)
		for _, row := range partition.QuarantinedRows {
			switch row.table {
			case "public/questionnaire_submissions":
				if _, exists := submissionSource[row.source]; !exists || len(reasons[row.source]) == 0 {
					return SurveyUnresolvedCandidates{}, errSurveyUnresolvedCandidateArchive
				}
				selectedSubmissions[row.source] = reasons[row.source]
			case "public/questionnaire_submission_answers":
				answer, exists := answerSource[row.source]
				if !exists || len(reasons[answer.value.SubmissionID]) == 0 {
					return SurveyUnresolvedCandidates{}, errSurveyUnresolvedCandidateArchive
				}
				selectedAnswers[row.source] = reasons[answer.value.SubmissionID]
			default:
				return SurveyUnresolvedCandidates{}, errSurveyUnresolvedCandidateArchive
			}
		}
	}
	if len(collectSurveyOrphans(questions, options, baseSubmissions, baseAnswers, consumedQuestions, consumedOptions, consumedSubmissions, consumedAnswers)) != 0 {
		return SurveyUnresolvedCandidates{}, errSurveyUnresolvedCandidateArchive
	}
	if len(selectedSubmissions) == 0 && len(selectedAnswers) != 0 {
		return SurveyUnresolvedCandidates{}, errSurveyUnresolvedCandidateArchive
	}
	result := SurveyUnresolvedCandidates{SubmissionReasons: make(map[SurveyUnresolvedReason]int)}
	for _, row := range submissions {
		reasons, selected := selectedSubmissions[row.value.ID]
		if !selected {
			continue
		}
		result.Submissions = append(result.Submissions, SurveyUnresolvedSubmissionCandidate{
			Source: surveyUnresolvedSubmissionSource(row.value), ArchivedRow: row.archive, Reasons: append([]SurveyUnresolvedReason(nil), reasons...),
		})
		for _, reason := range reasons {
			result.SubmissionReasons[reason]++
		}
	}
	for _, row := range answers {
		reasons, selected := selectedAnswers[row.value.ID]
		if !selected {
			continue
		}
		result.Answers = append(result.Answers, SurveyUnresolvedAnswerCandidate{
			Source: surveyUnresolvedAnswerSource(row.value), ArchivedRow: row.archive, SubmissionReasons: append([]SurveyUnresolvedReason(nil), reasons...),
		})
	}
	if len(result.Submissions) != len(selectedSubmissions) || len(result.Answers) != len(selectedAnswers) {
		return SurveyUnresolvedCandidates{}, errSurveyUnresolvedCandidateArchive
	}
	return result, nil
}

func readSurveyUnresolvedRows[T any](ctx context.Context, archive ArchiveSource, archiveRunID, table string, schema surveyUnresolvedSchema) ([]archivedValue[T], error) {
	result := make([]archivedValue[T], 0)
	expectedOrdinal := int64(1)
	err := archive.EachTableRow(ctx, archiveRunID, table, func(row v1archive.ArchivedRow) error {
		if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != expectedOrdinal ||
			row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) ||
			!json.Valid(row.Payload) || !validSurveyUnresolvedPayload(row, schema) {
			return errSurveyUnresolvedCandidateArchive
		}
		var value T
		if err := json.Unmarshal(row.Payload, &value); err != nil {
			return errSurveyUnresolvedCandidateArchive
		}
		result = append(result, archivedValue[T]{archive: row, value: value})
		expectedOrdinal++
		return nil
	})
	if err != nil {
		return nil, errSurveyUnresolvedCandidateArchive
	}
	return result, nil
}

func validSurveyUnresolvedPayload(row v1archive.ArchivedRow, schema surveyUnresolvedSchema) bool {
	for _, field := range row.RedactedFields {
		if schema.retained[field] {
			return false
		}
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(row.Payload, &object) != nil || len(object) != len(schema.columns) {
		return false
	}
	for _, field := range schema.columns {
		value, found := object[field]
		if !found || (!schema.nullable[field] && bytes.Equal(bytes.TrimSpace(value), []byte("null"))) {
			return false
		}
	}
	return true
}

func uniqueSurveySourceIDs[T any](rows []archivedValue[T], sourceID func(T) int64) bool {
	seen := make(map[int64]bool, len(rows))
	for _, row := range rows {
		value := sourceID(row.value)
		if seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func surveyUnresolvedSubmissionSource(value surveyUnresolvedSubmissionJSON) SurveyUnresolvedSubmissionSource {
	return SurveyUnresolvedSubmissionSource{
		SourceID: value.ID, QuestionnaireSourceID: value.QuestionnaireID, UnionID: value.UnionID, FollowUserUserID: value.FollowUserUserID,
		MatchedBy: value.MatchedBy, SourceChannel: value.SourceChannel, CampaignID: value.CampaignID, StaffID: value.StaffID,
		TotalScore: value.TotalScore, FinalTags: cloneSurveyRaw(value.FinalTags), RedirectURLSnapshot: value.RedirectURLSnapshot,
		AssessmentResult: cloneSurveyRaw(value.AssessmentResult), SubmittedAt: value.SubmittedAt, CreatedAt: value.CreatedAt,
	}
}

func surveyUnresolvedAnswerSource(value surveyUnresolvedAnswerJSON) SurveyUnresolvedAnswerSource {
	return SurveyUnresolvedAnswerSource{
		SourceID: value.ID, SubmissionSourceID: value.SubmissionID, QuestionSourceID: value.QuestionID, QuestionType: value.QuestionType,
		QuestionTitleSnapshot: value.QuestionTitleSnapshot, SelectedOptionIDs: append([]int64(nil), value.SelectedOptionIDs...),
		SelectedOptionTexts: append([]string(nil), value.SelectedOptionTexts...), SelectedOptionScores: cloneSurveyRaw(value.SelectedOptionScores),
		SelectedOptionTags: cloneSurveyRaw(value.SelectedOptionTags), TextValue: value.TextValue, ScoreContribution: value.ScoreContribution,
		CreatedAt: value.CreatedAt,
	}
}

func cloneSurveyRaw(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}

func surveyUnresolvedReasons(aggregate surveyport.ValidatedImportAggregate) map[int64][]SurveyUnresolvedReason {
	questions := make(map[int64]surveyport.ImportQuestion, len(aggregate.Questions))
	for _, question := range aggregate.Questions {
		questions[question.SourceID] = question
	}
	options := make(map[int64]surveyport.ImportOption, len(aggregate.Options))
	for _, option := range aggregate.Options {
		options[option.SourceID] = option
	}
	result := make(map[int64][]SurveyUnresolvedReason)
	for _, answer := range aggregate.Answers {
		var reasons []SurveyUnresolvedReason
		question, found := questions[answer.SourceQuestionID]
		if !found {
			result[answer.SourceSubmissionID] = mergeSurveyUnresolvedReasons(result[answer.SourceSubmissionID], []SurveyUnresolvedReason{SurveyUnresolvedMissingQuestion})
			continue
		} else if question.Type != answer.QuestionType {
			result[answer.SourceSubmissionID] = mergeSurveyUnresolvedReasons(result[answer.SourceSubmissionID], []SurveyUnresolvedReason{SurveyUnresolvedQuestionTypeMismatch})
			continue
		}
		for _, selected := range answer.SelectedOptions {
			option, found := options[selected.SourceOptionID]
			if !found {
				reasons = append(reasons, SurveyUnresolvedMissingOption)
				continue
			}
			if option.SourceQuestionID != answer.SourceQuestionID {
				reasons = append(reasons, SurveyUnresolvedOptionParentMismatch)
			}
		}
		if len(reasons) == 0 {
			continue
		}
		result[answer.SourceSubmissionID] = mergeSurveyUnresolvedReasons(result[answer.SourceSubmissionID], reasons)
	}
	return result
}

func mergeSurveyUnresolvedReasons(existing, additions []SurveyUnresolvedReason) []SurveyUnresolvedReason {
	seen := make(map[SurveyUnresolvedReason]bool, len(existing)+len(additions))
	for _, reason := range existing {
		seen[reason] = true
	}
	for _, reason := range additions {
		seen[reason] = true
	}
	result := make([]SurveyUnresolvedReason, 0, len(seen))
	for reason := range seen {
		result = append(result, reason)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}
