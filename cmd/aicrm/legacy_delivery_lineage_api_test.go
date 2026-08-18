package main

import (
	"context"
	"encoding/json"
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
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

func TestLegacyDeliveryLineageGETContractAndRedaction(t *testing.T) {
	stamp := time.Date(2026, 8, 19, 3, 4, 5, 6, time.FixedZone("CST", 8*3600))
	outbound := &legacyDeliveryLineageOutboundStub{page: outboundport.DeliveryLineagePage{Complete: true, Items: []outboundport.DeliveryLineageItem{{LineageID: "outbound-task:42", InternalState: "outcome_unknown", AttemptCount: 2, UpdatedAt: stamp}}}}
	events := &legacyDeliveryLineageEventStub{page: eventport.DeliveryLineagePage{Complete: true, Items: []eventport.DeliveryLineageItem{{LineageID: "event-delivery:v1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", InternalState: "completed", AttemptCount: 1, UpdatedAt: stamp}}}}
	response := httptest.NewRecorder()
	deliveryLineageRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, outbound, events).ServeHTTP(response, legacyRequest(http.MethodGet, legacyDeliveryLineagePath, legacyToken(1)))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 6 || body["ok"] != true || body["limit"] != float64(50) || body["offset"] != float64(0) || body["has_more"] != false {
		t.Fatalf("body=%#v", body)
	}
	interpretation := body["interpretation"].(map[string]any)
	if interpretation["kind"] != "internal_processing_only" || interpretation["external_delivery"] != "unknown" || interpretation["external_receipt"] != "unknown" {
		t.Fatalf("interpretation=%#v", interpretation)
	}
	items := body["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["record_kind"] != "event_delivery" || items[1].(map[string]any)["record_kind"] != "outbound_task" {
		t.Fatalf("items=%#v", items)
	}
	for _, raw := range items {
		item := raw.(map[string]any)
		if len(item) != 7 || item["external_delivery"] != "unknown" || item["external_receipt"] != "unknown" || item["updated_at"] != stamp.UTC().Format(time.RFC3339Nano) {
			t.Fatalf("item=%#v", item)
		}
	}
	assertNoDeliveryLineageSensitiveFields(t, body)
	if outbound.calls != 1 || events.calls != 1 || outbound.limit != 51 || events.limit != 51 {
		t.Fatalf("calls/limits outbound=%d/%d events=%d/%d", outbound.calls, outbound.limit, events.calls, events.limit)
	}
}

func TestLegacyDeliveryLineageEmptyAndPagination(t *testing.T) {
	for _, test := range []struct {
		name       string
		path       string
		outbound   outboundport.DeliveryLineagePage
		events     eventport.DeliveryLineagePage
		wantLimit  int32
		wantMore   bool
		wantItems  int
		wantOffset int64
	}{
		{"empty", legacyDeliveryLineagePath, outboundport.DeliveryLineagePage{Complete: true}, eventport.DeliveryLineagePage{Complete: true}, 51, false, 0, 0},
		{"has more", legacyDeliveryLineagePath + "?limit=1", outboundport.DeliveryLineagePage{Complete: true, Items: []outboundport.DeliveryLineageItem{
			{LineageID: "outbound-task:2", InternalState: "sent", UpdatedAt: time.Date(2026, 8, 19, 3, 4, 7, 0, time.UTC)},
			{LineageID: "outbound-task:1", InternalState: "pending", UpdatedAt: time.Date(2026, 8, 19, 3, 4, 6, 0, time.UTC)},
		}}, eventport.DeliveryLineagePage{Complete: true}, 2, true, 1, 0},
		{"window", legacyDeliveryLineagePath + "?limit=1&offset=1", outboundport.DeliveryLineagePage{Complete: true, Items: []outboundport.DeliveryLineageItem{
			{LineageID: "outbound-task:2", InternalState: "sent", UpdatedAt: time.Date(2026, 8, 19, 3, 4, 7, 0, time.UTC)},
			{LineageID: "outbound-task:1", InternalState: "pending", UpdatedAt: time.Date(2026, 8, 19, 3, 4, 6, 0, time.UTC)},
		}}, eventport.DeliveryLineagePage{Complete: true}, 3, false, 1, 1},
		{"offset equals merged length", legacyDeliveryLineagePath + "?limit=1&offset=1", outboundport.DeliveryLineagePage{Complete: true, Items: []outboundport.DeliveryLineageItem{
			{LineageID: "outbound-task:1", InternalState: "pending", UpdatedAt: time.Date(2026, 8, 19, 3, 4, 6, 0, time.UTC)},
		}}, eventport.DeliveryLineagePage{Complete: true}, 3, false, 0, 1},
		{"offset exceeds merged length", legacyDeliveryLineagePath + "?limit=1&offset=2", outboundport.DeliveryLineagePage{Complete: true, Items: []outboundport.DeliveryLineageItem{
			{LineageID: "outbound-task:1", InternalState: "pending", UpdatedAt: time.Date(2026, 8, 19, 3, 4, 6, 0, time.UTC)},
		}}, eventport.DeliveryLineagePage{Complete: true}, 4, false, 0, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			outbound := &legacyDeliveryLineageOutboundStub{page: test.outbound}
			events := &legacyDeliveryLineageEventStub{page: test.events}
			response := httptest.NewRecorder()
			deliveryLineageRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, outbound, events).ServeHTTP(response, legacyRequest(http.MethodGet, test.path, legacyToken(2)))
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var body legacyDeliveryLineageSuccess
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if len(body.Items) != test.wantItems || body.Offset != test.wantOffset || body.HasMore != test.wantMore || outbound.limit != test.wantLimit || events.limit != test.wantLimit {
				t.Fatalf("body=%+v limits=%d/%d", body, outbound.limit, events.limit)
			}
		})
	}
}

func TestLegacyDeliveryLineageRejectsUnauthorizedMethodsAndQueriesBeforeRead(t *testing.T) {
	unauthenticatedOutbound, unauthenticatedEvents := &legacyDeliveryLineageOutboundStub{}, &legacyDeliveryLineageEventStub{}
	unauthenticated := httptest.NewRecorder()
	deliveryLineageRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, unauthenticatedOutbound, unauthenticatedEvents).ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, legacyDeliveryLineagePath, nil))
	if unauthenticated.Code != http.StatusUnauthorized || unauthenticatedOutbound.calls != 0 || unauthenticatedEvents.calls != 0 {
		t.Fatalf("unauthenticated status/calls=%d/%d/%d", unauthenticated.Code, unauthenticatedOutbound.calls, unauthenticatedEvents.calls)
	}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			outbound, events := &legacyDeliveryLineageOutboundStub{}, &legacyDeliveryLineageEventStub{}
			response := httptest.NewRecorder()
			deliveryLineageRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, outbound, events).ServeHTTP(response, httptest.NewRequest(method, legacyDeliveryLineagePath, nil))
			if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet || outbound.calls != 0 || events.calls != 0 {
				t.Fatalf("status/allow/calls=%d/%q/%d/%d", response.Code, response.Header().Get("Allow"), outbound.calls, events.calls)
			}
		})
	}
	for _, path := range []string{
		legacyDeliveryLineagePath + "?limit=0", legacyDeliveryLineagePath + "?limit=101", legacyDeliveryLineagePath + "?offset=1000001",
		legacyDeliveryLineagePath + "?limit=1&limit=2", legacyDeliveryLineagePath + "?offset=-1", legacyDeliveryLineagePath + "?limit=+1",
		legacyDeliveryLineagePath + "?limit=", legacyDeliveryLineagePath + "?unknown=1", legacyDeliveryLineagePath + "?limit=%ZZ",
	} {
		t.Run(path, func(t *testing.T) {
			outbound, events := &legacyDeliveryLineageOutboundStub{}, &legacyDeliveryLineageEventStub{}
			response := httptest.NewRecorder()
			deliveryLineageRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, outbound, events).ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(3)))
			if response.Code != http.StatusUnprocessableEntity || outbound.calls != 0 || events.calls != 0 {
				t.Fatalf("status/calls=%d/%d/%d body=%s", response.Code, outbound.calls, events.calls, response.Body.String())
			}
		})
	}
	for _, principal := range []authport.Principal{{AdminUserID: 9, Role: authport.RoleOps}, {AdminUserID: 9, Role: authport.RoleSales}} {
		outbound, events := &legacyDeliveryLineageOutboundStub{}, &legacyDeliveryLineageEventStub{}
		response := httptest.NewRecorder()
		deliveryLineageRouter(t, principal, outbound, events).ServeHTTP(response, legacyRequest(http.MethodGet, legacyDeliveryLineagePath, legacyToken(4)))
		if response.Code != http.StatusForbidden || outbound.calls != 0 || events.calls != 0 {
			t.Fatalf("principal=%+v status/calls=%d/%d/%d", principal, response.Code, outbound.calls, events.calls)
		}
	}
}

func TestLegacyDeliveryLineageFailsClosedWithoutPartialResponse(t *testing.T) {
	stamp := time.Date(2026, 8, 19, 3, 4, 5, 0, time.UTC)
	for _, test := range []struct {
		name     string
		outbound *legacyDeliveryLineageOutboundStub
		events   *legacyDeliveryLineageEventStub
	}{
		{"outbound failure", &legacyDeliveryLineageOutboundStub{err: context.DeadlineExceeded}, &legacyDeliveryLineageEventStub{}},
		{"events failure", &legacyDeliveryLineageOutboundStub{page: outboundport.DeliveryLineagePage{Complete: true}}, &legacyDeliveryLineageEventStub{err: errors.New("store failure")}},
		{"incomplete source", &legacyDeliveryLineageOutboundStub{page: outboundport.DeliveryLineagePage{Complete: false}}, &legacyDeliveryLineageEventStub{}},
		{"invalid state", &legacyDeliveryLineageOutboundStub{page: outboundport.DeliveryLineagePage{Complete: true, Items: []outboundport.DeliveryLineageItem{{LineageID: "outbound-task:9", InternalState: "provider_sent", UpdatedAt: stamp}}}}, &legacyDeliveryLineageEventStub{page: eventport.DeliveryLineagePage{Complete: true}}},
		{"noncanonical outbound id", &legacyDeliveryLineageOutboundStub{page: outboundport.DeliveryLineagePage{Complete: true, Items: []outboundport.DeliveryLineageItem{{LineageID: "outbound-task:01", InternalState: "pending", UpdatedAt: stamp}}}}, &legacyDeliveryLineageEventStub{page: eventport.DeliveryLineagePage{Complete: true}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			deliveryLineageRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, test.outbound, test.events).ServeHTTP(response, legacyRequest(http.MethodGet, legacyDeliveryLineagePath, legacyToken(5)))
			if response.Code != http.StatusServiceUnavailable || response.Body.String() != "{\"error_code\":\"delivery_lineage_unavailable\",\"ok\":false,\"status_code\":503}\n" {
				t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
			}
		})
	}
}

func deliveryLineageRouter(t *testing.T, principal authport.Principal, outbound outboundport.DeliveryLineageReader, events eventport.DeliveryLineageReader) http.Handler {
	t.Helper()
	service := &legacyAuthStub{principal: principal}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := NewHandler(service, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.deliveryLineage = legacyDeliveryLineageReaders{outbound: outbound, events: events}
	router, err := newAPIHandlerWithAll(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy, nil)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func assertNoDeliveryLineageSensitiveFields(t *testing.T, value any) {
	t.Helper()
	forbidden := map[string]bool{
		"customer": true, "owner": true, "batch": true, "chunk": true, "recipient": true, "external_id": true,
		"unionid": true, "openid": true, "phone": true, "name": true, "payload": true, "event_type": true,
		"idempotency": true, "trace": true, "source": true, "command": true, "consumer": true, "river": true,
		"provider": true, "error": true, "receipt": true, "db": true,
	}
	var walk func(any)
	walk = func(node any) {
		switch current := node.(type) {
		case map[string]any:
			for key, child := range current {
				if forbidden[strings.ToLower(key)] {
					t.Fatalf("forbidden key %q in %#v", key, current)
				}
				walk(child)
			}
		case []any:
			for _, child := range current {
				walk(child)
			}
		}
	}
	walk(value)
}

type legacyDeliveryLineageOutboundStub struct {
	page  outboundport.DeliveryLineagePage
	err   error
	calls int
	limit int32
}

func (stub *legacyDeliveryLineageOutboundStub) ListDeliveryLineage(_ context.Context, limit int32) (outboundport.DeliveryLineagePage, error) {
	stub.calls++
	stub.limit = limit
	return stub.page, stub.err
}

type legacyDeliveryLineageEventStub struct {
	page  eventport.DeliveryLineagePage
	err   error
	calls int
	limit int32
}

func (stub *legacyDeliveryLineageEventStub) ListDeliveryLineage(_ context.Context, limit int32) (eventport.DeliveryLineagePage, error) {
	stub.calls++
	stub.limit = limit
	return stub.page, stub.err
}
