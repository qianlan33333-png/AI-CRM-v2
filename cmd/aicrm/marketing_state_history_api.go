package main

import (
	"context"
	"github.com/go-chi/chi/v5"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	"net/http"
)

func (h *Handler) ListMarketingStateHistorySnapshot(w http.ResponseWriter, r *http.Request) {
	h.serveMarketingStateHistory(w, r, "MarketingStateSnapshot", false)
}
func (h *Handler) GetMarketingStateHistorySnapshot(w http.ResponseWriter, r *http.Request) {
	h.serveMarketingStateHistory(w, r, "MarketingStateSnapshot", true)
}
func (h *Handler) ListMarketingStateHistoryChange(w http.ResponseWriter, r *http.Request) {
	h.serveMarketingStateHistory(w, r, "MarketingStateChange", false)
}
func (h *Handler) GetMarketingStateHistoryChange(w http.ResponseWriter, r *http.Request) {
	h.serveMarketingStateHistory(w, r, "MarketingStateChange", true)
}
func (h *Handler) ListMarketingStateHistoryValueSnapshot(w http.ResponseWriter, r *http.Request) {
	h.serveMarketingStateHistory(w, r, "ValueSegmentSnapshot", false)
}
func (h *Handler) GetMarketingStateHistoryValueSnapshot(w http.ResponseWriter, r *http.Request) {
	h.serveMarketingStateHistory(w, r, "ValueSegmentSnapshot", true)
}
func (h *Handler) ListMarketingStateHistoryValueChange(w http.ResponseWriter, r *http.Request) {
	h.serveMarketingStateHistory(w, r, "ValueSegmentChange", false)
}
func (h *Handler) GetMarketingStateHistoryValueChange(w http.ResponseWriter, r *http.Request) {
	h.serveMarketingStateHistory(w, r, "ValueSegmentChange", true)
}

func (h *Handler) serveMarketingStateHistory(w http.ResponseWriter, r *http.Request, kind string, detail bool) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.marketingStateHistory) {
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_marketing_state_history_query"})
		return
	}
	query := segmentport.MarketingStateHistoryQuery{Limit: limit, Offset: offset}
	switch kind {
	case "MarketingStateSnapshot":
		writeStaticHistory(w, r, detail, id, limit, offset, h.marketingStateHistory.GetHistoricalMarketingStateSnapshot, func(ctx context.Context) ([]segmentport.HistoricalMarketingStateSnapshot, int64, error) {
			return h.marketingStateHistory.ListHistoricalMarketingStateSnapshot(ctx, query)
		}, func(value segmentport.HistoricalMarketingStateSnapshot) (int64, error) {
			_, err := segmentapp.HistoricalMarketingStateSnapshotDigest(value)
			return value.ID, err
		})
	case "MarketingStateChange":
		writeStaticHistory(w, r, detail, id, limit, offset, h.marketingStateHistory.GetHistoricalMarketingStateChange, func(ctx context.Context) ([]segmentport.HistoricalMarketingStateChange, int64, error) {
			return h.marketingStateHistory.ListHistoricalMarketingStateChange(ctx, query)
		}, func(value segmentport.HistoricalMarketingStateChange) (int64, error) {
			_, err := segmentapp.HistoricalMarketingStateChangeDigest(value)
			return value.ID, err
		})
	case "ValueSegmentSnapshot":
		writeStaticHistory(w, r, detail, id, limit, offset, h.marketingStateHistory.GetHistoricalValueSegmentSnapshot, func(ctx context.Context) ([]segmentport.HistoricalValueSegmentSnapshot, int64, error) {
			return h.marketingStateHistory.ListHistoricalValueSegmentSnapshot(ctx, query)
		}, func(value segmentport.HistoricalValueSegmentSnapshot) (int64, error) {
			_, err := segmentapp.HistoricalValueSegmentSnapshotDigest(value)
			return value.ID, err
		})
	case "ValueSegmentChange":
		writeStaticHistory(w, r, detail, id, limit, offset, h.marketingStateHistory.GetHistoricalValueSegmentChange, func(ctx context.Context) ([]segmentport.HistoricalValueSegmentChange, int64, error) {
			return h.marketingStateHistory.ListHistoricalValueSegmentChange(ctx, query)
		}, func(value segmentport.HistoricalValueSegmentChange) (int64, error) {
			_, err := segmentapp.HistoricalValueSegmentChangeDigest(value)
			return value.ID, err
		})
	default:
		staticHistoryUnavailable(w)
	}
}
