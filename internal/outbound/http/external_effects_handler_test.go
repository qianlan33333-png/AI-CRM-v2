package outboundhttp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
)

var errExternalEffectsHandlerFixture = errors.New("external effects handler fixture")

type externalEffectsApplicationStub struct {
	page            outboundapp.ExternalEffectJobPage
	diagnostics     outboundapp.ExternalEffectsDiagnostics
	listErr         error
	diagnosticErr   error
	queries         []outboundapp.ExternalEffectJobQuery
	listCalls       int
	diagnosticCalls int
}

func (stub *externalEffectsApplicationStub) ListJobs(_ context.Context, query outboundapp.ExternalEffectJobQuery) (outboundapp.ExternalEffectJobPage, error) {
	stub.listCalls++
	stub.queries = append(stub.queries, query)
	return stub.page, stub.listErr
}

func (stub *externalEffectsApplicationStub) Diagnostics(context.Context) (outboundapp.ExternalEffectsDiagnostics, error) {
	stub.diagnosticCalls++
	return stub.diagnostics, stub.diagnosticErr
}

func TestExternalEffectsJobsAllowsAdminAndOpsWithClosedResponse(t *testing.T) {
	t.Parallel()

	for _, role := range []authport.Role{authport.RoleAdmin, authport.RoleOps} {
		t.Run(string(role), func(t *testing.T) {
			stub := validExternalEffectsApplicationStub()
			stub.page.PageSize = 25
			stub.page.AppliedFilters = outboundapp.ExternalEffectAppliedFilters{
				Status: outboundapp.TaskStatusOutcomeUnknown, Handling: outboundapp.ExternalEffectManualReview,
			}
			handler := mustExternalEffectsHandler(t, stub)
			request := authorizedExternalEffectsRequest(http.MethodGet,
				ExternalEffectsJobsPath+"?status=outcome_unknown&classification=manual_review&limit=25", role)
			response := httptest.NewRecorder()
			handler.Jobs(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", response.Code, response.Body)
			}
			if response.Header().Get("Cache-Control") != "no-store" || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
				t.Fatalf("headers = %#v", response.Header())
			}
			if stub.listCalls != 1 || len(stub.queries) != 1 || stub.queries[0].Status != outboundapp.TaskStatusOutcomeUnknown ||
				stub.queries[0].Handling != outboundapp.ExternalEffectManualReview || stub.queries[0].Limit != 25 {
				t.Fatalf("queries = %+v", stub.queries)
			}
			body := response.Body.String()
			for _, forbidden := range []string{
				"customer_id", "owner_staff_id", "recipient", "message_body", "payload", "provider_token",
				"provider_message_id", "receipt", "external_userid", "unionid", "mobile",
			} {
				if strings.Contains(body, forbidden) {
					t.Fatalf("response leaked %q: %s", forbidden, body)
				}
			}
			var decoded map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded["provider_execution_eligible"] != false || decoded["real_external_call_executed"] != false ||
				decoded["delivery_proven"] != false || decoded["local_fact_only"] != true ||
				decoded["delivery_semantics"] != outboundapp.ExternalEffectsDeliverySemantics {
				t.Fatalf("unsafe flags = %#v", decoded)
			}
		})
	}
}

func TestExternalEffectsDiagnosticsReturnsRiskCountsWithoutDeliveryClaim(t *testing.T) {
	t.Parallel()

	stub := validExternalEffectsApplicationStub()
	handler := mustExternalEffectsHandler(t, stub)
	response := httptest.NewRecorder()
	handler.Diagnostics(response, authorizedExternalEffectsRequest(http.MethodGet, ExternalEffectsDiagnosticsPath, authport.RoleOps))
	if response.Code != http.StatusOK || stub.diagnosticCalls != 1 {
		t.Fatalf("status/calls = %d/%d body=%s", response.Code, stub.diagnosticCalls, response.Body)
	}
	body := response.Body.String()
	for _, expected := range []string{`"outcome_unknown_count":1`, `"manual_review_count":2`, `"manual_review_required":true`, `"provider_execution_eligible":false`, `"real_external_call_executed":false`, `"delivery_proven":false`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("body missing %s: %s", expected, body)
		}
	}
}

func TestExternalEffectsAuthorizationFailsClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		request    *http.Request
		wantStatus int
	}{
		{name: "missing principal", request: httptest.NewRequest(http.MethodGet, ExternalEffectsJobsPath, nil), wantStatus: http.StatusUnauthorized},
		{name: "sales role", request: authorizedExternalEffectsRequest(http.MethodGet, ExternalEffectsJobsPath, authport.RoleSales), wantStatus: http.StatusForbidden},
		{name: "wrong capability", request: externalEffectsRequestWithAuthorization(authport.RoleAdmin, authport.Authorization{Capability: authport.CapabilityOutboundRead, Scope: authport.ScopeGlobal}), wantStatus: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := validExternalEffectsApplicationStub()
			response := httptest.NewRecorder()
			mustExternalEffectsHandler(t, stub).Jobs(response, test.request)
			if response.Code != test.wantStatus || stub.listCalls != 0 {
				t.Fatalf("status/calls = %d/%d body=%s", response.Code, stub.listCalls, response.Body)
			}
		})
	}
}

func TestExternalEffectsJobsParserRejectsNonCanonicalQueries(t *testing.T) {
	t.Parallel()

	validCursor := outboundapp.ExternalEffectsCursorPrefix + strings.Repeat("A", 24)
	tests := []struct {
		name       string
		raw        string
		wantStatus int
	}{
		{name: "unknown key", raw: "owner_userid=7", wantStatus: http.StatusBadRequest},
		{name: "repeated status", raw: "status=pending&status=sent", wantStatus: http.StatusBadRequest},
		{name: "empty status", raw: "status=", wantStatus: http.StatusBadRequest},
		{name: "queued is not an approved status", raw: "status=queued", wantStatus: http.StatusBadRequest},
		{name: "accepted is not an approved status", raw: "status=accepted", wantStatus: http.StatusBadRequest},
		{name: "completed is not an approved status", raw: "status=completed", wantStatus: http.StatusBadRequest},
		{name: "unknown classification", raw: "classification=deliverable", wantStatus: http.StatusBadRequest},
		{name: "incompatible status classification", raw: "status=sent&classification=manual_review", wantStatus: http.StatusBadRequest},
		{name: "leading zero limit", raw: "limit=050", wantStatus: http.StatusBadRequest},
		{name: "zero limit", raw: "limit=0", wantStatus: http.StatusBadRequest},
		{name: "oversize limit", raw: "limit=101", wantStatus: http.StatusBadRequest},
		{name: "space", raw: "status=+pending", wantStatus: http.StatusBadRequest},
		{name: "malformed percent", raw: "status=%zz", wantStatus: http.StatusBadRequest},
		{name: "invalid utf8", raw: "status=%ff", wantStatus: http.StatusBadRequest},
		{name: "empty cursor", raw: "cursor=", wantStatus: http.StatusBadRequest},
		{name: "bad cursor prefix", raw: "cursor=raw-123", wantStatus: http.StatusBadRequest},
		{name: "repeated cursor", raw: "cursor=" + validCursor + "&cursor=" + validCursor, wantStatus: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := validExternalEffectsApplicationStub()
			request := authorizedExternalEffectsRequest(http.MethodGet, ExternalEffectsJobsPath+"?"+test.raw, authport.RoleAdmin)
			response := httptest.NewRecorder()
			mustExternalEffectsHandler(t, stub).Jobs(response, request)
			if response.Code != test.wantStatus || stub.listCalls != 0 {
				t.Fatalf("status/calls = %d/%d body=%s", response.Code, stub.listCalls, response.Body)
			}
		})
	}
}

func TestExternalEffectsHandlerMapsCursorAndDependencyErrors(t *testing.T) {
	t.Parallel()

	validCursor := outboundapp.ExternalEffectsCursorPrefix + strings.Repeat("A", 24)
	for _, test := range []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "cursor", err: outboundapp.ErrInvalidExternalEffectsCursor, wantStatus: http.StatusBadRequest},
		{name: "query", err: outboundapp.ErrInvalidExternalEffectsQuery, wantStatus: http.StatusBadRequest},
		{name: "unavailable", err: errors.Join(outboundapp.ErrExternalEffectsUnavailable, errExternalEffectsHandlerFixture), wantStatus: http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := validExternalEffectsApplicationStub()
			stub.listErr = test.err
			request := authorizedExternalEffectsRequest(http.MethodGet, ExternalEffectsJobsPath+"?cursor="+validCursor, authport.RoleAdmin)
			response := httptest.NewRecorder()
			mustExternalEffectsHandler(t, stub).Jobs(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d body=%s", response.Code, response.Body)
			}
			if strings.Contains(response.Body.String(), errExternalEffectsHandlerFixture.Error()) {
				t.Fatalf("raw dependency error escaped: %s", response.Body)
			}
		})
	}
}

func TestExternalEffectsHandlerRejectsInvalidSuccessfulApplicationsAs503(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*externalEffectsApplicationStub)
		path   string
	}{
		{name: "raw task id", path: ExternalEffectsJobsPath, mutate: func(stub *externalEffectsApplicationStub) { stub.page.Items[0].ID = "42" }},
		{name: "delivery claim", path: ExternalEffectsJobsPath, mutate: func(stub *externalEffectsApplicationStub) { stub.page.DeliveryProven = true }},
		{name: "mismatched classification", path: ExternalEffectsJobsPath, mutate: func(stub *externalEffectsApplicationStub) {
			stub.page.Items[0].Handling = outboundapp.ExternalEffectFrozen
		}},
		{name: "diagnostic sum", path: ExternalEffectsDiagnosticsPath, mutate: func(stub *externalEffectsApplicationStub) { stub.diagnostics.Total++ }},
		{name: "diagnostic delivery claim", path: ExternalEffectsDiagnosticsPath, mutate: func(stub *externalEffectsApplicationStub) { stub.diagnostics.ProviderExecutionEligible = true }},
		{name: "diagnostic unsafe count", path: ExternalEffectsDiagnosticsPath, mutate: func(stub *externalEffectsApplicationStub) {
			stub.diagnostics.ByStatus.Pending = maximumSafeJSONInteger
			stub.diagnostics.ByStatus.RetryableFailed = 1
			stub.diagnostics.Total = maximumSafeJSONInteger
			stub.diagnostics.ByClassification.SafeLocalHandling = maximumSafeJSONInteger
		}},
		{name: "diagnostic unencodable time", path: ExternalEffectsDiagnosticsPath, mutate: func(stub *externalEffectsApplicationStub) {
			stub.diagnostics.GeneratedAt = time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)
		}},
		{name: "job unencodable time", path: ExternalEffectsJobsPath, mutate: func(stub *externalEffectsApplicationStub) {
			stub.page.Items[0].StatusUpdatedAt = time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)
		}},
		{name: "job page size mismatch", path: ExternalEffectsJobsPath, mutate: func(stub *externalEffectsApplicationStub) {
			stub.page.PageSize = 25
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := validExternalEffectsApplicationStub()
			test.mutate(stub)
			request := authorizedExternalEffectsRequest(http.MethodGet, test.path, authport.RoleAdmin)
			response := httptest.NewRecorder()
			handler := mustExternalEffectsHandler(t, stub)
			if test.path == ExternalEffectsJobsPath {
				handler.Jobs(response, request)
			} else {
				handler.Diagnostics(response, request)
			}
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d body=%s", response.Code, response.Body)
			}
		})
	}
}

func TestExternalEffectsDiagnosticsRejectsQueriesAndWritesNeverRun(t *testing.T) {
	t.Parallel()

	stub := validExternalEffectsApplicationStub()
	handler := mustExternalEffectsHandler(t, stub)
	response := httptest.NewRecorder()
	handler.Diagnostics(response, authorizedExternalEffectsRequest(http.MethodGet, ExternalEffectsDiagnosticsPath+"?status=pending", authport.RoleAdmin))
	if response.Code != http.StatusBadRequest || stub.diagnosticCalls != 0 {
		t.Fatalf("query status/calls = %d/%d", response.Code, stub.diagnosticCalls)
	}
	response = httptest.NewRecorder()
	handler.Jobs(response, authorizedExternalEffectsRequest(http.MethodPost, ExternalEffectsJobsPath, authport.RoleAdmin))
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet || stub.listCalls != 0 {
		t.Fatalf("method status/allow/calls = %d/%q/%d", response.Code, response.Header().Get("Allow"), stub.listCalls)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedExternalEffectsRequest(http.MethodGet, ExternalEffectsJobsPath+"/extra", authport.RoleAdmin))
	if response.Code != http.StatusNotFound || stub.listCalls != 0 {
		t.Fatalf("extra path status/calls = %d/%d", response.Code, stub.listCalls)
	}
}

func validExternalEffectsApplicationStub() *externalEffectsApplicationStub {
	now := time.Date(2026, time.August, 21, 5, 0, 0, 0, time.UTC)
	counts := outboundapp.ExternalEffectStatusCounts{Pending: 1, Sending: 1, Sent: 1, RetryableFailed: 1, FinalFailed: 1, OutcomeUnknown: 1, Cancelled: 1}
	return &externalEffectsApplicationStub{
		page: outboundapp.ExternalEffectJobPage{
			Items: []outboundapp.ExternalEffectJob{{
				ID: safeExternalEffectJobID(), Status: outboundapp.TaskStatusOutcomeUnknown,
				Handling: outboundapp.ExternalEffectManualReview, AttemptCount: 2,
				CreatedAt: now, StatusUpdatedAt: now.Add(time.Minute),
			}},
			PageSize:                  outboundapp.ExternalEffectsDefaultLimit,
			AppliedFilters:            outboundapp.ExternalEffectAppliedFilters{},
			ProviderExecutionEligible: false, RealExternalCallExecuted: false, DeliveryProven: false,
			LocalFactOnly: true, DeliverySemantics: outboundapp.ExternalEffectsDeliverySemantics,
		},
		diagnostics: outboundapp.ExternalEffectsDiagnostics{
			Total: 7, ByStatus: counts,
			ByClassification: outboundapp.ExternalEffectClassificationCounts{SafeLocalHandling: 2, Frozen: 3, ManualReview: 2},
			Risk: outboundapp.ExternalEffectRiskSummary{
				Level: outboundapp.ExternalEffectRiskOutcomeUnknownPresent, OutcomeUnknownCount: 1,
				ManualReviewCount: 2, ManualReviewRequired: true,
			},
			GeneratedAt: now, ProviderExecutionEligible: false, RealExternalCallExecuted: false,
			DeliveryProven: false, LocalFactOnly: true, DeliverySemantics: outboundapp.ExternalEffectsDeliverySemantics,
		},
	}
}

func safeExternalEffectJobID() string {
	return outboundapp.ExternalEffectJobIDPrefix + base64.RawURLEncoding.EncodeToString(make([]byte, 16))
}

func mustExternalEffectsHandler(t *testing.T, application ExternalEffectsApplication) *ExternalEffectsHandler {
	t.Helper()
	handler, err := NewExternalEffectsHandler(application)
	if err != nil {
		t.Fatalf("NewExternalEffectsHandler() error = %v", err)
	}
	return handler
}

func authorizedExternalEffectsRequest(method, target string, role authport.Role) *http.Request {
	return externalEffectsRequestWithAuthorizationForTarget(method, target, role, authport.Authorization{
		Capability: authport.CapabilityOperationsRead, Scope: authport.ScopeGlobal,
	})
}

func externalEffectsRequestWithAuthorization(role authport.Role, authorization authport.Authorization) *http.Request {
	return externalEffectsRequestWithAuthorizationForTarget(http.MethodGet, ExternalEffectsJobsPath, role, authorization)
}

func externalEffectsRequestWithAuthorizationForTarget(method, target string, role authport.Role, authorization authport.Authorization) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 7, Role: role}, authport.SessionRef("external-effects-session"))
	ctx, _ = authport.WithAuthorization(ctx, authorization)
	return request.WithContext(ctx)
}
