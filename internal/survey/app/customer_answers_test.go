package app

import (
	"context"
	"errors"
	"testing"
	"time"

	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

func TestCustomerAnswerServiceProjectsOnlyChoiceAnswersAndMarksBounds(t *testing.T) {
	now := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	candidates := []CustomerAnswerCandidate{
		customerAnswerCandidate(3, 31, now, "+8613800138000"),
		customerAnswerCandidate(2, 30, now.Add(-time.Minute), "+8613900139000"),
		customerAnswerCandidate(1, 29, now.Add(-2*time.Minute), "+8613700137000"),
	}
	matcher := &customerAnswerMatcherStub{matchedPhones: map[string]bool{
		"+8613800138000": true, "+8613900139000": true, "+8613700137000": true,
	}}
	service := NewCustomerAnswerService(customerAnswerUoW{}, &customerAnswerStoreStub{items: candidates}, matcher, "corp-1")
	page, err := service.ListCustomerSurveyAnswers(context.Background(), 41, 2)
	if err != nil {
		t.Fatal(err)
	}
	if page.CustomerID != 41 || page.ScannedCount != 3 || page.MatchedCount != 3 || !page.ResultTruncated || page.ScanTruncated || len(page.Items) != 2 {
		t.Fatalf("page=%+v", page)
	}
	if len(page.Items[0].ChoiceAnswers) != 1 || page.Items[0].ChoiceAnswers[0].QuestionType != surveyport.SingleChoice ||
		len(page.Items[0].ChoiceAnswers[0].OptionIDs) != 1 || page.Items[0].ChoiceAnswers[0].OptionIDs[0] != 9 {
		t.Fatalf("safe answers=%+v", page.Items[0].ChoiceAnswers)
	}
	if matcher.calls != 1 || len(matcher.requests) != 3 || matcher.requests[0].Refs[0].Value != "+8613800138000" {
		t.Fatalf("match calls/requests=%d/%+v", matcher.calls, matcher.requests)
	}
}

func TestCustomerAnswerServiceFailsClosedOnMatcherError(t *testing.T) {
	service := NewCustomerAnswerService(customerAnswerUoW{}, &customerAnswerStoreStub{items: []CustomerAnswerCandidate{
		customerAnswerCandidate(1, 29, time.Now().UTC(), "+8613800138000"),
	}}, &customerAnswerMatcherStub{err: errors.New("identity unavailable")}, "corp-1")
	page, err := service.ListCustomerSurveyAnswers(context.Background(), 41, 30)
	if !errors.Is(err, ErrCustomerAnswersUnavailable) || len(page.Items) != 0 {
		t.Fatalf("page/err=%+v/%v", page, err)
	}
}

func TestCustomerAnswerServiceMarksScanTruncationWithoutClaimingCompleteness(t *testing.T) {
	items := make([]CustomerAnswerCandidate, CustomerAnswerScanLimit+1)
	now := time.Now().UTC()
	for index := range items {
		items[index] = customerAnswerCandidate(int64(len(items)-index), 29, now.Add(-time.Duration(index)*time.Second), "+8613800138000")
	}
	service := NewCustomerAnswerService(customerAnswerUoW{}, &customerAnswerStoreStub{items: items}, &customerAnswerMatcherStub{}, "corp-1")
	page, err := service.ListCustomerSurveyAnswers(context.Background(), 41, 30)
	if err != nil || !page.ScanTruncated || page.ScannedCount != CustomerAnswerScanLimit {
		t.Fatalf("page/err=%+v/%v", page, err)
	}
	if matcher := service.matcher.(*customerAnswerMatcherStub); matcher.calls != 1 || len(matcher.requests) != int(CustomerAnswerScanLimit) {
		t.Fatalf("matcher calls/requests=%d/%d", matcher.calls, len(matcher.requests))
	}
}

func TestCustomerAnswerServiceFailsClosedWhenExternalHintHasNoConfiguredScope(t *testing.T) {
	candidate := customerAnswerCandidate(1, 29, time.Now().UTC(), "")
	candidate.ExternalUserID = "external-secret"
	service := NewCustomerAnswerService(customerAnswerUoW{}, &customerAnswerStoreStub{items: []CustomerAnswerCandidate{candidate}}, &customerAnswerMatcherStub{}, "")
	page, err := service.ListCustomerSurveyAnswers(context.Background(), 41, 30)
	if !errors.Is(err, ErrCustomerAnswersUnavailable) || len(page.Items) != 0 {
		t.Fatalf("page/error=%+v/%v", page, err)
	}
}

func customerAnswerCandidate(id int64, questionnaireID surveyport.ID, submittedAt time.Time, phone string) CustomerAnswerCandidate {
	return CustomerAnswerCandidate{
		ID: id, QuestionnaireID: questionnaireID, Mobile: phone, SubmittedAt: submittedAt,
		Answers: []surveyport.SubmissionAnswer{
			{QuestionID: 1, QuestionType: surveyport.SingleChoice, SortOrder: 1, SelectedOptions: []surveyport.SubmissionAnswerOption{{OptionID: 9, OptionText: "secret-label"}}},
			{QuestionID: 2, QuestionType: surveyport.Textarea, SortOrder: 2, TextValue: "secret-free-text"},
			{QuestionID: 3, QuestionType: surveyport.Mobile, SortOrder: 3, TextValue: phone},
		},
	}
}

type customerAnswerUoW struct{}

func (customerAnswerUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type customerAnswerStoreStub struct {
	items []CustomerAnswerCandidate
	err   error
}

func (stub *customerAnswerStoreStub) ListRecentCustomerAnswerCandidates(context.Context, int32) ([]CustomerAnswerCandidate, error) {
	return append([]CustomerAnswerCandidate(nil), stub.items...), stub.err
}

type customerAnswerMatcherStub struct {
	matchedPhones map[string]bool
	err           error
	calls         int
	requests      []identityport.CustomerMatchRequest
}

func (stub *customerAnswerMatcherStub) MatchCustomers(_ context.Context, requests []identityport.CustomerMatchRequest) ([]bool, error) {
	stub.calls++
	stub.requests = append(stub.requests, requests...)
	if stub.err != nil {
		return nil, stub.err
	}
	result := make([]bool, len(requests))
	for index, request := range requests {
		for _, ref := range request.Refs {
			if ref.Kind == identityport.KindPhone && stub.matchedPhones[ref.Value] {
				result[index] = true
			}
		}
	}
	return result, nil
}
