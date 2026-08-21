package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

func TestSurveySafeAdminRoutesKeepCentralAuthAndCSRF(t *testing.T) {
	auth := &surveySafeRouteAuth{}
	leaf := &surveySafeRouteLeaf{}
	router := surveySafeAdminRouter(t, auth, leaf)

	analysis := legacyRequest(http.MethodGet, "/api/admin/questionnaires/7/analysis", legacyToken(91))
	analysisResponse := httptest.NewRecorder()
	router.ServeHTTP(analysisResponse, analysis)
	if analysisResponse.Code != http.StatusNoContent || leaf.analysisCalls != 1 || auth.csrfCalls != 0 {
		t.Fatalf("analysis status/calls/csrf=%d/%d/%d", analysisResponse.Code, leaf.analysisCalls, auth.csrfCalls)
	}

	missingCSRF := legacyRequest(http.MethodPost, "/api/admin/questionnaires/7/export/preview", legacyToken(92))
	missingCSRF.Header.Set("Content-Type", "application/json")
	missingCSRFResponse := httptest.NewRecorder()
	router.ServeHTTP(missingCSRFResponse, missingCSRF)
	if missingCSRFResponse.Code != http.StatusForbidden || leaf.previewCalls != 0 || auth.csrfCalls != 0 {
		t.Fatalf("missing csrf status/calls/csrf=%d/%d/%d", missingCSRFResponse.Code, leaf.previewCalls, auth.csrfCalls)
	}

	preview := legacyRequest(http.MethodPost, "/api/admin/questionnaires/7/export/preview", legacyToken(93))
	preview.Header.Set("Content-Type", "application/json")
	preview.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
	previewResponse := httptest.NewRecorder()
	router.ServeHTTP(previewResponse, preview)
	if previewResponse.Code != http.StatusNoContent || leaf.previewCalls != 1 || auth.csrfCalls != 1 {
		t.Fatalf("preview status/calls/csrf=%d/%d/%d", previewResponse.Code, leaf.previewCalls, auth.csrfCalls)
	}
	if len(auth.capabilities) != 2 {
		t.Fatalf("capabilities=%v", auth.capabilities)
	}
	for _, capability := range auth.capabilities {
		if capability != authport.CapabilityQuestionnairesRead {
			t.Fatalf("capability=%s", capability)
		}
	}
}

type surveySafeRouteLeaf struct {
	analysisCalls int
	previewCalls  int
}

func (leaf *surveySafeRouteLeaf) Results(writer http.ResponseWriter, _ *http.Request) {
	leaf.analysisCalls++
	writer.WriteHeader(http.StatusNoContent)
}

func (leaf *surveySafeRouteLeaf) ExportPreview(writer http.ResponseWriter, _ *http.Request) {
	leaf.previewCalls++
	writer.WriteHeader(http.StatusNoContent)
}

type surveySafeRouteAuth struct {
	capabilities []authport.Capability
	csrfCalls    int
}

func (*surveySafeRouteAuth) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	return authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, nil
}

func (auth *surveySafeRouteAuth) Authorize(_ context.Context, _ authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	auth.capabilities = append(auth.capabilities, capability)
	if capability != authport.CapabilityQuestionnairesRead {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
}

func (auth *surveySafeRouteAuth) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	auth.csrfCalls++
	return nil
}

func (*surveySafeRouteAuth) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func surveySafeAdminRouter(t *testing.T, service authport.Service, leaf surveySafeAdminHTTP) http.Handler {
	t.Helper()
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := NewHandler(service, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.surveySafeAdmin = leaf
	router, err := newAPIHandlerWithAll(
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		authHandler,
		authHandler,
		legacy,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return router
}
