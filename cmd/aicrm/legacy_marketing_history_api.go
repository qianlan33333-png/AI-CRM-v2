package main

import (
	"context"
	"github.com/go-chi/chi/v5"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	"net/http"
)

func (h *Handler) ListLegacyMarketingHistoryStates(w http.ResponseWriter, r *http.Request) {
	h.serveLegacyMarketingHistory(w, r, "State", false)
}
func (h *Handler) GetLegacyMarketingHistoryState(w http.ResponseWriter, r *http.Request) {
	h.serveLegacyMarketingHistory(w, r, "State", true)
}
func (h *Handler) ListLegacyMarketingHistoryValues(w http.ResponseWriter, r *http.Request) {
	h.serveLegacyMarketingHistory(w, r, "Value", false)
}
func (h *Handler) GetLegacyMarketingHistoryValue(w http.ResponseWriter, r *http.Request) {
	h.serveLegacyMarketingHistory(w, r, "Value", true)
}
func (h *Handler) serveLegacyMarketingHistory(w http.ResponseWriter, r *http.Request, kind string, detail bool) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.legacyMarketingHistory) {
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_legacy_marketing_history_query"})
		return
	}
	query := segmentport.LegacyMarketingHistoryQuery{Limit: limit, Offset: offset}
	switch kind {
	case "State":
		writeStaticHistory(w, r, detail, id, limit, offset, h.legacyMarketingHistory.GetHistoricalLegacyMarketingState, func(ctx context.Context) ([]segmentport.HistoricalLegacyMarketingState, int64, error) {
			return h.legacyMarketingHistory.ListHistoricalLegacyMarketingState(ctx, query)
		}, func(v segmentport.HistoricalLegacyMarketingState) (int64, error) {
			_, err := segmentapp.HistoricalLegacyMarketingStateDigest(v)
			return v.ID, err
		})
	case "Value":
		writeStaticHistory(w, r, detail, id, limit, offset, h.legacyMarketingHistory.GetHistoricalLegacyMarketingValue, func(ctx context.Context) ([]segmentport.HistoricalLegacyMarketingValue, int64, error) {
			return h.legacyMarketingHistory.ListHistoricalLegacyMarketingValue(ctx, query)
		}, func(v segmentport.HistoricalLegacyMarketingValue) (int64, error) {
			_, err := segmentapp.HistoricalLegacyMarketingValueDigest(v)
			return v.ID, err
		})
	default:
		staticHistoryUnavailable(w)
	}
}
