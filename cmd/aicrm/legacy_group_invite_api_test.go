package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

type legacyGroupInviteStub struct {
	item    mediaport.GroupInvite
	page    mediaport.GroupInvitePage
	create  mediaport.GroupInviteCreateCommand
	update  mediaport.GroupInviteUpdateCommand
	archive mediaport.GroupInviteArchiveCommand
	writes  int
	err     error
}

func (stub *legacyGroupInviteStub) List(context.Context, mediaport.GroupInviteListQuery) (mediaport.GroupInvitePage, error) {
	return stub.page, stub.err
}
func (stub *legacyGroupInviteStub) Get(context.Context, int64) (mediaport.GroupInvite, error) {
	return stub.item, stub.err
}
func (stub *legacyGroupInviteStub) Create(_ context.Context, command mediaport.GroupInviteCreateCommand) (mediaport.GroupInvite, error) {
	stub.create, stub.writes = command, stub.writes+1
	return stub.item, stub.err
}
func (stub *legacyGroupInviteStub) Update(_ context.Context, command mediaport.GroupInviteUpdateCommand) (mediaport.GroupInvite, error) {
	stub.update, stub.writes = command, stub.writes+1
	return stub.item, stub.err
}
func (stub *legacyGroupInviteStub) Archive(_ context.Context, command mediaport.GroupInviteArchiveCommand) (mediaport.GroupInvite, error) {
	stub.archive, stub.writes = command, stub.writes+1
	return stub.item, stub.err
}

func TestH03LegacyGroupInviteFiveRoutesRBACCSRFAndLocalOnlyEnvelope(t *testing.T) {
	item := legacyGroupInviteItem()
	stub := &legacyGroupInviteStub{item: item, page: mediaport.GroupInvitePage{Items: []mediaport.GroupInvite{item}, Total: 1, Limit: 100}}
	router, auth := legacyGroupInviteRouter(t, stub)

	for _, path := range []string{"/api/admin/group-invite-library", "/api/admin/group-invite-library/7"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(91)))
		if response.Code != http.StatusOK || response.Body.String() == "" {
			t.Fatalf("GET %s=%d %s", path, response.Code, response.Body.String())
		}
	}
	if capabilities := auth.capabilities(); len(capabilities) != 2 || capabilities[0] != authport.CapabilityMediaLibraryRead || capabilities[1] != authport.CapabilityMediaLibraryRead {
		t.Fatalf("read capabilities=%v", capabilities)
	}
	auth.reset()
	body := `{"name":"体验群","title":"加入体验群","description":"点击卡片入群","join_url":"https://work.weixin.qq.com/gm/safe-token","cover_image_id":19,"enabled":true}`
	for _, test := range []struct{ method, path string }{
		{http.MethodPost, "/api/admin/group-invite-library"},
		{http.MethodPut, "/api/admin/group-invite-library/7"},
		{http.MethodDelete, "/api/admin/group-invite-library/7"},
	} {
		response := httptest.NewRecorder()
		requestBody := body
		if test.method == http.MethodDelete {
			requestBody = ""
		}
		router.ServeHTTP(response, legacyChannelWriteRequest(test.method, test.path, requestBody))
		if response.Code != http.StatusOK || !containsAll(response.Body.String(), `"local_only":true`, `"provider_call_executed":false`, `"real_external_call_executed":false`) {
			t.Fatalf("%s %s=%d %s", test.method, test.path, response.Code, response.Body.String())
		}
	}
	if capabilities := auth.capabilities(); len(capabilities) != 3 || capabilities[0] != authport.CapabilityMediaLibraryWrite || capabilities[1] != authport.CapabilityMediaLibraryWrite || capabilities[2] != authport.CapabilityMediaLibraryWrite {
		t.Fatalf("write capabilities=%v", capabilities)
	}
	if stub.create.Actor != 1 || stub.create.Title != "加入体验群" || stub.create.CoverImageID != 19 || stub.update.ID != 7 || stub.archive.ID != 7 || stub.archive.IdempotencyKey != "legacy-group-invite:archive:7" {
		t.Fatalf("commands create=%#v update=%#v archive=%#v", stub.create, stub.update, stub.archive)
	}
}

func TestH03LegacyGroupInviteRejectsUnknownFieldsBadQueryAndMissingCSRF(t *testing.T) {
	stub := &legacyGroupInviteStub{item: legacyGroupInviteItem()}
	router, _ := legacyGroupInviteRouter(t, stub)
	for _, request := range []*http.Request{
		legacyChannelWriteRequest(http.MethodPost, "/api/admin/group-invite-library", `{"title":"入群","join_url":"https://work.weixin.qq.com/gm/safe","config_id":"forbidden"}`),
		legacyRequest(http.MethodGet, "/api/admin/group-invite-library?provider_state=ready", legacyToken(92)),
		legacyChannelWriteRequest(http.MethodPut, "/api/admin/group-invite-library/7", `{}`),
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || stub.writes != 0 {
			t.Fatalf("bad request=%d writes=%d body=%s", response.Code, stub.writes, response.Body.String())
		}
	}
	missingCSRF := legacyChannelWriteRequest(http.MethodDelete, "/api/admin/group-invite-library/7", "")
	missingCSRF.Header.Set("Origin", "https://cross.invalid")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, missingCSRF)
	if response.Code != http.StatusForbidden || stub.writes != 0 {
		t.Fatalf("missing csrf=%d writes=%d", response.Code, stub.writes)
	}
}

func TestH03LegacyGroupInviteErrorClassification(t *testing.T) {
	for _, test := range []struct {
		err  error
		want int
	}{
		{mediaapp.ErrGroupInviteInvalidReference, http.StatusBadRequest},
		{mediaapp.ErrGroupInviteNotFound, http.StatusNotFound},
		{mediaapp.ErrGroupInviteConflict, http.StatusConflict},
		{mediaapp.ErrGroupInviteUnavailable, http.StatusServiceUnavailable},
	} {
		stub := &legacyGroupInviteStub{err: test.err}
		router, _ := legacyGroupInviteRouter(t, stub)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/group-invite-library/7", legacyToken(93)))
		if response.Code != test.want || !containsAll(response.Body.String(), `"provider_call_executed":false`, `"real_external_call_executed":false`) {
			t.Fatalf("err=%v status=%d body=%s", test.err, response.Code, response.Body.String())
		}
	}
}

func legacyGroupInviteRouter(t *testing.T, groupInvites groupInviteApplication) (http.Handler, *recordingAuth) {
	t.Helper()
	service := &recordingAuth{}
	legacy, err := NewHandlerWithOutboundProductsMediaAndSurvey(service, &legacyCustomerStub{result: legacyCustomerResult()},
		&legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, &legacyProductStub{}, &legacyMediaStub{}, &legacySurveyStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.groupInvites = groupInvites
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router, service
}

func legacyGroupInviteItem() mediaport.GroupInvite {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	return mediaport.GroupInvite{ID: 7, Name: "体验群", Title: "加入体验群", Description: "点击卡片入群",
		JoinURL: "https://work.weixin.qq.com/gm/safe-token", CoverImageID: 19, Enabled: true,
		CreatedBy: 1, UpdatedBy: 1, Version: 1, CreatedAt: now, UpdatedAt: now}
}

func containsAll(value string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(value, needle) {
			return false
		}
	}
	return true
}
