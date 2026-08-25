package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const channelAcquisitionMaximumBodyBytes = 64 << 10

type channelAcquisitionMutationApplication interface {
	UpdateChannel(context.Context, contactapp.UpdateChannelCommand) (contactapp.Channel, error)
}

type channelAcquisitionPreviewApplication interface {
	Preview(context.Context, int64) (contactapp.ChannelAcquisitionPreview, error)
}

type channelAcquisitionCSRFValidator interface {
	ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error
}

// ChannelAcquisitionHandler is a leaf transport adapter. Root composition
// supplies routing and the real browser-session authenticator; this handler
// independently preserves the frozen authorization and CSRF boundary.
type ChannelAcquisitionHandler struct {
	mutation channelAcquisitionMutationApplication
	preview  channelAcquisitionPreviewApplication
	csrf     channelAcquisitionCSRFValidator
}

func NewChannelAcquisitionHandler(mutation channelAcquisitionMutationApplication, preview channelAcquisitionPreviewApplication, csrf channelAcquisitionCSRFValidator) (*ChannelAcquisitionHandler, error) {
	if channelAcquisitionNil(mutation) || channelAcquisitionNil(preview) || channelAcquisitionNil(csrf) {
		return nil, errors.New("channel acquisition dependencies are required")
	}
	return &ChannelAcquisitionHandler{mutation: mutation, preview: preview, csrf: csrf}, nil
}

// Preview returns a local readiness projection. It never asks a provider to
// create, fetch, or refresh a QR code.
func (handler *ChannelAcquisitionHandler) Preview(writer http.ResponseWriter, request *http.Request, rawChannelID string) {
	channelAcquisitionSecurityHeaders(writer)
	if !channelAcquisitionRequireMethod(writer, request, http.MethodGet) {
		return
	}
	if _, err := handler.authorize(request, authport.CapabilityChannelsRead, false); err != nil {
		channelAcquisitionWriteError(writer, request, err)
		return
	}
	channelID, err := channelAcquisitionID(rawChannelID)
	if err != nil {
		channelAcquisitionWriteValidation(writer, request, "channel_id", err)
		return
	}
	response, err := handler.preview.Preview(request.Context(), channelID)
	if err != nil {
		channelAcquisitionWriteError(writer, request, err)
		return
	}
	if !channelAcquisitionValidPreview(response, channelID) {
		channelAcquisitionWriteError(writer, request, contactapp.ErrChannelUnavailable)
		return
	}
	channelAcquisitionWriteJSON(writer, http.StatusOK, channelAcquisitionPreviewResponse{
		ChannelAcquisitionPreview: response,
		LocalOnly:                 true,
		ProviderExecutionEligible: false,
		RealExternalCallExecuted:  false,
	})
}

// UpdateAssignees validates only the CH01 assignment patch. The domain service
// retains the channel receipt, event append, and transaction ownership.
func (handler *ChannelAcquisitionHandler) UpdateAssignees(writer http.ResponseWriter, request *http.Request, rawChannelID string) {
	channelAcquisitionSecurityHeaders(writer)
	if !channelAcquisitionRequireMethod(writer, request, http.MethodPut) {
		return
	}
	actor, err := handler.authorize(request, authport.CapabilityChannelsWrite, true)
	if err != nil {
		channelAcquisitionWriteError(writer, request, err)
		return
	}
	channelID, err := channelAcquisitionID(rawChannelID)
	if err != nil {
		channelAcquisitionWriteValidation(writer, request, "channel_id", err)
		return
	}
	key, err := channelAcquisitionIdempotencyKey(request)
	if err != nil {
		channelAcquisitionWriteValidation(writer, request, "Idempotency-Key", err)
		return
	}
	patch, err := channelAcquisitionAssignmentPatch(writer, request)
	if err != nil {
		channelAcquisitionWriteValidation(writer, request, "body", err)
		return
	}
	updated, err := handler.mutation.UpdateChannel(request.Context(), contactapp.UpdateChannelCommand{Actor: actor, ChannelID: channelID, IdempotencyKey: key, Patch: patch})
	if err != nil {
		channelAcquisitionWriteError(writer, request, err)
		return
	}
	if updated.ID != channelID || !channelAcquisitionValidAssignees(updated.Assignees) {
		channelAcquisitionWriteError(writer, request, contactapp.ErrChannelUnavailable)
		return
	}
	channelAcquisitionWriteJSON(writer, http.StatusOK, channelAcquisitionAssignmentResponse{
		ChannelID: channelID, Assignees: updated.Assignees,
		LocalOnly: true, ProviderExecutionEligible: false, RealExternalCallExecuted: false,
	})
}

func (handler *ChannelAcquisitionHandler) authorize(request *http.Request, capability authport.Capability, csrfRequired bool) (int64, error) {
	if handler == nil || channelAcquisitionNil(handler.mutation) || channelAcquisitionNil(handler.preview) || channelAcquisitionNil(handler.csrf) || request == nil {
		return 0, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, contactapp.ErrChannelUnavailable)
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	if !principalOK || principal.AdminUserID < 1 {
		return 0, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	if !authorizationOK || authorization.Capability != capability || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 ||
		(principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) {
		return 0, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	if !csrfRequired {
		return principal.AdminUserID, nil
	}
	session, sessionOK := authport.SessionFromContext(request.Context())
	values := request.Header.Values("X-CSRF-Token")
	if !sessionOK {
		return 0, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}
	if len(values) != 1 || !channelAcquisitionValidCSRF(values[0]) {
		return 0, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrCSRFInvalid)
	}
	if err := handler.csrf.ValidateCSRF(request.Context(), session, authport.CSRFToken(values[0])); err != nil {
		code := platformhttp.CodeUnauthorized
		if errors.Is(err, authport.ErrUnauthenticated) {
			code = platformhttp.CodeUnauthenticated
		} else if errors.Is(err, authport.ErrAuthenticationUnavailable) {
			code = platformhttp.CodeDependencyUnavailable
		}
		return 0, platformhttp.NewError(code, err)
	}
	return principal.AdminUserID, nil
}

type channelAcquisitionAssignment struct {
	AssignmentMode     string                     `json:"assignment_mode"`
	AssignmentStrategy string                     `json:"assignment_strategy"`
	OverflowPolicy     string                     `json:"overflow_policy"`
	Assignees          []channelAcquisitionMember `json:"assignees"`
}

type channelAcquisitionMember struct {
	StaffID      string `json:"staff_id"`
	Status       string `json:"status"`
	Priority     int32  `json:"priority"`
	RatioPercent *int32 `json:"ratio_percent,omitempty"`
	MaxScans24h  *int32 `json:"max_scans_24h,omitempty"`
}

type channelAcquisitionAssignmentResponse struct {
	ChannelID                 int64                        `json:"channel_id"`
	Assignees                 []contactapp.ChannelAssignee `json:"assignees"`
	LocalOnly                 bool                         `json:"local_only"`
	ProviderExecutionEligible bool                         `json:"provider_execution_eligible"`
	RealExternalCallExecuted  bool                         `json:"real_external_call_executed"`
}

type channelAcquisitionPreviewResponse struct {
	contactapp.ChannelAcquisitionPreview
	LocalOnly                 bool `json:"local_only"`
	ProviderExecutionEligible bool `json:"provider_execution_eligible"`
	RealExternalCallExecuted  bool `json:"real_external_call_executed"`
}

func channelAcquisitionAssignmentPatch(writer http.ResponseWriter, request *http.Request) (json.RawMessage, error) {
	object, err := channelAcquisitionDecodeObject(writer, request)
	if err != nil {
		return nil, err
	}
	for key := range object {
		if key != "assignment_mode" && key != "assignment_strategy" && key != "overflow_policy" && key != "assignees" {
			return nil, contactapp.ErrInvalidChannel
		}
	}
	assigneesRaw, present := object["assignees"]
	if !present {
		return nil, contactapp.ErrInvalidChannel
	}
	command := channelAcquisitionAssignment{AssignmentMode: "multi_staff", AssignmentStrategy: "ratio", OverflowPolicy: "least_loaded"}
	if raw, ok := object["assignment_mode"]; ok && json.Unmarshal(raw, &command.AssignmentMode) != nil {
		return nil, contactapp.ErrInvalidChannel
	}
	if raw, ok := object["assignment_strategy"]; ok && json.Unmarshal(raw, &command.AssignmentStrategy) != nil {
		return nil, contactapp.ErrInvalidChannel
	}
	if raw, ok := object["overflow_policy"]; ok && json.Unmarshal(raw, &command.OverflowPolicy) != nil {
		return nil, contactapp.ErrInvalidChannel
	}
	if command.AssignmentMode != "single_owner" && command.AssignmentMode != "multi_staff" ||
		command.AssignmentStrategy != "ratio" && command.AssignmentStrategy != "cap_switch" ||
		!channelAcquisitionValidText(command.OverflowPolicy, 200) {
		return nil, contactapp.ErrInvalidChannel
	}
	assignees, err := channelAcquisitionDecodeMembers(assigneesRaw)
	if err != nil {
		return nil, contactapp.ErrInvalidChannel
	}
	command.Assignees = assignees
	channelAcquisitionNormalizeAssignment(&command)
	if !channelAcquisitionValidAssignment(command) {
		return nil, contactapp.ErrInvalidChannel
	}
	patch, err := json.Marshal(command)
	if err != nil {
		return nil, contactapp.ErrInvalidChannel
	}
	return patch, nil
}

func channelAcquisitionValidAssignment(command channelAcquisitionAssignment) bool {
	active, totalRatio := 0, int64(0)
	seen := make(map[string]struct{}, len(command.Assignees))
	for _, assignee := range command.Assignees {
		if !channelAcquisitionValidText(assignee.StaffID, 200) || (assignee.Status != "" && assignee.Status != "active" && assignee.Status != "inactive" && assignee.Status != "archived") || assignee.Priority < 0 {
			return false
		}
		if _, duplicate := seen[assignee.StaffID]; duplicate {
			return false
		}
		seen[assignee.StaffID] = struct{}{}
		status := assignee.Status
		if status == "" {
			status = "active"
		}
		if status != "active" {
			continue
		}
		active++
		if command.AssignmentStrategy == "ratio" {
			if assignee.RatioPercent == nil || *assignee.RatioPercent < 1 || assignee.MaxScans24h != nil {
				return false
			}
			totalRatio += int64(*assignee.RatioPercent)
		} else if assignee.MaxScans24h == nil || *assignee.MaxScans24h < 1 || assignee.RatioPercent != nil {
			return false
		}
	}
	return active >= 1 && active <= 5 && (command.AssignmentMode != "single_owner" || active == 1) &&
		(command.AssignmentStrategy != "ratio" || totalRatio == 100)
}

func channelAcquisitionNormalizeAssignment(command *channelAcquisitionAssignment) {
	if command == nil {
		return
	}
	for index := range command.Assignees {
		if command.Assignees[index].Status == "" {
			command.Assignees[index].Status = "active"
		}
		if command.Assignees[index].Priority == 0 {
			command.Assignees[index].Priority = int32(index + 1)
		}
	}
}

func channelAcquisitionDecodeObject(writer http.ResponseWriter, request *http.Request) (map[string]json.RawMessage, error) {
	if request == nil || request.Body == nil {
		return nil, io.EOF
	}
	return channelAcquisitionDecodeObjectReader(http.MaxBytesReader(writer, request.Body, channelAcquisitionMaximumBodyBytes))
}

func channelAcquisitionDecodeObjectReader(reader io.Reader) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(reader)
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return nil, contactapp.ErrInvalidChannel
	}
	result := make(map[string]json.RawMessage)
	for decoder.More() {
		token, tokenErr := decoder.Token()
		key, ok := token.(string)
		if tokenErr != nil || !ok || !utf8.ValidString(key) || key == "" {
			return nil, contactapp.ErrInvalidChannel
		}
		if _, duplicate := result[key]; duplicate {
			return nil, contactapp.ErrInvalidChannel
		}
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil {
			return nil, contactapp.ErrInvalidChannel
		}
		result[key] = raw
	}
	if token, tokenErr := decoder.Token(); tokenErr != nil || token != json.Delim('}') {
		return nil, contactapp.ErrInvalidChannel
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, contactapp.ErrInvalidChannel
	}
	return result, nil
}

func channelAcquisitionDecodeMembers(raw json.RawMessage) ([]channelAcquisitionMember, error) {
	var rawMembers []json.RawMessage
	if json.Unmarshal(raw, &rawMembers) != nil {
		return nil, contactapp.ErrInvalidChannel
	}
	result := make([]channelAcquisitionMember, 0, len(rawMembers))
	for _, rawMember := range rawMembers {
		fields, err := channelAcquisitionDecodeObjectReader(bytes.NewReader(rawMember))
		if err != nil {
			return nil, contactapp.ErrInvalidChannel
		}
		for key := range fields {
			if key != "staff_id" && key != "status" && key != "priority" && key != "ratio_percent" && key != "max_scans_24h" {
				return nil, contactapp.ErrInvalidChannel
			}
		}
		staffID, present := fields["staff_id"]
		if !present {
			return nil, contactapp.ErrInvalidChannel
		}
		member := channelAcquisitionMember{}
		if json.Unmarshal(staffID, &member.StaffID) != nil {
			return nil, contactapp.ErrInvalidChannel
		}
		if value, ok := fields["status"]; ok && json.Unmarshal(value, &member.Status) != nil {
			return nil, contactapp.ErrInvalidChannel
		}
		if value, ok := fields["priority"]; ok && json.Unmarshal(value, &member.Priority) != nil {
			return nil, contactapp.ErrInvalidChannel
		}
		if value, ok := fields["ratio_percent"]; ok {
			var parsed int32
			if json.Unmarshal(value, &parsed) != nil {
				return nil, contactapp.ErrInvalidChannel
			}
			member.RatioPercent = &parsed
		}
		if value, ok := fields["max_scans_24h"]; ok {
			var parsed int32
			if json.Unmarshal(value, &parsed) != nil {
				return nil, contactapp.ErrInvalidChannel
			}
			member.MaxScans24h = &parsed
		}
		result = append(result, member)
	}
	return result, nil
}

func channelAcquisitionID(raw string) (int64, error) {
	if raw == "" || len(raw) > 19 || raw[0] == '0' {
		return 0, contactapp.ErrInvalidChannel
	}
	for _, character := range raw {
		if character < '0' || character > '9' {
			return 0, contactapp.ErrInvalidChannel
		}
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 {
		return 0, contactapp.ErrInvalidChannel
	}
	return value, nil
}

func channelAcquisitionIdempotencyKey(request *http.Request) (string, error) {
	if request == nil {
		return "", contactapp.ErrInvalidChannel
	}
	values := request.Header.Values("Idempotency-Key")
	if len(values) != 1 || !channelAcquisitionValidText(values[0], 128) || len(values[0]) < 16 {
		return "", contactapp.ErrInvalidChannel
	}
	return values[0], nil
}

func channelAcquisitionValidPreview(preview contactapp.ChannelAcquisitionPreview, channelID int64) bool {
	if preview.ChannelID != channelID || !channelAcquisitionValidText(preview.ChannelCode, 200) || !channelAcquisitionValidText(preview.ChannelName, 200) ||
		(preview.Lifecycle.State != "ready" && preview.Lifecycle.State != "draft" && preview.Lifecycle.State != "paused" && preview.Lifecycle.State != "archived") ||
		preview.Lifecycle.EntrantReady != (preview.Lifecycle.State == "ready") ||
		(preview.QRCode.Status != "not_generated" && preview.QRCode.Status != "legacy_untracked") ||
		!channelAcquisitionValidAssignees(preview.Assignees) {
		return false
	}
	if preview.Lifecycle.State == "ready" && len(preview.Lifecycle.ReadinessBlockers) != 0 || preview.Lifecycle.State != "ready" && len(preview.Lifecycle.ReadinessBlockers) == 0 {
		return false
	}
	for _, blocker := range preview.Lifecycle.ReadinessBlockers {
		if !channelAcquisitionValidText(blocker, 100) {
			return false
		}
	}
	return channelAcquisitionValidTextOrEmpty(preview.QRCode.SceneValue, 10000) && channelAcquisitionValidTextOrEmpty(preview.QRCode.URL, 10000) &&
		channelAcquisitionValidTextOrEmpty(preview.Share.URL, 10000) && channelAcquisitionValidTextOrEmpty(preview.Share.CopyText, 10000)
}

func channelAcquisitionValidAssignees(assignees []contactapp.ChannelAssignee) bool {
	if len(assignees) < 1 || len(assignees) > 5 {
		return false
	}
	seen := make(map[string]struct{}, len(assignees))
	for _, assignee := range assignees {
		if !channelAcquisitionValidText(assignee.WeComUserID, 200) || !channelAcquisitionValidText(assignee.DisplayName, 200) || assignee.Status != "active" || assignee.Priority < 1 {
			return false
		}
		if _, duplicate := seen[assignee.WeComUserID]; duplicate {
			return false
		}
		seen[assignee.WeComUserID] = struct{}{}
	}
	return true
}

func channelAcquisitionValidCSRF(value string) bool { return channelAcquisitionValidText(value, 4096) }
func channelAcquisitionValidTextOrEmpty(value string, maximum int) bool {
	return value == "" || channelAcquisitionValidText(value, maximum)
}
func channelAcquisitionValidText(value string, maximum int) bool {
	return value != "" && value == strings.TrimSpace(value) && utf8.ValidString(value) && len(value) <= maximum && strings.IndexFunc(value, unicode.IsControl) < 0
}

func channelAcquisitionRequireMethod(writer http.ResponseWriter, request *http.Request, method string) bool {
	if request != nil && request.Method == method {
		return true
	}
	if writer != nil {
		writer.Header().Set("Allow", method)
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
	return false
}

func channelAcquisitionWriteValidation(writer http.ResponseWriter, request *http.Request, field string, err error) {
	channelAcquisitionWriteError(writer, request, platformhttp.NewError(platformhttp.CodeValidationFailed, err, platformhttp.FieldError{Field: field, Reason: "invalid"}))
}

func channelAcquisitionWriteError(writer http.ResponseWriter, request *http.Request, err error) {
	if writer == nil {
		return
	}
	channelAcquisitionSecurityHeaders(writer)
	if request == nil {
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if platformhttp.ErrorCodeOf(err) != platformhttp.CodeInternal {
		platformhttp.WriteError(writer, request, err)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, contactapp.ErrInvalidChannel):
		code = platformhttp.CodeValidationFailed
	case errors.Is(err, contactapp.ErrChannelNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, contactapp.ErrChannelConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func channelAcquisitionWriteJSON(writer http.ResponseWriter, status int, value any) {
	if writer == nil {
		return
	}
	channelAcquisitionSecurityHeaders(writer)
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func channelAcquisitionSecurityHeaders(writer http.ResponseWriter) {
	if writer != nil {
		writer.Header().Set("Cache-Control", "private, no-store")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
	}
}

func channelAcquisitionNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
