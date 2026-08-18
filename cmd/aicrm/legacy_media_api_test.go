package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

type legacyMediaStub struct {
	result     mediaport.Image
	err        error
	command    mediaport.UploadCommand
	facets     mediaport.ImageFacets
	facetsErr  error
	facetCalls int
	listPage   mediaport.ImageListPage
	listErr    error
	listQuery  mediaport.ImageListQuery
	listCalls  int
}

func (stub *legacyMediaStub) Upload(_ context.Context, command mediaport.UploadCommand) (mediaport.Image, error) {
	stub.command = command
	return stub.result, stub.err
}

func (stub *legacyMediaStub) Facets(context.Context) (mediaport.ImageFacets, error) {
	stub.facetCalls++
	return stub.facets, stub.facetsErr
}

func (stub *legacyMediaStub) ListImages(_ context.Context, query mediaport.ImageListQuery) (mediaport.ImageListPage, error) {
	stub.listCalls++
	stub.listQuery = query
	return stub.listPage, stub.listErr
}

type legacyMediaAuthStub struct {
	principal         authport.Principal
	authenticateErr   error
	authorizeErr      error
	seen              []authport.Capability
	authenticateCalls int
	csrfCalls         int
}

func (stub *legacyMediaAuthStub) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	stub.authenticateCalls++
	if stub.authenticateErr != nil {
		return authport.Principal{}, stub.authenticateErr
	}
	if stub.principal.AdminUserID < 1 {
		return authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}, nil
	}
	return stub.principal, nil
}

func (stub *legacyMediaAuthStub) Authorize(_ context.Context, _ authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	stub.seen = append(stub.seen, capability)
	if stub.authorizeErr != nil {
		return authport.Authorization{}, stub.authorizeErr
	}
	return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
}

func (stub *legacyMediaAuthStub) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	stub.csrfCalls++
	return nil
}

func (*legacyMediaAuthStub) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func TestH01A1LegacyImageUploadPreservesEnvelopeAndMultipartContract(t *testing.T) {
	now := time.Date(2026, 8, 14, 14, 0, 0, 0, time.UTC)
	stub := &legacyMediaStub{result: mediaport.Image{ID: 91, Name: "封面", FileName: "cover.png", FileSize: 73,
		MimeType: "image/png", Width: 1, Height: 1, Description: "说明", Tags: "hero", Category: "cover", CreatedAt: now, UpdatedAt: now}}
	router, auth := legacyMediaRouter(t, stub)
	request := legacyImageRequest(t, "cover.png", "image/png", "media-key-0000001")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if seen := auth.capabilities(); len(seen) != 1 || seen[0] != authport.CapabilityMediaImagesWrite {
		t.Fatalf("capabilities=%v", seen)
	}
	if stub.command.Actor != 1 || stub.command.IdempotencyKey != "media-key-0000001" || stub.command.FileName != "cover.png" || stub.command.DeclaredType != "image/png" || stub.command.Name != "封面" || stub.command.Description != "说明" || stub.command.Tags != "hero" || stub.command.Category != "cover" {
		t.Fatalf("command=%+v", stub.command)
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["ok"] != true || body["source_status"] != "local_upload" || body["route_owner"] != "ai_crm_next" || body["fallback_used"] != false || body["real_external_call_executed"] != false || body["storage_adapter_mode"] != "postgresql" || body["adapter_mode"] != "postgresql" {
		t.Fatalf("envelope=%#v", body)
	}
	item, ok := body["item"].(map[string]any)
	if !ok || item["id"] != float64(91) || item["name"] != "封面" || item["file_name"] != "cover.png" || item["file_size"] != float64(73) {
		t.Fatalf("item=%#v", body["item"])
	}
}

func TestH01A1LegacyImageUploadMintsKeyAndReturnsExactErrorEnvelope(t *testing.T) {
	stub := &legacyMediaStub{err: mediaapp.ErrConflict}
	router, _ := legacyMediaRouter(t, stub)
	request := legacyImageRequest(t, "cover.png", "image/png", "")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || len(stub.command.IdempotencyKey) < 16 {
		t.Fatalf("status/key=%d/%q body=%s", response.Code, stub.command.IdempotencyKey, response.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil || body["ok"] != false || body["error"] != mediaapp.ErrConflict.Error() || body["source_status"] != "next_media_library_error" || body["route_owner"] != "ai_crm_next" || body["fallback_used"] != false || body["real_external_call_executed"] != false {
		t.Fatalf("error envelope=%#v err=%v", body, err)
	}
}

func TestH01A1LegacyImageUploadRequiresSessionCapabilityAndCSRF(t *testing.T) {
	router, _ := legacyMediaRouter(t, &legacyMediaStub{})
	for _, test := range []struct {
		name string
		edit func(*http.Request)
		want int
	}{
		{"missing session", func(r *http.Request) { r.Header.Set("Cookie", "") }, http.StatusUnauthorized},
		{"missing csrf", func(r *http.Request) { r.Header.Del("X-CSRF-Token") }, http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := legacyImageRequest(t, "cover.png", "image/png", "media-key-0000002")
			test.edit(request)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.want, response.Body.String())
			}
		})
	}
}

func TestP4ImageFacetsReturnsExactNineFieldEnvelopeWithoutCSRF(t *testing.T) {
	stub := &legacyMediaStub{facets: mediaport.ImageFacets{
		Categories: []string{"Alpha", "beta", "分类"},
		Tags:       []string{"Alpha", "beta", "标签"},
	}}
	auth := &legacyMediaAuthStub{}
	router := legacyMediaRouterWithAuth(t, stub, auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyImageFacetsRequest(http.MethodGet, true))

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if auth.authenticateCalls != 1 || len(auth.seen) != 1 || auth.seen[0] != authport.CapabilityMediaLibraryRead || auth.csrfCalls != 0 {
		t.Fatalf("authenticate_calls=%d capabilities=%v csrf_calls=%d", auth.authenticateCalls, auth.seen, auth.csrfCalls)
	}
	if stub.facetCalls != 1 {
		t.Fatalf("facet calls=%d", stub.facetCalls)
	}
	body := decodeImageFacetsJSONObject(t, response.Body.Bytes())
	assertExactImageFacetsJSONKeys(t, body, "ok", "categories", "tags", "source_status", "route_owner", "fallback_used", "real_external_call_executed", "storage_adapter_mode", "adapter_mode")
	for _, forbidden := range []string{"item", "error", "message", "detail", "enabled", "url", "object_key", "actor"} {
		if _, exists := body[forbidden]; exists {
			t.Fatalf("forbidden response key %q in %s", forbidden, response.Body.String())
		}
	}
	var payload legacyImageFacetsSuccess
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || payload.SourceStatus != "next_media_library" || payload.RouteOwner != "ai_crm_next" || payload.FallbackUsed || payload.RealExternalCallExecuted || payload.StorageAdapterMode != "postgresql" || payload.AdapterMode != "postgresql" {
		t.Fatalf("payload=%#v", payload)
	}
	if strings.Join(payload.Categories, "|") != "Alpha|beta|分类" || strings.Join(payload.Tags, "|") != "Alpha|beta|标签" {
		t.Fatalf("payload=%#v", payload)
	}
}

func TestP4ImageFacetsReturnsNonNullEmptyArrays(t *testing.T) {
	stub := &legacyMediaStub{}
	router := legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyImageFacetsRequest(http.MethodGet, true))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload legacyImageFacetsSuccess
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Categories == nil || payload.Tags == nil || len(payload.Categories) != 0 || len(payload.Tags) != 0 {
		t.Fatalf("payload=%#v body=%s", payload, response.Body.String())
	}
}

func TestP4ImageFacetsUsesCanonicalSafeInternalError(t *testing.T) {
	stub := &legacyMediaStub{facetsErr: errors.New("pq: internal-marker-0358 actor-marker-77")}
	router := legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyImageFacetsRequest(http.MethodGet, true))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := decodeImageFacetsJSONObject(t, response.Body.Bytes())
	assertExactImageFacetsJSONKeys(t, body, "code", "message", "request_id")
	var code, message, requestID string
	if err := json.Unmarshal(body["code"], &code); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body["message"], &message); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body["request_id"], &requestID); err != nil {
		t.Fatal(err)
	}
	if code != "INTERNAL_ERROR" || message != "An internal error occurred." || requestID == "" {
		t.Fatalf("code=%q message=%q request_id=%q", code, message, requestID)
	}
	for _, forbidden := range []string{"internal-marker-0358", "actor-marker-77", "source_status", "route_owner", "next_media_library_error", `"ok"`} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("leaked %q in %s", forbidden, response.Body.String())
		}
	}
}

func TestP4ImageFacetsRequiresSessionAndReadCapability(t *testing.T) {
	t.Run("missing session", func(t *testing.T) {
		stub := &legacyMediaStub{}
		auth := &legacyMediaAuthStub{}
		router := legacyMediaRouterWithAuth(t, stub, auth)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyImageFacetsRequest(http.MethodGet, false))
		if response.Code != http.StatusUnauthorized || stub.facetCalls != 0 || auth.authenticateCalls != 0 || len(auth.seen) != 0 || auth.csrfCalls != 0 {
			t.Fatalf("status=%d calls=%d authenticate_calls=%d capabilities=%v csrf_calls=%d body=%s", response.Code, stub.facetCalls, auth.authenticateCalls, auth.seen, auth.csrfCalls, response.Body.String())
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		stub := &legacyMediaStub{}
		auth := &legacyMediaAuthStub{authorizeErr: authport.ErrUnauthorized}
		router := legacyMediaRouterWithAuth(t, stub, auth)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyImageFacetsRequest(http.MethodGet, true))
		if response.Code != http.StatusForbidden || stub.facetCalls != 0 || auth.authenticateCalls != 1 || len(auth.seen) != 1 || auth.seen[0] != authport.CapabilityMediaLibraryRead || auth.csrfCalls != 0 {
			t.Fatalf("status=%d calls=%d authenticate_calls=%d capabilities=%v csrf_calls=%d body=%s", response.Code, stub.facetCalls, auth.authenticateCalls, auth.seen, auth.csrfCalls, response.Body.String())
		}
	})
}

func TestP4ImageFacetsMethodMismatchUsesRouter405(t *testing.T) {
	stub := &legacyMediaStub{}
	auth := &legacyMediaAuthStub{}
	router := legacyMediaRouterWithAuth(t, stub, auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyImageFacetsRequest(http.MethodPost, true))
	if response.Code != http.StatusMethodNotAllowed || stub.facetCalls != 0 || auth.authenticateCalls != 0 || len(auth.seen) != 0 || auth.csrfCalls != 0 {
		t.Fatalf("status=%d calls=%d authenticate_calls=%d capabilities=%v csrf_calls=%d body=%s", response.Code, stub.facetCalls, auth.authenticateCalls, auth.seen, auth.csrfCalls, response.Body.String())
	}
}

func TestP4ImageListReturnsExactEnvelopeItemAndQueryWithoutCSRF(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 34, 56, 123456789, time.UTC).Format(time.RFC3339Nano)
	item := mediaport.ImageListItem{
		ID: 42, Name: "封面", FileName: "cover.png", MimeType: "image/png", FileSize: 123,
		Enabled: true, Description: "说明", Tags: []string{"hero", "首页"}, Category: "cover", Width: 640, Height: 480,
		CreatedAt: now, UpdatedAt: now,
		Thumb160URL:   "/api/admin/image-library/42/variants/thumb_160",
		Thumb320URL:   "/api/admin/image-library/42/variants/thumb_320",
		ThumbURL:      "/api/admin/image-library/42/variants/thumb_320",
		PreviewURL:    "/api/admin/image-library/42/variants/mobile_1080",
		Mobile1080URL: "/api/admin/image-library/42/variants/mobile_1080",
		Large1440URL:  "/api/admin/image-library/42/variants/large_1440",
		OriginalURL:   "/api/admin/image-library/42/variants/original",
	}
	stub := &legacyMediaStub{listPage: mediaport.ImageListPage{Items: []mediaport.ImageListItem{item}, Total: 3, Limit: 1, Offset: 0}}
	auth := &legacyMediaAuthStub{}
	router := legacyMediaRouterWithAuth(t, stub, auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyImageListRequest(http.MethodGet,
		"limit=-9&offset=-3&enabled_only=false&q=%20hero%20&category=cover&tags=hero%2C%E9%A6%96%E9%A1%B5&tag_group=hero%2Cbanner&tag_group=%E9%A6%96%E9%A1%B5&only_unlabeled=true", true))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if auth.authenticateCalls != 1 || len(auth.seen) != 1 || auth.seen[0] != authport.CapabilityMediaLibraryRead || auth.csrfCalls != 0 || stub.listCalls != 1 {
		t.Fatalf("authenticate=%d capabilities=%v csrf=%d list=%d", auth.authenticateCalls, auth.seen, auth.csrfCalls, stub.listCalls)
	}
	query := stub.listQuery
	if query.Limit != -9 || query.Offset != -3 || query.EnabledOnly || query.Search != " hero " || query.Category != "cover" ||
		query.Tags != "hero,首页" || !query.OnlyUnlabeled || strings.Join(query.TagGroups, "|") != "hero,banner|首页" {
		t.Fatalf("query=%#v", query)
	}
	body := decodeImageFacetsJSONObject(t, response.Body.Bytes())
	assertExactImageFacetsJSONKeys(t, body, "ok", "items", "total", "limit", "offset", "count", "has_more", "next_offset",
		"source_status", "route_owner", "fallback_used", "real_external_call_executed", "storage_adapter_mode", "adapter_mode")
	var payload legacyImageListSuccess
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || payload.Total != 3 || payload.Limit != 1 || payload.Offset != 0 || payload.Count != 1 || !payload.HasMore ||
		payload.NextOffset == nil || *payload.NextOffset != 1 || payload.SourceStatus != "next_media_library" || payload.RouteOwner != "ai_crm_next" ||
		payload.FallbackUsed || payload.RealExternalCallExecuted || payload.StorageAdapterMode != "postgresql" || payload.AdapterMode != "postgresql" {
		t.Fatalf("payload=%#v", payload)
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(body["items"], &items); err != nil || len(items) != 1 {
		t.Fatalf("items=%v err=%v", items, err)
	}
	assertExactImageFacetsJSONKeys(t, items[0], "id", "name", "file_name", "mime_type", "file_size", "enabled", "description", "tags", "category",
		"width", "height", "created_at", "updated_at", "thumb_160_url", "thumb_320_url", "thumb_url", "preview_url", "mobile_1080_url", "large_1440_url", "original_url")
	for _, forbidden := range []string{"provider", "cache", "base64", "blob", "checksum", "private_url", "raw_url", "source", "source_url", "content_type", "ai_metadata", "created_by", "actor"} {
		if _, exists := items[0][forbidden]; exists {
			t.Fatalf("forbidden item key %q body=%s", forbidden, response.Body.String())
		}
	}
	var tags []string
	if err := json.Unmarshal(items[0]["tags"], &tags); err != nil || tags == nil || strings.Join(tags, "|") != "hero|首页" {
		t.Fatalf("tags=%q err=%v", tags, err)
	}
	for key, want := range map[string]string{
		"created_at": now, "updated_at": now,
		"thumb_160_url": item.Thumb160URL, "thumb_320_url": item.Thumb320URL, "thumb_url": item.ThumbURL,
		"preview_url": item.PreviewURL, "mobile_1080_url": item.Mobile1080URL, "large_1440_url": item.Large1440URL, "original_url": item.OriginalURL,
	} {
		var got string
		if err := json.Unmarshal(items[0][key], &got); err != nil || got != want {
			t.Fatalf("%s=%q want=%q err=%v", key, got, want, err)
		}
	}
}

func TestP4ImageListDefaultsAndNonNullEmptyPage(t *testing.T) {
	stub := &legacyMediaStub{listPage: mediaport.ImageListPage{Items: []mediaport.ImageListItem{}, Total: 0, Limit: 100, Offset: 0}}
	router := legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyImageListRequest(http.MethodGet, "", true))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if stub.listQuery.Limit != 100 || stub.listQuery.Offset != 0 || !stub.listQuery.EnabledOnly || stub.listQuery.Search != "" ||
		stub.listQuery.Category != "" || stub.listQuery.Tags != "" || stub.listQuery.TagGroups == nil || len(stub.listQuery.TagGroups) != 0 || stub.listQuery.OnlyUnlabeled {
		t.Fatalf("query=%#v", stub.listQuery)
	}
	var payload legacyImageListSuccess
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Items == nil || len(payload.Items) != 0 || payload.Count != 0 || payload.HasMore || payload.NextOffset != nil {
		t.Fatalf("payload=%#v body=%s", payload, response.Body.String())
	}
}

func TestP4ImageListAcceptsFrozenQueryLexicalsAndRepeatedGroups(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want mediaport.ImageListQuery
	}{
		{
			name: "explicit defaults and empty strings",
			raw:  "limit=0&offset=0&enabled_only=true&q=&category=&tags=&only_unlabeled=false",
			want: mediaport.ImageListQuery{Limit: 0, Offset: 0, EnabledOnly: true, TagGroups: []string{}},
		},
		{
			name: "int64 boundaries unicode and repeated groups",
			raw:  "limit=-9223372036854775808&offset=9223372036854775807&enabled_only=false&q=%E7%B4%A0%E6%9D%90&category=%E5%B0%81%E9%9D%A2&tags=A%2CB&tag_group=&tag_group=A%2CB&tag_group=%E6%A0%87%E7%AD%BE&only_unlabeled=true",
			want: mediaport.ImageListQuery{Limit: -9223372036854775808, Offset: 9223372036854775807, EnabledOnly: false, Search: "素材", Category: "封面", Tags: "A,B", TagGroups: []string{"", "A,B", "标签"}, OnlyUnlabeled: true},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseLegacyImageListQuery(test.raw)
			if err != nil || got.Limit != test.want.Limit || got.Offset != test.want.Offset || got.EnabledOnly != test.want.EnabledOnly ||
				got.Search != test.want.Search || got.Category != test.want.Category || got.Tags != test.want.Tags ||
				got.OnlyUnlabeled != test.want.OnlyUnlabeled || strings.Join(got.TagGroups, "|") != strings.Join(test.want.TagGroups, "|") || got.TagGroups == nil {
				t.Fatalf("got=%#v want=%#v err=%v", got, test.want, err)
			}
		})
	}
}

func TestP4ImageListRejectsStrictQueryViolationsWithCanonical422(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"empty limit", "limit="}, {"whitespace limit", "limit=%20"}, {"plus limit", "limit=%2B1"},
		{"decimal limit", "limit=1.5"}, {"exponent limit", "limit=1e2"}, {"overflow limit", "limit=9223372036854775808"},
		{"empty offset", "offset="}, {"decimal offset", "offset=0.5"}, {"overflow offset", "offset=-9223372036854775809"},
		{"uppercase enabled", "enabled_only=True"}, {"numeric enabled", "enabled_only=1"}, {"empty enabled", "enabled_only="},
		{"uppercase unlabeled", "only_unlabeled=FALSE"}, {"empty unlabeled", "only_unlabeled="},
		{"duplicate limit", "limit=1&limit=2"}, {"duplicate offset", "offset=1&offset=2"},
		{"duplicate enabled", "enabled_only=true&enabled_only=false"}, {"duplicate q", "q=a&q=b"},
		{"duplicate category", "category=a&category=b"}, {"duplicate tags", "tags=a&tags=b"},
		{"duplicate unlabeled", "only_unlabeled=true&only_unlabeled=false"}, {"unknown key", "future_filter=x"},
		{"malformed percent", "q=%"}, {"malformed utf8 value", "q=%FF"}, {"malformed utf8 key", "%FF=x"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyMediaStub{}
			router := legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyImageListRequest(http.MethodGet, test.raw, true))
			if response.Code != http.StatusUnprocessableEntity || stub.listCalls != 0 {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, stub.listCalls, response.Body.String())
			}
			body := decodeImageFacetsJSONObject(t, response.Body.Bytes())
			assertExactImageFacetsJSONKeys(t, body, "code", "message", "request_id")
			var code, message, requestID string
			if err := json.Unmarshal(body["code"], &code); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(body["message"], &message); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(body["request_id"], &requestID); err != nil {
				t.Fatal(err)
			}
			if code != "VALIDATION_FAILED" || message != "Validation failed." || requestID == "" || strings.Contains(response.Body.String(), "detail") {
				t.Fatalf("code=%q message=%q request_id=%q body=%s", code, message, requestID, response.Body.String())
			}
		})
	}
}

func TestP4ImageListUsesCanonicalSafeInternalError(t *testing.T) {
	stub := &legacyMediaStub{listErr: errors.New("pq: private-sql-marker-0356 actor-marker-77")}
	router := legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyImageListRequest(http.MethodGet, "", true))
	if response.Code != http.StatusInternalServerError || stub.listCalls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, stub.listCalls, response.Body.String())
	}
	body := decodeImageFacetsJSONObject(t, response.Body.Bytes())
	assertExactImageFacetsJSONKeys(t, body, "code", "message", "request_id")
	if strings.Contains(response.Body.String(), "private-sql-marker-0356") || strings.Contains(response.Body.String(), "actor-marker-77") ||
		strings.Contains(response.Body.String(), "source_status") || strings.Contains(response.Body.String(), `"ok"`) {
		t.Fatalf("unsafe body=%s", response.Body.String())
	}
}

func TestP4ImageListRejectsMalformedSuccessProjection(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 34, 56, 0, time.UTC).Format(time.RFC3339Nano)
	base := "/api/admin/image-library/9/variants/"
	valid := mediaport.ImageListItem{
		ID: 9, FileName: "safe.png", MimeType: "image/png", FileSize: 1, Enabled: true, Tags: []string{}, Width: 1, Height: 1,
		CreatedAt: now, UpdatedAt: now, Thumb160URL: base + "thumb_160", Thumb320URL: base + "thumb_320",
		ThumbURL: base + "thumb_320", PreviewURL: base + "mobile_1080", Mobile1080URL: base + "mobile_1080",
		Large1440URL: base + "large_1440", OriginalURL: base + "original",
	}
	tests := []struct {
		name string
		edit func(*mediaport.ImageListItem)
	}{
		{"nil tags", func(item *mediaport.ImageListItem) { item.Tags = nil }},
		{"non RFC3339 timestamp", func(item *mediaport.ImageListItem) { item.CreatedAt = "2026-08-17 12:34:56" }},
		{"absolute raw URL", func(item *mediaport.ImageListItem) { item.OriginalURL = "https://private.invalid/blob" }},
		{"wrong compatibility alias", func(item *mediaport.ImageListItem) { item.ThumbURL = item.Thumb160URL }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := valid
			test.edit(&item)
			stub := &legacyMediaStub{listPage: mediaport.ImageListPage{Items: []mediaport.ImageListItem{item}, Total: 1, Limit: 100, Offset: 0}}
			router := legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyImageListRequest(http.MethodGet, "", true))
			if response.Code != http.StatusInternalServerError || stub.listCalls != 1 {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, stub.listCalls, response.Body.String())
			}
			body := decodeImageFacetsJSONObject(t, response.Body.Bytes())
			assertExactImageFacetsJSONKeys(t, body, "code", "message", "request_id")
			if strings.Contains(response.Body.String(), "private.invalid") || strings.Contains(response.Body.String(), "original_url") || strings.Contains(response.Body.String(), `"items"`) {
				t.Fatalf("unsafe malformed projection response=%s", response.Body.String())
			}
		})
	}
}

func TestP4ImageListRejectsMalformedPageProjection(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		page mediaport.ImageListPage
	}{
		{"wrong effective limit", "limit=501", mediaport.ImageListPage{Items: []mediaport.ImageListItem{}, Total: 0, Limit: 501, Offset: 0}},
		{"wrong effective offset", "offset=-9", mediaport.ImageListPage{Items: []mediaport.ImageListItem{}, Total: 0, Limit: 100, Offset: -9}},
		{"negative total", "", mediaport.ImageListPage{Items: []mediaport.ImageListItem{}, Total: -1, Limit: 100, Offset: 0}},
		{"empty page inside range", "", mediaport.ImageListPage{Items: []mediaport.ImageListItem{}, Total: 1, Limit: 100, Offset: 0}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyMediaStub{listPage: test.page}
			router := legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyImageListRequest(http.MethodGet, test.raw, true))
			if response.Code != http.StatusInternalServerError || stub.listCalls != 1 {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, stub.listCalls, response.Body.String())
			}
			body := decodeImageFacetsJSONObject(t, response.Body.Bytes())
			assertExactImageFacetsJSONKeys(t, body, "code", "message", "request_id")
		})
	}
}

func TestP4ImageListRequiresSessionAndReadCapability(t *testing.T) {
	t.Run("missing session", func(t *testing.T) {
		stub := &legacyMediaStub{}
		auth := &legacyMediaAuthStub{}
		router := legacyMediaRouterWithAuth(t, stub, auth)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyImageListRequest(http.MethodGet, "", false))
		if response.Code != http.StatusUnauthorized || stub.listCalls != 0 || auth.authenticateCalls != 0 || len(auth.seen) != 0 || auth.csrfCalls != 0 {
			t.Fatalf("status=%d list=%d authenticate=%d capabilities=%v csrf=%d body=%s", response.Code, stub.listCalls, auth.authenticateCalls, auth.seen, auth.csrfCalls, response.Body.String())
		}
	})
	t.Run("invalid session", func(t *testing.T) {
		stub := &legacyMediaStub{}
		auth := &legacyMediaAuthStub{authenticateErr: authport.ErrUnauthenticated}
		router := legacyMediaRouterWithAuth(t, stub, auth)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyImageListRequest(http.MethodGet, "", true))
		if response.Code != http.StatusUnauthorized || stub.listCalls != 0 || auth.authenticateCalls != 1 || len(auth.seen) != 0 || auth.csrfCalls != 0 {
			t.Fatalf("status=%d list=%d authenticate=%d capabilities=%v csrf=%d body=%s", response.Code, stub.listCalls, auth.authenticateCalls, auth.seen, auth.csrfCalls, response.Body.String())
		}
	})
	for _, role := range []authport.Role{authport.RoleAdmin, authport.RoleOps} {
		t.Run("authorized "+string(role), func(t *testing.T) {
			stub := &legacyMediaStub{listPage: mediaport.ImageListPage{Items: []mediaport.ImageListItem{}, Total: 0, Limit: 100, Offset: 0}}
			auth := &legacyMediaAuthStub{principal: authport.Principal{AdminUserID: 7, Role: role}}
			router := legacyMediaRouterWithAuth(t, stub, auth)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyImageListRequest(http.MethodGet, "", true))
			if response.Code != http.StatusOK || stub.listCalls != 1 || auth.authenticateCalls != 1 || len(auth.seen) != 1 ||
				auth.seen[0] != authport.CapabilityMediaLibraryRead || auth.csrfCalls != 0 {
				t.Fatalf("status=%d list=%d authenticate=%d capabilities=%v csrf=%d body=%s", response.Code, stub.listCalls, auth.authenticateCalls, auth.seen, auth.csrfCalls, response.Body.String())
			}
		})
	}
	t.Run("sales forbidden", func(t *testing.T) {
		stub := &legacyMediaStub{}
		auth := &legacyMediaAuthStub{principal: authport.Principal{AdminUserID: 8, Role: authport.RoleSales}, authorizeErr: authport.ErrUnauthorized}
		router := legacyMediaRouterWithAuth(t, stub, auth)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyImageListRequest(http.MethodGet, "", true))
		if response.Code != http.StatusForbidden || stub.listCalls != 0 || auth.authenticateCalls != 1 || len(auth.seen) != 1 ||
			auth.seen[0] != authport.CapabilityMediaLibraryRead || auth.csrfCalls != 0 {
			t.Fatalf("status=%d list=%d authenticate=%d capabilities=%v csrf=%d body=%s", response.Code, stub.listCalls, auth.authenticateCalls, auth.seen, auth.csrfCalls, response.Body.String())
		}
	})
}

func TestP4ImageListMethodMismatchUsesRouter405BeforeAuth(t *testing.T) {
	stub := &legacyMediaStub{}
	auth := &legacyMediaAuthStub{}
	router := legacyMediaRouterWithAuth(t, stub, auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyImageListRequest(http.MethodPost, "", true))
	if response.Code != http.StatusMethodNotAllowed || stub.listCalls != 0 || auth.authenticateCalls != 0 || len(auth.seen) != 0 || auth.csrfCalls != 0 {
		t.Fatalf("status=%d list=%d authenticate=%d capabilities=%v csrf=%d body=%s", response.Code, stub.listCalls, auth.authenticateCalls, auth.seen, auth.csrfCalls, response.Body.String())
	}
}

func legacyMediaRouter(t *testing.T, media legacyMediaApplication) (http.Handler, *recordingAuth) {
	t.Helper()
	service := &recordingAuth{}
	return legacyMediaRouterWithAuth(t, media, service), service
}

func legacyMediaRouterWithAuth(t *testing.T, media legacyMediaApplication, service authport.Service) http.Handler {
	return legacyMediaRouterWithAuthAndLogger(t, media, service, slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

func legacyMediaRouterWithAuthAndLogger(t *testing.T, media legacyMediaApplication, service authport.Service, logger *slog.Logger) http.Handler {
	t.Helper()
	legacy, err := NewHandlerWithOutboundProductsAndMedia(service, &legacyCustomerStub{result: legacyCustomerResult()},
		&legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, &legacyProductStub{}, media)
	if err != nil {
		t.Fatal(err)
	}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(logger,
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func legacyImageListRequest(method, rawQuery string, withSession bool) *http.Request {
	request := httptest.NewRequest(method, legacyImageListPath, nil)
	request.URL.RawQuery = rawQuery
	if withSession {
		request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(62)})
	}
	return request
}

func legacyImageFacetsRequest(method string, withSession bool) *http.Request {
	request := httptest.NewRequest(method, legacyImageFacetsPath, nil)
	if withSession {
		request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(61)})
	}
	return request
}

func decodeImageFacetsJSONObject(t *testing.T, encoded []byte) map[string]json.RawMessage {
	t.Helper()
	var body map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func assertExactImageFacetsJSONKeys(t *testing.T, body map[string]json.RawMessage, expected ...string) {
	t.Helper()
	if len(body) != len(expected) {
		t.Fatalf("keys=%v expected=%v", imageFacetsJSONKeys(body), expected)
	}
	for _, key := range expected {
		if _, exists := body[key]; !exists {
			t.Fatalf("missing key %q in %v", key, imageFacetsJSONKeys(body))
		}
	}
}

func imageFacetsJSONKeys(body map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(body))
	for key := range body {
		keys = append(keys, key)
	}
	return keys
}

func legacyImageRequest(t *testing.T, filename, mediaType, key string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header["Content-Disposition"] = []string{`form-data; name="image"; filename="` + filename + `"`}
	header["Content-Type"] = []string{mediaType}
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	pixel := image.NewRGBA(image.Rect(0, 0, 1, 1))
	pixel.Set(0, 0, color.RGBA{R: 8, A: 255})
	if err = png.Encode(part, pixel); err != nil {
		t.Fatal(err)
	}
	for field, value := range map[string]string{"name": "封面", "description": "说明", "tags": "hero", "category": "cover"} {
		if err = writer.WriteField(field, value); err != nil {
			t.Fatal(err)
		}
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/admin/image-library/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-CSRF-Token", legacyToken(52))
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(51)})
	return request
}
