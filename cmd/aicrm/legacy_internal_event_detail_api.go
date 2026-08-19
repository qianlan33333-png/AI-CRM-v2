package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	eventapp "github.com/qianlan33333-png/AI-CRM-v2/internal/events/app"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

const legacyInternalEventDetailPath = "/api/admin/internal-events/{event_id}"

type legacyInternalEventDetailHandler struct {
	service *eventapp.AdminDetailService
}

func newLegacyInternalEventDetailHandler(repository eventport.AdminDetailRepository) *legacyInternalEventDetailHandler {
	if repository == nil {
		return &legacyInternalEventDetailHandler{}
	}
	return &legacyInternalEventDetailHandler{service: eventapp.NewAdminDetailService(repository, nil)}
}

type legacyInternalEventDetailResponse struct {
	OK                           bool                    `json:"ok"`
	Item                         legacyInternalEventItem `json:"item"`
	ObservedAt                   string                  `json:"observed_at"`
	RegistryID                   string                  `json:"registry_id"`
	SourceStatus                 string                  `json:"source_status"`
	DeliveryObservationAvailable bool                    `json:"delivery_observation_available"`
	ExternalDelivery             string                  `json:"external_delivery"`
	RouteOwner                   string                  `json:"route_owner"`
	RealExternalCallExecuted     bool                    `json:"real_external_call_executed"`
}

func (handler *legacyInternalEventDetailHandler) Get(writer http.ResponseWriter, request *http.Request) {
	if status, code, message := legacyInternalEventsAuthorizationFailure(request); status != 0 {
		writeLegacyInternalEventsError(writer, request, status, code, message)
		return
	}
	if request == nil || request.URL == nil || request.URL.RawQuery != "" {
		writeLegacyInternalEventsError(writer, request, http.StatusBadRequest, "internal_event_detail_invalid", "invalid internal event detail request")
		return
	}
	rawID := legacyInternalEventDetailRawID(request)
	eventID, err := parseLegacyInternalEventDetailID(rawID)
	if err != nil {
		writeLegacyInternalEventsError(writer, request, http.StatusBadRequest, "internal_event_detail_invalid", "invalid internal event detail request")
		return
	}
	if handler == nil || handler.service == nil {
		writeLegacyInternalEventsError(writer, request, http.StatusServiceUnavailable, "internal_event_observation_unavailable", "internal event observation unavailable")
		return
	}
	result, err := handler.service.Get(request.Context(), eventport.EventID(eventID))
	if errors.Is(err, eventapp.ErrAdminDetailNotFound) {
		writeLegacyInternalEventsError(writer, request, http.StatusNotFound, "internal_event_not_found", "internal event not found")
		return
	}
	if err != nil {
		writeLegacyInternalEventsError(writer, request, http.StatusServiceUnavailable, "internal_event_observation_unavailable", "internal event observation unavailable")
		return
	}
	item := legacyInternalEventItem{
		EventID: int64(result.Item.EventID), EventType: result.Item.EventType,
		OccurredAt: result.Item.OccurredAt.UTC().Format(time.RFC3339Nano),
		Dispatched: result.Item.Dispatched, Deliveries: make([]legacyInternalEventDeliveryItem, 0, len(result.Item.Deliveries)),
	}
	for _, delivery := range result.Item.Deliveries {
		var completedAt *string
		if delivery.CompletedAt != nil {
			value := delivery.CompletedAt.UTC().Format(time.RFC3339Nano)
			completedAt = &value
		}
		item.Deliveries = append(item.Deliveries, legacyInternalEventDeliveryItem{
			Consumer: delivery.Consumer, Status: delivery.Status, AttemptCount: delivery.AttemptCount, CompletedAt: completedAt,
		})
	}
	writeLegacyInternalEventsJSON(writer, http.StatusOK, legacyInternalEventDetailResponse{
		OK: true, Item: item, ObservedAt: result.ObservedAt.UTC().Format(time.RFC3339Nano),
		RegistryID: eventport.AdminReadRegistryID, SourceStatus: "local_read_model",
		DeliveryObservationAvailable: true, ExternalDelivery: "unknown", RouteOwner: "ai_crm_next",
		RealExternalCallExecuted: false,
	})
}

func legacyInternalEventDetailRawID(request *http.Request) string {
	if request == nil || request.URL == nil {
		return ""
	}
	const prefix = "/api/admin/internal-events/"
	path := request.URL.EscapedPath()
	if strings.HasPrefix(path, prefix) {
		segment := strings.TrimPrefix(path, prefix)
		if strings.Contains(segment, "/") {
			return ""
		}
		// The path contract is canonical ASCII decimal. Reject encoded bytes
		// before chi can decode them into a different apparent identifier.
		if strings.Contains(segment, "%") {
			return ""
		}
		return segment
	}
	return chi.URLParam(request, "event_id")
}

func parseLegacyInternalEventDetailID(value string) (int64, error) {
	if value == "" || (len(value) > 1 && value[0] == '0') {
		return 0, errors.New("invalid internal event detail id")
	}
	for index := 0; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return 0, errors.New("invalid internal event detail id")
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, errors.New("invalid internal event detail id")
	}
	return parsed, nil
}
