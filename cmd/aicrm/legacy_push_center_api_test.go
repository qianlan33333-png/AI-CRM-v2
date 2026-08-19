package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	pushcenterapp "github.com/qianlan33333-png/AI-CRM-v2/internal/pushcenter/app"
)

func TestLegacyPushCenterReadRoutesNormalContractAndNoCSRF(t *testing.T) {
	service := &legacyPushCenterStub{summary: legacyPushCenterSummary()}
	router := legacyPushCenterRouter(t, &legacyAuthStub{csrfErr: errors.New("GET must not validate csrf")}, service)
	for _, testCase := range []struct {
		name, target string
		wantKey      string
	}{
		{"sections", "/api/admin/push-center/sections?section=%20questionnaire%20&external_userid=%20ext-1%20", "sections"},
		{"stats", "/api/admin/push-center/stats?status=sent", "counts"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodGet, testCase.target, legacyToken(51)))
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var payload map[string]any
			if err := json.NewDecoder(response.Body).Decode(&payload); err != nil || payload["ok"] != true || payload[testCase.wantKey] == nil {
				t.Fatalf("payload=%#v err=%v", payload, err)
			}
			sections, ok := payload["sections"].([]any)
			if !ok || len(sections) != 13 {
				t.Fatalf("sections=%#v", payload["sections"])
			}
			definitions, ok := payload["status_definitions"].([]any)
			if !ok || len(definitions) != 9 {
				t.Fatalf("status definitions=%#v", payload["status_definitions"])
			}
			if testCase.wantKey == "counts" {
				counts := payload["counts"].(map[string]any)
				if counts["sent"] != float64(2) || counts["shadow_warning"] != float64(1) || payload["real_external_call_executed"] != false || payload["runtime_queue"] == nil {
					t.Fatalf("stats payload=%#v", payload)
				}
			}
		})
	}
	if service.filters[0].Section != "questionnaire" || service.filters[0].ExternalUserID != "ext-1" || service.filters[1].Status != "sent" {
		t.Fatalf("service filters=%+v", service.filters)
	}
}

func TestLegacyPushCenterRejectsNonAdminEvenWhenOuterPolicyGrantsOperationsRead(t *testing.T) {
	staffID := int64(7)
	for _, testCase := range []struct {
		name      string
		principal authport.Principal
	}{
		{name: "ops", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleOps}},
		{name: "sales", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleSales, StaffID: &staffID}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			pushCenter := &legacyPushCenterStub{summary: legacyPushCenterSummary()}
			router := legacyPushCenterRouter(t, &dataHealthAuthStub{
				principal:     testCase.principal,
				authorization: authport.Authorization{Capability: authport.CapabilityOperationsRead, Scope: authport.ScopeGlobal},
			}, pushCenter)
			for _, target := range []string{"/api/admin/push-center/sections", "/api/admin/push-center/stats"} {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, legacyRequest(http.MethodGet, target, legacyToken(52)))
				if response.Code != http.StatusForbidden {
					t.Fatalf("%s status=%d body=%s", target, response.Code, response.Body.String())
				}
			}
			if len(pushCenter.filters) != 0 {
				t.Fatalf("source was called: %#v", pushCenter.filters)
			}
		})
	}
}

func TestLegacyPushCenterDegradesRatherThanZero(t *testing.T) {
	router := legacyPushCenterRouter(t, &legacyAuthStub{}, &legacyPushCenterStub{err: errors.New("read model offline")})
	response := httptest.NewRecorder()

	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/push-center/stats?created_from=not-a-time", legacyToken(53)))
	if response.Code != http.StatusOK {
		t.Fatalf("degraded status=%d body=%s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["degraded"] != true || payload["error_code"] != "production_read_unavailable" || payload["total"] != float64(0) || payload["runtime_queue"] == nil || payload["real_external_call_executed"] != false {
		t.Fatalf("degraded payload=%#v", payload)
	}
	if definitions, ok := payload["status_definitions"].([]any); !ok || len(definitions) != 9 {
		t.Fatalf("degraded definitions=%#v", payload["status_definitions"])
	}
}

type legacyPushCenterStub struct {
	summary pushcenterapp.Summary
	err     error
	filters []pushcenterapp.Filter
}

func (stub *legacyPushCenterStub) Read(_ context.Context, filter pushcenterapp.Filter) (pushcenterapp.Summary, error) {
	stub.filters = append(stub.filters, filter)
	return stub.summary, stub.err
}

func legacyPushCenterSummary() pushcenterapp.Summary {
	return pushcenterapp.Summary{Total: 3,
		ByStatus:          map[string]int64{"pending": 1, "sent": 1, "sent_with_shadow_warning": 1},
		ByEffectiveStatus: map[string]int64{"pending": 1, "sent": 1, "reconciled": 1},
		BySection:         map[string]int64{"questionnaire": 3},
	}
}

func legacyPushCenterRouter(t *testing.T, service authport.Service, pushCenter legacyPushCenterApplication) http.Handler {
	t.Helper()
	legacy := &Handler{auth: service, pushCenter: pushCenter}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}
