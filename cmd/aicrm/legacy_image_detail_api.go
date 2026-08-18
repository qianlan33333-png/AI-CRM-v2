package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const (
	legacyImageDetailPath       = "/api/admin/image-library/{image_id}"
	legacyImageDetailMaxJSONLen = 30 << 20
)

var (
	errInvalidImageDetailRequest = errors.New("invalid image detail request")
	imageDetailIDPattern         = regexp.MustCompile(`^[1-9][0-9]*$`)
)

type legacyImageDetailApplication interface {
	GetImageDetail(context.Context, int64) (mediaapp.ImageDetail, error)
}

type legacyImageDetailQuery struct {
	IncludeData bool
	Variant     string
}

type legacyImageDetailSuccess struct {
	OK                       bool                  `json:"ok"`
	Item                     legacyImageDetailItem `json:"item"`
	SourceStatus             string                `json:"source_status"`
	RouteOwner               string                `json:"route_owner"`
	FallbackUsed             bool                  `json:"fallback_used"`
	RealExternalCallExecuted bool                  `json:"real_external_call_executed"`
	StorageAdapterMode       string                `json:"storage_adapter_mode"`
	AdapterMode              string                `json:"adapter_mode"`
}

type legacyImageDetailItem struct {
	ID                    int64          `json:"id"`
	Name                  string         `json:"name"`
	FileName              string         `json:"file_name"`
	MimeType              string         `json:"mime_type"`
	FileSize              int32          `json:"file_size"`
	Description           string         `json:"description"`
	Category              string         `json:"category"`
	Width                 int32          `json:"width"`
	Height                int32          `json:"height"`
	CreatedAt             string         `json:"created_at"`
	UpdatedAt             string         `json:"updated_at"`
	ContentType           string         `json:"content_type"`
	Tags                  []string       `json:"tags"`
	Enabled               bool           `json:"enabled"`
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
	VariantURL            string         `json:"variant_url,omitempty"`
	DataBase64            string         `json:"data_base64,omitempty"`
	DataURL               string         `json:"data_url,omitempty"`
}

func (handler *Handler) GetImageDetail(writer http.ResponseWriter, request *http.Request) {
	if request == nil || request.URL == nil {
		writeLegacyImageDetailError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	imageID, err := parseLegacyImageDetailID(chi.URLParam(request, "image_id"))
	if err != nil {
		writeLegacyImageDetailError(writer, request, platformhttp.CodeValidationFailed)
		return
	}
	query, err := parseLegacyImageDetailQuery(request.URL.RawQuery)
	if err != nil {
		writeLegacyImageDetailError(writer, request, platformhttp.CodeValidationFailed)
		return
	}
	if handler == nil || nilLegacyDependency(handler.media) {
		writeLegacyImageDetailError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	application, ok := handler.media.(legacyImageDetailApplication)
	if !ok || nilLegacyDependency(application) {
		writeLegacyImageDetailError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	detail, err := application.GetImageDetail(request.Context(), imageID)
	if err != nil {
		if errors.Is(err, mediaapp.ErrImageDetailNotFound) {
			writeLegacyImageDetailError(writer, request, platformhttp.CodeNotFound)
			return
		}
		writeLegacyImageDetailError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}

	response := legacyImageDetailSuccess{
		OK: true, Item: projectLegacyImageDetailItem(detail, query), SourceStatus: "next_media_library",
		RouteOwner: "ai_crm_next", FallbackUsed: false, RealExternalCallExecuted: false,
		StorageAdapterMode: "postgresql", AdapterMode: "postgresql",
	}
	payload, err := json.Marshal(response)
	if err != nil || len(payload) > legacyImageDetailMaxJSONLen {
		writeLegacyImageDetailError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	writeLegacyImageDetailJSON(writer, http.StatusOK, payload)
}

func parseLegacyImageDetailID(value string) (int64, error) {
	if !imageDetailIDPattern.MatchString(value) {
		return 0, errInvalidImageDetailRequest
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 {
		return 0, errInvalidImageDetailRequest
	}
	return id, nil
}

func parseLegacyImageDetailQuery(rawQuery string) (legacyImageDetailQuery, error) {
	if !utf8.ValidString(rawQuery) {
		return legacyImageDetailQuery{}, errInvalidImageDetailRequest
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return legacyImageDetailQuery{}, errInvalidImageDetailRequest
	}
	for key, values := range values {
		if (key != "include_data" && key != "variant") || !utf8.ValidString(key) || len(values) != 1 || !utf8.ValidString(values[0]) {
			return legacyImageDetailQuery{}, errInvalidImageDetailRequest
		}
	}
	result := legacyImageDetailQuery{}
	if value, exists := values["include_data"]; exists {
		if value[0] == "true" {
			result.IncludeData = true
		} else if value[0] != "false" {
			return legacyImageDetailQuery{}, errInvalidImageDetailRequest
		}
	}
	if value, exists := values["variant"]; exists {
		result.Variant = value[0]
		if result.Variant != "" && !mediaapp.ValidImageVariantKey(result.Variant) {
			return legacyImageDetailQuery{}, errInvalidImageDetailRequest
		}
	}
	return result, nil
}

func projectLegacyImageDetailItem(detail mediaapp.ImageDetail, query legacyImageDetailQuery) legacyImageDetailItem {
	base := "/api/admin/image-library/" + strconv.FormatInt(detail.ID, 10) + "/variants/"
	item := legacyImageDetailItem{
		ID: detail.ID, Name: detail.Name, FileName: detail.FileName, MimeType: detail.MimeType, FileSize: detail.FileSize,
		Description: detail.Description, Category: detail.Category, Width: detail.Width, Height: detail.Height,
		CreatedAt: detail.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: detail.UpdatedAt.UTC().Format(time.RFC3339Nano),
		ContentType: detail.MimeType, Tags: detail.Tags, Enabled: detail.Enabled, Source: "upload", SourceURL: "", ThumbMediaID: "",
		ThumbMediaIDExpiresAt: "", AIMetadata: map[string]any{}, Thumb160URL: base + "thumb_160", Thumb320URL: base + "thumb_320",
		ThumbURL: base + "thumb_320", PreviewURL: base + "mobile_1080", Mobile1080URL: base + "mobile_1080",
		Large1440URL: base + "large_1440", OriginalURL: base + "original",
	}
	if query.Variant != "" {
		item.VariantURL = base + query.Variant
	}
	if query.IncludeData {
		item.DataBase64 = base64.StdEncoding.EncodeToString(detail.Content)
		item.DataURL = "data:" + detail.MimeType + ";base64," + item.DataBase64
	}
	return item
}

func writeLegacyImageDetailJSON(writer http.ResponseWriter, status int, payload []byte) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_, _ = writer.Write(payload)
}

func writeLegacyImageDetailError(writer http.ResponseWriter, request *http.Request, code platformhttp.ErrorCode) {
	status, message := http.StatusServiceUnavailable, "A required dependency is unavailable."
	switch code {
	case platformhttp.CodeValidationFailed:
		status, message = http.StatusUnprocessableEntity, "Validation failed."
	case platformhttp.CodeNotFound:
		status, message = http.StatusNotFound, "The resource was not found."
	case platformhttp.CodeDependencyUnavailable:
	default:
		code = platformhttp.CodeDependencyUnavailable
	}
	platformhttp.MarkCompatibilityError(writer, code)
	requestID := ""
	if request != nil {
		requestID = platformhttp.RequestID(request.Context())
	}
	if requestID == "" {
		requestID = "unknown"
	}
	payload, err := json.Marshal(struct {
		Code      platformhttp.ErrorCode `json:"code"`
		Message   string                 `json:"message"`
		RequestID string                 `json:"request_id"`
	}{Code: code, Message: message, RequestID: requestID})
	if err != nil {
		return
	}
	writeLegacyImageDetailJSON(writer, status, payload)
}
