package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/media/domain"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const legacyImageCollectionPath = "/api/admin/image-library"

var (
	errInvalidImageCreateRequest = errors.New("invalid image create request")
	legacyImageCreateMaxBody     = int64(base64.StdEncoding.EncodedLen(domain.MaxImageBytes) + (64 << 10))
)

type legacyImageCreateApplication interface {
	Upload(context.Context, mediaport.UploadCommand) (mediaport.Image, error)
}

type legacyImageCreateSuccess struct {
	OK                       bool                  `json:"ok"`
	Item                     legacyImageCreateItem `json:"item"`
	SourceStatus             string                `json:"source_status"`
	RouteOwner               string                `json:"route_owner"`
	FallbackUsed             bool                  `json:"fallback_used"`
	RealExternalCallExecuted bool                  `json:"real_external_call_executed"`
	StorageAdapterMode       string                `json:"storage_adapter_mode"`
	AdapterMode              string                `json:"adapter_mode"`
}

// This is deliberately a new projection. It never serializes the shared
// upload command, data URL, bytes, checksum, provider fields, or raw tags.
type legacyImageCreateItem struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	FileName    string   `json:"file_name"`
	FileSize    int32    `json:"file_size"`
	MimeType    string   `json:"mime_type"`
	Width       int32    `json:"width"`
	Height      int32    `json:"height"`
	Enabled     bool     `json:"enabled"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Category    string   `json:"category"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

type legacyImageCreateInput struct{ Command mediaport.UploadCommand }

func (handler *Handler) CreateImage(writer http.ResponseWriter, request *http.Request) {
	if request == nil || request.URL == nil || handler == nil {
		writeLegacyImageCreateError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorized := authport.AuthorizationFromContext(request.Context())
	if !principalOK || principal.AdminUserID < 1 || !authorized || authorization.Capability != authport.CapabilityMediaImagesWrite || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		writeLegacyImageCreateError(writer, request, platformhttp.CodeUnauthorized)
		return
	}
	if nilLegacyDependency(handler.media) {
		writeLegacyImageCreateError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	application, ok := handler.media.(legacyImageCreateApplication)
	if !ok || nilLegacyDependency(application) {
		writeLegacyImageCreateError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	input, err := parseLegacyImageCreateBody(writer, request, principal.AdminUserID)
	if err != nil {
		writeLegacyImageCreateError(writer, request, platformhttp.CodeMalformedRequest)
		return
	}
	result, err := application.Upload(request.Context(), input.Command)
	if err != nil {
		switch {
		case errors.Is(err, mediaapp.ErrInvalidUpload):
			writeLegacyImageCreateError(writer, request, platformhttp.CodeMalformedRequest)
		case errors.Is(err, mediaapp.ErrConflict):
			writeLegacyImageCreateError(writer, request, platformhttp.CodeConflict)
		default:
			writeLegacyImageCreateError(writer, request, platformhttp.CodeDependencyUnavailable)
		}
		return
	}
	payload, err := json.Marshal(legacyImageCreateSuccess{
		OK: true, Item: projectLegacyImageCreateItem(result), SourceStatus: "local_repository_write",
		RouteOwner: "ai_crm_next", FallbackUsed: false, RealExternalCallExecuted: false,
		StorageAdapterMode: "postgresql", AdapterMode: "postgresql",
	})
	if err != nil {
		writeLegacyImageCreateError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	writeLegacyImageCreateJSON(writer, http.StatusOK, payload)
}

func parseLegacyImageCreateBody(writer http.ResponseWriter, request *http.Request, actor int64) (legacyImageCreateInput, error) {
	if request == nil || request.Body == nil || actor < 1 || !legacyImageCreateJSONContentType(request.Header.Get("Content-Type")) {
		return legacyImageCreateInput{}, errInvalidImageCreateRequest
	}
	keys := request.Header.Values("Idempotency-Key")
	if len(keys) != 1 || keys[0] != strings.TrimSpace(keys[0]) || len(keys[0]) < 16 || len(keys[0]) > 128 || !utf8.ValidString(keys[0]) {
		return legacyImageCreateInput{}, errInvalidImageCreateRequest
	}
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, legacyImageCreateMaxBody))
	if err != nil || !utf8.Valid(body) {
		return legacyImageCreateInput{}, errInvalidImageCreateRequest
	}
	values, err := legacyImageCreateObject(body)
	if err != nil {
		return legacyImageCreateInput{}, err
	}
	dataURL, err := legacyImageCreateString(values, "data_url", true, "")
	if err != nil {
		return legacyImageCreateInput{}, err
	}
	fileName, err := legacyImageCreateString(values, "file_name", true, "")
	if err != nil {
		return legacyImageCreateInput{}, err
	}
	name, err := legacyImageCreateString(values, "name", false, "")
	if err != nil {
		return legacyImageCreateInput{}, err
	}
	description, err := legacyImageCreateString(values, "description", false, "")
	if err != nil {
		return legacyImageCreateInput{}, err
	}
	category, err := legacyImageCreateString(values, "category", false, "")
	if err != nil {
		return legacyImageCreateInput{}, err
	}
	tags, err := legacyImageCreateTags(values)
	if err != nil {
		return legacyImageCreateInput{}, err
	}
	enabled, err := legacyImageCreateEnabled(values)
	if err != nil {
		return legacyImageCreateInput{}, err
	}
	name, err = normalizeLegacyImageCreateText(name, 200, true)
	if err != nil {
		return legacyImageCreateInput{}, err
	}
	description, err = normalizeLegacyImageCreateText(description, 10_000, true)
	if err != nil {
		return legacyImageCreateInput{}, err
	}
	category, err = normalizeLegacyImageCreateText(category, 200, true)
	if err != nil {
		return legacyImageCreateInput{}, err
	}
	content, declaredType, err := decodeLegacyImageDataURL(dataURL)
	if err != nil {
		return legacyImageCreateInput{}, err
	}
	// Reject an unsafe filename, MIME mismatch, partial image, dimensions, and
	// pixel/byte limits before the application is invoked. Upload repeats this
	// admission inside its UoW so alternate callers receive the same boundary.
	if _, err := domain.Inspect(fileName, declaredType, content); err != nil {
		return legacyImageCreateInput{}, errInvalidImageCreateRequest
	}
	return legacyImageCreateInput{Command: mediaport.UploadCommand{
		Actor: actor, IdempotencyKey: keys[0], FileName: fileName, DeclaredType: declaredType, Content: content,
		Name: name, Description: description, Tags: strings.Join(tags, ","), Category: category, Enabled: enabled,
	}}, nil
}

func legacyImageCreateObject(body []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errInvalidImageCreateRequest
	}
	allowed := map[string]struct{}{"data_url": {}, "name": {}, "file_name": {}, "tags": {}, "description": {}, "category": {}, "enabled": {}}
	values := make(map[string]json.RawMessage, len(allowed))
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || !utf8.ValidString(key) {
			return nil, errInvalidImageCreateRequest
		}
		if _, permitted := allowed[key]; !permitted {
			return nil, errInvalidImageCreateRequest
		}
		if _, duplicate := values[key]; duplicate {
			return nil, errInvalidImageCreateRequest
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, errInvalidImageCreateRequest
		}
		values[key] = append(json.RawMessage(nil), value...)
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, errInvalidImageCreateRequest
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errInvalidImageCreateRequest
	}
	return values, nil
}

func legacyImageCreateString(values map[string]json.RawMessage, key string, required bool, fallback string) (string, error) {
	raw, found := values[key]
	if !found {
		if required {
			return "", errInvalidImageCreateRequest
		}
		return fallback, nil
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || !utf8.ValidString(value) {
		return "", errInvalidImageCreateRequest
	}
	return value, nil
}

func legacyImageCreateTags(values map[string]json.RawMessage) ([]string, error) {
	raw, found := values["tags"]
	if !found {
		return []string{}, nil
	}
	var input []string
	if json.Unmarshal(raw, &input) != nil || input == nil {
		return nil, errInvalidImageCreateRequest
	}
	seen := make(map[string]struct{}, len(input))
	result := make([]string, 0, len(input))
	for _, rawTag := range input {
		tag, err := normalizeLegacyImageCreateText(rawTag, 64, false)
		if err != nil || strings.Contains(tag, ",") {
			return nil, errInvalidImageCreateRequest
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
		if len(result) > 50 {
			return nil, errInvalidImageCreateRequest
		}
	}
	return result, nil
}

func legacyImageCreateEnabled(values map[string]json.RawMessage) (*bool, error) {
	raw, found := values["enabled"]
	if !found {
		return nil, nil
	}
	var enabled bool
	if json.Unmarshal(raw, &enabled) != nil {
		return nil, errInvalidImageCreateRequest
	}
	return &enabled, nil
}

func normalizeLegacyImageCreateText(value string, maxRunes int, allowEmpty bool) (string, error) {
	if !utf8.ValidString(value) {
		return "", errInvalidImageCreateRequest
	}
	value = strings.TrimSpace(value)
	if (!allowEmpty && value == "") || utf8.RuneCountInString(value) > maxRunes {
		return "", errInvalidImageCreateRequest
	}
	return value, nil
}

func decodeLegacyImageDataURL(value string) ([]byte, string, error) {
	prefixes := []struct{ prefix, mediaType string }{
		{"data:image/png;base64,", "image/png"},
		{"data:image/jpeg;base64,", "image/jpeg"},
		{"data:image/gif;base64,", "image/gif"},
	}
	for _, candidate := range prefixes {
		if !strings.HasPrefix(value, candidate.prefix) {
			continue
		}
		encoded := strings.TrimPrefix(value, candidate.prefix)
		if encoded == "" || len(encoded) > base64.StdEncoding.EncodedLen(domain.MaxImageBytes) {
			return nil, "", errInvalidImageCreateRequest
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(decoded) == 0 || len(decoded) > domain.MaxImageBytes || base64.StdEncoding.EncodeToString(decoded) != encoded {
			return nil, "", errInvalidImageCreateRequest
		}
		return decoded, candidate.mediaType, nil
	}
	return nil, "", errInvalidImageCreateRequest
}

func legacyImageCreateJSONContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && strings.EqualFold(mediaType, "application/json")
}

func projectLegacyImageCreateItem(image mediaport.Image) legacyImageCreateItem {
	tags := []string{}
	if image.Tags != "" {
		tags = strings.Split(image.Tags, ",")
	}
	return legacyImageCreateItem{
		ID: image.ID, Name: image.Name, FileName: image.FileName, FileSize: image.FileSize, MimeType: image.MimeType,
		Width: image.Width, Height: image.Height, Enabled: image.Enabled, Description: image.Description, Tags: tags,
		Category: image.Category, CreatedAt: image.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: image.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func writeLegacyImageCreateJSON(writer http.ResponseWriter, status int, payload []byte) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_, _ = writer.Write(payload)
}

func writeLegacyImageCollectionMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Allow", "GET, POST")
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusMethodNotAllowed)
}

func legacyImageCreateSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		next.ServeHTTP(legacyImageCreateHeaderWriter{ResponseWriter: writer}, request)
	})
}

type legacyImageCreateHeaderWriter struct{ http.ResponseWriter }

func (writer legacyImageCreateHeaderWriter) WriteHeader(status int) {
	switch status {
	case http.StatusBadRequest:
		platformhttp.MarkCompatibilityError(writer.ResponseWriter, platformhttp.CodeMalformedRequest)
	case http.StatusUnauthorized:
		platformhttp.MarkCompatibilityError(writer.ResponseWriter, platformhttp.CodeUnauthenticated)
	case http.StatusForbidden:
		platformhttp.MarkCompatibilityError(writer.ResponseWriter, platformhttp.CodeUnauthorized)
	case http.StatusConflict:
		platformhttp.MarkCompatibilityError(writer.ResponseWriter, platformhttp.CodeConflict)
	case http.StatusServiceUnavailable:
		platformhttp.MarkCompatibilityError(writer.ResponseWriter, platformhttp.CodeDependencyUnavailable)
	}
	writer.setSecurityHeaders()
	writer.ResponseWriter.WriteHeader(status)
}

func (writer legacyImageCreateHeaderWriter) Write(payload []byte) (int, error) {
	writer.setSecurityHeaders()
	return writer.ResponseWriter.Write(payload)
}

func (writer legacyImageCreateHeaderWriter) setSecurityHeaders() {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}

func writeLegacyImageCreateError(writer http.ResponseWriter, request *http.Request, code platformhttp.ErrorCode) {
	status := http.StatusServiceUnavailable
	switch code {
	case platformhttp.CodeMalformedRequest:
		status = http.StatusBadRequest
	case platformhttp.CodeUnauthorized:
		status = http.StatusForbidden
	case platformhttp.CodeConflict:
		status = http.StatusConflict
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
	}{Code: code, Message: legacyImageCreateMessage(code), RequestID: requestID})
	if err == nil {
		writeLegacyImageCreateJSON(writer, status, payload)
	}
}

func legacyImageCreateMessage(code platformhttp.ErrorCode) string {
	switch code {
	case platformhttp.CodeMalformedRequest:
		return "The request is malformed."
	case platformhttp.CodeUnauthorized:
		return "Permission is denied."
	case platformhttp.CodeConflict:
		return "The request conflicts with the current state."
	default:
		return "A required dependency is unavailable."
	}
}
