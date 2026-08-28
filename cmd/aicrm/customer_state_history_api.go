package main

import (
	"context"
	"github.com/go-chi/chi/v5"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"net/http"
)

func (h *Handler) ListCustomerStateHistorySnapshot(w http.ResponseWriter, r *http.Request) {
	h.serveCustomerStateHistory(w, r, "CustomerStatusSnapshot", false)
}
func (h *Handler) GetCustomerStateHistorySnapshot(w http.ResponseWriter, r *http.Request) {
	h.serveCustomerStateHistory(w, r, "CustomerStatusSnapshot", true)
}
func (h *Handler) ListCustomerStateHistoryChange(w http.ResponseWriter, r *http.Request) {
	h.serveCustomerStateHistory(w, r, "CustomerStatusChange", false)
}
func (h *Handler) GetCustomerStateHistoryChange(w http.ResponseWriter, r *http.Request) {
	h.serveCustomerStateHistory(w, r, "CustomerStatusChange", true)
}
func (h *Handler) ListCustomerStateHistoryClassTermTagMapping(w http.ResponseWriter, r *http.Request) {
	h.serveCustomerStateHistory(w, r, "ClassTermTagMapping", false)
}
func (h *Handler) GetCustomerStateHistoryClassTermTagMapping(w http.ResponseWriter, r *http.Request) {
	h.serveCustomerStateHistory(w, r, "ClassTermTagMapping", true)
}

func (h *Handler) serveCustomerStateHistory(w http.ResponseWriter, r *http.Request, kind string, detail bool) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.customerStateHistory) {
		staticHistoryUnavailable(w)
		return
	}
	var id int64
	var limit, offset int32
	var valid bool
	if detail {
		id, valid = audienceHistoryID(chi.URLParam(r, "history_id"))
		valid = valid && r.URL.RawQuery == ""
	} else {
		limit, offset, valid = audienceHistoryPage(r.URL.RawQuery)
	}
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_customer_state_history_query"})
		return
	}
	query := contactport.CustomerStateHistoryQuery{Limit: limit, Offset: offset}
	switch kind {
	case "CustomerStatusSnapshot":
		writeStaticHistory(w, r, detail, id, limit, offset, h.customerStateHistory.GetHistoricalCustomerStatusSnapshot, func(ctx context.Context) ([]contactport.HistoricalCustomerStatusSnapshot, int64, error) {
			return h.customerStateHistory.ListHistoricalCustomerStatusSnapshot(ctx, query)
		}, func(value contactport.HistoricalCustomerStatusSnapshot) (int64, error) {
			_, err := contactapp.HistoricalCustomerStatusSnapshotDigest(value)
			return value.ID, err
		})
	case "CustomerStatusChange":
		writeStaticHistory(w, r, detail, id, limit, offset, h.customerStateHistory.GetHistoricalCustomerStatusChange, func(ctx context.Context) ([]contactport.HistoricalCustomerStatusChange, int64, error) {
			return h.customerStateHistory.ListHistoricalCustomerStatusChange(ctx, query)
		}, func(value contactport.HistoricalCustomerStatusChange) (int64, error) {
			_, err := contactapp.HistoricalCustomerStatusChangeDigest(value)
			return value.ID, err
		})
	case "ClassTermTagMapping":
		writeStaticHistory(w, r, detail, id, limit, offset, h.customerStateHistory.GetHistoricalClassTermTagMapping, func(ctx context.Context) ([]contactport.HistoricalClassTermTagMapping, int64, error) {
			return h.customerStateHistory.ListHistoricalClassTermTagMapping(ctx, query)
		}, func(value contactport.HistoricalClassTermTagMapping) (int64, error) {
			_, err := contactapp.HistoricalClassTermTagMappingDigest(value)
			return value.ID, err
		})
	default:
		staticHistoryUnavailable(w)
	}
}
