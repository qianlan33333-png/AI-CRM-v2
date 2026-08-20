package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

func TestLocalTagCatalogHandlerRoutesEveryNativeOperation(t *testing.T) {
	t.Parallel()

	application := &localTagCatalogApplicationStub{
		list: func(context.Context) (contactapp.LegacyTagCatalog, error) {
			return localTagCatalogFixture(), nil
		},
		createGroup: func(_ context.Context, command contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, contactapp.LegacyTag, error) {
			assertLocalTagCommand(t, command, 71, "local-tag-key-0001")
			if command.GroupName != "Lifecycle" || command.FirstTagName != "Warm" {
				t.Fatalf("create group command = %#v", command)
			}
			return contactapp.LegacyTagGroup{ID: 31, Name: "Lifecycle", SortOrder: 2}, contactapp.LegacyTag{ID: 32, GroupID: 31, GroupName: "Lifecycle", Name: "Warm", SortOrder: 0}, nil
		},
		updateGroup: func(_ context.Context, command contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, error) {
			assertLocalTagCommand(t, command, 71, "local-tag-key-0001")
			if command.GroupID != 11 || command.GroupName != "Renewed" {
				t.Fatalf("update group command = %#v", command)
			}
			return contactapp.LegacyTagGroup{ID: 11, Name: "Renewed", SortOrder: 0}, nil
		},
		archiveGroup: func(_ context.Context, command contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, error) {
			assertLocalTagCommand(t, command, 71, "local-tag-key-0001")
			if command.GroupID != 11 {
				t.Fatalf("archive group command = %#v", command)
			}
			return contactapp.LegacyTagGroup{ID: 11, Name: "__ARCHIVED_INTERNAL_SENTINEL__", SortOrder: 0}, nil
		},
		reorderGroups: func(_ context.Context, command contactapp.LegacyTagCommand) ([]contactapp.LegacyTagGroup, error) {
			assertLocalTagCommand(t, command, 71, "local-tag-key-0001")
			if strings.Join(int64Strings(command.IDs), ",") != "12,11" {
				t.Fatalf("reorder group ids = %#v", command.IDs)
			}
			return []contactapp.LegacyTagGroup{{ID: 12, Name: "Fit", SortOrder: 0}, {ID: 11, Name: "Lifecycle", SortOrder: 1}}, nil
		},
		createTag: func(_ context.Context, command contactapp.LegacyTagCommand) (contactapp.LegacyTag, error) {
			assertLocalTagCommand(t, command, 71, "local-tag-key-0001")
			if command.GroupID != 11 || command.TagName != "Hot" {
				t.Fatalf("create tag command = %#v", command)
			}
			return contactapp.LegacyTag{ID: 21, GroupID: 11, GroupName: "Lifecycle", Name: "Hot", SortOrder: 1}, nil
		},
		updateTag: func(_ context.Context, command contactapp.LegacyTagCommand) (contactapp.LegacyTag, error) {
			assertLocalTagCommand(t, command, 71, "local-tag-key-0001")
			if command.TagID != 21 || command.TagName != "Nurture" {
				t.Fatalf("update tag command = %#v", command)
			}
			return contactapp.LegacyTag{ID: 21, GroupID: 11, GroupName: "Lifecycle", Name: "Nurture", SortOrder: 1}, nil
		},
		archiveTag: func(_ context.Context, command contactapp.LegacyTagCommand) (contactapp.LegacyTag, error) {
			assertLocalTagCommand(t, command, 71, "local-tag-key-0001")
			if command.TagID != 21 {
				t.Fatalf("archive tag command = %#v", command)
			}
			return contactapp.LegacyTag{ID: 21, Name: "__ARCHIVED_INTERNAL_SENTINEL__"}, nil
		},
		reorderTags: func(_ context.Context, command contactapp.LegacyTagCommand) ([]contactapp.LegacyTag, error) {
			assertLocalTagCommand(t, command, 71, "local-tag-key-0001")
			if strings.Join(int64Strings(command.IDs), ",") != "22,21" {
				t.Fatalf("reorder tag ids = %#v", command.IDs)
			}
			return []contactapp.LegacyTag{{ID: 22, GroupID: 12, GroupName: "Fit", Name: "ICP", SortOrder: 0}, {ID: 21, GroupID: 11, GroupName: "Lifecycle", Name: "Warm", SortOrder: 1}}, nil
		},
	}
	router := localTagCatalogRouter(t, application)

	for _, test := range []struct {
		name, method, path, body string
		capability               authport.Capability
		wantStatus               int
	}{
		{"catalog", http.MethodGet, "/api/v1/tag-groups", "", authport.CapabilityCustomersRead, http.StatusOK},
		{"legacy list projection", http.MethodGet, "/api/v1/tags", "", authport.CapabilityCustomersRead, http.StatusOK},
		{"create group", http.MethodPost, "/api/v1/tag-groups", `{"name":"Lifecycle","first_tag_name":"Warm"}`, authport.CapabilityCustomersWrite, http.StatusCreated},
		{"reorder groups", http.MethodPut, "/api/v1/tag-groups/reorder", `{"ids":[12,11]}`, authport.CapabilityCustomersWrite, http.StatusOK},
		{"update group", http.MethodPatch, "/api/v1/tag-groups/11", `{"name":"Renewed"}`, authport.CapabilityCustomersWrite, http.StatusOK},
		{"archive group", http.MethodDelete, "/api/v1/tag-groups/11", "", authport.CapabilityCustomersWrite, http.StatusOK},
		{"create tag", http.MethodPost, "/api/v1/tags", `{"group_id":11,"name":"Hot"}`, authport.CapabilityCustomersWrite, http.StatusCreated},
		{"reorder tags", http.MethodPut, "/api/v1/tags/reorder", `{"ids":[22,21]}`, authport.CapabilityCustomersWrite, http.StatusOK},
		{"update tag", http.MethodPatch, "/api/v1/tags/21", `{"name":"Nurture"}`, authport.CapabilityCustomersWrite, http.StatusOK},
		{"archive tag", http.MethodDelete, "/api/v1/tags/21", "", authport.CapabilityCustomersWrite, http.StatusOK},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			response := serveLocalTagCatalog(router, localTagCatalogRequest(t, test.method, test.path, test.body, test.capability, authport.RoleAdmin, true))
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
			assertLocalTagCatalogHeaders(t, response)
			if strings.Contains(response.Body.String(), "__ARCHIVED_INTERNAL_SENTINEL__") {
				t.Fatalf("archive response leaked service tombstone: %s", response.Body.String())
			}
			if test.method == http.MethodDelete {
				var tombstone map[string]json.RawMessage
				if err := json.Unmarshal(response.Body.Bytes(), &tombstone); err != nil {
					t.Fatalf("decode archive tombstone: %v", err)
				}
				if len(tombstone) != 2 || string(tombstone["id"]) == "" || string(tombstone["archived"]) != "true" {
					t.Fatalf("archive tombstone = %s, want exact id/archived", response.Body.String())
				}
			}
		})
	}

	if application.calls["list"] != 2 || application.calls["createGroup"] != 1 || application.calls["updateGroup"] != 1 || application.calls["archiveGroup"] != 1 || application.calls["reorderGroups"] != 1 || application.calls["createTag"] != 1 || application.calls["updateTag"] != 1 || application.calls["archiveTag"] != 1 || application.calls["reorderTags"] != 1 {
		t.Fatalf("operation calls = %#v", application.calls)
	}
}

func TestLocalTagCatalogHandlerRejectsSalesAndMalformedWriteBeforeService(t *testing.T) {
	t.Parallel()
	application := &localTagCatalogApplicationStub{}
	router := localTagCatalogRouter(t, application)

	for _, test := range []struct {
		name, method, path, body string
		capability               authport.Capability
		role                     authport.Role
		wantStatus               int
		wantCode                 platformhttp.ErrorCode
		mutate                   func(*http.Request)
	}{
		{"sales list denied", http.MethodGet, "/api/v1/tag-groups", "", authport.CapabilityCustomersRead, authport.RoleSales, http.StatusForbidden, platformhttp.CodeUnauthorized, nil},
		{"owner scoped write denied", http.MethodPost, "/api/v1/tags", `{"group_id":11,"name":"Hot"}`, authport.CapabilityCustomersWrite, authport.RoleSales, http.StatusForbidden, platformhttp.CodeUnauthorized, nil},
		{"unknown body field", http.MethodPost, "/api/v1/tag-groups", `{"name":"Lifecycle","first_tag_name":"Warm","private":"no"}`, authport.CapabilityCustomersWrite, authport.RoleAdmin, http.StatusBadRequest, platformhttp.CodeMalformedRequest, nil},
		{"duplicate idempotency", http.MethodPatch, "/api/v1/tags/21", `{"name":"Warm"}`, authport.CapabilityCustomersWrite, authport.RoleAdmin, http.StatusBadRequest, platformhttp.CodeMalformedRequest, func(request *http.Request) { request.Header.Add("Idempotency-Key", "local-tag-key-0002") }},
		{"short idempotency", http.MethodDelete, "/api/v1/tags/21", "", authport.CapabilityCustomersWrite, authport.RoleAdmin, http.StatusBadRequest, platformhttp.CodeMalformedRequest, func(request *http.Request) { request.Header.Set("Idempotency-Key", "short") }},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			request := localTagCatalogRequest(t, test.method, test.path, test.body, test.capability, test.role, true)
			if test.mutate != nil {
				test.mutate(request)
			}
			response := serveLocalTagCatalog(router, request)
			assertLocalTagCatalogError(t, response, test.wantStatus, test.wantCode)
		})
	}
	if len(application.calls) != 0 {
		t.Fatalf("application called on rejected requests: %#v", application.calls)
	}
}

func TestLocalTagCatalogHandlerClassifiesErrorsAndFailsClosedOnInvalidResponses(t *testing.T) {
	t.Parallel()
	application := &localTagCatalogApplicationStub{
		createTag: func(context.Context, contactapp.LegacyTagCommand) (contactapp.LegacyTag, error) {
			return contactapp.LegacyTag{}, errors.Join(contactapp.ErrLegacyTagReferenced, errors.New("private database cause"))
		},
		updateTag: func(context.Context, contactapp.LegacyTagCommand) (contactapp.LegacyTag, error) {
			return contactapp.LegacyTag{ID: 21, GroupID: 11, GroupName: "Lifecycle", Name: "  unsafe ", SortOrder: 0}, nil
		},
	}
	router := localTagCatalogRouter(t, application)

	conflict := serveLocalTagCatalog(router, localTagCatalogRequest(t, http.MethodPost, "/api/v1/tags", `{"group_id":11,"name":"Hot"}`, authport.CapabilityCustomersWrite, authport.RoleOps, true))
	assertLocalTagCatalogError(t, conflict, http.StatusConflict, platformhttp.CodeConflict)
	if strings.Contains(conflict.Body.String(), "private database cause") {
		t.Fatalf("error leaked private cause: %s", conflict.Body.String())
	}

	invalid := serveLocalTagCatalog(router, localTagCatalogRequest(t, http.MethodPatch, "/api/v1/tags/21", `{"name":"Warm"}`, authport.CapabilityCustomersWrite, authport.RoleOps, true))
	assertLocalTagCatalogError(t, invalid, http.StatusServiceUnavailable, platformhttp.CodeDependencyUnavailable)
}

func TestLocalTagCatalogHandlerFailsClosedOnDuplicateOrInconsistentCatalog(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		catalog contactapp.LegacyTagCatalog
	}{
		{
			name: "duplicate group id",
			catalog: contactapp.LegacyTagCatalog{
				Groups: []contactapp.LegacyTagGroup{{ID: 11, Name: "Lifecycle", SortOrder: 0}, {ID: 11, Name: "Duplicate", SortOrder: 1}},
				Tags:   []contactapp.LegacyTag{},
			},
		},
		{
			name: "duplicate tag id",
			catalog: contactapp.LegacyTagCatalog{
				Groups: []contactapp.LegacyTagGroup{{ID: 11, Name: "Lifecycle", SortOrder: 0}},
				Tags: []contactapp.LegacyTag{
					{ID: 21, GroupID: 11, GroupName: "Lifecycle", Name: "Warm", SortOrder: 0},
					{ID: 21, GroupID: 11, GroupName: "Lifecycle", Name: "Hot", SortOrder: 1},
				},
			},
		},
		{
			name: "tag group name drift",
			catalog: contactapp.LegacyTagCatalog{
				Groups: []contactapp.LegacyTagGroup{{ID: 11, Name: "Lifecycle", SortOrder: 0}},
				Tags:   []contactapp.LegacyTag{{ID: 21, GroupID: 11, GroupName: "Different", Name: "Warm", SortOrder: 0}},
			},
		},
		{
			name: "tag group missing",
			catalog: contactapp.LegacyTagCatalog{
				Groups: []contactapp.LegacyTagGroup{{ID: 11, Name: "Lifecycle", SortOrder: 0}},
				Tags:   []contactapp.LegacyTag{{ID: 21, GroupID: 12, GroupName: "Missing", Name: "Warm", SortOrder: 0}},
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			application := &localTagCatalogApplicationStub{list: func(context.Context) (contactapp.LegacyTagCatalog, error) {
				return test.catalog, nil
			}}
			response := serveLocalTagCatalog(localTagCatalogRouter(t, application), localTagCatalogRequest(t, http.MethodGet, "/api/v1/tag-groups", "", authport.CapabilityCustomersRead, authport.RoleAdmin, true))
			assertLocalTagCatalogError(t, response, http.StatusServiceUnavailable, platformhttp.CodeDependencyUnavailable)
			if strings.Contains(response.Body.String(), "Lifecycle") || strings.Contains(response.Body.String(), "Duplicate") || strings.Contains(response.Body.String(), "Different") || strings.Contains(response.Body.String(), "Missing") {
				t.Fatalf("invalid catalog leaked through response: %s", response.Body.String())
			}
		})
	}
}

type localTagCatalogApplicationStub struct {
	list          func(context.Context) (contactapp.LegacyTagCatalog, error)
	createGroup   func(context.Context, contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, contactapp.LegacyTag, error)
	updateGroup   func(context.Context, contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, error)
	archiveGroup  func(context.Context, contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, error)
	reorderGroups func(context.Context, contactapp.LegacyTagCommand) ([]contactapp.LegacyTagGroup, error)
	createTag     func(context.Context, contactapp.LegacyTagCommand) (contactapp.LegacyTag, error)
	updateTag     func(context.Context, contactapp.LegacyTagCommand) (contactapp.LegacyTag, error)
	archiveTag    func(context.Context, contactapp.LegacyTagCommand) (contactapp.LegacyTag, error)
	reorderTags   func(context.Context, contactapp.LegacyTagCommand) ([]contactapp.LegacyTag, error)
	calls         map[string]int
}

func (stub *localTagCatalogApplicationStub) List(ctx context.Context) (contactapp.LegacyTagCatalog, error) {
	stub.called("list")
	if stub.list == nil {
		return contactapp.LegacyTagCatalog{}, nil
	}
	return stub.list(ctx)
}
func (stub *localTagCatalogApplicationStub) CreateGroup(ctx context.Context, command contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, contactapp.LegacyTag, error) {
	stub.called("createGroup")
	if stub.createGroup == nil {
		return contactapp.LegacyTagGroup{}, contactapp.LegacyTag{}, nil
	}
	return stub.createGroup(ctx, command)
}
func (stub *localTagCatalogApplicationStub) UpdateGroup(ctx context.Context, command contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, error) {
	stub.called("updateGroup")
	if stub.updateGroup == nil {
		return contactapp.LegacyTagGroup{}, nil
	}
	return stub.updateGroup(ctx, command)
}
func (stub *localTagCatalogApplicationStub) ArchiveGroup(ctx context.Context, command contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, error) {
	stub.called("archiveGroup")
	if stub.archiveGroup == nil {
		return contactapp.LegacyTagGroup{}, nil
	}
	return stub.archiveGroup(ctx, command)
}
func (stub *localTagCatalogApplicationStub) ReorderGroups(ctx context.Context, command contactapp.LegacyTagCommand) ([]contactapp.LegacyTagGroup, error) {
	stub.called("reorderGroups")
	if stub.reorderGroups == nil {
		return nil, nil
	}
	return stub.reorderGroups(ctx, command)
}
func (stub *localTagCatalogApplicationStub) CreateTag(ctx context.Context, command contactapp.LegacyTagCommand) (contactapp.LegacyTag, error) {
	stub.called("createTag")
	if stub.createTag == nil {
		return contactapp.LegacyTag{}, nil
	}
	return stub.createTag(ctx, command)
}
func (stub *localTagCatalogApplicationStub) UpdateTag(ctx context.Context, command contactapp.LegacyTagCommand) (contactapp.LegacyTag, error) {
	stub.called("updateTag")
	if stub.updateTag == nil {
		return contactapp.LegacyTag{}, nil
	}
	return stub.updateTag(ctx, command)
}
func (stub *localTagCatalogApplicationStub) ArchiveTag(ctx context.Context, command contactapp.LegacyTagCommand) (contactapp.LegacyTag, error) {
	stub.called("archiveTag")
	if stub.archiveTag == nil {
		return contactapp.LegacyTag{}, nil
	}
	return stub.archiveTag(ctx, command)
}
func (stub *localTagCatalogApplicationStub) ReorderTags(ctx context.Context, command contactapp.LegacyTagCommand) ([]contactapp.LegacyTag, error) {
	stub.called("reorderTags")
	if stub.reorderTags == nil {
		return nil, nil
	}
	return stub.reorderTags(ctx, command)
}
func (stub *localTagCatalogApplicationStub) called(name string) {
	if stub.calls == nil {
		stub.calls = map[string]int{}
	}
	stub.calls[name]++
}

func localTagCatalogFixture() contactapp.LegacyTagCatalog {
	return contactapp.LegacyTagCatalog{Groups: []contactapp.LegacyTagGroup{{ID: 11, Name: "Lifecycle", SortOrder: 0}}, Tags: []contactapp.LegacyTag{{ID: 21, GroupID: 11, GroupName: "Lifecycle", Name: "Warm", SortOrder: 0}}}
}

func localTagCatalogRouter(t *testing.T, application localTagCatalogApplication) http.Handler {
	t.Helper()
	handler, err := NewLocalTagCatalogHandler(application)
	if err != nil {
		t.Fatalf("NewLocalTagCatalogHandler() error = %v", err)
	}
	return generated.HandlerWithOptions(handler, generated.ChiServerOptions{BaseRouter: chi.NewRouter(), ErrorHandlerFunc: platformhttp.RequestErrorHandler})
}

func localTagCatalogRequest(t *testing.T, method, path, body string, capability authport.Capability, role authport.Role, includeAuthorization bool) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if method != http.MethodGet {
		request.Header.Set("X-CSRF-Token", "csrf-local-tag")
		request.Header.Set("Idempotency-Key", "local-tag-key-0001")
	}
	ctx := authport.WithAuthenticatedSession(context.Background(), authport.Principal{AdminUserID: 71, Role: role}, "local-tag-session")
	if includeAuthorization {
		var err error
		ctx, err = authport.WithAuthorization(ctx, authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal})
		if err != nil {
			t.Fatalf("WithAuthorization() error = %v", err)
		}
	}
	return request.WithContext(ctx)
}

func serveLocalTagCatalog(handler http.Handler, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertLocalTagCatalogError(t *testing.T, response *httptest.ResponseRecorder, wantStatus int, wantCode platformhttp.ErrorCode) {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", response.Code, wantStatus, response.Body.String())
	}
	assertLocalTagCatalogHeaders(t, response)
	var body struct {
		Code      platformhttp.ErrorCode `json:"code"`
		Message   string                 `json:"message"`
		RequestID string                 `json:"request_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Code != wantCode || body.Message == "" || body.RequestID == "" {
		t.Fatalf("error body = %#v", body)
	}
}

func assertLocalTagCatalogHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", response.Header().Get("Cache-Control"))
	}
	if !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("Content-Type = %q", response.Header().Get("Content-Type"))
	}
}
func assertLocalTagCommand(t *testing.T, command contactapp.LegacyTagCommand, actor int64, key string) {
	t.Helper()
	if command.Actor != actor || command.IdempotencyKey != key {
		t.Fatalf("command actor/key = %d/%q", command.Actor, command.IdempotencyKey)
	}
}
func int64Strings(values []int64) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = strconv.FormatInt(value, 10)
	}
	return out
}
