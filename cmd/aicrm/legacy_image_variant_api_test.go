package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
)

type legacyImageVariantStub struct {
	legacyMediaStub
	variant     mediaapp.ImageVariant
	variantErr  error
	variantID   int64
	variantKey  string
	variantCall int
}

func (stub *legacyImageVariantStub) GetImageVariant(_ context.Context, id int64, key string) (mediaapp.ImageVariant, error) {
	stub.variantCall++
	stub.variantID, stub.variantKey = id, key
	return stub.variant, stub.variantErr
}

func TestLegacyImageVariantReturnsFullyFormedImageForAllKeys(t *testing.T) {
	for _, test := range []struct {
		key, mediaType string
	}{
		{key: "thumb_160", mediaType: "image/png"},
		{key: "thumb_320", mediaType: "image/jpeg"},
		{key: "mobile_1080", mediaType: "image/gif"},
		{key: "large_1440", mediaType: "image/png"},
		{key: "original", mediaType: "image/jpeg"},
	} {
		t.Run(test.key, func(t *testing.T) {
			body := []byte("validated-" + test.key)
			stub := &legacyImageVariantStub{variant: testLegacyImageVariant(body, test.mediaType)}
			auth := &legacyMediaAuthStub{}
			router := legacyMediaRouterWithAuth(t, stub, auth)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyImageVariantRequest(http.MethodGet, "42", test.key, true))
			if response.Code != http.StatusOK || string(response.Body.Bytes()) != string(body) || stub.variantCall != 1 || stub.variantID != 42 || stub.variantKey != test.key {
				t.Fatalf("status=%d body=%q calls=%d id=%d key=%q", response.Code, response.Body.Bytes(), stub.variantCall, stub.variantID, stub.variantKey)
			}
			if response.Header().Get("Content-Type") != test.mediaType || response.Header().Get("Content-Length") != testLegacyImageVariantKeyLength(test.key) ||
				response.Header().Get("ETag") != stub.variant.ETag || response.Header().Get("Cache-Control") != "private, no-cache" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("headers=%v", response.Header())
			}
			if auth.authenticateCalls != 1 || len(auth.seen) != 1 || auth.seen[0] != authport.CapabilityMediaLibraryRead || auth.csrfCalls != 0 {
				t.Fatalf("auth=%#v", auth)
			}
		})
	}

}

func TestLegacyImageVariantIfNoneMatchWeakComparison(t *testing.T) {
	body := []byte("validated-image")
	stub := &legacyImageVariantStub{variant: testLegacyImageVariant(body, "image/png")}
	router := legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{})
	for _, test := range []struct {
		name, header string
		want         int
	}{
		{name: "exact", header: stub.variant.ETag, want: http.StatusNotModified},
		{name: "weak", header: "W/" + stub.variant.ETag, want: http.StatusNotModified},
		{name: "list", header: `"not-a-match", W/` + stub.variant.ETag, want: http.StatusNotModified},
		{name: "star", header: "*", want: http.StatusNotModified},
		{name: "invalid", header: "W/not-an-etag", want: http.StatusOK},
		{name: "mismatch", header: `"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`, want: http.StatusOK},
		{name: "injection", header: "\r\nX-Evil: yes", want: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := legacyImageVariantRequest(http.MethodGet, "42", "original", true)
			request.Header.Set("If-None-Match", test.header)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.want || response.Header().Get("ETag") != stub.variant.ETag || response.Header().Get("Cache-Control") != "private, no-cache" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("status=%d headers=%v", response.Code, response.Header())
			}
			if test.want == http.StatusNotModified {
				if response.Body.Len() != 0 || response.Header().Get("Content-Type") != "" || response.Header().Get("Content-Length") != "" {
					t.Fatalf("304 body=%q headers=%v", response.Body.Bytes(), response.Header())
				}
			} else if string(response.Body.Bytes()) != string(body) {
				t.Fatalf("200 body=%q", response.Body.Bytes())
			}
		})
	}

	request := legacyImageVariantRequest(http.MethodGet, "42", "original", true)
	request.Header.Add("If-None-Match", "*")
	request.Header.Add("If-None-Match", "W/not-an-etag")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || string(response.Body.Bytes()) != string(body) {
		t.Fatalf("multiple invalid If-None-Match fields status=%d body=%q", response.Code, response.Body.Bytes())
	}
}

func TestLegacyImageVariantRejectsInvalidPathAndMapsFailuresSafely(t *testing.T) {
	for _, test := range []struct {
		name, imageID, key string
		err                error
		want               int
	}{
		{name: "zero", imageID: "0", key: "original", want: http.StatusUnprocessableEntity},
		{name: "leading zero", imageID: "01", key: "original", want: http.StatusUnprocessableEntity},
		{name: "sign", imageID: "+1", key: "original", want: http.StatusUnprocessableEntity},
		{name: "negative", imageID: "-1", key: "original", want: http.StatusUnprocessableEntity},
		{name: "overflow", imageID: "9223372036854775808", key: "original", want: http.StatusUnprocessableEntity},
		{name: "key", imageID: "1", key: "unexpected", want: http.StatusUnprocessableEntity},
		{name: "missing", imageID: "1", key: "original", err: mediaapp.ErrImageVariantNotFound, want: http.StatusNotFound},
		{name: "storage failure", imageID: "1", key: "original", err: errors.New("pq: filename=secret.png checksum=secret"), want: http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &legacyImageVariantStub{variant: testLegacyImageVariant([]byte("would-be-binary"), "image/png"), variantErr: test.err}
			router := legacyMediaRouterWithAuth(t, stub, &legacyMediaAuthStub{})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyImageVariantRequest(http.MethodGet, test.imageID, test.key, true))
			if response.Code != test.want || (test.want == http.StatusUnprocessableEntity && stub.variantCall != 0) {
				t.Fatalf("status=%d calls=%d body=%q", response.Code, stub.variantCall, response.Body.Bytes())
			}
			if strings.Contains(response.Body.String(), "would-be-binary") || strings.Contains(response.Body.String(), "secret") {
				t.Fatalf("unsafe response body=%q", response.Body.Bytes())
			}
			if test.want == http.StatusNotFound && !strings.Contains(response.Body.String(), `"code":"NOT_FOUND"`) {
				t.Fatalf("not-found body=%q", response.Body.Bytes())
			}
			if test.want == http.StatusServiceUnavailable && !strings.Contains(response.Body.String(), `"code":"DEPENDENCY_UNAVAILABLE"`) {
				t.Fatalf("unavailable body=%q", response.Body.Bytes())
			}
		})
	}
}

func TestLegacyImageVariantRouterSecurityAndMethodGuard(t *testing.T) {
	for _, role := range []authport.Role{authport.RoleAdmin, authport.RoleOps} {
		t.Run("allowed "+string(role), func(t *testing.T) {
			stub := &legacyImageVariantStub{variant: testLegacyImageVariant([]byte("image"), "image/png")}
			auth := &legacyMediaAuthStub{principal: authport.Principal{AdminUserID: 7, Role: role}}
			response := httptest.NewRecorder()
			legacyMediaRouterWithAuth(t, stub, auth).ServeHTTP(response, legacyImageVariantRequest(http.MethodGet, "1", "original", true))
			if response.Code != http.StatusOK || stub.variantCall != 1 || auth.csrfCalls != 0 {
				t.Fatalf("status=%d calls=%d csrf=%d", response.Code, stub.variantCall, auth.csrfCalls)
			}
		})
	}

	stub := &legacyImageVariantStub{variant: testLegacyImageVariant([]byte("image"), "image/png")}
	forbiddenAuth := &legacyMediaAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleSales}, authorizeErr: authport.ErrUnauthorized}
	response := httptest.NewRecorder()
	legacyMediaRouterWithAuth(t, stub, forbiddenAuth).ServeHTTP(response, legacyImageVariantRequest(http.MethodGet, "1", "original", true))
	if response.Code != http.StatusForbidden || stub.variantCall != 0 {
		t.Fatalf("sales status=%d calls=%d", response.Code, stub.variantCall)
	}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method+" before authentication", func(t *testing.T) {
			stub := &legacyImageVariantStub{variant: testLegacyImageVariant([]byte("image"), "image/png")}
			auth := &legacyMediaAuthStub{}
			response := httptest.NewRecorder()
			legacyMediaRouterWithAuth(t, stub, auth).ServeHTTP(response, legacyImageVariantRequest(method, "1", "original", false))
			if response.Code != http.StatusMethodNotAllowed || stub.variantCall != 0 || auth.authenticateCalls != 0 || len(auth.seen) != 0 || auth.csrfCalls != 0 || response.Header().Get("Allow") != http.MethodGet {
				t.Fatalf("status=%d calls=%d auth=%#v allow=%q", response.Code, stub.variantCall, auth, response.Header().Get("Allow"))
			}
		})
	}

	stub = &legacyImageVariantStub{variant: testLegacyImageVariant([]byte("image"), "image/png")}
	auth := &legacyMediaAuthStub{}
	response = httptest.NewRecorder()
	legacyMediaRouterWithAuth(t, stub, auth).ServeHTTP(response, legacyImageVariantRequest(http.MethodGet, "1", "original", false))
	if response.Code != http.StatusUnauthorized || stub.variantCall != 0 || auth.authenticateCalls != 0 {
		t.Fatalf("anonymous status=%d calls=%d auth=%#v", response.Code, stub.variantCall, auth)
	}
}

func legacyImageVariantRequest(method, imageID, key string, withSession bool) *http.Request {
	request := httptest.NewRequest(method, "/api/admin/image-library/"+imageID+"/variants/"+key, nil)
	if withSession {
		request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(73)})
	}
	return request
}

func testLegacyImageVariant(body []byte, mediaType string) mediaapp.ImageVariant {
	digest := sha256.Sum256(body)
	return mediaapp.ImageVariant{Content: body, MediaType: mediaType, ETag: `"` + hex.EncodeToString(digest[:]) + `"`}
}

func testLegacyImageVariantKeyLength(key string) string {
	return strconv.Itoa(len("validated-" + key))
}
