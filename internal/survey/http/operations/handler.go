// Package operationshttp exposes the local-only Survey Operations contract.
package operationshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

const (
	OperationsPath                    = "/api/admin/questionnaires/{questionnaire_id}/operations"
	CompletionPath                    = "/api/admin/questionnaires/{questionnaire_id}/operations/completion"
	ExternalPushPath                  = "/api/admin/questionnaires/{questionnaire_id}/operations/external-push"
	ExternalPushTestPath              = "/api/admin/questionnaires/{questionnaire_id}/operations/external-push/test"
	OperationsPagePath                = "/admin/questionnaires/{questionnaire_id}/operations"
	GlobalExternalPushLogsPath        = "/admin/questionnaires/external-push-logs"
	QuestionnaireExternalPushLogsPath = "/admin/questionnaires/{questionnaire_id}/external-push-logs"

	maximumBodyBytes = 4 * 1024
)

var canonicalUnsignedDecimal = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

type Application interface {
	Get(context.Context, surveyport.ID) (surveyport.OperationsProjection, error)
	SaveCompletion(context.Context, surveyport.SaveCompletionOperationsCommand) (surveyport.OperationsProjection, error)
	SaveExternalPush(context.Context, surveyport.SaveExternalPushOperationsCommand) (surveyport.OperationsProjection, error)
	QueueExternalPushTest(context.Context, surveyport.QueueExternalPushTestCommand) (surveyport.ExternalPushTest, error)
	ListExternalPushLogs(context.Context, *surveyport.ID, int32, int32) (surveyport.ExternalPushLogPage, error)
}

var _ Application = (*surveyapp.OperationsService)(nil)

type Handler struct {
	Application Application
}

func New(application Application) *Handler { return &Handler{Application: application} }

func (h *Handler) GetOperations(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	if r == nil || r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if !readAuthorized(r) {
		writeAuthorizationError(w, r)
		return
	}
	id, err := parseQuestionnaireID(r, "/api/admin/questionnaires/", "/operations")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_questionnaire_id")
		return
	}
	h.writeOperations(w, r, id)
}

// GetOperationsPage is a data-only carrier for the legacy admin path. It does
// not render HTML or accept a provider URL, secret, or execution instruction.
func (h *Handler) GetOperationsPage(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	if r == nil || r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if !readAuthorized(r) {
		writeAuthorizationError(w, r)
		return
	}
	id, err := parseQuestionnaireID(r, "/admin/questionnaires/", "/operations")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_questionnaire_id")
		return
	}
	h.writeOperations(w, r, id)
}

func (h *Handler) SaveCompletion(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	if r == nil || r.Method != http.MethodPut {
		writeMethodNotAllowed(w, http.MethodPut)
		return
	}
	actor, ok := writeAuthorized(r)
	if !ok {
		writeAuthorizationError(w, r)
		return
	}
	id, err := parseQuestionnaireID(r, "/api/admin/questionnaires/", "/operations/completion")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_questionnaire_id")
		return
	}
	key, err := idempotencyKey(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_idempotency_key")
		return
	}
	completion, err := decodeCompletion(r)
	if err != nil || !validCompletion(completion) {
		writeError(w, http.StatusBadRequest, "invalid_operations_request")
		return
	}
	if h == nil || h.Application == nil {
		writeError(w, http.StatusServiceUnavailable, "survey_operations_unavailable")
		return
	}
	result, err := h.Application.SaveCompletion(r.Context(), surveyport.SaveCompletionOperationsCommand{
		QuestionnaireID: id, Actor: actor, IdempotencyKey: key, Completion: completion,
	})
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	if !validOperationsProjection(result, id) || result.Completion != completion {
		writeError(w, http.StatusServiceUnavailable, "survey_operations_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) SaveExternalPush(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	if r == nil || r.Method != http.MethodPut {
		writeMethodNotAllowed(w, http.MethodPut)
		return
	}
	actor, ok := writeAuthorized(r)
	if !ok {
		writeAuthorizationError(w, r)
		return
	}
	id, err := parseQuestionnaireID(r, "/api/admin/questionnaires/", "/operations/external-push")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_questionnaire_id")
		return
	}
	key, err := idempotencyKey(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_idempotency_key")
		return
	}
	externalPush, err := decodeExternalPush(r)
	if err != nil || !validExternalPush(externalPush) {
		writeError(w, http.StatusBadRequest, "invalid_operations_request")
		return
	}
	if h == nil || h.Application == nil {
		writeError(w, http.StatusServiceUnavailable, "survey_operations_unavailable")
		return
	}
	result, err := h.Application.SaveExternalPush(r.Context(), surveyport.SaveExternalPushOperationsCommand{
		QuestionnaireID: id, Actor: actor, IdempotencyKey: key, ExternalPush: externalPush,
	})
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	if !validOperationsProjection(result, id) || result.ExternalPush != externalPush {
		writeError(w, http.StatusServiceUnavailable, "survey_operations_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) QueueExternalPushTest(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	if r == nil || r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	actor, ok := writeAuthorized(r)
	if !ok {
		writeAuthorizationError(w, r)
		return
	}
	id, err := parseQuestionnaireID(r, "/api/admin/questionnaires/", "/operations/external-push/test")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_questionnaire_id")
		return
	}
	key, err := idempotencyKey(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_idempotency_key")
		return
	}
	if err := requireEmptyBody(r); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_operations_request")
		return
	}
	if h == nil || h.Application == nil {
		writeError(w, http.StatusServiceUnavailable, "survey_operations_unavailable")
		return
	}
	result, err := h.Application.QueueExternalPushTest(r.Context(), surveyport.QueueExternalPushTestCommand{
		QuestionnaireID: id, Actor: actor, IdempotencyKey: key,
	})
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	if !validExternalPushTest(result, id) {
		writeError(w, http.StatusServiceUnavailable, "survey_operations_unavailable")
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (h *Handler) ListGlobalExternalPushLogs(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	if r == nil || r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if !readAuthorized(r) {
		writeAuthorizationError(w, r)
		return
	}
	limit, offset, err := parsePageQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_page")
		return
	}
	if h == nil || h.Application == nil {
		writeError(w, http.StatusServiceUnavailable, "survey_operations_unavailable")
		return
	}
	page, err := h.Application.ListExternalPushLogs(r.Context(), nil, limit, offset)
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	if !validLogPage(page, nil, limit, offset) {
		writeError(w, http.StatusServiceUnavailable, "survey_operations_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) ListQuestionnaireExternalPushLogs(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	if r == nil || r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if !readAuthorized(r) {
		writeAuthorizationError(w, r)
		return
	}
	id, err := parseQuestionnaireID(r, "/admin/questionnaires/", "/external-push-logs")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_questionnaire_id")
		return
	}
	limit, offset, err := parsePageQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_page")
		return
	}
	if h == nil || h.Application == nil {
		writeError(w, http.StatusServiceUnavailable, "survey_operations_unavailable")
		return
	}
	page, err := h.Application.ListExternalPushLogs(r.Context(), &id, limit, offset)
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	if !validLogPage(page, &id, limit, offset) {
		writeError(w, http.StatusServiceUnavailable, "survey_operations_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) writeOperations(w http.ResponseWriter, r *http.Request, id surveyport.ID) {
	if r.URL.RawQuery != "" {
		writeError(w, http.StatusBadRequest, "invalid_operations_request")
		return
	}
	if h == nil || h.Application == nil {
		writeError(w, http.StatusServiceUnavailable, "survey_operations_unavailable")
		return
	}
	result, err := h.Application.Get(r.Context(), id)
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	if !validOperationsProjection(result, id) {
		writeError(w, http.StatusServiceUnavailable, "survey_operations_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func parseQuestionnaireID(r *http.Request, prefix, suffix string) (surveyport.ID, error) {
	if r == nil || r.URL == nil {
		return 0, surveyapp.ErrInvalidOperations
	}
	escaped := r.URL.EscapedPath()
	if !strings.HasPrefix(escaped, prefix) || !strings.HasSuffix(escaped, suffix) {
		return 0, surveyapp.ErrInvalidOperations
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(escaped, prefix), suffix)
	if raw == "" || !canonicalUnsignedDecimal.MatchString(raw) || raw == "0" {
		return 0, surveyapp.ErrInvalidOperations
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 || strconv.FormatInt(value, 10) != raw || escaped != prefix+raw+suffix {
		return 0, surveyapp.ErrInvalidOperations
	}
	return surveyport.ID(value), nil
}

func parsePageQuery(raw string) (int32, int32, error) {
	limit, offset := surveyapp.ExternalPushLogDefaultLimit, int32(0)
	if raw == "" {
		return limit, offset, nil
	}
	seen := map[string]struct{}{}
	for _, pair := range strings.Split(raw, "&") {
		if pair == "" || strings.ContainsAny(pair, "%+;") {
			return 0, 0, surveyapp.ErrInvalidOperations
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 || parts[1] == "" || parts[0] != "limit" && parts[0] != "offset" {
			return 0, 0, surveyapp.ErrInvalidOperations
		}
		if _, duplicate := seen[parts[0]]; duplicate {
			return 0, 0, surveyapp.ErrInvalidOperations
		}
		seen[parts[0]] = struct{}{}
		parsed, err := parseCanonicalInt32(parts[1])
		if err != nil {
			return 0, 0, surveyapp.ErrInvalidOperations
		}
		if parts[0] == "limit" {
			if parsed < 1 || parsed > surveyapp.ExternalPushLogMaximumLimit {
				return 0, 0, surveyapp.ErrInvalidOperations
			}
			limit = parsed
		} else {
			if parsed < 0 || parsed > surveyapp.ExternalPushLogMaximumOffset {
				return 0, 0, surveyapp.ErrInvalidOperations
			}
			offset = parsed
		}
	}
	return limit, offset, nil
}

func parseCanonicalInt32(raw string) (int32, error) {
	if !canonicalUnsignedDecimal.MatchString(raw) {
		return 0, surveyapp.ErrInvalidOperations
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || strconv.FormatInt(value, 10) != raw {
		return 0, surveyapp.ErrInvalidOperations
	}
	return int32(value), nil
}

func idempotencyKey(r *http.Request) (string, error) {
	if r == nil {
		return "", surveyapp.ErrInvalidOperations
	}
	values := r.Header.Values("Idempotency-Key")
	if len(values) != 1 || values[0] == "" || strings.TrimSpace(values[0]) != values[0] || !utf8.ValidString(values[0]) || utf8.RuneCountInString(values[0]) < 16 || utf8.RuneCountInString(values[0]) > 128 {
		return "", surveyapp.ErrInvalidOperations
	}
	return values[0], nil
}

func decodeCompletion(r *http.Request) (surveyport.CompletionOperations, error) {
	values, err := decodeObject(r)
	if err != nil {
		return surveyport.CompletionOperations{}, err
	}
	value := surveyport.CompletionOperations{}
	for key, raw := range values {
		switch key {
		case "navigation_target_id":
			if err := decodeSingle(raw, &value.NavigationTargetID); err != nil {
				return surveyport.CompletionOperations{}, err
			}
		case "channel_id":
			if err := decodeSingle(raw, &value.ChannelID); err != nil {
				return surveyport.CompletionOperations{}, err
			}
		default:
			return surveyport.CompletionOperations{}, surveyapp.ErrInvalidOperations
		}
	}
	return value, nil
}

func decodeExternalPush(r *http.Request) (surveyport.ExternalPushOperations, error) {
	values, err := decodeObject(r)
	if err != nil {
		return surveyport.ExternalPushOperations{}, err
	}
	rawEnabled, present := values["enabled"]
	if !present {
		return surveyport.ExternalPushOperations{}, surveyapp.ErrInvalidOperations
	}
	value := surveyport.ExternalPushOperations{}
	if err := decodeSingle(rawEnabled, &value.Enabled); err != nil {
		return surveyport.ExternalPushOperations{}, err
	}
	for key, raw := range values {
		switch key {
		case "enabled":
		case "configuration_reference":
			if err := decodeSingle(raw, &value.ConfigurationReference); err != nil {
				return surveyport.ExternalPushOperations{}, err
			}
		default:
			return surveyport.ExternalPushOperations{}, surveyapp.ErrInvalidOperations
		}
	}
	return value, nil
}

func decodeObject(r *http.Request) (map[string]json.RawMessage, error) {
	if r == nil || r.Body == nil {
		return nil, surveyapp.ErrInvalidOperations
	}
	mediaType, parameters, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" || len(parameters) > 1 {
		return nil, surveyapp.ErrInvalidOperations
	}
	for key, value := range parameters {
		if key != "charset" || !strings.EqualFold(value, "utf-8") {
			return nil, surveyapp.ErrInvalidOperations
		}
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maximumBodyBytes+1))
	if err != nil || len(body) == 0 || len(body) > maximumBodyBytes || !utf8.Valid(body) {
		return nil, surveyapp.ErrInvalidOperations
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, surveyapp.ErrInvalidOperations
	}
	result := map[string]json.RawMessage{}
	for decoder.More() {
		token, tokenErr := decoder.Token()
		key, ok := token.(string)
		if tokenErr != nil || !ok || key == "" {
			return nil, surveyapp.ErrInvalidOperations
		}
		if _, duplicate := result[key]; duplicate {
			return nil, surveyapp.ErrInvalidOperations
		}
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil {
			return nil, surveyapp.ErrInvalidOperations
		}
		result[key] = append(json.RawMessage{}, raw...)
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return nil, surveyapp.ErrInvalidOperations
	}
	if token, trailingErr := decoder.Token(); token != nil || !errors.Is(trailingErr, io.EOF) {
		return nil, surveyapp.ErrInvalidOperations
	}
	return result, nil
}

func decodeSingle(raw json.RawMessage, destination any) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return surveyapp.ErrInvalidOperations
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return surveyapp.ErrInvalidOperations
	}
	var trailing any
	if !errors.Is(decoder.Decode(&trailing), io.EOF) {
		return surveyapp.ErrInvalidOperations
	}
	return nil
}

func requireEmptyBody(r *http.Request) error {
	if r == nil || r.Body == nil {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 2))
	if err != nil || len(body) != 0 {
		return surveyapp.ErrInvalidOperations
	}
	return nil
}

func readAuthorized(r *http.Request) bool {
	if r == nil {
		return false
	}
	principal, principalOK := authport.PrincipalFromContext(r.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(r.Context())
	return principalOK && principal.AdminUserID > 0 && (principal.Role == authport.RoleAdmin || principal.Role == authport.RoleOps) &&
		authorizationOK && authorization.Capability == authport.CapabilityQuestionnairesRead && authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}

func writeAuthorized(r *http.Request) (int64, bool) {
	if r == nil {
		return 0, false
	}
	principal, principalOK := authport.PrincipalFromContext(r.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(r.Context())
	if !principalOK || principal.AdminUserID < 1 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) ||
		!authorizationOK || authorization.Capability != authport.CapabilityQuestionnairesWrite || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return 0, false
	}
	return principal.AdminUserID, true
}

func validOperationsProjection(value surveyport.OperationsProjection, questionnaireID surveyport.ID) bool {
	return value.QuestionnaireID == questionnaireID && questionnaireID > 0 && value.LocalOnly &&
		validCompletion(value.Completion) && validExternalPush(value.ExternalPush)
}

func validCompletion(value surveyport.CompletionOperations) bool {
	return value.ChannelID >= 0 && validOptionalOpaqueReference(value.NavigationTargetID) &&
		(value.NavigationTargetID != "" || value.ChannelID == 0)
}

func validExternalPush(value surveyport.ExternalPushOperations) bool {
	return value.Enabled && validOpaqueReference(value.ConfigurationReference) || !value.Enabled && value.ConfigurationReference == ""
}

func validOptionalOpaqueReference(value string) bool {
	return value == "" || validOpaqueReference(value)
}

func validOpaqueReference(value string) bool {
	if !utf8.ValidString(value) || value == "" || utf8.RuneCountInString(value) > 128 || strings.TrimSpace(value) != value || strings.Contains(value, "://") {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("-_.:", character) {
			continue
		}
		return false
	}
	return true
}

func validExternalPushTest(value surveyport.ExternalPushTest, questionnaireID surveyport.ID) bool {
	return value.TestRunID > 0 && value.QuestionnaireID == questionnaireID && value.Status == surveyapp.ExternalPushTestQueued &&
		value.AttemptCount == 0 && !value.SideEffectExecuted && !value.ProviderResultReceived && !value.UnknownAfterDispatch &&
		!value.AutoRetryAllowed && !value.CreatedAt.IsZero() && !value.UpdatedAt.IsZero() && !value.UpdatedAt.Before(value.CreatedAt)
}

func validLogPage(value surveyport.ExternalPushLogPage, questionnaireID *surveyport.ID, limit, offset int32) bool {
	if !value.LocalOnly || value.Items == nil || value.Total < 0 || value.Limit != limit || value.Offset != offset || int64(offset) > value.Total || len(value.Items) > int(limit) ||
		value.HasMore != (int64(offset)+int64(len(value.Items)) < value.Total) {
		return false
	}
	for index, item := range value.Items {
		if questionnaireID != nil && item.QuestionnaireID != *questionnaireID || !validExternalPushTest(item, item.QuestionnaireID) {
			return false
		}
		if index > 0 {
			previous := value.Items[index-1]
			if previous.CreatedAt.Before(item.CreatedAt) || previous.CreatedAt.Equal(item.CreatedAt) && previous.TestRunID <= item.TestRunID {
				return false
			}
		}
	}
	return true
}

func writeApplicationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, surveyapp.ErrInvalidOperations):
		writeError(w, http.StatusBadRequest, "invalid_operations_request")
	case errors.Is(err, surveyapp.ErrNotFound):
		writeError(w, http.StatusNotFound, "questionnaire_not_found")
	case errors.Is(err, surveyapp.ErrConflict):
		writeError(w, http.StatusConflict, "operations_conflict")
	case errors.Is(err, surveyapp.ErrExternalPushNotConfigured):
		writeError(w, http.StatusConflict, "external_push_not_configured")
	default:
		writeError(w, http.StatusServiceUnavailable, "survey_operations_unavailable")
	}
}

func writeAuthorizationError(w http.ResponseWriter, r *http.Request) {
	if r == nil {
		writeError(w, http.StatusUnauthorized, "authentication_required")
		return
	}
	if _, ok := authport.PrincipalFromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required")
		return
	}
	writeError(w, http.StatusForbidden, "permission_denied")
}

func writeMethodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
}

func setHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{
		"ok":                       false,
		"error":                    map[string]string{"code": code},
		"local_only":               true,
		"side_effect_executed":     false,
		"provider_result_received": false,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
