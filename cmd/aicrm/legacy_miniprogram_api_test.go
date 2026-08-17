package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

type legacyMiniProgramStub struct {
	item       mediaport.MiniProgram
	page       mediaport.MiniProgramPage
	mutation   mediaport.MiniProgramMutationResult
	resolution mediaport.MiniProgramThumbnailResolutionResult
	list       mediaport.MiniProgramListQuery
	create     mediaport.MiniProgramCreateCommand
	update     mediaport.MiniProgramUpdateCommand
	delete     mediaport.MiniProgramDeleteCommand
	resolve    mediaport.MiniProgramResolveThumbnailCommand
	writes     int
	err        error
}

func (stub *legacyMiniProgramStub) List(_ context.Context, query mediaport.MiniProgramListQuery) (mediaport.MiniProgramPage, error) {
	stub.list = query
	return stub.page, stub.err
}
func (stub *legacyMiniProgramStub) Get(context.Context, int64) (mediaport.MiniProgram, error) {
	return stub.item, stub.err
}
func (stub *legacyMiniProgramStub) Create(_ context.Context, command mediaport.MiniProgramCreateCommand) (mediaport.MiniProgramMutationResult, error) {
	stub.create, stub.writes = command, stub.writes+1
	return stub.mutation, stub.err
}
func (stub *legacyMiniProgramStub) Update(_ context.Context, command mediaport.MiniProgramUpdateCommand) (mediaport.MiniProgramMutationResult, error) {
	stub.update, stub.writes = command, stub.writes+1
	return stub.mutation, stub.err
}
func (stub *legacyMiniProgramStub) Delete(_ context.Context, command mediaport.MiniProgramDeleteCommand) (mediaport.MiniProgramDeleteResult, error) {
	stub.delete, stub.writes = command, stub.writes+1
	return mediaport.MiniProgramDeleteResult{ID: command.ID, Deleted: true}, stub.err
}
func (stub *legacyMiniProgramStub) ResolveThumbnail(_ context.Context, command mediaport.MiniProgramResolveThumbnailCommand) (mediaport.MiniProgramThumbnailResolutionResult, error) {
	stub.resolve, stub.writes = command, stub.writes+1
	return stub.resolution, stub.err
}

func TestMiniProgramCarrierAndSixAPIRoutesKeepRBACCSRFAndLocalOnly(t *testing.T) {
	item := legacyMiniProgramItem()
	resolution := mediaport.ThumbnailCacheResolution{Status: mediaport.ThumbnailNotAvailable, CacheOwner: mediaport.ThumbnailCacheOwner, CacheReceipt: "local-cache:miss"}
	stub := &legacyMiniProgramStub{item: item, page: mediaport.MiniProgramPage{Items: []mediaport.MiniProgram{item}, Total: 1, Limit: 100},
		mutation:   mediaport.MiniProgramMutationResult{Item: item, Changed: true},
		resolution: mediaport.MiniProgramThumbnailResolutionResult{Item: item, Resolution: resolution}}
	router, auth := legacyMiniProgramRouter(t, stub)

	carrier := httptest.NewRecorder()
	router.ServeHTTP(carrier, legacyRequest(http.MethodGet, "/admin/miniprogram-library", legacyToken(95)))
	if carrier.Code != http.StatusFound || carrier.Header().Get("Location") != "/?legacy_admin_path=%2Fadmin%2Fminiprogram-library" {
		t.Fatalf("carrier=%d location=%q", carrier.Code, carrier.Header().Get("Location"))
	}
	for _, path := range []string{"/api/admin/miniprogram-library?limit=200&offset=0&enabled_only=false&q=wx", "/api/admin/miniprogram-library/7"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(96)))
		if response.Code != http.StatusOK || !containsAll(response.Body.String(), `"local_only":true`, `"provider_call_executed":false`, `"real_external_call_executed":false`) {
			t.Fatalf("GET %s=%d %s", path, response.Code, response.Body.String())
		}
	}
	if stub.list.Limit != 200 || stub.list.Offset != 0 || stub.list.EnabledOnly || stub.list.Search != "wx" {
		t.Fatalf("list query=%#v", stub.list)
	}
	if capabilities := auth.capabilities(); len(capabilities) != 3 ||
		capabilities[0] != authport.CapabilityMediaLibraryRead ||
		capabilities[1] != authport.CapabilityMediaLibraryRead ||
		capabilities[2] != authport.CapabilityMediaLibraryRead {
		t.Fatalf("read capabilities=%v", capabilities)
	}

	auth.reset()
	create := legacyChannelWriteRequest(http.MethodPost, "/api/admin/miniprogram-library",
		`{"name":"卡片","app_id":"wx-demo","page_path":"pages/home","title":"首页","thumb_image_id":11,"resolve_thumb_media":false}`)
	update := legacyChannelWriteRequest(http.MethodPut, "/api/admin/miniprogram-library/7", `{"name":"","thumb_image_id":null}`)
	remove := legacyChannelWriteRequest(http.MethodDelete, "/api/admin/miniprogram-library/7", "")
	resolve := legacyChannelWriteRequest(http.MethodPost, "/api/admin/miniprogram-library/7/test-resolve", "")
	for _, request := range []*http.Request{create, update, remove, resolve} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !containsAll(response.Body.String(), `"local_only":true`, `"provider_call_executed":false`, `"real_external_call_executed":false`) {
			t.Fatalf("%s %s=%d %s", request.Method, request.URL.Path, response.Code, response.Body.String())
		}
	}
	if stub.create.Actor != 1 || stub.create.AppID != "wx-demo" || stub.create.PagePath != "pages/home" ||
		stub.create.ResolveThumbMedia == nil || *stub.create.ResolveThumbMedia || stub.update.ID != 7 ||
		stub.update.Name == nil || *stub.update.Name != "" || !stub.update.ThumbnailImageID.Present || stub.update.ThumbnailImageID.Value != nil {
		t.Fatalf("commands create=%#v update=%#v", stub.create, stub.update)
	}
	if stub.delete.IdempotencyKey != "legacy-miniprogram:delete:7" || stub.resolve.IdempotencyKey != "legacy-miniprogram:test-resolve:7" {
		t.Fatalf("deterministic keys delete=%q resolve=%q", stub.delete.IdempotencyKey, stub.resolve.IdempotencyKey)
	}
	if capabilities := auth.capabilities(); len(capabilities) != 4 {
		t.Fatalf("write capabilities=%v", capabilities)
	} else {
		for _, capability := range capabilities {
			if capability != authport.CapabilityMediaLibraryWrite {
				t.Fatalf("write capability=%v", capabilities)
			}
		}
	}
}

func TestMiniProgramHTTPRejectsUnknownFieldsBadQueriesEmptyPatchAndMissingCSRF(t *testing.T) {
	stub := &legacyMiniProgramStub{item: legacyMiniProgramItem()}
	router, _ := legacyMiniProgramRouter(t, stub)
	for _, request := range []*http.Request{
		legacyChannelWriteRequest(http.MethodPost, "/api/admin/miniprogram-library", `{"appid":"wx","pagepath":"p","title":"t","provider_media_id":"forbidden"}`),
		legacyRequest(http.MethodGet, "/api/admin/miniprogram-library?provider_state=ready", legacyToken(97)),
		legacyRequest(http.MethodGet, "/api/admin/miniprogram-library?limit=invalid", legacyToken(98)),
		legacyChannelWriteRequest(http.MethodPut, "/api/admin/miniprogram-library/7", `{}`),
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || stub.writes != 0 {
			t.Fatalf("bad request=%s %s status=%d writes=%d body=%s", request.Method, request.URL, response.Code, stub.writes, response.Body.String())
		}
	}
	missingCSRF := legacyChannelWriteRequest(http.MethodPost, "/api/admin/miniprogram-library/7/test-resolve", "")
	missingCSRF.Header.Set("Origin", "https://cross.invalid")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, missingCSRF)
	if response.Code != http.StatusForbidden || stub.writes != 0 {
		t.Fatalf("missing csrf=%d writes=%d body=%s", response.Code, stub.writes, response.Body.String())
	}
}

func TestMiniProgramHTTPPreservesThumbMediaPresenceForApplicationRejection(t *testing.T) {
	stub := &legacyMiniProgramStub{mutation: mediaport.MiniProgramMutationResult{Item: legacyMiniProgramItem()}}
	router, _ := legacyMiniProgramRouter(t, stub)
	for _, body := range []string{`{"appid":"wx","pagepath":"p","title":"t","thumb_media_id":"client"}`, `{"appid":"wx","pagepath":"p","title":"t","thumb_media_id":null}`} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyChannelWriteRequest(http.MethodPost, "/api/admin/miniprogram-library", body))
		if response.Code != http.StatusOK || !stub.create.ThumbMediaID.Present {
			t.Fatalf("body=%s status=%d command=%#v response=%s", body, response.Code, stub.create, response.Body.String())
		}
	}
}

func TestMiniProgramHTTPErrorClassification(t *testing.T) {
	for _, test := range []struct {
		err  error
		want int
	}{
		{mediaapp.ErrInvalidMiniProgramOperation, http.StatusBadRequest},
		{mediaapp.ErrMiniProgramImageNotFound, http.StatusBadRequest},
		{mediaapp.ErrMiniProgramNotFound, http.StatusNotFound},
		{mediaapp.ErrMiniProgramConflict, http.StatusConflict},
		{mediaapp.ErrMiniProgramUnavailable, http.StatusServiceUnavailable},
	} {
		stub := &legacyMiniProgramStub{err: test.err}
		router, _ := legacyMiniProgramRouter(t, stub)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/miniprogram-library/7", legacyToken(99)))
		if response.Code != test.want || !containsAll(response.Body.String(), `"provider_call_executed":false`, `"real_external_call_executed":false`) {
			t.Fatalf("err=%v status=%d body=%s", test.err, response.Code, response.Body.String())
		}
	}
}

func legacyMiniProgramRouter(t *testing.T, miniPrograms miniProgramApplication) (http.Handler, *recordingAuth) {
	t.Helper()
	service := &recordingAuth{}
	legacy, err := NewHandlerWithOutboundProductsMediaAndSurvey(service, &legacyCustomerStub{result: legacyCustomerResult()},
		&legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, &legacyProductStub{}, &legacyMediaStub{}, &legacySurveyStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.miniPrograms = miniPrograms
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

func legacyMiniProgramItem() mediaport.MiniProgram {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	imageID := int64(11)
	return mediaport.MiniProgram{ID: 7, Name: "卡片", AppID: "wx-demo", PagePath: "pages/home", Title: "首页",
		ThumbnailImageID: &imageID, Enabled: true, CreatedBy: 1, UpdatedBy: 1, Version: 1, CreatedAt: now, UpdatedAt: now}
}
