package main

import (
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
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

func (handler *Handler) ListChannels(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.channels) || request == nil {
		writeLegacyChannelError(writer, contactapp.ErrChannelUnavailable)
		return
	}
	query := request.URL.Query()
	for key := range query {
		if key != "limit" && key != "status" && key != "include_archived" {
			writeLegacyChannelError(writer, contactapp.ErrInvalidChannel)
			return
		}
	}
	limit := int64(100)
	var err error
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		limit, err = strconv.ParseInt(raw, 10, 32)
	}
	includeArchived := false
	if raw := strings.TrimSpace(query.Get("include_archived")); raw != "" {
		includeArchived, err = strconv.ParseBool(raw)
	}
	if err != nil || limit < 1 || limit > int64(contactapp.MaximumChannelListLimit) {
		writeLegacyChannelError(writer, contactapp.ErrInvalidChannel)
		return
	}
	channels, err := handler.channels.ListChannels(request.Context(), int32(limit), query.Get("status"), includeArchived)
	if err != nil {
		writeLegacyChannelError(writer, err)
		return
	}
	items := make([]map[string]any, len(channels))
	for index, channel := range channels {
		items[index], err = legacyChannel(channel)
		if err != nil {
			writeLegacyChannelError(writer, err)
			return
		}
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "channels": items, "reason": "channels_listed", "source": "ai_crm_next"})
}

func (handler *Handler) GetChannel(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.channels) || request == nil {
		writeLegacyChannelError(writer, contactapp.ErrChannelUnavailable)
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(request, "channel_id")), 10, 64)
	if err != nil || id < 1 {
		writeLegacyChannelError(writer, contactapp.ErrChannelNotFound)
		return
	}
	channel, err := handler.channels.GetChannel(request.Context(), id)
	if err != nil {
		writeLegacyChannelError(writer, err)
		return
	}
	mapped, err := legacyChannel(channel)
	if err != nil {
		writeLegacyChannelError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "channel": mapped, "reason": "channel_loaded", "source": "ai_crm_next"})
}

func (handler *Handler) CreateChannel(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.channels) || request == nil {
		writeLegacyChannelError(writer, contactapp.ErrChannelUnavailable)
		return
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		writeLegacyChannelError(writer, authport.ErrUnauthorized)
		return
	}
	body, values, err := legacyChannelBody(writer, request)
	if err != nil {
		writeLegacyChannelError(writer, err)
		return
	}
	key, err := channelIdempotencyKey(request, "legacy-channel-create")
	if err != nil {
		writeLegacyChannelError(writer, err)
		return
	}
	channel, err := handler.channels.CreateChannel(request.Context(), contactapp.CreateChannelCommand{Actor: principal.AdminUserID, IdempotencyKey: key, ChannelCode: stringField(values, "channel_code"), ChannelName: stringField(values, "channel_name"), Status: stringField(values, "status"), LegacyProjection: body})
	if err != nil {
		writeLegacyChannelError(writer, err)
		return
	}
	mapped, err := legacyChannel(channel)
	if err != nil {
		writeLegacyChannelError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{"ok": true, "channel": mapped, "reason": "channel_created", "source": "ai_crm_next", "fallback_used": false, "real_external_call_executed": false})
}

func (handler *Handler) UpdateChannel(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.channels) || request == nil {
		writeLegacyChannelError(writer, contactapp.ErrChannelUnavailable)
		return
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		writeLegacyChannelError(writer, authport.ErrUnauthorized)
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(request, "channel_id")), 10, 64)
	if err != nil || id < 1 {
		writeLegacyChannelError(writer, contactapp.ErrChannelNotFound)
		return
	}
	body, values, err := legacyChannelBody(writer, request)
	if err != nil {
		writeLegacyChannelError(writer, err)
		return
	}
	if len(values) == 1 {
		if raw, ok := values["status"]; ok {
			var status string
			if json.Unmarshal(raw, &status) != nil || status != "active" && status != "inactive" && status != "archived" {
				writeLegacyChannelError(writer, contactapp.ErrInvalidChannel)
				return
			}
		}
	}
	key, err := channelIdempotencyKey(request, "legacy-channel-update")
	if err != nil {
		writeLegacyChannelError(writer, err)
		return
	}
	channel, err := handler.channels.UpdateChannel(request.Context(), contactapp.UpdateChannelCommand{Actor: principal.AdminUserID, ChannelID: id, IdempotencyKey: key, Patch: body})
	if err != nil {
		writeLegacyChannelError(writer, err)
		return
	}
	mapped, err := legacyChannel(channel)
	if err != nil {
		writeLegacyChannelError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "channel": mapped, "reason": "channel_updated", "source": "ai_crm_next", "fallback_used": false, "real_external_call_executed": false})
}

func legacyChannelBody(writer http.ResponseWriter, request *http.Request) (json.RawMessage, map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 256<<10))
	decoder.UseNumber()
	var values map[string]json.RawMessage
	if decoder.Decode(&values) != nil || values == nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		return nil, nil, contactapp.ErrInvalidChannel
	}
	allowed := map[string]bool{"channel_type": true, "carrier_type": true, "channel_name": true, "channel_code": true, "scene_value": true, "qr_url": true, "status": true, "owner_staff_id": true, "customer_channel": true, "link_url": true, "final_url": true, "welcome_message": true, "welcome_image_library_ids": true, "welcome_miniprogram_library_ids": true, "welcome_attachment_library_ids": true, "welcome_group_invite_library_ids": true, "auto_accept_friend": true, "entry_tag_id": true, "entry_tag_name": true, "entry_tag_group_name": true, "assignment_mode": true, "assignment_strategy": true, "overflow_policy": true, "assignment_config_json": true}
	for key := range values {
		if !allowed[key] {
			return nil, nil, contactapp.ErrInvalidChannel
		}
	}
	body, err := json.Marshal(values)
	if err != nil {
		return nil, nil, contactapp.ErrInvalidChannel
	}
	return body, values, nil
}
func stringField(values map[string]json.RawMessage, key string) string {
	var value string
	_ = json.Unmarshal(values[key], &value)
	return value
}
func channelIdempotencyKey(request *http.Request, prefix string) (string, error) {
	if key := strings.TrimSpace(request.Header.Get("Idempotency-Key")); key != "" {
		return key, nil
	}
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", contactapp.ErrChannelUnavailable
	}
	return prefix + ":" + hex.EncodeToString(value[:]), nil
}
func legacyChannel(channel contactapp.Channel) (map[string]any, error) {
	var result map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(channel.LegacyProjection)))
	decoder.UseNumber()
	if decoder.Decode(&result) != nil || result == nil {
		return nil, contactapp.ErrChannelUnavailable
	}
	result["id"] = channel.ID
	result["channel_code"] = channel.ChannelCode
	result["channel_name"] = channel.ChannelName
	result["status"] = channel.Status
	result["created_at"] = channel.CreatedAt.UTC()
	result["updated_at"] = channel.UpdatedAt.UTC()
	result["assignees"] = []any{}
	result["assignment_stats_24h"] = []any{}
	result["assignee_count"] = 0
	result["channel_contact_count"] = 0
	result["latest_channel_entered_at"] = ""
	result["qrcode_asset_id"] = 0
	result["qrcode_status"] = "not_generated"
	result["qr_download_url"] = ""
	result["share_url"] = ""
	result["copy_text"] = ""
	return result, nil
}
func writeLegacyChannelError(writer http.ResponseWriter, err error) {
	status, code := http.StatusServiceUnavailable, platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, contactapp.ErrInvalidChannel):
		status, code = http.StatusBadRequest, platformhttp.CodeMalformedRequest
	case errors.Is(err, contactapp.ErrChannelNotFound):
		status, code = http.StatusNotFound, platformhttp.CodeNotFound
	case errors.Is(err, contactapp.ErrChannelConflict):
		status, code = http.StatusConflict, platformhttp.CodeConflict
	case errors.Is(err, authport.ErrUnauthorized):
		status, code = http.StatusForbidden, platformhttp.CodeUnauthorized
	}
	platformhttp.MarkCompatibilityError(writer, code)
	writeJSON(writer, status, map[string]any{"ok": false, "detail": err.Error()})
}
