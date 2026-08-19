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

	adminopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/app"
	adminopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/port"
	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

func TestExecutionRuntimeRoutesKeepReadOnlyDiagnosticContract(t *testing.T) {
	observed := time.Date(2026, 8, 16, 16, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	reader := &executionRuntimeHTTPReader{snapshot: adminopsport.RuntimeSnapshot{
		Control: &adminopsport.RuntimeControl{Name: "control", State: "observed", Details: map[string]string{"secret_ref": "must-not-leak"}, ObservedAt: observed},
		Observations: []adminopsport.RuntimeObservation{
			{Source: "channel_entry", Queue: "channel", Status: "observed", Attempt: 1, Details: map[string]string{"queue_depth": "2"}, ObservedAt: observed},
			{Source: "group_ops", Queue: "group", Status: "observed", Attempt: 2, Details: map[string]string{"external_userid": "must-not-leak"}, ObservedAt: observed},
			{Source: "wecom_media_status", Queue: "media", Status: "observed", Attempt: 3, StatusURL: "https://media.example.test/status?access_token=must-not-leak", ObservedAt: observed},
		}, ObservedAt: observed,
	}, timeline: adminopsport.ExecutionTimeline{ObservedAt: observed, Graph: adminopsport.ExecutionGraph{Roots: []adminopsport.ExecutionGraphNode{{ID: "root", Kind: "attempt", Status: "observed", Message: "must-not-leak", ObservedAt: observed}}}}, found: true}
	router := executionRuntimeRouter(t, &legacyAuthStub{}, adminopsapp.NewExecutionRuntimeService(reader))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/execution-runtime", legacyToken(61)))
	if response.Code != http.StatusOK {
		t.Fatalf("runtime status=%d body=%s", response.Code, response.Body.String())
	}
	var runtime map[string]any
	if err := json.NewDecoder(response.Body).Decode(&runtime); err != nil {
		t.Fatal(err)
	}
	if runtime["ok"] != true || runtime["observed_only"] != true || runtime["real_external_call_executed"] != false {
		t.Fatalf("runtime=%#v", runtime)
	}
	observations := runtime["observations"].([]any)
	if len(observations) != 3 || observations[1].(map[string]any)["details"].(map[string]any)["external_userid"] != "[REDACTED]" || observations[2].(map[string]any)["status_url"] != "https://media.example.test/status" {
		t.Fatalf("observations=%#v", observations)
	}
	if !strings.HasSuffix(runtime["observed_at"].(string), "Z") || runtime["control"].(map[string]any)["details"].(map[string]any)["secret_ref"] != "[REDACTED]" {
		t.Fatalf("runtime timestamp/redaction=%#v", runtime)
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/executions/%20exe_runtime%20", legacyToken(62)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"execution_id":"exe_runtime"`) || !strings.Contains(response.Body.String(), `"message":"[REDACTED]"`) {
		t.Fatalf("timeline status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestExecutionRuntimeRoutesDistinguishMissingControlNotFoundAndUnavailable(t *testing.T) {
	router := executionRuntimeRouter(t, &legacyAuthStub{}, adminopsapp.NewExecutionRuntimeService(emptyExecutionRuntimeReader{}))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/execution-runtime", legacyToken(63)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ok":false`) || !strings.Contains(response.Body.String(), `"control":null`) {
		t.Fatalf("missing control status=%d body=%s", response.Code, response.Body.String())
	}
	for _, target := range []string{"/api/admin/executions/not-execution", "/api/admin/executions/exe_"} {
		response = httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, target, legacyToken(64)))
		if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error":"execution_not_found"`) {
			t.Fatalf("target=%s status=%d body=%s", target, response.Code, response.Body.String())
		}
	}
	unavailable := executionRuntimeRouter(t, &legacyAuthStub{}, adminopsapp.NewExecutionRuntimeService(&executionRuntimeHTTPReader{runtimeErr: errors.New("read down"), timelineErr: errors.New("read down")}))
	for _, target := range []string{"/api/admin/execution-runtime", "/api/admin/executions/exe_runtime"} {
		response = httptest.NewRecorder()
		unavailable.ServeHTTP(response, legacyRequest(http.MethodGet, target, legacyToken(65)))
		if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "execution_") || !strings.Contains(response.Body.String(), "_unavailable") {
			t.Fatalf("target=%s status=%d body=%s", target, response.Code, response.Body.String())
		}
	}

	ops := executionRuntimeRouter(t, &legacyAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleOps}}, adminopsapp.NewExecutionRuntimeService(emptyExecutionRuntimeReader{}))
	response = httptest.NewRecorder()
	ops.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/execution-runtime", legacyToken(66)))
	if response.Code != http.StatusForbidden {
		t.Fatalf("ops status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestExecutionRuntimeRoutesNormalizeAbsentDetailsToObjects(t *testing.T) {
	observed := time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC)
	reader := &executionRuntimeHTTPReader{snapshot: adminopsport.RuntimeSnapshot{
		Control:      &adminopsport.RuntimeControl{Name: "control", State: "observed", ObservedAt: observed},
		Observations: []adminopsport.RuntimeObservation{{Source: "source", Queue: "queue", Status: "observed", ObservedAt: observed}},
		ObservedAt:   observed,
	}}
	router := executionRuntimeRouter(t, &legacyAuthStub{}, adminopsapp.NewExecutionRuntimeService(reader))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/execution-runtime", legacyToken(68)))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `"details":null`) || !strings.Contains(response.Body.String(), `"details":{}`) {
		t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
	}
}

func TestExecutionRuntimePageIsAdminOnlyCarrier(t *testing.T) {
	for _, test := range []struct {
		name      string
		principal authport.Principal
		want      int
	}{
		{name: "admin", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, want: http.StatusFound},
		{name: "ops", principal: authport.Principal{AdminUserID: 8, Role: authport.RoleOps}, want: http.StatusForbidden},
		{name: "sales", principal: authport.Principal{AdminUserID: 9, Role: authport.RoleSales}, want: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := executionRuntimeRouter(t, &legacyAuthStub{principal: test.principal}, adminopsapp.NewExecutionRuntimeService(emptyExecutionRuntimeReader{}))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodGet, legacyExecutionRuntimePagePath, legacyToken(67)))
			if response.Code != test.want || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("status/headers=%d/%q", response.Code, response.Header().Get("X-Content-Type-Options"))
			}
			if test.want == http.StatusFound && (response.Header().Get("Location") != "/?legacy_admin_path=%2Fadmin%2Fexecution-runtime" || response.Header().Get("Cache-Control") != "private, no-store") {
				t.Fatalf("location/cache=%q/%q", response.Header().Get("Location"), response.Header().Get("Cache-Control"))
			}
		})
	}

	router := executionRuntimeRouter(t, &legacyAuthStub{}, adminopsapp.NewExecutionRuntimeService(emptyExecutionRuntimeReader{}))
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, legacyExecutionRuntimePagePath, nil))
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("method/status/headers=%s/%d/%q/%q/%q", method, response.Code, response.Header().Get("Allow"), response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"))
		}
	}
}

func executionRuntimeRouter(t *testing.T, service authport.Service, runtime legacyExecutionRuntimeApplication) http.Handler {
	t.Helper()
	legacy := &Handler{auth: service, executionRuntime: runtime}
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

type executionRuntimeHTTPReader struct {
	snapshot    adminopsport.RuntimeSnapshot
	timeline    adminopsport.ExecutionTimeline
	found       bool
	runtimeErr  error
	timelineErr error
}

func (reader *executionRuntimeHTTPReader) ReadExecutionRuntime(context.Context) (adminopsport.RuntimeSnapshot, error) {
	return reader.snapshot, reader.runtimeErr
}

func (reader *executionRuntimeHTTPReader) ReadExecutionTimeline(context.Context, string) (adminopsport.ExecutionTimeline, bool, error) {
	return reader.timeline, reader.found, reader.timelineErr
}
