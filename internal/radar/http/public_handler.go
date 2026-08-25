package http

import (
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"reflect"
	"strings"

	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

const (
	PublicRedirectPattern = "/r/{code}"
	PublicEventPattern    = "/api/h5/radar-contents/{code}/events"
)

type PublicHandler struct{ application radarport.TrackingApplication }

func NewPublicHandler(application radarport.TrackingApplication) (*PublicHandler, error) {
	if nilTrackingApplication(application) {
		return nil, radarport.ErrUnavailable
	}
	return &PublicHandler{application: application}, nil
}

func (handler *PublicHandler) Redirect(writer stdhttp.ResponseWriter, request *stdhttp.Request, code string) {
	if request == nil || request.Method != stdhttp.MethodGet || strings.TrimSpace(code) != code || request.URL == nil || request.URL.RawQuery != "" || requireEmptyBody(request) != nil {
		writePublicError(writer, stdhttp.StatusBadRequest, "malformed_request")
		return
	}
	result, err := handler.application.ResolvePublicRedirect(request.Context(), code)
	if err != nil {
		writePublicApplicationError(writer, err)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("X-Radar-Receipt-ID", result.Receipt.ReceiptID)
	stdhttp.Redirect(writer, request, result.DestinationURL, stdhttp.StatusFound)
}

func (handler *PublicHandler) RecordEvent(writer stdhttp.ResponseWriter, request *stdhttp.Request, code string) {
	if request == nil || request.Method != stdhttp.MethodPost || strings.TrimSpace(code) != code || request.URL == nil || request.URL.RawQuery != "" {
		writePublicError(writer, stdhttp.StatusBadRequest, "malformed_request")
		return
	}
	key, ok := idempotencyKey(request)
	if !ok {
		writePublicError(writer, stdhttp.StatusBadRequest, "idempotency_key_invalid")
		return
	}
	var body struct {
		Stage *radarport.EventStage `json:"stage"`
		Page  *int32                `json:"page,omitempty"`
		Extra map[string]any        `json:"extra,omitempty"`
	}
	if err := decodeStrictJSON(request, &body); err != nil || body.Stage == nil {
		writePublicError(writer, stdhttp.StatusBadRequest, "malformed_request")
		return
	}
	receipt, err := handler.application.RecordPublicEvent(request.Context(), radarport.RecordEventCommand{PublicCode: code, Stage: *body.Stage, Page: body.Page, Extra: body.Extra, IdempotencyKey: key})
	if err != nil {
		writePublicApplicationError(writer, err)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writeJSON(writer, stdhttp.StatusOK, struct {
		OK bool `json:"ok"`
		radarport.EventReceipt
	}{OK: true, EventReceipt: receipt})
}

func writePublicApplicationError(writer stdhttp.ResponseWriter, err error) {
	switch {
	case errors.Is(err, radarport.ErrNotFound):
		writePublicError(writer, stdhttp.StatusNotFound, "not_found")
	case errors.Is(err, radarport.ErrInvalidArgument):
		writePublicError(writer, stdhttp.StatusBadRequest, "invalid_event")
	case errors.Is(err, radarport.ErrIdempotencyConflict):
		writePublicError(writer, stdhttp.StatusConflict, "idempotency_conflict")
	default:
		writePublicError(writer, stdhttp.StatusServiceUnavailable, "unavailable")
	}
}

func writePublicError(writer stdhttp.ResponseWriter, status int, code string) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"ok": false, "error_code": code})
}

func nilTrackingApplication(value radarport.TrackingApplication) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Pointer && reflected.IsNil()
}
