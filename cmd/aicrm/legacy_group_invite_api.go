package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type groupInviteApplication interface {
	List(context.Context, mediaport.GroupInviteListQuery) (mediaport.GroupInvitePage, error)
	Get(context.Context, int64) (mediaport.GroupInvite, error)
	Create(context.Context, mediaport.GroupInviteCreateCommand) (mediaport.GroupInvite, error)
	Update(context.Context, mediaport.GroupInviteUpdateCommand) (mediaport.GroupInvite, error)
	Archive(context.Context, mediaport.GroupInviteArchiveCommand) (mediaport.GroupInvite, error)
}

type legacyGroupInviteRequest struct {
	Name         *string `json:"name"`
	Title        *string `json:"title"`
	Description  *string `json:"description"`
	JoinURL      *string `json:"join_url"`
	CoverImageID *int64  `json:"cover_image_id"`
	Enabled      *bool   `json:"enabled"`
}

func (handler *Handler) ListGroupInvites(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.groupInvites) || request == nil {
		writeGroupInviteError(writer, mediaapp.ErrGroupInviteUnavailable)
		return
	}
	query, err := legacyGroupInviteQuery(request)
	if err != nil {
		writeGroupInviteError(writer, err)
		return
	}
	page, err := handler.groupInvites.List(request.Context(), query)
	if err != nil {
		writeGroupInviteError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "items": page.Items, "group_invites": page.Items,
		"total": page.Total, "limit": page.Limit, "offset": page.Offset, "provider_call_executed": false})
}

func (handler *Handler) GetGroupInvite(writer http.ResponseWriter, request *http.Request) {
	id, err := groupInviteID(handler, request)
	if err != nil {
		writeGroupInviteError(writer, err)
		return
	}
	item, err := handler.groupInvites.Get(request.Context(), id)
	if err != nil {
		writeGroupInviteError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "item": item, "group_invite": item, "provider_call_executed": false})
}

func (handler *Handler) CreateGroupInvite(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.groupInvites) || request == nil {
		writeGroupInviteError(writer, mediaapp.ErrGroupInviteUnavailable)
		return
	}
	principal, body, err := groupInviteBody(writer, request)
	if err != nil || body.Title == nil || body.JoinURL == nil {
		if err == nil {
			err = mediaapp.ErrInvalidGroupInviteOperation
		}
		writeGroupInviteError(writer, err)
		return
	}
	key, err := groupInviteKey(request, "create", 0)
	if err != nil {
		writeGroupInviteError(writer, err)
		return
	}
	name, description := "", ""
	if body.Name != nil {
		name = *body.Name
	}
	if body.Description != nil {
		description = *body.Description
	}
	cover := int64(0)
	if body.CoverImageID != nil {
		cover = *body.CoverImageID
	}
	item, err := handler.groupInvites.Create(request.Context(), mediaport.GroupInviteCreateCommand{
		Name: name, Title: *body.Title, Description: description, JoinURL: *body.JoinURL,
		CoverImageID: cover, Enabled: body.Enabled, Actor: principal.AdminUserID, IdempotencyKey: key,
	})
	if err != nil {
		writeGroupInviteError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "item": item, "group_invite": item, "item_id": item.ID,
		"local_only": true, "provider_call_executed": false, "real_external_call_executed": false})
}

func (handler *Handler) UpdateGroupInvite(writer http.ResponseWriter, request *http.Request) {
	id, err := groupInviteID(handler, request)
	if err != nil {
		writeGroupInviteError(writer, err)
		return
	}
	principal, body, err := groupInviteBody(writer, request)
	if err != nil || emptyLegacyGroupInviteRequest(body) {
		if err == nil {
			err = mediaapp.ErrInvalidGroupInviteOperation
		}
		writeGroupInviteError(writer, err)
		return
	}
	key, err := groupInviteKey(request, "update", id)
	if err != nil {
		writeGroupInviteError(writer, err)
		return
	}
	item, err := handler.groupInvites.Update(request.Context(), mediaport.GroupInviteUpdateCommand{ID: id,
		GroupInvitePatch: mediaport.GroupInvitePatch{Name: body.Name, Title: body.Title, Description: body.Description,
			JoinURL: body.JoinURL, CoverImageID: body.CoverImageID, Enabled: body.Enabled}, Actor: principal.AdminUserID, IdempotencyKey: key})
	if err != nil {
		writeGroupInviteError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "item": item, "group_invite": item,
		"local_only": true, "provider_call_executed": false, "real_external_call_executed": false})
}

func emptyLegacyGroupInviteRequest(body legacyGroupInviteRequest) bool {
	return body.Name == nil && body.Title == nil && body.Description == nil && body.JoinURL == nil && body.CoverImageID == nil && body.Enabled == nil
}

func (handler *Handler) ArchiveGroupInvite(writer http.ResponseWriter, request *http.Request) {
	id, err := groupInviteID(handler, request)
	if err != nil {
		writeGroupInviteError(writer, err)
		return
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		writeGroupInviteError(writer, authport.ErrUnauthorized)
		return
	}
	key, err := groupInviteKey(request, "archive", id)
	if err != nil {
		writeGroupInviteError(writer, err)
		return
	}
	item, err := handler.groupInvites.Archive(request.Context(), mediaport.GroupInviteArchiveCommand{ID: id, Actor: principal.AdminUserID, IdempotencyKey: key})
	if err != nil {
		writeGroupInviteError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "item": item, "archived": true,
		"local_only": true, "provider_call_executed": false, "real_external_call_executed": false})
}

func legacyGroupInviteQuery(request *http.Request) (mediaport.GroupInviteListQuery, error) {
	query := mediaport.GroupInviteListQuery{Limit: mediaapp.DefaultGroupInviteLimit, EnabledOnly: true}
	for key := range request.URL.Query() {
		if key != "limit" && key != "offset" && key != "enabled_only" && key != "q" {
			return query, mediaapp.ErrInvalidGroupInviteOperation
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
		return query, mediaapp.ErrInvalidGroupInviteOperation
	}
	return query, nil
}

func groupInviteID(handler *Handler, request *http.Request) (int64, error) {
	if handler == nil || nilLegacyDependency(handler.groupInvites) || request == nil {
		return 0, mediaapp.ErrGroupInviteUnavailable
	}
	id, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(request, "item_id")), 10, 64)
	if err != nil || id < 1 {
		return 0, mediaapp.ErrInvalidGroupInviteOperation
	}
	return id, nil
}

func groupInviteBody(writer http.ResponseWriter, request *http.Request) (authport.Principal, legacyGroupInviteRequest, error) {
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		return authport.Principal{}, legacyGroupInviteRequest{}, authport.ErrUnauthorized
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 64<<10))
	decoder.DisallowUnknownFields()
	var body legacyGroupInviteRequest
	if decoder.Decode(&body) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		return principal, body, mediaapp.ErrInvalidGroupInviteOperation
	}
	return principal, body, nil
}

func groupInviteKey(request *http.Request, operation string, id int64) (string, error) {
	if key := request.Header.Get("Idempotency-Key"); key != "" {
		return key, nil
	}
	if operation == "archive" {
		return "legacy-group-invite:archive:" + strconv.FormatInt(id, 10), nil
	}
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", mediaapp.ErrGroupInviteUnavailable
	}
	return "legacy-group-invite:" + operation + ":" + hex.EncodeToString(value[:]), nil
}

func writeGroupInviteError(writer http.ResponseWriter, err error) {
	status, code := http.StatusServiceUnavailable, platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, mediaapp.ErrInvalidGroupInviteOperation), errors.Is(err, mediaapp.ErrGroupInviteInvalidReference):
		status, code = http.StatusBadRequest, platformhttp.CodeMalformedRequest
	case errors.Is(err, mediaapp.ErrGroupInviteNotFound):
		status, code = http.StatusNotFound, platformhttp.CodeNotFound
	case errors.Is(err, mediaapp.ErrGroupInviteConflict), errors.Is(err, mediaapp.ErrGroupInviteHasReferences):
		status, code = http.StatusConflict, platformhttp.CodeConflict
	case errors.Is(err, authport.ErrUnauthorized):
		status, code = http.StatusForbidden, platformhttp.CodeUnauthorized
	}
	platformhttp.MarkCompatibilityError(writer, code)
	writeJSON(writer, status, map[string]any{"ok": false, "message": err.Error(), "detail": err.Error(),
		"local_only": true, "provider_call_executed": false, "real_external_call_executed": false})
}
