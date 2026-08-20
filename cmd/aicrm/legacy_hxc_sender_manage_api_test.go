package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
)

func TestHXCSenderManagementRoutesCompleteLocalLifecycleWithReadback(t *testing.T) {
	source := &hxcReadStub{projection: hxcManageProjection()}
	manager := &hxcManageStub{source: source}
	router := hxcSenderManageRouter(t, &legacyAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}}, source, manager)

	save := hxcManageRequest(http.MethodPost, legacyHXCSenderReadPath, `{"id":"cfg-2","sender_userid":"bob","display_name":"Bob","priority":3,"is_active":true}`)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, save)
	if response.Code != http.StatusOK || manager.saveCalls != 1 || manager.last.Actor != "admin:7" || !strings.Contains(response.Body.String(), `"operation":"saved"`) || !strings.Contains(response.Body.String(), `"readback_confirmed":true`) || !strings.Contains(response.Body.String(), `"real_external_call_executed":false`) {
		t.Fatalf("save status=%d manager=%#v body=%s", response.Code, manager, response.Body.String())
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, hxcManageRequest(http.MethodPut, legacyHXCSenderReorderPath, `{"ids":["cfg-2","cfg-1"]}`))
	if response.Code != http.StatusOK || manager.reorderCalls != 1 || !strings.Contains(response.Body.String(), `"operation":"reordered"`) {
		t.Fatalf("reorder status=%d calls=%d body=%s", response.Code, manager.reorderCalls, response.Body.String())
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, hxcManageRequest(http.MethodDelete, "/api/admin/hxc-dashboard/send-config/alice", ""))
	if response.Code != http.StatusOK || manager.archiveCalls != 1 || manager.last.SenderUserID != "alice" || !strings.Contains(response.Body.String(), `"operation":"archived"`) {
		t.Fatalf("archive status=%d manager=%#v body=%s", response.Code, manager, response.Body.String())
	}
}

func TestHXCSenderManagementFailsClosedBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name      string
		principal authport.Principal
		mutate    func(*http.Request)
		want      int
	}{
		{name: "ops denied", principal: authport.Principal{AdminUserID: 8, Role: authport.RoleOps}, want: http.StatusForbidden},
		{name: "missing csrf", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, mutate: func(request *http.Request) {
			request.Header.Del("Origin")
			request.Header.Del("X-CSRF-Token")
			request.Header.Del("Cookie")
		}, want: http.StatusUnauthorized},
		{name: "duplicate key", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, mutate: func(request *http.Request) { request.Header.Add("Idempotency-Key", "hxc-write-key-00000002") }, want: http.StatusForbidden},
		{name: "unknown field", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, mutate: func(request *http.Request) {}, want: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := &hxcReadStub{projection: hxcManageProjection()}
			manager := &hxcManageStub{source: source}
			router := hxcSenderManageRouter(t, &legacyAuthStub{principal: test.principal}, source, manager)
			body := `{"id":"cfg-2","sender_userid":"bob","display_name":"Bob","priority":3,"is_active":true}`
			if test.name == "unknown field" {
				body = `{"id":"cfg-2","sender_userid":"bob","display_name":"Bob","priority":3,"is_active":true,"provider":true}`
			}
			request := hxcManageRequest(http.MethodPost, legacyHXCSenderReadPath, body)
			if test.mutate != nil {
				test.mutate(request)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.want || manager.saveCalls != 0 {
				t.Fatalf("status=%d want=%d calls=%d body=%s", response.Code, test.want, manager.saveCalls, response.Body.String())
			}
		})
	}
}

func hxcManageRequest(method, path, body string) *http.Request {
	request := legacyChannelWriteRequest(method, path, body)
	request.Header.Set("Idempotency-Key", "hxc-write-key-00000001")
	return request
}

func hxcSenderManageRouter(t *testing.T, service authport.Service, source hxcSenderRead, manager hxcSenderManage) http.Handler {
	t.Helper()
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := NewHandler(service, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.hxcSender = &hxcSenderHandler{reader: source, manager: manager}
	router, err := newAPIHandlerWithAll(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy, nil)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func hxcManageProjection() hxcapp.Projection {
	stamp := time.Date(2026, 8, 20, 5, 0, 0, 0, time.UTC)
	return hxcapp.Projection{
		SendConfigs:       []hxcport.SenderConfig{{ID: "cfg-1", SenderUserID: "alice", DisplayName: "Alice", Priority: 0, IsActive: true, CreatedAt: stamp, UpdatedAt: stamp}},
		Directory:         []hxcapp.Candidate{{WeComUserID: "alice", DisplayName: "Alice", IsSender: true, Priority: 0, IsActive: true}, {WeComUserID: "bob", DisplayName: "Bob", IsSender: false}},
		ActiveSenderCount: 1,
		LastSyncedAt:      stamp,
	}
}

type hxcManageStub struct {
	source       *hxcReadStub
	last         hxcapp.ManageCommand
	saveCalls    int
	reorderCalls int
	archiveCalls int
}

func (stub *hxcManageStub) Save(_ context.Context, command hxcapp.ManageCommand) (hxcport.SenderConfig, error) {
	stub.last = command
	stub.saveCalls++
	stamp := time.Date(2026, 8, 20, 5, 1, 0, 0, time.UTC)
	item := hxcport.SenderConfig{ID: command.ID, SenderUserID: command.SenderUserID, DisplayName: command.DisplayName, Priority: command.Priority, IsActive: command.Active, CreatedAt: stamp, UpdatedAt: stamp}
	stub.source.projection.SendConfigs = append(stub.source.projection.SendConfigs, item)
	return item, nil
}

func (stub *hxcManageStub) Reorder(_ context.Context, _, _ string, ids []string) ([]hxcport.SenderConfig, error) {
	stub.reorderCalls++
	byID := map[string]hxcport.SenderConfig{}
	for _, item := range stub.source.projection.SendConfigs {
		byID[item.ID] = item
	}
	items := make([]hxcport.SenderConfig, 0, len(ids))
	for priority, id := range ids {
		item := byID[id]
		item.Priority = priority
		items = append(items, item)
	}
	stub.source.projection.SendConfigs = items
	return items, nil
}

func (stub *hxcManageStub) Archive(_ context.Context, command hxcapp.ManageCommand) error {
	stub.last = command
	stub.archiveCalls++
	items := stub.source.projection.SendConfigs[:0]
	for _, item := range stub.source.projection.SendConfigs {
		if item.SenderUserID != command.SenderUserID {
			items = append(items, item)
		}
	}
	stub.source.projection.SendConfigs = items
	return nil
}
