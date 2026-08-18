package main

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const legacyImageVariantPath = "/api/admin/image-library/{image_id}/variants/{variant_key}"

var legacyImageVariantIDPattern = regexp.MustCompile(`^[1-9][0-9]*$`)

// Keep this optional capability local to the transport adapter. In particular,
// legacyMediaApplication remains limited to the already-owned upload/facets
// contract rather than becoming a central compatibility bucket.
type legacyImageVariantApplication interface {
	GetImageVariant(context.Context, int64, string) (mediaapp.ImageVariant, error)
}

func (handler *Handler) GetImageVariant(writer http.ResponseWriter, request *http.Request) {
	if request == nil || request.URL == nil {
		return
	}
	imageID, err := parseLegacyImageVariantID(chi.URLParam(request, "image_id"))
	if err != nil || !mediaapp.ValidImageVariantKey(chi.URLParam(request, "variant_key")) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeValidationFailed, mediaapp.ErrInvalidImageVariant))
		return
	}
	if handler == nil || nilLegacyDependency(handler.media) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, mediaapp.ErrImageVariantUnavailable))
		return
	}
	application, ok := handler.media.(legacyImageVariantApplication)
	if !ok || nilLegacyDependency(application) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, mediaapp.ErrImageVariantUnavailable))
		return
	}

	variant, err := application.GetImageVariant(request.Context(), imageID, chi.URLParam(request, "variant_key"))
	if err != nil || !validLegacyImageVariant(variant) {
		code := platformhttp.CodeDependencyUnavailable
		if errors.Is(err, mediaapp.ErrInvalidImageVariant) {
			code = platformhttp.CodeValidationFailed
		} else if errors.Is(err, mediaapp.ErrImageVariantNotFound) {
			code = platformhttp.CodeNotFound
		}
		platformhttp.WriteError(writer, request, platformhttp.NewError(code, mediaapp.ErrImageVariantUnavailable))
		return
	}

	writer.Header().Set("ETag", variant.ETag)
	writer.Header().Set("Cache-Control", "private, no-cache")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if ifNoneMatchMatches(request, variant.ETag) {
		writer.WriteHeader(http.StatusNotModified)
		return
	}
	writer.Header().Set("Content-Type", variant.MediaType)
	writer.Header().Set("Content-Length", strconv.Itoa(len(variant.Content)))
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(variant.Content)
}

func parseLegacyImageVariantID(value string) (int64, error) {
	if !legacyImageVariantIDPattern.MatchString(value) {
		return 0, mediaapp.ErrInvalidImageVariant
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 1 {
		return 0, mediaapp.ErrInvalidImageVariant
	}
	return parsed, nil
}

func validLegacyImageVariant(variant mediaapp.ImageVariant) bool {
	if len(variant.Content) == 0 || len(variant.Content) > 10<<20 ||
		(variant.MediaType != "image/png" && variant.MediaType != "image/jpeg" && variant.MediaType != "image/gif") ||
		!mediaapp.ValidImageVariantETag(variant.ETag) {
		return false
	}
	return true
}

// ifNoneMatchMatches applies RFC weak comparison only to syntactically valid
// entity-tag lists. Invalid input deliberately falls through to a normal 200.
func ifNoneMatchMatches(request *http.Request, currentETag string) bool {
	if request == nil || !mediaapp.ValidImageVariantETag(currentETag) {
		return false
	}
	values := request.Header.Values("If-None-Match")
	if len(values) == 0 {
		return false
	}
	// HTTP permits repeated field values. Parse their combined list so a lone
	// wildcard never accidentally masks an invalid second field.
	matched, valid := parseIfNoneMatch(strings.Join(values, ","), currentETag)
	return valid && matched
}

func parseIfNoneMatch(value, currentETag string) (bool, bool) {
	if strings.ContainsAny(value, "\r\n") {
		return false, false
	}
	index := 0
	skipOWS := func() {
		for index < len(value) && (value[index] == ' ' || value[index] == '\t') {
			index++
		}
	}
	skipOWS()
	if index == len(value) {
		return false, false
	}
	if value[index] == '*' {
		index++
		skipOWS()
		return index == len(value), index == len(value)
	}
	matched := false
	for {
		skipOWS()
		if strings.HasPrefix(value[index:], "W/") {
			index += 2
		}
		if index >= len(value) || value[index] != '"' {
			return false, false
		}
		start := index
		index++
		for index < len(value) && value[index] != '"' {
			if value[index] < 0x21 || value[index] > 0x7e {
				return false, false
			}
			index++
		}
		if index == len(value) {
			return false, false
		}
		index++
		if value[start:index] == currentETag {
			matched = true
		}
		skipOWS()
		if index == len(value) {
			return matched, true
		}
		if value[index] != ',' {
			return false, false
		}
		index++
		skipOWS()
		if index == len(value) {
			return false, false
		}
	}
}
