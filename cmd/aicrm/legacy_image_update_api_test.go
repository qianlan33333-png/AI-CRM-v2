package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
)

type legacyImageUpdateStub struct {
	legacyMediaStub
	result  mediaapp.ImageMetadata
	err     error
	command mediaapp.ImageMetadataUpdateCommand
	calls   int
}

func (stub *legacyImageUpdateStub) UpdateImageMetadata(_ context.Context, command mediaapp.ImageMetadataUpdateCommand) (mediaapp.ImageMetadata, error) {
	stub.calls++
	stub.command = command
	return stub.result, stub.err
}

func TestLegacyImageUpdateWritesExactLocalEnvelope(t *testing.T) {
	stamp := time.Date(2026, 8, 19, 1, 2, 3, 4, time.UTC)
	stub := &legacyImageUpdateStub{result: mediaapp.ImageMetadata{ID: 42, Name: "新封面", FileName: "cover.png", MimeType: "image/png", FileSize: 5,
		Enabled: false, Description: "说明", Tags: "hero,首页", Category: "cover", Width: 640, Height: 480, CreatedAt: stamp, UpdatedAt: stamp.Add(time.Second)}}
	auth := &legacyMediaAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}}
	response := httptest.NewRecorder()
	legacyMediaRouterWithAuth(t, stub, auth).ServeHTTP(response, legacyImageUpdateRequest(http.MethodPut, "42", `{"name":" 新封面 ","enabled":false}`, true, true, "application/json; charset=utf-8"))
	if response.Code != http.StatusOK || stub.calls != 1 || stub.command.ImageID != 42 || stub.command.Actor != 7 || stub.command.Patch.Name == nil || *stub.command.Patch.Name != " 新封面 " || stub.command.Patch.Enabled == nil || *stub.command.Patch.Enabled ||
		auth.authenticateCalls != 1 || len(auth.seen) != 1 || auth.seen[0] != authport.CapabilityMediaLibraryWrite || auth.csrfCalls != 1 {
		t.Fatalf("status=%d calls=%d command=%#v auth=%#v body=%s", response.Code, stub.calls, stub.command, auth, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/json" || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers=%v", response.Header())
	}
	body := decodeLegacyImageDetailObject(t, response.Body.Bytes())
	assertExactLegacyImageDetailKeys(t, body, "ok", "item", "source_status", "route_owner", "fallback_used", "real_external_call_executed", "storage_adapter_mode", "adapter_mode")
	if string(body["ok"]) != "true" || string(body["source_status"]) != `"local_repository_write"` || string(body["route_owner"]) != `"ai_crm_next"` || string(body["fallback_used"]) != "false" || string(body["real_external_call_executed"]) != "false" {
		t.Fatalf("body=%s", response.Body.String())
	}
	var item map[string]json.RawMessage
	if err := json.Unmarshal(body["item"], &item); err != nil {
		t.Fatal(err)
	}
	assertExactLegacyImageDetailKeys(t, item, "id", "name", "file_name", "mime_type", "file_size", "enabled", "description", "tags", "category", "width", "height", "created_at", "updated_at", "content_type", "source", "source_url", "thumb_media_id", "thumb_media_id_expires_at", "ai_metadata", "thumb_160_url", "thumb_320_url", "thumb_url", "preview_url", "mobile_1080_url", "large_1440_url", "original_url")
	for _, forbidden := range []string{"data_base64", "data_url", "checksum", "actor", "created_by", "variant_url", "storage", "provider"} {
		if _, exists := item[forbidden]; exists {
			t.Fatalf("forbidden item key %q: %s", forbidden, response.Body.String())
		}
	}
	if string(item["enabled"]) != "false" || string(item["tags"]) != `["hero","首页"]` || string(item["ai_metadata"]) != "{}" || string(item["thumb_url"]) != string(item["thumb_320_url"]) || string(item["preview_url"]) != string(item["mobile_1080_url"]) {
		t.Fatalf("item=%s", body["item"])
	}
}

func TestLegacyImageUpdateRejectsMalformedBodyBeforeTheApplication(t *testing.T) {
	invalid := []struct{ name, body, contentType string }{
		{"missing media type", `{}`, ""}, {"not json", `{}`, "text/plain"}, {"array", `[]`, "application/json"},
		{"trailing", `{} {}`, "application/json"}, {"duplicate", `{"name":"a","name":"b"}`, "application/json"},
		{"unknown data url", `{"data_url":"secret"}`, "application/json"}, {"null", `{"name":null}`, "application/json"},
		{"wrong scalar", `{"enabled":"false"}`, "application/json"}, {"wrong tags", `{"tags":{}}`, "application/json"},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyImageUpdateStub{result: imageUpdateResult()}
			response := httptest.NewRecorder()
			legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{}).ServeHTTP(response, legacyImageUpdateRequest(http.MethodPut, "1", test.body, true, true, test.contentType))
			if response.Code != http.StatusBadRequest || stub.calls != 0 || !strings.Contains(response.Body.String(), `"code":"MALFORMED_REQUEST"`) ||
				response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("status=%d calls=%d headers=%v body=%s", response.Code, stub.calls, response.Header(), response.Body.String())
			}
		})
	}

	tooLarge := `{"name":"` + strings.Repeat("x", legacyImageUpdateMaxBodyLen) + `"}`
	stub := &legacyImageUpdateStub{result: imageUpdateResult()}
	response := httptest.NewRecorder()
	legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{}).ServeHTTP(response, legacyImageUpdateRequest(http.MethodPut, "1", tooLarge, true, true, "application/json"))
	if response.Code != http.StatusBadRequest || stub.calls != 0 {
		t.Fatalf("large status=%d calls=%d", response.Code, stub.calls)
	}
}

func TestLegacyImageUpdateRejectsCommaTagAndInvalidUTF8BeforeTheApplication(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{"comma tag", `{"tags":[" safe ","one,two"]}`},
		{"invalid utf8", string([]byte{'{', '"', 'n', 'a', 'm', 'e', '"', ':', '"', 0xff, '"', '}'})},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyImageUpdateStub{result: imageUpdateResult()}
			response := httptest.NewRecorder()
			legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{}).ServeHTTP(response, legacyImageUpdateRequest(http.MethodPut, "1", test.body, true, true, "application/json"))
			if response.Code != http.StatusBadRequest || stub.calls != 0 || !strings.Contains(response.Body.String(), `"code":"MALFORMED_REQUEST"`) ||
				response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("status=%d calls=%d headers=%v body=%s", response.Code, stub.calls, response.Header(), response.Body.String())
			}
		})
	}
}

func TestLegacyImageUpdateBoundaryRequestsReachApplicationAndMapItsValidation(t *testing.T) {
	makeTags := func(count int, value string) []string {
		tags := make([]string, count)
		for index := range tags {
			tags[index] = value + strings.Repeat("x", index%2)
		}
		return tags
	}
	for _, test := range []struct {
		name    string
		payload any
		invalid bool
	}{
		{"name zero", map[string]any{"name": ""}, false},
		{"name one", map[string]any{"name": "x"}, false},
		{"name 200", map[string]any{"name": strings.Repeat("x", 200)}, false},
		{"name 201", map[string]any{"name": strings.Repeat("x", 201)}, true},
		{"description zero", map[string]any{"description": ""}, false},
		{"description one", map[string]any{"description": "x"}, false},
		{"description 10000", map[string]any{"description": strings.Repeat("x", 10_000)}, false},
		{"description 10001", map[string]any{"description": strings.Repeat("x", 10_001)}, true},
		{"category zero", map[string]any{"category": ""}, false},
		{"category one", map[string]any{"category": "x"}, false},
		{"category 200", map[string]any{"category": strings.Repeat("x", 200)}, false},
		{"category 201", map[string]any{"category": strings.Repeat("x", 201)}, true},
		{"tags zero", map[string]any{"tags": []string{}}, false},
		{"tags one", map[string]any{"tags": []string{"x"}}, false},
		{"tags 50", map[string]any{"tags": makeTags(50, "tag")}, false},
		{"tags 51", map[string]any{"tags": makeTags(51, "tag")}, true},
		{"tag 64", map[string]any{"tags": []string{strings.Repeat("x", 64)}}, false},
		{"tag 65", map[string]any{"tags": []string{strings.Repeat("x", 65)}}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, err := json.Marshal(test.payload)
			if err != nil {
				t.Fatal(err)
			}
			stub := &legacyImageUpdateStub{result: imageUpdateResult()}
			if test.invalid {
				stub.err = mediaapp.ErrInvalidImageMetadataUpdate
			}
			response := httptest.NewRecorder()
			legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{}).ServeHTTP(response, legacyImageUpdateRequest(http.MethodPut, "1", string(body), true, true, "application/json"))
			wantStatus := http.StatusOK
			if test.invalid {
				wantStatus = http.StatusBadRequest
			}
			if response.Code != wantStatus || stub.calls != 1 {
				t.Fatalf("status=%d want=%d calls=%d body=%s", response.Code, wantStatus, stub.calls, response.Body.String())
			}
		})
	}
}

func TestLegacyImageUpdateRouterAndFailureContract(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		stub := &legacyImageUpdateStub{result: imageUpdateResult()}
		auth := &legacyMediaAuthStub{}
		response := httptest.NewRecorder()
		legacyMediaRouterWithAuth(t, stub, auth).ServeHTTP(response, legacyImageUpdateRequest(method, "1", `{}`, false, false, "application/json"))
		if response.Code != http.StatusMethodNotAllowed || stub.calls != 0 || auth.authenticateCalls != 0 || len(auth.seen) != 0 || auth.csrfCalls != 0 ||
			response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("method=%s status=%d calls=%d auth=%#v headers=%v", method, response.Code, stub.calls, auth, response.Header())
		}
	}
	for _, request := range []*http.Request{
		legacyImageUpdateRequest(http.MethodPut, "0", `{"data_url":"forbidden"}`, false, false, "application/json"),
		legacyImageUpdateRequest(http.MethodPut, "1", `{"data_url":"forbidden"}`, false, false, "application/json"),
	} {
		stub := &legacyImageUpdateStub{result: imageUpdateResult()}
		response := httptest.NewRecorder()
		legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{}).ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized || stub.calls != 0 || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("anonymous status=%d calls=%d body=%s", response.Code, stub.calls, response.Body.String())
		}
	}
	for _, imageID := range []string{"0", "01", "-1", "abc", "9223372036854775808"} {
		t.Run("invalid image id "+imageID, func(t *testing.T) {
			stub := &legacyImageUpdateStub{result: imageUpdateResult()}
			response := httptest.NewRecorder()
			legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{}).ServeHTTP(response, legacyImageUpdateRequest(http.MethodPut, imageID, `{}`, true, true, "application/json"))
			if response.Code != http.StatusBadRequest || stub.calls != 0 || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("id=%q status=%d calls=%d headers=%v body=%s", imageID, response.Code, stub.calls, response.Header(), response.Body.String())
			}
		})
	}
	for _, test := range []struct {
		name string
		auth *legacyMediaAuthStub
		csrf bool
	}{
		{"csrf required", &legacyMediaAuthStub{}, false},
		{"write scope required", &legacyMediaAuthStub{authorizeErr: authport.ErrUnauthorized}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyImageUpdateStub{result: imageUpdateResult()}
			response := httptest.NewRecorder()
			legacyMediaRouterWithAuth(t, stub, test.auth).ServeHTTP(response, legacyImageUpdateRequest(http.MethodPut, "1", `{}`, true, test.csrf, "application/json"))
			if response.Code != http.StatusForbidden || stub.calls != 0 || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("status=%d calls=%d auth=%#v body=%s", response.Code, stub.calls, test.auth, response.Body.String())
			}
		})
	}
	for _, test := range []struct {
		err  error
		want int
		code string
	}{
		{mediaapp.ErrImageMetadataNotFound, http.StatusNotFound, "NOT_FOUND"},
		{errors.New("private pg failure checksum=secret"), http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE"},
	} {
		stub := &legacyImageUpdateStub{result: imageUpdateResult(), err: test.err}
		response := httptest.NewRecorder()
		legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{}).ServeHTTP(response, legacyImageUpdateRequest(http.MethodPut, "1", `{}`, true, true, "application/json"))
		if response.Code != test.want || stub.calls != 1 || !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`) || strings.Contains(response.Body.String(), "secret") {
			t.Fatalf("status=%d calls=%d body=%s", response.Code, stub.calls, response.Body.String())
		}
	}
}

func TestLegacyImageUpdateSecurityHeadersPreserveGatewayErrorClassification(t *testing.T) {
	for _, test := range []struct {
		name        string
		auth        *legacyMediaAuthStub
		withSession bool
		withCSRF    bool
		wantStatus  int
		wantCode    string
	}{
		{"unauthenticated", &legacyMediaAuthStub{}, false, false, http.StatusUnauthorized, "UNAUTHENTICATED"},
		{"unauthorized", &legacyMediaAuthStub{authorizeErr: authport.ErrUnauthorized}, true, true, http.StatusForbidden, "UNAUTHORIZED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			router := legacyMediaRouterWithAuthAndLogger(t, &legacyImageUpdateStub{result: imageUpdateResult()}, test.auth, slog.New(slog.NewJSONHandler(&logs, nil)))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyImageUpdateRequest(http.MethodPut, "1", `{}`, test.withSession, test.withCSRF, "application/json"))
			if response.Code != test.wantStatus || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
			}
			var entry map[string]any
			if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry); err != nil || entry["err"] != test.wantCode {
				t.Fatalf("log=%q entry=%#v err=%v", logs.String(), entry, err)
			}
		})
	}
}

func legacyImageUpdateRequest(method, id, body string, withSession, withCSRF bool, contentType string) *http.Request {
	request := httptest.NewRequest(method, "/api/admin/image-library/"+id, strings.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if withSession {
		request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(86)})
	}
	if withCSRF {
		request.Header.Set("X-CSRF-Token", legacyToken(87))
	}
	return request
}

func imageUpdateResult() mediaapp.ImageMetadata {
	stamp := time.Date(2026, 8, 19, 1, 2, 3, 0, time.UTC)
	return mediaapp.ImageMetadata{ID: 1, Name: "cover", FileName: "cover.png", MimeType: "image/png", FileSize: 1, Enabled: true,
		Width: 1, Height: 1, CreatedAt: stamp, UpdatedAt: stamp}
}
