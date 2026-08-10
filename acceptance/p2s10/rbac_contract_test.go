package p2s10

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	authapp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/app"
	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

func TestOperationRBACAndDataScope(t *testing.T) {
	staffID := int64(42)
	service := new(authapp.Service)
	handler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		principal  authport.Principal
		capability authport.Capability
		wantStatus int
		wantScope  authport.ScopeKind
		wantOwner  int64
	}{
		{"admin config global", authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}, authport.CapabilityConfigOverviewRead, http.StatusNoContent, authport.ScopeGlobal, 0},
		{"ops identity global", authport.Principal{AdminUserID: 2, Role: authport.RoleOps}, authport.CapabilityIdentityResolve, http.StatusNoContent, authport.ScopeGlobal, 0},
		{"ops config denied", authport.Principal{AdminUserID: 2, Role: authport.RoleOps}, authport.CapabilityConfigOverviewRead, http.StatusForbidden, "", 0},
		{"sales customer owner", authport.Principal{AdminUserID: 3, Role: authport.RoleSales, StaffID: &staffID}, authport.CapabilityCustomersRead, http.StatusNoContent, authport.ScopeOwnerStaff, 42},
		{"sales identity denied", authport.Principal{AdminUserID: 3, Role: authport.RoleSales, StaffID: &staffID}, authport.CapabilityIdentityBind, http.StatusForbidden, "", 0},
		{"sales without staff denied", authport.Principal{AdminUserID: 3, Role: authport.RoleSales}, authport.CapabilityCustomersRead, http.StatusForbidden, "", 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				called = true
				authorization, ok := authport.AuthorizationFromContext(request.Context())
				if !ok || authorization.Capability != test.capability || authorization.Scope != test.wantScope || authorization.OwnerStaffID != test.wantOwner {
					t.Fatalf("authorization = %#v, present=%t", authorization, ok)
				}
				writer.WriteHeader(http.StatusNoContent)
			})
			protected, wrapErr := handler.Authorize(test.capability, next)
			if wrapErr != nil {
				t.Fatal(wrapErr)
			}
			ctx := authport.WithAuthenticatedSession(context.Background(), test.principal, "fixture-session")
			request := httptest.NewRequest(http.MethodGet, "/fixture", nil).WithContext(ctx)
			recorder := httptest.NewRecorder()
			protected.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			if called != (test.wantStatus == http.StatusNoContent) {
				t.Fatalf("next called = %t", called)
			}
			if test.wantStatus == http.StatusForbidden {
				var body struct {
					Code string `json:"code"`
				}
				if decodeErr := json.NewDecoder(recorder.Body).Decode(&body); decodeErr != nil || body.Code != "UNAUTHORIZED" {
					t.Fatalf("forbidden body = %q, decode=%v", recorder.Body.String(), decodeErr)
				}
			}
		})
	}
}

func TestAuthorizationMiddlewareFailsClosed(t *testing.T) {
	service := new(authapp.Service)
	handler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	if wrapped, wrapErr := handler.Authorize(authport.Capability("unknown"), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})); wrapped != nil || wrapErr == nil {
		t.Fatalf("unknown capability constructor = %v, %v", wrapped, wrapErr)
	}
	protected, err := handler.Authorize(authport.CapabilityCustomersRead, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("unauthenticated request reached handler")
	}))
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	protected.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/fixture", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing principal status = %d, want 401", recorder.Code)
	}
}

func TestSalesOwnerScopeCannotEnumerateOtherCustomers(t *testing.T) {
	staffID := int64(42)
	authorization, err := new(authapp.Service).Authorize(
		context.Background(),
		authport.Principal{AdminUserID: 3, Role: authport.RoleSales, StaffID: &staffID},
		authport.CapabilityCustomersRead,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !authorization.AllowsOwner(42) || authorization.AllowsOwner(7) || authorization.AllowsOwner(0) {
		t.Fatalf("owner scope allowed an invalid owner: %#v", authorization)
	}
}
