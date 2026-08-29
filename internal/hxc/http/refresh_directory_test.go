package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
)

func TestDirectoryRefreshHandlerRequiresGlobalOperationsManageAndReturnsReceipt(t *testing.T) {
	application := &directoryRefreshStub{result: hxcapp.RefreshDirectoryResult{SyncedCount: 2, ProviderReadExecuted: true}}
	request := httptest.NewRequest(http.MethodPost, RefreshDirectoryPath, strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "hxc-directory-http-0001")
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 7, Role: authport.RoleOps}, "session")
	ctx, _ = authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityOperationsManage, Scope: authport.ScopeGlobal})
	response := httptest.NewRecorder()
	DirectoryRefreshHandler{Application: application}.ServeHTTP(response, request.WithContext(ctx))
	if response.Code != http.StatusOK || application.calls != 1 || !strings.Contains(response.Body.String(), `"provider_read_executed":true`) || !strings.Contains(response.Body.String(), `"synced_count":2`) {
		t.Fatalf("status/calls/body=%d/%d/%s", response.Code, application.calls, response.Body.String())
	}
}

func TestDirectoryRefreshHandlerFailsClosed(t *testing.T) {
	application := &directoryRefreshStub{err: errors.New("provider")}
	request := httptest.NewRequest(http.MethodPost, RefreshDirectoryPath, strings.NewReader(`{}`))
	request.Header.Set("Idempotency-Key", "hxc-directory-http-0002")
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, "session")
	ctx, _ = authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityOperationsManage, Scope: authport.ScopeGlobal})
	response := httptest.NewRecorder()
	DirectoryRefreshHandler{Application: application}.ServeHTTP(response, request.WithContext(ctx))
	if response.Code != http.StatusServiceUnavailable || application.calls != 1 || !strings.Contains(response.Body.String(), `"provider_read_executed":false`) {
		t.Fatalf("status/calls/body=%d/%d/%s", response.Code, application.calls, response.Body.String())
	}

	unauthorized := httptest.NewRequest(http.MethodPost, RefreshDirectoryPath, strings.NewReader(`{}`))
	response = httptest.NewRecorder()
	DirectoryRefreshHandler{Application: application}.ServeHTTP(response, unauthorized)
	if response.Code != http.StatusForbidden || application.calls != 1 {
		t.Fatalf("unauthorized status/calls=%d/%d", response.Code, application.calls)
	}
}

type directoryRefreshStub struct {
	result hxcapp.RefreshDirectoryResult
	err    error
	calls  int
}

func (stub *directoryRefreshStub) Refresh(context.Context, hxcapp.RefreshDirectoryCommand) (hxcapp.RefreshDirectoryResult, error) {
	stub.calls++
	return stub.result, stub.err
}
