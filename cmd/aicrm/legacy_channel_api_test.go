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
	router.ServeHTTP(create, legacyChannelConfigurationWriteRequest(http.MethodPost, "/api/admin/channels", `{"channel_name":"公开课","channel_code":"course","status":"active","welcome_message":"欢迎","assignment_config_json":{}}`))
	if create.Code != http.StatusCreated || stub.writes != 1 || stub.create.Actor != 1 || stub.create.IdempotencyKey == "" || !strings.Contains(create.Body.String(), `"provider_execution_eligible":false`) || !strings.Contains(create.Body.String(), `"real_external_call_executed":false`) {
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
	router.ServeHTTP(patch, legacyChannelConfigurationWriteRequest(http.MethodPatch, "/api/admin/channels/1", `{"status":"archived"}`))
	if patch.Code != 200 || stub.update.ChannelID != 1 {
		t.Fatalf("patch=%d update=%#v body=%s", patch.Code, stub.update, patch.Body.String())
	}
}

func TestC01LegacyChannelRequiresExplicitIdempotencyKey(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*http.Request)
		wantStatus int
	}{
		{name: "missing idempotency key", mutate: func(request *http.Request) { request.Header.Del("Idempotency-Key") }, wantStatus: http.StatusBadRequest},
		{name: "duplicate idempotency key", mutate: func(request *http.Request) { request.Header.Add("Idempotency-Key", "channel-test:fedcba0987654321") }, wantStatus: http.StatusBadRequest},
		{name: "short idempotency key", mutate: func(request *http.Request) { request.Header.Set("Idempotency-Key", "too-short") }, wantStatus: http.StatusBadRequest},
		{name: "long idempotency key", mutate: func(request *http.Request) { request.Header.Set("Idempotency-Key", strings.Repeat("x", 129)) }, wantStatus: http.StatusBadRequest},
		{name: "padded idempotency key", mutate: func(request *http.Request) { request.Header.Set("Idempotency-Key", " channel-test:1234567890abcdef ") }, wantStatus: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyChannelStub{item: legacyChannelItem()}
			router, _ := legacyChannelRouter(t, stub)
			request := legacyChannelConfigurationWriteRequest(http.MethodPost, "/api/admin/channels", `{"channel_name":"本地渠道","channel_code":"local","status":"active"}`)
			test.mutate(request)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus || stub.writes != 0 {
				t.Fatalf("status=%d want=%d writes=%d body=%s", response.Code, test.wantStatus, stub.writes, response.Body.String())
			}
		})
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

func TestC01LegacyChannelListNeverExposesLegacyProjection(t *testing.T) {
	item := legacyChannelItem()
	item.LegacyProjection = json.RawMessage(`{"channel_name":"公开课","channel_code":"course","status":"active","welcome_message":"must_not_list","owner_staff_id":"staff-1","entry_tag_id":"tag-1","welcome_image_library_ids":[41],"assignment_mode":"single_owner","assignment_config_json":{"ratio":100},"link_url":"https://outside.invalid/link","final_url":"https://outside.invalid/final","share_url":"https://outside.invalid/share"}`)
	router, _ := legacyChannelRouter(t, &legacyChannelStub{item: item, rows: []contactapp.Channel{item}})

	list := httptest.NewRecorder()
	router.ServeHTTP(list, legacyRequest(http.MethodGet, "/api/admin/channels?limit=300&include_archived=true", legacyToken(95)))
	if list.Code != http.StatusOK {
		t.Fatalf("list=%d body=%s", list.Code, list.Body.String())
	}
	var listBody struct {
		Channels []map[string]any `json:"channels"`
	}
	if err := json.NewDecoder(list.Body).Decode(&listBody); err != nil || len(listBody.Channels) != 1 {
		t.Fatalf("list body=%#v err=%v", listBody, err)
	}
	row := listBody.Channels[0]
	wantKeys := map[string]bool{"id": true, "channel_name": true, "channel_code": true, "status": true, "assignee_count": true, "channel_contact_count": true, "created_at": true, "updated_at": true}
	if len(row) != len(wantKeys) || row["assignee_count"] != float64(0) || row["channel_contact_count"] != float64(0) {
		t.Fatalf("list row=%#v", row)
	}
	for key := range row {
		if !wantKeys[key] {
			t.Fatalf("unexpected list field %q in %#v", key, row)
		}
	}

	detail := httptest.NewRecorder()
	router.ServeHTTP(detail, legacyRequest(http.MethodGet, "/api/admin/channels/1", legacyToken(96)))
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"welcome_message":"must_not_list"`) {
		t.Fatalf("detail=%d body=%s", detail.Code, detail.Body.String())
	}
}

func TestC01LegacyChannelProjectsValidatedAssignee(t *testing.T) {
	item := legacyChannelItem()
	item.Assignees = []contactapp.ChannelAssignee{{WeComUserID: "staff-7", DisplayName: "运营成员"}}
	item.LegacyProjection = json.RawMessage(`{"schema_version":1,"channel_type":"qrcode","owner_staff_id":"staff-7"}`)
	projected, err := legacyChannel(item)
	if err != nil {
		t.Fatal(err)
	}
	if projected["assignee_count"] != 1 {
		t.Fatalf("assignee_count=%#v", projected["assignee_count"])
	}
	assignees, ok := projected["assignees"].([]map[string]string)
	if !ok || len(assignees) != 1 || assignees[0]["wecom_userid"] != "staff-7" || assignees[0]["display_name"] != "运营成员" {
		t.Fatalf("assignees=%#v", projected["assignees"])
	}
	if row := legacyChannelListItem(item); row["assignee_count"] != 1 {
		t.Fatalf("list assignee_count=%#v", row["assignee_count"])
	}
}

func TestC01LegacyChannelKeepsFrozenSingleAssigneeContract(t *testing.T) {
	item := legacyChannelItem()
	item.Assignees = []contactapp.ChannelAssignee{
		{WeComUserID: "staff-1", DisplayName: "成员一", Status: "active", Priority: 1},
		{WeComUserID: "staff-2", DisplayName: "成员二", Status: "active", Priority: 2},
	}
	projected, err := legacyChannel(item)
	if err != nil {
		t.Fatal(err)
	}
	assignees, ok := projected["assignees"].([]map[string]string)
	if !ok || len(assignees) != 1 || assignees[0]["wecom_userid"] != "staff-1" || projected["assignee_count"] != 1 {
		t.Fatalf("legacy detail assignees/count = %#v/%#v", projected["assignees"], projected["assignee_count"])
	}
	if row := legacyChannelListItem(item); row["assignee_count"] != 1 {
		t.Fatalf("legacy list assignee_count = %#v", row["assignee_count"])
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

func legacyChannelConfigurationWriteRequest(method, path, body string) *http.Request {
	request := legacyChannelWriteRequest(method, path, body)
	request.Header.Set("Idempotency-Key", "channel-test:1234567890abcdef")
	return request
}
func legacyChannelItem() contactapp.Channel {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	projection, _ := json.Marshal(map[string]any{"schema_version": 1, "channel_type": "qrcode", "carrier_type": "qrcode", "channel_code": "course", "channel_name": "公开课", "status": "active"})
	return contactapp.Channel{ID: 1, ChannelCode: "course", ChannelName: "公开课", Status: "active", LegacyProjection: projection, CreatedBy: 1, UpdatedBy: 1, CreatedAt: now, UpdatedAt: now}
}
