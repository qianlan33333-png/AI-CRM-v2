package p2s11

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

func TestFifthConcurrentRequestForSameAccountReturns429(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
	gateway, err := platformhttp.NewGateway(platformhttp.GatewayOptions{Logger: logger, RequestTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := authhttp.NewHandler(frozenAuth{})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 5)
	release := make(chan struct{})
	endpoint := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started <- struct{}{}
		<-release
		writer.WriteHeader(http.StatusNoContent)
	})
	handler := compose(t, gateway, auth, endpoint)

	var group sync.WaitGroup
	responses := make(chan int, 4)
	for range 4 {
		group.Add(1)
		go func() {
			defer group.Done()
			responses <- serve(handler).Code
		}()
	}
	for range 4 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("four admitted requests did not enter the handler")
		}
	}
	fifth := serve(handler)
	if fifth.Code != http.StatusTooManyRequests {
		t.Fatalf("fifth status = %d, body=%s", fifth.Code, fifth.Body.String())
	}
	assertError(t, fifth, "CONCURRENCY_LIMITED")
	var limitedLog map[string]any
	if err = json.NewDecoder(&logBuffer).Decode(&limitedLog); err != nil {
		t.Fatalf("decode 429 access log: %v", err)
	}
	if limitedLog["account"] != "admin:7" || limitedLog["status"] != float64(http.StatusTooManyRequests) ||
		limitedLog["err"] != "CONCURRENCY_LIMITED" {
		t.Fatalf("429 access log = %#v", limitedLog)
	}
	close(release)
	group.Wait()
	close(responses)
	for status := range responses {
		if status != http.StatusNoContent {
			t.Fatalf("admitted status = %d, want 204", status)
		}
	}
	if afterRelease := serve(handler); afterRelease.Code != http.StatusNoContent {
		t.Fatalf("slot was not released: status=%d", afterRelease.Code)
	}
}

func TestCooperativeDeadlineUsesUnified503(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	gateway, err := platformhttp.NewGateway(platformhttp.GatewayOptions{Logger: logger, RequestTimeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	tail, err := gateway.RecoveryErrorLog(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	if err != nil {
		t.Fatal(err)
	}
	tail, err = gateway.TimeoutMiddleware(tail)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/customers", nil)
	request.Header.Set(platformhttp.RequestIDHeader, "timeout-contract-1")
	response := httptest.NewRecorder()
	handler, err := gateway.RequestIDMiddleware(tail)
	if err != nil {
		t.Fatal(err)
	}
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("timeout status = %d, body=%s", response.Code, response.Body.String())
	}
	assertError(t, response, "DEPENDENCY_UNAVAILABLE")
}

func TestAPIProcessPublishesHealthAndProtectsFrozenOperations(t *testing.T) {
	repoRoot := repositoryRoot(t)
	binary := filepath.Join(t.TempDir(), "aicrm")
	build := exec.Command("go", "build", "-trimpath", "-o", binary, "./cmd/aicrm")
	build.Dir = repoRoot
	build.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOFLAGS=-mod=readonly")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build API binary: %v: %s", err, output)
	}
	address := availableAddress(t)
	command := exec.Command(binary, "--role=api")
	command.Dir = repoRoot
	command.Env = append(os.Environ(),
		"AICRM_DATABASE_URL=postgres://aicrm:acceptance-only@127.0.0.1:5432/aicrm?sslmode=disable",
		"AICRM_HTTP_LISTEN_ADDRESS="+address,
		"AICRM_API_PGX_MAX_CONNS=10",
	)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	waitForHealth(t, address, command, &stderr)

	client := &http.Client{Timeout: time.Second}
	operations := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/admin/config/overview"},
		{http.MethodPost, "/api/v1/auth/logout"},
		{http.MethodGet, "/api/v1/auth/session"},
		{http.MethodGet, "/api/v1/customers"},
		{http.MethodGet, "/api/v1/customers/1"},
		{http.MethodPatch, "/api/v1/customers/1"},
		{http.MethodGet, "/api/v1/customers/1/events"},
		{http.MethodPost, "/api/v1/identity/bind"},
		{http.MethodPost, "/api/v1/identity/ingest"},
		{http.MethodPost, "/api/v1/identity/resolve"},
	}
	for _, operation := range operations {
		request, err := http.NewRequest(operation.method, "http://"+address+operation.path, strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("%s %s: %v", operation.method, operation.path, err)
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s %s status/body/read = %d/%s/%v", operation.method, operation.path, response.StatusCode, body, readErr)
		}
		var payload struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(body, &payload) != nil || payload.Code != "UNAUTHENTICATED" {
			t.Fatalf("%s %s payload = %s", operation.method, operation.path, body)
		}
	}
	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("API graceful exit: %v, stderr=%s", err, stderr.String())
	}
}

type frozenAuth struct{}

func (frozenAuth) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	return authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, nil
}

func (frozenAuth) Authorize(_ context.Context, principal authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	if principal.AdminUserID != 7 || capability != authport.CapabilityCustomersRead {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
}

func (frozenAuth) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func compose(t *testing.T, gateway *platformhttp.Gateway, auth *authhttp.Handler, endpoint http.Handler) http.Handler {
	t.Helper()
	tail, err := gateway.RecoveryErrorLog(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	tail, err = gateway.TimeoutMiddleware(tail)
	if err != nil {
		t.Fatal(err)
	}
	tail, err = gateway.AccountBudgetMiddleware(tail)
	if err != nil {
		t.Fatal(err)
	}
	tail, err = auth.Authorize(authport.CapabilityCustomersRead, tail)
	if err != nil {
		t.Fatal(err)
	}
	tail = auth.Authenticate(tail)
	tail, err = gateway.RequestIDMiddleware(tail)
	if err != nil {
		t.Fatal(err)
	}
	return tail
}

func serve(handler http.Handler) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/customers", nil)
	request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: "acceptance-session"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertError(t *testing.T, response *httptest.ResponseRecorder, want string) {
	t.Helper()
	var payload struct {
		Code      string `json:"code"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload.Code != want || payload.RequestID == "" {
		t.Fatalf("error payload = %+v, decode=%v, body=%s", payload, err, response.Body.String())
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve acceptance source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}

func availableAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func waitForHealth(t *testing.T, address string, command *exec.Cmd, stderr *bytes.Buffer) {
	t.Helper()
	client := &http.Client{Timeout: 200 * time.Millisecond}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get("http://" + address + "/healthz")
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		if command.ProcessState != nil {
			t.Fatalf("API exited before health: %s", stderr.String())
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("API health timeout: %s", stderr.String())
}
