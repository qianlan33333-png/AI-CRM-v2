package legacyaudience

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
)

const InboundWebhookPathPrefix = "/api/ai/audience/packages/"

type InboundWebhookAuthenticator interface {
	Authenticate(context.Context, *http.Request, []byte) (InboundWebhookIdentity, error)
}

type InboundWebhookHandler struct {
	application   InboundWebhookApplication
	authenticator InboundWebhookAuthenticator
}

func NewInboundWebhookHandler(application InboundWebhookApplication, authenticator InboundWebhookAuthenticator) (*InboundWebhookHandler, error) {
	if nilInterface(application) || nilInterface(authenticator) {
		return nil, ErrUnavailable
	}
	return &InboundWebhookHandler{application: application, authenticator: authenticator}, nil
}

func (handler *InboundWebhookHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	setInboundWebhookHeaders(writer)
	if handler == nil || nilInterface(handler.application) || nilInterface(handler.authenticator) || request == nil || request.URL == nil {
		writeFailure(writer, request, ErrUnavailable)
		return
	}
	if request.Method != http.MethodPost {
		writeMethodNotAllowed(writer, request, http.MethodPost)
		return
	}
	if !requireNoQuery(writer, request) {
		return
	}
	packageID, problem := inboundWebhookPackageID(request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	body, problem := readInboundWebhookBody(writer, request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	identity, err := handler.authenticator.Authenticate(request.Context(), request, body)
	if err != nil {
		writeFailure(writer, request, err)
		return
	}
	input, problem := decodeInboundWebhookBody(body)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	input.PackageID, input.ClientID, input.TransportEventID = packageID, identity.ClientID, identity.TransportEventID
	input.PayloadDigest = sha256.Sum256(body)
	result, err := handler.application.Accept(request.Context(), input)
	if err != nil {
		writeFailure(writer, request, err)
		return
	}
	response := struct {
		OK                       bool                  `json:"ok"`
		Accepted                 bool                  `json:"accepted"`
		Deduplicated             bool                  `json:"deduplicated"`
		Recorded                 InboundWebhookReceipt `json:"recorded"`
		Signal                   any                   `json:"signal"`
		AutomationSendPlan       any                   `json:"automation_send_plan"`
		ExternalEffectJobID      any                   `json:"external_effect_job_id"`
		RecordOnly               bool                  `json:"record_only"`
		RealExternalCallExecuted bool                  `json:"real_external_call_executed"`
	}{
		OK: true, Accepted: true, Deduplicated: result.Replayed, Recorded: result.Receipt,
		RecordOnly: true, RealExternalCallExecuted: false,
	}
	writeJSON(writer, http.StatusOK, response)
}

func inboundWebhookPackageID(request *http.Request) (int64, *requestProblem) {
	if request.URL.RawPath != "" || !strings.HasPrefix(request.URL.Path, InboundWebhookPathPrefix) || !strings.HasSuffix(request.URL.Path, "/webhook") ||
		strings.Contains(request.RequestURI, "%") || strings.Contains(request.URL.Path, "//") || strings.Contains(request.URL.Path, "\\") {
		return 0, malformed("path", "invalid")
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, InboundWebhookPathPrefix), "/webhook")
	if raw == "" || strings.Contains(raw, "/") {
		return 0, malformed("path", "invalid")
	}
	return parseID(raw, "package_id")
}

func readInboundWebhookBody(writer http.ResponseWriter, request *http.Request) ([]byte, *requestProblem) {
	if request.Body == nil {
		return nil, malformed("body", "required")
	}
	contentTypes := request.Header.Values("Content-Type")
	if len(contentTypes) != 1 {
		return nil, &requestProblem{status: http.StatusUnsupportedMediaType, code: "MALFORMED_REQUEST", field: "content_type", reason: "application_json_required"}
	}
	mediaType, _, err := mime.ParseMediaType(contentTypes[0])
	if err != nil || mediaType != "application/json" {
		return nil, &requestProblem{status: http.StatusUnsupportedMediaType, code: "MALFORMED_REQUEST", field: "content_type", reason: "application_json_required"}
	}
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, MaximumRequestBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, &requestProblem{status: http.StatusRequestEntityTooLarge, code: "MALFORMED_REQUEST", field: "body", reason: "too_large"}
		}
		return nil, malformed("body", "invalid")
	}
	if len(body) == 0 {
		return nil, malformed("body", "required")
	}
	return body, nil
}

func decodeInboundWebhookBody(body []byte) (InboundWebhookInput, *requestProblem) {
	allowed := map[string]bool{"external_event_id": true, "member_event_id": true, "status": true, "message": true, "action": true}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	opening, err := decoder.Token()
	if err != nil {
		return InboundWebhookInput{}, malformed("body", "invalid_json")
	}
	if delimiter, ok := opening.(json.Delim); !ok || delimiter != '{' {
		return InboundWebhookInput{}, malformed("body", "object_required")
	}
	fields := make(map[string]json.RawMessage, len(allowed))
	for decoder.More() {
		keyToken, tokenErr := decoder.Token()
		key, ok := keyToken.(string)
		if tokenErr != nil || !ok {
			return InboundWebhookInput{}, malformed("body", "invalid_json")
		}
		if !allowed[key] {
			return InboundWebhookInput{}, malformed("body", "unknown_field")
		}
		if _, duplicate := fields[key]; duplicate {
			return InboundWebhookInput{}, malformed(key, "duplicate")
		}
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil {
			return InboundWebhookInput{}, malformed(key, "invalid_json")
		}
		fields[key] = append(json.RawMessage(nil), raw...)
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return InboundWebhookInput{}, malformed("body", "invalid_json")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return InboundWebhookInput{}, malformed("body", "trailing_data")
	}
	externalEventID, problem := requiredString(fields, "external_event_id")
	if problem != nil {
		return InboundWebhookInput{}, problem
	}
	input := InboundWebhookInput{ExternalEventID: externalEventID, Message: json.RawMessage(`{}`), Action: json.RawMessage(`{}`)}
	if raw, present := fields["member_event_id"]; present && string(raw) != "null" {
		value, parseProblem := integerValue(raw, "member_event_id", 1, 1<<62)
		if parseProblem != nil {
			return InboundWebhookInput{}, parseProblem
		}
		input.MemberEventID = &value
	}
	if raw, present := fields["status"]; present {
		value, parseProblem := stringValue(raw, "status")
		if parseProblem != nil {
			return InboundWebhookInput{}, parseProblem
		}
		input.Status = value
	}
	if raw, present := fields["message"]; present {
		if !validJSONObject(raw) {
			return InboundWebhookInput{}, validation("message", "object_required")
		}
		input.Message = append(json.RawMessage(nil), raw...)
	}
	if raw, present := fields["action"]; present {
		if !validJSONObject(raw) {
			return InboundWebhookInput{}, validation("action", "object_required")
		}
		input.Action = append(json.RawMessage(nil), raw...)
	}
	if !validInboundEventID(input.ExternalEventID, 1) || len(input.Status) > 64 || strings.TrimSpace(input.Status) != input.Status {
		return InboundWebhookInput{}, validation("body", "invalid")
	}
	return input, nil
}

func setInboundWebhookHeaders(writer http.ResponseWriter) {
	setSecurityHeaders(writer)
	if writer != nil {
		writer.Header().Set("X-AICRM-Real-External-Call-Executed", "false")
	}
}

type RetiredOutboundSubscriptionAuthenticator interface {
	Authenticate(context.Context, *http.Request) error
}

type RetiredOutboundSubscriptionHandler struct {
	authenticator RetiredOutboundSubscriptionAuthenticator
}

func NewRetiredOutboundSubscriptionHandler(authenticator RetiredOutboundSubscriptionAuthenticator) (*RetiredOutboundSubscriptionHandler, error) {
	if nilInterface(authenticator) {
		return nil, ErrUnavailable
	}
	return &RetiredOutboundSubscriptionHandler{authenticator: authenticator}, nil
}

func (handler *RetiredOutboundSubscriptionHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	setInboundWebhookHeaders(writer)
	if handler == nil || nilInterface(handler.authenticator) || request == nil {
		writeFailure(writer, request, ErrUnavailable)
		return
	}
	if err := handler.authenticator.Authenticate(request.Context(), request); err != nil {
		writeFailure(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusGone, map[string]any{"ok": false, "error": "webhook_configuration_retired"})
}
