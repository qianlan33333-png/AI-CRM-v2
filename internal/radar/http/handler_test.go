package http

import (
	"bytes"
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

type applicationSpy struct {
	listInput     radarport.ListInput
	createCommand radarport.CreateCommand
	updateCommand radarport.UpdateCommand
	statusCommand radarport.SetStatusCommand
	getID         radarport.LinkID
	shareID       radarport.LinkID
	listCalls     int
	createCalls   int
	updateCalls   int
	statusCalls   int
	getCalls      int
	shareCalls    int
	optionsCalls  int
	mutationErr   error
}

func (spy *applicationSpy) List(_ context.Context, input radarport.ListInput) (radarport.Page, error) {
	spy.listCalls++
	spy.listInput = input
	return radarport.Page{Items: []radarport.Link{}, Limit: input.Limit, Offset: input.Offset, StatusFilter: input.Status, Sort: input.Sort, LocalProjection: true}, nil
}

func (spy *applicationSpy) Get(_ context.Context, id radarport.LinkID) (radarport.LinkResponse, error) {
	spy.getCalls++
	spy.getID = id
	return sampleResponse(id), nil
}

func (spy *applicationSpy) Create(_ context.Context, command radarport.CreateCommand) (radarport.LinkResponse, error) {
	spy.createCalls++
	spy.createCommand = command
	if spy.mutationErr != nil {
		return radarport.LinkResponse{}, spy.mutationErr
	}
	return sampleResponse(1), nil
}

func (spy *applicationSpy) Update(_ context.Context, command radarport.UpdateCommand) (radarport.LinkResponse, error) {
	spy.updateCalls++
	spy.updateCommand = command
	if spy.mutationErr != nil {
		return radarport.LinkResponse{}, spy.mutationErr
	}
	result := sampleResponse(command.LinkID)
	result.Link.Version = command.ExpectedVersion + 1
	return result, nil
}

func (spy *applicationSpy) SetStatus(_ context.Context, command radarport.SetStatusCommand) (radarport.LinkResponse, error) {
	spy.statusCalls++
	spy.statusCommand = command
	if spy.mutationErr != nil {
		return radarport.LinkResponse{}, spy.mutationErr
	}
	result := sampleResponse(command.LinkID)
	result.Link.Status = command.Target
	result.Link.Version = command.ExpectedVersion + 1
	return result, nil
}

func (spy *applicationSpy) Share(_ context.Context, id radarport.LinkID) (radarport.ShareProjection, error) {
	spy.shareCalls++
	spy.shareID = id
	return radarport.ShareProjection{
		LinkID: id, PublicCode: "rd_AAAAAAAAAAAAAAAAAAAAAA", Status: radarport.StatusDraft,
		Available:       false,
		LocalProjection: true,
	}, nil
}

func (spy *applicationSpy) Options(context.Context) radarport.Options {
	spy.optionsCalls++
	return radarport.Options{
		Statuses:                 []radarport.Status{radarport.StatusDraft, radarport.StatusEnabled, radarport.StatusDisabled},
		StatusFilters:            []radarport.StatusFilter{radarport.StatusFilterAll, radarport.StatusFilterDraft, radarport.StatusFilterEnabled, radarport.StatusFilterDisabled},
		Sorts:                    []radarport.Sort{radarport.SortUpdatedDesc, radarport.SortCreatedDesc, radarport.SortNameAsc},
		DestinationSchemes:       []string{"https"},
		LocalProjection:          true,
		PublicRouteReady:         false,
		RealExternalCallExecuted: false,
	}
}

type authorizerSpy struct {
	actor       Actor
	err         error
	permissions []Permission
}

func (spy *authorizerSpy) Authorize(_ context.Context, permission Permission) (Actor, error) {
	spy.permissions = append(spy.permissions, permission)
	return spy.actor, spy.err
}

type csrfSpy struct {
	calls int
	err   error
}

func (spy *csrfSpy) Verify(*stdhttp.Request) error {
	spy.calls++
	return spy.err
}

func TestRouteFragmentMetadataIsClosed(t *testing.T) {
	fragment := newHTTPFixture(t, &applicationSpy{}, &authorizerSpy{actor: Actor{ID: 1}}, &csrfSpy{})
	got := fragment.Routes()
	want := []Route{
		{Method: "GET", Pattern: BasePath, Permission: PermissionAdminRead},
		{Method: "POST", Pattern: BasePath, Permission: PermissionAdminWrite, RequiresCSRF: true},
		{Method: "GET", Pattern: BasePath + "/{link_id}", Permission: PermissionAdminRead},
		{Method: "PATCH", Pattern: BasePath + "/{link_id}", Permission: PermissionAdminWrite, RequiresCSRF: true},
		{Method: "POST", Pattern: BasePath + "/{link_id}/enable", Permission: PermissionAdminWrite, RequiresCSRF: true},
		{Method: "POST", Pattern: BasePath + "/{link_id}/disable", Permission: PermissionAdminWrite, RequiresCSRF: true},
		{Method: "GET", Pattern: BasePath + "/{link_id}/share", Permission: PermissionAdminRead},
		{Method: "GET", Pattern: BasePath + "/new/options", Permission: PermissionAdminRead},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("routes=%+v", got)
	}
	got[0].Pattern = "mutated"
	if fragment.Routes()[0].Pattern != BasePath {
		t.Fatal("Routes returned mutable internal state")
	}
}

func TestCreateUpdateAndStatusAdaptation(t *testing.T) {
	application := &applicationSpy{}
	authorizer := &authorizerSpy{actor: Actor{ID: 73}}
	csrf := &csrfSpy{}
	fragment := newHTTPFixture(t, application, authorizer, csrf)

	create := httptest.NewRequest(stdhttp.MethodPost, BasePath, strings.NewReader(`{
		"expected_version":0,
		"name":"Guide",
		"title":"Read guide",
		"destination_url":"https://docs.example.com/guide",
		"cover_image_id":9,
		"attachment_id":null
	}`))
	create.Header.Set("Content-Type", "application/json; charset=utf-8")
	create.Header.Set("Idempotency-Key", "radar-http-create-001")
	createRecorder := httptest.NewRecorder()
	fragment.ServeHTTP(createRecorder, create)
	if createRecorder.Code != stdhttp.StatusCreated || application.createCalls != 1 || csrf.calls != 1 || !reflect.DeepEqual(authorizer.permissions, []Permission{PermissionAdminWrite}) {
		t.Fatalf("create status/calls/csrf/permissions=%d/%d/%d/%v body=%s", createRecorder.Code, application.createCalls, csrf.calls, authorizer.permissions, createRecorder.Body.String())
	}
	if application.createCommand.ActorID != 73 || application.createCommand.ExpectedVersion != 0 || application.createCommand.CoverImageID == nil || *application.createCommand.CoverImageID != 9 || application.createCommand.AttachmentID != nil {
		t.Fatalf("create command=%+v", application.createCommand)
	}

	patch := httptest.NewRequest(stdhttp.MethodPatch, BasePath+"/42", strings.NewReader(`{
		"expected_version":1,
		"title":"Updated",
		"cover_image_id":null
	}`))
	patch.Header.Set("Content-Type", "application/json")
	patch.Header.Set("Idempotency-Key", "radar-http-update-001")
	patchRecorder := httptest.NewRecorder()
	fragment.ServeHTTP(patchRecorder, patch)
	if patchRecorder.Code != stdhttp.StatusOK || application.updateCalls != 1 || csrf.calls != 2 {
		t.Fatalf("patch status/calls/csrf=%d/%d/%d body=%s", patchRecorder.Code, application.updateCalls, csrf.calls, patchRecorder.Body.String())
	}
	if application.updateCommand.LinkID != 42 || !application.updateCommand.Title.Set || application.updateCommand.Title.Value != "Updated" || !application.updateCommand.CoverImageID.Set || application.updateCommand.CoverImageID.Value != nil {
		t.Fatalf("update command=%+v", application.updateCommand)
	}

	enable := httptest.NewRequest(stdhttp.MethodPost, BasePath+"/42/enable", strings.NewReader(`{"expected_version":2}`))
	enable.Header.Set("Content-Type", "application/json")
	enable.Header.Set("Idempotency-Key", "radar-http-enable-001")
	enableRecorder := httptest.NewRecorder()
	fragment.ServeHTTP(enableRecorder, enable)
	if enableRecorder.Code != stdhttp.StatusOK || application.statusCalls != 1 || application.statusCommand.Target != radarport.StatusEnabled || application.statusCommand.ActorID != 73 || csrf.calls != 3 {
		t.Fatalf("enable status/command/csrf=%d/%+v/%d body=%s", enableRecorder.Code, application.statusCommand, csrf.calls, enableRecorder.Body.String())
	}
	for _, recorder := range []*httptest.ResponseRecorder{createRecorder, patchRecorder, enableRecorder} {
		if recorder.Header().Get("Cache-Control") != "private, no-store" || recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("headers=%v", recorder.Header())
		}
	}
}

func TestListGetShareAndOptionsUseReadPermission(t *testing.T) {
	application := &applicationSpy{}
	authorizer := &authorizerSpy{actor: Actor{ID: 8}}
	csrf := &csrfSpy{}
	fragment := newHTTPFixture(t, application, authorizer, csrf)

	listRecorder := httptest.NewRecorder()
	fragment.ServeHTTP(listRecorder, httptest.NewRequest(stdhttp.MethodGet, BasePath+"?status=enabled&sort=name_asc&limit=25&offset=5", nil))
	if listRecorder.Code != stdhttp.StatusOK || application.listCalls != 1 || application.listInput != (radarport.ListInput{Status: radarport.StatusFilterEnabled, Sort: radarport.SortNameAsc, Limit: 25, Offset: 5}) {
		t.Fatalf("list status/input=%d/%+v body=%s", listRecorder.Code, application.listInput, listRecorder.Body.String())
	}

	getRecorder := httptest.NewRecorder()
	fragment.ServeHTTP(getRecorder, httptest.NewRequest(stdhttp.MethodGet, BasePath+"/42", nil))
	shareRecorder := httptest.NewRecorder()
	fragment.ServeHTTP(shareRecorder, httptest.NewRequest(stdhttp.MethodGet, BasePath+"/42/share", nil))
	optionsRecorder := httptest.NewRecorder()
	fragment.ServeHTTP(optionsRecorder, httptest.NewRequest(stdhttp.MethodGet, BasePath+"/new/options", nil))
	if getRecorder.Code != stdhttp.StatusOK || shareRecorder.Code != stdhttp.StatusOK || optionsRecorder.Code != stdhttp.StatusOK || application.getID != 42 || application.shareID != 42 || application.optionsCalls != 1 || csrf.calls != 0 {
		t.Fatalf("read statuses=%d/%d/%d ids=%d/%d options=%d csrf=%d", getRecorder.Code, shareRecorder.Code, optionsRecorder.Code, application.getID, application.shareID, application.optionsCalls, csrf.calls)
	}
	if !reflect.DeepEqual(authorizer.permissions, []Permission{PermissionAdminRead, PermissionAdminRead, PermissionAdminRead, PermissionAdminRead}) {
		t.Fatalf("permissions=%v", authorizer.permissions)
	}
	var share radarport.ShareProjection
	if json.Unmarshal(shareRecorder.Body.Bytes(), &share) != nil || share.Available || !share.LocalProjection || share.PublicRouteReady || share.RealExternalCallExecuted || share.SharePath != "" || share.QRPayload != "" {
		t.Fatalf("share body=%s", shareRecorder.Body.String())
	}
	var options radarport.Options
	if json.Unmarshal(optionsRecorder.Body.Bytes(), &options) != nil || !options.LocalProjection || options.PublicRouteReady || options.RealExternalCallExecuted || !reflect.DeepEqual(options.DestinationSchemes, []string{"https"}) {
		t.Fatalf("options body=%s", optionsRecorder.Body.String())
	}
}

func TestStrictDTOContentTypeSizeAndConflictMapping(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		contentType string
		key         string
		wantStatus  int
	}{
		{name: "unknown field", body: `{"expected_version":0,"name":"N","title":"T","destination_url":"https://example.com","unknown":true}`, contentType: "application/json", key: "radar-http-strict-001", wantStatus: stdhttp.StatusBadRequest},
		{name: "case variant field", body: `{"expected_version":0,"Name":"N","title":"T","destination_url":"https://example.com"}`, contentType: "application/json", key: "radar-http-strict-case1", wantStatus: stdhttp.StatusBadRequest},
		{name: "duplicate field", body: `{"expected_version":0,"name":"N","name":"Again","title":"T","destination_url":"https://example.com"}`, contentType: "application/json", key: "radar-http-strict-002", wantStatus: stdhttp.StatusBadRequest},
		{name: "wrong media", body: `{}`, contentType: "text/plain", key: "radar-http-strict-003", wantStatus: stdhttp.StatusUnsupportedMediaType},
		{name: "missing key", body: `{"expected_version":0,"name":"N","title":"T","destination_url":"https://example.com"}`, contentType: "application/json", wantStatus: stdhttp.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			application := &applicationSpy{}
			fragment := newHTTPFixture(t, application, &authorizerSpy{actor: Actor{ID: 1}}, &csrfSpy{})
			request := httptest.NewRequest(stdhttp.MethodPost, BasePath, strings.NewReader(test.body))
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			if test.key != "" {
				request.Header.Set("Idempotency-Key", test.key)
			}
			recorder := httptest.NewRecorder()
			fragment.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus || application.createCalls != 0 {
				t.Fatalf("status/calls=%d/%d body=%s", recorder.Code, application.createCalls, recorder.Body.String())
			}
		})
	}

	application := &applicationSpy{}
	fragment := newHTTPFixture(t, application, &authorizerSpy{actor: Actor{ID: 1}}, &csrfSpy{})
	invalidUTF8Body := append([]byte(`{"expected_version":0,"name":"`), 0xff)
	invalidUTF8Body = append(invalidUTF8Body, []byte(`","title":"T","destination_url":"https://example.com"}`)...)
	invalidUTF8 := httptest.NewRequest(stdhttp.MethodPost, BasePath, bytes.NewReader(invalidUTF8Body))
	invalidUTF8.Header.Set("Content-Type", "application/json")
	invalidUTF8.Header.Set("Idempotency-Key", "radar-http-invalidutf8")
	invalidUTF8Recorder := httptest.NewRecorder()
	fragment.ServeHTTP(invalidUTF8Recorder, invalidUTF8)
	if invalidUTF8Recorder.Code != stdhttp.StatusBadRequest || application.createCalls != 0 {
		t.Fatalf("invalid UTF-8 status/calls=%d/%d body=%s", invalidUTF8Recorder.Code, application.createCalls, invalidUTF8Recorder.Body.String())
	}

	oversized := httptest.NewRequest(stdhttp.MethodPost, BasePath, bytes.NewReader(bytes.Repeat([]byte{'x'}, int(radarport.MaximumRequestBodyBytes)+1)))
	oversized.Header.Set("Content-Type", "application/json")
	oversized.Header.Set("Idempotency-Key", "radar-http-large-0001")
	oversizedRecorder := httptest.NewRecorder()
	fragment.ServeHTTP(oversizedRecorder, oversized)
	if oversizedRecorder.Code != stdhttp.StatusRequestEntityTooLarge || application.createCalls != 0 {
		t.Fatalf("oversized status/calls=%d/%d", oversizedRecorder.Code, application.createCalls)
	}

	application.mutationErr = radarport.ErrConflict
	conflict := httptest.NewRequest(stdhttp.MethodPatch, BasePath+"/1", strings.NewReader(`{"expected_version":1,"name":"Next"}`))
	conflict.Header.Set("Content-Type", "application/json")
	conflict.Header.Set("Idempotency-Key", "radar-http-conflict-01")
	conflictRecorder := httptest.NewRecorder()
	fragment.ServeHTTP(conflictRecorder, conflict)
	if conflictRecorder.Code != stdhttp.StatusConflict {
		t.Fatalf("conflict status=%d body=%s", conflictRecorder.Code, conflictRecorder.Body.String())
	}
}

func TestPermissionAndCSRFFailClosed(t *testing.T) {
	application := &applicationSpy{}
	unauthenticated := newHTTPFixture(t, application, &authorizerSpy{err: ErrUnauthenticated}, &csrfSpy{})
	unauthenticatedRecorder := httptest.NewRecorder()
	unauthenticated.ServeHTTP(unauthenticatedRecorder, httptest.NewRequest(stdhttp.MethodGet, BasePath, nil))
	if unauthenticatedRecorder.Code != stdhttp.StatusUnauthorized || application.listCalls != 0 {
		t.Fatalf("unauth status/calls=%d/%d", unauthenticatedRecorder.Code, application.listCalls)
	}

	forbidden := newHTTPFixture(t, application, &authorizerSpy{err: ErrForbidden}, &csrfSpy{})
	forbiddenRequest := httptest.NewRequest(stdhttp.MethodPost, BasePath, strings.NewReader(`{}`))
	forbiddenRecorder := httptest.NewRecorder()
	forbidden.ServeHTTP(forbiddenRecorder, forbiddenRequest)
	if forbiddenRecorder.Code != stdhttp.StatusForbidden || application.createCalls != 0 {
		t.Fatalf("forbidden status/calls=%d/%d", forbiddenRecorder.Code, application.createCalls)
	}

	csrf := &csrfSpy{err: ErrCSRFInvalid}
	csrfFragment := newHTTPFixture(t, application, &authorizerSpy{actor: Actor{ID: 1}}, csrf)
	csrfRequest := httptest.NewRequest(stdhttp.MethodPost, BasePath, strings.NewReader(`{}`))
	csrfRecorder := httptest.NewRecorder()
	csrfFragment.ServeHTTP(csrfRecorder, csrfRequest)
	if csrfRecorder.Code != stdhttp.StatusForbidden || csrf.calls != 1 || application.createCalls != 0 {
		t.Fatalf("csrf status/calls/app=%d/%d/%d", csrfRecorder.Code, csrf.calls, application.createCalls)
	}
}

func TestMethodsQueriesAndCanonicalIDsFailClosed(t *testing.T) {
	fragment := newHTTPFixture(t, &applicationSpy{}, &authorizerSpy{actor: Actor{ID: 1}}, &csrfSpy{})
	cases := []struct {
		method string
		path   string
		status int
	}{
		{method: stdhttp.MethodDelete, path: BasePath, status: stdhttp.StatusMethodNotAllowed},
		{method: stdhttp.MethodGet, path: BasePath + "/0", status: stdhttp.StatusBadRequest},
		{method: stdhttp.MethodGet, path: BasePath + "/01", status: stdhttp.StatusBadRequest},
		{method: stdhttp.MethodGet, path: BasePath + "/-1", status: stdhttp.StatusBadRequest},
		{method: stdhttp.MethodGet, path: BasePath + "/9223372036854775808", status: stdhttp.StatusBadRequest},
		{method: stdhttp.MethodGet, path: BasePath + "/1/extra", status: stdhttp.StatusNotFound},
		{method: stdhttp.MethodGet, path: BasePath + "?limit=01", status: stdhttp.StatusBadRequest},
		{method: stdhttp.MethodGet, path: BasePath + "?limit=20&limit=30", status: stdhttp.StatusBadRequest},
		{method: stdhttp.MethodGet, path: BasePath + "?unknown=1", status: stdhttp.StatusBadRequest},
	}
	for _, test := range cases {
		recorder := httptest.NewRecorder()
		fragment.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, nil))
		if recorder.Code != test.status {
			t.Errorf("%s %s status=%d want=%d body=%s", test.method, test.path, recorder.Code, test.status, recorder.Body.String())
		}
	}

	encodedRecorder := httptest.NewRecorder()
	fragment.ServeHTTP(encodedRecorder, httptest.NewRequest(stdhttp.MethodGet, BasePath+"/1%2Fshare", nil))
	if encodedRecorder.Code != stdhttp.StatusBadRequest {
		t.Fatalf("encoded status=%d body=%s", encodedRecorder.Code, encodedRecorder.Body.String())
	}
}

func newHTTPFixture(t *testing.T, application radarport.Application, authorizer Authorizer, csrf CSRFVerifier) *RouteFragment {
	t.Helper()
	fragment, err := NewRouteFragment(application, authorizer, csrf)
	if err != nil {
		t.Fatal(err)
	}
	return fragment
}

func sampleResponse(id radarport.LinkID) radarport.LinkResponse {
	created := time.Date(2026, 8, 22, 1, 2, 3, 0, time.UTC)
	return radarport.LinkResponse{
		Link: radarport.Link{
			LinkID:         id,
			PublicCode:     "rd_AAAAAAAAAAAAAAAAAAAAAA",
			Name:           "Guide",
			Title:          "Read guide",
			DestinationURL: "https://example.com/guide",
			Status:         radarport.StatusDraft,
			Version:        1,
			CreatedBy:      1,
			UpdatedBy:      1,
			CreatedAt:      created,
			UpdatedAt:      created,
		},
		LocalProjection: true,
	}
}
