package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

type legacyMediaStub struct {
	result  mediaport.Image
	err     error
	command mediaport.UploadCommand
}

func (stub *legacyMediaStub) Upload(_ context.Context, command mediaport.UploadCommand) (mediaport.Image, error) {
	stub.command = command
	return stub.result, stub.err
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

func legacyMediaRouter(t *testing.T, media legacyMediaApplication) (http.Handler, *recordingAuth) {
	t.Helper()
	service := &recordingAuth{}
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
	return router, service
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
