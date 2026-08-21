package port

import "time"

type SafeSubmissionStats struct {
	SubmissionCount   int64      `json:"submission_count"`
	LatestSubmittedAt *time.Time `json:"latest_submitted_at,omitempty"`
	AverageScore      float64    `json:"average_score"`
}

type SafeEnumOptionAggregate struct {
	OptionID       int64 `json:"option_id"`
	SelectionCount int64 `json:"selection_count"`
}

type SafeEnumQuestionAggregate struct {
	QuestionID    int64                     `json:"question_id"`
	QuestionType  QuestionType              `json:"question_type"`
	SortOrder     int                       `json:"sort_order"`
	AnsweredCount int64                     `json:"answered_count"`
	Options       []SafeEnumOptionAggregate `json:"options"`
}

// SafeSubmissionAnalysis is intentionally incapable of carrying respondent
// identity, question/option labels, free text, result tokens, redirect URLs or
// provider receipts.
type SafeSubmissionAnalysis struct {
	OK                       bool                        `json:"ok"`
	QuestionnaireID          ID                          `json:"questionnaire_id"`
	Stats                    SafeSubmissionStats         `json:"stats"`
	Questions                []SafeEnumQuestionAggregate `json:"questions"`
	TotalQuestions           int32                       `json:"total_questions"`
	Limit                    int32                       `json:"limit"`
	Offset                   int32                       `json:"offset"`
	ScannedSubmissionCount   int64                       `json:"scanned_submission_count"`
	AggregationComplete      bool                        `json:"aggregation_complete"`
	Deidentified             bool                        `json:"deidentified"`
	ContainsRawIdentity      bool                        `json:"contains_raw_identity"`
	ContainsFreeText         bool                        `json:"contains_free_text"`
	LocalOnly                bool                        `json:"local_only"`
	RealExternalCallExecuted bool                        `json:"real_external_call_executed"`
}

type SafeExportPreviewField string

const (
	SafePreviewRowNumber       SafeExportPreviewField = "row_number"
	SafePreviewSubmittedAt     SafeExportPreviewField = "submitted_at"
	SafePreviewScore           SafeExportPreviewField = "score"
	SafePreviewChoiceOptionIDs SafeExportPreviewField = "choice_option_ids"
)

type SafeExportPreviewRequest struct {
	Fields []SafeExportPreviewField
	Limit  int32
	Offset int32
}

type SafeChoiceAnswerPreview struct {
	QuestionID   int64        `json:"question_id"`
	QuestionType QuestionType `json:"question_type"`
	SortOrder    int          `json:"sort_order"`
	OptionIDs    []int64      `json:"option_ids"`
}

// Pointer fields make the caller-selected whitelist explicit while retaining a
// closed JSON object. ChoiceOptionIDs never includes textarea/mobile values or
// option labels.
type SafeExportPreviewRow struct {
	RowNumber       *int64                     `json:"row_number,omitempty"`
	SubmittedAt     *time.Time                 `json:"submitted_at,omitempty"`
	Score           *float64                   `json:"score,omitempty"`
	ChoiceOptionIDs *[]SafeChoiceAnswerPreview `json:"choice_option_ids,omitempty"`
}

type SafeExportPreview struct {
	OK                       bool                     `json:"ok"`
	QuestionnaireID          ID                       `json:"questionnaire_id"`
	Fields                   []SafeExportPreviewField `json:"fields"`
	Rows                     []SafeExportPreviewRow   `json:"rows"`
	Total                    int64                    `json:"total"`
	Limit                    int32                    `json:"limit"`
	Offset                   int32                    `json:"offset"`
	HasMore                  bool                     `json:"has_more"`
	FileCreated              bool                     `json:"file_created"`
	Deidentified             bool                     `json:"deidentified"`
	ContainsRawIdentity      bool                     `json:"contains_raw_identity"`
	ContainsFreeText         bool                     `json:"contains_free_text"`
	LocalOnly                bool                     `json:"local_only"`
	RealExternalCallExecuted bool                     `json:"real_external_call_executed"`
}
