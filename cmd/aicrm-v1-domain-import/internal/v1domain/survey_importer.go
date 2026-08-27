package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

const surveyImportRunID = "v1-domain-a1-survey"

type SurveyImportResult struct {
	ImportedQuestionnaires int
	ImportedQuestions      int
	ImportedOptions        int
	ImportedSubmissions    int
	ImportedAnswers        int
	QuarantinedRows        int
	ReplayedQuestionnaires int
}

type SurveyImporter struct {
	archive  ArchiveSource
	uow      UnitOfWork
	service  *surveyapp.ImportService
	journals map[string]*Journal
	actorID  int64
}

func NewSurveyImporter(archive ArchiveSource, uow UnitOfWork, service *surveyapp.ImportService, journals map[string]*Journal, actorID int64) (*SurveyImporter, error) {
	if archive == nil || uow == nil || service == nil || actorID < 1 {
		return nil, ErrInvalidScope
	}
	for _, table := range surveyTableIDs() {
		if journals[table] == nil {
			return nil, ErrInvalidScope
		}
	}
	return &SurveyImporter{archive: archive, uow: uow, service: service, journals: journals, actorID: actorID}, nil
}

type archivedValue[T any] struct {
	archive v1archive.ArchivedRow
	value   T
}

type surveyQuestionnaireJSON struct {
	ID                int64           `json:"id"`
	Slug              string          `json:"slug"`
	Name              string          `json:"name"`
	Title             string          `json:"title"`
	Description       string          `json:"description"`
	AnswerDisplayMode string          `json:"answer_display_mode"`
	AssessmentEnabled bool            `json:"assessment_enabled"`
	AssessmentConfig  json.RawMessage `json:"assessment_config"`
	IsDisabled        bool            `json:"is_disabled"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type surveyQuestionJSON struct {
	ID                     int64     `json:"id"`
	QuestionnaireID        int64     `json:"questionnaire_id"`
	Type                   string    `json:"type"`
	Title                  string    `json:"title"`
	Required               bool      `json:"required"`
	SortOrder              int       `json:"sort_order"`
	PlaceholderText        string    `json:"placeholder_text"`
	AssessmentDimensionKey string    `json:"assessment_dimension_key"`
	SidebarProfileField    string    `json:"sidebar_profile_field"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type surveyOptionJSON struct {
	ID                 int64           `json:"id"`
	QuestionID         int64           `json:"question_id"`
	OptionText         string          `json:"option_text"`
	Score              float64         `json:"score"`
	AssessmentTypeKey  string          `json:"assessment_type_key"`
	TagCodes           json.RawMessage `json:"tag_codes"`
	IsOther            bool            `json:"is_other"`
	OtherPlaceholder   string          `json:"other_placeholder"`
	OtherMaximumLength int             `json:"other_max_length"`
	SortOrder          int             `json:"sort_order"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type surveySubmissionJSON struct {
	ID                  int64           `json:"id"`
	QuestionnaireID     int64           `json:"questionnaire_id"`
	UnionID             string          `json:"unionid"`
	FollowUserUserID    string          `json:"follow_user_userid"`
	MatchedBy           string          `json:"matched_by"`
	SourceChannel       string          `json:"source_channel"`
	CampaignID          string          `json:"campaign_id"`
	StaffID             string          `json:"staff_id"`
	TotalScore          float64         `json:"total_score"`
	FinalTags           json.RawMessage `json:"final_tags"`
	ResultToken         string          `json:"result_token"`
	RedirectURLSnapshot string          `json:"redirect_url_snapshot"`
	SubmittedAt         time.Time       `json:"submitted_at"`
	CreatedAt           time.Time       `json:"created_at"`
}

type surveyAnswerJSON struct {
	ID                    int64     `json:"id"`
	SubmissionID          int64     `json:"submission_id"`
	QuestionID            int64     `json:"question_id"`
	QuestionType          string    `json:"question_type"`
	QuestionTitleSnapshot string    `json:"question_title_snapshot"`
	SelectedOptionIDs     []int64   `json:"selected_option_ids"`
	SelectedOptionTexts   []string  `json:"selected_option_texts_snapshot"`
	TextValue             string    `json:"text_value"`
	CreatedAt             time.Time `json:"created_at"`
}

func (importer *SurveyImporter) Import(ctx context.Context, archiveRunID string) (SurveyImportResult, error) {
	if importer == nil || archiveRunID == "" {
		return SurveyImportResult{}, ErrInvalidScope
	}
	questionnaires, err := readArchivedValues[surveyQuestionnaireJSON](ctx, importer.archive, archiveRunID, "public/questionnaires")
	if err != nil {
		return SurveyImportResult{}, err
	}
	questions, err := readArchivedValues[surveyQuestionJSON](ctx, importer.archive, archiveRunID, "public/questionnaire_questions")
	if err != nil {
		return SurveyImportResult{}, err
	}
	options, err := readArchivedValues[surveyOptionJSON](ctx, importer.archive, archiveRunID, "public/questionnaire_options")
	if err != nil {
		return SurveyImportResult{}, err
	}
	submissions, err := readArchivedValues[surveySubmissionJSON](ctx, importer.archive, archiveRunID, "public/questionnaire_submissions")
	if err != nil {
		return SurveyImportResult{}, err
	}
	answers, err := readArchivedValues[surveyAnswerJSON](ctx, importer.archive, archiveRunID, "public/questionnaire_submission_answers")
	if err != nil {
		return SurveyImportResult{}, err
	}
	questionsByParent := groupSurveyQuestions(questions)
	optionsByParent := groupSurveyOptions(options)
	submissionsByParent := groupSurveySubmissions(submissions)
	answersByParent := groupSurveyAnswers(answers)
	consumedQuestions, consumedOptions := map[int64]bool{}, map[int64]bool{}
	consumedSubmissions, consumedAnswers := map[int64]bool{}, map[int64]bool{}
	result := SurveyImportResult{}
	for _, questionnaire := range questionnaires {
		aggregate, rows := buildSurveyAggregate(questionnaire, questionsByParent, optionsByParent, submissionsByParent, answersByParent,
			consumedQuestions, consumedOptions, consumedSubmissions, consumedAnswers)
		partition := partitionSurveyAggregate(aggregate, rows)
		if len(partition.QuarantinedRows) > 0 {
			if err := importer.recordSurveyQuarantine(ctx, partition.QuarantinedRows, "survey_definition_history_unresolved"); err != nil {
				return SurveyImportResult{}, err
			}
			result.QuarantinedRows += len(partition.QuarantinedRows)
		}
		imported, importErr := importer.service.Import(ctx, surveyport.ImportRequest{
			MigrationActor: importer.actorID, RunID: surveyImportRunID,
			IdempotencyKey: SourceIdentifier(questionnaire.archive.SourceKeyHMAC), Aggregate: partition.Importable,
		})
		if errors.Is(importErr, surveyapp.ErrInvalidImport) {
			if err := importer.recordSurveyQuarantine(ctx, partition.ImportRows, "invalid_survey_aggregate"); err != nil {
				return SurveyImportResult{}, err
			}
			result.QuarantinedRows += len(partition.ImportRows)
			continue
		}
		if importErr != nil {
			return SurveyImportResult{}, importErr
		}
		if err := importer.recordSurveyImports(ctx, partition.ImportRows, imported); err != nil {
			return SurveyImportResult{}, err
		}
		result.ImportedQuestionnaires++
		result.ImportedQuestions += imported.ImportedQuestions
		result.ImportedOptions += imported.ImportedOptions
		result.ImportedSubmissions += imported.ImportedSubmissions
		result.ImportedAnswers += imported.ImportedAnswers
		if imported.Replayed {
			result.ReplayedQuestionnaires++
		}
	}
	orphans := collectSurveyOrphans(questions, options, submissions, answers, consumedQuestions, consumedOptions, consumedSubmissions, consumedAnswers)
	if len(orphans) > 0 {
		if err := importer.recordSurveyQuarantine(ctx, orphans, "survey_parent_unresolved"); err != nil {
			return SurveyImportResult{}, err
		}
		result.QuarantinedRows += len(orphans)
	}
	return result, nil
}

type surveyArchiveRef struct {
	table   string
	archive v1archive.ArchivedRow
	source  int64
}

// surveyAggregatePartition isolates only submissions whose answer snapshot
// cannot be related to the current V1 definition. The owner importer remains
// responsible for every other domain validation.
type surveyAggregatePartition struct {
	Importable      surveyport.ValidatedImportAggregate
	ImportRows      []surveyArchiveRef
	QuarantinedRows []surveyArchiveRef
}

func buildSurveyAggregate(questionnaire archivedValue[surveyQuestionnaireJSON], questions map[int64][]archivedValue[surveyQuestionJSON], options map[int64][]archivedValue[surveyOptionJSON], submissions map[int64][]archivedValue[surveySubmissionJSON], answers map[int64][]archivedValue[surveyAnswerJSON], consumedQuestions, consumedOptions, consumedSubmissions, consumedAnswers map[int64]bool) (surveyport.ValidatedImportAggregate, []surveyArchiveRef) {
	source := questionnaire.value
	aggregate := surveyport.ValidatedImportAggregate{Questionnaire: surveyport.ImportQuestionnaire{
		SourceID: source.ID, Slug: source.Slug, Name: source.Name, Title: source.Title, Description: source.Description,
		AnswerDisplayMode: surveyport.AnswerDisplayMode(source.AnswerDisplayMode), AssessmentEnabled: source.AssessmentEnabled,
		AssessmentConfig: source.AssessmentConfig, IsDisabled: source.IsDisabled, Version: 1, SubmissionCount: 0,
		CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt,
	}}
	rows := []surveyArchiveRef{{table: "public/questionnaires", archive: questionnaire.archive, source: source.ID}}
	questionSortOrders := map[int64]int{}
	for _, question := range questions[source.ID] {
		value := question.value
		consumedQuestions[value.ID] = true
		// V1 definitions use one-based positions; V2 owns zero-based positions.
		// Subtraction preserves gaps/duplicates for the owner validator to reject.
		questionSortOrders[value.ID] = value.SortOrder - 1
		aggregate.Questions = append(aggregate.Questions, surveyport.ImportQuestion{
			SourceID: value.ID, SourceQuestionnaireID: value.QuestionnaireID, Type: surveyport.QuestionType(value.Type),
			Title: value.Title, Required: value.Required, SortOrder: value.SortOrder - 1, PlaceholderText: value.PlaceholderText,
			AssessmentDimensionKey: value.AssessmentDimensionKey, SidebarProfileField: value.SidebarProfileField,
			Validation: json.RawMessage(`{}`), CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
		})
		rows = append(rows, surveyArchiveRef{table: "public/questionnaire_questions", archive: question.archive, source: value.ID})
		for _, option := range options[value.ID] {
			item := option.value
			consumedOptions[item.ID] = true
			if !item.IsOther {
				// V1 stores inactive input defaults on ordinary options. Keep the
				// original in the archive, not as active V2 input configuration.
				item.OtherPlaceholder, item.OtherMaximumLength = "", 0
			}
			aggregate.Options = append(aggregate.Options, surveyport.ImportOption{
				SourceID: item.ID, SourceQuestionID: item.QuestionID, OptionText: item.OptionText, Score: item.Score,
				AssessmentTypeKey: item.AssessmentTypeKey, TagCodes: item.TagCodes, IsOther: item.IsOther,
				OtherPlaceholder: item.OtherPlaceholder, OtherMaxLength: item.OtherMaximumLength,
				SortOrder: item.SortOrder - 1, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
			})
			rows = append(rows, surveyArchiveRef{table: "public/questionnaire_options", archive: option.archive, source: item.ID})
		}
	}
	for _, submission := range submissions[source.ID] {
		value := submission.value
		consumedSubmissions[value.ID] = true
		resultToken := value.ResultToken
		if v1archive.IsRedacted(submission.archive, "result_token") {
			resultToken = ""
		}
		aggregate.Submissions = append(aggregate.Submissions, surveyport.ImportSubmission{
			SourceID: value.ID, SourceQuestionnaireID: value.QuestionnaireID, UnionID: value.UnionID,
			FollowUserUserID: value.FollowUserUserID, MatchedBy: value.MatchedBy, SourceChannel: value.SourceChannel,
			CampaignID: value.CampaignID, StaffID: value.StaffID, TotalScore: value.TotalScore, FinalTags: value.FinalTags,
			ResultToken: resultToken, RedirectURLSnapshot: value.RedirectURLSnapshot,
			SubmittedAt: value.SubmittedAt, CreatedAt: value.CreatedAt,
		})
		rows = append(rows, surveyArchiveRef{table: "public/questionnaire_submissions", archive: submission.archive, source: value.ID})
		for _, answer := range answers[value.ID] {
			item := answer.value
			consumedAnswers[item.ID] = true
			selected := make([]surveyport.ImportAnswerOption, len(item.SelectedOptionIDs))
			for index, optionID := range item.SelectedOptionIDs {
				text := ""
				if index < len(item.SelectedOptionTexts) {
					text = item.SelectedOptionTexts[index]
				}
				selected[index] = surveyport.ImportAnswerOption{SourceOptionID: optionID, OptionText: text}
			}
			aggregate.Answers = append(aggregate.Answers, surveyport.ImportAnswer{
				SourceID: item.ID, SourceSubmissionID: item.SubmissionID, SourceQuestionID: item.QuestionID,
				QuestionType: surveyport.QuestionType(item.QuestionType), QuestionTitle: item.QuestionTitleSnapshot,
				SortOrder: questionSortOrders[item.QuestionID], SelectedOptions: selected, TextValue: item.TextValue, CreatedAt: item.CreatedAt,
			})
			rows = append(rows, surveyArchiveRef{table: "public/questionnaire_submission_answers", archive: answer.archive, source: item.ID})
		}
	}
	return aggregate, rows
}

func partitionSurveyAggregate(aggregate surveyport.ValidatedImportAggregate, rows []surveyArchiveRef) surveyAggregatePartition {
	questions := make(map[int64]surveyport.ImportQuestion, len(aggregate.Questions))
	for _, question := range aggregate.Questions {
		questions[question.SourceID] = question
	}
	options := make(map[int64]surveyport.ImportOption, len(aggregate.Options))
	for _, option := range aggregate.Options {
		options[option.SourceID] = option
	}

	quarantinedSubmissions := make(map[int64]bool)
	answerSubmission := make(map[int64]int64, len(aggregate.Answers))
	for _, answer := range aggregate.Answers {
		answerSubmission[answer.SourceID] = answer.SourceSubmissionID
		question, questionOK := questions[answer.SourceQuestionID]
		if !questionOK || question.Type != answer.QuestionType {
			quarantinedSubmissions[answer.SourceSubmissionID] = true
			continue
		}
		for _, selected := range answer.SelectedOptions {
			option, optionOK := options[selected.SourceOptionID]
			if !optionOK || option.SourceQuestionID != answer.SourceQuestionID {
				quarantinedSubmissions[answer.SourceSubmissionID] = true
				break
			}
		}
	}

	result := surveyAggregatePartition{Importable: aggregate}
	result.Importable.Submissions = make([]surveyport.ImportSubmission, 0, len(aggregate.Submissions))
	for _, submission := range aggregate.Submissions {
		if !quarantinedSubmissions[submission.SourceID] {
			result.Importable.Submissions = append(result.Importable.Submissions, submission)
		}
	}
	result.Importable.Answers = make([]surveyport.ImportAnswer, 0, len(aggregate.Answers))
	for _, answer := range aggregate.Answers {
		if !quarantinedSubmissions[answer.SourceSubmissionID] {
			result.Importable.Answers = append(result.Importable.Answers, answer)
		}
	}
	for _, row := range rows {
		quarantine := row.table == "public/questionnaire_submissions" && quarantinedSubmissions[row.source]
		if row.table == "public/questionnaire_submission_answers" && quarantinedSubmissions[answerSubmission[row.source]] {
			quarantine = true
		}
		if quarantine {
			result.QuarantinedRows = append(result.QuarantinedRows, row)
			continue
		}
		result.ImportRows = append(result.ImportRows, row)
	}
	return result
}

func (importer *SurveyImporter) recordSurveyQuarantine(ctx context.Context, rows []surveyArchiveRef, reason string) error {
	return importer.uow.Within(ctx, func(tx context.Context) error {
		for _, row := range rows {
			if err := importer.journals[row.table].Record(tx, TerminalReceipt{
				SourceKeyDigest: row.archive.SourceKeyHMAC, PayloadDigest: row.archive.PayloadHMAC,
				Disposition: "quarantine", Reason: reason,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (importer *SurveyImporter) recordSurveyImports(ctx context.Context, rows []surveyArchiveRef, result surveyport.ImportResult) error {
	return importer.uow.Within(ctx, func(tx context.Context) error {
		for _, row := range rows {
			targetID, found := surveyTargetID(row, result.Mapping)
			if !found {
				return ErrConflict
			}
			targetDigest := sha256.Sum256([]byte(row.table + "\x00" + targetID + "\x00" + hex.EncodeToString(row.archive.PayloadHMAC[:])))
			if err := importer.journals[row.table].Record(tx, TerminalReceipt{
				SourceKeyDigest: row.archive.SourceKeyHMAC, PayloadDigest: row.archive.PayloadHMAC,
				Disposition: "import", TargetID: targetID, TargetDigest: targetDigest,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func surveyTargetID(row surveyArchiveRef, mapping surveyport.ImportIDMap) (string, bool) {
	var target int64
	switch row.table {
	case "public/questionnaires":
		target = mapping.Questionnaires[row.source]
	case "public/questionnaire_questions":
		target = mapping.Questions[row.source].TargetID
	case "public/questionnaire_options":
		target = mapping.Options[row.source].TargetID
	case "public/questionnaire_submissions":
		target = mapping.Submissions[row.source].TargetID
	case "public/questionnaire_submission_answers":
		target = mapping.Answers[row.source]
	}
	return strconv.FormatInt(target, 10), target > 0
}

func readArchivedValues[T any](ctx context.Context, archive ArchiveSource, runID, table string) ([]archivedValue[T], error) {
	result := make([]archivedValue[T], 0)
	err := archive.EachTableRow(ctx, runID, table, func(row v1archive.ArchivedRow) error {
		var value T
		if err := json.Unmarshal(row.Payload, &value); err != nil {
			return fmt.Errorf("decode archived %s row %d: %w", table, row.SourceOrdinal, err)
		}
		result = append(result, archivedValue[T]{archive: row, value: value})
		return nil
	})
	return result, err
}

func groupSurveyQuestions(rows []archivedValue[surveyQuestionJSON]) map[int64][]archivedValue[surveyQuestionJSON] {
	result := map[int64][]archivedValue[surveyQuestionJSON]{}
	for _, row := range rows {
		result[row.value.QuestionnaireID] = append(result[row.value.QuestionnaireID], row)
	}
	return result
}
func groupSurveyOptions(rows []archivedValue[surveyOptionJSON]) map[int64][]archivedValue[surveyOptionJSON] {
	result := map[int64][]archivedValue[surveyOptionJSON]{}
	for _, row := range rows {
		result[row.value.QuestionID] = append(result[row.value.QuestionID], row)
	}
	return result
}
func groupSurveySubmissions(rows []archivedValue[surveySubmissionJSON]) map[int64][]archivedValue[surveySubmissionJSON] {
	result := map[int64][]archivedValue[surveySubmissionJSON]{}
	for _, row := range rows {
		result[row.value.QuestionnaireID] = append(result[row.value.QuestionnaireID], row)
	}
	return result
}
func groupSurveyAnswers(rows []archivedValue[surveyAnswerJSON]) map[int64][]archivedValue[surveyAnswerJSON] {
	result := map[int64][]archivedValue[surveyAnswerJSON]{}
	for _, row := range rows {
		result[row.value.SubmissionID] = append(result[row.value.SubmissionID], row)
	}
	return result
}

func collectSurveyOrphans(questions []archivedValue[surveyQuestionJSON], options []archivedValue[surveyOptionJSON], submissions []archivedValue[surveySubmissionJSON], answers []archivedValue[surveyAnswerJSON], consumedQuestions, consumedOptions, consumedSubmissions, consumedAnswers map[int64]bool) []surveyArchiveRef {
	result := make([]surveyArchiveRef, 0)
	for _, row := range questions {
		if !consumedQuestions[row.value.ID] {
			result = append(result, surveyArchiveRef{"public/questionnaire_questions", row.archive, row.value.ID})
		}
	}
	for _, row := range options {
		if !consumedOptions[row.value.ID] {
			result = append(result, surveyArchiveRef{"public/questionnaire_options", row.archive, row.value.ID})
		}
	}
	for _, row := range submissions {
		if !consumedSubmissions[row.value.ID] {
			result = append(result, surveyArchiveRef{"public/questionnaire_submissions", row.archive, row.value.ID})
		}
	}
	for _, row := range answers {
		if !consumedAnswers[row.value.ID] {
			result = append(result, surveyArchiveRef{"public/questionnaire_submission_answers", row.archive, row.value.ID})
		}
	}
	return result
}

func surveyTableIDs() []string {
	return []string{"public/questionnaires", "public/questionnaire_questions", "public/questionnaire_options", "public/questionnaire_submissions", "public/questionnaire_submission_answers"}
}
