package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

func TestLegacyCustomerProfileQuestionnaireAnswersReturnsSafeReadModelAndExplicitAssessmentDifference(t *testing.T) {
	sentAt := time.Date(2026, time.August, 26, 10, 0, 0, 0, time.UTC)
	reader := &legacyCustomerProfileQuestionnaireAnswersReaderStub{page: surveyport.CustomerSurveyAnswerPage{
		CustomerID: 44, Limit: surveyapp.CustomerAnswerMaximumLimit, ScanLimit: surveyapp.CustomerAnswerScanLimit, ScannedCount: 1, MatchedCount: 1,
		Items: []surveyport.CustomerSurveyAnswer{{SubmissionID: 8, QuestionnaireID: 6, SubmittedAt: sentAt, Score: 7.5,
			ChoiceAnswers: []surveyport.SafeChoiceAnswerPreview{{QuestionID: 3, QuestionType: surveyport.SingleChoice, SortOrder: 0, OptionIDs: []int64{9}}}}},
	}}
	handler := mustLegacyCustomerProfileQuestionnaireAnswersHandler(t, &legacyCustomerProfileMessagesDetailStub{},
		&legacyCustomerProfileMessagesIdentityStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}},
		&legacyCustomerProfileMessagesUnionStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}}, reader)
	response := serveLegacyCustomerProfileQuestionnaireAnswers(handler, authorizedLegacyCustomerProfileQuestionnaireAnswersRequest(http.MethodGet, "unionid=union-secret&external_userid=external-secret"))
	if response.Code != http.StatusOK || reader.calls != 1 || reader.customerID != 44 || reader.limit != surveyapp.CustomerAnswerMaximumLimit {
		t.Fatalf("status/reader=%d/%d/%d/%d body=%s", response.Code, reader.calls, reader.customerID, reader.limit, response.Body.String())
	}
	var body legacyCustomerProfileQuestionnaireAnswersSuccess
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || body.Count != 1 || body.LatestAssessmentResult != nil || body.AssessmentStatus != "v2_assessment_unavailable" ||
		len(body.Answers) != 1 || body.Answers[0].SubmittedAt != sentAt.Format(time.RFC3339) || body.Answers[0].ChoiceAnswers[0].OptionIDs[0] != 9 {
		t.Fatalf("body=%+v", body)
	}
	assertLegacyCustomerProfileQuestionnaireAnswersNoSensitiveFields(t, response.Body.String(), "union-secret", "external-secret")
}

func TestLegacyCustomerProfileQuestionnaireAnswersAcceptsOnlyVerifiedPhoneInputAndFailsClosed(t *testing.T) {
	reader := &legacyCustomerProfileQuestionnaireAnswersReaderStub{page: surveyport.CustomerSurveyAnswerPage{
		CustomerID: 44, Limit: surveyapp.CustomerAnswerMaximumLimit, ScanLimit: surveyapp.CustomerAnswerScanLimit,
	}}
	identity := &legacyCustomerProfileMessagesIdentityStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}}
	handler := mustLegacyCustomerProfileQuestionnaireAnswersHandler(t, &legacyCustomerProfileMessagesDetailStub{}, identity,
		&legacyCustomerProfileMessagesUnionStub{}, reader)
	response := serveLegacyCustomerProfileQuestionnaireAnswers(handler, authorizedLegacyCustomerProfileQuestionnaireAnswersRequest(http.MethodGet, "mobile=%2B8613800138000"))
	if response.Code != http.StatusOK || identity.calls != 1 || identity.ref.Kind != identityport.KindPhone || identity.ref.Value != "+8613800138000" || reader.calls != 1 {
		t.Fatalf("status/identity/reader=%d/%d/%+v/%d body=%s", response.Code, identity.calls, identity.ref, reader.calls, response.Body.String())
	}

	reader.calls = 0
	response = serveLegacyCustomerProfileQuestionnaireAnswers(handler, authorizedLegacyCustomerProfileQuestionnaireAnswersRequest(http.MethodGet, "mobile=13800138000"))
	if response.Code != http.StatusUnprocessableEntity || reader.calls != 0 {
		t.Fatalf("status/reader=%d/%d body=%s", response.Code, reader.calls, response.Body.String())
	}
}

func TestLegacyCustomerProfileQuestionnaireAnswersRejectsUnsafeOrConflictingIdentityBeforeRead(t *testing.T) {
	for _, rawQuery := range []string{"", "user_id=9", "openid=unsafe", "unionid=one&unionid=two"} {
		t.Run(rawQuery, func(t *testing.T) {
			reader := &legacyCustomerProfileQuestionnaireAnswersReaderStub{}
			handler := mustLegacyCustomerProfileQuestionnaireAnswersHandler(t, &legacyCustomerProfileMessagesDetailStub{},
				&legacyCustomerProfileMessagesIdentityStub{}, &legacyCustomerProfileMessagesUnionStub{}, reader)
			response := serveLegacyCustomerProfileQuestionnaireAnswers(handler, authorizedLegacyCustomerProfileQuestionnaireAnswersRequest(http.MethodGet, rawQuery))
			if response.Code != http.StatusUnprocessableEntity || reader.calls != 0 {
				t.Fatalf("query=%q status/reader=%d/%d body=%s", rawQuery, response.Code, reader.calls, response.Body.String())
			}
		})
	}
	reader := &legacyCustomerProfileQuestionnaireAnswersReaderStub{}
	handler := mustLegacyCustomerProfileQuestionnaireAnswersHandler(t, &legacyCustomerProfileMessagesDetailStub{},
		&legacyCustomerProfileMessagesIdentityStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 45}},
		&legacyCustomerProfileMessagesUnionStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}}, reader)
	response := serveLegacyCustomerProfileQuestionnaireAnswers(handler, authorizedLegacyCustomerProfileQuestionnaireAnswersRequest(http.MethodGet, "unionid=one&external_userid=two"))
	if response.Code != http.StatusConflict || reader.calls != 0 {
		t.Fatalf("status/reader=%d/%d body=%s", response.Code, reader.calls, response.Body.String())
	}
}

func TestLegacyCustomerProfileQuestionnaireAnswersRequiresAdminGlobalAndGET(t *testing.T) {
	reader := &legacyCustomerProfileQuestionnaireAnswersReaderStub{}
	handler := mustLegacyCustomerProfileQuestionnaireAnswersHandler(t, &legacyCustomerProfileMessagesDetailStub{},
		&legacyCustomerProfileMessagesIdentityStub{}, &legacyCustomerProfileMessagesUnionStub{}, reader)
	response := serveLegacyCustomerProfileQuestionnaireAnswers(handler, httptest.NewRequest(http.MethodGet, legacyCustomerProfileQuestionnaireAnswersPath+"?unionid=one", nil))
	if response.Code != http.StatusForbidden || reader.calls != 0 {
		t.Fatalf("status/reader=%d/%d", response.Code, reader.calls)
	}
	response = serveLegacyCustomerProfileQuestionnaireAnswers(handler, authorizedLegacyCustomerProfileQuestionnaireAnswersRequest(http.MethodPost, "unionid=one"))
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet || reader.calls != 0 {
		t.Fatalf("status/allow/reader=%d/%q/%d", response.Code, response.Header().Get("Allow"), reader.calls)
	}
}

func mustLegacyCustomerProfileQuestionnaireAnswersHandler(t *testing.T, detail customerDetailApplication, identity identityResolveApplication, union legacyMessageArchiveUnionResolver, reader surveyport.CustomerSurveyAnswerReader) *legacyCustomerProfileQuestionnaireAnswersHandler {
	t.Helper()
	handler, err := newLegacyCustomerProfileQuestionnaireAnswersHandler(detail, identity, union, reader, "corp-test")
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func serveLegacyCustomerProfileQuestionnaireAnswers(handler *legacyCustomerProfileQuestionnaireAnswersHandler, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.Get(response, request)
	return response
}

func authorizedLegacyCustomerProfileQuestionnaireAnswersRequest(method, rawQuery string) *http.Request {
	request := httptest.NewRequest(method, legacyCustomerProfileQuestionnaireAnswersPath, nil)
	request.URL.RawQuery = rawQuery
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, authport.SessionRef("questionnaire-answer-test"))
	ctx, _ = authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal})
	return request.WithContext(ctx)
}

func assertLegacyCustomerProfileQuestionnaireAnswersNoSensitiveFields(t *testing.T, body string, values ...string) {
	t.Helper()
	lower := strings.ToLower(body)
	for _, key := range []string{`"unionid"`, `"external_userid"`, `"mobile"`, `"user_id"`, `"text_value"`, `"option_text"`} {
		if strings.Contains(lower, key) {
			t.Fatalf("forbidden key %s in %s", key, body)
		}
	}
	for _, value := range values {
		if value != "" && strings.Contains(body, value) {
			t.Fatalf("sensitive value %q in %s", value, body)
		}
	}
}

type legacyCustomerProfileQuestionnaireAnswersReaderStub struct {
	page       surveyport.CustomerSurveyAnswerPage
	err        error
	customerID contactport.CustomerID
	limit      int32
	calls      int
}

func (stub *legacyCustomerProfileQuestionnaireAnswersReaderStub) ListCustomerSurveyAnswers(_ context.Context, customerID contactport.CustomerID, limit int32) (surveyport.CustomerSurveyAnswerPage, error) {
	stub.calls++
	stub.customerID, stub.limit = customerID, limit
	return stub.page, stub.err
}

var _ surveyport.CustomerSurveyAnswerReader = (*legacyCustomerProfileQuestionnaireAnswersReaderStub)(nil)
