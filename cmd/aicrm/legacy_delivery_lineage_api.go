package main

import (
	"errors"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const (
	legacyDeliveryLineagePath        = "/api/admin/delivery-lineage"
	legacyDeliveryLineageDefaultSize = int64(50)
	legacyDeliveryLineageMaximumSize = int64(100)
	legacyDeliveryLineageMaximumSkip = int64(1_000_000)
)

var (
	errInvalidDeliveryLineageQuery        = errors.New("invalid delivery lineage query")
	deliveryLineageDecimalPattern         = regexp.MustCompile(`^[0-9]+$`)
	deliveryLineagePositiveDecimalPattern = regexp.MustCompile(`^[1-9][0-9]*$`)
	deliveryLineageEventIDPattern         = regexp.MustCompile(`^event-delivery:v1:[0-9a-f]{64}$`)
)

type legacyDeliveryLineageReaders struct {
	outbound outboundport.DeliveryLineageReader
	events   eventport.DeliveryLineageReader
}

type legacyDeliveryLineageQuery struct {
	Limit  int64
	Offset int64
}

type legacyDeliveryLineageRecord struct {
	LineageID        string `json:"lineage_id"`
	RecordKind       string `json:"record_kind"`
	InternalState    string `json:"internal_state"`
	AttemptCount     int32  `json:"attempt_count"`
	UpdatedAt        string `json:"updated_at"`
	ExternalDelivery string `json:"external_delivery"`
	ExternalReceipt  string `json:"external_receipt"`
}

type legacyDeliveryLineageInterpretation struct {
	Kind             string `json:"kind"`
	ExternalDelivery string `json:"external_delivery"`
	ExternalReceipt  string `json:"external_receipt"`
}

type legacyDeliveryLineageSuccess struct {
	OK             bool                                `json:"ok"`
	Items          []legacyDeliveryLineageRecord       `json:"items"`
	Limit          int64                               `json:"limit"`
	Offset         int64                               `json:"offset"`
	HasMore        bool                                `json:"has_more"`
	Interpretation legacyDeliveryLineageInterpretation `json:"interpretation"`
}

func (handler *Handler) GetDeliveryLineage(writer http.ResponseWriter, request *http.Request) {
	if !legacyDeliveryLineageAuthorized(request) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	if handler == nil || handler.deliveryLineage.outbound == nil || handler.deliveryLineage.events == nil || request.URL == nil {
		writeLegacyDeliveryLineageUnavailable(writer)
		return
	}
	query, err := parseLegacyDeliveryLineageQuery(request.URL.RawQuery)
	if err != nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeValidationFailed, err))
		return
	}
	window, err := deliveryLineageSourceWindow(query)
	if err != nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeValidationFailed, err))
		return
	}
	outbound, err := handler.deliveryLineage.outbound.ListDeliveryLineage(request.Context(), window)
	if err != nil {
		writeLegacyDeliveryLineageUnavailable(writer)
		return
	}
	events, err := handler.deliveryLineage.events.ListDeliveryLineage(request.Context(), window)
	if err != nil {
		writeLegacyDeliveryLineageUnavailable(writer)
		return
	}
	merged, ok := mergeLegacyDeliveryLineage(outbound, events)
	if !ok {
		writeLegacyDeliveryLineageUnavailable(writer)
		return
	}
	start := int(query.Offset)
	items := []legacyDeliveryLineageRecord{}
	hasMore := false
	if start < len(merged) {
		end := start + int(query.Limit)
		if end > len(merged) {
			end = len(merged)
		}
		items = append(items, merged[start:end]...)
		hasMore = len(merged) > end
	}
	writeJSON(writer, http.StatusOK, legacyDeliveryLineageSuccess{
		OK: true, Items: items, Limit: query.Limit, Offset: query.Offset, HasMore: hasMore,
		Interpretation: legacyDeliveryLineageInterpretation{Kind: "internal_processing_only", ExternalDelivery: "unknown", ExternalReceipt: "unknown"},
	})
}

func parseLegacyDeliveryLineageQuery(rawQuery string) (legacyDeliveryLineageQuery, error) {
	if !utf8.ValidString(rawQuery) {
		return legacyDeliveryLineageQuery{}, errInvalidDeliveryLineageQuery
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return legacyDeliveryLineageQuery{}, errInvalidDeliveryLineageQuery
	}
	for key, values := range values {
		if (key != "limit" && key != "offset") || !utf8.ValidString(key) || len(values) != 1 || !utf8.ValidString(values[0]) {
			return legacyDeliveryLineageQuery{}, errInvalidDeliveryLineageQuery
		}
	}
	limit, err := parseDeliveryLineageInteger(values, "limit", legacyDeliveryLineageDefaultSize, 1, legacyDeliveryLineageMaximumSize)
	if err != nil {
		return legacyDeliveryLineageQuery{}, err
	}
	offset, err := parseDeliveryLineageInteger(values, "offset", 0, 0, legacyDeliveryLineageMaximumSkip)
	if err != nil {
		return legacyDeliveryLineageQuery{}, err
	}
	return legacyDeliveryLineageQuery{Limit: limit, Offset: offset}, nil
}

func parseDeliveryLineageInteger(values url.Values, key string, fallback, minimum, maximum int64) (int64, error) {
	entries, exists := values[key]
	if !exists {
		return fallback, nil
	}
	if len(entries) != 1 || !deliveryLineageDecimalPattern.MatchString(entries[0]) {
		return 0, errInvalidDeliveryLineageQuery
	}
	value, err := strconv.ParseInt(entries[0], 10, 64)
	if err != nil || value < minimum || value > maximum {
		return 0, errInvalidDeliveryLineageQuery
	}
	return value, nil
}

func deliveryLineageSourceWindow(query legacyDeliveryLineageQuery) (int32, error) {
	if query.Limit < 1 || query.Offset < 0 || query.Offset > math.MaxInt64-query.Limit-1 {
		return 0, errInvalidDeliveryLineageQuery
	}
	window := query.Offset + query.Limit + 1
	if window > math.MaxInt32 {
		return 0, errInvalidDeliveryLineageQuery
	}
	return int32(window), nil
}

func mergeLegacyDeliveryLineage(outbound outboundport.DeliveryLineagePage, events eventport.DeliveryLineagePage) ([]legacyDeliveryLineageRecord, bool) {
	if !outbound.Complete || !events.Complete {
		return nil, false
	}
	merged := make([]legacyDeliveryLineageRecord, 0, len(outbound.Items)+len(events.Items))
	seen := make(map[string]struct{}, len(outbound.Items)+len(events.Items))
	for _, item := range outbound.Items {
		record, ok := legacyOutboundDeliveryLineageRecord(item)
		if !ok {
			return nil, false
		}
		if _, duplicate := seen[record.LineageID]; duplicate {
			return nil, false
		}
		seen[record.LineageID] = struct{}{}
		merged = append(merged, record)
	}
	for _, item := range events.Items {
		record, ok := legacyEventDeliveryLineageRecord(item)
		if !ok {
			return nil, false
		}
		if _, duplicate := seen[record.LineageID]; duplicate {
			return nil, false
		}
		seen[record.LineageID] = struct{}{}
		merged = append(merged, record)
	}
	sort.Slice(merged, func(left, right int) bool {
		leftTime, _ := time.Parse(time.RFC3339Nano, merged[left].UpdatedAt)
		rightTime, _ := time.Parse(time.RFC3339Nano, merged[right].UpdatedAt)
		if !leftTime.Equal(rightTime) {
			return leftTime.After(rightTime)
		}
		if merged[left].RecordKind != merged[right].RecordKind {
			return merged[left].RecordKind < merged[right].RecordKind
		}
		return merged[left].LineageID < merged[right].LineageID
	})
	return merged, true
}

func legacyOutboundDeliveryLineageRecord(item outboundport.DeliveryLineageItem) (legacyDeliveryLineageRecord, bool) {
	if !strings.HasPrefix(item.LineageID, "outbound-task:") || !legacyPositiveDecimal(item.LineageID[len("outbound-task:"):]) || !legacyOutboundDeliveryState(item.InternalState) || item.AttemptCount < 0 || item.UpdatedAt.IsZero() {
		return legacyDeliveryLineageRecord{}, false
	}
	return legacyDeliveryLineageRecord{LineageID: item.LineageID, RecordKind: "outbound_task", InternalState: item.InternalState, AttemptCount: item.AttemptCount, UpdatedAt: item.UpdatedAt.UTC().Format(time.RFC3339Nano), ExternalDelivery: "unknown", ExternalReceipt: "unknown"}, true
}

func legacyEventDeliveryLineageRecord(item eventport.DeliveryLineageItem) (legacyDeliveryLineageRecord, bool) {
	if !deliveryLineageEventIDPattern.MatchString(item.LineageID) || !legacyEventDeliveryState(item.InternalState) || item.AttemptCount < 0 || item.UpdatedAt.IsZero() {
		return legacyDeliveryLineageRecord{}, false
	}
	return legacyDeliveryLineageRecord{LineageID: item.LineageID, RecordKind: "event_delivery", InternalState: item.InternalState, AttemptCount: item.AttemptCount, UpdatedAt: item.UpdatedAt.UTC().Format(time.RFC3339Nano), ExternalDelivery: "unknown", ExternalReceipt: "unknown"}, true
}

func legacyPositiveDecimal(value string) bool {
	if !deliveryLineagePositiveDecimalPattern.MatchString(value) {
		return false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed > 0
}

func legacyOutboundDeliveryState(state string) bool {
	switch state {
	case "pending", "sending", "sent", "retryable_failed", "final_failed", "outcome_unknown", "cancelled":
		return true
	default:
		return false
	}
}

func legacyEventDeliveryState(state string) bool {
	switch state {
	case "pending", "processing", "completed", "final_failed", "outcome_unknown":
		return true
	default:
		return false
	}
}

func writeLegacyDeliveryLineageUnavailable(writer http.ResponseWriter) {
	platformhttp.MarkCompatibilityError(writer, platformhttp.CodeDependencyUnavailable)
	writeJSON(writer, http.StatusServiceUnavailable, map[string]any{"ok": false, "status_code": http.StatusServiceUnavailable, "error_code": "delivery_lineage_unavailable"})
}

func writeLegacyDeliveryLineageMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Allow", http.MethodGet)
	writer.WriteHeader(http.StatusMethodNotAllowed)
}

func legacyDeliveryLineageAuthorized(request *http.Request) bool {
	if request == nil {
		return false
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	return principalOK && principal.AdminUserID > 0 && principal.Role == authport.RoleAdmin && authorizationOK && authorization.Capability == authport.CapabilityAdminRead && authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}
