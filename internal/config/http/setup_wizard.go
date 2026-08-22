// Package confighttp adapts the strictly local setup-wizard configuration flow.
// It never invokes providers, OAuth, payments, webhooks, releases, or workers.
package confighttp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	configapp "github.com/qianlan33333-png/AI-CRM-v2/internal/config/app"
	configport "github.com/qianlan33333-png/AI-CRM-v2/internal/config/port"
)

const (
	SetupWizardPath     = "/api/admin/setup-wizard"
	maximumRequestBytes = 64 << 10
)

var allowedSetupWizardFields = map[string]bool{
	"wecom.corp_id":          true,
	"wecom.agent_id":         true,
	"wecom.secret":           true,
	"wecom.callback_token":   true,
	"wecom.callback_aes_key": true,
	"ai.api_key":             true,
	"expected_digest":        true,
	"admin_action_token":     true,
}

var requiredSetupWizardFields = []string{
	"wecom.corp_id",
	"wecom.agent_id",
	"wecom.secret",
	"wecom.callback_token",
	"wecom.callback_aes_key",
	"ai.api_key",
	"expected_digest",
	"admin_action_token",
}

type Handler struct {
	application setupWizardApplication
}

type setupWizardApplication interface {
	Get(context.Context) (configapp.SetupWizardSnapshot, error)
	Save(context.Context, configapp.SetupWizardSaveInput) (configapp.SetupWizardSaveResult, error)
}

func NewHandler(application setupWizardApplication) (*Handler, error) {
	if application == nil {
		return nil, configapp.ErrInvalidSetupWizardRequest
	}
	return &Handler{application: application}, nil
}

// ServeHTTP owns exactly the two setup-wizard endpoints. The composition root
// must mount POST through the canonical RequireCSRF middleware. The leaf does
// not validate a second time: a server-side validator may consume a one-time
// token, so duplicate validation would be observably unsafe.
func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	setHeaders(writer)
	if handler == nil || handler.application == nil || request == nil || request.URL == nil {
		writeError(writer, http.StatusServiceUnavailable, "setup_wizard_unavailable")
		return
	}
	if request.URL.EscapedPath() != request.URL.Path || strings.Contains(request.URL.Path, `\`) {
		writeError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	switch {
	case request.URL.Path == SetupWizardPath:
		switch request.Method {
		case http.MethodGet:
			handler.get(writer, request)
		case http.MethodPost:
			handler.save(writer, request)
		default:
			writeMethodNotAllowed(writer, "GET, POST")
		}
	default:
		writeError(writer, http.StatusNotFound, "not_found")
	}
}

func (handler *Handler) get(writer http.ResponseWriter, request *http.Request) {
	if request.URL.RawQuery != "" || !emptyBody(writer, request) {
		writeError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	session, _, status, code := setupWizardAuthorization(request, authport.CapabilityConfigOverviewRead)
	if code != "" {
		writeError(writer, status, code)
		return
	}
	snapshot, err := handler.application.Get(request.Context())
	if err != nil {
		writeApplicationError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok":                  true,
		"expected_digest":     snapshot.ExpectedDigest,
		"editable":            snapshot.Editable,
		"editable_configured": snapshot.Configured,
		"masked":              snapshot.Masked,
		"admin_action_token":  setupWizardActionToken(session, http.MethodPost, SetupWizardPath),
		"external":            false,
		"local_only":          true,
		"runtime_applied":     false,
	})
}

func (handler *Handler) save(writer http.ResponseWriter, request *http.Request) {
	if request.URL.RawQuery != "" {
		writeError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	session, principal, status, code := setupWizardAuthorization(request, authport.CapabilityConfigSettingsManage)
	if code != "" {
		writeError(writer, status, code)
		return
	}
	input, token, decodeCode := decodeSetupWizardInput(writer, request)
	if decodeCode != "" {
		writeError(writer, http.StatusBadRequest, decodeCode)
		return
	}
	expectedToken := setupWizardActionToken(session, request.Method, request.URL.Path)
	if len(token) != len(expectedToken) || !hmac.Equal([]byte(token), []byte(expectedToken)) {
		writeError(writer, http.StatusBadRequest, "invalid_action_token")
		return
	}
	idempotencyKey, keyCode := setupWizardIdempotencyKey(request)
	if keyCode != "" {
		writeError(writer, http.StatusBadRequest, keyCode)
		return
	}
	input.Actor = "admin:" + strconv.FormatInt(principal.AdminUserID, 10)
	input.IdempotencyKey = idempotencyKey
	result, err := handler.application.Save(request.Context(), input)
	if err != nil {
		writeApplicationError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok":              true,
		"config":          result.Snapshot,
		"receipt":         result.Receipt,
		"external":        false,
		"local_only":      true,
		"runtime_applied": false,
	})
}

func setupWizardAuthorization(request *http.Request, capability authport.Capability) (authport.SessionRef, authport.Principal, int, string) {
	if request == nil {
		return "", authport.Principal{}, http.StatusUnauthorized, "authentication_required"
	}
	session, sessionOK := authport.SessionFromContext(request.Context())
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	if !sessionOK || !principalOK || principal.AdminUserID < 1 {
		return "", authport.Principal{}, http.StatusUnauthorized, "authentication_required"
	}
	if !authorizationOK || authorization.Capability != capability || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return "", authport.Principal{}, http.StatusForbidden, "permission_denied"
	}
	return session, principal, 0, ""
}

func setupWizardActionToken(session authport.SessionRef, method, path string) string {
	mac := hmac.New(sha256.New, []byte(session))
	_, _ = mac.Write([]byte("v1\n" + method + "\n" + path))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func setupWizardIdempotencyKey(request *http.Request) (string, string) {
	values := request.Header.Values("Idempotency-Key")
	if len(values) != 1 || values[0] == "" || values[0] != strings.TrimSpace(values[0]) || len(values[0]) > 200 {
		return "", "invalid_idempotency_key"
	}
	return values[0], ""
}

func decodeSetupWizardInput(writer http.ResponseWriter, request *http.Request) (configapp.SetupWizardSaveInput, string, string) {
	contentType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return configapp.SetupWizardSaveInput{}, "", "invalid_request"
	}
	return decodeSetupWizardJSON(writer, request)
}

func decodeSetupWizardJSON(writer http.ResponseWriter, request *http.Request) (configapp.SetupWizardSaveInput, string, string) {
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maximumRequestBytes))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return configapp.SetupWizardSaveInput{}, "", "invalid_request"
	}
	fields := make(map[string]json.RawMessage, len(requiredSetupWizardFields)+1)
	for decoder.More() {
		keyToken, tokenErr := decoder.Token()
		key, keyOK := keyToken.(string)
		if tokenErr != nil || !keyOK || !allowedSetupWizardFields[key] {
			return configapp.SetupWizardSaveInput{}, "", "invalid_request"
		}
		if _, duplicate := fields[key]; duplicate {
			return configapp.SetupWizardSaveInput{}, "", "invalid_request"
		}
		var value json.RawMessage
		if decodeErr := decoder.Decode(&value); decodeErr != nil {
			return configapp.SetupWizardSaveInput{}, "", "invalid_request"
		}
		fields[key] = value
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') || !jsonDecoderEOF(decoder) {
		return configapp.SetupWizardSaveInput{}, "", "invalid_request"
	}
	return setupWizardInputFromJSON(fields)
}

func setupWizardInputFromJSON(fields map[string]json.RawMessage) (configapp.SetupWizardSaveInput, string, string) {
	if !hasAllSetupWizardFields(fields) {
		return configapp.SetupWizardSaveInput{}, "", "invalid_request"
	}
	stringsByKey := make(map[string]string, len(requiredSetupWizardFields))
	for _, key := range requiredSetupWizardFields {
		if key == "wecom.agent_id" {
			continue
		}
		var value string
		if err := json.Unmarshal(fields[key], &value); err != nil {
			return configapp.SetupWizardSaveInput{}, "", "invalid_request"
		}
		stringsByKey[key] = value
	}
	var agent json.Number
	decoder := json.NewDecoder(bytes.NewReader(fields["wecom.agent_id"]))
	decoder.UseNumber()
	if decoder.Decode(&agent) != nil || !jsonDecoderEOF(decoder) {
		return configapp.SetupWizardSaveInput{}, "", "invalid_request"
	}
	agentID, err := strconv.ParseInt(agent.String(), 10, 64)
	if err != nil {
		return configapp.SetupWizardSaveInput{}, "", "invalid_request"
	}
	return configapp.SetupWizardSaveInput{
		WeComCorpID: stringsByKey["wecom.corp_id"], WeComAgentID: agentID,
		WeComSecret: stringsByKey["wecom.secret"], WeComCallbackToken: stringsByKey["wecom.callback_token"],
		WeComCallbackAESKey: stringsByKey["wecom.callback_aes_key"], AIAPIKey: stringsByKey["ai.api_key"],
		ExpectedDigest: stringsByKey["expected_digest"],
	}, stringsByKey["admin_action_token"], ""
}

func hasAllSetupWizardFields(fields map[string]json.RawMessage) bool {
	for _, key := range requiredSetupWizardFields {
		if _, exists := fields[key]; !exists {
			return false
		}
	}
	return true
}

func jsonDecoderEOF(decoder *json.Decoder) bool {
	var extra any
	return errors.Is(decoder.Decode(&extra), io.EOF)
}

func emptyBody(writer http.ResponseWriter, request *http.Request) bool {
	if request.Body == nil {
		return true
	}
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, maximumRequestBytes))
	return err == nil && len(body) == 0
}

func writeApplicationError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, configport.ErrSecretSetting):
		writeError(writer, http.StatusBadRequest, "secret_input_forbidden")
	case errors.Is(err, configapp.ErrInvalidSetupWizardRequest), errors.Is(err, configport.ErrInvalidSetting):
		writeError(writer, http.StatusBadRequest, "invalid_setting")
	case errors.Is(err, configapp.ErrSetupWizardConflict), errors.Is(err, configport.ErrIdempotencyConflict):
		writeError(writer, http.StatusConflict, "setup_wizard_conflict")
	default:
		writeError(writer, http.StatusServiceUnavailable, "setup_wizard_unavailable")
	}
}

func setHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}

func writeMethodNotAllowed(writer http.ResponseWriter, allow string) {
	writer.Header().Set("Allow", allow)
	writer.WriteHeader(http.StatusMethodNotAllowed)
}

func writeError(writer http.ResponseWriter, status int, code string) {
	writeJSON(writer, status, map[string]any{"ok": false, "error": code})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
