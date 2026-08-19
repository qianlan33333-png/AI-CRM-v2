package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const legacyImageUpdateMaxBodyLen = 64 << 10

var errInvalidImageUpdateRequest = errors.New("invalid image update request")

type legacyImageUpdateApplication interface {
	UpdateImageMetadata(context.Context, mediaapp.ImageMetadataUpdateCommand) (mediaapp.ImageMetadata, error)
}

type legacyImageUpdateSuccess struct {
	OK                       bool                  `json:"ok"`
	Item                     legacyImageUpdateItem `json:"item"`
	SourceStatus             string                `json:"source_status"`
	RouteOwner               string                `json:"route_owner"`
	FallbackUsed             bool                  `json:"fallback_used"`
	RealExternalCallExecuted bool                  `json:"real_external_call_executed"`
	StorageAdapterMode       string                `json:"storage_adapter_mode"`
	AdapterMode              string                `json:"adapter_mode"`
}

type legacyImageUpdateItem struct {
	ID                    int64          `json:"id"`
	Name                  string         `json:"name"`
	FileName              string         `json:"file_name"`
	MimeType              string         `json:"mime_type"`
	FileSize              int64          `json:"file_size"`
	Enabled               bool           `json:"enabled"`
	Description           string         `json:"description"`
	Tags                  []string       `json:"tags"`
	Category              string         `json:"category"`
	Width                 int            `json:"width"`
	Height                int            `json:"height"`
	CreatedAt             string         `json:"created_at"`
	UpdatedAt             string         `json:"updated_at"`
	ContentType           string         `json:"content_type"`
	Source                string         `json:"source"`
	SourceURL             string         `json:"source_url"`
	ThumbMediaID          string         `json:"thumb_media_id"`
	ThumbMediaIDExpiresAt string         `json:"thumb_media_id_expires_at"`
	AIMetadata            map[string]any `json:"ai_metadata"`
	Thumb160URL           string         `json:"thumb_160_url"`
	Thumb320URL           string         `json:"thumb_320_url"`
	ThumbURL              string         `json:"thumb_url"`
	PreviewURL            string         `json:"preview_url"`
	Mobile1080URL         string         `json:"mobile_1080_url"`
	Large1440URL          string         `json:"large_1440_url"`
	OriginalURL           string         `json:"original_url"`
}

func (handler *Handler) UpdateImageMetadata(writer http.ResponseWriter, request *http.Request) {
	if request == nil || request.URL == nil {
		writeLegacyImageUpdateError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	imageID, err := parseLegacyImageDetailID(chi.URLParam(request, "image_id"))
	if err != nil {
		writeLegacyImageUpdateError(writer, request, platformhttp.CodeMalformedRequest)
		return
	}
	patch, err := parseLegacyImageUpdateBody(writer, request)
	if err != nil {
		writeLegacyImageUpdateError(writer, request, platformhttp.CodeMalformedRequest)
		return
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		writeLegacyImageUpdateError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	if handler == nil || nilLegacyDependency(handler.media) {
		writeLegacyImageUpdateError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	application, ok := handler.media.(legacyImageUpdateApplication)
	if !ok || nilLegacyDependency(application) {
		writeLegacyImageUpdateError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	image, err := application.UpdateImageMetadata(request.Context(), mediaapp.ImageMetadataUpdateCommand{
		ImageID: imageID,
		Actor:   principal.AdminUserID,
		Patch:   patch,
	})
	if err != nil {
		switch {
		case errors.Is(err, mediaapp.ErrImageMetadataNotFound):
			writeLegacyImageUpdateError(writer, request, platformhttp.CodeNotFound)
		case errors.Is(err, mediaapp.ErrInvalidImageMetadataUpdate):
			writeLegacyImageUpdateError(writer, request, platformhttp.CodeMalformedRequest)
		default:
			writeLegacyImageUpdateError(writer, request, platformhttp.CodeDependencyUnavailable)
		}
		return
	}
	payload, err := json.Marshal(legacyImageUpdateSuccess{
		OK: true, Item: projectLegacyImageUpdateItem(image), SourceStatus: "local_repository_write",
		RouteOwner: "ai_crm_next", FallbackUsed: false, RealExternalCallExecuted: false,
		StorageAdapterMode: "postgresql", AdapterMode: "postgresql",
	})
	if err != nil {
		writeLegacyImageUpdateError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	writeLegacyImageUpdateJSON(writer, http.StatusOK, payload)
}

func parseLegacyImageUpdateBody(writer http.ResponseWriter, request *http.Request) (mediaapp.ImageMetadataPatch, error) {
	if request == nil || request.Body == nil || !legacyImageUpdateJSONContentType(request.Header.Get("Content-Type")) {
		return mediaapp.ImageMetadataPatch{}, errInvalidImageUpdateRequest
	}
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, legacyImageUpdateMaxBodyLen))
	if err != nil || !utf8.Valid(body) {
		return mediaapp.ImageMetadataPatch{}, errInvalidImageUpdateRequest
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return mediaapp.ImageMetadataPatch{}, errInvalidImageUpdateRequest
	}
	seen := make(map[string]struct{}, 5)
	patch := mediaapp.ImageMetadataPatch{}
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || !utf8.ValidString(key) {
			return mediaapp.ImageMetadataPatch{}, errInvalidImageUpdateRequest
		}
		if _, exists := seen[key]; exists {
			return mediaapp.ImageMetadataPatch{}, errInvalidImageUpdateRequest
		}
		seen[key] = struct{}{}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return mediaapp.ImageMetadataPatch{}, errInvalidImageUpdateRequest
		}
		switch key {
		case "name":
			var stringValue string
			if json.Unmarshal(value, &stringValue) != nil || !utf8.ValidString(stringValue) {
				return mediaapp.ImageMetadataPatch{}, errInvalidImageUpdateRequest
			}
			patch.Name = &stringValue
		case "description":
			var stringValue string
			if json.Unmarshal(value, &stringValue) != nil || !utf8.ValidString(stringValue) {
				return mediaapp.ImageMetadataPatch{}, errInvalidImageUpdateRequest
			}
			patch.Description = &stringValue
		case "category":
			var stringValue string
			if json.Unmarshal(value, &stringValue) != nil || !utf8.ValidString(stringValue) {
				return mediaapp.ImageMetadataPatch{}, errInvalidImageUpdateRequest
			}
			patch.Category = &stringValue
		case "tags":
			var tags []string
			if json.Unmarshal(value, &tags) != nil || tags == nil {
				return mediaapp.ImageMetadataPatch{}, errInvalidImageUpdateRequest
			}
			for _, tag := range tags {
				if !utf8.ValidString(tag) || strings.Contains(strings.TrimSpace(tag), ",") {
					return mediaapp.ImageMetadataPatch{}, errInvalidImageUpdateRequest
				}
			}
			patch.Tags = &tags
		case "enabled":
			var enabled bool
			if json.Unmarshal(value, &enabled) != nil {
				return mediaapp.ImageMetadataPatch{}, errInvalidImageUpdateRequest
			}
			patch.Enabled = &enabled
		default:
			return mediaapp.ImageMetadataPatch{}, errInvalidImageUpdateRequest
		}
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return mediaapp.ImageMetadataPatch{}, errInvalidImageUpdateRequest
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return mediaapp.ImageMetadataPatch{}, errInvalidImageUpdateRequest
	}
	return patch, nil
}

func legacyImageUpdateJSONContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && strings.EqualFold(mediaType, "application/json")
}

func projectLegacyImageUpdateItem(image mediaapp.ImageMetadata) legacyImageUpdateItem {
	base := "/api/admin/image-library/" + strconv.FormatInt(image.ID, 10) + "/variants/"
	tags := []string{}
	if image.Tags != "" {
		tags = strings.Split(image.Tags, ",")
	}
	return legacyImageUpdateItem{
		ID: image.ID, Name: image.Name, FileName: image.FileName, MimeType: image.MimeType, FileSize: image.FileSize,
		Enabled: image.Enabled, Description: image.Description, Tags: tags, Category: image.Category,
		Width: image.Width, Height: image.Height, CreatedAt: image.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: image.UpdatedAt.UTC().Format(time.RFC3339Nano), ContentType: image.MimeType, Source: "upload",
		SourceURL: "", ThumbMediaID: "", ThumbMediaIDExpiresAt: "", AIMetadata: map[string]any{},
		Thumb160URL: base + "thumb_160", Thumb320URL: base + "thumb_320", ThumbURL: base + "thumb_320",
		PreviewURL: base + "mobile_1080", Mobile1080URL: base + "mobile_1080", Large1440URL: base + "large_1440",
		OriginalURL: base + "original",
	}
}

func writeLegacyImageUpdateJSON(writer http.ResponseWriter, status int, payload []byte) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_, _ = writer.Write(payload)
}

func legacyImageUpdateSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		next.ServeHTTP(legacyImageUpdateHeaderWriter{ResponseWriter: writer}, request)
	})
}

type legacyImageUpdateHeaderWriter struct {
	http.ResponseWriter
}

func (writer legacyImageUpdateHeaderWriter) WriteHeader(status int) {
	switch status {
	case http.StatusBadRequest:
		platformhttp.MarkCompatibilityError(writer.ResponseWriter, platformhttp.CodeMalformedRequest)
	case http.StatusUnauthorized:
		platformhttp.MarkCompatibilityError(writer.ResponseWriter, platformhttp.CodeUnauthenticated)
	case http.StatusForbidden:
		platformhttp.MarkCompatibilityError(writer.ResponseWriter, platformhttp.CodeUnauthorized)
	case http.StatusNotFound:
		platformhttp.MarkCompatibilityError(writer.ResponseWriter, platformhttp.CodeNotFound)
	case http.StatusServiceUnavailable:
		platformhttp.MarkCompatibilityError(writer.ResponseWriter, platformhttp.CodeDependencyUnavailable)
	}
	writer.setSecurityHeaders()
	writer.ResponseWriter.WriteHeader(status)
}

func (writer legacyImageUpdateHeaderWriter) Write(payload []byte) (int, error) {
	writer.setSecurityHeaders()
	return writer.ResponseWriter.Write(payload)
}

func (writer legacyImageUpdateHeaderWriter) setSecurityHeaders() {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}

func writeLegacyImageDetailMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Allow", "GET, PUT, DELETE")
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusMethodNotAllowed)
}

func writeLegacyImageUpdateError(writer http.ResponseWriter, request *http.Request, code platformhttp.ErrorCode) {
	status, message := http.StatusServiceUnavailable, "A required dependency is unavailable."
	switch code {
	case platformhttp.CodeMalformedRequest:
		status, message = http.StatusBadRequest, "The request is malformed."
	case platformhttp.CodeNotFound:
		status, message = http.StatusNotFound, "The resource was not found."
	case platformhttp.CodeDependencyUnavailable:
	default:
		code = platformhttp.CodeDependencyUnavailable
	}
	platformhttp.MarkCompatibilityError(writer, code)
	requestID := "unknown"
	if request != nil && platformhttp.RequestID(request.Context()) != "" {
		requestID = platformhttp.RequestID(request.Context())
	}
	payload, err := json.Marshal(struct {
		Code      platformhttp.ErrorCode `json:"code"`
		Message   string                 `json:"message"`
		RequestID string                 `json:"request_id"`
	}{Code: code, Message: message, RequestID: requestID})
	if err == nil {
		writeLegacyImageUpdateJSON(writer, status, payload)
	}
}
