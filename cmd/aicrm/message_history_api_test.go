package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

type messageHistoryAPIReader struct {
	err          error
	empty        bool
	inconsistent bool
	invalid      bool
	calls        int
	query        wecomport.MessageHistoryQuery
}

func (reader *messageHistoryAPIReader) ListHistoricalMessages(_ context.Context, query wecomport.MessageHistoryQuery) ([]wecomport.HistoricalMessage, int64, error) {
	reader.calls++
	reader.query = query
	if reader.empty {
		return nil, 0, reader.err
	}
	if reader.inconsistent {
		return []wecomport.HistoricalMessage{messageHistoryAPIValue(11)}, 2, reader.err
	}
	item := messageHistoryAPIValue(11)
	if query.CustomerID != nil {
		customerID := *query.CustomerID
		item.CustomerID = &customerID
	}
	if query.ChatType != "" {
		item.ChatType = query.ChatType
	}
	if reader.invalid {
		item.SourcePayloadDigest = [32]byte{}
	}
	return []wecomport.HistoricalMessage{item}, 1, reader.err
}

func (reader *messageHistoryAPIReader) GetHistoricalMessage(_ context.Context, id int64) (wecomport.HistoricalMessage, error) {
	reader.calls++
	item := messageHistoryAPIValue(id)
	if reader.invalid {
		item.SourcePayloadDigest = [32]byte{}
	}
	return item, reader.err
}

func messageHistoryAPIValue(id int64) wecomport.HistoricalMessage {
	sequence := int64(-7)
	return wecomport.HistoricalMessage{
		ID: id, SourceID: 101, Sequence: &sequence, CustomerID: nil, ChatType: "private", MessageType: "text", ContentMasked: nil,
		OriginalSendTime: "2026-08-27 13:36:01", SendTimeBasis: "civil_unzoned", SentAt: nil,
		CreatedAt: time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC), SourcePayloadDigest: messageHistoryAPIDigest(3),
	}
}

func messageHistoryAPIDigest(seed byte) [32]byte {
	var digest [32]byte
	for index := range digest {
		digest[index] = seed
	}
	return digest
}

type messageHistoryAPIAuth struct {
	role         authport.Role
	csrfCalls    int
	capabilities []authport.Capability
}

func (service *messageHistoryAPIAuth) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	return authport.Principal{AdminUserID: 1, Role: service.role}, nil
}
func (service *messageHistoryAPIAuth) Authorize(_ context.Context, principal authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	if principal.Role != authport.RoleAdmin || capability != authport.CapabilityAdminRead {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	service.capabilities = append(service.capabilities, capability)
	return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
}
func (service *messageHistoryAPIAuth) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	service.csrfCalls++
	return nil
}
func (*messageHistoryAPIAuth) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func messageHistoryRouter(t *testing.T, reader wecomport.MessageHistoryReader, auth authport.Service) http.Handler {
	t.Helper()
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.messageHistory = reader
	authHandler, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.NotFoundHandler(), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func TestFinalRouterMessageHistoryAdminReadWithoutCSRF(t *testing.T) {
	reader := &messageHistoryAPIReader{}
	auth := &messageHistoryAPIAuth{role: authport.RoleAdmin}
	router := messageHistoryRouter(t, reader, auth)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/message-history?customer_id=7&chat_type=private&limit=1&offset=0", legacyToken(0xa1)))
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(response.Body.String(), `"source":"v1_history"`) || !strings.Contains(response.Body.String(), `"read_only":true`) || !strings.Contains(response.Body.String(), `"real_external_call_executed":false`) || !strings.Contains(response.Body.String(), `"customer_id":7`) || !strings.Contains(response.Body.String(), `"content_masked":null`) || !strings.Contains(response.Body.String(), `"original_send_time":"2026-08-27 13:36:01"`) {
		t.Fatalf("list response=%d %s", response.Code, response.Body.String())
	}
	if reader.query.CustomerID == nil || *reader.query.CustomerID != 7 || reader.query.ChatType != "private" || reader.query.Limit != 1 || reader.query.Offset != 0 {
		t.Fatalf("reader query=%+v", reader.query)
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/message-history/12", legacyToken(0xa2)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":12`) || !strings.Contains(response.Body.String(), `"source_id":101`) || !strings.Contains(response.Body.String(), `"source_payload_digest":[3,3`) {
		t.Fatalf("detail response=%d %s", response.Code, response.Body.String())
	}
	if auth.csrfCalls != 0 || len(auth.capabilities) != 2 {
		t.Fatalf("csrf/capabilities=%d/%v", auth.csrfCalls, auth.capabilities)
	}
	for _, capability := range auth.capabilities {
		if capability != authport.CapabilityAdminRead {
			t.Fatalf("capability=%s", capability)
		}
	}

	currentCookie := httptest.NewRequest(http.MethodGet, "/api/admin/message-history", nil)
	currentCookie.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: legacyToken(0xa3)})
	response = httptest.NewRecorder()
	router.ServeHTTP(response, currentCookie)
	if response.Code != http.StatusOK || auth.csrfCalls != 0 {
		t.Fatalf("cookie read=%d csrf=%d", response.Code, auth.csrfCalls)
	}
}

func TestFinalRouterMessageHistoryRejectsInvalidAndUnauthorizedReads(t *testing.T) {
	for _, test := range []struct {
		name    string
		role    authport.Role
		request func(string) *http.Request
		want    int
	}{
		{"anonymous", "", func(path string) *http.Request { return httptest.NewRequest(http.MethodGet, path, nil) }, http.StatusUnauthorized},
		{"ops", authport.RoleOps, func(path string) *http.Request { return legacyRequest(http.MethodGet, path, legacyToken(0xa4)) }, http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := &messageHistoryAPIReader{}
			router := messageHistoryRouter(t, reader, &messageHistoryAPIAuth{role: test.role})
			for _, path := range []string{"/api/admin/message-history", "/api/admin/message-history/12"} {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, test.request(path))
				if response.Code != test.want {
					t.Fatalf("%s %s status=%d", test.name, path, response.Code)
				}
			}
			if reader.calls != 0 {
				t.Fatalf("denied request reached reader=%d", reader.calls)
			}
		})
	}

	reader := &messageHistoryAPIReader{}
	router := messageHistoryRouter(t, reader, &messageHistoryAPIAuth{role: authport.RoleAdmin})
	for _, query := range []string{"customer_id=0", "customer_id=01", "customer_id=1&customer_id=2", "chat_type=unknown", "chat_type=private&chat_type=group", "limit=0", "limit=101", "limit=01", "limit=1&limit=2", "offset=-1", "offset=01", "offset=1&offset=2", "unknown=true", "limit=%zz", "limit=1;offset=2"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/message-history?"+query, legacyToken(0xa5)))
		if response.Code != http.StatusBadRequest || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("query=%s status=%d %s", query, response.Code, response.Body.String())
		}
	}
	for _, id := range []string{"0", "01", "-1", "x", "9223372036854775808"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/message-history/"+id, legacyToken(0xa6)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("id=%s status=%d", id, response.Code)
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/message-history/12?limit=1", legacyToken(0xa7)))
	if response.Code != http.StatusBadRequest || reader.calls != 0 {
		t.Fatalf("detail query/reader=%d/%d", response.Code, reader.calls)
	}
	for _, path := range []string{"/api/admin/message-history", "/api/admin/message-history/12"} {
		response = httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodPost, path, legacyToken(0xa8)))
		if response.Code != http.StatusBadRequest && response.Code != http.StatusMethodNotAllowed && response.Code != http.StatusNotFound {
			t.Fatalf("POST %s status=%d", path, response.Code)
		}
	}
	if reader.calls != 0 {
		t.Fatalf("non-GET reached reader=%d", reader.calls)
	}
}

func TestFinalRouterMessageHistoryFailsClosedForReaderProblems(t *testing.T) {
	reader := &messageHistoryAPIReader{empty: true}
	router := messageHistoryRouter(t, reader, &messageHistoryAPIAuth{role: authport.RoleAdmin})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/message-history", legacyToken(0xa9)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"items":[]`) || !strings.Contains(response.Body.String(), `"total":0`) || reader.query.Limit != 50 || reader.query.Offset != 0 {
		t.Fatalf("empty response=%d %s", response.Code, response.Body.String())
	}
	for _, problem := range []string{"error", "inconsistent", "invalid"} {
		reader.empty, reader.err, reader.inconsistent, reader.invalid = false, nil, problem == "inconsistent", problem == "invalid"
		if problem == "error" {
			reader.err = errors.New("private database and identity detail")
		}
		response = httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/message-history", legacyToken(0xaa)))
		if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "private database") || strings.Contains(response.Body.String(), `"items"`) || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s list response=%d %s", problem, response.Code, response.Body.String())
		}
		if problem != "inconsistent" {
			response = httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/message-history/12", legacyToken(0xab)))
			if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "private database") {
				t.Fatalf("%s detail response=%d %s", problem, response.Code, response.Body.String())
			}
		}
	}
	var typedNil *messageHistoryAPIReader
	router = messageHistoryRouter(t, typedNil, &messageHistoryAPIAuth{role: authport.RoleAdmin})
	response = httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/message-history", legacyToken(0xac)))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("typed nil status=%d", response.Code)
	}
}

var _ wecomport.MessageHistoryReader = (*messageHistoryAPIReader)(nil)
var _ authport.Service = (*messageHistoryAPIAuth)(nil)
