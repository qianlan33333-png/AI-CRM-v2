package port

import (
	"context"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

type CustomerSurveyAnswer struct {
	SubmissionID    int64
	QuestionnaireID ID
	SubmittedAt     time.Time
	Score           float64
	ChoiceAnswers   []SafeChoiceAnswerPreview
}

// CustomerSurveyAnswerPage is explicitly a bounded recent-candidate view, not
// a complete customer history. Identity and free-text values cannot fit in it.
type CustomerSurveyAnswerPage struct {
	CustomerID      contactport.CustomerID
	Items           []CustomerSurveyAnswer
	Limit           int32
	ScanLimit       int32
	ScannedCount    int32
	MatchedCount    int32
	ScanTruncated   bool
	ResultTruncated bool
}

type CustomerSurveyAnswerReader interface {
	ListCustomerSurveyAnswers(context.Context, contactport.CustomerID, int32) (CustomerSurveyAnswerPage, error)
}
