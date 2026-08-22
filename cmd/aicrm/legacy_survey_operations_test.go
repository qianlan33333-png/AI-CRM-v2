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

func TestSurveyOperationsRoutesKeepCentralCapabilityAndCSRF(t *testing.T) {
	auth := &surveyOperationsRouteAuth{}
	leaf := &surveyOperationsRouteLeaf{}
	router := surveyOperationsRouter(t, auth, leaf)

	read := legacyRequest(http.MethodGet, "/api/admin/questionnaires/7/operations", legacyToken(111))
	readResponse := httptest.NewRecorder()
	router.ServeHTTP(readResponse, read)
	if readResponse.Code != http.StatusNoContent || leaf.getCalls != 1 || auth.csrfCalls != 0 {
		t.Fatalf("read status/get/csrf=%d/%d/%d", readResponse.Code, leaf.getCalls, auth.csrfCalls)
	}

	missingCSRF := legacyRequest(http.MethodPut, "/api/admin/questionnaires/7/operations/completion", legacyToken(112))
	missingCSRF.Header.Set("Content-Type", "application/json")
	missingCSRFResponse := httptest.NewRecorder()
	router.ServeHTTP(missingCSRFResponse, missingCSRF)
	if missingCSRFResponse.Code != http.StatusForbidden || leaf.completionCalls != 0 || auth.csrfCalls != 0 {
		t.Fatalf("missing csrf status/completion/csrf=%d/%d/%d", missingCSRFResponse.Code, leaf.completionCalls, auth.csrfCalls)
	}

	completion := legacyRequest(http.MethodPut, "/api/admin/questionnaires/7/operations/completion", legacyToken(113))
	completion.Header.Set("Content-Type", "application/json")
	completion.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
	completionResponse := httptest.NewRecorder()
	router.ServeHTTP(completionResponse, completion)
	if completionResponse.Code != http.StatusNoContent || leaf.completionCalls != 1 || auth.csrfCalls != 1 {
		t.Fatalf("completion status/calls/csrf=%d/%d/%d", completionResponse.Code, leaf.completionCalls, auth.csrfCalls)
	}

	test := legacyRequest(http.MethodPost, "/api/admin/questionnaires/7/operations/external-push/test", legacyToken(114))
	test.Header.Set("X-CSRF-Token", strings.Repeat("B", 43))
	testResponse := httptest.NewRecorder()
	router.ServeHTTP(testResponse, test)
	if testResponse.Code != http.StatusNoContent || leaf.queueCalls != 1 || auth.csrfCalls != 2 {
		t.Fatalf("queue status/calls/csrf=%d/%d/%d", testResponse.Code, leaf.queueCalls, auth.csrfCalls)
	}
	if len(auth.capabilities) != 3 || auth.capabilities[0] != authport.CapabilityQuestionnairesRead || auth.capabilities[1] != authport.CapabilityQuestionnairesWrite || auth.capabilities[2] != authport.CapabilityQuestionnairesWrite {
		t.Fatalf("capabilities=%v", auth.capabilities)
	}
}

type surveyOperationsRouteLeaf struct {
	getCalls        int
	completionCalls int
	queueCalls      int
}

func (leaf *surveyOperationsRouteLeaf) GetOperations(writer http.ResponseWriter, _ *http.Request) {
	leaf.getCalls++
	writer.WriteHeader(http.StatusNoContent)
}
func (*surveyOperationsRouteLeaf) GetOperationsPage(http.ResponseWriter, *http.Request) {}
func (leaf *surveyOperationsRouteLeaf) SaveCompletion(writer http.ResponseWriter, _ *http.Request) {
	leaf.completionCalls++
	writer.WriteHeader(http.StatusNoContent)
}
func (*surveyOperationsRouteLeaf) SaveExternalPush(http.ResponseWriter, *http.Request) {}
func (leaf *surveyOperationsRouteLeaf) QueueExternalPushTest(writer http.ResponseWriter, _ *http.Request) {
	leaf.queueCalls++
	writer.WriteHeader(http.StatusNoContent)
}
func (*surveyOperationsRouteLeaf) ListGlobalExternalPushLogs(http.ResponseWriter, *http.Request) {}
func (*surveyOperationsRouteLeaf) ListQuestionnaireExternalPushLogs(http.ResponseWriter, *http.Request) {
}

type surveyOperationsRouteAuth struct {
	capabilities []authport.Capability
	csrfCalls    int
}

func (*surveyOperationsRouteAuth) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	return authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, nil
}

func (auth *surveyOperationsRouteAuth) Authorize(_ context.Context, _ authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	auth.capabilities = append(auth.capabilities, capability)
	if capability != authport.CapabilityQuestionnairesRead && capability != authport.CapabilityQuestionnairesWrite {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
}

func (auth *surveyOperationsRouteAuth) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	auth.csrfCalls++
	return nil
}

func (*surveyOperationsRouteAuth) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func surveyOperationsRouter(t *testing.T, service authport.Service, leaf surveyOperationsHTTP) http.Handler {
	t.Helper()
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := NewHandler(service, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.surveyOperations = leaf
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
