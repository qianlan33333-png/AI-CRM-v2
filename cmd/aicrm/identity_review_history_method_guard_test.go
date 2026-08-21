package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

type identityReviewMethodGuardAuth struct {
	mu                sync.Mutex
	authenticateCalls int
	csrfCalls         int
	capabilities      []authport.Capability
}

func (service *identityReviewMethodGuardAuth) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	service.mu.Lock()
	service.authenticateCalls++
	service.mu.Unlock()
	return authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}, nil
}

func (service *identityReviewMethodGuardAuth) Authorize(_ context.Context, _ authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	service.mu.Lock()
	service.capabilities = append(service.capabilities, capability)
	service.mu.Unlock()
	return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
}

func (service *identityReviewMethodGuardAuth) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	service.mu.Lock()
	service.csrfCalls++
	service.mu.Unlock()
	return nil
}

func (*identityReviewMethodGuardAuth) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func (service *identityReviewMethodGuardAuth) calls() (int, int, []authport.Capability) {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.authenticateCalls, service.csrfCalls, append([]authport.Capability(nil), service.capabilities...)
}

func TestIdentityReviewCollectionRejectsUnsupportedMethodsBeforeAuthentication(t *testing.T) {
	service := &identityReviewMethodGuardAuth{}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := newAPIHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), authHandler, authHandler)
	if err != nil {
		t.Fatal(err)
	}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(method, identityMergeReviewCollectionPath, nil))
			authenticateCalls, csrfCalls, capabilities := service.calls()
			if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet ||
				response.Header().Get("Cache-Control") != "no-store" ||
				response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Body.Len() != 0 ||
				authenticateCalls != 0 || csrfCalls != 0 || len(capabilities) != 0 {
				t.Fatalf("status=%d allow=%q cache=%q nosniff=%q auth=%d csrf=%d capabilities=%v body=%q", response.Code, response.Header().Get("Allow"), response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"), authenticateCalls, csrfCalls, capabilities, response.Body.String())
			}
		})
	}
}
