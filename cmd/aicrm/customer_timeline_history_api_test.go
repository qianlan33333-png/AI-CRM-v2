package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

type customerTimelineHistoryAPIStub struct {
	item  contact.CustomerTimelineHistoryRead
	total int64
	err   error
	query contact.CustomerTimelineHistoryQuery
}

func (stub *customerTimelineHistoryAPIStub) GetHistoricalCustomerTimelineEvent(context.Context, int64) (contact.CustomerTimelineHistoryRead, error) {
	return stub.item, stub.err
}

func (stub *customerTimelineHistoryAPIStub) ListHistoricalCustomerTimelineEvents(_ context.Context, query contact.CustomerTimelineHistoryQuery) ([]contact.CustomerTimelineHistoryRead, int64, error) {
	stub.query = query
	if stub.err != nil {
		return nil, 0, stub.err
	}
	return []contact.CustomerTimelineHistoryRead{stub.item}, stub.total, nil
}

func customerTimelineHistoryFixture() *customerTimelineHistoryAPIStub {
	at := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	return &customerTimelineHistoryAPIStub{item: contact.CustomerTimelineHistoryRead{ID: 17, SourceID: 91, EventID: "evt", EventType: "opened", EventTime: at, SourceTable: "orders", SourceValue: "42", CreatedAt: at}, total: 1}
}

func customerTimelineHistoryRouter(reader contact.CustomerTimelineHistoryReader) http.Handler {
	handler := &Handler{customerTimelineHistory: reader}
	router := chi.NewRouter()
	router.Get("/api/admin/customer-timeline-history/events", handler.ListCustomerTimelineHistoryEvents)
	router.Get("/api/admin/customer-timeline-history/events/{history_id}", handler.GetCustomerTimelineHistoryEvent)
	return router
}

func TestCustomerTimelineHistoryAPIUsesSafeReadModel(t *testing.T) {
	stub := customerTimelineHistoryFixture()
	router := customerTimelineHistoryRouter(stub)
	for _, path := range []string{"/api/admin/customer-timeline-history/events?limit=1&offset=0", "/api/admin/customer-timeline-history/events/17"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("path=%s status=%d body=%s", path, response.Code, response.Body.String())
		}
		body := response.Body.String()
		for _, required := range []string{`"source":"v1_history"`, `"read_only":true`, `"real_external_call_executed":false`, `"event_id":"evt"`} {
			if !strings.Contains(body, required) {
				t.Fatalf("path=%s missing=%s body=%s", path, required, body)
			}
		}
		for _, forbidden := range []string{"unionid", "metadata_json", "title", "summary", "digest"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("path=%s leaked=%s body=%s", path, forbidden, body)
			}
		}
	}
	if stub.query != (contact.CustomerTimelineHistoryQuery{Limit: 1, Offset: 0}) {
		t.Fatalf("query=%+v", stub.query)
	}
}

func TestCustomerTimelineHistoryAPIFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name   string
		reader contact.CustomerTimelineHistoryReader
		path   string
		status int
	}{
		{name: "nil reader", path: "/api/admin/customer-timeline-history/events?limit=1", status: http.StatusServiceUnavailable},
		{name: "bad page", reader: customerTimelineHistoryFixture(), path: "/api/admin/customer-timeline-history/events?limit=0", status: http.StatusBadRequest},
		{name: "bad id", reader: customerTimelineHistoryFixture(), path: "/api/admin/customer-timeline-history/events/0", status: http.StatusBadRequest},
		{name: "backend", reader: &customerTimelineHistoryAPIStub{err: errors.New("down")}, path: "/api/admin/customer-timeline-history/events?limit=1", status: http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			customerTimelineHistoryRouter(test.reader).ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != test.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
