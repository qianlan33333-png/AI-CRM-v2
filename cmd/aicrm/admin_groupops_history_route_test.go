package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
)

// groupOpsHistoryFinalRouter deliberately builds the same API composition root
// used by production. It must not be replaced with a hand-built chi router:
// these routes depend on the legacy registration's Authenticate/AdminRead/CSRF
// arguments.
func groupOpsHistoryFinalRouter(t *testing.T, reader groupopsport.HistoricalReader, service *adminGroupOpsHistoryAuth) http.Handler {
	t.Helper()
	legacy, err := NewHandler(service, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.groupOpsHistory = newAdminGroupOpsHistory(reader)
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(
		slog.New(slog.NewJSONHandler(io.Discard, nil)), http.NotFoundHandler(), authHandler, authHandler, legacy,
	)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func TestFinalRouterMountsGroupOpsHistoryReadsWithAdminReadWithoutCSRF(t *testing.T) {
	reader := &adminGroupOpsHistoryStub{}
	service := &adminGroupOpsHistoryAuth{role: authport.RoleAdmin}
	router := groupOpsHistoryFinalRouter(t, reader, service)

	for _, path := range adminGroupOpsHistoryPaths {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, path+"?limit=1", legacyToken(0x81)))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	if reader.calls != len(adminGroupOpsHistoryPaths) || service.csrfCalls != 0 {
		t.Fatalf("reader/csrf calls=%d/%d", reader.calls, service.csrfCalls)
	}
	capabilities := service.capabilities()
	if len(capabilities) != len(adminGroupOpsHistoryPaths) {
		t.Fatalf("capabilities=%v", capabilities)
	}
	for _, capability := range capabilities {
		if capability != authport.CapabilityAdminRead {
			t.Fatalf("capabilities=%v", capabilities)
		}
	}
}

func TestFinalRouterProtectsGroupOpsHistoryReads(t *testing.T) {
	for _, test := range []struct {
		name    string
		role    authport.Role
		request func(string) *http.Request
		want    int
	}{
		{
			name: "anonymous",
			request: func(path string) *http.Request {
				return httptest.NewRequest(http.MethodGet, path, nil)
			},
			want: http.StatusUnauthorized,
		},
		{
			name: "operations",
			role: authport.RoleOps,
			request: func(path string) *http.Request {
				return legacyRequest(http.MethodGet, path+"?limit=1", legacyToken(0x82))
			},
			want: http.StatusForbidden,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := &adminGroupOpsHistoryStub{}
			service := &adminGroupOpsHistoryAuth{role: test.role}
			router := groupOpsHistoryFinalRouter(t, reader, service)
			for _, path := range adminGroupOpsHistoryPaths {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, test.request(path))
				if response.Code != test.want {
					t.Fatalf("GET %s status=%d want=%d body=%s", path, response.Code, test.want, response.Body.String())
				}
			}
			if reader.calls != 0 || service.csrfCalls != 0 {
				t.Fatalf("reader/csrf calls=%d/%d", reader.calls, service.csrfCalls)
			}
		})
	}
}

func TestFinalRouterGroupOpsHistoryMissingReaderFailsClosed(t *testing.T) {
	var missing *adminGroupOpsHistoryStub
	service := &adminGroupOpsHistoryAuth{role: authport.RoleAdmin}
	router := groupOpsHistoryFinalRouter(t, missing, service)

	for _, path := range adminGroupOpsHistoryPaths {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, path+"?limit=1", legacyToken(0x83)))
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	if service.csrfCalls != 0 {
		t.Fatalf("csrf calls=%d", service.csrfCalls)
	}
}
