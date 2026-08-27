package port

import (
	"encoding/json"
	"time"
)

// ImportQuestionnaire is the validated, history-preserving definition row
// consumed by the Survey-owned importer. SourceID is migration metadata; it
// is never written as a V2 primary key. CreatedBy is intentionally absent:
// the importer receives one explicit migration actor for the whole aggregate.
type ImportQuestionnaire struct {
	SourceID          int64
	Slug              string
	Name              string
	Title             string
	Description       string
	AnswerDisplayMode AnswerDisplayMode
	AssessmentEnabled bool
	AssessmentConfig  json.RawMessage
	IsDisabled        bool
	Version           int64
	SubmissionCount   int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ImportQuestion keeps the source parent relation so a single transaction can
// allocate the questionnaire ID before allocating its child IDs.
type ImportQuestion struct {
	SourceID               int64
	SourceQuestionnaireID  int64
	Type                   QuestionType
	Title                  string
	Required               bool
	SortOrder              int
	PlaceholderText        string
	AssessmentDimensionKey string
	SidebarProfileField    string
	Validation             json.RawMessage
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type ImportOption struct {
	SourceID          int64
	SourceQuestionID  int64
	OptionText        string
	Score             float64
	AssessmentTypeKey string
	TagCodes          json.RawMessage
	IsOther           bool
	OtherPlaceholder  string
	OtherMaxLength    int
	SortOrder         int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ImportSubmission deliberately has no respondent_key/openid/
// external_userid/customer_name fields. Those values are not proven by the V1
// source mapping and therefore stay empty in the V2 snapshot.
type ImportSubmission struct {
	SourceID              int64
	SourceQuestionnaireID int64
	UnionID               string
	FollowUserUserID      string
	MatchedBy             string
	Mobile                string
	SourceChannel         string
	CampaignID            string
	StaffID               string
	TotalScore            float64
	FinalTags             json.RawMessage
	ResultToken           string
	RedirectURLSnapshot   string
	SubmittedAt           time.Time
	CreatedAt             time.Time
}

type ImportAnswerOption struct {
	SourceOptionID int64
	OptionText     string
}

// ImportAnswer keeps source option IDs until the owner writer has allocated
// V2 option IDs in this same transaction. The writer stores the resolved V2
// IDs together with the historical option text snapshot.
type ImportAnswer struct {
	SourceID           int64
	SourceSubmissionID int64
	SourceQuestionID   int64
	QuestionType       QuestionType
	QuestionTitle      string
	SortOrder          int
	SelectedOptions    []ImportAnswerOption
	TextValue          string
	CreatedAt          time.Time
}

// ValidatedImportAggregate is the only data shape accepted by the owner
// importer. It contains candidates and source relation metadata, never raw V1
// rows, provider clients, runtime jobs, or radar click events.
type ValidatedImportAggregate struct {
	Questionnaire ImportQuestionnaire
	Questions     []ImportQuestion
	Options       []ImportOption
	Submissions   []ImportSubmission
	Answers       []ImportAnswer
}

type ImportRequest struct {
	MigrationActor int64
	RunID          string
	IdempotencyKey string
	Aggregate      ValidatedImportAggregate
}

type ImportQuestionReference struct {
	TargetID        int64
	QuestionnaireID int64
	Type            QuestionType
	Title           string
	SortOrder       int
}

type ImportOptionReference struct {
	TargetID   int64
	QuestionID int64
}

type ImportSubmissionReference struct {
	TargetID        int64
	QuestionnaireID int64
}

// ImportIDMap is returned only after the target transaction commits, or from
// a verified completed receipt replay. No V1 ID is ever reused as a V2 ID.
type ImportIDMap struct {
	Questionnaires map[int64]int64
	Questions      map[int64]ImportQuestionReference
	Options        map[int64]ImportOptionReference
	Submissions    map[int64]ImportSubmissionReference
	Answers        map[int64]int64
}

type ImportResult struct {
	ReceiptID           int64
	Replayed            bool
	Mapping             ImportIDMap
	ImportedQuestions   int
	ImportedOptions     int
	ImportedSubmissions int
	ImportedAnswers     int
}
