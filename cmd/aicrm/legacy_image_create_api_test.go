package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

type legacyImageCreateStub struct {
	legacyMediaStub
	calls int
}

func (stub *legacyImageCreateStub) Upload(ctx context.Context, command mediaport.UploadCommand) (mediaport.Image, error) {
	stub.calls++
	return stub.legacyMediaStub.Upload(ctx, command)
}

func TestP4ImageCreateWritesSafeProjectionAndCanonicalCommand(t *testing.T) {
	stamp := time.Date(2026, 8, 19, 2, 3, 4, 0, time.UTC)
	stub := &legacyImageCreateStub{legacyMediaStub: legacyMediaStub{result: mediaport.Image{
		ID: 42, Name: "封面", FileName: "cover.png", FileSize: 70, MimeType: "image/png", Width: 1, Height: 1,
		Enabled: false, Description: "说明", Tags: "hero,首页", Category: "cover", CreatedAt: stamp, UpdatedAt: stamp,
	}}}
	auth := &legacyMediaAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleOps}}
	request := legacyImageCreateRequest(t, http.MethodPost, map[string]any{
		"data_url": legacyCreatePNGDataURL(t), "file_name": "cover.png", "name": " 封面 ",
		"tags": []string{" hero ", "hero", "首页"}, "description": " 说明 ", "category": " cover ", "enabled": false,
	}, true, true)
	response := httptest.NewRecorder()
	legacyMediaRouterWithAuth(t, stub, auth).ServeHTTP(response, request)
	if response.Code != http.StatusOK || stub.calls != 1 || stub.command.Actor != 7 || stub.command.Name != "封面" || stub.command.Description != "说明" || stub.command.Category != "cover" || stub.command.Tags != "hero,首页" || stub.command.Enabled == nil || *stub.command.Enabled {
		t.Fatalf("status=%d calls=%d command=%+v body=%s", response.Code, stub.calls, stub.command, response.Body.String())
	}
	if auth.authenticateCalls != 1 || len(auth.seen) != 1 || auth.seen[0] != authport.CapabilityMediaImagesWrite || auth.csrfCalls != 1 || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("auth=%+v headers=%v", auth, response.Header())
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	assertExactLegacyImageCreateKeys(t, body, "ok", "item", "source_status", "route_owner", "fallback_used", "real_external_call_executed", "storage_adapter_mode", "adapter_mode")
	var item map[string]json.RawMessage
	if err := json.Unmarshal(body["item"], &item); err != nil {
		t.Fatal(err)
	}
	assertExactLegacyImageCreateKeys(t, item, "id", "name", "file_name", "file_size", "mime_type", "width", "height", "enabled", "description", "tags", "category", "created_at", "updated_at")
	for _, forbidden := range []string{"data_url", "base64", "checksum", "provider", "storage", "adapter", "created_by", "raw"} {
		if _, found := item[forbidden]; found {
			t.Fatalf("leaked %q: %s", forbidden, response.Body.String())
		}
	}
	if string(item["enabled"]) != "false" || string(item["tags"]) != `["hero","首页"]` || string(body["source_status"]) != `"local_repository_write"` {
		t.Fatalf("body=%s", response.Body.String())
	}
}

func TestP4ImageCreateRejectsUnsafeInputAndStrictMethodBeforeApp(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{"unknown", `{"data_url":"data:image/png;base64,ZmFrZQ==","file_name":"a.png","provider_id":"x"}`},
		{"duplicate", `{"data_url":"data:image/png;base64,ZmFrZQ==","data_url":"data:image/png;base64,ZmFrZQ==","file_name":"a.png"}`},
		{"fake legacy", `{"data_url":"data:image/png;base64,ZmFrZQ==","file_name":"a.png"}`},
		{"uppercase prefix", `{"data_url":"data:image/PNG;base64,ZmFrZQ==","file_name":"a.png"}`},
		{"alternate base64", `{"data_url":"data:image/png;base64,____","file_name":"a.png"}`},
		{"missing key", `{"data_url":"data:image/png;base64,ZmFrZQ==","file_name":"a.png"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyImageCreateStub{legacyMediaStub: legacyMediaStub{err: mediaapp.ErrInvalidUpload}}
			request := legacyImageCreateRawRequest(http.MethodPost, test.body, true, true, test.name != "missing key")
			response := httptest.NewRecorder()
			legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{}).ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || stub.calls != 0 || !strings.Contains(response.Body.String(), `"code":"MALFORMED_REQUEST"`) {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, stub.calls, response.Body.String())
			}
		})
	}
	stub := &legacyImageCreateStub{}
	auth := &legacyMediaAuthStub{}
	response := httptest.NewRecorder()
	legacyMediaRouterWithAuth(t, stub, auth).ServeHTTP(response, legacyImageCreateRawRequest(http.MethodPatch, `{}`, true, false, false))
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "GET, POST" || stub.calls != 0 || auth.authenticateCalls != 0 || len(auth.seen) != 0 || auth.csrfCalls != 0 {
		t.Fatalf("status=%d allow=%q calls=%d auth=%+v", response.Code, response.Header().Get("Allow"), stub.calls, auth)
	}
}

func TestP4ImageCreateRequiresCanonicalWireImageAndSafeMetadataBeforeApp(t *testing.T) {
	pngURL := legacyCreatePNGDataURL(t)
	pngPayload := strings.TrimPrefix(pngURL, "data:image/png;base64,")
	longTag := strings.Repeat("t", 65)
	tooManyTags := make([]string, 51)
	for index := range tooManyTags {
		tooManyTags[index] = string(rune('a'+index%26)) + string(rune('A'+index/26))
	}
	for _, test := range []struct {
		name     string
		dataURL  string
		fileName string
		tags     any
	}{
		{"data-url parameter", "data:image/png;charset=utf-8;base64," + pngPayload, "safe.png", []string{}},
		{"data-url whitespace", "data:image/png;base64," + pngPayload[:8] + "\n" + pngPayload[8:], "safe.png", []string{}},
		{"data-url escaped", "data:image/png;base64," + strings.ReplaceAll(pngPayload, "=", "%3D"), "safe.png", []string{}},
		{"data-url declared mismatch", "data:image/jpeg;base64," + pngPayload, "safe.jpeg", []string{}},
		{"data-url truncated", "data:image/png;base64," + pngPayload[:len(pngPayload)-4], "safe.png", []string{}},
		{"filename path", pngURL, "../safe.png", []string{}},
		{"filename control", pngURL, "safe\u0000.png", []string{}},
		{"tag too long", pngURL, "safe.png", []string{longTag}},
		{"tag comma", pngURL, "safe.png", []string{"one,two"}},
		{"too many tags", pngURL, "safe.png", tooManyTags},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyImageCreateStub{}
			request := legacyImageCreateRequest(t, http.MethodPost, map[string]any{"data_url": test.dataURL, "file_name": test.fileName, "tags": test.tags}, true, true)
			response := httptest.NewRecorder()
			legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{}).ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || stub.calls != 0 {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, stub.calls, response.Body.String())
			}
		})
	}
}

func TestP4ImageCreateAcceptsCanonicalPNGJPEGAndGIF(t *testing.T) {
	for _, test := range []struct {
		name, mediaType, fileName string
		encode                    func(*bytes.Buffer, image.Image) error
	}{
		{"png", "image/png", "safe.png", func(buffer *bytes.Buffer, value image.Image) error { return png.Encode(buffer, value) }},
		{"jpeg", "image/jpeg", "safe.jpeg", func(buffer *bytes.Buffer, value image.Image) error { return jpeg.Encode(buffer, value, nil) }},
		{"gif", "image/gif", "safe.gif", func(buffer *bytes.Buffer, value image.Image) error { return gif.Encode(buffer, value, nil) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var content bytes.Buffer
			value := image.NewRGBA(image.Rect(0, 0, 1, 1))
			value.Set(0, 0, color.RGBA{R: 7, G: 8, B: 9, A: 255})
			if err := test.encode(&content, value); err != nil {
				t.Fatal(err)
			}
			stub := &legacyImageCreateStub{legacyMediaStub: legacyMediaStub{result: mediaport.Image{ID: 1, FileName: test.fileName, MimeType: test.mediaType, Enabled: true}}}
			request := legacyImageCreateRequest(t, http.MethodPost, map[string]any{"data_url": "data:" + test.mediaType + ";base64," + base64.StdEncoding.EncodeToString(content.Bytes()), "file_name": test.fileName}, true, true)
			response := httptest.NewRecorder()
			legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{}).ServeHTTP(response, request)
			if response.Code != http.StatusOK || stub.calls != 1 || stub.command.DeclaredType != test.mediaType {
				t.Fatalf("status=%d calls=%d command=%+v body=%s", response.Code, stub.calls, stub.command, response.Body.String())
			}
		})
	}
}

func TestP4ImageCreateAuthCSRFDenialsAndErrorsNeverLeakOrInvokeUnexpectedly(t *testing.T) {
	for _, test := range []struct {
		name                  string
		withSession, withCSRF bool
		auth                  *legacyMediaAuthStub
		wantStatus            int
		wantCalls             int
	}{
		{"no session", false, true, &legacyMediaAuthStub{}, http.StatusUnauthorized, 0},
		{"no csrf", true, false, &legacyMediaAuthStub{}, http.StatusForbidden, 0},
		{"authorization denied", true, true, &legacyMediaAuthStub{authorizeErr: authport.ErrUnauthorized}, http.StatusForbidden, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyImageCreateStub{}
			response := httptest.NewRecorder()
			legacyMediaRouterWithAuth(t, stub, test.auth).ServeHTTP(response, legacyImageCreateRequest(t, http.MethodPost, map[string]any{"data_url": legacyCreatePNGDataURL(t), "file_name": "safe.png"}, test.withSession, test.withCSRF))
			if response.Code != test.wantStatus || stub.calls != test.wantCalls || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("status=%d calls=%d headers=%v body=%s", response.Code, stub.calls, response.Header(), response.Body.String())
			}
		})
	}
	for _, test := range []struct {
		name string
		err  error
		code int
		want string
	}{
		{"conflict", mediaapp.ErrConflict, http.StatusConflict, "CONFLICT"},
		{"unavailable", errors.New("storage unavailable"), http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyImageCreateStub{legacyMediaStub: legacyMediaStub{err: test.err}}
			response := httptest.NewRecorder()
			legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{}).ServeHTTP(response, legacyImageCreateRequest(t, http.MethodPost, map[string]any{"data_url": legacyCreatePNGDataURL(t), "file_name": "safe.png"}, true, true))
			var body map[string]json.RawMessage
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != test.code || stub.calls != 1 || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("status=%d calls=%d headers=%v body=%s", response.Code, stub.calls, response.Header(), response.Body.String())
			}
			assertExactLegacyImageCreateKeys(t, body, "code", "message", "request_id")
			if string(body["code"]) != `"`+test.want+`"` {
				t.Fatalf("body=%s", response.Body.String())
			}
		})
	}
}

func TestP4ImageCreateAuthenticationPrecedesMissingDependency(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, legacyImageCollectionPath, strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	(&Handler{}).CreateImage(response, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"code":"UNAUTHORIZED"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func legacyImageCreateRequest(t *testing.T, method string, payload any, withSession, withCSRF bool) *http.Request {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return legacyImageCreateRawRequest(method, string(body), withSession, withCSRF, true)
}

func legacyImageCreateRawRequest(method, body string, withSession, withCSRF, withKey bool) *http.Request {
	request := httptest.NewRequest(method, legacyImageCollectionPath, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	if withSession {
		request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(91)})
	}
	if withCSRF {
		request.Header.Set("X-CSRF-Token", legacyToken(92))
	}
	if withKey {
		request.Header.Set("Idempotency-Key", "image-create-key-0001")
	}
	return request
}

func legacyCreatePNGDataURL(t *testing.T) string {
	t.Helper()
	var imageBytes bytes.Buffer
	pixel := image.NewRGBA(image.Rect(0, 0, 1, 1))
	pixel.Set(0, 0, color.RGBA{R: 7, A: 255})
	if err := png.Encode(&imageBytes, pixel); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes.Bytes())
}

func assertExactLegacyImageCreateKeys(t *testing.T, body map[string]json.RawMessage, expected ...string) {
	t.Helper()
	if len(body) != len(expected) {
		t.Fatalf("keys=%v want=%v", legacyImageCreateKeys(body), expected)
	}
	for _, key := range expected {
		if _, found := body[key]; !found {
			t.Fatalf("missing %q in %v", key, legacyImageCreateKeys(body))
		}
	}
}

func legacyImageCreateKeys(body map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(body))
	for key := range body {
		keys = append(keys, key)
	}
	return keys
}
