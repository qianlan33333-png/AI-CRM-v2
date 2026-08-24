package authhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

type optionalAuthService struct {
	principal authport.Principal
	err       error
	calls     int
}

func (service *optionalAuthService) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	service.calls++
	return service.principal, service.err
}

func (*optionalAuthService) Authorize(context.Context, authport.Principal, authport.Capability) (authport.Authorization, error) {
	return authport.Authorization{}, nil
}

func (*optionalAuthService) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func (*optionalAuthService) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func TestAuthenticateOptionalPassesMissingSessionAndAttachesValidSession(t *testing.T) {
	staffID := int64(7)
	service := &optionalAuthService{principal: authport.Principal{AdminUserID: 9, Role: authport.RoleSales, StaffID: &staffID}}
	handler, err := NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	seenAuthenticated := false
	next := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		principal, authenticated := authport.PrincipalFromContext(request.Context())
		seenAuthenticated = authenticated
		if authenticated && principal != service.principal {
			t.Fatalf("principal=%+v", principal)
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	middleware := handler.AuthenticateOptional(next)

	missingResponse := httptest.NewRecorder()
	middleware.ServeHTTP(missingResponse, httptest.NewRequest(http.MethodPost, "/api/sidebar/context-token", nil))
	if missingResponse.Code != http.StatusNoContent || seenAuthenticated || service.calls != 0 {
		t.Fatalf("missing session status/authenticated/calls=%d/%t/%d", missingResponse.Code, seenAuthenticated, service.calls)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/sidebar/context-token", nil)
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "sidebar-session"})
	response := httptest.NewRecorder()
	middleware.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || !seenAuthenticated || service.calls != 1 {
		t.Fatalf("valid session status/authenticated/calls=%d/%t/%d", response.Code, seenAuthenticated, service.calls)
	}
}

func TestAuthenticateOptionalFailsClosedForInvalidSession(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "invalid", err: authport.ErrUnauthenticated, wantStatus: http.StatusUnauthorized},
		{name: "unavailable", err: authport.ErrAuthenticationUnavailable, wantStatus: http.StatusServiceUnavailable},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service := &optionalAuthService{err: testCase.err}
			handler, err := NewHandler(service)
			if err != nil {
				t.Fatal(err)
			}
			called := false
			request := httptest.NewRequest(http.MethodPost, "/api/sidebar/context-token", nil)
			request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "sidebar-session"})
			response := httptest.NewRecorder()
			handler.AuthenticateOptional(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })).ServeHTTP(response, request)
			if response.Code != testCase.wantStatus || called || service.calls != 1 {
				t.Fatalf("status/called/calls=%d/%t/%d", response.Code, called, service.calls)
			}
		})
	}
}
