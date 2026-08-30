package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	customer360port "github.com/qianlan33333-png/AI-CRM-v2/internal/customer360/port"
	sidebarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/app"
	sidebarhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/http"
)

type sidebarActivityAPITimelineFake struct {
	read  contactport.Customer360Read
	calls int
}

func (fake *sidebarActivityAPITimelineFake) ReadCustomer360(context.Context, contactport.Customer360ReadInput) (contactport.Customer360Read, error) {
	fake.calls++
	return fake.read, nil
}

type sidebarActivityAPIChatFake struct {
	page  customer360port.CustomerChatActivityPage
	calls int
}

func (fake *sidebarActivityAPIChatFake) ListCustomerChatActivity(context.Context, customer360port.CustomerChatActivityQuery) (customer360port.CustomerChatActivityPage, error) {
	fake.calls++
	return fake.page, nil
}

func TestSidebarActivityCandidateAdapterUsesBoundContextAndReturnsSafeFlags(t *testing.T) {
	owner := int64(7)
	profiles := &sidebarRouteProfiles{profile: contactport.SidebarProfile{CustomerID: 41, OwnerStaffID: owner, Name: "customer", UpdatedAt: time.Now().UTC()}}
	sidebarService, err := sidebarapp.NewService(sidebarRouteCorp{}, &sidebarRouteIdentity{status: "found"}, sidebarRoutePhones{}, profiles, sidebarRouteSurveys{}, sidebarRouteOrders{}, sidebarRouteMembers{}, sidebarRouteMedia{}, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	principal := authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin, StaffID: &owner}
	minted, err := sidebarService.MintContext(context.Background(), principal, "sidebar-activity-api-session", true, "wm_external_41")
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	timeline := &sidebarActivityAPITimelineFake{read: contactport.Customer360Read{
		Customer: contactport.Customer360Customer{ID: 41},
		Timeline: []contactport.Customer360TimelineEntry{{ID: 1, EventType: "radar_opened", OccurredAt: stamp}},
	}}
	chat := &sidebarActivityAPIChatFake{page: customer360port.CustomerChatActivityPage{
		CustomerID: 41, ChatType: "private",
		Items: []customer360port.CustomerChatActivityEntry{{ChatType: "private", MessageType: "text", SentAt: stamp}},
	}}
	activityService, err := sidebarapp.NewActivityService(timeline, chat)
	if err != nil {
		t.Fatal(err)
	}
	contextHandler, err := sidebarhttp.NewHandler(sidebarService)
	if err != nil {
		t.Fatal(err)
	}
	activityHandler, err := sidebarhttp.NewActivityHandler(contextHandler, activityService)
	if err != nil {
		t.Fatal(err)
	}
	candidate := &candidateHandler{sidebarActivity: activityHandler}
	authService := &sidebarRouteAuth{
		principal:     principal,
		authorization: authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
	}
	authHandler, err := authhttp.NewHandler(authService)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), authHandler, candidate)
	if err != nil {
		t.Fatal(err)
	}

	timelineRequest := httptest.NewRequest(http.MethodGet, "/api/sidebar/v2/timeline?external_userid=untrusted&limit=1", nil)
	timelineRequest.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: "sidebar-activity-api-session"})
	timelineRequest.Header.Set("X-Sidebar-Context-Token", minted.Token)
	timelineResponse := httptest.NewRecorder()
	router.ServeHTTP(timelineResponse, timelineRequest)
	if timelineResponse.Code != http.StatusOK || timeline.calls != 1 || chat.calls != 0 {
		t.Fatalf("timeline status/calls=%d/%d/%d", timelineResponse.Code, timeline.calls, chat.calls)
	}
	assertSidebarActivitySafeResponse(t, timelineResponse, []string{"id", "event_type", "occurred_at"})

	chatRequest := httptest.NewRequest(http.MethodGet, "/api/sidebar/v2/chat-activity?customer_id=999&chat_type=private&limit=1", nil)
	chatRequest.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: "sidebar-activity-api-session"})
	chatRequest.Header.Set("X-Sidebar-Context-Token", minted.Token)
	chatResponse := httptest.NewRecorder()
	router.ServeHTTP(chatResponse, chatRequest)
	if chatResponse.Code != http.StatusOK || timeline.calls != 1 || chat.calls != 1 {
		t.Fatalf("chat status/calls=%d/%d/%d", chatResponse.Code, timeline.calls, chat.calls)
	}
	assertSidebarActivitySafeResponse(t, chatResponse, []string{"chat_type", "message_type", "sent_at"})

	zeroLimit := int32(0)
	zeroTimelineResponse := httptest.NewRecorder()
	candidate.ListSidebarTimeline(zeroTimelineResponse, httptest.NewRequest(http.MethodGet, "/api/sidebar/v2/timeline?limit=0", nil), api.ListSidebarTimelineParams{Limit: &zeroLimit})
	if zeroTimelineResponse.Code != http.StatusBadRequest || timeline.calls != 1 || chat.calls != 1 {
		t.Fatalf("zero timeline limit status/calls=%d/%d/%d", zeroTimelineResponse.Code, timeline.calls, chat.calls)
	}
	zeroChatResponse := httptest.NewRecorder()
	candidate.ListSidebarChatActivity(zeroChatResponse, httptest.NewRequest(http.MethodGet, "/api/sidebar/v2/chat-activity?limit=0", nil), api.ListSidebarChatActivityParams{Limit: &zeroLimit})
	if zeroChatResponse.Code != http.StatusBadRequest || timeline.calls != 1 || chat.calls != 1 {
		t.Fatalf("zero chat limit status/calls=%d/%d/%d", zeroChatResponse.Code, timeline.calls, chat.calls)
	}

	wrongMethod := httptest.NewRecorder()
	router.ServeHTTP(wrongMethod, httptest.NewRequest(http.MethodPost, "/api/sidebar/v2/timeline", nil))
	if wrongMethod.Code != http.StatusMethodNotAllowed || timeline.calls != 1 || chat.calls != 1 {
		t.Fatalf("wrong method status/calls=%d/%d/%d headers=%v body=%s", wrongMethod.Code, timeline.calls, chat.calls, wrongMethod.Header(), wrongMethod.Body.String())
	}
}

func TestSidebarActivityCandidateAdapterFailsClosedWithoutDependency(t *testing.T) {
	response := httptest.NewRecorder()
	(&candidateHandler{}).ListSidebarTimeline(response, httptest.NewRequest(http.MethodGet, "/api/sidebar/v2/timeline", nil), api.ListSidebarTimelineParams{})
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func assertSidebarActivitySafeResponse(t *testing.T, response *httptest.ResponseRecorder, allowed []string) {
	t.Helper()
	var body struct {
		Items  []map[string]any `json:"items"`
		Safety struct {
			LocalOnly                 bool `json:"local_only"`
			ProviderExecutionEligible bool `json:"provider_execution_eligible"`
			RealExternalCallExecuted  bool `json:"real_external_call_executed"`
		} `json:"safety"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Items) != 1 || !body.Safety.LocalOnly || body.Safety.ProviderExecutionEligible || body.Safety.RealExternalCallExecuted {
		t.Fatalf("body=%s err=%v", response.Body.String(), err)
	}
	if len(body.Items[0]) != len(allowed) {
		t.Fatalf("unsafe fields=%v", body.Items[0])
	}
	for _, field := range allowed {
		if _, ok := body.Items[0][field]; !ok {
			t.Fatalf("missing safe field %q in %v", field, body.Items[0])
		}
	}
}
