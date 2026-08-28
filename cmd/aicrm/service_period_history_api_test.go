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
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

type servicePeriodHistoryAPIStub struct {
	err           error
	definitionID  int64
	limit, offset int32
}

func (s *servicePeriodHistoryAPIStub) ListServicePeriodHistoryDefinitions(_ context.Context, limit, offset int32) ([]productport.ServicePeriodHistoryProduct, int64, error) {
	s.limit, s.offset = limit, offset
	return []productport.ServicePeriodHistoryProduct{}, 0, s.err
}
func (s *servicePeriodHistoryAPIStub) ListServicePeriodHistoryEntitlements(_ context.Context, id int64, limit, offset int32) ([]productport.ServicePeriodHistoryEntitlement, int64, error) {
	s.definitionID, s.limit, s.offset = id, limit, offset
	return []productport.ServicePeriodHistoryEntitlement{{ID: 9, DefinitionID: id, Status: "expired", RenewalCount: -1}}, 1, s.err
}
func (s *servicePeriodHistoryAPIStub) ListServicePeriodHistoryEvents(_ context.Context, id int64, limit, offset int32) ([]productport.ServicePeriodHistoryEvent, int64, error) {
	s.definitionID, s.limit, s.offset = id, limit, offset
	return []productport.ServicePeriodHistoryEvent{{ID: 10, DefinitionID: id, EventType: "admin_adjusted", DurationDays: -7}}, 1, s.err
}

func TestServicePeriodHistoryAuthenticatedReadonlyRoutes(t *testing.T) {
	auth := &recordingAuth{}
	legacy, err := NewHandlerWithOutboundProductsMediaAndSurvey(auth, &legacyCustomerStub{result: legacyCustomerResult()}, &legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, &legacyProductStub{}, &legacyMediaStub{}, &legacySurveyStub{})
	if err != nil {
		t.Fatal(err)
	}
	history := &servicePeriodHistoryAPIStub{}
	legacy.servicePeriodHistory = history
	authHandler, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path, want string
		cap        authport.Capability
	}{
		{"/api/admin/service-period-history?limit=20&offset=0", `"items":[]`, authport.CapabilityProductsRead},
		{"/api/admin/service-period-history/2/entitlements?limit=20&offset=0", `"customer_id":null`, authport.CapabilityEntitlementsRead},
		{"/api/admin/service-period-history/2/events?limit=20&offset=0", `"duration_days":-7`, authport.CapabilityEntitlementsRead},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, test.path, legacyToken(90)))
		if response.Code != 200 || history.limit != 20 || history.offset != 0 || !strings.Contains(response.Body.String(), test.want) || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("response=%d %s", response.Code, response.Body)
		}
		for _, want := range []string{`"source":"v1_history"`, `"read_only":true`, `"real_external_call_executed":false`} {
			if !strings.Contains(response.Body.String(), want) {
				t.Fatal(want)
			}
		}
		caps := auth.capabilities()
		if caps[len(caps)-1] != test.cap {
			t.Fatalf("capabilities=%v", caps)
		}
	}
	for _, path := range []string{"/api/admin/service-period-history?limit=101", "/api/admin/service-period-history?limit=2&limit=3", "/api/admin/service-period-history?offset=-1", "/api/admin/service-period-history?execute=true", "/api/admin/service-period-history/0/events"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(91)))
		if response.Code != 400 {
			t.Fatalf("%s status=%d", path, response.Code)
		}
	}
	history.err = errors.New("private database detail")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/service-period-history", legacyToken(92)))
	if response.Code != 503 || strings.Contains(response.Body.String(), "private database detail") {
		t.Fatal("dependency failure not closed")
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/admin/service-period-history", nil))
	if response.Code != 401 {
		t.Fatalf("unauthenticated=%d", response.Code)
	}
}
