package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

func (*Handler) MiniProgramLibraryPage(writer http.ResponseWriter, request *http.Request) {
	if request == nil {
		return
	}
	http.Redirect(writer, request, "/?legacy_admin_path="+url.QueryEscape("/admin/miniprogram-library"), http.StatusFound)
}

func (handler *Handler) ListMiniPrograms(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.miniPrograms) || request == nil {
		writeMiniProgramError(writer, mediaapp.ErrMiniProgramUnavailable)
		return
	}
	query, err := legacyMiniProgramQuery(request)
	if err != nil {
		writeMiniProgramError(writer, err)
		return
	}
	page, err := handler.miniPrograms.List(request.Context(), query)
	if err != nil {
		writeMiniProgramError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "items": page.Items, "miniprograms": page.Items,
		"total": page.Total, "limit": page.Limit, "offset": page.Offset, "local_only": true,
		"provider_call_executed": false, "real_external_call_executed": false})
}

func (handler *Handler) GetMiniProgram(writer http.ResponseWriter, request *http.Request) {
	id, err := miniProgramID(handler, request)
	if err != nil {
		writeMiniProgramError(writer, err)
		return
	}
	item, err := handler.miniPrograms.Get(request.Context(), id)
	if err != nil {
		writeMiniProgramError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "item": item, "miniprogram": item,
		"local_only": true, "provider_call_executed": false, "real_external_call_executed": false})
}

func (handler *Handler) CreateMiniProgram(writer http.ResponseWriter, request *http.Request) {
	principal, patch, err := miniProgramBody(handler, writer, request)
	if err != nil {
		writeMiniProgramError(writer, err)
		return
	}
	key, err := miniProgramKey(request, "create", 0)
	if err != nil {
		writeMiniProgramError(writer, err)
		return
	}
	command := mediaport.MiniProgramCreateCommand{ThumbnailImageID: patch.ThumbnailImageID.Value,
		ThumbMediaID: patch.ThumbMediaID, ResolveThumbMedia: patch.ResolveThumbMedia, Enabled: patch.Enabled,
		Actor: principal.AdminUserID, IdempotencyKey: key}
	if patch.Name != nil {
		command.Name = *patch.Name
	}
	if patch.AppID != nil {
		command.AppID = *patch.AppID
	}
	if patch.PagePath != nil {
		command.PagePath = *patch.PagePath
	}
	if patch.Title != nil {
		command.Title = *patch.Title
	}
	result, err := handler.miniPrograms.Create(request.Context(), command)
	if err != nil {
		writeMiniProgramError(writer, err)
		return
	}
	writeMiniProgramMutation(writer, result)
}

func (handler *Handler) UpdateMiniProgram(writer http.ResponseWriter, request *http.Request) {
	id, err := miniProgramID(handler, request)
	if err != nil {
		writeMiniProgramError(writer, err)
		return
	}
	principal, patch, err := miniProgramBody(handler, writer, request)
	if err != nil || emptyMiniProgramPatch(patch) {
		if err == nil {
			err = mediaapp.ErrInvalidMiniProgramOperation
		}
		writeMiniProgramError(writer, err)
		return
	}
	key, err := miniProgramKey(request, "update", id)
	if err != nil {
		writeMiniProgramError(writer, err)
		return
	}
	result, err := handler.miniPrograms.Update(request.Context(), mediaport.MiniProgramUpdateCommand{ID: id,
		MiniProgramPatch: patch, Actor: principal.AdminUserID, IdempotencyKey: key})
	if err != nil {
		writeMiniProgramError(writer, err)
		return
	}
	writeMiniProgramMutation(writer, result)
}

func (handler *Handler) DeleteMiniProgram(writer http.ResponseWriter, request *http.Request) {
	id, err := miniProgramID(handler, request)
	if err != nil {
		writeMiniProgramError(writer, err)
		return
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		writeMiniProgramError(writer, authport.ErrUnauthorized)
		return
	}
	key, err := miniProgramKey(request, "delete", id)
	if err != nil {
		writeMiniProgramError(writer, err)
		return
	}
	result, err := handler.miniPrograms.Delete(request.Context(), mediaport.MiniProgramDeleteCommand{ID: id,
		Actor: principal.AdminUserID, IdempotencyKey: key})
	if err != nil {
		writeMiniProgramError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "id": result.ID, "item_id": result.ID,
		"deleted": result.Deleted, "local_only": true, "provider_call_executed": false, "real_external_call_executed": false})
}

func (handler *Handler) TestResolveMiniProgram(writer http.ResponseWriter, request *http.Request) {
	id, err := miniProgramID(handler, request)
	if err != nil {
		writeMiniProgramError(writer, err)
		return
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		writeMiniProgramError(writer, authport.ErrUnauthorized)
		return
	}
	key, err := miniProgramKey(request, "test-resolve", id)
	if err != nil {
		writeMiniProgramError(writer, err)
		return
	}
	result, err := handler.miniPrograms.ResolveThumbnail(request.Context(), mediaport.MiniProgramResolveThumbnailCommand{
		ID: id, Actor: principal.AdminUserID, IdempotencyKey: key})
	if err != nil {
		writeMiniProgramError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "item": result.Item, "miniprogram": result.Item,
		"resolution": result.Resolution, "changed": result.Changed, "thumb_media_id": result.Item.ThumbnailMediaID,
		"local_only": true, "provider_call_executed": false, "real_external_call_executed": false})
}

func legacyMiniProgramQuery(request *http.Request) (mediaport.MiniProgramListQuery, error) {
	query := mediaport.MiniProgramListQuery{Limit: 100, EnabledOnly: true}
	for key := range request.URL.Query() {
		if key != "limit" && key != "offset" && key != "enabled_only" && key != "q" {
			return query, mediaapp.ErrInvalidMiniProgramOperation
		}
	}
	var err error
	if raw := strings.TrimSpace(request.URL.Query().Get("limit")); raw != "" {
		var value int64
		value, err = strconv.ParseInt(raw, 10, 32)
		query.Limit = int32(value)
	}
	if err == nil {
		if raw := strings.TrimSpace(request.URL.Query().Get("offset")); raw != "" {
			var value int64
			value, err = strconv.ParseInt(raw, 10, 32)
			query.Offset = int32(value)
		}
	}
	if err == nil {
		if raw := strings.TrimSpace(request.URL.Query().Get("enabled_only")); raw != "" {
			query.EnabledOnly, err = strconv.ParseBool(raw)
		}
	}
	query.Search = request.URL.Query().Get("q")
	if err != nil {
		return query, mediaapp.ErrInvalidMiniProgramOperation
	}
	return query, nil
}

func miniProgramID(handler *Handler, request *http.Request) (int64, error) {
	if handler == nil || nilLegacyDependency(handler.miniPrograms) || request == nil {
		return 0, mediaapp.ErrMiniProgramUnavailable
	}
	id, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(request, "item_id")), 10, 64)
	if err != nil || id < 1 {
		return 0, mediaapp.ErrInvalidMiniProgramOperation
	}
	return id, nil
}

func miniProgramBody(handler *Handler, writer http.ResponseWriter, request *http.Request) (authport.Principal, mediaport.MiniProgramPatch, error) {
	if handler == nil || nilLegacyDependency(handler.miniPrograms) || request == nil {
		return authport.Principal{}, mediaport.MiniProgramPatch{}, mediaapp.ErrMiniProgramUnavailable
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		return authport.Principal{}, mediaport.MiniProgramPatch{}, authport.ErrUnauthorized
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 64<<10))
	var raw json.RawMessage
	if decoder.Decode(&raw) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		return principal, mediaport.MiniProgramPatch{}, mediaapp.ErrInvalidMiniProgramOperation
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return principal, mediaport.MiniProgramPatch{}, mediaapp.ErrInvalidMiniProgramOperation
	}
	allowed := map[string]bool{"name": true, "appid": true, "app_id": true, "pagepath": true, "page_path": true,
		"title": true, "thumb_image_id": true, "thumb_media_id": true, "resolve_thumb_media": true, "enabled": true}
	for key := range object {
		if !allowed[key] {
			return principal, mediaport.MiniProgramPatch{}, mediaapp.ErrInvalidMiniProgramOperation
		}
	}
	var patch mediaport.MiniProgramPatch
	if json.Unmarshal(raw, &patch) != nil {
		return principal, patch, mediaapp.ErrInvalidMiniProgramOperation
	}
	return principal, patch, nil
}

func emptyMiniProgramPatch(patch mediaport.MiniProgramPatch) bool {
	return patch.Name == nil && patch.AppID == nil && patch.PagePath == nil && patch.Title == nil &&
		!patch.ThumbnailImageID.Present && !patch.ThumbMediaID.Present && patch.ResolveThumbMedia == nil && patch.Enabled == nil
}

func miniProgramKey(request *http.Request, operation string, id int64) (string, error) {
	if request == nil {
		return "", mediaapp.ErrInvalidMiniProgramOperation
	}
	if key := request.Header.Get("Idempotency-Key"); key != "" {
		return key, nil
	}
	if operation == "delete" || operation == "test-resolve" {
		return "legacy-miniprogram:" + operation + ":" + strconv.FormatInt(id, 10), nil
	}
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", mediaapp.ErrMiniProgramUnavailable
	}
	return "legacy-miniprogram:" + operation + ":" + hex.EncodeToString(value[:]), nil
}

func writeMiniProgramMutation(writer http.ResponseWriter, result mediaport.MiniProgramMutationResult) {
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "item": result.Item, "miniprogram": result.Item,
		"item_id": result.Item.ID, "changed": result.Changed, "thumb_resolve": result.ThumbnailResolve,
		"local_only": true, "provider_call_executed": false, "real_external_call_executed": false})
}

func writeMiniProgramError(writer http.ResponseWriter, err error) {
	status, code := http.StatusServiceUnavailable, platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, mediaapp.ErrInvalidMiniProgramOperation), errors.Is(err, mediaapp.ErrMiniProgramImageNotFound):
		status, code = http.StatusBadRequest, platformhttp.CodeMalformedRequest
	case errors.Is(err, mediaapp.ErrMiniProgramNotFound):
		status, code = http.StatusNotFound, platformhttp.CodeNotFound
	case errors.Is(err, mediaapp.ErrMiniProgramConflict):
		status, code = http.StatusConflict, platformhttp.CodeConflict
	case errors.Is(err, authport.ErrUnauthorized):
		status, code = http.StatusForbidden, platformhttp.CodeUnauthorized
	}
	platformhttp.MarkCompatibilityError(writer, code)
	writeJSON(writer, status, map[string]any{"ok": false, "message": err.Error(), "detail": err.Error(),
		"local_only": true, "provider_call_executed": false, "real_external_call_executed": false})
}
