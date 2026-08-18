package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"
	"unicode/utf8"

	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const legacyImageListPath = "/api/admin/image-library"

var (
	errInvalidImageListQuery      = errors.New("invalid image list query")
	errInvalidImageListProjection = errors.New("invalid image list projection")
	imageListIntegerPattern       = regexp.MustCompile(`^-?[0-9]+$`)
)

type legacyImageListApplication interface {
	ListImages(context.Context, mediaport.ImageListQuery) (mediaport.ImageListPage, error)
}

type legacyImageListSuccess struct {
	OK                       bool                      `json:"ok"`
	Items                    []mediaport.ImageListItem `json:"items"`
	Total                    int64                     `json:"total"`
	Limit                    int64                     `json:"limit"`
	Offset                   int64                     `json:"offset"`
	Count                    int64                     `json:"count"`
	HasMore                  bool                      `json:"has_more"`
	NextOffset               *int64                    `json:"next_offset"`
	SourceStatus             string                    `json:"source_status"`
	RouteOwner               string                    `json:"route_owner"`
	FallbackUsed             bool                      `json:"fallback_used"`
	RealExternalCallExecuted bool                      `json:"real_external_call_executed"`
	StorageAdapterMode       string                    `json:"storage_adapter_mode"`
	AdapterMode              string                    `json:"adapter_mode"`
}

func (handler *Handler) GetImageList(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || request == nil || request.URL == nil || nilLegacyDependency(handler.media) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeInternal, errInvalidImageListQuery))
		return
	}
	application, ok := handler.media.(legacyImageListApplication)
	if !ok || nilLegacyDependency(application) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeInternal, errInvalidImageListQuery))
		return
	}
	query, err := parseLegacyImageListQuery(request.URL.RawQuery)
	if err != nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeValidationFailed, err))
		return
	}
	page, err := application.ListImages(request.Context(), query)
	if err != nil || !validLegacyImageListPage(page, query) {
		if err == nil {
			err = errInvalidImageListProjection
		}
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeInternal, err))
		return
	}
	if page.Items == nil {
		page.Items = []mediaport.ImageListItem{}
	}
	count := int64(len(page.Items))
	hasMore := page.Offset < page.Total && count < page.Total-page.Offset
	var nextOffset *int64
	if hasMore {
		value := page.Offset + count
		nextOffset = &value
	}
	writeJSON(writer, http.StatusOK, legacyImageListSuccess{
		OK: true, Items: page.Items, Total: page.Total, Limit: page.Limit, Offset: page.Offset,
		Count: count, HasMore: hasMore, NextOffset: nextOffset, SourceStatus: "next_media_library",
		RouteOwner: "ai_crm_next", FallbackUsed: false, RealExternalCallExecuted: false,
		StorageAdapterMode: "postgresql", AdapterMode: "postgresql",
	})
}

func parseLegacyImageListQuery(rawQuery string) (mediaport.ImageListQuery, error) {
	if !utf8.ValidString(rawQuery) {
		return mediaport.ImageListQuery{}, errInvalidImageListQuery
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return mediaport.ImageListQuery{}, errInvalidImageListQuery
	}
	allowed := map[string]struct{}{
		"limit": {}, "offset": {}, "enabled_only": {}, "q": {}, "category": {}, "tags": {},
		"tag_group": {}, "only_unlabeled": {},
	}
	for key, entries := range values {
		if _, ok := allowed[key]; !ok || !utf8.ValidString(key) {
			return mediaport.ImageListQuery{}, errInvalidImageListQuery
		}
		for _, entry := range entries {
			if !utf8.ValidString(entry) {
				return mediaport.ImageListQuery{}, errInvalidImageListQuery
			}
		}
	}

	limit, err := parseImageListInteger(values, "limit", 100)
	if err != nil {
		return mediaport.ImageListQuery{}, err
	}
	offset, err := parseImageListInteger(values, "offset", 0)
	if err != nil {
		return mediaport.ImageListQuery{}, err
	}
	enabledOnly, err := parseImageListBoolean(values, "enabled_only", true)
	if err != nil {
		return mediaport.ImageListQuery{}, err
	}
	onlyUnlabeled, err := parseImageListBoolean(values, "only_unlabeled", false)
	if err != nil {
		return mediaport.ImageListQuery{}, err
	}
	search, err := parseImageListScalar(values, "q", "")
	if err != nil {
		return mediaport.ImageListQuery{}, err
	}
	category, err := parseImageListScalar(values, "category", "")
	if err != nil {
		return mediaport.ImageListQuery{}, err
	}
	tags, err := parseImageListScalar(values, "tags", "")
	if err != nil {
		return mediaport.ImageListQuery{}, err
	}
	groups := append([]string{}, values["tag_group"]...)
	return mediaport.ImageListQuery{
		Limit: limit, Offset: offset, EnabledOnly: enabledOnly, Search: search, Category: category,
		Tags: tags, TagGroups: groups, OnlyUnlabeled: onlyUnlabeled,
	}, nil
}

func parseImageListInteger(values url.Values, key string, fallback int64) (int64, error) {
	value, exists, err := parseImageListOptionalScalar(values, key)
	if err != nil {
		return 0, err
	}
	if !exists {
		return fallback, nil
	}
	if !imageListIntegerPattern.MatchString(value) {
		return 0, errInvalidImageListQuery
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, errInvalidImageListQuery
	}
	return parsed, nil
}

func parseImageListBoolean(values url.Values, key string, fallback bool) (bool, error) {
	value, exists, err := parseImageListOptionalScalar(values, key)
	if err != nil {
		return false, err
	}
	if !exists {
		return fallback, nil
	}
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, errInvalidImageListQuery
	}
}

func parseImageListScalar(values url.Values, key, fallback string) (string, error) {
	value, exists, err := parseImageListOptionalScalar(values, key)
	if err != nil {
		return "", err
	}
	if !exists {
		return fallback, nil
	}
	return value, nil
}

func parseImageListOptionalScalar(values url.Values, key string) (string, bool, error) {
	entries, exists := values[key]
	if !exists {
		return "", false, nil
	}
	if len(entries) != 1 {
		return "", true, errInvalidImageListQuery
	}
	return entries[0], true, nil
}

func validLegacyImageListPage(page mediaport.ImageListPage, query mediaport.ImageListQuery) bool {
	expectedLimit, expectedOffset := effectiveLegacyImageListPage(query.Limit, query.Offset)
	count := int64(len(page.Items))
	if page.Limit != expectedLimit || page.Offset != expectedOffset {
		return false
	}
	if page.Total < 0 || page.Limit < 1 || page.Limit > 500 || page.Offset < 0 || count > page.Limit || count > page.Total {
		return false
	}
	if count == 0 && page.Offset < page.Total {
		return false
	}
	if count > 0 && page.Offset > page.Total-count {
		return false
	}
	for _, item := range page.Items {
		if !validLegacyImageListItem(item) {
			return false
		}
	}
	return true
}

func effectiveLegacyImageListPage(limit, offset int64) (int64, int64) {
	switch {
	case limit == 0:
		limit = 100
	case limit < 0:
		limit = 1
	case limit > 500:
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func validLegacyImageListItem(item mediaport.ImageListItem) bool {
	if item.ID < 1 || item.FileName == "" || item.FileSize < 1 || item.FileSize > 10_485_760 || !item.Enabled ||
		item.Tags == nil || len(item.Tags) > 50 || item.Width < 1 || item.Width > 10_000 || item.Height < 1 || item.Height > 10_000 ||
		int64(item.Width)*int64(item.Height) > 40_000_000 {
		return false
	}
	switch item.MimeType {
	case "image/png", "image/jpeg", "image/gif":
	default:
		return false
	}
	for _, tag := range item.Tags {
		if utf8.RuneCountInString(tag) > 64 {
			return false
		}
	}
	if _, err := time.Parse(time.RFC3339, item.CreatedAt); err != nil {
		return false
	}
	if _, err := time.Parse(time.RFC3339, item.UpdatedAt); err != nil {
		return false
	}
	base := "/api/admin/image-library/" + strconv.FormatInt(item.ID, 10) + "/variants/"
	return item.Thumb160URL == base+"thumb_160" &&
		item.Thumb320URL == base+"thumb_320" &&
		item.ThumbURL == item.Thumb320URL &&
		item.PreviewURL == base+"mobile_1080" &&
		item.Mobile1080URL == item.PreviewURL &&
		item.Large1440URL == base+"large_1440" &&
		item.OriginalURL == base+"original"
}
