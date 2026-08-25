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
	input contactport.Customer360ReadInput
	calls int
}

func (fake *sidebarActivityTimelineFake) ReadCustomer360(_ context.Context, input contactport.Customer360ReadInput) (contactport.Customer360Read, error) {
	fake.calls++
	fake.input = input
	return fake.read, fake.err
}

type sidebarActivityChatFake struct {
	page  customer360port.CustomerChatActivityPage
	err   error
	query customer360port.CustomerChatActivityQuery
	calls int
}

func (fake *sidebarActivityChatFake) ListCustomerChatActivity(_ context.Context, query customer360port.CustomerChatActivityQuery) (customer360port.CustomerChatActivityPage, error) {
	fake.calls++
	fake.query = query
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
		handler.Timeline(response, request, "", 0)
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
	timelineResponse := call("/api/sidebar/v2/activity/timeline", func(writer http.ResponseWriter, request *http.Request) {
		handler.Timeline(writer, request, "timeline-input-cursor", 7)
	})
	if timelineResponse.Code != http.StatusOK || timeline.calls != 1 || chat.calls != 0 || timelineResponse.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("timeline status/calls/headers=%d/%d/%d/%v", timelineResponse.Code, timeline.calls, chat.calls, timelineResponse.Header())
	}
	var timelinePage struct {
		Items      []map[string]any `json:"items"`
		NextCursor string           `json:"next_cursor"`
	}
	if err := json.Unmarshal(timelineResponse.Body.Bytes(), &timelinePage); err != nil || len(timelinePage.Items) != 1 || len(timelinePage.Items[0]) != 3 || timelinePage.Items[0]["id"] != float64(7) || timelinePage.Items[0]["event_type"] != "radar_opened" || timelinePage.NextCursor != "timeline-next-cursor" || timeline.input.TimelineCursor != "timeline-input-cursor" || timeline.input.TimelineLimit != 7 {
		t.Fatalf("timeline payload=%s err=%v", timelineResponse.Body.String(), err)
	}
	chatResponse := call("/api/sidebar/v2/activity/chat", func(writer http.ResponseWriter, request *http.Request) {
		handler.Chat(writer, request, "private", "chat-input-cursor", 8)
	})
	if chatResponse.Code != http.StatusOK || timeline.calls != 1 || chat.calls != 1 {
		t.Fatalf("chat status/calls=%d/%d/%d", chatResponse.Code, timeline.calls, chat.calls)
	}
	var chatPage struct {
		Items          []map[string]any `json:"items"`
		NextCursor     string           `json:"next_cursor"`
		PreviousCursor string           `json:"previous_cursor"`
	}
	if err := json.Unmarshal(chatResponse.Body.Bytes(), &chatPage); err != nil || len(chatPage.Items) != 1 || len(chatPage.Items[0]) != 3 || chatPage.Items[0]["chat_type"] != "private" || chatPage.Items[0]["message_type"] != "text" || chatPage.NextCursor != "chat-next-cursor" || chatPage.PreviousCursor != "chat-previous-cursor" || chat.query.ChatType != "private" || chat.query.Cursor != "chat-input-cursor" || chat.query.Limit != 8 {
		t.Fatalf("chat payload=%s err=%v", chatResponse.Body.String(), err)
	}
}

func TestActivityHandlerFailsClosedWhenReadUnavailable(t *testing.T) {
	handler, timeline, _, token, authenticated := sidebarActivityHandler(t)
	timeline.err = errors.New("read failed")
	request := httptest.NewRequest(http.MethodGet, "/api/sidebar/v2/activity/timeline", nil).WithContext(authenticated)
	request.Header.Set("X-Sidebar-Context-Token", token)
	response := httptest.NewRecorder()
	handler.Timeline(response, request, "", 0)
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
	timelineNext := "timeline-next-cursor"
	chatNext, chatPrevious := "chat-next-cursor", "chat-previous-cursor"
	timeline := &sidebarActivityTimelineFake{read: contactport.Customer360Read{Customer: contactport.Customer360Customer{ID: 41}, Timeline: []contactport.Customer360TimelineEntry{{ID: 7, EventType: "radar_opened", OccurredAt: stamp}}, TimelineNextCursor: &timelineNext}}
	chat := &sidebarActivityChatFake{page: customer360port.CustomerChatActivityPage{CustomerID: 41, ChatType: "private", Items: []customer360port.CustomerChatActivityEntry{{ChatType: "private", MessageType: "text", SentAt: stamp}}, NextCursor: &chatNext, PreviousCursor: &chatPrevious}}
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
