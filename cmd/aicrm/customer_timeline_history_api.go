package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

func (h *Handler) ListCustomerTimelineHistoryEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.customerTimelineHistory) {
		customerTimelineHistoryUnavailable(w)
		return
	}
	limit, offset, ok := audienceHistoryPage(r.URL.RawQuery)
	if !ok {
		customerTimelineHistoryInvalid(w)
		return
	}
	items, total, err := h.customerTimelineHistory.ListHistoricalCustomerTimelineEvents(r.Context(), contact.CustomerTimelineHistoryQuery{Limit: limit, Offset: offset})
	if err != nil || total < 0 || int64(len(items)) != min(int64(limit), max(int64(0), total-int64(offset))) {
		customerTimelineHistoryUnavailable(w)
		return
	}
	for _, item := range items {
		if !validCustomerTimelineHistoryRead(item) {
			customerTimelineHistoryUnavailable(w)
			return
		}
	}
	if items == nil {
		items = []contact.CustomerTimelineHistoryRead{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "items": items, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) GetCustomerTimelineHistoryEvent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.customerTimelineHistory) {
		customerTimelineHistoryUnavailable(w)
		return
	}
	id, ok := audienceHistoryID(chi.URLParam(r, "history_id"))
	if !ok || r.URL.RawQuery != "" {
		customerTimelineHistoryInvalid(w)
		return
	}
	item, err := h.customerTimelineHistory.GetHistoricalCustomerTimelineEvent(r.Context(), id)
	if err != nil || item.ID != id || !validCustomerTimelineHistoryRead(item) {
		customerTimelineHistoryUnavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "item": item})
}

func validCustomerTimelineHistoryRead(value contact.CustomerTimelineHistoryRead) bool {
	return value.ID > 0 && !value.EventTime.IsZero() && !value.CreatedAt.IsZero() && (value.CustomerID == nil || *value.CustomerID > 0)
}

func customerTimelineHistoryInvalid(w http.ResponseWriter) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_customer_timeline_history_query"})
}

func customerTimelineHistoryUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "customer_timeline_history_unavailable"})
}
