package main

import (
	"crypto/hmac"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	configapp "github.com/qianlan33333-png/AI-CRM-v2/internal/config/app"
	configport "github.com/qianlan33333-png/AI-CRM-v2/internal/config/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const appSettingsResourcePath = "/api/admin/config/app-settings"

// SaveAppSettingsResource is the legacy JSON companion to the existing HTML
// settings form. It deliberately delegates every setting mutation to the
// already-owned Config service: no secret material, second settings store, or
// generic configuration layer is introduced here.
func (handler *Handler) SaveAppSettingsResource(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.settings) || request == nil {
		writeAppSettingsResourceError(writer, http.StatusServiceUnavailable, "settings_unavailable")
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 64<<10)
	values, token, code := decodeAppSettingsResourcePayload(request)
	if code != "" {
		writeAppSettingsResourceError(writer, http.StatusBadRequest, code)
		return
	}
	session, sessionOK := authport.SessionFromContext(request.Context())
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	expected := adminOpsActionToken(session, request.Method, appSettingsResourcePath)
	if !sessionOK || !principalOK || principal.AdminUserID < 1 || len(token) != len(expected) || !hmac.Equal([]byte(token), []byte(expected)) {
		writeAppSettingsResourceError(writer, http.StatusBadRequest, "invalid_action_token")
		return
	}
	before, err := handler.settings.List(request.Context(), configapp.SettingsListInput{})
	if err != nil {
		writeAppSettingsResourceError(writer, http.StatusServiceUnavailable, "settings_unavailable")
		return
	}
	if appSettingsResourceHasRawSecret(values, before.MetadataMap) {
		writeAppSettingsResourceError(writer, http.StatusBadRequest, "secret_input_forbidden")
		return
	}
	if err := handler.settings.Save(request.Context(), configapp.SaveSettingsInput{
		Values: values, Actor: "admin:" + strconvFormatInt(principal.AdminUserID), RequestID: platformhttp.RequestID(request.Context()),
	}); err != nil {
		writeAppSettingsResourceServiceError(writer, err)
		return
	}
	projection, err := handler.settings.List(request.Context(), configapp.SettingsListInput{})
	if err != nil {
		writeAppSettingsResourceError(writer, http.StatusServiceUnavailable, "settings_unavailable")
		return
	}
	changed := publicAppSettingsResourceChanges(values, projection.MetadataMap)
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok":                          true,
		"changed":                     changed,
		"changed_count":               len(changed),
		"config":                      projection,
		"source_status":               "next_command",
		"fallback_used":               false,
		"real_external_call_executed": false,
	})
}

func appSettingsResourceHasRawSecret(values map[string][]string, metadata map[configport.Key]configapp.SettingMetadata) bool {
	for key, value := range values {
		item, known := metadata[configport.Key(key)]
		if known && item.Mode == "masked" && len(value) == 1 && strings.TrimSpace(value[0]) != "" {
			return true
		}
	}
	return false
}

// appSettingsResourceActionToken remains per-session, method, and exact
// route. The read resource exposes it only to an already-authorized session.
func appSettingsResourceActionToken(session authport.SessionRef) string {
	return adminOpsActionToken(session, http.MethodPut, appSettingsResourcePath)
}

func decodeAppSettingsResourcePayload(request *http.Request) (map[string][]string, string, string) {
	if request == nil || request.Body == nil {
		return nil, "", "payload_must_be_object"
	}
	decoder := json.NewDecoder(request.Body)
	decoder.UseNumber()
	var payload map[string]json.RawMessage
	if err := decoder.Decode(&payload); err != nil || payload == nil {
		return nil, "", "payload_must_be_object"
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, "", "payload_must_be_object"
	}
	var confirm bool
	if raw, present := payload["confirm"]; !present || json.Unmarshal(raw, &confirm) != nil || !confirm {
		return nil, "", "confirmation_required"
	}
	values, code := decodeAppSettingsResourceValues(payload["settings"])
	if code != "" {
		return nil, "", code
	}
	token := strings.TrimSpace(request.Header.Get("X-Admin-Action-Token"))
	if token == "" {
		_ = json.Unmarshal(payload["admin_action_token"], &token)
		token = strings.TrimSpace(token)
	}
	return values, token, ""
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("trailing JSON value")
	}
	return err
}

func decodeAppSettingsResourceValues(raw json.RawMessage) (map[string][]string, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string][]string{}, ""
	}
	var settings map[string]json.RawMessage
	if json.Unmarshal(raw, &settings) != nil || settings == nil {
		return nil, "settings_must_be_object"
	}
	values := make(map[string][]string, len(settings))
	for key, rawValue := range settings {
		var text string
		if json.Unmarshal(rawValue, &text) != nil {
			var number json.Number
			if json.Unmarshal(rawValue, &number) != nil || strings.TrimSpace(number.String()) == "" {
				return nil, "invalid_setting_value"
			}
			text = number.String()
		}
		values[key] = []string{text}
	}
	return values, ""
}

func publicAppSettingsResourceChanges(values map[string][]string, metadata map[configport.Key]configapp.SettingMetadata) []map[string]any {
	keys := make([]string, 0, len(values))
	for key, value := range values {
		item, known := metadata[configport.Key(key)]
		if known && item.Mode == "masked" && len(value) == 1 && strings.TrimSpace(value[0]) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	changed := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		item, known := metadata[configport.Key(key)]
		if known && item.Mode == "masked" {
			changed = append(changed, map[string]any{"key": key, "configured": true, "masked": true})
			continue
		}
		changed = append(changed, map[string]any{"key": key, "value": values[key][0]})
	}
	return changed
}

func writeAppSettingsResourceServiceError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, configport.ErrSecretSetting):
		writeAppSettingsResourceError(writer, http.StatusBadRequest, "secret_input_forbidden")
	case errors.Is(err, configport.ErrIdempotencyConflict):
		writeAppSettingsResourceError(writer, http.StatusConflict, "settings_idempotency_conflict")
	case errors.Is(err, configapp.ErrInvalidAppSettingsRequest), errors.Is(err, configport.ErrUnknownSetting), errors.Is(err, configport.ErrInvalidSetting):
		writeAppSettingsResourceError(writer, http.StatusBadRequest, "invalid_setting")
	default:
		writeAppSettingsResourceError(writer, http.StatusServiceUnavailable, "settings_unavailable")
	}
}

func writeAppSettingsResourceError(writer http.ResponseWriter, status int, code string) {
	writeJSON(writer, status, map[string]any{"ok": false, "error": code})
}
