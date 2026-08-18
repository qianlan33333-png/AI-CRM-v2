// Package port freezes the Survey questionnaire CRUD contract.
package port

import (
	"encoding/json"
	"time"
)

type ID int64
type QuestionType string
type AnswerDisplayMode string

const (
	SingleChoice QuestionType      = "single_choice"
	MultiChoice  QuestionType      = "multi_choice"
	Textarea     QuestionType      = "textarea"
	Mobile       QuestionType      = "mobile"
	AllInOne     AnswerDisplayMode = "all_in_one"
	OneByOne     AnswerDisplayMode = "one_by_one"
)

type Validation struct {
	MinimumSelections *int `json:"min_selections,omitempty"`
	MaximumSelections *int `json:"max_selections,omitempty"`
	MinimumLength     *int `json:"min_length,omitempty"`
	MaximumLength     *int `json:"max_length,omitempty"`
}

type Option struct {
	ID                 ID       `json:"id,omitempty"`
	OptionText         string   `json:"option_text"`
	Score              float64  `json:"score"`
	AssessmentTypeKey  string   `json:"assessment_type_key"`
	TagCodes           []string `json:"tag_codes"`
	IsOther            bool     `json:"is_other"`
	OtherPlaceholder   string   `json:"other_placeholder"`
	OtherMaximumLength int      `json:"other_max_length"`
	SortOrder          int      `json:"sort_order"`
}

type Question struct {
	ID                     ID           `json:"id,omitempty"`
	Type                   QuestionType `json:"type"`
	Title                  string       `json:"title"`
	AssessmentDimensionKey string       `json:"assessment_dimension_key"`
	SidebarProfileField    string       `json:"sidebar_profile_field"`
	Required               bool         `json:"required"`
	SortOrder              int          `json:"sort_order"`
	PlaceholderText        string       `json:"placeholder_text"`
	Validation             Validation   `json:"validation,omitempty"`
	Options                []Option     `json:"options"`
}

type ScoreRule struct {
	MinimumScore *float64 `json:"min_score"`
	MaximumScore *float64 `json:"max_score"`
	TagCodes     []string `json:"tag_codes"`
	SortOrder    int      `json:"sort_order"`
}

type Questionnaire struct {
	ID                ID                `json:"id"`
	Name              string            `json:"name"`
	Title             string            `json:"title"`
	Description       string            `json:"description"`
	AnswerDisplayMode AnswerDisplayMode `json:"answer_display_mode"`
	AssessmentEnabled bool              `json:"assessment_enabled"`
	AssessmentConfig  json.RawMessage   `json:"assessment_config"`
	Slug              string            `json:"slug"`
	IsDisabled        bool              `json:"is_disabled"`
	Questions         []Question        `json:"questions"`
	ScoreRules        []ScoreRule       `json:"score_rules"`
	CreatedBy         int64             `json:"created_by"`
	Version           int64             `json:"version"`
	SubmissionCount   int64             `json:"submission_count"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

type CreateCommand struct {
	Questionnaire
	Actor          int64
	IdempotencyKey string
}

// UpdateCommand deliberately uses the same frozen questionnaire schema as
// create. The legacy editor sends the definition itself, rather than a new
// DTO or patch envelope.
type UpdateCommand struct {
	Questionnaire
	Actor          int64
	IdempotencyKey string
}

type DeleteResult struct {
	Questionnaire Questionnaire
	Deleted       bool
}

type LegacyPage struct {
	Items  []Questionnaire
	Total  int64
	Limit  int32
	Offset int32
}

// SubmissionResult is the aggregate read model for one questionnaire. Rules
// stays empty until the F02 assessment contract exists; the field is frozen so
// the admin envelope never changes shape.
type SubmissionResult struct {
	QuestionnaireID   ID
	SubmissionCount   int64
	LatestSubmittedAt time.Time
	AverageScore      float64
	Rules             []ScoreRule
}

// SubmissionAnswer is the immutable per-question snapshot stored with one
// submission. QuestionID references the definition row captured at submit
// time; it is deliberately not a foreign key because editor writes replace
// definition rows.
type SubmissionAnswer struct {
	QuestionID      int64
	QuestionType    QuestionType
	QuestionTitle   string
	SortOrder       int
	SelectedOptions []SubmissionAnswerOption
	TextValue       string
}

type SubmissionAnswerOption struct {
	OptionID   int64
	OptionText string
}

// Submission is the Survey-owned submission snapshot. Identity fields are
// opaque values captured at submit time; reads never resolve them against
// another domain.
type Submission struct {
	ID                  int64
	QuestionnaireID     ID
	RespondentKey       string
	OpenID              string
	UnionID             string
	ExternalUserID      string
	CustomerName        string
	FollowUserUserID    string
	MatchedBy           string
	Mobile              string
	SourceChannel       string
	CampaignID          string
	StaffID             string
	TotalScore          float64
	FinalTags           []string
	ResultToken         string
	RedirectURLSnapshot string
	SubmittedAt         time.Time
	CreatedAt           time.Time
	Answers             []SubmissionAnswer
}

type SubmissionPage struct {
	Items  []Submission
	Total  int64
	Limit  int32
	Offset int32
}

// SubmissionExportQuestion is the current definition order used for CSV
// question columns.
type SubmissionExportQuestion struct {
	ID        int64
	Title     string
	SortOrder int
}

type SubmissionExportSnapshot struct {
	QuestionnaireID ID
	Slug            string
	Questions       []SubmissionExportQuestion
	Submissions     []Submission
	Total           int64
}

// SubmissionCSVDownload is the fully encoded in-memory CSV response. No file,
// job, or storage object is ever created.
type SubmissionCSVDownload struct {
	Filename    string
	ContentType string
	Body        []byte
	Total       int64
}
