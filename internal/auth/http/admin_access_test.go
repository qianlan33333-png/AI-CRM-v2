package authhttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authapp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/app"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

type adminAccessApplicationStub struct {
	members   []authapp.AdminAccessMember
	listErr   error
	saveErr   error
	listCalls int
	saveCalls int
	saved     []authapp.AdminAccessSaveInput
}

func (stub *adminAccessApplicationStub) List(context.Context) ([]authapp.AdminAccessMember, error) {
	stub.listCalls++
	return stub.members, stub.listErr
}

func (stub *adminAccessApplicationStub) Save(_ context.Context, input []authapp.AdminAccessSaveInput) ([]authapp.AdminAccessMember, error) {
	stub.saveCalls++
	stub.saved = append([]authapp.AdminAccessSaveInput(nil), input...)
	return stub.members, stub.saveErr
}

func TestAdminAccessHandlerReadsAndSavesOnlyGlobalConfigAdmin(t *testing.T) {
	application := &adminAccessApplicationStub{members: []authapp.AdminAccessMember{{AdminUserID: 7, DisplayName: "管理员", LoginEnabled: true}}}
	handler, err := NewAdminAccessHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	get := adminAccessRequest(t, http.MethodGet, nil, authport.CapabilityConfigSettingsManage)
	getResponse := httptest.NewRecorder()
	handler.ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusOK || application.listCalls != 1 || !strings.Contains(getResponse.Body.String(), `"login_enabled":true`) || !strings.Contains(getResponse.Body.String(), `"local_only":true`) {
		t.Fatalf("GET status/calls/body=%d/%d/%s", getResponse.Code, application.listCalls, getResponse.Body.String())
	}
	put := adminAccessRequest(t, http.MethodPut, strings.NewReader(`{"members":[{"admin_user_id":7,"login_enabled":false}]}`), authport.CapabilityConfigSettingsManage)
	put.Header.Set("Content-Type", "application/json")
	put.Header.Set("Idempotency-Key", "admin-access-0001")
	putResponse := httptest.NewRecorder()
	handler.ServeHTTP(putResponse, put)
	if putResponse.Code != http.StatusOK || application.saveCalls != 1 || len(application.saved) != 1 || application.saved[0].AdminUserID != 7 || application.saved[0].LoginEnabled || !strings.Contains(putResponse.Body.String(), `"idempotency_key":"admin-access-0001"`) {
		t.Fatalf("PUT status/saved/body=%d/%#v/%s", putResponse.Code, application.saved, putResponse.Body.String())
	}
	wrong := adminAccessRequest(t, http.MethodGet, nil, authport.CapabilityConfigOverviewRead)
	wrongResponse := httptest.NewRecorder()
	handler.ServeHTTP(wrongResponse, wrong)
	if wrongResponse.Code != http.StatusForbidden || application.listCalls != 1 {
		t.Fatalf("wrong GET status/calls=%d/%d", wrongResponse.Code, application.listCalls)
	}
}

func TestAdminAccessHandlerRejectsBadWriteBeforeApplication(t *testing.T) {
	application := &adminAccessApplicationStub{}
	handler, err := NewAdminAccessHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	validBody := `{"members":[{"admin_user_id":7,"login_enabled":false}]}`
	for _, testCase := range []struct {
		name    string
		body    string
		content string
		key     []string
	}{
		{name: "missing key", body: validBody, content: "application/json"},
		{name: "duplicate key", body: validBody, content: "application/json", key: []string{"one", "two"}},
		{name: "unknown field", body: `{"members":[],"role":"admin"}`, content: "application/json", key: []string{"one"}},
		{name: "duplicate member", body: `{"members":[{"admin_user_id":7,"login_enabled":false},{"admin_user_id":7,"login_enabled":true}]}`, content: "application/json", key: []string{"one"}},
		{name: "form", body: validBody, content: "application/x-www-form-urlencoded", key: []string{"one"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := adminAccessRequest(t, http.MethodPut, strings.NewReader(testCase.body), authport.CapabilityConfigSettingsManage)
			request.Header.Set("Content-Type", testCase.content)
			for _, key := range testCase.key {
				request.Header.Add("Idempotency-Key", key)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
			}
		})
	}
	if application.saveCalls != 0 {
		t.Fatalf("invalid write reached application %d times", application.saveCalls)
	}
	application.saveErr = authapp.ErrAdminAccessMemberMissing
	request := adminAccessRequest(t, http.MethodPut, strings.NewReader(validBody), authport.CapabilityConfigSettingsManage)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "missing-member")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"invalid_member"`) {
		t.Fatalf("missing member status/body=%d/%s", response.Code, response.Body.String())
	}
	application.saveErr = errors.New("db")
	request = adminAccessRequest(t, http.MethodPut, strings.NewReader(validBody), authport.CapabilityConfigSettingsManage)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "unavailable-member")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "db") {
		t.Fatalf("unavailable status/body=%d/%s", response.Code, response.Body.String())
	}
}

func adminAccessRequest(t *testing.T, method string, body io.Reader, capability authport.Capability) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, AdminAccessPath, body)
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 42, Role: authport.RoleAdmin}, "session-ref")
	ctx, err := authport.WithAuthorization(ctx, authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	return request.WithContext(ctx)
}
