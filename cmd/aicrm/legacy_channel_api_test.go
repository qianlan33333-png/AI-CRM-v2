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
)

type legacyChannelStub struct {
	item   contactapp.Channel
	rows   []contactapp.Channel
	err    error
	create contactapp.CreateChannelCommand
	update contactapp.UpdateChannelCommand
	writes int
}

func (stub *legacyChannelStub) ListChannels(context.Context, int32, string, bool) ([]contactapp.Channel, error) {
	return stub.rows, stub.err
}
func (stub *legacyChannelStub) GetChannel(context.Context, int64) (contactapp.Channel, error) {
	return stub.item, stub.err
}
func (stub *legacyChannelStub) CreateChannel(_ context.Context, c contactapp.CreateChannelCommand) (contactapp.Channel, error) {
	stub.create = c
	stub.writes++
	return stub.item, stub.err
}
func (stub *legacyChannelStub) UpdateChannel(_ context.Context, c contactapp.UpdateChannelCommand) (contactapp.Channel, error) {
	stub.update = c
	stub.writes++
	return stub.item, stub.err
}

func TestC01LegacyChannelRoutesRBACCSRFRoundTrip(t *testing.T) {
	item := legacyChannelItem()
	stub := &legacyChannelStub{item: item, rows: []contactapp.Channel{item}}
	router, auth := legacyChannelRouter(t, stub)
	create := httptest.NewRecorder()
	router.ServeHTTP(create, legacyChannelWriteRequest(http.MethodPost, "/api/admin/channels", `{"channel_name":"公开课","channel_code":"course","status":"active","welcome_message":"欢迎","assignment_config_json":{}}`))
	if create.Code != http.StatusCreated || stub.writes != 1 || stub.create.Actor != 1 || stub.create.IdempotencyKey == "" || !strings.Contains(create.Body.String(), `"real_external_call_executed":false`) {
		t.Fatalf("create=%d writes=%d command=%#v body=%s", create.Code, stub.writes, stub.create, create.Body.String())
	}
	if seen := auth.capabilities(); len(seen) != 1 || seen[0] != authport.CapabilityChannelsWrite {
		t.Fatalf("capabilities=%v", seen)
	}
	auth.reset()
	list := httptest.NewRecorder()
	router.ServeHTTP(list, legacyRequest(http.MethodGet, "/api/admin/channels?limit=300", legacyToken(90)))
	if list.Code != 200 || !strings.Contains(list.Body.String(), `"channels"`) {
		t.Fatalf("list=%d %s", list.Code, list.Body.String())
	}
	detail := httptest.NewRecorder()
	router.ServeHTTP(detail, legacyRequest(http.MethodGet, "/api/admin/channels/1", legacyToken(91)))
	if detail.Code != 200 || !strings.Contains(detail.Body.String(), `"channel"`) {
		t.Fatalf("detail=%d %s", detail.Code, detail.Body.String())
	}
	patch := httptest.NewRecorder()
	router.ServeHTTP(patch, legacyChannelWriteRequest(http.MethodPatch, "/api/admin/channels/1", `{"status":"archived"}`))
	if patch.Code != 200 || stub.update.ChannelID != 1 {
		t.Fatalf("patch=%d update=%#v body=%s", patch.Code, stub.update, patch.Body.String())
	}
}

func TestC01LegacyChannelRejectsCrossOriginUnknownAndStableErrors(t *testing.T) {
	stub := &legacyChannelStub{item: legacyChannelItem()}
	router, _ := legacyChannelRouter(t, stub)
	cross := legacyChannelWriteRequest(http.MethodPost, "/api/admin/channels", `{"channel_name":"x"}`)
	cross.Header.Set("Origin", "https://cross.invalid")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, cross)
	if response.Code != http.StatusForbidden || stub.writes != 0 {
		t.Fatalf("cross=%d writes=%d", response.Code, stub.writes)
	}
	unknown := httptest.NewRecorder()
	router.ServeHTTP(unknown, legacyChannelWriteRequest(http.MethodPost, "/api/admin/channels", `{"tenant`+`_id":"forbidden"}`))
	if unknown.Code != http.StatusBadRequest || stub.writes != 0 {
		t.Fatalf("unknown=%d writes=%d body=%s", unknown.Code, stub.writes, unknown.Body.String())
	}
	stub.err = contactapp.ErrChannelNotFound
	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, legacyRequest(http.MethodGet, "/api/admin/channels/99", legacyToken(92)))
	if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), `"ok":false`) {
		t.Fatalf("missing=%d %s", missing.Code, missing.Body.String())
	}
}

func legacyChannelRouter(t *testing.T, channels legacyChannelApplication) (http.Handler, *recordingAuth) {
	t.Helper()
	service := &recordingAuth{}
	legacy, err := NewHandlerWithOutboundProductsMediaAndSurvey(service, &legacyCustomerStub{result: legacyCustomerResult()}, &legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, &legacyProductStub{}, &legacyMediaStub{}, &legacySurveyStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.channels = channels
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router, service
}
func legacyChannelWriteRequest(method, path, body string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://example.com")
	request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(93)})
	request.AddCookie(&http.Cookie{Name: LegacyCSRFCookieName, Value: legacyToken(94)})
	return request
}
func legacyChannelItem() contactapp.Channel {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	projection, _ := json.Marshal(map[string]any{"schema_version": 1, "channel_type": "qrcode", "carrier_type": "qrcode", "channel_code": "course", "channel_name": "公开课", "status": "active"})
	return contactapp.Channel{ID: 1, ChannelCode: "course", ChannelName: "公开课", Status: "active", LegacyProjection: projection, CreatedBy: 1, UpdatedBy: 1, CreatedAt: now, UpdatedAt: now}
}
