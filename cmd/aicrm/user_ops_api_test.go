package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/userops/domain"
	useropshttp "github.com/qianlan33333-png/AI-CRM-v2/internal/userops/http"
	useropsport "github.com/qianlan33333-png/AI-CRM-v2/internal/userops/port"
)

func TestUserOpsRootWriteUsesSingleCanonicalCSRFGate(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		csrf          string
		csrfErr       error
		wantStatus    int
		wantCSRFCAlls int
		wantLeafCalls int
	}{
		{name: "missing token", wantStatus: http.StatusForbidden},
		{name: "wrong token", csrf: strings.Repeat("B", 43), csrfErr: authport.ErrCSRFInvalid, wantStatus: http.StatusForbidden, wantCSRFCAlls: 1},
		{name: "valid token", csrf: strings.Repeat("A", 43), wantStatus: http.StatusOK, wantCSRFCAlls: 1, wantLeafCalls: 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			auth := newUserOpsRootAuth(authport.Principal{AdminUserID: 41, Role: authport.RoleAdmin})
			auth.csrfErr = testCase.csrfErr
			application := &userOpsRootApplication{}
			handler := newUserOpsRootHandler(t, auth, application)

			request := httptest.NewRequest(http.MethodPut, "/api/admin/user-ops/customers/7/dnd", strings.NewReader(`{"reason":"local preference"}`))
			request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: "userops-root-session"})
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "userops-root-key-001")
			if testCase.csrf != "" {
				request.Header.Set("X-CSRF-Token", testCase.csrf)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != testCase.wantStatus || auth.csrfCalls != testCase.wantCSRFCAlls || application.setDNDCalls != testCase.wantLeafCalls {
				t.Fatalf("status/csrf/leaf = %d/%d/%d, want %d/%d/%d", response.Code, auth.csrfCalls, application.setDNDCalls, testCase.wantStatus, testCase.wantCSRFCAlls, testCase.wantLeafCalls)
			}
			if testCase.wantStatus != http.StatusOK {
				return
			}
			body := response.Body.String()
			for _, field := range []string{
				`"provider_execution_eligible":false`,
				`"real_external_call_executed":false`,
				`"delivery_proven":false`,
			} {
				if !strings.Contains(body, field) {
					t.Fatalf("response missing top-level local safety %s: %s", field, body)
				}
			}
			if strings.Contains(body, `"safety"`) {
				t.Fatalf("response must not emit nested safety: %s", body)
			}
		})
	}
}

func TestUserOpsRootRejectsSalesAndOwnerScopedContexts(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		principal     authport.Principal
		authorization authport.Authorization
		wantCSRFCAlls int
	}{
		{
			name:          "sales role",
			principal:     authport.Principal{AdminUserID: 42, Role: authport.RoleSales},
			authorization: authport.Authorization{Capability: authport.CapabilityOperationsManage, Scope: authport.ScopeGlobal},
			wantCSRFCAlls: 1,
		},
		{
			name:          "owner scope",
			principal:     authport.Principal{AdminUserID: 43, Role: authport.RoleAdmin},
			authorization: authport.Authorization{Capability: authport.CapabilityOperationsManage, Scope: authport.ScopeOwnerStaff, OwnerStaffID: 73},
			wantCSRFCAlls: 1,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			auth := newUserOpsRootAuth(testCase.principal)
			auth.authorization = testCase.authorization
			application := &userOpsRootApplication{}
			handler := newUserOpsRootHandler(t, auth, application)

			request := httptest.NewRequest(http.MethodPut, "/api/admin/user-ops/customers/7/dnd", strings.NewReader(`{"reason":"local preference"}`))
			request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: "userops-root-session"})
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "userops-root-key-002")
			request.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != http.StatusForbidden || application.setDNDCalls != 0 || auth.csrfCalls != testCase.wantCSRFCAlls {
				t.Fatalf("status/leaf/csrf = %d/%d/%d, want 403/0/%d", response.Code, application.setDNDCalls, auth.csrfCalls, testCase.wantCSRFCAlls)
			}
		})
	}
}

func TestUserOpsPlanIDWireValuesAreLossless(t *testing.T) {
	for _, raw := range []string{"", "0", "01", "+1", "9223372036854775808"} {
		if _, err := parseUserOpsPlanID(raw); !errors.Is(err, useropsport.ErrInvalid) {
			t.Fatalf("parseUserOpsPlanID(%q) error = %v, want invalid", raw, err)
		}
	}
	planID, err := parseUserOpsPlanID("9223372036854775807")
	if err != nil || planID != domain.PlanID(9223372036854775807) {
		t.Fatalf("parse max plan id = %d, %v", planID, err)
	}
	at := time.Date(2026, time.August, 23, 8, 0, 0, 0, time.UTC)
	response, err := userOpsSendRecordPageResponse(useropsport.SendRecordPage{
		Items: []domain.SendRecord{{
			ID:              domain.SendRecordID(9223372036854775807),
			PlanID:          planID,
			CustomerID:      7,
			TechnicalStatus: domain.SendTechnicalStatePendingReview,
			CreatedAt:       at,
			UpdatedAt:       at,
		}},
		Safety: useropsport.LocalSafety(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := response.Items[0].SendRecordId; got != "9223372036854775807" || response.Items[0].PlanId != "9223372036854775807" {
		t.Fatalf("wire ids = %#v, want decimal strings", response.Items[0])
	}
}

func newUserOpsRootHandler(t *testing.T, auth *userOpsRootAuth, application useropsport.Application) http.Handler {
	t.Helper()
	authHandler, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := useropshttp.NewHandler(application, userOpsAuthorizer{}, userOpsCanonicalCSRF{})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := newAPIHandler(
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		authHandler,
		&candidateHandler{Handler: authHandler, userOps: leaf},
	)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

type userOpsRootAuth struct {
	principal     authport.Principal
	authorization authport.Authorization
	csrfErr       error
	csrfCalls     int
}

func newUserOpsRootAuth(principal authport.Principal) *userOpsRootAuth {
	return &userOpsRootAuth{
		principal: principal,
		authorization: authport.Authorization{
			Capability: authport.CapabilityOperationsManage,
			Scope:      authport.ScopeGlobal,
		},
	}
}

func (auth *userOpsRootAuth) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	return auth.principal, nil
}

func (auth *userOpsRootAuth) Authorize(_ context.Context, _ authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	if capability == authport.CapabilityOperationsRead {
		return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
	}
	if capability != authport.CapabilityOperationsManage {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	return auth.authorization, nil
}

func (auth *userOpsRootAuth) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	auth.csrfCalls++
	return auth.csrfErr
}

func (*userOpsRootAuth) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

type userOpsRootApplication struct {
	setDNDCalls int
}

func (*userOpsRootApplication) Overview(context.Context, useropsport.DirectoryQuery) (useropsport.Overview, error) {
	return useropsport.Overview{}, useropsport.ErrUnavailable
}

func (*userOpsRootApplication) ListCustomers(context.Context, useropsport.DirectoryQuery) (useropsport.DirectoryPage, error) {
	return useropsport.DirectoryPage{}, useropsport.ErrUnavailable
}

func (*userOpsRootApplication) GetCustomerDetail(context.Context, domain.CustomerID) (useropsport.CustomerDetailResult, error) {
	return useropsport.CustomerDetailResult{}, useropsport.ErrUnavailable
}

func (*userOpsRootApplication) SafeExport(context.Context, useropsport.SafeExportRequest) (useropsport.SafeExport, error) {
	return useropsport.SafeExport{}, useropsport.ErrUnavailable
}

func (*userOpsRootApplication) PreviewBatch(context.Context, useropsport.BatchPreviewInput) (useropsport.BatchPreview, error) {
	return useropsport.BatchPreview{}, useropsport.ErrUnavailable
}

func (*userOpsRootApplication) CreateLocalPlan(context.Context, useropsport.CreateLocalPlanInput) (useropsport.LocalPlanResult, error) {
	return useropsport.LocalPlanResult{}, useropsport.ErrUnavailable
}

func (application *userOpsRootApplication) SetDND(_ context.Context, input useropsport.UpsertDNDInput) (useropsport.DNDMutationResult, error) {
	application.setDNDCalls++
	at := time.Date(2026, time.August, 23, 8, 0, 0, 0, time.UTC)
	return useropsport.DNDMutationResult{
		DND:    &domain.DoNotDisturb{CustomerID: input.CustomerID, Reason: input.Reason, Version: 1, CreatedAt: at, UpdatedAt: at},
		Safety: useropsport.LocalSafety(),
	}, nil
}

func (*userOpsRootApplication) ClearDND(context.Context, useropsport.ClearDNDInput) (useropsport.DNDMutationResult, error) {
	return useropsport.DNDMutationResult{}, useropsport.ErrUnavailable
}

func (*userOpsRootApplication) ListSendRecords(context.Context, useropsport.SendRecordQuery) (useropsport.SendRecordPage, error) {
	return useropsport.SendRecordPage{}, useropsport.ErrUnavailable
}
