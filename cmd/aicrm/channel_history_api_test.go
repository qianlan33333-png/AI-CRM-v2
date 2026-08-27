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

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

type channelHistoryStub struct {
	err           error
	id            int64
	limit, offset int32
}

func (s *channelHistoryStub) ListHistoricalChannelContacts(_ context.Context, id int64, limit, offset int32) ([]contactport.HistoricalChannelContact, int64, error) {
	s.id, s.limit, s.offset = id, limit, offset
	return []contactport.HistoricalChannelContact{{ID: 5, ChannelID: id, SourceContactID: 90, EnterCount: 2}}, 1, s.err
}
func (s *channelHistoryStub) ListHistoricalChannelAssignees(_ context.Context, id int64) ([]contactport.HistoricalChannelAssignee, error) {
	return []contactport.HistoricalChannelAssignee{{ID: 6, ChannelID: id, SourceAssigneeID: 91, StaffReference: "historical-staff", SourceCreatedAt: "2025-01-02T03:04:05.000000", SourceUpdatedAt: "2025-01-02T03:04:05.000000"}}, s.err
}

func TestChannelHistoryUsesAuthenticatedReadOnlyRoute(t *testing.T) {
	service := &recordingAuth{}
	legacy, err := NewHandlerWithOutboundProductsMediaAndSurvey(service, &legacyCustomerStub{result: legacyCustomerResult()}, &legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, &legacyProductStub{}, &legacyMediaStub{}, &legacySurveyStub{})
	if err != nil {
		t.Fatal(err)
	}
	channels := &legacyChannelStub{item: legacyChannelItem()}
	history := &channelHistoryStub{}
	legacy.channels, legacy.channelHistory = channels, history
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/channels/1/history?limit=20&offset=2", legacyToken(90)))
	if response.Code != 200 || history.id != 1 || history.limit != 20 || history.offset != 2 || channels.writes != 0 {
		t.Fatalf("status=%d reader=%+v writes=%d", response.Code, history, channels.writes)
	}
	for _, want := range []string{`"source":"v1_history"`, `"read_only":true`, `"real_external_call_executed":false`, `"customer_id":null`, `"source_created_at":"2025-01-02T03:04:05.000000"`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("missing %s", want)
		}
	}
	if response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("missing no-store")
	}
	if seen := service.capabilities(); len(seen) != 1 || seen[0] != authport.CapabilityCustomersRead {
		t.Fatalf("capabilities=%v", seen)
	}
	for _, path := range []string{"/api/admin/channels/0/history", "/api/admin/channels/1/history?limit=101", "/api/admin/channels/1/history?offset=-1", "/api/admin/channels/1/history?limit=1&limit=2", "/api/admin/channels/1/history?current=true", "/api/admin/channels/1/history?limit=%xx"} {
		r := httptest.NewRecorder()
		router.ServeHTTP(r, legacyRequest(http.MethodGet, path, legacyToken(91)))
		if r.Code != 400 {
			t.Fatalf("path=%s status=%d", path, r.Code)
		}
	}
	history.err = errors.New("private database detail")
	failed := httptest.NewRecorder()
	router.ServeHTTP(failed, legacyRequest(http.MethodGet, "/api/admin/channels/1/history", legacyToken(92)))
	if failed.Code != 503 || strings.Contains(failed.Body.String(), "private database detail") {
		t.Fatalf("status=%d", failed.Code)
	}
	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/admin/channels/1/history", nil))
	if unauthenticated.Code != 401 {
		t.Fatalf("unauthenticated status=%d", unauthenticated.Code)
	}
}
