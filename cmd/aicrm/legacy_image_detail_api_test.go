package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
)

type legacyImageDetailStub struct {
	legacyMediaStub
	detail     mediaapp.ImageDetail
	detailErr  error
	detailID   int64
	detailCall int
}

func (stub *legacyImageDetailStub) GetImageDetail(_ context.Context, id int64) (mediaapp.ImageDetail, error) {
	stub.detailCall++
	stub.detailID = id
	return stub.detail, stub.detailErr
}

func TestLegacyImageDetailReturnsExactEnvelopeAndOptionalFields(t *testing.T) {
	createdAt := time.Date(2026, 8, 19, 9, 8, 7, 654321987, time.FixedZone("legacy", 8*60*60))
	detail := mediaapp.ImageDetail{
		ID: 42, Name: "封面", FileName: "cover.png", MimeType: "image/png", FileSize: 5, Description: "说明", Category: "cover",
		Tags: []string{"hero", "首页"}, Enabled: true, Width: 640, Height: 480, CreatedAt: createdAt, UpdatedAt: createdAt.Add(time.Second), Content: []byte("hello"),
	}
	stub := &legacyImageDetailStub{detail: detail}
	auth := &legacyMediaAuthStub{}
	response := httptest.NewRecorder()
	legacyMediaRouterWithAuth(t, stub, auth).ServeHTTP(response, legacyImageDetailRequest(http.MethodGet, "42", "", true))
	if response.Code != http.StatusOK || stub.detailCall != 1 || stub.detailID != 42 || auth.authenticateCalls != 1 ||
		len(auth.seen) != 1 || auth.seen[0] != authport.CapabilityMediaLibraryRead || auth.csrfCalls != 0 {
		t.Fatalf("status=%d calls=%d id=%d auth=%#v body=%s", response.Code, stub.detailCall, stub.detailID, auth, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/json" || response.Header().Get("Cache-Control") != "private, no-store" ||
		response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("ETag") != "" {
		t.Fatalf("headers=%v", response.Header())
	}
	body := decodeLegacyImageDetailObject(t, response.Body.Bytes())
	assertExactLegacyImageDetailKeys(t, body, "ok", "item", "source_status", "route_owner", "fallback_used", "real_external_call_executed", "storage_adapter_mode", "adapter_mode")
	if string(body["ok"]) != "true" || string(body["source_status"]) != `"next_media_library"` || string(body["route_owner"]) != `"ai_crm_next"` ||
		string(body["fallback_used"]) != "false" || string(body["real_external_call_executed"]) != "false" || string(body["storage_adapter_mode"]) != `"postgresql"` || string(body["adapter_mode"]) != `"postgresql"` {
		t.Fatalf("body=%s", response.Body.String())
	}
	var item map[string]json.RawMessage
	if err := json.Unmarshal(body["item"], &item); err != nil {
		t.Fatal(err)
	}
	assertExactLegacyImageDetailKeys(t, item,
		"id", "name", "file_name", "mime_type", "file_size", "description", "category", "width", "height", "created_at", "updated_at",
		"content_type", "tags", "enabled", "source", "source_url", "thumb_media_id", "thumb_media_id_expires_at", "ai_metadata",
		"thumb_160_url", "thumb_320_url", "thumb_url", "preview_url", "mobile_1080_url", "large_1440_url", "original_url")
	for key, want := range map[string]string{
		"id": "42", "name": `"封面"`, "file_name": `"cover.png"`, "mime_type": `"image/png"`, "file_size": "5", "description": `"说明"`, "category": `"cover"`,
		"width": "640", "height": "480", "created_at": `"2026-08-19T01:08:07.654321987Z"`, "updated_at": `"2026-08-19T01:08:08.654321987Z"`,
		"content_type": `"image/png"`, "enabled": "true", "source": `"upload"`, "source_url": `""`, "thumb_media_id": `""`, "thumb_media_id_expires_at": `""`, "ai_metadata": "{}",
		"thumb_160_url": `"/api/admin/image-library/42/variants/thumb_160"`, "thumb_320_url": `"/api/admin/image-library/42/variants/thumb_320"`, "thumb_url": `"/api/admin/image-library/42/variants/thumb_320"`,
		"preview_url": `"/api/admin/image-library/42/variants/mobile_1080"`, "mobile_1080_url": `"/api/admin/image-library/42/variants/mobile_1080"`, "large_1440_url": `"/api/admin/image-library/42/variants/large_1440"`, "original_url": `"/api/admin/image-library/42/variants/original"`,
	} {
		if string(item[key]) != want {
			t.Fatalf("item[%s]=%s want=%s", key, item[key], want)
		}
	}
	if string(item["tags"]) != `["hero","首页"]` {
		t.Fatalf("tags=%s", item["tags"])
	}
	for _, forbidden := range []string{"variant_url", "data_base64", "data_url", "checksum", "blob", "actor", "storage", "provider"} {
		if _, exists := item[forbidden]; exists {
			t.Fatalf("forbidden item key %q: %s", forbidden, response.Body.String())
		}
	}
}

func TestLegacyImageDetailIncludeDataAndVariants(t *testing.T) {
	detail := testLegacyImageDetail()
	for _, test := range []struct {
		name, query, variant string
		include              bool
	}{
		{name: "empty variant", query: "variant=", variant: ""},
		{name: "data and thumb", query: "include_data=true&variant=thumb_160", variant: "thumb_160", include: true},
		{name: "thumb 320", query: "variant=thumb_320", variant: "thumb_320"},
		{name: "mobile", query: "variant=mobile_1080", variant: "mobile_1080"},
		{name: "large", query: "variant=large_1440", variant: "large_1440"},
		{name: "original", query: "variant=original", variant: "original"},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyImageDetailStub{detail: detail}
			response := httptest.NewRecorder()
			legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{}).ServeHTTP(response, legacyImageDetailRequest(http.MethodGet, "42", test.query, true))
			if response.Code != http.StatusOK || stub.detailCall != 1 {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, stub.detailCall, response.Body.String())
			}
			var body struct {
				Item map[string]json.RawMessage `json:"item"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if test.variant == "" {
				if _, exists := body.Item["variant_url"]; exists {
					t.Fatalf("variant URL unexpectedly present: %s", response.Body.String())
				}
			} else if string(body.Item["variant_url"]) != `"/api/admin/image-library/42/variants/`+test.variant+`"` {
				t.Fatalf("variant URL=%s", body.Item["variant_url"])
			}
			if !test.include {
				if _, data := body.Item["data_base64"]; data {
					t.Fatalf("data unexpectedly present: %s", response.Body.String())
				}
				return
			}
			encoded := base64.StdEncoding.EncodeToString(detail.Content)
			if string(body.Item["data_base64"]) != `"`+encoded+`"` || string(body.Item["data_url"]) != `"data:image/png;base64,`+encoded+`"` {
				t.Fatalf("data body=%s", response.Body.String())
			}
		})
	}
}

func TestLegacyImageDetailRejectsInvalidInputAndMapsFailuresSafely(t *testing.T) {
	invalid := []struct{ name, id, query string }{
		{"zero", "0", ""}, {"leading zero", "01", ""}, {"sign", "+1", ""}, {"overflow", "9223372036854775808", ""},
		{"unknown query", "1", "unexpected=true"}, {"duplicate include", "1", "include_data=true&include_data=false"}, {"duplicate variant", "1", "variant=original&variant=thumb_160"},
		{"bad bool", "1", "include_data=True"}, {"bad variant", "1", "variant=blob"}, {"bad percent", "1", "variant=%ZZ"}, {"bad utf8", "1", "variant=%FF"},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyImageDetailStub{detail: testLegacyImageDetail()}
			response := httptest.NewRecorder()
			legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{}).ServeHTTP(response, legacyImageDetailRequest(http.MethodGet, test.id, test.query, true))
			if response.Code != http.StatusUnprocessableEntity || stub.detailCall != 0 || !strings.Contains(response.Body.String(), `"code":"VALIDATION_FAILED"`) ||
				response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("status=%d calls=%d headers=%v body=%s", response.Code, stub.detailCall, response.Header(), response.Body.String())
			}
		})
	}

	// Parsing must fail before the handler asks whether the registered Media
	// implementation supports detail projection. This remains true after the
	// final authentication and router chain has accepted the session.
	missingDetail := &legacyMediaStub{}
	missingDetailAuth := &legacyMediaAuthStub{}
	missingDetailResponse := httptest.NewRecorder()
	legacyMediaRouterWithAuth(t, missingDetail, missingDetailAuth).ServeHTTP(missingDetailResponse, legacyImageDetailRequest(http.MethodGet, "0", "", true))
	if missingDetailResponse.Code != http.StatusUnprocessableEntity || missingDetailAuth.authenticateCalls != 1 ||
		len(missingDetailAuth.seen) != 1 || missingDetailAuth.seen[0] != authport.CapabilityMediaLibraryRead ||
		!strings.Contains(missingDetailResponse.Body.String(), `"code":"VALIDATION_FAILED"`) {
		t.Fatalf("missing media malformed status=%d auth=%#v body=%s", missingDetailResponse.Code, missingDetailAuth, missingDetailResponse.Body.String())
	}

	for _, test := range []struct {
		name string
		err  error
		want int
		code string
	}{
		{name: "missing", err: mediaapp.ErrImageDetailNotFound, want: http.StatusNotFound, code: "NOT_FOUND"},
		{name: "storage", err: errors.New("sql failed cover.png actor=7 checksum=secret"), want: http.StatusServiceUnavailable, code: "DEPENDENCY_UNAVAILABLE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyImageDetailStub{detail: testLegacyImageDetail(), detailErr: test.err}
			response := httptest.NewRecorder()
			legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{}).ServeHTTP(response, legacyImageDetailRequest(http.MethodGet, "1", "", true))
			if response.Code != test.want || !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`) || strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), "cover.png") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestLegacyImageDetailRouterSecurityMethodGuardAndResponseLimit(t *testing.T) {
	for _, role := range []authport.Role{authport.RoleAdmin, authport.RoleOps} {
		t.Run("allowed "+string(role), func(t *testing.T) {
			stub := &legacyImageDetailStub{detail: testLegacyImageDetail()}
			auth := &legacyMediaAuthStub{principal: authport.Principal{AdminUserID: 7, Role: role}}
			response := httptest.NewRecorder()
			legacyMediaRouterWithAuth(t, stub, auth).ServeHTTP(response, legacyImageDetailRequest(http.MethodGet, "1", "include_data=false", true))
			if response.Code != http.StatusOK || stub.detailCall != 1 || auth.csrfCalls != 0 || strings.Contains(response.Body.String(), `"data_base64"`) {
				t.Fatalf("status=%d calls=%d auth=%#v", response.Code, stub.detailCall, auth)
			}
		})
	}

	forbidden := &legacyImageDetailStub{detail: testLegacyImageDetail()}
	response := httptest.NewRecorder()
	legacyMediaRouterWithAuth(t, forbidden, &legacyMediaAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleSales}, authorizeErr: authport.ErrUnauthorized}).ServeHTTP(response, legacyImageDetailRequest(http.MethodGet, "1", "", true))
	if response.Code != http.StatusForbidden || forbidden.detailCall != 0 {
		t.Fatalf("sales status=%d calls=%d", response.Code, forbidden.detailCall)
	}

	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete} {
		t.Run(method+" before authentication", func(t *testing.T) {
			stub := &legacyImageDetailStub{detail: testLegacyImageDetail()}
			auth := &legacyMediaAuthStub{}
			response := httptest.NewRecorder()
			legacyMediaRouterWithAuth(t, stub, auth).ServeHTTP(response, legacyImageDetailRequest(method, "1", "", false))
			if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") == "" || stub.detailCall != 0 || auth.authenticateCalls != 0 || len(auth.seen) != 0 || auth.csrfCalls != 0 {
				t.Fatalf("status=%d allow=%q calls=%d auth=%#v", response.Code, response.Header().Get("Allow"), stub.detailCall, auth)
			}
		})
	}

	updateAnonymous := &legacyImageDetailStub{detail: testLegacyImageDetail()}
	response = httptest.NewRecorder()
	legacyMediaRouterWithAuth(t, updateAnonymous, &legacyMediaAuthStub{}).ServeHTTP(response, legacyImageDetailRequest(http.MethodPut, "1", "", false))
	if response.Code != http.StatusUnauthorized || updateAnonymous.detailCall != 0 {
		t.Fatalf("anonymous update status=%d calls=%d", response.Code, updateAnonymous.detailCall)
	}

	anonymous := &legacyImageDetailStub{detail: testLegacyImageDetail()}
	response = httptest.NewRecorder()
	legacyMediaRouterWithAuth(t, anonymous, &legacyMediaAuthStub{}).ServeHTTP(response, legacyImageDetailRequest(http.MethodGet, "1", "", false))
	if response.Code != http.StatusUnauthorized || anonymous.detailCall != 0 {
		t.Fatalf("anonymous status=%d calls=%d", response.Code, anonymous.detailCall)
	}
	for _, test := range []struct{ name, id, query string }{
		{name: "invalid id", id: "0"},
		{name: "invalid query", id: "1", query: "unexpected=true"},
	} {
		t.Run("anonymous "+test.name, func(t *testing.T) {
			stub := &legacyImageDetailStub{detail: testLegacyImageDetail()}
			result := httptest.NewRecorder()
			legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{}).ServeHTTP(result, legacyImageDetailRequest(http.MethodGet, test.id, test.query, false))
			if result.Code != http.StatusUnauthorized || stub.detailCall != 0 {
				t.Fatalf("status=%d calls=%d body=%s", result.Code, stub.detailCall, result.Body.String())
			}
		})
	}

	largeContent := &bytes.Buffer{}
	if err := (&png.Encoder{CompressionLevel: png.NoCompression}).Encode(largeContent, image.NewNRGBA(image.Rect(0, 0, 1024, 1024))); err != nil {
		t.Fatal(err)
	}
	if len(largeContent.Bytes()) > 10<<20 {
		t.Fatalf("large fixture size = %d, exceeds 10 MiB source limit", len(largeContent.Bytes()))
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(largeContent.Bytes()))
	if err != nil || format != "png" || config.Width != 1024 || config.Height != 1024 {
		t.Fatalf("large fixture config/format/error=%#v/%q/%v", config, format, err)
	}
	large := &legacyImageDetailStub{detail: testLegacyImageDetail()}
	large.detail.ID = 1
	large.detail.Content = largeContent.Bytes()
	large.detail.FileSize = int32(len(large.detail.Content))
	large.detail.Width, large.detail.Height = 1024, 1024
	response = httptest.NewRecorder()
	legacyMediaRouterWithAuth(t, large, &legacyMediaAuthStub{}).ServeHTTP(response, legacyImageDetailRequest(http.MethodGet, "1", "include_data=true", true))
	if response.Code != http.StatusOK || response.Body.Len() <= 8<<20 || large.detailCall != 1 || response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("large include_data status/body/calls/headers=%d/%d/%d/%v", response.Code, response.Body.Len(), large.detailCall, response.Header())
	}

	overLimit := &legacyImageDetailStub{detail: testLegacyImageDetail()}
	overLimit.detail.Content = make([]byte, 23<<20)
	response = httptest.NewRecorder()
	legacyMediaRouterWithAuth(t, overLimit, &legacyMediaAuthStub{}).ServeHTTP(response, legacyImageDetailRequest(http.MethodGet, "1", "include_data=true", true))
	if response.Code != http.StatusServiceUnavailable || response.Body.Len() == 0 || response.Header().Get("Cache-Control") != "private, no-store" || strings.Contains(response.Body.String(), "data_base64") {
		t.Fatalf("limit status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
}

func legacyImageDetailRequest(method, id, rawQuery string, withSession bool) *http.Request {
	request := httptest.NewRequest(method, "/api/admin/image-library/"+id, nil)
	request.URL.RawQuery = rawQuery
	if withSession {
		request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(75)})
	}
	return request
}

func testLegacyImageDetail() mediaapp.ImageDetail {
	stamp := time.Date(2026, 8, 19, 1, 2, 3, 4, time.UTC)
	return mediaapp.ImageDetail{ID: 42, Name: "cover", FileName: "cover.png", MimeType: "image/png", FileSize: 5, Description: "desc", Category: "cat", Tags: []string{"hero"}, Width: 2, Height: 2, CreatedAt: stamp, UpdatedAt: stamp, Content: []byte("hello")}
}

func decodeLegacyImageDetailObject(t *testing.T, encoded []byte) map[string]json.RawMessage {
	t.Helper()
	var body map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func assertExactLegacyImageDetailKeys(t *testing.T, values map[string]json.RawMessage, expected ...string) {
	t.Helper()
	if len(values) != len(expected) {
		t.Fatalf("keys=%v expected=%v", legacyImageDetailJSONKeys(values), expected)
	}
	for _, key := range expected {
		if _, exists := values[key]; !exists {
			t.Fatalf("missing key %q in %v", key, legacyImageDetailJSONKeys(values))
		}
	}
}

func legacyImageDetailJSONKeys(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
