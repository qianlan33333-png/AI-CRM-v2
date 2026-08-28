package main

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (h *Handler) ListServicePeriodHistoryDefinitions(w http.ResponseWriter, r *http.Request) {
	h.listServicePeriodHistory(w, r, "definitions")
}
func (h *Handler) ListServicePeriodHistoryEntitlements(w http.ResponseWriter, r *http.Request) {
	h.listServicePeriodHistory(w, r, "entitlements")
}
func (h *Handler) ListServicePeriodHistoryEvents(w http.ResponseWriter, r *http.Request) {
	h.listServicePeriodHistory(w, r, "events")
}

func (h *Handler) listServicePeriodHistory(w http.ResponseWriter, r *http.Request, kind string) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.servicePeriodHistory) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "service_period_history_unavailable"})
		return
	}
	limit, offset, ok := servicePeriodHistoryPage(r.URL.RawQuery)
	id := int64(0)
	if kind != "definitions" {
		var err error
		id, err = strconv.ParseInt(chi.URLParam(r, "definition_id"), 10, 64)
		ok = ok && err == nil && id > 0
	}
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_service_period_history_query"})
		return
	}
	var items any
	var total int64
	var err error
	switch kind {
	case "definitions":
		items, total, err = h.servicePeriodHistory.ListServicePeriodHistoryDefinitions(r.Context(), limit, offset)
	case "entitlements":
		items, total, err = h.servicePeriodHistory.ListServicePeriodHistoryEntitlements(r.Context(), id, limit, offset)
	case "events":
		items, total, err = h.servicePeriodHistory.ListServicePeriodHistoryEvents(r.Context(), id, limit, offset)
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "service_period_history_unavailable"})
		return
	}
	response := map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "items": items, "total": total, "limit": limit, "offset": offset}
	if kind != "definitions" {
		response["definition_id"] = id
	}
	writeJSON(w, http.StatusOK, response)
}

func servicePeriodHistoryPage(raw string) (int32, int32, bool) {
	query, err := url.ParseQuery(raw)
	if err != nil {
		return 0, 0, false
	}
	limit, offset := int64(50), int64(0)
	for key, values := range query {
		if len(values) != 1 || key != "limit" && key != "offset" {
			return 0, 0, false
		}
		value, err := strconv.ParseInt(values[0], 10, 32)
		if err != nil {
			return 0, 0, false
		}
		if key == "limit" {
			limit = value
		} else {
			offset = value
		}
	}
	return int32(limit), int32(offset), limit >= 1 && limit <= 100 && offset >= 0
}
