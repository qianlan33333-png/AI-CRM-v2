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
	sidebarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/app"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

func TestOtherStaffChatHandlerReturnsOnlyMaskedLocalFields(t *testing.T) {
	handler, reader, token, authenticated := sidebarOtherStaffChatHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/api/sidebar/v2/other-staff-chats", nil).WithContext(authenticated)
	request.Header.Set("X-Sidebar-Context-Token", token)
	response := httptest.NewRecorder()
	handler.List(response, request)
	if response.Code != http.StatusOK || reader.calls != 1 || response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("status/calls/headers=%d/%d/%v", response.Code, reader.calls, response.Header())
	}
	var payload struct {
		Items  []map[string]any  `json:"items"`
		Safety sidebarapp.Safety `json:"safety"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || len(payload.Items) != 1 || len(payload.Items[0]) != 4 ||
		payload.Items[0]["staff_userid"] != "staff-other" || payload.Items[0]["message_type"] != "text" || payload.Items[0]["content_masked"] != "已脱敏内容" ||
		payload.Safety != (sidebarapp.Safety{LocalOnly: true}) {
		t.Fatalf("payload=%s err=%v", response.Body.String(), err)
	}
	if reader.query != (wecomport.CustomerOtherStaffChatQuery{CustomerID: contactport.CustomerID(41), OwnerStaffID: 7}) {
		t.Fatalf("query=%#v", reader.query)
	}
}

func TestOtherStaffChatHandlerFailsClosedWithoutVerifiedLocalRead(t *testing.T) {
	handler, reader, token, authenticated := sidebarOtherStaffChatHandler(t)
	reader.err = errors.New("archive unavailable")
	request := httptest.NewRequest(http.MethodGet, "/api/sidebar/v2/other-staff-chats", nil).WithContext(authenticated)
	request.Header.Set("X-Sidebar-Context-Token", token)
	response := httptest.NewRecorder()
	handler.List(response, request)
	if response.Code != http.StatusServiceUnavailable || reader.calls != 1 || response.Body.Len() == 0 {
		t.Fatalf("status/calls/body=%d/%d/%q", response.Code, reader.calls, response.Body.String())
	}
}

func sidebarOtherStaffChatHandler(t *testing.T) (*OtherStaffChatHandler, *otherStaffChatHTTPFake, string, context.Context) {
	t.Helper()
	principal := thumbnailViewerPrincipal()
	profiles := thumbnailProfiles{profile: contactport.SidebarProfile{CustomerID: 41, OwnerStaffID: 7, Name: "customer", UpdatedAt: time.Now().UTC()}}
	contextService, err := sidebarapp.NewService(thumbnailCorp{}, thumbnailIdentity{}, thumbnailPhones{}, profiles, thumbnailSurveys{}, thumbnailOrders{}, thumbnailMembers{}, &thumbnailMedia{}, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	minted, err := contextService.MintContext(context.Background(), principal, authport.SessionRef("other-staff-browser-session"), true, "wm_external_41")
	if err != nil || minted.Token == "" {
		t.Fatalf("mint=%+v err=%v", minted, err)
	}
	contextHandler, err := NewHandler(contextService)
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	reader := &otherStaffChatHTTPFake{page: wecomport.CustomerOtherStaffChatPage{Items: []wecomport.CustomerOtherStaffChat{{
		StaffUserID: "staff-other", MessageType: "text", ContentMasked: "已脱敏内容", SentAt: stamp,
	}}}}
	service, err := sidebarapp.NewOtherStaffChatService(reader)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewOtherStaffChatHandler(contextHandler, service)
	if err != nil {
		t.Fatal(err)
	}
	authenticated := authport.WithAuthenticatedSession(context.Background(), principal, "other-staff-browser-session")
	authenticated, err = authport.WithAuthorization(authenticated, authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	return handler, reader, minted.Token, authenticated
}

type otherStaffChatHTTPFake struct {
	page  wecomport.CustomerOtherStaffChatPage
	err   error
	query wecomport.CustomerOtherStaffChatQuery
	calls int
}

func (fake *otherStaffChatHTTPFake) ListCustomerOtherStaffChats(_ context.Context, query wecomport.CustomerOtherStaffChatQuery) (wecomport.CustomerOtherStaffChatPage, error) {
	fake.calls++
	fake.query = query
	return fake.page, fake.err
}

var _ wecomport.CustomerOtherStaffChatReader = (*otherStaffChatHTTPFake)(nil)
