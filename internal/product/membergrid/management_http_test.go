package membergrid

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeManagementApplication struct {
	shareResponse              ShareSettingsResponse
	createViewResponse         SavedViewResponse
	updateViewResponse         SavedViewResponse
	deleteViewResponse         DeleteSavedViewResponse
	createCollaboratorResponse CollaboratorResponse
	updateCollaboratorResponse CollaboratorResponse
	deleteCollaboratorResponse DeleteCollaboratorResponse

	shareErr              error
	createViewErr         error
	updateViewErr         error
	deleteViewErr         error
	createCollaboratorErr error
	updateCollaboratorErr error
	deleteCollaboratorErr error

	shareProductID            int64
	createViewCommand         CreateSavedViewCommand
	updateViewCommand         UpdateSavedViewCommand
	deleteViewCommand         DeleteSavedViewCommand
	createCollaboratorCommand CreateCollaboratorCommand
	updateCollaboratorCommand UpdateCollaboratorCommand
	deleteCollaboratorCommand DeleteCollaboratorCommand

	shareCalls              int
	createViewCalls         int
	updateViewCalls         int
	deleteViewCalls         int
	createCollaboratorCalls int
	updateCollaboratorCalls int
	deleteCollaboratorCalls int
}

func (application *fakeManagementApplication) ShareSettings(_ context.Context, productID int64) (ShareSettingsResponse, error) {
	application.shareCalls++
	application.shareProductID = productID
	return application.shareResponse, application.shareErr
}

func (application *fakeManagementApplication) CreateSavedView(_ context.Context, command CreateSavedViewCommand) (SavedViewResponse, error) {
	application.createViewCalls++
	application.createViewCommand = command
	return application.createViewResponse, application.createViewErr
}

func (application *fakeManagementApplication) UpdateSavedView(_ context.Context, command UpdateSavedViewCommand) (SavedViewResponse, error) {
	application.updateViewCalls++
	application.updateViewCommand = command
	return application.updateViewResponse, application.updateViewErr
}

func (application *fakeManagementApplication) DeleteSavedView(_ context.Context, command DeleteSavedViewCommand) (DeleteSavedViewResponse, error) {
	application.deleteViewCalls++
	application.deleteViewCommand = command
	return application.deleteViewResponse, application.deleteViewErr
}

func (application *fakeManagementApplication) CreateCollaborator(_ context.Context, command CreateCollaboratorCommand) (CollaboratorResponse, error) {
	application.createCollaboratorCalls++
	application.createCollaboratorCommand = command
	return application.createCollaboratorResponse, application.createCollaboratorErr
}

func (application *fakeManagementApplication) UpdateCollaborator(_ context.Context, command UpdateCollaboratorCommand) (CollaboratorResponse, error) {
	application.updateCollaboratorCalls++
	application.updateCollaboratorCommand = command
	return application.updateCollaboratorResponse, application.updateCollaboratorErr
}

func (application *fakeManagementApplication) DeleteCollaborator(_ context.Context, command DeleteCollaboratorCommand) (DeleteCollaboratorResponse, error) {
	application.deleteCollaboratorCalls++
	application.deleteCollaboratorCommand = command
	return application.deleteCollaboratorResponse, application.deleteCollaboratorErr
}

func (application *fakeManagementApplication) totalCalls() int {
	return application.shareCalls + application.createViewCalls + application.updateViewCalls + application.deleteViewCalls +
		application.createCollaboratorCalls + application.updateCollaboratorCalls + application.deleteCollaboratorCalls
}

type fakeManagementAuthorizer struct {
	actor        ManagementActor
	err          error
	capabilities []string
}

func (authorizer *fakeManagementAuthorizer) Authorize(_ context.Context, capability string) (ManagementActor, error) {
	authorizer.capabilities = append(authorizer.capabilities, capability)
	return authorizer.actor, authorizer.err
}

type fakeManagementCSRF struct {
	err   error
	calls int
}

func (csrf *fakeManagementCSRF) Verify(*http.Request) error {
	csrf.calls++
	return csrf.err
}

func managementTestFragment(t *testing.T, application *fakeManagementApplication, authorizer *fakeManagementAuthorizer, csrf *fakeManagementCSRF) http.Handler {
	t.Helper()
	if authorizer == nil {
		authorizer = &fakeManagementAuthorizer{actor: ManagementActor{ID: 17}}
	}
	if csrf == nil {
		csrf = &fakeManagementCSRF{}
	}
	handler, err := NewManagementHandler(application, authorizer, csrf)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := NewManagementRouteFragment(handler)
	if err != nil {
		t.Fatal(err)
	}
	return fragment
}

func managementRequest(method, path, body, key string) *http.Request {
	request := httptest.NewRequest(method, "http://membergrid.local"+path, strings.NewReader(body))
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodDelete {
		request.Header.Set("Content-Type", "application/json")
		if key != "" {
			request.Header.Set("Idempotency-Key", key)
		}
	}
	return request
}

func managementSampleView(productID, viewID int64) SavedView {
	created := time.Date(2026, 8, 22, 2, 3, 4, 0, time.UTC)
	return SavedView{
		ID: viewID, ServiceProductID: productID, Name: "活跃成员", State: StateActive,
		Sort: ViewSortGrantedAtDesc, Columns: []string{"display_name", "state", "granted_at"},
		Version: 1, CreatedBy: 17, CreatedAt: created, UpdatedAt: created,
	}
}

func managementSampleCollaborator(productID, collaboratorID int64) Collaborator {
	created := time.Date(2026, 8, 22, 2, 4, 5, 0, time.UTC)
	return Collaborator{
		ID: collaboratorID, ServiceProductID: productID, StaffID: 29, Permission: CollaboratorPermissionEdit,
		Version: 1, InvitedBy: 17, CreatedAt: created, UpdatedAt: created,
	}
}

func TestManagementRouteFragmentDispatchesSevenClosedRoutes(t *testing.T) {
	view := managementSampleView(7, 11)
	collaborator := managementSampleCollaborator(7, 13)
	application := &fakeManagementApplication{
		shareResponse: ShareSettingsResponse{
			ServiceProductID: 7, SavedViews: []SavedView{view}, Collaborators: []Collaborator{collaborator},
			CollaboratorEditIsLocalMetadataOnly: true,
		},
		createViewResponse: SavedViewResponse{OK: true, View: view},
		updateViewResponse: SavedViewResponse{OK: true, View: view},
		deleteViewResponse: DeleteSavedViewResponse{OK: true, Deleted: true, View: view},
		createCollaboratorResponse: CollaboratorResponse{
			OK: true, Collaborator: collaborator, EditPermissionIsLocalMetadataOnly: true,
		},
		updateCollaboratorResponse: CollaboratorResponse{
			OK: true, Collaborator: collaborator, EditPermissionIsLocalMetadataOnly: true,
		},
		deleteCollaboratorResponse: DeleteCollaboratorResponse{
			OK: true, Deleted: true, Collaborator: collaborator, EditPermissionIsLocalMetadataOnly: true,
		},
	}
	authorizer := &fakeManagementAuthorizer{actor: ManagementActor{ID: 17}}
	csrf := &fakeManagementCSRF{}
	fragment := managementTestFragment(t, application, authorizer, csrf)

	cloneSource := int64(11)
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		key        string
		wantStatus int
	}{
		{
			name: "create direct view", method: http.MethodPost,
			path: RoutePrefix + "/7/member-views",
			body: `{"expected_version":0,"name":"活跃成员","state":"active","sort":"granted_at_desc","columns":["display_name","state","granted_at"]}`,
			key:  "create-view-key-0001", wantStatus: http.StatusCreated,
		},
		{
			name: "update view relative", method: http.MethodPut, path: "/7/member-views/11",
			body: `{"expected_version":1,"name":"更新视图","state":"all","sort":"granted_at_desc","columns":["display_name","masked_mobile"]}`,
			key:  "update-view-key-0001", wantStatus: http.StatusOK,
		},
		{
			name: "delete view", method: http.MethodDelete, path: RoutePrefix + "/7/member-views/11",
			body: `{"expected_version":1}`, key: "delete-view-key-0001", wantStatus: http.StatusOK,
		},
		{
			name: "share settings", method: http.MethodGet,
			path: RoutePrefix + "/7/member-grid/share-settings", wantStatus: http.StatusOK,
		},
		{
			name: "create collaborator", method: http.MethodPost, path: "/7/member-grid/collaborators",
			body: `{"expected_version":0,"staff_id":29,"permission":"edit"}`,
			key:  "create-collab-key-01", wantStatus: http.StatusCreated,
		},
		{
			name: "update collaborator", method: http.MethodPut, path: RoutePrefix + "/7/member-grid/collaborators/13",
			body: `{"expected_version":1,"permission":"view"}`,
			key:  "update-collab-key-01", wantStatus: http.StatusOK,
		},
		{
			name: "delete collaborator", method: http.MethodDelete, path: "/7/member-grid/collaborators/13",
			body: `{"expected_version":1}`, key: "delete-collab-key-01", wantStatus: http.StatusOK,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			fragment.ServeHTTP(recorder, managementRequest(testCase.method, testCase.path, testCase.body, testCase.key))
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status/body=%d/%s", recorder.Code, recorder.Body.String())
			}
			assertSecurityHeaders(t, recorder)
			if recorder.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("content-type=%q", recorder.Header().Get("Content-Type"))
			}
		})
	}

	if application.shareProductID != 7 || application.shareCalls != 1 || application.createViewCalls != 1 || application.updateViewCalls != 1 ||
		application.deleteViewCalls != 1 || application.createCollaboratorCalls != 1 || application.updateCollaboratorCalls != 1 ||
		application.deleteCollaboratorCalls != 1 {
		t.Fatalf("application calls=%+v", application)
	}
	if command := application.createViewCommand; command.ServiceProductID != 7 || command.ExpectedVersion != 0 || command.Name != "活跃成员" ||
		command.State != StateActive || command.Sort != ViewSortGrantedAtDesc || !reflect.DeepEqual(command.Columns, []string{"display_name", "state", "granted_at"}) ||
		command.SourceViewID != nil || command.ActorID != 17 || command.IdempotencyKey != "create-view-key-0001" {
		t.Fatalf("create view command=%+v", command)
	}
	if command := application.updateViewCommand; command.ServiceProductID != 7 || command.ViewID != 11 || command.ExpectedVersion != 1 ||
		command.Name != "更新视图" || command.State != StateAll || command.Sort != ViewSortGrantedAtDesc ||
		!reflect.DeepEqual(command.Columns, []string{"display_name", "masked_mobile"}) || command.ActorID != 17 {
		t.Fatalf("update view command=%+v", command)
	}
	if command := application.deleteViewCommand; command.ServiceProductID != 7 || command.ViewID != 11 || command.ExpectedVersion != 1 || command.ActorID != 17 {
		t.Fatalf("delete view command=%+v", command)
	}
	if command := application.createCollaboratorCommand; command.ServiceProductID != 7 || command.ExpectedVersion != 0 ||
		command.StaffID != 29 || command.Permission != CollaboratorPermissionEdit || command.ActorID != 17 {
		t.Fatalf("create collaborator command=%+v", command)
	}
	if command := application.updateCollaboratorCommand; command.ServiceProductID != 7 || command.CollaboratorID != 13 || command.ExpectedVersion != 1 ||
		command.Permission != CollaboratorPermissionView || command.ActorID != 17 {
		t.Fatalf("update collaborator command=%+v", command)
	}
	if command := application.deleteCollaboratorCommand; command.ServiceProductID != 7 || command.CollaboratorID != 13 || command.ExpectedVersion != 1 || command.ActorID != 17 {
		t.Fatalf("delete collaborator command=%+v", command)
	}
	if csrf.calls != 6 {
		t.Fatalf("csrf calls=%d", csrf.calls)
	}
	wantCapabilities := []string{
		CapabilityProductsWrite, CapabilityProductsWrite, CapabilityProductsWrite, CapabilityProductsRead,
		CapabilityProductsWrite, CapabilityProductsWrite, CapabilityProductsWrite,
	}
	if !reflect.DeepEqual(authorizer.capabilities, wantCapabilities) {
		t.Fatalf("capabilities=%v want=%v", authorizer.capabilities, wantCapabilities)
	}

	// A clone request is a separate closed shape: name + source only, and the
	// application receives no caller-controlled filter/sort/columns.
	application.createViewCommand = CreateSavedViewCommand{}
	recorder := httptest.NewRecorder()
	fragment.ServeHTTP(recorder, managementRequest(http.MethodPost, "/7/member-views",
		`{"expected_version":0,"name":"复制视图","source_view_id":11}`, "clone-view-key-00001"))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("clone status/body=%d/%s", recorder.Code, recorder.Body.String())
	}
	if command := application.createViewCommand; command.SourceViewID == nil || *command.SourceViewID != cloneSource || command.State != "" || command.Sort != "" || len(command.Columns) != 0 {
		t.Fatalf("clone command=%+v", command)
	}
}

func TestManagementShareSettingsAlwaysExposeLocalOnlyFlags(t *testing.T) {
	application := &fakeManagementApplication{shareResponse: ShareSettingsResponse{
		ServiceProductID:                        9,
		SavedViews:                              []SavedView{},
		Collaborators:                           []Collaborator{},
		ExternalShareSupported:                  false,
		ExternalShareEnabled:                    false,
		RealExternalCallExecuted:                false,
		CollaboratorEditIsLocalMetadataOnly:     true,
		CollaboratorEditGrantsCentralPermission: false,
	}}
	recorder := httptest.NewRecorder()
	managementTestFragment(t, application, nil, nil).ServeHTTP(recorder,
		managementRequest(http.MethodGet, "/9/member-grid/share-settings", "", ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status/body=%d/%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"external_share_supported", "external_share_enabled", "real_external_call_executed", "collaborator_edit_grants_central_permission"} {
		if value, ok := body[key].(bool); !ok || value {
			t.Fatalf("%s=%v", key, body[key])
		}
	}
	if value, ok := body["collaborator_edit_is_local_metadata_only"].(bool); !ok || !value {
		t.Fatalf("local metadata flag=%v", body["collaborator_edit_is_local_metadata_only"])
	}
	for _, forbidden := range []string{"public_token", "share_url", "qr_code", "provider_receipt", "external_userid", "unionid"} {
		if _, exists := body[forbidden]; exists {
			t.Fatalf("forbidden response field=%q", forbidden)
		}
	}
}

func TestManagementBodiesAreStrictAndBuiltInViewIsImmutable(t *testing.T) {
	validKey := strings.Repeat("v", 20)
	cases := []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
		wantStatus  int
	}{
		{name: "empty", method: http.MethodPost, path: "/1/member-views", body: "", contentType: "application/json", wantStatus: http.StatusBadRequest},
		{name: "array", method: http.MethodPost, path: "/1/member-views", body: `[]`, contentType: "application/json", wantStatus: http.StatusBadRequest},
		{name: "unknown", method: http.MethodPost, path: "/1/member-views", body: `{"expected_version":0,"name":"x","raw_filter":{"sql":"1=1"}}`, contentType: "application/json", wantStatus: http.StatusBadRequest},
		{name: "duplicate", method: http.MethodPost, path: "/1/member-views", body: `{"expected_version":0,"expected_version":0,"name":"x"}`, contentType: "application/json", wantStatus: http.StatusBadRequest},
		{name: "trailing", method: http.MethodPost, path: "/1/member-views", body: `{} {}`, contentType: "application/json", wantStatus: http.StatusBadRequest},
		{name: "wrong type", method: http.MethodPost, path: "/1/member-views", body: `{"expected_version":"0","name":"x"}`, contentType: "application/json", wantStatus: http.StatusUnprocessableEntity},
		{name: "missing create fields", method: http.MethodPost, path: "/1/member-views", body: `{"expected_version":0,"name":"x"}`, contentType: "application/json", wantStatus: http.StatusUnprocessableEntity},
		{name: "clone null", method: http.MethodPost, path: "/1/member-views", body: `{"expected_version":0,"name":"x","source_view_id":null}`, contentType: "application/json", wantStatus: http.StatusUnprocessableEntity},
		{name: "clone arbitrary configuration", method: http.MethodPost, path: "/1/member-views", body: `{"expected_version":0,"name":"x","source_view_id":2,"state":"all"}`, contentType: "application/json", wantStatus: http.StatusUnprocessableEntity},
		{name: "missing delete version", method: http.MethodDelete, path: "/1/member-views/2", body: `{}`, contentType: "application/json", wantStatus: http.StatusUnprocessableEntity},
		{name: "missing collaborator field", method: http.MethodPost, path: "/1/member-grid/collaborators", body: `{"expected_version":0,"staff_id":2}`, contentType: "application/json", wantStatus: http.StatusUnprocessableEntity},
		{name: "missing content type", method: http.MethodPost, path: "/1/member-views", body: `{}`, contentType: "", wantStatus: http.StatusBadRequest},
		{name: "text content type", method: http.MethodPost, path: "/1/member-views", body: `{}`, contentType: "text/plain", wantStatus: http.StatusBadRequest},
		{name: "oversized", method: http.MethodPost, path: "/1/member-views", body: `{"name":"` + strings.Repeat("x", int(maximumManagementBodyBytes)) + `"}`, contentType: "application/json", wantStatus: http.StatusBadRequest},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			application := &fakeManagementApplication{}
			recorder := httptest.NewRecorder()
			request := managementRequest(testCase.method, testCase.path, testCase.body, validKey)
			if testCase.contentType == "" {
				request.Header.Del("Content-Type")
			} else {
				request.Header.Set("Content-Type", testCase.contentType)
			}
			managementTestFragment(t, application, nil, nil).ServeHTTP(recorder, request)
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status/body=%d/%s", recorder.Code, recorder.Body.String())
			}
			if application.totalCalls() != 0 {
				t.Fatalf("invalid body reached application: %+v", application)
			}
			assertSecurityHeaders(t, recorder)
		})
	}

	for _, malformed := range []struct {
		name   string
		body   []byte
		mutate func(*http.Request)
	}{
		{name: "invalid utf8", body: []byte{'{', '"', 'n', 'a', 'm', 'e', '"', ':', '"', 0xff, '"', '}'}},
		{name: "duplicate content type", body: []byte(`{}`), mutate: func(request *http.Request) {
			request.Header.Add("Content-Type", "application/json")
		}},
	} {
		t.Run(malformed.name, func(t *testing.T) {
			application := &fakeManagementApplication{}
			request := httptest.NewRequest(http.MethodPost, "http://membergrid.local/1/member-views", strings.NewReader(string(malformed.body)))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", validKey)
			if malformed.mutate != nil {
				malformed.mutate(request)
			}
			recorder := httptest.NewRecorder()
			managementTestFragment(t, application, nil, nil).ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest || application.totalCalls() != 0 {
				t.Fatalf("status/calls/body=%d/%d/%s", recorder.Code, application.totalCalls(), recorder.Body.String())
			}
		})
	}

	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		application := &fakeManagementApplication{}
		recorder := httptest.NewRecorder()
		body := `{"expected_version":1}`
		if method == http.MethodPut {
			body = `{"expected_version":1,"name":"x","state":"all","sort":"granted_at_desc","columns":["state"]}`
		}
		managementTestFragment(t, application, nil, nil).ServeHTTP(recorder,
			managementRequest(method, "/1/member-views/default", body, "built-in-key-000001"))
		if recorder.Code != http.StatusConflict {
			t.Fatalf("method=%s status/body=%d/%s", method, recorder.Code, recorder.Body.String())
		}
		if application.totalCalls() != 0 {
			t.Fatal("built-in default view reached application")
		}
	}
}

func TestManagementPermissionCSRFAndIdempotencyAreRequiredBeforeMutation(t *testing.T) {
	tests := []struct {
		name          string
		authorizerErr error
		actorID       int64
		csrfErr       error
		keyMode       string
		wantStatus    int
		wantCSRFCalls int
	}{
		{name: "authentication", authorizerErr: ErrAuthenticationRequired, actorID: 17, keyMode: "valid", wantStatus: http.StatusUnauthorized},
		{name: "permission", authorizerErr: ErrPermissionDenied, actorID: 17, keyMode: "valid", wantStatus: http.StatusForbidden},
		{name: "empty actor", actorID: 0, keyMode: "valid", wantStatus: http.StatusUnauthorized},
		{name: "csrf", actorID: 17, csrfErr: errors.New("bad token"), keyMode: "valid", wantStatus: http.StatusForbidden, wantCSRFCalls: 1},
		{name: "missing idempotency", actorID: 17, keyMode: "missing", wantStatus: http.StatusBadRequest, wantCSRFCalls: 1},
		{name: "short idempotency", actorID: 17, keyMode: "short", wantStatus: http.StatusBadRequest, wantCSRFCalls: 1},
		{name: "duplicate idempotency", actorID: 17, keyMode: "duplicate", wantStatus: http.StatusBadRequest, wantCSRFCalls: 1},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			application := &fakeManagementApplication{}
			authorizer := &fakeManagementAuthorizer{actor: ManagementActor{ID: testCase.actorID}, err: testCase.authorizerErr}
			csrf := &fakeManagementCSRF{err: testCase.csrfErr}
			request := managementRequest(http.MethodPost, "/1/member-grid/collaborators",
				`{"expected_version":0,"staff_id":2,"permission":"view"}`, "permission-key-0001")
			switch testCase.keyMode {
			case "missing":
				request.Header.Del("Idempotency-Key")
			case "short":
				request.Header.Set("Idempotency-Key", "short")
			case "duplicate":
				request.Header.Add("Idempotency-Key", strings.Repeat("d", 20))
			}
			recorder := httptest.NewRecorder()
			managementTestFragment(t, application, authorizer, csrf).ServeHTTP(recorder, request)
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status/body=%d/%s", recorder.Code, recorder.Body.String())
			}
			if application.totalCalls() != 0 {
				t.Fatal("failed permission/csrf/idempotency reached application")
			}
			if csrf.calls != testCase.wantCSRFCalls {
				t.Fatalf("csrf calls=%d want=%d", csrf.calls, testCase.wantCSRFCalls)
			}
			if len(authorizer.capabilities) != 1 || authorizer.capabilities[0] != CapabilityProductsWrite {
				t.Fatalf("capabilities=%v", authorizer.capabilities)
			}
		})
	}

	// The read-only share-settings path asks only for products.read and never
	// invokes the CSRF verifier or accepts a write fallback.
	application := &fakeManagementApplication{shareResponse: ShareSettingsResponse{ServiceProductID: 1, SavedViews: []SavedView{}, Collaborators: []Collaborator{}}}
	authorizer := &fakeManagementAuthorizer{actor: ManagementActor{ID: 17}}
	csrf := &fakeManagementCSRF{err: errors.New("must not run")}
	recorder := httptest.NewRecorder()
	managementTestFragment(t, application, authorizer, csrf).ServeHTTP(recorder,
		managementRequest(http.MethodGet, "/1/member-grid/share-settings", "", ""))
	if recorder.Code != http.StatusOK || csrf.calls != 0 || !reflect.DeepEqual(authorizer.capabilities, []string{CapabilityProductsRead}) {
		t.Fatalf("status/csrf/capabilities/body=%d/%d/%v/%s", recorder.Code, csrf.calls, authorizer.capabilities, recorder.Body.String())
	}
}

func TestManagementPathsMethodsAndDomainErrorsFailClosed(t *testing.T) {
	pathCases := []struct {
		path       string
		wantStatus int
	}{
		{path: "/0/member-grid/share-settings", wantStatus: http.StatusBadRequest},
		{path: "/01/member-grid/share-settings", wantStatus: http.StatusBadRequest},
		{path: "/9223372036854775808/member-grid/share-settings", wantStatus: http.StatusBadRequest},
		{path: "/1/member-views/0", wantStatus: http.StatusBadRequest},
		{path: "/1/member-grid/share-settings/extra", wantStatus: http.StatusNotFound},
		{path: "/1/member-grid/share-settings/", wantStatus: http.StatusBadRequest},
		{path: "/1/member-grid/share-settings?x=1", wantStatus: http.StatusBadRequest},
		{path: "/1/member-grid/share-settings?", wantStatus: http.StatusBadRequest},
		{path: "/%31/member-grid/share-settings", wantStatus: http.StatusBadRequest},
		{path: "/1%2F2/member-grid/share-settings", wantStatus: http.StatusBadRequest},
		{path: "/1%5C2/member-grid/share-settings", wantStatus: http.StatusBadRequest},
		{path: "/1//member-grid/share-settings", wantStatus: http.StatusBadRequest},
	}
	for _, testCase := range pathCases {
		t.Run(testCase.path, func(t *testing.T) {
			application := &fakeManagementApplication{}
			recorder := httptest.NewRecorder()
			method := http.MethodGet
			body := ""
			key := ""
			if strings.Contains(testCase.path, "/member-views/0") {
				method = http.MethodDelete
				body = `{"expected_version":1}`
				key = "invalid-path-key-001"
			}
			managementTestFragment(t, application, nil, nil).ServeHTTP(recorder, managementRequest(method, testCase.path, body, key))
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status/body=%d/%s", recorder.Code, recorder.Body.String())
			}
			if application.totalCalls() != 0 {
				t.Fatal("invalid path reached application")
			}
		})
	}

	methodCases := []struct {
		method string
		path   string
		allow  string
	}{
		{method: http.MethodGet, path: "/1/member-views", allow: http.MethodPost},
		{method: http.MethodPost, path: "/1/member-views/2", allow: http.MethodPut + ", " + http.MethodDelete},
		{method: http.MethodPost, path: "/1/member-grid/share-settings", allow: http.MethodGet},
		{method: http.MethodGet, path: "/1/member-grid/collaborators", allow: http.MethodPost},
		{method: http.MethodPost, path: "/1/member-grid/collaborators/2", allow: http.MethodPut + ", " + http.MethodDelete},
	}
	for _, testCase := range methodCases {
		application := &fakeManagementApplication{}
		recorder := httptest.NewRecorder()
		managementTestFragment(t, application, nil, nil).ServeHTTP(recorder, managementRequest(testCase.method, testCase.path, `{}`, "method-key-0000001"))
		if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != testCase.allow || recorder.Body.Len() != 0 {
			t.Fatalf("path=%s status/allow/body=%d/%q/%q", testCase.path, recorder.Code, recorder.Header().Get("Allow"), recorder.Body.String())
		}
		if application.totalCalls() != 0 {
			t.Fatal("method mismatch reached application")
		}
	}

	domainCases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "not found", err: ErrNotFound, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
		{name: "cas", err: ErrConflict, wantStatus: http.StatusConflict, wantCode: "CONFLICT"},
		{name: "inactive staff", err: ErrInactiveStaff, wantStatus: http.StatusUnprocessableEntity, wantCode: "VALIDATION_FAILED"},
		{name: "dependency", err: errors.New("database down"), wantStatus: http.StatusServiceUnavailable, wantCode: "DEPENDENCY_UNAVAILABLE"},
	}
	for _, testCase := range domainCases {
		t.Run(testCase.name, func(t *testing.T) {
			application := &fakeManagementApplication{createCollaboratorErr: testCase.err}
			recorder := httptest.NewRecorder()
			managementTestFragment(t, application, nil, nil).ServeHTTP(recorder,
				managementRequest(http.MethodPost, "/1/member-grid/collaborators",
					`{"expected_version":0,"staff_id":2,"permission":"view"}`, "domain-error-key-001"))
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status/body=%d/%s", recorder.Code, recorder.Body.String())
			}
			var body struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.Code != testCase.wantCode {
				t.Fatalf("decode/code=%v/%q body=%s", err, body.Code, recorder.Body.String())
			}
		})
	}
}
