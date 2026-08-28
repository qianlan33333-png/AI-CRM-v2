package port

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrSurveyUnresolvedHistoryInvalid     = errors.New("invalid survey source history")
	ErrSurveyUnresolvedHistoryConflict    = errors.New("survey source history conflict")
	ErrSurveyUnresolvedHistoryUnavailable = errors.New("survey source history unavailable")
)

// Source snapshots only: source question/option IDs are not resolved V2 definitions.
type HistoricalUnresolvedSurveySubmission struct {
	ID                     int64           `json:"id"`
	SourceKeyDigest        [32]byte        `json:"-"`
	SourcePayloadDigest    [32]byte        `json:"-"`
	SourceFieldDigest      [32]byte        `json:"-"`
	SourceID               int64           `json:"source_id"`
	QuestionnaireSourceID  int64           `json:"questionnaire_source_id"`
	QuestionnaireID        *int64          `json:"questionnaire_id"`
	CustomerID             *int64          `json:"customer_id"`
	MatchedBy              string          `json:"matched_by"`
	SourceChannel          string          `json:"source_channel"`
	TotalScore             float64         `json:"total_score"`
	FinalTags              json.RawMessage `json:"final_tags"`
	SubmittedAt            time.Time       `json:"submitted_at"`
	CreatedAt              time.Time       `json:"created_at"`
	UnionIDDigest          [32]byte        `json:"-"`
	FollowUserUserIDDigest [32]byte        `json:"-"`
	CampaignIDDigest       [32]byte        `json:"-"`
	StaffIDDigest          [32]byte        `json:"-"`
	RedirectURLDigest      [32]byte        `json:"-"`
	AssessmentResultDigest [32]byte        `json:"-"`
}

type HistoricalUnresolvedSurveyAnswer struct {
	ID                    int64           `json:"id"`
	SourceKeyDigest       [32]byte        `json:"-"`
	SourcePayloadDigest   [32]byte        `json:"-"`
	SourceFieldDigest     [32]byte        `json:"-"`
	SourceID              int64           `json:"source_id"`
	SubmissionID          int64           `json:"submission_id"`
	SubmissionSourceID    int64           `json:"submission_source_id"`
	QuestionSourceID      int64           `json:"question_source_id"`
	QuestionType          string          `json:"question_type"`
	QuestionTitleSnapshot string          `json:"question_title_snapshot"`
	SelectedOptionIDs     json.RawMessage `json:"selected_option_ids"`
	SelectedOptionTexts   json.RawMessage `json:"selected_option_texts"`
	SelectedOptionScores  json.RawMessage `json:"selected_option_scores"`
	SelectedOptionTags    json.RawMessage `json:"selected_option_tags"`
	TextValue             string          `json:"text_value"`
	ScoreContribution     float64         `json:"score_contribution"`
	CreatedAt             time.Time       `json:"created_at"`
}

type SurveyUnresolvedHistoryReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}
type SurveyUnresolvedHistoryQuery struct {
	QuestionnaireID *int64
	Limit, Offset   int32
}
type SurveyUnresolvedHistoryJournal interface {
	LoadSurveyUnresolvedHistory(context.Context, string, string) (SurveyUnresolvedHistoryReceipt, bool, error)
	RecordSurveyUnresolvedHistory(context.Context, SurveyUnresolvedHistoryReceipt) error
}
type SurveyUnresolvedHistoryStore interface {
	CreateHistoricalUnresolvedSurveySubmission(context.Context, HistoricalUnresolvedSurveySubmission) (HistoricalUnresolvedSurveySubmission, error)
	GetHistoricalUnresolvedSurveySubmission(context.Context, int64) (HistoricalUnresolvedSurveySubmission, error)
	CreateHistoricalUnresolvedSurveyAnswer(context.Context, HistoricalUnresolvedSurveyAnswer) (HistoricalUnresolvedSurveyAnswer, error)
	GetHistoricalUnresolvedSurveyAnswer(context.Context, int64) (HistoricalUnresolvedSurveyAnswer, error)
}
type SurveyUnresolvedHistoryReader interface {
	GetHistoricalUnresolvedSurveySubmission(context.Context, int64) (HistoricalUnresolvedSurveySubmission, error)
	GetHistoricalUnresolvedSurveyAnswer(context.Context, int64) (HistoricalUnresolvedSurveyAnswer, error)
	ListHistoricalUnresolvedSurveySubmissions(context.Context, SurveyUnresolvedHistoryQuery) ([]HistoricalUnresolvedSurveySubmission, int64, error)
	ListHistoricalUnresolvedSurveyAnswers(context.Context, int64, SurveyUnresolvedHistoryQuery) ([]HistoricalUnresolvedSurveyAnswer, int64, error)
}
