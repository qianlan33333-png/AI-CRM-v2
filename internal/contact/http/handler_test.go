package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

func TestStageHandlerListStagesReturnsServiceRows(t *testing.T) {
	t.Parallel()

	want := []contactport.Stage{
		{ID: 2, Name: "qualified", SortOrder: 10, Config: json.RawMessage(`{"color":"blue"}`)},
		{ID: 7, Name: "won", SortOrder: 20, Config: json.RawMessage(`{"threshold":3}`)},
	}
	service := &stageServiceStub{
		list: func(context.Context) ([]contactport.Stage, error) { return want, nil },
	}
	response := serveStage(t, stageRouter(t, service), stageRequest(t, http.MethodGet, "/api/v1/stages", "", authport.CapabilityStagesRead, 42, true))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	assertStageResponseHeaders(t, response)
	var body struct {
		Items []struct {
			ID        int64          `json:"id"`
			Name      string         `json:"name"`
			SortOrder int32          `json:"sort_order"`
			Config    map[string]any `json:"config"`
		} `json:"items"`
	}
	decodeStageJSON(t, response, &body)
	if len(body.Items) != len(want) {
		t.Fatalf("items = %d, want %d", len(body.Items), len(want))
	}
	if got := body.Items[0]; got.ID != 2 || got.Name != "qualified" || got.SortOrder != 10 || got.Config["color"] != "blue" {
		t.Fatalf("first item = %#v, want qualified stage", got)
	}
	if got := body.Items[1]; got.ID != 7 || got.Name != "won" || got.SortOrder != 20 || got.Config["threshold"] != float64(3) {
		t.Fatalf("second item = %#v, want won stage", got)
	}
	if service.listCalls != 1 || service.createCalls != 0 || service.renameCalls != 0 {
		t.Fatalf("service calls list/create/rename = %d/%d/%d, want 1/0/0", service.listCalls, service.createCalls, service.renameCalls)
	}
}

func TestStageHandlerCreateAndRenamePassAdminActor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		service    *stageServiceStub
		wantStatus int
		assert     func(*testing.T, *stageServiceStub)
	}{
		{
			name:   "create",
			method: http.MethodPost,
			path:   "/api/v1/stages",
			body:   `{"name":"qualified","sort_order":4,"config":{"source":"manual"}}`,
			service: &stageServiceStub{
				create: func(_ context.Context, command contactport.CreateStageCommand) (contactport.Stage, error) {
					return contactport.Stage{ID: 9, Name: command.Name, SortOrder: command.SortOrder, Config: command.Config}, nil
				},
			},
			wantStatus: http.StatusCreated,
			assert: func(t *testing.T, service *stageServiceStub) {
				t.Helper()
				if service.createCalls != 1 || service.renameCalls != 0 || service.listCalls != 0 {
					t.Fatalf("service calls list/create/rename = %d/%d/%d, want 0/1/0", service.listCalls, service.createCalls, service.renameCalls)
				}
				if got := service.createCommand; got.Name != "qualified" || got.SortOrder != 4 || got.Actor != "admin:77" || string(got.Config) != `{"source":"manual"}` {
					t.Fatalf("create command = %#v, want exact name/order/config/admin actor", got)
				}
			},
		},
		{
			name:   "rename",
			method: http.MethodPatch,
			path:   "/api/v1/stages/9",
			body:   `{"name":"closed_won"}`,
			service: &stageServiceStub{
				rename: func(_ context.Context, command contactport.RenameStageCommand) (contactport.Stage, error) {
					return contactport.Stage{ID: command.ID, Name: command.Name, SortOrder: 4, Config: json.RawMessage(`{}`)}, nil
				},
			},
			wantStatus: http.StatusOK,
			assert: func(t *testing.T, service *stageServiceStub) {
				t.Helper()
				if service.createCalls != 0 || service.renameCalls != 1 || service.listCalls != 0 {
					t.Fatalf("service calls list/create/rename = %d/%d/%d, want 0/0/1", service.listCalls, service.createCalls, service.renameCalls)
				}
				if got := service.renameCommand; got.ID != 9 || got.Name != "closed_won" || got.Actor != "admin:77" {
					t.Fatalf("rename command = %#v, want exact id/name/admin actor", got)
				}
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response := serveStage(t, stageRouter(t, test.service), stageRequest(t, test.method, test.path, test.body, authport.CapabilityStagesWrite, 77, true))
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
			assertStageResponseHeaders(t, response)
			test.assert(t, test.service)
			assertStageResponseDoesNotContain(t, response, "admin:77", "stage service", "password")
		})
	}
}

func TestStageHandlerRejectsMissingOrWrongAuthorizationBeforeService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		capability  authport.Capability
		principal   int64
		includeAuth bool
		wantStatus  int
		wantCode    platformhttp.ErrorCode
	}{
		{
			name: "missing authorization", method: http.MethodGet, path: "/api/v1/stages",
			principal: 42, includeAuth: false, wantStatus: http.StatusForbidden, wantCode: platformhttp.CodeUnauthorized,
		},
		{
			name: "list has write authorization", method: http.MethodGet, path: "/api/v1/stages",
			capability: authport.CapabilityStagesWrite, principal: 42, includeAuth: true, wantStatus: http.StatusForbidden, wantCode: platformhttp.CodeUnauthorized,
		},
		{
			name: "create has read authorization", method: http.MethodPost, path: "/api/v1/stages", body: `{"name":"qualified"}`,
			capability: authport.CapabilityStagesRead, principal: 42, includeAuth: true, wantStatus: http.StatusForbidden, wantCode: platformhttp.CodeUnauthorized,
		},
		{
			name: "authorized request has no principal", method: http.MethodPatch, path: "/api/v1/stages/1", body: `{"name":"qualified"}`,
			capability: authport.CapabilityStagesWrite, principal: 0, includeAuth: true, wantStatus: http.StatusUnauthorized, wantCode: platformhttp.CodeUnauthenticated,
		},
		{
			name: "authorized request has invalid principal", method: http.MethodGet, path: "/api/v1/stages",
			capability: authport.CapabilityStagesRead, principal: -1, includeAuth: true, wantStatus: http.StatusUnauthorized, wantCode: platformhttp.CodeUnauthenticated,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			service := &stageServiceStub{}
			request := stageRequest(t, test.method, test.path, test.body, test.capability, test.principal, test.includeAuth)
			response := serveStage(t, stageRouter(t, service), request)
			assertStageError(t, response, test.wantStatus, test.wantCode)
			assertServiceUntouched(t, service)
		})
	}
}

func TestStageHandlerRejectsGeneratedRouteAndCSRFParameterErrorsBeforeService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request func(*testing.T) *http.Request
	}{
		{
			name: "non numeric stage id",
			request: func(t *testing.T) *http.Request {
				return stageRequest(t, http.MethodPatch, "/api/v1/stages/not-a-number", `{"name":"qualified"}`, authport.CapabilityStagesWrite, 42, true)
			},
		},
		{
			name: "missing csrf header",
			request: func(t *testing.T) *http.Request {
				request := stageRequest(t, http.MethodPost, "/api/v1/stages", `{"name":"qualified"}`, authport.CapabilityStagesWrite, 42, true)
				request.Header.Del("X-CSRF-Token")
				return request
			},
		},
		{
			name: "duplicate csrf header",
			request: func(t *testing.T) *http.Request {
				request := stageRequest(t, http.MethodPost, "/api/v1/stages", `{"name":"qualified"}`, authport.CapabilityStagesWrite, 42, true)
				request.Header.Add("X-CSRF-Token", "duplicated-token")
				return request
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			service := &stageServiceStub{}
			response := serveStage(t, stageRouter(t, service), test.request(t))
			assertStageError(t, response, http.StatusBadRequest, platformhttp.CodeMalformedRequest)
			assertServiceUntouched(t, service)
		})
	}
}

func TestStageHandlerRejectsMalformedBodiesBeforeService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "unknown field", body: `{"name":"qualified","private_field":"do_not_accept"}`},
		{name: "multiple json values", body: `{"name":"qualified"} {"name":"second"}`},
		{name: "invalid json", body: `{"name":`},
		{name: "oversize body", body: `{"name":"` + strings.Repeat("x", maxStageBodyBytes) + `"}`},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			service := &stageServiceStub{}
			response := serveStage(t, stageRouter(t, service), stageRequest(t, http.MethodPost, "/api/v1/stages", test.body, authport.CapabilityStagesWrite, 42, true))
			assertStageError(t, response, http.StatusBadRequest, platformhttp.CodeMalformedRequest)
			assertServiceUntouched(t, service)
			assertStageResponseDoesNotContain(t, response, "private_field", "do_not_accept", "second", "xxxxxxxx")
		})
	}
}

func TestStageHandlerClassifiesServiceErrorsWithoutLeakingCauses(t *testing.T) {
	t.Parallel()

	secret := "postgres://postgres:private-password@127.0.0.1/aicrm_test"
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		service    *stageServiceStub
		wantStatus int
		wantCode   platformhttp.ErrorCode
	}{
		{
			name:       "list dependency error",
			method:     http.MethodGet,
			path:       "/api/v1/stages",
			service:    &stageServiceStub{list: func(context.Context) ([]contactport.Stage, error) { return nil, errors.New(secret) }},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   platformhttp.CodeDependencyUnavailable,
		},
		{
			name:   "create validation error",
			method: http.MethodPost,
			path:   "/api/v1/stages",
			body:   `{"name":"qualified"}`,
			service: &stageServiceStub{create: func(context.Context, contactport.CreateStageCommand) (contactport.Stage, error) {
				return contactport.Stage{}, contactport.ErrInvalidStage
			}},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   platformhttp.CodeValidationFailed,
		},
		{
			name:   "rename missing stage",
			method: http.MethodPatch,
			path:   "/api/v1/stages/9",
			body:   `{"name":"qualified"}`,
			service: &stageServiceStub{rename: func(context.Context, contactport.RenameStageCommand) (contactport.Stage, error) {
				return contactport.Stage{}, contactport.ErrStageNotFound
			}},
			wantStatus: http.StatusNotFound,
			wantCode:   platformhttp.CodeNotFound,
		},
		{
			name:   "create unique conflict",
			method: http.MethodPost,
			path:   "/api/v1/stages",
			body:   `{"name":"qualified"}`,
			service: &stageServiceStub{create: func(context.Context, contactport.CreateStageCommand) (contactport.Stage, error) {
				return contactport.Stage{}, &pgconn.PgError{Code: "23505", Message: secret}
			}},
			wantStatus: http.StatusConflict,
			wantCode:   platformhttp.CodeConflict,
		},
		{
			name:   "rename dependency error",
			method: http.MethodPatch,
			path:   "/api/v1/stages/9",
			body:   `{"name":"qualified"}`,
			service: &stageServiceStub{rename: func(context.Context, contactport.RenameStageCommand) (contactport.Stage, error) {
				return contactport.Stage{}, errors.New(secret)
			}},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   platformhttp.CodeDependencyUnavailable,
		},
		{
			name:   "service returned invalid stage",
			method: http.MethodPost,
			path:   "/api/v1/stages",
			body:   `{"name":"qualified"}`,
			service: &stageServiceStub{create: func(context.Context, contactport.CreateStageCommand) (contactport.Stage, error) {
				return contactport.Stage{ID: 0, Name: "qualified", Config: json.RawMessage(`{}`)}, nil
			}},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   platformhttp.CodeDependencyUnavailable,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			capability := authport.CapabilityStagesRead
			if test.method != http.MethodGet {
				capability = authport.CapabilityStagesWrite
			}
			response := serveStage(t, stageRouter(t, test.service), stageRequest(t, test.method, test.path, test.body, capability, 42, true))
			assertStageError(t, response, test.wantStatus, test.wantCode)
			assertStageResponseDoesNotContain(t, response, secret, "stage service returned an invalid stage", "23505")
		})
	}
}

func TestNewStageHandlerRejectsNilServices(t *testing.T) {
	t.Parallel()

	if _, err := NewHandler(nil); err == nil {
		t.Fatal("NewHandler(nil) error = nil, want fail-closed error")
	}
	var typedNil *stageServiceStub
	if _, err := NewHandler(typedNil); err == nil {
		t.Fatal("NewHandler(typed nil) error = nil, want fail-closed error")
	}
}

type stageServiceStub struct {
	list   func(context.Context) ([]contactport.Stage, error)
	create func(context.Context, contactport.CreateStageCommand) (contactport.Stage, error)
	rename func(context.Context, contactport.RenameStageCommand) (contactport.Stage, error)

	listCalls     int
	createCalls   int
	renameCalls   int
	createCommand contactport.CreateStageCommand
	renameCommand contactport.RenameStageCommand
}

func (stub *stageServiceStub) ListStages(ctx context.Context) ([]contactport.Stage, error) {
	stub.listCalls++
	if stub.list == nil {
		return nil, nil
	}
	return stub.list(ctx)
}

func (stub *stageServiceStub) CreateStage(ctx context.Context, command contactport.CreateStageCommand) (contactport.Stage, error) {
	stub.createCalls++
	stub.createCommand = command
	if stub.create == nil {
		return contactport.Stage{}, nil
	}
	return stub.create(ctx, command)
}

func (stub *stageServiceStub) RenameStage(ctx context.Context, command contactport.RenameStageCommand) (contactport.Stage, error) {
	stub.renameCalls++
	stub.renameCommand = command
	if stub.rename == nil {
		return contactport.Stage{}, nil
	}
	return stub.rename(ctx, command)
}

func stageRouter(t *testing.T, service contactport.StageService) http.Handler {
	t.Helper()
	handler, err := NewHandler(service)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	return generated.HandlerWithOptions(handler, generated.ChiServerOptions{
		BaseRouter:       chi.NewRouter(),
		ErrorHandlerFunc: platformhttp.RequestErrorHandler,
	})
}

func stageRequest(
	t *testing.T,
	method string,
	path string,
	body string,
	capability authport.Capability,
	principalID int64,
	includeAuthorization bool,
) *http.Request {
	t.Helper()

	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if method == http.MethodPost || method == http.MethodPatch {
		request.Header.Set("X-CSRF-Token", "csrf-test-token")
	}
	ctx := context.Background()
	if principalID != 0 {
		if principalID < 0 {
			principalID = 0
		}
		ctx = authport.WithAuthenticatedSession(ctx, authport.Principal{AdminUserID: principalID, Role: authport.RoleAdmin}, "session-token")
	}
	if includeAuthorization {
		var err error
		ctx, err = authport.WithAuthorization(ctx, authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal})
		if err != nil {
			t.Fatalf("WithAuthorization() error = %v", err)
		}
	}
	return request.WithContext(ctx)
}

func serveStage(t *testing.T, handler http.Handler, request *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertStageError(t *testing.T, response *httptest.ResponseRecorder, wantStatus int, wantCode platformhttp.ErrorCode) {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", response.Code, wantStatus, response.Body.String())
	}
	assertStageResponseHeaders(t, response)
	var body struct {
		Code      platformhttp.ErrorCode `json:"code"`
		Message   string                 `json:"message"`
		RequestID string                 `json:"request_id"`
	}
	decodeStageJSON(t, response, &body)
	if body.Code != wantCode {
		t.Fatalf("error code = %q, want %q", body.Code, wantCode)
	}
	if body.Message == "" || body.RequestID == "" {
		t.Fatalf("stable error response = %#v, want nonempty message and request_id", body)
	}
}

func assertStageResponseHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}

func assertServiceUntouched(t *testing.T, service *stageServiceStub) {
	t.Helper()
	if service.listCalls != 0 || service.createCalls != 0 || service.renameCalls != 0 {
		t.Fatalf("service calls list/create/rename = %d/%d/%d, want 0/0/0", service.listCalls, service.createCalls, service.renameCalls)
	}
}

func decodeStageJSON(t *testing.T, response *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), destination); err != nil {
		t.Fatalf("response JSON error = %v; body=%q", err, response.Body.String())
	}
}

func assertStageResponseDoesNotContain(t *testing.T, response *httptest.ResponseRecorder, values ...string) {
	t.Helper()
	body := response.Body.String()
	for _, value := range values {
		if value != "" && strings.Contains(body, value) {
			t.Fatalf("response leaked %q: %s", value, body)
		}
	}
}
