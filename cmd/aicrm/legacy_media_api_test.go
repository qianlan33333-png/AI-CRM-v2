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
}

func (stub *legacyMediaStub) Upload(_ context.Context, command mediaport.UploadCommand) (mediaport.Image, error) {
	stub.command = command
	return stub.result, stub.err
}

func (stub *legacyMediaStub) Facets(context.Context) (mediaport.ImageFacets, error) {
	stub.facetCalls++
	return stub.facets, stub.facetsErr
}

type legacyMediaAuthStub struct {
	authorizeErr      error
	seen              []authport.Capability
	authenticateCalls int
	csrfCalls         int
}

func (stub *legacyMediaAuthStub) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	stub.authenticateCalls++
	return authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}, nil
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

func legacyMediaRouter(t *testing.T, media legacyMediaApplication) (http.Handler, *recordingAuth) {
	t.Helper()
	service := &recordingAuth{}
	return legacyMediaRouterWithAuth(t, media, service), service
}

func legacyMediaRouterWithAuth(t *testing.T, media legacyMediaApplication, service authport.Service) http.Handler {
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
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
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
