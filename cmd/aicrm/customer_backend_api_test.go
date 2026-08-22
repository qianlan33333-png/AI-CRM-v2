package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contacthttp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/http"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

func TestCandidateCustomerMobileFilterResolvesBeforeContactWithoutLeakingPhone(t *testing.T) {
	application := &customerBackendListStub{result: contactapp.CustomerListResult{
		Items: []contactapp.CustomerRecord{}, Watermark: time.Now().UTC(),
	}}
	listHandler, err := contacthttp.NewCustomerListHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	resolver := &customerBackendIdentityStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 42}}
	handler := &candidateHandler{customers: listHandler, customerIdentity: resolver}
	mobile := "+8613800138000"
	request := customerBackendRequest(t, authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, authport.Authorization{
		Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal,
	})
	response := httptest.NewRecorder()
	handler.ListCustomers(response, request, api.ListCustomersParams{Mobile: &mobile})
	if response.Code != http.StatusOK || resolver.calls != 1 || application.calls != 1 {
		t.Fatalf("status/resolver/contact=%d/%d/%d body=%s", response.Code, resolver.calls, application.calls, response.Body.String())
	}
	if resolver.ref.Kind != identityport.KindPhone || resolver.ref.Scope != "phone:e164" || resolver.ref.Value != mobile ||
		application.input.CustomerID == nil || *application.input.CustomerID != 42 || application.input.MatchNone {
		t.Fatalf("identity/contact=%+v/%+v", resolver.ref, application.input)
	}
	if strings.Contains(response.Body.String(), mobile) {
		t.Fatal("mobile escaped into customer list response")
	}
}

func TestCandidateCustomerMobileFilterFailsClosed(t *testing.T) {
	tests := []struct {
		name       string
		principal  authport.Principal
		auth       authport.Authorization
		result     identityport.ResolveResult
		err        error
		wantStatus int
		wantCalls  int
	}{
		{name: "not_found_is_empty", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, auth: authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal}, result: identityport.ResolveResult{Status: identityport.ResolveNotFound}, wantStatus: http.StatusOK, wantCalls: 1},
		{name: "conflict", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, auth: authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal}, result: identityport.ResolveResult{Status: identityport.ResolveConflict}, wantStatus: http.StatusServiceUnavailable, wantCalls: 1},
		{name: "dependency", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, auth: authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal}, err: errors.New("identity unavailable"), wantStatus: http.StatusServiceUnavailable, wantCalls: 1},
		{name: "unauthorized_before_resolution", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, auth: authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeGlobal}, wantStatus: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			application := &customerBackendListStub{result: contactapp.CustomerListResult{Items: []contactapp.CustomerRecord{}, Watermark: time.Now().UTC()}}
			listHandler, err := contacthttp.NewCustomerListHandler(application)
			if err != nil {
				t.Fatal(err)
			}
			resolver := &customerBackendIdentityStub{result: test.result, err: test.err}
			handler := &candidateHandler{customers: listHandler, customerIdentity: resolver}
			mobile := "+8613800138000"
			request := customerBackendRequest(t, test.principal, test.auth)
			response := httptest.NewRecorder()
			handler.ListCustomers(response, request, api.ListCustomersParams{Mobile: &mobile})
			if response.Code != test.wantStatus || resolver.calls != test.wantCalls {
				t.Fatalf("status/resolver/contact=%d/%d/%d body=%s", response.Code, resolver.calls, application.calls, response.Body.String())
			}
			if test.name == "not_found_is_empty" && (application.calls != 1 || !application.input.MatchNone || application.input.CustomerID != nil) {
				t.Fatalf("not-found contact input=%+v", application.input)
			}
			if test.name != "not_found_is_empty" && application.calls != 0 {
				t.Fatalf("contact calls=%d", application.calls)
			}
		})
	}
}

func TestCandidateCustomerMobileFilterRejectsNonCanonicalPhoneBeforeIdentity(t *testing.T) {
	application := &customerBackendListStub{}
	listHandler, err := contacthttp.NewCustomerListHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	resolver := &customerBackendIdentityStub{}
	handler := &candidateHandler{customers: listHandler, customerIdentity: resolver}
	mobile := "+86 13800138000"
	request := customerBackendRequest(t, authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, authport.Authorization{
		Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal,
	})
	response := httptest.NewRecorder()
	handler.ListCustomers(response, request, api.ListCustomersParams{Mobile: &mobile})
	if response.Code != http.StatusBadRequest || resolver.calls != 0 || application.calls != 0 {
		t.Fatalf("status/resolver/contact=%d/%d/%d body=%s", response.Code, resolver.calls, application.calls, response.Body.String())
	}
}

func TestCandidateCustomerSurveyAnswersSafeProjectionAndOwnerScope(t *testing.T) {
	staffID := int64(31)
	detail := &customerBackendDetailStub{}
	reader := &customerBackendAnswersStub{page: surveyport.CustomerSurveyAnswerPage{
		CustomerID: 42, Limit: 30, ScanLimit: 500, ScannedCount: 2, MatchedCount: 1,
		Items: []surveyport.CustomerSurveyAnswer{{
			SubmissionID: 9, QuestionnaireID: 7, SubmittedAt: time.Now().UTC(), Score: 10,
			ChoiceAnswers: []surveyport.SafeChoiceAnswerPreview{{QuestionID: 5, QuestionType: surveyport.SingleChoice, SortOrder: 1, OptionIDs: []int64{6}}},
		}},
	}}
	handler := &candidateHandler{customerDetailReader: detail, customerSurveyAnswers: reader}
	request := customerBackendRequest(t, authport.Principal{AdminUserID: 8, Role: authport.RoleSales, StaffID: &staffID}, authport.Authorization{
		Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: staffID,
	})
	response := httptest.NewRecorder()
	handler.ListCustomerSurveyAnswers(response, request, 42, api.ListCustomerSurveyAnswersParams{})
	if response.Code != http.StatusOK || detail.input.OwnerStaffID == nil || *detail.input.OwnerStaffID != staffID || reader.calls != 1 {
		t.Fatalf("status/detail/reader=%d/%+v/%d body=%s", response.Code, detail.input, reader.calls, response.Body.String())
	}
	body := response.Body.String()
	for _, forbidden := range []string{"respondent", "openid", "unionid", "external_userid", "mobile", "free-text-secret", "option-label-secret"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("unsafe field %q in response: %s", forbidden, body)
		}
	}
	for _, required := range []string{`"identity_values_included":false`, `"free_text_included":false`, `"scan_truncated":false`, `"scan_limit":500`} {
		if !strings.Contains(body, required) {
			t.Fatalf("missing %s in response: %s", required, body)
		}
	}
}

func TestCandidateCustomerSurveyAnswersReturns404BeforeSurveyRead(t *testing.T) {
	detail := &customerBackendDetailStub{err: contactapp.ErrCustomerNotFound}
	reader := &customerBackendAnswersStub{}
	handler := &candidateHandler{customerDetailReader: detail, customerSurveyAnswers: reader}
	request := customerBackendRequest(t, authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, authport.Authorization{
		Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal,
	})
	response := httptest.NewRecorder()
	handler.ListCustomerSurveyAnswers(response, request, 42, api.ListCustomerSurveyAnswersParams{})
	if response.Code != http.StatusNotFound || reader.calls != 0 {
		t.Fatalf("status/reader=%d/%d body=%s", response.Code, reader.calls, response.Body.String())
	}
}

func customerBackendRequest(t *testing.T, principal authport.Principal, authorization authport.Authorization) *http.Request {
	t.Helper()
	ctx := authport.WithAuthenticatedSession(context.Background(), principal, "session")
	var err error
	ctx, err = authport.WithAuthorization(ctx, authorization)
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
}

type customerBackendListStub struct {
	result contactapp.CustomerListResult
	err    error
	calls  int
	input  contactapp.CustomerListInput
}

func (stub *customerBackendListStub) List(_ context.Context, input contactapp.CustomerListInput) (contactapp.CustomerListResult, error) {
	stub.calls++
	stub.input = input
	return stub.result, stub.err
}

type customerBackendIdentityStub struct {
	result identityport.ResolveResult
	err    error
	calls  int
	ref    identityport.IDRef
}

func (stub *customerBackendIdentityStub) Resolve(_ context.Context, ref identityport.IDRef) (identityport.ResolveResult, error) {
	stub.calls++
	stub.ref = ref
	return stub.result, stub.err
}

type customerBackendDetailStub struct {
	input contactapp.CustomerDetailInput
	err   error
}

func (stub *customerBackendDetailStub) Get(_ context.Context, input contactapp.CustomerDetailInput) (contactapp.CustomerDetailStoreResult, error) {
	stub.input = input
	return contactapp.CustomerDetailStoreResult{}, stub.err
}

type customerBackendAnswersStub struct {
	page  surveyport.CustomerSurveyAnswerPage
	err   error
	calls int
}

func (stub *customerBackendAnswersStub) ListCustomerSurveyAnswers(_ context.Context, _ contactport.CustomerID, _ int32) (surveyport.CustomerSurveyAnswerPage, error) {
	stub.calls++
	return stub.page, stub.err
}
