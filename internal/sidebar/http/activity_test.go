package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	customer360port "github.com/qianlan33333-png/AI-CRM-v2/internal/customer360/port"
	sidebarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/app"
)

type sidebarActivityTimelineFake struct {
	read  contactport.Customer360Read
	err   error
	calls int
}

func (fake *sidebarActivityTimelineFake) ReadCustomer360(context.Context, contactport.Customer360ReadInput) (contactport.Customer360Read, error) {
	fake.calls++
	return fake.read, fake.err
}

type sidebarActivityChatFake struct {
	page  customer360port.CustomerChatActivityPage
	err   error
	calls int
}

func (fake *sidebarActivityChatFake) ListCustomerChatActivity(context.Context, customer360port.CustomerChatActivityQuery) (customer360port.CustomerChatActivityPage, error) {
	fake.calls++
	return fake.page, fake.err
}

func TestActivityHandlerRequiresGetBrowserSessionAndContextHeader(t *testing.T) {
	handler, timeline, chat, token, authenticated := sidebarActivityHandler(t)
	call := func(method, suppliedToken string, ctx context.Context) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, "/api/sidebar/v2/activity/timeline", nil).WithContext(ctx)
		if suppliedToken != "" {
			request.Header.Set("X-Sidebar-Context-Token", suppliedToken)
		}
		response := httptest.NewRecorder()
		handler.Timeline(response, request)
		return response
	}

	wrongMethod := call(http.MethodPost, token, authenticated)
	if wrongMethod.Code != http.StatusMethodNotAllowed || wrongMethod.Header().Get("Allow") != http.MethodGet || timeline.calls != 0 || chat.calls != 0 {
		t.Fatalf("wrong method status/allow/calls=%d/%q/%d/%d", wrongMethod.Code, wrongMethod.Header().Get("Allow"), timeline.calls, chat.calls)
	}
	missingHeader := call(http.MethodGet, "", authenticated)
	if missingHeader.Code != http.StatusUnauthorized || timeline.calls != 0 || chat.calls != 0 {
		t.Fatalf("missing header status/calls=%d/%d/%d", missingHeader.Code, timeline.calls, chat.calls)
	}
	missingSession := call(http.MethodGet, token, context.Background())
	if missingSession.Code != http.StatusUnauthorized || timeline.calls != 0 || chat.calls != 0 {
		t.Fatalf("missing session status/calls=%d/%d/%d", missingSession.Code, timeline.calls, chat.calls)
	}
}

func TestActivityHandlerProjectsOnlySafeActivityFields(t *testing.T) {
	handler, timeline, chat, token, authenticated := sidebarActivityHandler(t)
	call := func(path string, invoke func(http.ResponseWriter, *http.Request)) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, path, nil).WithContext(authenticated)
		request.Header.Set("X-Sidebar-Context-Token", token)
		response := httptest.NewRecorder()
		invoke(response, request)
		return response
	}
	timelineResponse := call("/api/sidebar/v2/activity/timeline", handler.Timeline)
	if timelineResponse.Code != http.StatusOK || timeline.calls != 1 || chat.calls != 0 || timelineResponse.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("timeline status/calls/headers=%d/%d/%d/%v", timelineResponse.Code, timeline.calls, chat.calls, timelineResponse.Header())
	}
	var timelineItems []map[string]any
	if err := json.Unmarshal(timelineResponse.Body.Bytes(), &timelineItems); err != nil || len(timelineItems) != 1 || len(timelineItems[0]) != 3 || timelineItems[0]["id"] != float64(7) || timelineItems[0]["event_type"] != "radar_opened" {
		t.Fatalf("timeline payload=%s err=%v", timelineResponse.Body.String(), err)
	}
	chatResponse := call("/api/sidebar/v2/activity/chat", handler.Chat)
	if chatResponse.Code != http.StatusOK || timeline.calls != 1 || chat.calls != 1 {
		t.Fatalf("chat status/calls=%d/%d/%d", chatResponse.Code, timeline.calls, chat.calls)
	}
	var chatItems []map[string]any
	if err := json.Unmarshal(chatResponse.Body.Bytes(), &chatItems); err != nil || len(chatItems) != 1 || len(chatItems[0]) != 3 || chatItems[0]["chat_type"] != "private" || chatItems[0]["message_type"] != "text" {
		t.Fatalf("chat payload=%s err=%v", chatResponse.Body.String(), err)
	}
}

func TestActivityHandlerFailsClosedWhenReadUnavailable(t *testing.T) {
	handler, timeline, _, token, authenticated := sidebarActivityHandler(t)
	timeline.err = errors.New("read failed")
	request := httptest.NewRequest(http.MethodGet, "/api/sidebar/v2/activity/timeline", nil).WithContext(authenticated)
	request.Header.Set("X-Sidebar-Context-Token", token)
	response := httptest.NewRecorder()
	handler.Timeline(response, request)
	if response.Code != http.StatusServiceUnavailable || timeline.calls != 1 || response.Body.String() == "" {
		t.Fatalf("unavailable status/calls/body=%d/%d/%s", response.Code, timeline.calls, response.Body.String())
	}
}

func sidebarActivityHandler(t *testing.T) (*ActivityHandler, *sidebarActivityTimelineFake, *sidebarActivityChatFake, string, context.Context) {
	t.Helper()
	principal := authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}
	profiles := thumbnailProfiles{profile: contactport.SidebarProfile{CustomerID: 41, OwnerStaffID: 7, Name: "customer", UpdatedAt: time.Now().UTC()}}
	contextService, err := sidebarapp.NewService(thumbnailCorp{}, thumbnailIdentity{}, profiles, thumbnailSurveys{}, thumbnailOrders{}, thumbnailMembers{}, &thumbnailMedia{}, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	minted, err := contextService.MintContext(context.Background(), principal, authport.SessionRef("activity-browser-session"), true, "wm_external_41")
	if err != nil || minted.Token == "" {
		t.Fatalf("mint=%+v err=%v", minted, err)
	}
	contextHandler, err := NewHandler(contextService)
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	timeline := &sidebarActivityTimelineFake{read: contactport.Customer360Read{Customer: contactport.Customer360Customer{ID: 41}, Timeline: []contactport.Customer360TimelineEntry{{ID: 7, EventType: "radar_opened", OccurredAt: stamp}}}}
	chat := &sidebarActivityChatFake{page: customer360port.CustomerChatActivityPage{CustomerID: 41, Items: []customer360port.CustomerChatActivityEntry{{ChatType: "private", MessageType: "text", SentAt: stamp}}}}
	activity, err := sidebarapp.NewActivityService(timeline, chat)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewActivityHandler(contextHandler, activity)
	if err != nil {
		t.Fatal(err)
	}
	authenticated := authport.WithAuthenticatedSession(context.Background(), principal, "activity-browser-session")
	authenticated, err = authport.WithAuthorization(authenticated, authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	return handler, timeline, chat, minted.Token, authenticated
}
