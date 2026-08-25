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
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	wecomtag "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/tag"
)

type legacyTagStub struct {
	catalog   contactapp.LegacyTagCatalog
	err       error
	command   contactapp.LegacyTagCommand
	listCalls int
	writes    int
}

type legacyTagReadAuthStub struct {
	principal     authport.Principal
	authorization authport.Authorization
}

func (stub *legacyTagReadAuthStub) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	return stub.principal, nil
}

func (stub *legacyTagReadAuthStub) Authorize(context.Context, authport.Principal, authport.Capability) (authport.Authorization, error) {
	return stub.authorization, nil
}

func (*legacyTagReadAuthStub) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func (*legacyTagReadAuthStub) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

type legacyTagSyncStub struct {
	command       contactapp.LegacyTagSyncCommand
	effectCommand wecomtag.QueueCommand
	err           error
}

func (stub *legacyTagSyncStub) Request(_ context.Context, command contactapp.LegacyTagSyncCommand) (contactapp.LegacyTagSyncAcceptance, wecomtag.Acceptance, error) {
	stub.command = command
	stub.effectCommand = wecomtag.QueueCommand{LegacyReceiptID: 41, Actor: command.Actor, IdempotencyKey: command.IdempotencyKey, Operation: wecomtag.OperationCatalogSync}
	return contactapp.LegacyTagSyncAcceptance{ReceiptID: 41, EventID: 42, RiverJobID: 43, State: contactapp.LegacyTagSyncQueued}, wecomTagAcceptanceFixture(wecomtag.OperationCatalogSync), stub.err
}

type legacyTagLiveStub struct {
	command       contactapp.LegacyTagLiveMutationCommand
	effectCommand wecomtag.QueueCommand
	err           error
}

func (stub *legacyTagLiveStub) Request(_ context.Context, command contactapp.LegacyTagLiveMutationCommand, externalUserID string, providerTagIDs []string) (contactapp.LegacyTagLiveMutationAcceptance, wecomtag.Acceptance, error) {
	stub.command = command
	operation := wecomtag.OperationMark
	if command.Operation == contactapp.LegacyTagLiveMutationUnmark {
		operation = wecomtag.OperationUnmark
	}
	stub.effectCommand = wecomtag.QueueCommand{LegacyReceiptID: 51, Actor: command.Actor, IdempotencyKey: command.IdempotencyKey, Operation: operation, ExternalUserID: externalUserID, ProviderTagIDs: providerTagIDs}
	return contactapp.LegacyTagLiveMutationAcceptance{ReceiptID: 51, EventID: 52, RiverJobID: 53, State: contactapp.LegacyTagLiveMutationQueued}, wecomTagAcceptanceFixture(operation), stub.err
}

type legacyTagStatusStub struct {
	status contactapp.LegacyTagExecutionGate
	err    error
	calls  int
}

type wecomTagEffectStub struct {
	reconcile wecomtag.ReconcileCommand
}

func wecomTagAcceptanceFixture(operation wecomtag.Operation) wecomtag.Acceptance {
	return wecomtag.Acceptance{
		EffectID: "eer_61", Operation: operation, State: eer.StateQueued, RiverJobID: 62,
		AcceptReceiptID: "eerop_63", QueueReceiptID: "eerop_64",
	}
}

func (stub *wecomTagEffectStub) Reconcile(_ context.Context, command wecomtag.ReconcileCommand) (wecomtag.Reconciliation, error) {
	stub.reconcile = command
	return wecomtag.Reconciliation{
		EffectID: command.EffectID, State: eer.StateReconciled, Resolution: command.Resolution, ReceiptID: "eerop_65",
	}, nil
}

func (stub *legacyTagStatusStub) Get(context.Context) (contactapp.LegacyTagExecutionGate, error) {
	stub.calls++
	return stub.status, stub.err
}

func (s *legacyTagStub) List(context.Context) (contactapp.LegacyTagCatalog, error) {
	s.listCalls++
	return s.catalog, s.err
}
func (s *legacyTagStub) GetGroup(_ context.Context, id int64) (contactapp.LegacyTagGroup, error) {
	for _, group := range s.catalog.Groups {
		if group.ID == id {
			return group, s.err
		}
	}
	return contactapp.LegacyTagGroup{}, contactapp.ErrLegacyTagNotFound
}
func (s *legacyTagStub) GetTag(_ context.Context, id int64) (contactapp.LegacyTag, error) {
	for _, tag := range s.catalog.Tags {
		if tag.ID == id {
			return tag, s.err
		}
	}
	return contactapp.LegacyTag{}, contactapp.ErrLegacyTagNotFound
}
func (s *legacyTagStub) CreateGroup(_ context.Context, c contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, contactapp.LegacyTag, error) {
	s.command = c
	s.writes++
	return s.catalog.Groups[0], s.catalog.Tags[0], s.err
}
func (s *legacyTagStub) UpdateGroup(_ context.Context, c contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, error) {
	s.command = c
	s.writes++
	return s.catalog.Groups[0], s.err
}
func (s *legacyTagStub) ArchiveGroup(_ context.Context, c contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, error) {
	s.command = c
	s.writes++
	return s.catalog.Groups[0], s.err
}
func (s *legacyTagStub) CreateTag(_ context.Context, c contactapp.LegacyTagCommand) (contactapp.LegacyTag, error) {
	s.command = c
	s.writes++
	return s.catalog.Tags[0], s.err
}
func (s *legacyTagStub) UpdateTag(_ context.Context, c contactapp.LegacyTagCommand) (contactapp.LegacyTag, error) {
	s.command = c
	s.writes++
	return s.catalog.Tags[0], s.err
}
func (s *legacyTagStub) ArchiveTag(_ context.Context, c contactapp.LegacyTagCommand) (contactapp.LegacyTag, error) {
	s.command = c
	s.writes++
	return s.catalog.Tags[0], s.err
}

func TestB02LegacyTagCatalogEnvelopeAndWriteCompatibility(t *testing.T) {
	stub := &legacyTagStub{catalog: legacyTagFixture()}
	router, auth := legacyTagRouter(t, stub)
	read := httptest.NewRecorder()
	router.ServeHTTP(read, legacyRequest(http.MethodGet, "/api/admin/wecom/tags", legacyToken(131)))
	if read.Code != 200 || !strings.Contains(read.Body.String(), `"groups"`) || !strings.Contains(read.Body.String(), `"total_tags":1`) || !strings.Contains(read.Body.String(), `"tag_limit":1000`) {
		t.Fatalf("read=%d %s", read.Code, read.Body.String())
	}
	if got := auth.capabilities(); len(got) != 1 || got[0] != authport.CapabilityCustomersRead {
		t.Fatalf("read capability=%v", got)
	}
	auth.reset()
	create := httptest.NewRecorder()
	req := legacyTagWriteRequest(http.MethodPost, "/api/admin/wecom/tag-groups", `{"group_name":"客户阶段","first_tag_name":"新客","actor":{"id":999},"idempotency_key":"body-key","trace_id":"trace-1","dry_run":false}`)
	req.Header.Set("Idempotency-Key", "header-key-0000001")
	router.ServeHTTP(create, req)
	if create.Code != 200 || stub.writes != 1 || stub.command.Actor != 1 || stub.command.IdempotencyKey != "header-key-0000001" || stub.command.TraceID != "trace-1" {
		t.Fatalf("create=%d writes=%d command=%#v body=%s", create.Code, stub.writes, stub.command, create.Body.String())
	}
	if got := auth.capabilities(); len(got) != 1 || got[0] != authport.CapabilityCustomersWrite {
		t.Fatalf("write capability=%v", got)
	}
	patch := httptest.NewRecorder()
	router.ServeHTTP(patch, legacyTagWriteRequest(http.MethodPatch, "/api/admin/wecom/tags/2", `{"tag_name":"成交"}`))
	if patch.Code != 200 || stub.command.TagID != 2 {
		t.Fatalf("patch=%d command=%#v body=%s", patch.Code, stub.command, patch.Body.String())
	}
}

func TestB02LegacyTagCatalogDegradesAndCSRFRejects(t *testing.T) {
	stub := &legacyTagStub{catalog: legacyTagFixture(), err: contactapp.ErrLegacyTagUnavailable}
	router, _ := legacyTagRouter(t, stub)
	read := httptest.NewRecorder()
	router.ServeHTTP(read, legacyRequest(http.MethodGet, "/api/admin/wecom/tags", legacyToken(132)))
	body := read.Body.String()
	if read.Code != 200 || !strings.Contains(body, `"error_code":"production_read_unavailable"`) || !strings.Contains(body, `"groups":[]`) || !strings.Contains(body, `"fixture_used":false`) {
		t.Fatalf("degraded=%d %s", read.Code, body)
	}
	stub.err = nil
	bad := legacyRequest(http.MethodPost, "/api/admin/wecom/tags", legacyToken(133))
	bad.Body = io.NopCloser(strings.NewReader(`{"group_id":1,"group_name":"组","tag_name":"标签"}`))
	bad.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, bad)
	if response.Code != http.StatusForbidden || stub.writes != 0 {
		t.Fatalf("csrf=%d writes=%d body=%s", response.Code, stub.writes, response.Body.String())
	}
}

func TestB02LegacyTagCatalogReadRequiresGlobalAdminOrOps(t *testing.T) {
	for _, role := range []authport.Role{authport.RoleAdmin, authport.RoleOps} {
		t.Run(string(role), func(t *testing.T) {
			tags := &legacyTagStub{catalog: legacyTagFixture()}
			auth := &legacyTagReadAuthStub{
				principal:     authport.Principal{AdminUserID: 7, Role: role},
				authorization: authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
			}
			handler := &Handler{auth: auth, legacyTags: tags}

			read := httptest.NewRecorder()
			legacyRoute(t, handler, authport.CapabilityCustomersRead, handler.ListLegacyTags).ServeHTTP(read, legacyRequest(http.MethodGet, "/api/admin/wecom/tags", legacyToken(151)))
			if read.Code != http.StatusOK || tags.listCalls != 1 {
				t.Fatalf("read=%d list_calls=%d body=%s", read.Code, tags.listCalls, read.Body.String())
			}

			page := httptest.NewRecorder()
			legacyRoute(t, handler, authport.CapabilityCustomersRead, handler.LegacyWecomTagsPage).ServeHTTP(page, legacyRequest(http.MethodGet, "/admin/wecom-tags", legacyToken(152)))
			if page.Code != http.StatusFound || page.Header().Get("Location") != "/?legacy_admin_path=%2Fadmin%2Fwecom-tags" {
				t.Fatalf("page=%d location=%q body=%s", page.Code, page.Header().Get("Location"), page.Body.String())
			}
		})
	}

	staffID := int64(71)
	tags := &legacyTagStub{catalog: legacyTagFixture()}
	auth := &legacyTagReadAuthStub{
		principal: authport.Principal{AdminUserID: 8, Role: authport.RoleSales, StaffID: &staffID},
		authorization: authport.Authorization{
			Capability:   authport.CapabilityCustomersRead,
			Scope:        authport.ScopeOwnerStaff,
			OwnerStaffID: staffID,
		},
	}
	handler := &Handler{auth: auth, legacyTags: tags}
	read := httptest.NewRecorder()
	legacyRoute(t, handler, authport.CapabilityCustomersRead, handler.ListLegacyTags).ServeHTTP(read, legacyRequest(http.MethodGet, "/api/admin/wecom/tags", legacyToken(153)))
	if read.Code != http.StatusForbidden || tags.listCalls != 0 {
		t.Fatalf("sales read=%d list_calls=%d body=%s", read.Code, tags.listCalls, read.Body.String())
	}
	page := httptest.NewRecorder()
	legacyRoute(t, handler, authport.CapabilityCustomersRead, handler.LegacyWecomTagsPage).ServeHTTP(page, legacyRequest(http.MethodGet, "/admin/wecom-tags", legacyToken(154)))
	if page.Code != http.StatusForbidden || page.Header().Get("Location") != "" || tags.listCalls != 0 {
		t.Fatalf("sales page=%d location=%q list_calls=%d body=%s", page.Code, page.Header().Get("Location"), tags.listCalls, page.Body.String())
	}
}

func TestB02ABLegacyTagExecutionGateRequiresGlobalAdminOrOps(t *testing.T) {
	for _, role := range []authport.Role{authport.RoleAdmin, authport.RoleOps} {
		t.Run(string(role), func(t *testing.T) {
			status := &legacyTagStatusStub{status: legacyTagExecutionGateFixture()}
			auth := &legacyTagReadAuthStub{
				principal:     authport.Principal{AdminUserID: 7, Role: role},
				authorization: authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
			}
			handler := &Handler{auth: auth, legacyTagStatus: status}
			response := httptest.NewRecorder()
			legacyRoute(t, handler, authport.CapabilityCustomersRead, handler.GetLegacyTagExecutionStatus).ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/wecom/tags/live/gate", legacyToken(155)))
			if response.Code != http.StatusOK || status.calls != 1 {
				t.Fatalf("%s gate=%d service_calls=%d body=%s", role, response.Code, status.calls, response.Body.String())
			}
		})
	}

	staffID := int64(71)
	status := &legacyTagStatusStub{status: legacyTagExecutionGateFixture()}
	auth := &legacyTagReadAuthStub{
		principal: authport.Principal{AdminUserID: 8, Role: authport.RoleSales, StaffID: &staffID},
		authorization: authport.Authorization{
			Capability:   authport.CapabilityCustomersRead,
			Scope:        authport.ScopeOwnerStaff,
			OwnerStaffID: staffID,
		},
	}
	handler := &Handler{auth: auth, legacyTagStatus: status}
	response := httptest.NewRecorder()
	legacyRoute(t, handler, authport.CapabilityCustomersRead, handler.GetLegacyTagExecutionStatus).ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/wecom/tags/live/gate", legacyToken(156)))
	if response.Code != http.StatusForbidden || status.calls != 0 {
		t.Fatalf("sales gate=%d service_calls=%d body=%s", response.Code, status.calls, response.Body.String())
	}
}

func TestB02LegacyTagCatalogCreateRequiresGlobalAdminOrOps(t *testing.T) {
	for _, role := range []authport.Role{authport.RoleAdmin, authport.RoleOps} {
		t.Run(string(role), func(t *testing.T) {
			tags := &legacyTagStub{catalog: legacyTagFixture()}
			auth := &legacyTagReadAuthStub{
				principal:     authport.Principal{AdminUserID: 7, Role: role},
				authorization: authport.Authorization{Capability: authport.CapabilityCustomersWrite, Scope: authport.ScopeGlobal},
			}
			handler := &Handler{auth: auth, legacyTags: tags}
			response := httptest.NewRecorder()
			request := legacyTagWriteRequest(http.MethodPost, "/api/admin/wecom/tag-groups", `{"group_name":"客户阶段","first_tag_name":"新客"}`)
			legacyRoute(t, handler, authport.CapabilityCustomersWrite, handler.CreateLegacyTagGroup).ServeHTTP(response, request)
			if response.Code != http.StatusOK || tags.writes != 1 {
				t.Fatalf("create=%d writes=%d body=%s", response.Code, tags.writes, response.Body.String())
			}
			var payload map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || !exactLegacyTagResponseKeys(payload, "ok", "reason", "source_status", "route_owner", "fallback_used", "real_external_call_executed", "sync_executed", "fixture_used", "dry_run", "group", "tag") ||
				payload["reason"] != "group_created" || payload["source_status"] != "local_catalog" ||
				payload["route_owner"] != "ai_crm_next" || payload["fallback_used"] != false ||
				payload["real_external_call_executed"] != false || payload["sync_executed"] != false ||
				payload["fixture_used"] != false || payload["dry_run"] != false {
				t.Fatalf("closed create payload=%v err=%v", payload, err)
			}
		})
	}

	staffID := int64(71)
	tags := &legacyTagStub{catalog: legacyTagFixture()}
	auth := &legacyTagReadAuthStub{
		principal: authport.Principal{AdminUserID: 8, Role: authport.RoleSales, StaffID: &staffID},
		authorization: authport.Authorization{
			Capability:   authport.CapabilityCustomersWrite,
			Scope:        authport.ScopeOwnerStaff,
			OwnerStaffID: staffID,
		},
	}
	handler := &Handler{auth: auth, legacyTags: tags}
	for _, attempt := range []struct {
		name    string
		method  string
		path    string
		body    string
		handler http.HandlerFunc
	}{
		{"create group", http.MethodPost, "/api/admin/wecom/tag-groups", `{"group_name":"客户阶段","first_tag_name":"新客"}`, handler.CreateLegacyTagGroup},
		{"update group", http.MethodPatch, "/api/admin/wecom/tag-groups/1", `{"group_name":"改名"}`, handler.MutateLegacyTagGroup},
		{"update tag", http.MethodPatch, "/api/admin/wecom/tags/2", `{"tag_name":"改名"}`, handler.MutateLegacyTag},
	} {
		t.Run(attempt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := legacyChannelWriteRequest(attempt.method, attempt.path, attempt.body)
			legacyRoute(t, handler, authport.CapabilityCustomersWrite, attempt.handler).ServeHTTP(response, request)
			if response.Code != http.StatusForbidden || tags.writes != 0 {
				t.Fatalf("sales mutation=%d writes=%d body=%s", response.Code, tags.writes, response.Body.String())
			}
		})
	}

	noDependencySales := &Handler{auth: auth}
	response := httptest.NewRecorder()
	request := legacyChannelWriteRequest(http.MethodPost, "/api/admin/wecom/tag-groups", `{"group_name":"客户阶段","first_tag_name":"新客"}`)
	legacyRoute(t, noDependencySales, authport.CapabilityCustomersWrite, noDependencySales.CreateLegacyTagGroup).ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || tags.writes != 0 {
		t.Fatalf("sales unavailable create=%d writes=%d body=%s", response.Code, tags.writes, response.Body.String())
	}

	for _, role := range []authport.Role{authport.RoleAdmin, authport.RoleOps} {
		t.Run("unavailable "+string(role), func(t *testing.T) {
			auth := &legacyTagReadAuthStub{
				principal:     authport.Principal{AdminUserID: 7, Role: role},
				authorization: authport.Authorization{Capability: authport.CapabilityCustomersWrite, Scope: authport.ScopeGlobal},
			}
			handler := &Handler{auth: auth}
			response := httptest.NewRecorder()
			request := legacyTagWriteRequest(http.MethodPost, "/api/admin/wecom/tag-groups", `{"group_name":"客户阶段","first_tag_name":"新客"}`)
			legacyRoute(t, handler, authport.CapabilityCustomersWrite, handler.CreateLegacyTagGroup).ServeHTTP(response, request)
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("%s unavailable create=%d body=%s", role, response.Code, response.Body.String())
			}
		})
	}
}

func TestB02LegacyTagCommandRequiresStrictHeaderAndPreservesDryRun(t *testing.T) {
	for _, test := range []struct {
		name    string
		request func() *http.Request
	}{
		{
			name: "missing header does not fall back to body",
			request: func() *http.Request {
				request := legacyChannelWriteRequest(http.MethodPost, "/api/admin/wecom/tag-groups", `{"group_name":"客户阶段","first_tag_name":"新客","idempotency_key":"body-key-0000001"}`)
				return request
			},
		},
		{
			name: "short header",
			request: func() *http.Request {
				request := legacyTagWriteRequest(http.MethodPost, "/api/admin/wecom/tag-groups", `{"group_name":"客户阶段","first_tag_name":"新客"}`)
				request.Header.Set("Idempotency-Key", "too-short")
				return request
			},
		},
		{
			name: "overlong header",
			request: func() *http.Request {
				request := legacyTagWriteRequest(http.MethodPost, "/api/admin/wecom/tag-groups", `{"group_name":"客户阶段","first_tag_name":"新客"}`)
				request.Header.Set("Idempotency-Key", strings.Repeat("x", 129))
				return request
			},
		},
		{
			name: "multiple headers",
			request: func() *http.Request {
				request := legacyTagWriteRequest(http.MethodPost, "/api/admin/wecom/tag-groups", `{"group_name":"客户阶段","first_tag_name":"新客"}`)
				request.Header.Add("Idempotency-Key", "tag-write-key-0002")
				return request
			},
		},
		{
			name: "not exactly trimmed",
			request: func() *http.Request {
				request := legacyTagWriteRequest(http.MethodPost, "/api/admin/wecom/tag-groups", `{"group_name":"客户阶段","first_tag_name":"新客"}`)
				request.Header.Set("Idempotency-Key", " tag-write-key-0001")
				return request
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyTagStub{catalog: legacyTagFixture()}
			router, _ := legacyTagRouter(t, stub)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, test.request())
			if response.Code != http.StatusBadRequest || stub.writes != 0 {
				t.Fatalf("status=%d writes=%d body=%s", response.Code, stub.writes, response.Body.String())
			}
		})
	}

	stub := &legacyTagStub{catalog: legacyTagFixture()}
	router, _ := legacyTagRouter(t, stub)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyTagWriteRequest(http.MethodPost, "/api/admin/wecom/tag-groups", `{"group_name":"客户阶段","first_tag_name":"新客","dry_run":true}`))
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); response.Code != http.StatusOK || err != nil || !exactLegacyTagResponseKeys(payload, "ok", "reason", "source_status", "route_owner", "fallback_used", "real_external_call_executed", "sync_executed", "fixture_used", "dry_run") ||
		payload["reason"] != "group_create_validated" || payload["dry_run"] != true ||
		payload["group"] != nil || payload["tag"] != nil || stub.writes != 0 {
		t.Fatalf("dry run status=%d writes=%d payload=%v err=%v", response.Code, stub.writes, payload, err)
	}
}

func TestB02ABLegacyTagSharedRoutesPreserveSessionCSRFAndQueuedBoundary(t *testing.T) {
	tags := &legacyTagStub{catalog: legacyTagFixture()}
	sync := &legacyTagSyncStub{}
	live := &legacyTagLiveStub{}
	status := &legacyTagStatusStub{status: legacyTagExecutionGateFixture()}
	typed := &wecomTagEffectStub{}
	router, auth := legacyTagRouterWithExecutionAndTyped(t, tags, sync, live, status, typed)

	for _, target := range []string{"/api/admin/wecom/tag-groups", "/api/admin/wecom/tag-groups/1", "/api/admin/wecom/tags/2", "/api/admin/wecom/tags/live/gate"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, target, legacyToken(145)))
		if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `"executed":true`) || !strings.Contains(response.Body.String(), `"real_external_call_executed":false`) {
			t.Fatalf("read %s=%d %s", target, response.Code, response.Body.String())
		}
		if target == "/api/admin/wecom/tags/live/gate" && (!exactLegacyTagResponseKeys(mustJSONMap(t, response.Body.Bytes()), "provider_execution_eligible", "local_command_acceptance_available", "local_queue_available", "sync_executed", "observed_at", "real_external_call_executed") || strings.Contains(response.Body.String(), "mode") || strings.Contains(response.Body.String(), "payload")) {
			t.Fatalf("gate projection=%s", response.Body.String())
		}
	}
	page := httptest.NewRecorder()
	router.ServeHTTP(page, legacyRequest(http.MethodGet, "/admin/wecom-tags", legacyToken(146)))
	if page.Code != http.StatusFound || page.Header().Get("Location") != "/?legacy_admin_path=%2Fadmin%2Fwecom-tags" {
		t.Fatalf("page=%d location=%q", page.Code, page.Header().Get("Location"))
	}

	manual := httptest.NewRecorder()
	request := legacyChannelWriteRequest(http.MethodPost, "/api/admin/wecom/tags/sync", `{"trace_id":"tag-sync","idempotency_key":"body-key"}`)
	request.Header.Set("Idempotency-Key", "header-key-000001")
	router.ServeHTTP(manual, request)
	if manual.Code != http.StatusAccepted || sync.command.Actor != 1 || sync.command.Kind != contactapp.LegacyTagSyncManual || sync.command.IdempotencyKey != "header-key-000001" || sync.effectCommand.Operation != wecomtag.OperationCatalogSync || sync.effectCommand.LegacyReceiptID != 41 || !strings.Contains(manual.Body.String(), `"effect_id":"eer_61"`) || strings.Contains(manual.Body.String(), `"executed":true`) {
		t.Fatalf("sync=%d command=%#v body=%s", manual.Code, sync.command, manual.Body.String())
	}
	due := httptest.NewRecorder()
	router.ServeHTTP(due, legacyTagWriteRequest(http.MethodPost, "/api/admin/wecom/tags/sync-due", `{}`))
	if due.Code != http.StatusAccepted || sync.command.Kind != contactapp.LegacyTagSyncDue {
		t.Fatalf("sync-due=%d command=%#v", due.Code, sync.command)
	}

	mark := httptest.NewRecorder()
	router.ServeHTTP(mark, legacyTagWriteRequest(http.MethodPost, "/api/admin/wecom/tags/live/mark", `{"tag_id":2,"external_userid":"u-1","trace_id":"tag-live"}`))
	if mark.Code != http.StatusAccepted || live.command.Actor != 1 || live.command.Operation != contactapp.LegacyTagLiveMutationMark || live.effectCommand.Operation != wecomtag.OperationMark || live.effectCommand.ExternalUserID != "u-1" || len(live.effectCommand.ProviderTagIDs) != 1 || live.effectCommand.ProviderTagIDs[0] != "2" || !strings.Contains(mark.Body.String(), `"real_external_call_executed":false`) {
		t.Fatalf("mark=%d command=%#v body=%s", mark.Code, live.command, mark.Body.String())
	}
	reconcile := httptest.NewRecorder()
	reconcileRequest := legacyTagWriteRequest(http.MethodPost, "/api/admin/wecom/tag-effects/eer_61/reconcile", `{"generation":2,"fence":3,"lease_expires_at":"2026-08-25T12:00:00Z","evidence_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","resolution":"provider_not_applied"}`)
	router.ServeHTTP(reconcile, reconcileRequest)
	if reconcile.Code != http.StatusOK || typed.reconcile.EffectID != "eer_61" || typed.reconcile.Actor != 1 || typed.reconcile.Resolution != wecomtag.ResolutionProviderNotApplied || !strings.Contains(reconcile.Body.String(), `"receipt_id":"eerop_65"`) {
		t.Fatalf("reconcile=%d command=%#v body=%s", reconcile.Code, typed.reconcile, reconcile.Body.String())
	}
	missingCSRF := httptest.NewRecorder()
	bad := legacyRequest(http.MethodPost, "/api/admin/wecom/tags/live/unmark", legacyToken(147))
	bad.Header.Set("Content-Type", "application/json")
	bad.Body = io.NopCloser(strings.NewReader(`{"tag_id":2}`))
	router.ServeHTTP(missingCSRF, bad)
	if missingCSRF.Code != http.StatusForbidden {
		t.Fatalf("missing csrf=%d %s", missingCSRF.Code, missingCSRF.Body.String())
	}
	if seen := auth.capabilities(); len(seen) != 9 || seen[0] != authport.CapabilityCustomersRead || seen[4] != authport.CapabilityCustomersRead || seen[5] != authport.CapabilityCustomersWrite || seen[8] != authport.CapabilityOperationsManage {
		t.Fatalf("capabilities=%v", seen)
	}
}

func TestWC01LegacyTagEffectPayloadIsTypedAndClosed(t *testing.T) {
	for _, raw := range []string{
		`{"external_userid":"u-1"}`,
		`{"external_userid":"u-1","tag_id":"t-1","tag_ids":["t-1"]}`,
		`{"external_userid":"u-1","tag_ids":[]}`,
		`{"external_userid":"u-1","tag_id":"t-1","unexpected":true}`,
	} {
		if _, _, ok := legacyTagLivePayload(json.RawMessage(raw)); ok {
			t.Fatalf("accepted invalid payload %s", raw)
		}
	}
	externalUserID, tagIDs, ok := legacyTagLivePayload(json.RawMessage(`{"external_userid":"u-1","tag_ids":["t-2","t-1"]}`))
	if !ok || externalUserID != "u-1" || len(tagIDs) != 2 || tagIDs[0] != "t-2" || tagIDs[1] != "t-1" {
		t.Fatalf("external_userid=%q tag_ids=%v ok=%t", externalUserID, tagIDs, ok)
	}
	if legacyTagSyncPayload(json.RawMessage(`{"unexpected":true}`)) || !legacyTagSyncPayload(json.RawMessage(`{"trace_id":"trace"}`)) {
		t.Fatal("sync payload allowlist drifted")
	}
}

func legacyTagWriteRequest(method, path, body string) *http.Request {
	request := legacyChannelWriteRequest(method, path, body)
	request.Header.Set("Idempotency-Key", "tag-write-key-0001")
	return request
}

func exactLegacyTagResponseKeys(payload map[string]any, keys ...string) bool {
	if len(payload) != len(keys) {
		return false
	}
	for _, key := range keys {
		if _, exists := payload[key]; !exists {
			return false
		}
	}
	return true
}

func legacyTagFixture() contactapp.LegacyTagCatalog {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	return contactapp.LegacyTagCatalog{Groups: []contactapp.LegacyTagGroup{{ID: 1, Name: "客户阶段"}}, Tags: []contactapp.LegacyTag{{ID: 2, GroupID: 1, GroupName: "客户阶段", Name: "新客"}}, SyncedAt: now}
}
func legacyTagRouter(t *testing.T, tags legacyTagApplication) (http.Handler, *recordingAuth) {
	return legacyTagRouterWithExecution(t, tags, &legacyTagSyncStub{}, &legacyTagLiveStub{}, &legacyTagStatusStub{status: legacyTagExecutionGateFixture()})
}

func legacyTagExecutionGateFixture() contactapp.LegacyTagExecutionGate {
	return contactapp.LegacyTagExecutionGate{LocalCommandAcceptanceAvailable: true, LocalQueueAvailable: true, ObservedAt: time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)}
}

func mustJSONMap(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return value
}
func legacyTagRouterWithExecution(t *testing.T, tags legacyTagApplication, sync legacyTagSyncApplication, live legacyTagLiveMutationApplication, status legacyTagExecutionStatusApplication) (http.Handler, *recordingAuth) {
	return legacyTagRouterWithExecutionAndTyped(t, tags, sync, live, status, &wecomTagEffectStub{})
}

func legacyTagRouterWithExecutionAndTyped(t *testing.T, tags legacyTagApplication, sync legacyTagSyncApplication, live legacyTagLiveMutationApplication, status legacyTagExecutionStatusApplication, typed wecomTagEffectApplication) (http.Handler, *recordingAuth) {
	t.Helper()
	service := &recordingAuth{}
	legacy, e := NewHandlerWithOutboundProductsMediaAndSurvey(service, &legacyCustomerStub{result: legacyCustomerResult()}, &legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, &legacyProductStub{}, &legacyMediaStub{}, &legacySurveyStub{})
	if e != nil {
		t.Fatal(e)
	}
	legacy.legacyTags = tags
	legacy.legacyTagSync = sync
	legacy.legacyTagLive = live
	legacy.legacyTagStatus = status
	legacy.wecomTagEffects = typed
	authHandler, e := authhttp.NewHandler(service)
	if e != nil {
		t.Fatal(e)
	}
	router, e := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if e != nil {
		t.Fatal(e)
	}
	return router, service
}
