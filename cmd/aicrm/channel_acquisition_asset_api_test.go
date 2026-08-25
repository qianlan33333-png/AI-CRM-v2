package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

func TestCH02AcquisitionAssetRoutesUseExactRootRBACAndCSRF(t *testing.T) {
	service := &legacyAuthStub{}
	legacy, err := NewHandler(service, &legacyCustomerStub{result: legacyCustomerResult()})
	if err != nil {
		t.Fatal(err)
	}
	legacy.channelAcquisitionAsset = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization, ok := authport.AuthorizationFromContext(request.Context())
		if !ok {
			t.Fatal("missing authorization")
		}
		want := authport.CapabilityChannelsRead
		if request.Method == http.MethodPost {
			want = authport.CapabilityChannelsWrite
		}
		if authorization.Capability != want || authorization.Scope != authport.ScopeGlobal {
			t.Fatalf("authorization=%+v want=%s", authorization, want)
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.NotFoundHandler(), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ method, path string }{
		{http.MethodPost, "/api/admin/channels/41/acquisition-assets"},
		{http.MethodGet, "/api/admin/channels/41/acquisition-assets"},
		{http.MethodGet, "/api/admin/channels/41/acquisition-assets/eer_7"},
		{http.MethodPost, "/api/admin/channels/41/acquisition-assets/eer_7/reconcile"},
	} {
		request := legacyRequest(test.method, test.path, legacyToken(0x41))
		if test.method == http.MethodPost {
			request.Header.Set("X-CSRF-Token", legacyToken(0x42))
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("%s %s status/body=%d/%s", test.method, test.path, response.Code, response.Body.String())
		}
	}
	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/admin/channels/41/acquisition-assets", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status/body=%d/%s", unauthenticated.Code, unauthenticated.Body.String())
	}
}

func TestCH03EntrantReceiptRoutesUseExactRootRBACAndCSRF(t *testing.T) {
	service := &legacyAuthStub{}
	legacy, err := NewHandler(service, &legacyCustomerStub{result: legacyCustomerResult()})
	if err != nil {
		t.Fatal(err)
	}
	legacy.entrantReceipts = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization, ok := authport.AuthorizationFromContext(request.Context())
		if !ok {
			t.Fatal("missing authorization")
		}
		want := authport.CapabilityChannelsRead
		if request.Method == http.MethodPost {
			want = authport.CapabilityChannelsWrite
		}
		if authorization.Capability != want || authorization.Scope != authport.ScopeGlobal {
			t.Fatalf("authorization=%+v want=%s", authorization, want)
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.NotFoundHandler(), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ method, path string }{
		{http.MethodGet, "/api/admin/channels/41/acquisition-entrant-receipts"},
		{http.MethodGet, "/api/admin/channels/41/acquisition-entrant-receipts/91"},
		{http.MethodPost, "/api/admin/channels/41/acquisition-entrant-receipts/91/reconcile"},
		{http.MethodGet, "/api/admin/channel-acquisition-entrant-receipts/unassigned"},
		{http.MethodGet, "/api/admin/channel-acquisition-entrant-receipts/unassigned/91"},
		{http.MethodPost, "/api/admin/channel-acquisition-entrant-receipts/unassigned/91/reconcile"},
	} {
		request := legacyRequest(test.method, test.path, legacyToken(0x41))
		if test.method == http.MethodPost {
			request.Header.Set("X-CSRF-Token", legacyToken(0x42))
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("%s %s status/body=%d/%s", test.method, test.path, response.Code, response.Body.String())
		}
	}
}
