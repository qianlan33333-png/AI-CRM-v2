package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/media/domain"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const (
	legacyAttachmentCollectionPath = "/api/admin/attachment-library"
	legacyAttachmentUploadPath     = legacyAttachmentCollectionPath + "/upload"
	legacyAttachmentDetailPath     = legacyAttachmentCollectionPath + "/{attachment_id}"
	legacyAttachmentDownloadPath   = legacyAttachmentDetailPath + "/download"
	legacyAttachmentMaxBody        = int64(domain.MaxAttachmentBytes + 128<<10)
	legacyAttachmentUpdateMaxBody  = 64 << 10
)

func isLegacyAttachmentPattern(pattern string) bool {
	return pattern == legacyAttachmentCollectionPath || pattern == legacyAttachmentUploadPath ||
		pattern == legacyAttachmentDetailPath || pattern == legacyAttachmentDownloadPath
}

var (
	errInvalidAttachmentRequest = errors.New("invalid attachment request")
	attachmentIDPattern         = regexp.MustCompile(`^[1-9][0-9]*$`)
)

// legacyAttachmentApplication is intentionally local-only. It has no media
// provider, remote URL, sharing, or public-download capability.
type legacyAttachmentApplication interface {
	List(context.Context, mediaport.AttachmentListQuery) (mediaport.AttachmentListPage, error)
	Get(context.Context, int64) (mediaport.Attachment, error)
	Upload(context.Context, mediaport.AttachmentUploadCommand) (mediaport.Attachment, error)
	Update(context.Context, mediaport.AttachmentUpdateCommand) (mediaport.Attachment, error)
	Delete(context.Context, mediaport.AttachmentDeleteCommand) (mediaapp.AttachmentDeleteResult, error)
	Download(context.Context, int64) (mediaapp.AttachmentDownload, error)
}

type legacyAttachmentItem struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	FileName    string   `json:"file_name"`
	MimeType    string   `json:"mime_type"`
	FileSize    int64    `json:"file_size"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Enabled     bool     `json:"enabled"`
	Version     int64    `json:"version"`
	CreatedBy   int64    `json:"created_by"`
	UpdatedBy   int64    `json:"updated_by"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

type legacyAttachmentListSuccess struct {
	Items  []legacyAttachmentItem `json:"items"`
	Total  int64                  `json:"total"`
	Limit  int64                  `json:"limit"`
	Offset int64                  `json:"offset"`
}

type legacyAttachmentDeleteSuccess struct {
	ID          int64 `json:"id"`
	Deleted     bool  `json:"deleted"`
	HardDeleted bool  `json:"hard_deleted"`
}

type legacyAttachmentDeleteReferences struct {
	AutomationAgents []legacyAttachmentReferenceID `json:"automation_agents"`
	Channels         []legacyAttachmentReferenceID `json:"channels"`
	RadarLinks       []legacyAttachmentReferenceID `json:"radar_links"`
}

type legacyAttachmentReferenceID struct {
	ID int64 `json:"id"`
}

type legacyAttachmentDeleteConflict struct {
	Error      string                           `json:"error"`
	References legacyAttachmentDeleteReferences `json:"references"`
}

func (handler *Handler) ListAttachments(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || request == nil || request.URL == nil || nilLegacyDependency(handler.attachments) {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	if _, ok := legacyAttachmentPrincipal(request, authport.CapabilityMediaLibraryRead); !ok {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeUnauthorized)
		return
	}
	query, err := parseLegacyAttachmentListQuery(request.URL.RawQuery)
	if err != nil {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeMalformedRequest)
		return
	}
	page, err := handler.attachments.List(request.Context(), query)
	if err != nil || !validLegacyAttachmentPage(page, query) {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	items := make([]legacyAttachmentItem, len(page.Items))
	for index, attachment := range page.Items {
		items[index] = projectLegacyAttachment(attachment)
	}
	writeLegacyAttachmentJSON(writer, http.StatusOK, legacyAttachmentListSuccess{Items: items, Total: page.Total, Limit: page.Limit, Offset: page.Offset})
}

// CreateAttachment and UploadAttachment intentionally share one multipart
// command. The old collection POST is only a route alias; it cannot create a
// JSON metadata placeholder without a local PDF blob.
func (handler *Handler) CreateAttachment(writer http.ResponseWriter, request *http.Request) {
	handler.uploadAttachment(writer, request)
}

func (handler *Handler) UploadAttachment(writer http.ResponseWriter, request *http.Request) {
	handler.uploadAttachment(writer, request)
}

func (handler *Handler) uploadAttachment(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || request == nil || request.URL == nil || nilLegacyDependency(handler.attachments) {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	actor, ok := legacyAttachmentPrincipal(request, authport.CapabilityMediaLibraryWrite)
	if !ok {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeUnauthorized)
		return
	}
	command, err := parseLegacyAttachmentUpload(writer, request, actor)
	if err != nil {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeMalformedRequest)
		return
	}
	attachment, err := handler.attachments.Upload(request.Context(), command)
	if err != nil {
		switch {
		case errors.Is(err, mediaapp.ErrInvalidAttachment):
			writeLegacyAttachmentError(writer, request, platformhttp.CodeMalformedRequest)
		case errors.Is(err, mediaapp.ErrAttachmentConflict):
			writeLegacyAttachmentError(writer, request, platformhttp.CodeConflict)
		default:
			writeLegacyAttachmentError(writer, request, platformhttp.CodeDependencyUnavailable)
		}
		return
	}
	if !validLegacyAttachment(attachment) {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	writeLegacyAttachmentJSON(writer, http.StatusOK, projectLegacyAttachment(attachment))
}

func (handler *Handler) GetAttachment(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || request == nil || request.URL == nil || nilLegacyDependency(handler.attachments) {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	if _, ok := legacyAttachmentPrincipal(request, authport.CapabilityMediaLibraryRead); !ok {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeUnauthorized)
		return
	}
	attachmentID, err := parseLegacyAttachmentID(chi.URLParam(request, "attachment_id"))
	if err != nil || request.URL.RawQuery != "" {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeMalformedRequest)
		return
	}
	attachment, err := handler.attachments.Get(request.Context(), attachmentID)
	if err != nil {
		if errors.Is(err, mediaapp.ErrAttachmentNotFound) {
			writeLegacyAttachmentError(writer, request, platformhttp.CodeNotFound)
		} else {
			writeLegacyAttachmentError(writer, request, platformhttp.CodeDependencyUnavailable)
		}
		return
	}
	if !validLegacyAttachment(attachment) {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	writeLegacyAttachmentJSON(writer, http.StatusOK, projectLegacyAttachment(attachment))
}

func (handler *Handler) UpdateAttachment(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || request == nil || request.URL == nil || nilLegacyDependency(handler.attachments) {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	actor, ok := legacyAttachmentPrincipal(request, authport.CapabilityMediaLibraryWrite)
	if !ok {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeUnauthorized)
		return
	}
	attachmentID, err := parseLegacyAttachmentID(chi.URLParam(request, "attachment_id"))
	if err != nil || request.URL.RawQuery != "" {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeMalformedRequest)
		return
	}
	command, err := parseLegacyAttachmentUpdate(writer, request, attachmentID, actor)
	if err != nil {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeMalformedRequest)
		return
	}
	attachment, err := handler.attachments.Update(request.Context(), command)
	if err != nil {
		switch {
		case errors.Is(err, mediaapp.ErrInvalidAttachment):
			writeLegacyAttachmentError(writer, request, platformhttp.CodeMalformedRequest)
		case errors.Is(err, mediaapp.ErrAttachmentNotFound):
			writeLegacyAttachmentError(writer, request, platformhttp.CodeNotFound)
		case errors.Is(err, mediaapp.ErrAttachmentConflict):
			writeLegacyAttachmentError(writer, request, platformhttp.CodeConflict)
		default:
			writeLegacyAttachmentError(writer, request, platformhttp.CodeDependencyUnavailable)
		}
		return
	}
	if !validLegacyAttachment(attachment) {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	writeLegacyAttachmentJSON(writer, http.StatusOK, projectLegacyAttachment(attachment))
}

func (handler *Handler) DeleteAttachment(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || request == nil || request.URL == nil || nilLegacyDependency(handler.attachments) {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	actor, ok := legacyAttachmentPrincipal(request, authport.CapabilityMediaLibraryWrite)
	if !ok {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeUnauthorized)
		return
	}
	attachmentID, err := parseLegacyAttachmentID(chi.URLParam(request, "attachment_id"))
	key, keyErr := legacyAttachmentIdempotencyKey(request)
	if err != nil || keyErr != nil || request.URL.RawQuery != "" || request.ContentLength != 0 {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeMalformedRequest)
		return
	}
	result, err := handler.attachments.Delete(request.Context(), mediaport.AttachmentDeleteCommand{AttachmentID: attachmentID, Actor: actor, IdempotencyKey: key})
	if err != nil {
		switch {
		case errors.Is(err, mediaapp.ErrAttachmentHasReferences):
			platformhttp.MarkCompatibilityError(writer, platformhttp.CodeConflict)
			writeLegacyAttachmentJSON(writer, http.StatusConflict, legacyAttachmentDeleteConflict{Error: "attachment_has_references", References: legacyAttachmentReferences(result.References)})
		case errors.Is(err, mediaapp.ErrAttachmentNotFound):
			writeLegacyAttachmentError(writer, request, platformhttp.CodeNotFound)
		case errors.Is(err, mediaapp.ErrAttachmentConflict):
			writeLegacyAttachmentError(writer, request, platformhttp.CodeConflict)
		default:
			writeLegacyAttachmentError(writer, request, platformhttp.CodeDependencyUnavailable)
		}
		return
	}
	if result.ID != attachmentID || !result.Deleted || !result.HardDeleted || result.References.Any() {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	writeLegacyAttachmentJSON(writer, http.StatusOK, legacyAttachmentDeleteSuccess{ID: result.ID, Deleted: true, HardDeleted: true})
}

func (handler *Handler) DownloadAttachment(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || request == nil || request.URL == nil || nilLegacyDependency(handler.attachments) {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	if _, ok := legacyAttachmentPrincipal(request, authport.CapabilityMediaLibraryRead); !ok {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeUnauthorized)
		return
	}
	attachmentID, err := parseLegacyAttachmentID(chi.URLParam(request, "attachment_id"))
	if err != nil || request.URL.RawQuery != "" {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeMalformedRequest)
		return
	}
	download, err := handler.attachments.Download(request.Context(), attachmentID)
	if err != nil {
		if errors.Is(err, mediaapp.ErrAttachmentNotFound) {
			writeLegacyAttachmentError(writer, request, platformhttp.CodeNotFound)
		} else {
			writeLegacyAttachmentError(writer, request, platformhttp.CodeDependencyUnavailable)
		}
		return
	}
	if !validLegacyAttachment(download.Attachment) || int64(len(download.Content)) != download.Attachment.FileSize {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": download.Attachment.FileName})
	if disposition == "" {
		writeLegacyAttachmentError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	writer.Header().Set("Content-Type", "application/pdf")
	writer.Header().Set("Content-Length", strconv.FormatInt(int64(len(download.Content)), 10))
	writer.Header().Set("Content-Disposition", disposition)
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("Content-Security-Policy", "sandbox")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(download.Content)
}

func parseLegacyAttachmentListQuery(rawQuery string) (mediaport.AttachmentListQuery, error) {
	if !utf8.ValidString(rawQuery) {
		return mediaport.AttachmentListQuery{}, errInvalidAttachmentRequest
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return mediaport.AttachmentListQuery{}, errInvalidAttachmentRequest
	}
	for key, entries := range values {
		if (key != "limit" && key != "offset" && key != "enabled_only" && key != "q") || len(entries) != 1 || !utf8.ValidString(key) || !utf8.ValidString(entries[0]) {
			return mediaport.AttachmentListQuery{}, errInvalidAttachmentRequest
		}
	}
	limit, err := parseLegacyAttachmentPageNumber(values, "limit", mediaport.DefaultAttachmentListLimit, 1, mediaport.MaximumAttachmentListLimit)
	if err != nil {
		return mediaport.AttachmentListQuery{}, err
	}
	offset, err := parseLegacyAttachmentPageNumber(values, "offset", 0, 0, 1<<62)
	if err != nil {
		return mediaport.AttachmentListQuery{}, err
	}
	enabledOnly := true
	if value, found := values["enabled_only"]; found {
		switch value[0] {
		case "true":
		case "false":
			enabledOnly = false
		default:
			return mediaport.AttachmentListQuery{}, errInvalidAttachmentRequest
		}
	}
	search := ""
	if value, found := values["q"]; found {
		search = strings.TrimSpace(value[0])
		if utf8.RuneCountInString(search) > 200 {
			return mediaport.AttachmentListQuery{}, errInvalidAttachmentRequest
		}
	}
	return mediaport.AttachmentListQuery{Limit: limit, Offset: offset, EnabledOnly: enabledOnly, Search: search}, nil
}

func parseLegacyAttachmentPageNumber(values url.Values, key string, fallback, minimum, maximum int64) (int64, error) {
	entries, found := values[key]
	if !found {
		return fallback, nil
	}
	value := entries[0]
	if value == "" || !allASCIIDigits(value) || len(value) > 19 {
		return 0, errInvalidAttachmentRequest
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, errInvalidAttachmentRequest
	}
	return parsed, nil
}

func parseLegacyAttachmentUpload(writer http.ResponseWriter, request *http.Request, actor int64) (mediaport.AttachmentUploadCommand, error) {
	if request == nil || request.Body == nil || actor < 1 {
		return mediaport.AttachmentUploadCommand{}, errInvalidAttachmentRequest
	}
	key, err := legacyAttachmentIdempotencyKey(request)
	if err != nil {
		return mediaport.AttachmentUploadCommand{}, err
	}
	mediaType, params, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") || params["boundary"] == "" {
		return mediaport.AttachmentUploadCommand{}, errInvalidAttachmentRequest
	}
	request.Body = http.MaxBytesReader(writer, request.Body, legacyAttachmentMaxBody)
	reader, err := request.MultipartReader()
	if err != nil {
		return mediaport.AttachmentUploadCommand{}, errInvalidAttachmentRequest
	}
	var (
		command   = mediaport.AttachmentUploadCommand{Actor: actor, IdempotencyKey: key, Tags: []string{}}
		seen      = map[string]bool{}
		hasUpload bool
	)
	for parts := 0; ; parts++ {
		part, readErr := reader.NextPart()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return mediaport.AttachmentUploadCommand{}, errInvalidAttachmentRequest
		}
		if parts >= 3 {
			part.Close()
			return mediaport.AttachmentUploadCommand{}, errInvalidAttachmentRequest
		}
		name := part.FormName()
		if name == "" || seen[name] {
			part.Close()
			return mediaport.AttachmentUploadCommand{}, errInvalidAttachmentRequest
		}
		seen[name] = true
		switch name {
		case "attachment":
			if part.FileName() == "" || part.Header.Get("Content-Type") == "" {
				part.Close()
				return mediaport.AttachmentUploadCommand{}, errInvalidAttachmentRequest
			}
			fileName, declaredType := part.FileName(), part.Header.Get("Content-Type")
			content, contentErr := domain.ReadAttachmentBounded(part)
			part.Close()
			if contentErr != nil {
				return mediaport.AttachmentUploadCommand{}, errInvalidAttachmentRequest
			}
			command.FileName, command.DeclaredType, command.Content, hasUpload = fileName, declaredType, content, true
		case "name", "tags":
			if part.FileName() != "" {
				part.Close()
				return mediaport.AttachmentUploadCommand{}, errInvalidAttachmentRequest
			}
			value, valueErr := readLegacyAttachmentFormText(part, 16<<10)
			part.Close()
			if valueErr != nil {
				return mediaport.AttachmentUploadCommand{}, errInvalidAttachmentRequest
			}
			if name == "name" {
				command.Name = value
			} else {
				command.Tags, valueErr = parseLegacyAttachmentTags(value)
				if valueErr != nil {
					return mediaport.AttachmentUploadCommand{}, errInvalidAttachmentRequest
				}
			}
		default:
			part.Close()
			return mediaport.AttachmentUploadCommand{}, errInvalidAttachmentRequest
		}
	}
	if !hasUpload {
		return mediaport.AttachmentUploadCommand{}, errInvalidAttachmentRequest
	}
	return command, nil
}

func readLegacyAttachmentFormText(part io.Reader, maximum int64) (string, error) {
	if part == nil || maximum < 1 {
		return "", errInvalidAttachmentRequest
	}
	value, err := io.ReadAll(io.LimitReader(part, maximum+1))
	if err != nil || int64(len(value)) > maximum || !utf8.Valid(value) {
		return "", errInvalidAttachmentRequest
	}
	return string(value), nil
}

func parseLegacyAttachmentTags(value string) ([]string, error) {
	if value == "" {
		return []string{}, nil
	}
	values := strings.Split(value, ",")
	if len(values) > 50 {
		return nil, errInvalidAttachmentRequest
	}
	for index := range values {
		values[index] = strings.TrimSpace(values[index])
		if values[index] == "" || !utf8.ValidString(values[index]) || utf8.RuneCountInString(values[index]) > 64 {
			return nil, errInvalidAttachmentRequest
		}
		for prior := 0; prior < index; prior++ {
			if values[prior] == values[index] {
				return nil, errInvalidAttachmentRequest
			}
		}
	}
	return values, nil
}

func parseLegacyAttachmentUpdate(writer http.ResponseWriter, request *http.Request, attachmentID, actor int64) (mediaport.AttachmentUpdateCommand, error) {
	if request == nil || request.Body == nil || attachmentID < 1 || actor < 1 || !legacyAttachmentJSONContentType(request.Header.Get("Content-Type")) {
		return mediaport.AttachmentUpdateCommand{}, errInvalidAttachmentRequest
	}
	key, err := legacyAttachmentIdempotencyKey(request)
	if err != nil {
		return mediaport.AttachmentUpdateCommand{}, err
	}
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, legacyAttachmentUpdateMaxBody))
	if err != nil || !utf8.Valid(body) {
		return mediaport.AttachmentUpdateCommand{}, errInvalidAttachmentRequest
	}
	values, err := legacyAttachmentJSONObject(body, map[string]bool{"expected_version": true, "name": true, "description": true, "tags": true, "enabled": true})
	if err != nil {
		return mediaport.AttachmentUpdateCommand{}, err
	}
	var command mediaport.AttachmentUpdateCommand
	command.AttachmentID, command.Actor, command.IdempotencyKey = attachmentID, actor, key
	if err = json.Unmarshal(values["expected_version"], &command.ExpectedVersion); err != nil || command.ExpectedVersion < 1 ||
		json.Unmarshal(values["name"], &command.Name) != nil || !utf8.ValidString(command.Name) ||
		json.Unmarshal(values["description"], &command.Description) != nil || !utf8.ValidString(command.Description) ||
		json.Unmarshal(values["tags"], &command.Tags) != nil || command.Tags == nil ||
		json.Unmarshal(values["enabled"], &command.Enabled) != nil {
		return mediaport.AttachmentUpdateCommand{}, errInvalidAttachmentRequest
	}
	for _, tag := range command.Tags {
		if !utf8.ValidString(tag) {
			return mediaport.AttachmentUpdateCommand{}, errInvalidAttachmentRequest
		}
	}
	return command, nil
}

func legacyAttachmentJSONObject(body []byte, allowed map[string]bool) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errInvalidAttachmentRequest
	}
	values := make(map[string]json.RawMessage, len(allowed))
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || !allowed[key] || !utf8.ValidString(key) {
			return nil, errInvalidAttachmentRequest
		}
		if _, exists := values[key]; exists {
			return nil, errInvalidAttachmentRequest
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, errInvalidAttachmentRequest
		}
		values[key] = append(json.RawMessage(nil), value...)
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, errInvalidAttachmentRequest
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errInvalidAttachmentRequest
	}
	if len(values) != len(allowed) {
		return nil, errInvalidAttachmentRequest
	}
	return values, nil
}

func parseLegacyAttachmentID(value string) (int64, error) {
	if !attachmentIDPattern.MatchString(value) {
		return 0, errInvalidAttachmentRequest
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 {
		return 0, errInvalidAttachmentRequest
	}
	return id, nil
}

func legacyAttachmentIdempotencyKey(request *http.Request) (string, error) {
	if request == nil {
		return "", errInvalidAttachmentRequest
	}
	values := request.Header.Values("Idempotency-Key")
	if len(values) != 1 || !utf8.ValidString(values[0]) || values[0] != strings.TrimSpace(values[0]) || len(values[0]) < 16 || len(values[0]) > 128 {
		return "", errInvalidAttachmentRequest
	}
	return values[0], nil
}

func legacyAttachmentPrincipal(request *http.Request, capability authport.Capability) (int64, bool) {
	if request == nil || (capability != authport.CapabilityMediaLibraryRead && capability != authport.CapabilityMediaLibraryWrite) {
		return 0, false
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	return principal.AdminUserID, principalOK && principal.AdminUserID > 0 && (principal.Role == authport.RoleAdmin || principal.Role == authport.RoleOps) &&
		authorizationOK && authorization.Capability == capability && authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}

func legacyAttachmentJSONContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && strings.EqualFold(mediaType, "application/json")
}

func validLegacyAttachmentPage(page mediaport.AttachmentListPage, query mediaport.AttachmentListQuery) bool {
	count := int64(len(page.Items))
	if page.Limit != query.Limit || page.Offset != query.Offset || page.Total < 0 || count > page.Limit || count > page.Total ||
		count == 0 && page.Offset < page.Total || count > 0 && page.Offset > page.Total-count {
		return false
	}
	for _, attachment := range page.Items {
		if !validLegacyAttachment(attachment) {
			return false
		}
	}
	return true
}

func validLegacyAttachment(attachment mediaport.Attachment) bool {
	if attachment.ID < 1 || attachment.Name == "" || attachment.FileName == "" || attachment.MimeType != "application/pdf" ||
		attachment.FileSize < 1 || attachment.FileSize > domain.MaxAttachmentBytes || attachment.Version < 1 || attachment.CreatedBy < 1 || attachment.UpdatedBy < 1 ||
		attachment.CreatedAt.IsZero() || attachment.UpdatedAt.IsZero() || attachment.UpdatedAt.Before(attachment.CreatedAt) ||
		!validLegacyAttachmentText(attachment.Name, 200, false) || !validLegacyAttachmentFileName(attachment.FileName) || !validLegacyAttachmentText(attachment.Description, 10_000, true) || len(attachment.Tags) > 50 {
		return false
	}
	seen := make(map[string]struct{}, len(attachment.Tags))
	for _, value := range attachment.Tags {
		if !validLegacyAttachmentText(value, 64, false) || strings.Contains(value, ",") {
			return false
		}
		if _, duplicate := seen[value]; duplicate {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validLegacyAttachmentText(value string, maximum int, allowEmpty bool) bool {
	return utf8.ValidString(value) && (allowEmpty || value != "") && value == strings.TrimSpace(value) && utf8.RuneCountInString(value) <= maximum
}

func validLegacyAttachmentFileName(value string) bool {
	if !validLegacyAttachmentText(value, 255, false) || value == "." || value == ".." || strings.ContainsAny(value, `/\\`) {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func projectLegacyAttachment(attachment mediaport.Attachment) legacyAttachmentItem {
	return legacyAttachmentItem{
		ID: attachment.ID, Name: attachment.Name, FileName: attachment.FileName, MimeType: attachment.MimeType, FileSize: attachment.FileSize,
		Description: attachment.Description, Tags: append([]string{}, attachment.Tags...), Enabled: attachment.Enabled, Version: attachment.Version,
		CreatedBy: attachment.CreatedBy, UpdatedBy: attachment.UpdatedBy, CreatedAt: attachment.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: attachment.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func legacyAttachmentReferences(references mediaapp.AttachmentDeleteReferences) legacyAttachmentDeleteReferences {
	return legacyAttachmentDeleteReferences{
		AutomationAgents: legacyAttachmentReferenceIDs(references.AutomationAgents),
		Channels:         legacyAttachmentReferenceIDs(references.Channels),
		RadarLinks:       legacyAttachmentReferenceIDs(references.RadarLinks),
	}
}

func legacyAttachmentReferenceIDs(ids []int64) []legacyAttachmentReferenceID {
	values := make([]legacyAttachmentReferenceID, len(ids))
	for index, id := range ids {
		values[index] = legacyAttachmentReferenceID{ID: id}
	}
	return values
}

func allASCIIDigits(value string) bool {
	for _, value := range []byte(value) {
		if value < '0' || value > '9' {
			return false
		}
	}
	return true
}

func writeLegacyAttachmentJSON(writer http.ResponseWriter, status int, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_, _ = writer.Write(payload)
}

func writeLegacyAttachmentError(writer http.ResponseWriter, request *http.Request, code platformhttp.ErrorCode) {
	status, message := http.StatusServiceUnavailable, "A required dependency is unavailable."
	switch code {
	case platformhttp.CodeMalformedRequest:
		status, message = http.StatusBadRequest, "The request is malformed."
	case platformhttp.CodeUnauthorized:
		status, message = http.StatusForbidden, "Permission is denied."
	case platformhttp.CodeNotFound:
		status, message = http.StatusNotFound, "The resource was not found."
	case platformhttp.CodeConflict:
		status, message = http.StatusConflict, "The request conflicts with the current state."
	case platformhttp.CodeDependencyUnavailable:
	default:
		code = platformhttp.CodeDependencyUnavailable
	}
	platformhttp.MarkCompatibilityError(writer, code)
	requestID := "unknown"
	if request != nil && platformhttp.RequestID(request.Context()) != "" {
		requestID = platformhttp.RequestID(request.Context())
	}
	writeLegacyAttachmentJSON(writer, status, struct {
		Code      platformhttp.ErrorCode `json:"code"`
		Message   string                 `json:"message"`
		RequestID string                 `json:"request_id"`
	}{Code: code, Message: message, RequestID: requestID})
}

func writeLegacyAttachmentCollectionMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	writeLegacyAttachmentMethodNotAllowed(writer, "GET, POST")
}

func writeLegacyAttachmentUploadMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	writeLegacyAttachmentMethodNotAllowed(writer, "POST")
}

func writeLegacyAttachmentDetailMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	writeLegacyAttachmentMethodNotAllowed(writer, "GET, PUT, DELETE")
}

func writeLegacyAttachmentDownloadMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	writeLegacyAttachmentMethodNotAllowed(writer, "GET")
}

func writeLegacyAttachmentMethodNotAllowed(writer http.ResponseWriter, allow string) {
	writer.Header().Set("Allow", allow)
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Content-Security-Policy", "sandbox")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.WriteHeader(http.StatusMethodNotAllowed)
}

func legacyAttachmentSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		next.ServeHTTP(legacyAttachmentHeaderWriter{ResponseWriter: writer}, request)
	})
}

type legacyAttachmentHeaderWriter struct{ http.ResponseWriter }

func (writer legacyAttachmentHeaderWriter) WriteHeader(status int) {
	switch status {
	case http.StatusBadRequest:
		platformhttp.MarkCompatibilityError(writer.ResponseWriter, platformhttp.CodeMalformedRequest)
	case http.StatusUnauthorized:
		platformhttp.MarkCompatibilityError(writer.ResponseWriter, platformhttp.CodeUnauthenticated)
	case http.StatusForbidden:
		platformhttp.MarkCompatibilityError(writer.ResponseWriter, platformhttp.CodeUnauthorized)
	case http.StatusNotFound:
		platformhttp.MarkCompatibilityError(writer.ResponseWriter, platformhttp.CodeNotFound)
	case http.StatusConflict:
		platformhttp.MarkCompatibilityError(writer.ResponseWriter, platformhttp.CodeConflict)
	case http.StatusServiceUnavailable:
		platformhttp.MarkCompatibilityError(writer.ResponseWriter, platformhttp.CodeDependencyUnavailable)
	}
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Content-Security-Policy", "sandbox")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.ResponseWriter.WriteHeader(status)
}

func (writer legacyAttachmentHeaderWriter) Write(payload []byte) (int, error) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Content-Security-Policy", "sandbox")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	return writer.ResponseWriter.Write(payload)
}
