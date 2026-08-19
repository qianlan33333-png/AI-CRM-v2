package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
)

type legacyAutomationAgentStub struct {
	item          automationport.Agent
	page          automationport.Page
	listKind      automationport.AutomationType
	create        automationport.CreateCommand
	update        automationport.UpdateCommand
	copy          automationport.MutationCommand
	publish       automationport.MutationCommand
	status        automationport.MutationCommand
	statusValue   automationport.AgentStatus
	fixed         automationport.FixedContentCommand
	mutationCalls int
}

func (s *legacyAutomationAgentStub) List(_ context.Context, kind automationport.AutomationType) (automationport.Page, error) {
	s.listKind = kind
	return s.page, nil
}
func (s *legacyAutomationAgentStub) Get(context.Context, automationport.AgentID) (automationport.Agent, error) {
	return s.item, nil
}
func (s *legacyAutomationAgentStub) Create(_ context.Context, command automationport.CreateCommand) (automationport.Agent, error) {
	s.create = command
	s.mutationCalls++
	return s.item, nil
}
func (s *legacyAutomationAgentStub) Update(_ context.Context, command automationport.UpdateCommand) (automationport.Agent, error) {
	s.update = command
	s.mutationCalls++
	return s.item, nil
}
func (s *legacyAutomationAgentStub) Copy(_ context.Context, command automationport.MutationCommand) (automationport.Agent, error) {
	s.copy = command
	s.mutationCalls++
	return s.item, nil
}
func (s *legacyAutomationAgentStub) Publish(_ context.Context, command automationport.MutationCommand) (automationport.Agent, error) {
	s.publish = command
	s.mutationCalls++
	return s.item, nil
}
func (s *legacyAutomationAgentStub) SetStatus(_ context.Context, command automationport.MutationCommand, status automationport.AgentStatus) (automationport.Agent, error) {
	s.status, s.statusValue = command, status
	s.mutationCalls++
	item := s.item
	item.Status = status
	return item, nil
}
func (s *legacyAutomationAgentStub) SaveFixedContent(_ context.Context, command automationport.FixedContentCommand) (automationport.Agent, error) {
	s.fixed = command
	s.mutationCalls++
	item := s.item
	item.FixedContentPackage = command.ContentPackage
	return item, nil
}

func TestP4AutomationAgentsABTwelveRoutesSessionCSRFRBACAndNoExternalEffect(t *testing.T) {
	item := legacyAutomationAgentItem()
	stub := &legacyAutomationAgentStub{item: item, page: automationport.Page{Items: []automationport.Agent{item}, Total: 1}}
	router, auth := legacyAutomationAgentRouter(t, stub)
	carrier := httptest.NewRecorder()
	router.ServeHTTP(carrier, legacyRequest(http.MethodGet, legacyAutomationAgentListPagePath, legacyToken(131)))
	if carrier.Code != http.StatusFound || carrier.Header().Get("Location") != "/?legacy_admin_path=%2Fadmin%2Fautomation-agents" || carrier.Header().Get("Cache-Control") != "private, no-store" || carrier.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("carrier=%d headers=%v body=%s", carrier.Code, carrier.Header(), carrier.Body.String())
	}
	edit := httptest.NewRecorder()
	router.ServeHTTP(edit, legacyRequest(http.MethodGet, "/admin/automation-agents/7/edit", legacyToken(131)))
	if edit.Code != http.StatusOK || !strings.Contains(edit.Body.String(), "编辑自动化话术") {
		t.Fatalf("edit=%d body=%s", edit.Code, edit.Body.String())
	}
	list := httptest.NewRecorder()
	router.ServeHTTP(list, legacyRequest(http.MethodGet, "/api/admin/automation-agents", legacyToken(132)))
	if list.Code != http.StatusOK || stub.listKind != "" || !strings.Contains(list.Body.String(), "\"total\":1") {
		t.Fatalf("list=%d kind=%q body=%s", list.Code, stub.listKind, list.Body.String())
	}
	var listBody struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listBody); err != nil || len(listBody.Items) != 1 {
		t.Fatalf("decode list err=%v body=%s", err, list.Body.String())
	}
	if got := listBody.Items[0]; len(got) != 10 || got["bound_package_key"] != "" || got["bound_package_id"] != nil || got["bound_package_name"] != "" || got["automation_type_label"] != nil || got["draft_role_prompt"] != nil || got["fixed_content_package"] != nil {
		t.Fatalf("unsafe or incomplete list item=%#v", got)
	}
	detail := httptest.NewRecorder()
	router.ServeHTTP(detail, legacyRequest(http.MethodGet, "/api/admin/automation-agents/7", legacyToken(133)))
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "\"has_unpublished_changes\":false") {
		t.Fatalf("detail=%d body=%s", detail.Code, detail.Body.String())
	}
	create := httptest.NewRecorder()
	router.ServeHTTP(create, legacyAutomationAgentWriteRequest(http.MethodPost, "/api/admin/automation-agents", "{\"agent_name\":\"话术\",\"agent_code\":\"agent_1\"}"))
	if create.Code != http.StatusOK || stub.create.Actor != 1 || stub.create.IdempotencyKey == "" || stub.create.Agent.DraftRolePrompt != "" || stub.create.Agent.DraftTaskPrompt != "" || create.Header().Get("X-AICRM-Real-External-Call-Executed") != "false" {
		t.Fatalf("create=%d command=%#v headers=%v", create.Code, stub.create, create.Header())
	}
	for _, route := range []struct{ method, path, body string }{{http.MethodPatch, "/api/admin/automation-agents/7", "{\"agent_name\":\"更新后的话术\"}"}, {http.MethodPost, "/api/admin/automation-agents/7/activate", ""}, {http.MethodPost, "/api/admin/automation-agents/7/copy", ""}, {http.MethodPut, "/api/admin/automation-agents/7/fixed-content", "{}"}, {http.MethodPost, "/api/admin/automation-agents/7/pause", ""}, {http.MethodPost, "/api/admin/automation-agents/7/publish", ""}, {http.MethodDelete, "/api/admin/automation-agents/7", ""}} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyAutomationAgentWriteRequest(route.method, route.path, route.body))
		if response.Code != http.StatusOK {
			t.Fatalf("%s %s=%d body=%s", route.method, route.path, response.Code, response.Body.String())
		}
	}
	if stub.update.ID != 7 || stub.update.AgentName == nil || *stub.update.AgentName != "更新后的话术" || stub.copy.ID != 7 || stub.fixed.ContentPackage.ContentText != "" || len(stub.fixed.ContentPackage.ImageLibraryIDs) != 0 || stub.status.ID != 7 || stub.statusValue != automationport.AgentStatusArchived || stub.mutationCalls != 8 {
		t.Fatalf("mutations=%d update=%#v copy=%#v fixed=%#v status=%#v/%q", stub.mutationCalls, stub.update, stub.copy, stub.fixed, stub.status, stub.statusValue)
	}
	if got := auth.capabilities(); len(got) != 12 {
		t.Fatalf("capabilities=%v", got)
	} else {
		for index, capability := range got {
			want := authport.CapabilityConfigSettingsManage
			if index < 4 {
				want = authport.CapabilityConfigOverviewRead
			}
			if capability != want {
				t.Fatalf("capability[%d]=%q want %q", index, capability, want)
			}
		}
	}
}

func TestP4AutomationAgentListCarrierIsAdminOnlyAndStrictBeforeAuth(t *testing.T) {
	for _, test := range []struct {
		name      string
		principal authport.Principal
		want      int
	}{
		{name: "admin", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, want: http.StatusFound},
		{name: "ops", principal: authport.Principal{AdminUserID: 8, Role: authport.RoleOps}, want: http.StatusForbidden},
		{name: "sales", principal: authport.Principal{AdminUserID: 9, Role: authport.RoleSales}, want: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := legacyAutomationAgentRouterWithAuth(t, &legacyAuthStub{principal: test.principal}, &legacyAutomationAgentStub{item: legacyAutomationAgentItem()})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodGet, legacyAutomationAgentListPagePath, legacyToken(140)))
			if response.Code != test.want || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("status/headers=%d/%q body=%s", response.Code, response.Header().Get("X-Content-Type-Options"), response.Body.String())
			}
		})
	}

	router := legacyAutomationAgentRouterWithAuth(t, &legacyAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}}, &legacyAutomationAgentStub{item: legacyAutomationAgentItem()})
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, legacyAutomationAgentListPagePath, nil))
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("method/status/headers=%s/%d/%q/%q/%q", method, response.Code, response.Header().Get("Allow"), response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"))
		}
	}
}

func TestP4AutomationAgentsABBoundaryAndErrorFailClosed(t *testing.T) {
	stub := &legacyAutomationAgentStub{item: legacyAutomationAgentItem()}
	router, _ := legacyAutomationAgentRouter(t, stub)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyAutomationAgentWriteRequest(http.MethodPost, "/api/admin/automation-agents", "{\"bound_package_key\":\"retired\"}"))
	if response.Code != http.StatusGone || stub.mutationCalls != 0 {
		t.Fatalf("retired=%d writes=%d body=%s", response.Code, stub.mutationCalls, response.Body.String())
	}
	crossOrigin := legacyAutomationAgentWriteRequest(http.MethodPost, "/api/admin/automation-agents", "{}")
	crossOrigin.Header.Set("Origin", "https://cross.invalid")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, crossOrigin)
	if response.Code != http.StatusForbidden || stub.mutationCalls != 0 {
		t.Fatalf("cross-origin=%d writes=%d body=%s", response.Code, stub.mutationCalls, response.Body.String())
	}
	invalidType := httptest.NewRecorder()
	router.ServeHTTP(invalidType, legacyRequest(http.MethodGet, "/api/admin/automation-agents?automation_type=tenant", legacyToken(134)))
	if invalidType.Code != http.StatusBadRequest || !strings.Contains(invalidType.Body.String(), "invalid_agent_payload") {
		t.Fatalf("invalid type=%d body=%s", invalidType.Code, invalidType.Body.String())
	}
	for _, test := range []struct {
		name    string
		request func() *http.Request
	}{
		{name: "missing idempotency key", request: func() *http.Request {
			request := legacyAutomationAgentWriteRequest(http.MethodPost, "/api/admin/automation-agents", `{"agent_name":"话术","agent_code":"agent_1"}`)
			request.Header.Del("Idempotency-Key")
			return request
		}},
		{name: "repeated idempotency key", request: func() *http.Request {
			request := legacyAutomationAgentWriteRequest(http.MethodPost, "/api/admin/automation-agents", `{"agent_name":"话术","agent_code":"agent_1"}`)
			request.Header.Add("Idempotency-Key", "automation-agent-idempotency-key-0002")
			return request
		}},
		{name: "unknown create field", request: func() *http.Request {
			return legacyAutomationAgentWriteRequest(http.MethodPost, "/api/admin/automation-agents", `{"agent_name":"话术","agent_code":"agent_1","unexpected":true}`)
		}},
		{name: "empty update", request: func() *http.Request {
			return legacyAutomationAgentWriteRequest(http.MethodPatch, "/api/admin/automation-agents/7", `{}`)
		}},
		{name: "unknown fixed content field", request: func() *http.Request {
			return legacyAutomationAgentWriteRequest(http.MethodPut, "/api/admin/automation-agents/7/fixed-content", `{"unexpected":true}`)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := stub.mutationCalls
			response := httptest.NewRecorder()
			router.ServeHTTP(response, test.request())
			if response.Code != http.StatusBadRequest || stub.mutationCalls != before || !strings.Contains(response.Body.String(), "invalid_agent_payload") {
				t.Fatalf("status=%d writes=%d/%d body=%s", response.Code, stub.mutationCalls, before, response.Body.String())
			}
		})
	}
}

func legacyAutomationAgentRouter(t *testing.T, agents automationport.AgentService) (http.Handler, *recordingAuth) {
	t.Helper()
	service := &recordingAuth{}
	return legacyAutomationAgentRouterWithAuth(t, service, agents), service
}

func legacyAutomationAgentRouterWithAuth(t *testing.T, service authport.Service, agents automationport.AgentService) http.Handler {
	t.Helper()
	legacy, err := NewHandlerWithOutboundProductsMediaAndSurvey(service, &legacyCustomerStub{result: legacyCustomerResult()}, &legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, &legacyProductStub{}, &legacyMediaStub{}, &legacySurveyStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.automationAgents = agents
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}
func legacyAutomationAgentWriteRequest(method, path, body string) *http.Request {
	request := legacyChannelWriteRequest(method, path, body)
	request.Header.Set("Idempotency-Key", "automation-agent-idempotency-key-0001")
	return request
}
func legacyAutomationAgentItem() automationport.Agent {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	return automationport.Agent{ID: 7, AgentName: "自动化话术", AgentCode: "agent_1", AutomationType: automationport.AutomationTypeAgent, Status: automationport.AgentStatusActive, DraftRolePrompt: "角色", DraftTaskPrompt: "任务", PublishedRolePrompt: "角色", PublishedTaskPrompt: "任务", DraftVersion: 1, PublishedVersion: 1, CreatedBy: 1, UpdatedBy: 1, CreatedAt: now, UpdatedAt: now, FixedContentPackage: automationport.FixedContentPackage{ImageLibraryIDs: []int64{}, MiniprogramLibraryIDs: []int64{}, AttachmentLibraryIDs: []int64{}, GroupInviteLibraryIDs: []int64{}}}
}
