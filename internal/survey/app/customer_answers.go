package app

import (
	"context"
	"errors"
	"math"
	"reflect"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

const (
	CustomerAnswerDefaultLimit int32 = 30
	CustomerAnswerMaximumLimit int32 = 100
	CustomerAnswerScanLimit    int32 = 500
)

var (
	ErrInvalidCustomerAnswerQuery = errors.New("invalid customer survey answer query")
	ErrCustomerAnswersUnavailable = errors.New("customer survey answers unavailable")
)

type CustomerAnswerCandidateStore interface {
	ListRecentCustomerAnswerCandidates(context.Context, int32) ([]CustomerAnswerCandidate, error)
}

// CustomerAnswerCandidate exists only across the Survey store/application
// boundary. Raw hints are consumed by Identity matching and cannot fit in the
// public CustomerSurveyAnswerReader result.
type CustomerAnswerCandidate struct {
	ID              int64
	QuestionnaireID surveyport.ID
	UnionID         string
	ExternalUserID  string
	Mobile          string
	TotalScore      float64
	SubmittedAt     time.Time
	Answers         []surveyport.SubmissionAnswer
}

type CustomerAnswerService struct {
	uow       platformport.UnitOfWork
	store     CustomerAnswerCandidateStore
	matcher   identityport.CustomerMatcher
	weComCorp string
}

func NewCustomerAnswerService(
	uow platformport.UnitOfWork,
	store CustomerAnswerCandidateStore,
	matcher identityport.CustomerMatcher,
	weComCorp string,
) *CustomerAnswerService {
	return &CustomerAnswerService{uow: uow, store: store, matcher: matcher, weComCorp: weComCorp}
}

func (service *CustomerAnswerService) ListCustomerSurveyAnswers(
	ctx context.Context,
	customerID contactport.CustomerID,
	limit int32,
) (surveyport.CustomerSurveyAnswerPage, error) {
	if limit == 0 {
		limit = CustomerAnswerDefaultLimit
	}
	if ctx == nil || customerID <= 0 || limit < 1 || limit > CustomerAnswerMaximumLimit {
		return surveyport.CustomerSurveyAnswerPage{}, ErrInvalidCustomerAnswerQuery
	}
	if service == nil || nilCustomerAnswerDependency(service.uow) || nilCustomerAnswerDependency(service.store) || nilCustomerAnswerDependency(service.matcher) {
		return surveyport.CustomerSurveyAnswerPage{}, ErrCustomerAnswersUnavailable
	}
	if err := ctx.Err(); err != nil {
		return surveyport.CustomerSurveyAnswerPage{}, errors.Join(ErrCustomerAnswersUnavailable, err)
	}

	var candidates []CustomerAnswerCandidate
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var readErr error
		candidates, readErr = service.store.ListRecentCustomerAnswerCandidates(tx, CustomerAnswerScanLimit+1)
		return readErr
	})
	if err != nil || len(candidates) > int(CustomerAnswerScanLimit+1) {
		return surveyport.CustomerSurveyAnswerPage{}, errors.Join(ErrCustomerAnswersUnavailable, err)
	}
	page := surveyport.CustomerSurveyAnswerPage{
		CustomerID: customerID, Limit: limit, ScanLimit: CustomerAnswerScanLimit,
		ScanTruncated: len(candidates) > int(CustomerAnswerScanLimit),
	}
	if page.ScanTruncated {
		candidates = candidates[:CustomerAnswerScanLimit]
	}
	page.ScannedCount = int32(len(candidates))

	var previous time.Time
	var previousID int64
	matchRequests := make([]identityport.CustomerMatchRequest, 0, len(candidates))
	matchIndexes := make([]int, 0, len(candidates))
	for index, candidate := range candidates {
		if !validCustomerAnswerCandidate(candidate) || !previous.IsZero() &&
			(candidate.SubmittedAt.After(previous) || candidate.SubmittedAt.Equal(previous) && candidate.ID >= previousID) {
			return surveyport.CustomerSurveyAnswerPage{}, ErrCustomerAnswersUnavailable
		}
		previous = candidate.SubmittedAt
		previousID = candidate.ID
		matchRequest, ok, safe := customerAnswerMatchRequest(candidate, customerID, service.weComCorp)
		if !safe {
			return surveyport.CustomerSurveyAnswerPage{}, ErrCustomerAnswersUnavailable
		}
		if !ok {
			continue
		}
		matchRequests = append(matchRequests, matchRequest)
		matchIndexes = append(matchIndexes, index)
	}
	matchedCandidates := make([]bool, len(candidates))
	if len(matchRequests) > 0 {
		matches, matchErr := service.matcher.MatchCustomers(ctx, matchRequests)
		if matchErr != nil {
			return surveyport.CustomerSurveyAnswerPage{}, errors.Join(ErrCustomerAnswersUnavailable, matchErr)
		}
		if len(matches) != len(matchRequests) {
			return surveyport.CustomerSurveyAnswerPage{}, ErrCustomerAnswersUnavailable
		}
		for index, matched := range matches {
			matchedCandidates[matchIndexes[index]] = matched
		}
	}

	for index, candidate := range candidates {
		if !matchedCandidates[index] {
			continue
		}
		page.MatchedCount++
		if len(page.Items) >= int(limit) {
			page.ResultTruncated = true
			continue
		}
		answers, projectErr := safeChoiceAnswers(candidate.Answers)
		if projectErr != nil {
			return surveyport.CustomerSurveyAnswerPage{}, ErrCustomerAnswersUnavailable
		}
		page.Items = append(page.Items, surveyport.CustomerSurveyAnswer{
			SubmissionID: candidate.ID, QuestionnaireID: candidate.QuestionnaireID,
			SubmittedAt: candidate.SubmittedAt.UTC(), Score: candidate.TotalScore, ChoiceAnswers: answers,
		})
	}
	return cloneCustomerSurveyAnswerPage(page), nil
}

func customerAnswerMatchRequest(candidate CustomerAnswerCandidate, customerID contactport.CustomerID, corpID string) (identityport.CustomerMatchRequest, bool, bool) {
	request := identityport.CustomerMatchRequest{CustomerID: customerID, LegacyUnionID: candidate.UnionID}
	if candidate.Mobile != "" {
		request.Refs = append(request.Refs, identityport.IDRef{
			Kind: identityport.KindPhone, Scope: "phone:e164", Value: candidate.Mobile,
			Assurance: identityport.AssuranceVerified, Source: "survey.customer_answers",
		})
	}
	if candidate.ExternalUserID != "" && corpID == "" {
		return identityport.CustomerMatchRequest{}, false, false
	}
	if candidate.ExternalUserID != "" {
		request.Refs = append(request.Refs, identityport.IDRef{
			Kind: identityport.KindWeComExternalUserID, Scope: "wecom-corp:" + corpID, Value: candidate.ExternalUserID,
			Assurance: identityport.AssuranceVerified, Source: "survey.customer_answers",
		})
	}
	return request, len(request.Refs) > 0 || request.LegacyUnionID != "", true
}

func validCustomerAnswerCandidate(candidate CustomerAnswerCandidate) bool {
	return candidate.ID > 0 && candidate.QuestionnaireID > 0 && !candidate.SubmittedAt.IsZero() &&
		!math.IsNaN(candidate.TotalScore) && !math.IsInf(candidate.TotalScore, 0)
}

func cloneCustomerSurveyAnswerPage(page surveyport.CustomerSurveyAnswerPage) surveyport.CustomerSurveyAnswerPage {
	result := page
	result.Items = make([]surveyport.CustomerSurveyAnswer, len(page.Items))
	for index := range page.Items {
		result.Items[index] = page.Items[index]
		result.Items[index].ChoiceAnswers = make([]surveyport.SafeChoiceAnswerPreview, len(page.Items[index].ChoiceAnswers))
		for answerIndex := range page.Items[index].ChoiceAnswers {
			result.Items[index].ChoiceAnswers[answerIndex] = page.Items[index].ChoiceAnswers[answerIndex]
			result.Items[index].ChoiceAnswers[answerIndex].OptionIDs = append([]int64(nil), page.Items[index].ChoiceAnswers[answerIndex].OptionIDs...)
		}
	}
	return result
}

func nilCustomerAnswerDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Pointer && reflected.IsNil()
}

var _ surveyport.CustomerSurveyAnswerReader = (*CustomerAnswerService)(nil)
