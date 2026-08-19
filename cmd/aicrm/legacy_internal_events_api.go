package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	eventapp "github.com/qianlan33333-png/AI-CRM-v2/internal/events/app"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const (
	legacyInternalEventsPath            = "/api/admin/internal-events"
	legacyInternalEventsDiagnosticsPath = "/api/admin/internal-events/diagnostics"
)

type legacyInternalEventsHandler struct {
	service *eventapp.AdminReadService
}

func newLegacyInternalEventsHandler(repository eventport.AdminReadRepository) *legacyInternalEventsHandler {
	if repository == nil {
		return &legacyInternalEventsHandler{}
	}
	return &legacyInternalEventsHandler{service: eventapp.NewAdminReadService(repository, time.Now)}
}

type legacyInternalEventListResponse struct {
	OK                           bool                      `json:"ok"`
	Items                        []legacyInternalEventItem `json:"items"`
	Total                        int64                     `json:"total"`
	Limit                        int64                     `json:"limit"`
	Offset                       int64                     `json:"offset"`
	ObservedAt                   string                    `json:"observed_at"`
	RegistryID                   string                    `json:"registry_id"`
	SourceStatus                 string                    `json:"source_status"`
	DeliveryObservationAvailable bool                      `json:"delivery_observation_available"`
	ExternalDelivery             string                    `json:"external_delivery"`
	RouteOwner                   string                    `json:"route_owner"`
	RealExternalCallExecuted     bool                      `json:"real_external_call_executed"`
}

type legacyInternalEventItem struct {
	EventID    int64                             `json:"event_id"`
	EventType  string                            `json:"event_type"`
	OccurredAt string                            `json:"occurred_at"`
	Dispatched bool                              `json:"dispatched"`
	Deliveries []legacyInternalEventDeliveryItem `json:"deliveries"`
}

type legacyInternalEventDeliveryItem struct {
	Consumer     string  `json:"consumer"`
	Status       string  `json:"status"`
	AttemptCount int32   `json:"attempt_count"`
	CompletedAt  *string `json:"completed_at"`
}

type legacyInternalEventDiagnosticsResponse struct {
	OK                       bool                                 `json:"ok"`
	Filters                  legacyInternalEventFilters           `json:"filters"`
	EventCount               int64                                `json:"event_count"`
	UndispatchedEventCount   int64                                `json:"undispatched_event_count"`
	DeliveryCounts           legacyInternalEventDeliveryCounts    `json:"delivery_counts"`
	ConsumerRegistry         []legacyInternalEventConsumerBinding `json:"consumer_registry"`
	ObservedAt               string                               `json:"observed_at"`
	RegistryID               string                               `json:"registry_id"`
	SourceStatus             string                               `json:"source_status"`
	ObservedDomains          []string                             `json:"observed_domains"`
	UnobservedDomains        []string                             `json:"unobserved_domains"`
	ExternalDelivery         string                               `json:"external_delivery"`
	RouteOwner               string                               `json:"route_owner"`
	RealExternalCallExecuted bool                                 `json:"real_external_call_executed"`
}

type legacyInternalEventFilters struct {
	EventType string `json:"event_type"`
	Consumer  string `json:"consumer"`
	Status    string `json:"status"`
}

type legacyInternalEventDeliveryCounts struct {
	Pending        int64 `json:"pending"`
	Processing     int64 `json:"processing"`
	Completed      int64 `json:"completed"`
	FinalFailed    int64 `json:"final_failed"`
	OutcomeUnknown int64 `json:"outcome_unknown"`
}

type legacyInternalEventConsumerBinding struct {
	Consumer   string   `json:"consumer"`
	EventTypes []string `json:"event_types"`
}

func (handler *legacyInternalEventsHandler) List(writer http.ResponseWriter, request *http.Request) {
	if status, code, message := legacyInternalEventsAuthorizationFailure(request); status != 0 {
		writeLegacyInternalEventsError(writer, request, status, code, message)
		return
	}
	if request == nil || request.URL == nil {
		writeLegacyInternalEventsError(writer, request, http.StatusBadRequest, "internal_event_query_invalid", "invalid internal event query")
		return
	}
	query, err := parseLegacyInternalEventQuery(request.URL.RawQuery, true)
	if err != nil {
		writeLegacyInternalEventsError(writer, request, http.StatusBadRequest, "internal_event_query_invalid", "invalid internal event query")
		return
	}
	if handler == nil || handler.service == nil {
		writeLegacyInternalEventsError(writer, request, http.StatusServiceUnavailable, "internal_event_observation_unavailable", "internal event observation unavailable")
		return
	}
	result, err := handler.service.List(request.Context(), query)
	if err != nil {
		writeLegacyInternalEventsError(writer, request, http.StatusServiceUnavailable, "internal_event_observation_unavailable", "internal event observation unavailable")
		return
	}
	items := make([]legacyInternalEventItem, 0, len(result.Items))
	for _, item := range result.Items {
		mapped := legacyInternalEventItem{
			EventID: int64(item.EventID), EventType: item.EventType, OccurredAt: item.OccurredAt.UTC().Format(time.RFC3339Nano),
			Dispatched: item.Dispatched, Deliveries: make([]legacyInternalEventDeliveryItem, 0, len(item.Deliveries)),
		}
		for _, delivery := range item.Deliveries {
			var completedAt *string
			if delivery.CompletedAt != nil {
				value := delivery.CompletedAt.UTC().Format(time.RFC3339Nano)
				completedAt = &value
			}
			mapped.Deliveries = append(mapped.Deliveries, legacyInternalEventDeliveryItem{
				Consumer: delivery.Consumer, Status: delivery.Status, AttemptCount: delivery.AttemptCount, CompletedAt: completedAt,
			})
		}
		items = append(items, mapped)
	}
	writeLegacyInternalEventsJSON(writer, http.StatusOK, legacyInternalEventListResponse{
		OK: true, Items: items, Total: result.Total, Limit: result.Limit, Offset: result.Offset,
		ObservedAt: result.ObservedAt.UTC().Format(time.RFC3339Nano), RegistryID: eventport.AdminReadRegistryID,
		SourceStatus: "local_read_model", DeliveryObservationAvailable: true, ExternalDelivery: "unknown",
		RouteOwner: "ai_crm_next", RealExternalCallExecuted: false,
	})
}

func (handler *legacyInternalEventsHandler) Diagnostics(writer http.ResponseWriter, request *http.Request) {
	if status, code, message := legacyInternalEventsAuthorizationFailure(request); status != 0 {
		writeLegacyInternalEventsError(writer, request, status, code, message)
		return
	}
	if request == nil || request.URL == nil {
		writeLegacyInternalEventsError(writer, request, http.StatusBadRequest, "internal_event_query_invalid", "invalid internal event query")
		return
	}
	query, err := parseLegacyInternalEventQuery(request.URL.RawQuery, false)
	if err != nil {
		writeLegacyInternalEventsError(writer, request, http.StatusBadRequest, "internal_event_query_invalid", "invalid internal event query")
		return
	}
	if handler == nil || handler.service == nil {
		writeLegacyInternalEventsError(writer, request, http.StatusServiceUnavailable, "internal_event_observation_unavailable", "internal event observation unavailable")
		return
	}
	result, err := handler.service.Diagnostics(request.Context(), query)
	if err != nil {
		writeLegacyInternalEventsError(writer, request, http.StatusServiceUnavailable, "internal_event_observation_unavailable", "internal event observation unavailable")
		return
	}
	registry := make([]legacyInternalEventConsumerBinding, 0, len(result.ConsumerRegistry))
	for _, binding := range result.ConsumerRegistry {
		registry = append(registry, legacyInternalEventConsumerBinding{Consumer: binding.Consumer, EventTypes: append([]string(nil), binding.EventTypes...)})
	}
	writeLegacyInternalEventsJSON(writer, http.StatusOK, legacyInternalEventDiagnosticsResponse{
		OK: true, Filters: legacyInternalEventFilters{EventType: result.Filters.EventType, Consumer: result.Filters.Consumer, Status: result.Filters.Status},
		EventCount: result.EventCount, UndispatchedEventCount: result.UndispatchedEventCount,
		DeliveryCounts:   legacyInternalEventDeliveryCounts{Pending: result.DeliveryCounts.Pending, Processing: result.DeliveryCounts.Processing, Completed: result.DeliveryCounts.Completed, FinalFailed: result.DeliveryCounts.FinalFailed, OutcomeUnknown: result.DeliveryCounts.OutcomeUnknown},
		ConsumerRegistry: registry, ObservedAt: result.ObservedAt.UTC().Format(time.RFC3339Nano), RegistryID: eventport.AdminReadRegistryID,
		SourceStatus: "local_read_model", ObservedDomains: append([]string(nil), result.ObservedDomains...), UnobservedDomains: append([]string(nil), result.UnobservedDomains...),
		ExternalDelivery: "unknown", RouteOwner: "ai_crm_next", RealExternalCallExecuted: false,
	})
}

func parseLegacyInternalEventQuery(rawQuery string, withPagination bool) (eventport.AdminReadQuery, error) {
	query := eventport.AdminReadQuery{Limit: 50}
	if !withPagination {
		query.Limit = 0
	}
	if rawQuery == "" {
		return query, nil
	}
	if !utf8.ValidString(rawQuery) {
		return eventport.AdminReadQuery{}, errors.New("invalid internal event query")
	}
	allowed := map[string]bool{"event_type": true, "consumer": true, "status": true}
	if withPagination {
		allowed["limit"], allowed["offset"] = true, true
	}
	seen := make(map[string]struct{})
	for _, pair := range strings.Split(rawQuery, "&") {
		if pair == "" {
			return eventport.AdminReadQuery{}, errors.New("invalid internal event query")
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return eventport.AdminReadQuery{}, errors.New("invalid internal event query")
		}
		key, err := url.QueryUnescape(parts[0])
		if err != nil || key == "" || !utf8.ValidString(key) || !allowed[key] {
			return eventport.AdminReadQuery{}, errors.New("invalid internal event query")
		}
		if _, exists := seen[key]; exists {
			return eventport.AdminReadQuery{}, errors.New("invalid internal event query")
		}
		seen[key] = struct{}{}
		value, err := url.QueryUnescape(parts[1])
		if err != nil || !utf8.ValidString(value) {
			return eventport.AdminReadQuery{}, errors.New("invalid internal event query")
		}
		switch key {
		case "event_type":
			query.EventType, err = parseLegacyInternalEventText(value)
		case "consumer":
			query.Consumer, err = parseLegacyInternalEventText(value)
			if err == nil {
				if _, ok := eventport.AdminReadBindingForConsumer(query.Consumer); !ok {
					err = errors.New("invalid internal event query")
				}
			}
		case "status":
			query.Status, err = parseLegacyInternalEventText(value)
			if err == nil && !validLegacyInternalEventStatus(query.Status) {
				err = errors.New("invalid internal event query")
			}
		case "limit":
			query.Limit, err = parseLegacyInternalEventDecimal(value, 1, 200)
		case "offset":
			query.Offset, err = parseLegacyInternalEventDecimal(value, 0, 100000)
		}
		if err != nil {
			return eventport.AdminReadQuery{}, errors.New("invalid internal event query")
		}
	}
	return query, nil
}

func parseLegacyInternalEventText(value string) (string, error) {
	value = strings.Trim(value, " \t\r\n\v\f")
	if value == "" || !utf8.ValidString(value) || len(value) > 200 {
		return "", errors.New("invalid internal event query")
	}
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return "", errors.New("invalid internal event query")
		}
	}
	return value, nil
}

func parseLegacyInternalEventDecimal(value string, minimum, maximum int64) (int64, error) {
	if value == "" || (len(value) > 1 && value[0] == '0') {
		return 0, errors.New("invalid internal event query")
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, errors.New("invalid internal event query")
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, errors.New("invalid internal event query")
	}
	return parsed, nil
}

func validLegacyInternalEventStatus(value string) bool {
	for _, status := range eventport.AdminReadStatuses() {
		if value == status {
			return true
		}
	}
	return false
}

func legacyInternalEventsAuthorizationFailure(request *http.Request) (int, string, string) {
	if request == nil {
		return http.StatusUnauthorized, "authentication_required", "authentication required"
	}
	_, sessionOK := authport.SessionFromContext(request.Context())
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	if !sessionOK || !principalOK || principal.AdminUserID <= 0 {
		return http.StatusUnauthorized, "authentication_required", "authentication required"
	}
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	if !authorizationOK || principal.Role != authport.RoleAdmin || authorization.Capability != authport.CapabilityAdminRead || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return http.StatusForbidden, "forbidden", "forbidden"
	}
	return 0, "", ""
}

func legacyInternalEventsSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		buffered := &legacyInternalEventsResponseWriter{ResponseWriter: writer, header: writer.Header().Clone()}
		setLegacyInternalEventsHeaders(buffered)
		next.ServeHTTP(buffered, request)
		buffered.flush()
	})
}

type legacyInternalEventsResponseWriter struct {
	http.ResponseWriter
	header      http.Header
	wroteHeader bool
	status      int
	body        bytes.Buffer
}

func (writer *legacyInternalEventsResponseWriter) Header() http.Header { return writer.header }

func (writer *legacyInternalEventsResponseWriter) WriteHeader(status int) {
	if writer.wroteHeader {
		return
	}
	writer.wroteHeader = true
	writer.status = status
}

func (writer *legacyInternalEventsResponseWriter) Write(body []byte) (int, error) {
	if !writer.wroteHeader {
		writer.WriteHeader(http.StatusOK)
	}
	return writer.body.Write(body)
}

func (writer *legacyInternalEventsResponseWriter) flush() {
	if !writer.wroteHeader {
		writer.WriteHeader(http.StatusOK)
	}
	underlying := writer.ResponseWriter.Header()
	for key, values := range writer.header {
		underlying[key] = append([]string(nil), values...)
	}
	setLegacyInternalEventsHeaders(writer.ResponseWriter)
	if writer.status >= http.StatusBadRequest {
		platformhttp.MarkCompatibilityError(writer.ResponseWriter, legacyInternalEventsCompatibilityCode(writer.status))
	}
	body := normalizeLegacyInternalEventsError(writer.status, writer.body.Bytes(), writer.header.Get(platformhttp.RequestIDHeader))
	writer.ResponseWriter.WriteHeader(writer.status)
	_, _ = writer.ResponseWriter.Write(body)
}

func legacyInternalEventsCompatibilityCode(status int) platformhttp.ErrorCode {
	switch status {
	case http.StatusBadRequest:
		return platformhttp.CodeMalformedRequest
	case http.StatusMethodNotAllowed:
		return platformhttp.CodeMalformedRequest
	case http.StatusUnauthorized:
		return platformhttp.CodeUnauthenticated
	case http.StatusForbidden:
		return platformhttp.CodeUnauthorized
	case http.StatusServiceUnavailable:
		return platformhttp.CodeDependencyUnavailable
	case http.StatusInternalServerError:
		return platformhttp.CodeInternal
	default:
		return platformhttp.CodeInternal
	}
}

func normalizeLegacyInternalEventsError(status int, body []byte, requestID string) []byte {
	if status != http.StatusUnauthorized && status != http.StatusForbidden && status != http.StatusServiceUnavailable && status != http.StatusInternalServerError {
		return body
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err == nil && envelope != nil {
		if value, ok := envelope["request_id"].(string); ok && value != "" {
			requestID = value
		}
	}
	if requestID == "" {
		requestID = "request-id-unavailable"
	}
	errorCode := "authentication_required"
	message := "authentication required"
	switch status {
	case http.StatusForbidden:
		errorCode = "forbidden"
		message = "forbidden"
	case http.StatusServiceUnavailable:
		errorCode = "internal_event_observation_unavailable"
		message = "internal event observation unavailable"
	case http.StatusInternalServerError:
		errorCode = "internal_event_observation_failed"
		message = "internal event observation failed"
	}
	encoded, err := json.Marshal(struct {
		OK                       bool   `json:"ok"`
		StatusCode               int    `json:"status_code"`
		ErrorCode                string `json:"error_code"`
		Message                  string `json:"message"`
		RequestID                string `json:"request_id"`
		RealExternalCallExecuted bool   `json:"real_external_call_executed"`
	}{
		OK: false, StatusCode: status, ErrorCode: errorCode, Message: message,
		RequestID: requestID, RealExternalCallExecuted: false,
	})
	if err != nil {
		return body
	}
	return append(encoded, '\n')
}

func setLegacyInternalEventsHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}

func writeLegacyInternalEventsJSON(writer http.ResponseWriter, status int, value any) {
	setLegacyInternalEventsHeaders(writer)
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeLegacyInternalEventsError(writer http.ResponseWriter, request *http.Request, status int, code, message string) {
	compatibilityCode := platformhttp.CodeInternal
	switch status {
	case http.StatusBadRequest:
		compatibilityCode = platformhttp.CodeMalformedRequest
	case http.StatusMethodNotAllowed:
		compatibilityCode = platformhttp.CodeMalformedRequest
	case http.StatusUnauthorized:
		compatibilityCode = platformhttp.CodeUnauthenticated
	case http.StatusForbidden:
		compatibilityCode = platformhttp.CodeUnauthorized
	case http.StatusNotFound:
		compatibilityCode = platformhttp.CodeNotFound
	case http.StatusServiceUnavailable:
		compatibilityCode = platformhttp.CodeDependencyUnavailable
	}
	platformhttp.MarkCompatibilityError(writer, compatibilityCode)
	requestID := platformhttp.RequestID(nil)
	if request != nil {
		requestID = platformhttp.RequestID(request.Context())
	}
	if requestID == "" {
		requestID = "request-id-unavailable"
	}
	writeLegacyInternalEventsJSON(writer, status, struct {
		OK                       bool   `json:"ok"`
		StatusCode               int    `json:"status_code"`
		ErrorCode                string `json:"error_code"`
		Message                  string `json:"message"`
		RequestID                string `json:"request_id"`
		RealExternalCallExecuted bool   `json:"real_external_call_executed"`
	}{OK: false, StatusCode: status, ErrorCode: code, Message: message, RequestID: requestID, RealExternalCallExecuted: false})
}

func writeLegacyInternalEventsMethodNotAllowed(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Allow", http.MethodGet)
	writeLegacyInternalEventsError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
}
