package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const hxcSenderWriteEvidence = "P4-HXC-SENDER-MANAGEMENT-2026-08-20"

type hxcSenderSaveRequest struct {
	ID           string `json:"id"`
	SenderUserID string `json:"sender_userid"`
	DisplayName  string `json:"display_name"`
	Priority     int    `json:"priority"`
	IsActive     bool   `json:"is_active"`
}

type hxcSenderReorderRequest struct {
	IDs []string `json:"ids"`
}

type hxcSenderMutationResponse struct {
	OK                       bool                      `json:"ok"`
	Operation                string                    `json:"operation"`
	Item                     *hxcSenderConfigResponse  `json:"item,omitempty"`
	Items                    []hxcSenderConfigResponse `json:"items"`
	LocalOnly                bool                      `json:"local_only"`
	ProviderCallExecuted     bool                      `json:"provider_call_executed"`
	RealExternalCallExecuted bool                      `json:"real_external_call_executed"`
	ReadbackConfirmed        bool                      `json:"readback_confirmed"`
}

func (handler *hxcSenderHandler) Save(writer http.ResponseWriter, request *http.Request) {
	actor, key, ok := hxcSenderWriteContext(request)
	if !ok {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	if handler == nil || handler.manager == nil || handler.reader == nil {
		writeHXCSenderUnavailable(writer)
		return
	}
	var input hxcSenderSaveRequest
	if !decodeClosedHXCSenderJSON(writer, request, []string{"id", "sender_userid", "display_name", "priority", "is_active"}, &input) {
		return
	}
	item, err := handler.manager.Save(request.Context(), hxcapp.ManageCommand{
		ID: input.ID, SenderUserID: input.SenderUserID, DisplayName: input.DisplayName,
		Priority: input.Priority, Active: input.IsActive, Actor: actor, IdempotencyKey: key,
	})
	if err != nil {
		writeHXCSenderManageError(writer, err)
		return
	}
	projection, err := handler.reader.Read(request.Context())
	if err != nil || !containsHXCSenderConfig(projection.SendConfigs, item.ID, item.SenderUserID) {
		writeHXCSenderUnavailable(writer)
		return
	}
	projected := projectHXCSenderConfig(item)
	writeHXCSenderMutation(writer, "saved", &projected, projection.SendConfigs)
}

func (handler *hxcSenderHandler) Reorder(writer http.ResponseWriter, request *http.Request) {
	actor, key, ok := hxcSenderWriteContext(request)
	if !ok {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	if handler == nil || handler.manager == nil || handler.reader == nil {
		writeHXCSenderUnavailable(writer)
		return
	}
	var input hxcSenderReorderRequest
	if !decodeClosedHXCSenderJSON(writer, request, []string{"ids"}, &input) {
		return
	}
	items, err := handler.manager.Reorder(request.Context(), actor, key, input.IDs)
	if err != nil {
		writeHXCSenderManageError(writer, err)
		return
	}
	projection, err := handler.reader.Read(request.Context())
	if err != nil || !sameHXCSenderOrder(items, projection.SendConfigs) {
		writeHXCSenderUnavailable(writer)
		return
	}
	writeHXCSenderMutation(writer, "reordered", nil, projection.SendConfigs)
}

func (handler *hxcSenderHandler) Archive(writer http.ResponseWriter, request *http.Request) {
	actor, key, ok := hxcSenderWriteContext(request)
	if !ok {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	if handler == nil || handler.manager == nil || handler.reader == nil {
		writeHXCSenderUnavailable(writer)
		return
	}
	senderUserID := chi.URLParam(request, "sender_userid")
	if senderUserID == "" || senderUserID != strings.TrimSpace(senderUserID) || len(senderUserID) > 200 || strings.ContainsAny(senderUserID, "/\\\x00\r\n") {
		writeHXCSenderManageError(writer, hxcapp.ErrInvalidCommand)
		return
	}
	if err := handler.manager.Archive(request.Context(), hxcapp.ManageCommand{SenderUserID: senderUserID, Actor: actor, IdempotencyKey: key}); err != nil {
		writeHXCSenderManageError(writer, err)
		return
	}
	projection, err := handler.reader.Read(request.Context())
	if err != nil || containsHXCSenderUser(projection.SendConfigs, senderUserID) {
		writeHXCSenderUnavailable(writer)
		return
	}
	writeHXCSenderMutation(writer, "archived", nil, projection.SendConfigs)
}

func decodeClosedHXCSenderJSON(writer http.ResponseWriter, request *http.Request, keys []string, target any) bool {
	if request == nil || request.Body == nil {
		writeHXCSenderManageError(writer, hxcapp.ErrInvalidCommand)
		return false
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 32<<10)
	decoder := json.NewDecoder(request.Body)
	decoder.UseNumber()
	var raw map[string]json.RawMessage
	if decoder.Decode(&raw) != nil || raw == nil || ensureJSONEOF(decoder) != nil || len(raw) != len(keys) {
		writeHXCSenderManageError(writer, hxcapp.ErrInvalidCommand)
		return false
	}
	for _, key := range keys {
		if _, present := raw[key]; !present {
			writeHXCSenderManageError(writer, hxcapp.ErrInvalidCommand)
			return false
		}
	}
	data, err := json.Marshal(raw)
	if err != nil || json.Unmarshal(data, target) != nil {
		writeHXCSenderManageError(writer, hxcapp.ErrInvalidCommand)
		return false
	}
	return true
}

func hxcSenderWriteContext(request *http.Request) (string, string, bool) {
	if request == nil {
		return "", "", false
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	keys := request.Header.Values("Idempotency-Key")
	if !principalOK || principal.AdminUserID < 1 || principal.Role != authport.RoleAdmin || !authorizationOK ||
		authorization.Capability != authport.CapabilityOperationsManage || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 ||
		len(keys) != 1 || len(keys[0]) < 16 || len(keys[0]) > 128 || keys[0] != strings.TrimSpace(keys[0]) {
		return "", "", false
	}
	return "admin:" + strconv.FormatInt(principal.AdminUserID, 10), keys[0], true
}

func writeHXCSenderManageError(writer http.ResponseWriter, err error) {
	status, code := http.StatusServiceUnavailable, "hxc_send_config_unavailable"
	if errors.Is(err, hxcapp.ErrInvalidCommand) {
		status, code = http.StatusBadRequest, "invalid_hxc_send_config"
	} else if errors.Is(err, hxcapp.ErrConfigConflict) {
		status, code = http.StatusConflict, "hxc_send_config_conflict"
	}
	if status == http.StatusServiceUnavailable {
		platformhttp.MarkCompatibilityError(writer, platformhttp.CodeDependencyUnavailable)
	}
	writeJSON(writer, status, map[string]any{"ok": false, "status_code": status, "error_code": code, "real_external_call_executed": false})
}

func writeHXCSenderMutation(writer http.ResponseWriter, operation string, item *hxcSenderConfigResponse, items []hxcport.SenderConfig) {
	projected := make([]hxcSenderConfigResponse, 0, len(items))
	for _, value := range items {
		projected = append(projected, projectHXCSenderConfig(value))
	}
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, http.StatusOK, hxcSenderMutationResponse{OK: true, Operation: operation, Item: item, Items: projected, LocalOnly: true, ProviderCallExecuted: false, RealExternalCallExecuted: false, ReadbackConfirmed: true})
}

func containsHXCSenderConfig(items []hxcport.SenderConfig, id, userID string) bool {
	for _, item := range items {
		if item.ID == id && item.SenderUserID == userID {
			return true
		}
	}
	return false
}

func containsHXCSenderUser(items []hxcport.SenderConfig, userID string) bool {
	for _, item := range items {
		if item.SenderUserID == userID {
			return true
		}
	}
	return false
}

func sameHXCSenderOrder(expected, actual []hxcport.SenderConfig) bool {
	if len(expected) != len(actual) {
		return false
	}
	for index := range expected {
		if expected[index].ID != actual[index].ID || expected[index].Priority != actual[index].Priority {
			return false
		}
	}
	return true
}
