package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	domainverification "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/domainverification"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

func TestDomainVerificationRootRoutePreservesLegacyReadContract(t *testing.T) {
	root := t.TempDir()
	const filename = "WW_verify_legacy-root.txt"
	if err := os.WriteFile(filepath.Join(root, filename), []byte(" \nlegacy-verification-text\t "), 0o600); err != nil {
		t.Fatal(err)
	}
	router := domainVerificationRouter(t, root)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/"+filename, nil))
	if response.Code != http.StatusOK || response.Body.String() != "legacy-verification-text" {
		t.Fatalf("GET root verification = %d %q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
}

func TestDomainVerificationFailsClosedAndDoesNotCaptureReservedRoutes(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "WW_verify_directory.txt"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "WW_verify_nonutf8.txt"), []byte{0xff}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "WW_verify_large.txt"), []byte(strings.Repeat("x", domainverification.MaxFileBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "WW_verify_outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "WW_verify_link.txt")); err != nil {
		t.Fatal(err)
	}
	router := domainVerificationRouter(t, root)

	for _, test := range []struct{ method, path string }{
		{http.MethodGet, "/WW_verify_missing.txt"},
		{http.MethodGet, "/WW_verify_directory.txt"},
		{http.MethodGet, "/WW_verify_nonutf8.txt"},
		{http.MethodGet, "/WW_verify_large.txt"},
		{http.MethodGet, "/WW_verify_link.txt"},
		{http.MethodGet, "/not-a-verification.txt"},
		{http.MethodPost, "/WW_verify_missing.txt"},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		if response.Code != http.StatusNotFound && !(test.method == http.MethodPost && response.Code == http.StatusBadRequest) {
			t.Fatalf("%s %s = %d, body=%s", test.method, test.path, response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), root) || strings.Contains(response.Body.String(), outside) {
			t.Fatalf("%s %s leaked filesystem path: %s", test.method, test.path, response.Body.String())
		}
	}

	for _, test := range []struct {
		path       string
		wantStatus int
		wantType   string
	}{
		{"/healthz", http.StatusOK, "application/json"},
		{"/login", http.StatusOK, "text/html"},
		{"/logout", http.StatusFound, ""},
		{"/api/v1/customers", http.StatusUnauthorized, "application/json"},
		{"/auth/wecom/start", http.StatusServiceUnavailable, "application/json"},
		{"/wecom/external-contact/callback", http.StatusNoContent, ""},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != test.wantStatus || (test.wantType != "" && !strings.HasPrefix(response.Header().Get("Content-Type"), test.wantType)) {
			t.Fatalf("reserved GET %s = %d %q body=%s", test.path, response.Code, response.Header().Get("Content-Type"), response.Body.String())
		}
	}
}

func domainVerificationRouter(t *testing.T, root string) http.Handler {
	t.Helper()
	reader, err := domainverification.New(root)
	if err != nil {
		t.Fatal(err)
	}
	service := &recordingAuth{}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	candidate := &candidateHandler{Handler: authHandler, domainVerification: reader}
	router, err := newAPIHandlerWithAll(
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) }),
		authHandler, candidate, nil, &HumanAuthHandler{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func TestDomainVerificationHandlerReturnsStableNotFound(t *testing.T) {
	root := t.TempDir()
	reader, err := domainverification.New(root)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	(&candidateHandler{domainVerification: reader}).GetDomainVerificationFile(response, httptest.NewRequest(http.MethodGet, "/WW_verify_missing.txt", nil), "WW_verify_missing.txt")
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), string(platformhttp.CodeNotFound)) {
		t.Fatalf("missing verification response = %d %s", response.Code, response.Body.String())
	}
}
