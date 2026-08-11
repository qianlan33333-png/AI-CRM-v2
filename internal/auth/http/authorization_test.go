package authhttp

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

type csrfAuthService struct {
	validateErr error
	calls       int
	session     authport.SessionRef
	csrf        authport.CSRFToken
}

func (*csrfAuthService) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	return authport.Principal{}, nil
}

func (*csrfAuthService) Authorize(context.Context, authport.Principal, authport.Capability) (authport.Authorization, error) {
	return authport.Authorization{}, nil
}

func (service *csrfAuthService) ValidateCSRF(_ context.Context, session authport.SessionRef, csrf authport.CSRFToken) error {
	service.calls++
	service.session, service.csrf = session, csrf
	return service.validateErr
}

func (*csrfAuthService) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func TestRequireCSRFPassesOnlyServerBoundToken(t *testing.T) {
	service := &csrfAuthService{}
	handler, err := NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	next := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		called = true
		writer.WriteHeader(http.StatusNoContent)
	})
	middleware, err := handler.RequireCSRF(next)
	if err != nil {
		t.Fatal(err)
	}
	csrf := strings.Repeat("A", 43)
	ctx := authport.WithAuthenticatedSession(context.Background(), authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}, "session-ref")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/stages", nil).WithContext(ctx)
	request.Header.Set("X-CSRF-Token", csrf)
	response := httptest.NewRecorder()

	middleware.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || !called || service.calls != 1 ||
		service.session != "session-ref" || service.csrf != authport.CSRFToken(csrf) {
		t.Fatalf("response/calls/session/csrf = %d/%t/%d/%q/%q", response.Code, called, service.calls, service.session, service.csrf)
	}
}

func TestRequireCSRFFailsClosed(t *testing.T) {
	secret := strings.Repeat("S", 43)
	tests := []struct {
		name        string
		withSession bool
		headers     []string
		validateErr error
		wantStatus  int
		wantCalls   int
	}{
		{name: "missing session", headers: []string{secret}, wantStatus: http.StatusUnauthorized},
		{name: "missing header", withSession: true, wantStatus: http.StatusForbidden},
		{name: "duplicate header", withSession: true, headers: []string{secret, secret}, wantStatus: http.StatusForbidden},
		{name: "token mismatch", withSession: true, headers: []string{secret}, validateErr: authport.ErrCSRFInvalid, wantStatus: http.StatusForbidden, wantCalls: 1},
		{name: "session invalid", withSession: true, headers: []string{secret}, validateErr: authport.ErrUnauthenticated, wantStatus: http.StatusUnauthorized, wantCalls: 1},
		{name: "store unavailable", withSession: true, headers: []string{secret}, validateErr: authport.ErrAuthenticationUnavailable, wantStatus: http.StatusServiceUnavailable, wantCalls: 1},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			service := &csrfAuthService{validateErr: testCase.validateErr}
			handler, err := NewHandler(service)
			if err != nil {
				t.Fatal(err)
			}
			called := false
			middleware, err := handler.RequireCSRF(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if testCase.withSession {
				ctx = authport.WithAuthenticatedSession(ctx, authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}, "session-ref")
			}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/stages", nil).WithContext(ctx)
			for _, value := range testCase.headers {
				request.Header.Add("X-CSRF-Token", value)
			}
			response := httptest.NewRecorder()

			middleware.ServeHTTP(response, request)

			if response.Code != testCase.wantStatus || called || service.calls != testCase.wantCalls {
				t.Fatalf("status/called/calls = %d/%t/%d, want %d/false/%d", response.Code, called, service.calls, testCase.wantStatus, testCase.wantCalls)
			}
			if bytes.Contains(response.Body.Bytes(), []byte(secret)) || bytes.Contains(response.Body.Bytes(), []byte("session-ref")) {
				t.Fatalf("response leaked CSRF or session: %s", response.Body.String())
			}
		})
	}
}

func TestRequireCSRFRejectsInvalidComponents(t *testing.T) {
	service := &csrfAuthService{}
	handler, err := NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	var nilHandler *Handler
	for _, testCase := range []struct {
		name    string
		handler *Handler
		next    http.Handler
	}{
		{name: "nil handler", handler: nilHandler, next: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})},
		{name: "nil service", handler: &Handler{}},
		{name: "nil next", handler: handler},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			middleware, requireErr := testCase.handler.RequireCSRF(testCase.next)
			if middleware != nil || !errors.Is(requireErr, authport.ErrUnauthorized) {
				t.Fatalf("RequireCSRF() = %v, %v; want nil, ErrUnauthorized", middleware, requireErr)
			}
		})
	}
}
