package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
)

type legacyImageDeleteStub struct {
	legacyMediaStub
	result  mediaapp.ImageDeleteResult
	err     error
	command mediaapp.ImageDeleteCommand
	calls   int
}

func (stub *legacyImageDeleteStub) DeleteImage(_ context.Context, command mediaapp.ImageDeleteCommand) (mediaapp.ImageDeleteResult, error) {
	stub.calls++
	stub.command = command
	return stub.result, stub.err
}

func TestLegacyImageDeleteWritesExactSuccessAndReferenceConflict(t *testing.T) {
	stub := &legacyImageDeleteStub{result: mediaapp.ImageDeleteResult{ID: 42, Deleted: true, HardDeleted: true}}
	auth := &legacyMediaAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}}
	response := httptest.NewRecorder()
	legacyMediaRouterWithAuth(t, stub, auth).ServeHTTP(response, legacyImageDeleteRequest("42", "force=false", "delete-image-key-0001", true, true))
	if response.Code != http.StatusOK || stub.calls != 1 || stub.command != (mediaapp.ImageDeleteCommand{ImageID: 42, Actor: 7, IdempotencyKey: "delete-image-key-0001"}) || auth.csrfCalls != 1 || len(auth.seen) != 1 || auth.seen[0] != authport.CapabilityMediaLibraryWrite {
		t.Fatalf("status=%d command=%#v calls=%d auth=%#v body=%s", response.Code, stub.command, stub.calls, auth, response.Body.String())
	}
	body := decodeLegacyImageDetailObject(t, response.Body.Bytes())
	assertExactLegacyImageDetailKeys(t, body, "ok", "deleted", "hard_deleted", "id", "references_cleared", "source_status", "route_owner", "fallback_used", "real_external_call_executed", "storage_adapter_mode", "adapter_mode")
	if string(body["ok"]) != "true" || string(body["deleted"]) != "true" || string(body["hard_deleted"]) != "true" || string(body["id"]) != "42" || string(body["references_cleared"]) != `{"miniprograms_cleared":0,"campaign_steps_cleared":0}` || string(body["source_status"]) != `"local_delete"` || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers=%v body=%s", response.Header(), response.Body.String())
	}

	stub = &legacyImageDeleteStub{result: mediaapp.ImageDeleteResult{ID: 42, References: mediaapp.ImageDeleteReferences{
		Miniprograms: []int64{7}, CampaignSteps: []int64{}, GroupInvites: []int64{9}, AutomationAgents: []int64{11}, Channels: []int64{13}, ImportPreflights: []int64{17},
	}}, err: mediaapp.ErrImageHasReferences}
	response = httptest.NewRecorder()
	legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleOps}}).ServeHTTP(response, legacyImageDeleteRequest("42", "force=true", "delete-image-key-0002", true, true))
	if response.Code != http.StatusConflict || stub.command.Force != true || strings.Contains(response.Body.String(), "title") || strings.Contains(response.Body.String(), "provider") {
		t.Fatalf("status=%d command=%#v body=%s", response.Code, stub.command, response.Body.String())
	}
	body = decodeLegacyImageDetailObject(t, response.Body.Bytes())
	assertExactLegacyImageDetailKeys(t, body, "ok", "error", "references", "source_status", "route_owner", "fallback_used", "real_external_call_executed", "storage_adapter_mode", "adapter_mode")
	var references map[string][]map[string]json.RawMessage
	if err := json.Unmarshal(body["references"], &references); err != nil {
		t.Fatal(err)
	}
	if string(body["error"]) != `"image_has_references"` || len(references) != 6 || string(references["miniprograms"][0]["id"]) != "7" || len(references["campaign_steps"]) != 0 || string(references["group_invites"][0]["id"]) != "9" || string(references["automation_agents"][0]["id"]) != "11" || string(references["channels"][0]["id"]) != "13" || string(references["import_preflights"][0]["id"]) != "17" {
		t.Fatalf("references=%s", body["references"])
	}

	minted := &legacyImageDeleteStub{result: mediaapp.ImageDeleteResult{ID: 43, Deleted: true, HardDeleted: true}}
	response = httptest.NewRecorder()
	legacyMediaRouterWithAuth(t, minted, &legacyMediaAuthStub{}).ServeHTTP(response, legacyImageDeleteRequest("43", "", "", true, true))
	if response.Code != http.StatusOK || len(minted.command.IdempotencyKey) != 32 || minted.command.IdempotencyKey == "" {
		t.Fatalf("minted status=%d command=%#v", response.Code, minted.command)
	}
}

func TestLegacyImageDeleteRejectsBadInputBeforeApplication(t *testing.T) {
	for _, test := range []struct{ name, id, query, key string }{
		{"zero", "0", "", "delete-image-key-0003"}, {"leading zero", "01", "", "delete-image-key-0003"}, {"overflow", "9223372036854775808", "", "delete-image-key-0003"},
		{"unknown query", "1", "force=true&x=1", "delete-image-key-0003"}, {"duplicate force", "1", "force=true&force=true", "delete-image-key-0003"}, {"noncanonical query", "1", "force=TRUE", "delete-image-key-0003"},
		{"short key", "1", "", "short"}, {"spaced key", "1", "", " delete-image-key-0003"},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyImageDeleteStub{result: mediaapp.ImageDeleteResult{ID: 1, Deleted: true, HardDeleted: true}}
			response := httptest.NewRecorder()
			legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{}).ServeHTTP(response, legacyImageDeleteRequest(test.id, test.query, test.key, true, true))
			if response.Code != http.StatusBadRequest || stub.calls != 0 || !strings.Contains(response.Body.String(), `"code":"MALFORMED_REQUEST"`) || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("status=%d calls=%d headers=%v body=%s", response.Code, stub.calls, response.Header(), response.Body.String())
			}
		})
	}

	stub := &legacyImageDeleteStub{result: mediaapp.ImageDeleteResult{ID: 1, Deleted: true, HardDeleted: true}}
	request := legacyImageDeleteRequest("1", "", "delete-image-key-0004", true, true)
	request.Body = io.NopCloser(strings.NewReader("{}"))
	request.ContentLength = 2
	response := httptest.NewRecorder()
	legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{}).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || stub.calls != 0 {
		t.Fatalf("body status=%d calls=%d body=%s", response.Code, stub.calls, response.Body.String())
	}
}

func TestLegacyImageDeleteSecurityAndApplicationErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		auth *legacyMediaAuthStub
		csrf bool
		want int
	}{
		{"anonymous", &legacyMediaAuthStub{}, false, http.StatusUnauthorized},
		{"csrf", &legacyMediaAuthStub{}, false, http.StatusForbidden},
		{"sales", &legacyMediaAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleSales}}, true, http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyImageDeleteStub{result: mediaapp.ImageDeleteResult{ID: 1, Deleted: true, HardDeleted: true}}
			response := httptest.NewRecorder()
			withSession := test.name != "anonymous"
			legacyMediaRouterWithAuth(t, stub, test.auth).ServeHTTP(response, legacyImageDeleteRequest("1", "", "delete-image-key-0005", withSession, test.csrf))
			if response.Code != test.want || stub.calls != 0 || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("status=%d calls=%d headers=%v body=%s", response.Code, stub.calls, response.Header(), response.Body.String())
			}
		})
	}
	for _, test := range []struct {
		err  error
		want int
		code string
	}{
		{mediaapp.ErrImageDeleteNotFound, http.StatusNotFound, "NOT_FOUND"},
		{mediaapp.ErrImageDeleteConflict, http.StatusConflict, "CONFLICT"},
		{errors.New("postgres secret checksum"), http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE"},
	} {
		stub := &legacyImageDeleteStub{err: test.err}
		response := httptest.NewRecorder()
		legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{}).ServeHTTP(response, legacyImageDeleteRequest("1", "", "delete-image-key-0006", true, true))
		if response.Code != test.want || stub.calls != 1 || !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`) || strings.Contains(response.Body.String(), "secret") {
			t.Fatalf("status=%d calls=%d body=%s", response.Code, stub.calls, response.Body.String())
		}
	}

	stub := &legacyImageDeleteStub{}
	response := httptest.NewRecorder()
	legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{}).ServeHTTP(response, legacyImageDeleteRequest("1", "", "", false, false).WithContext(context.Background()))
	if response.Code != http.StatusUnauthorized || stub.calls != 0 {
		t.Fatalf("dependency status=%d calls=%d body=%s", response.Code, stub.calls, response.Body.String())
	}
}

func legacyImageDeleteRequest(id, rawQuery, key string, withSession, withCSRF bool) *http.Request {
	request := httptest.NewRequest(http.MethodDelete, "/api/admin/image-library/"+id, nil)
	request.URL.RawQuery = rawQuery
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	if withSession {
		request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(91)})
	}
	if withCSRF {
		request.Header.Set("X-CSRF-Token", legacyToken(92))
	}
	return request
}
