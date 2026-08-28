package main

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

func (h *Handler) ListBroadcastJobHistory(w http.ResponseWriter, r *http.Request) {
	h.serveBroadcastJobHistory(w, r, false)
}
func (h *Handler) GetBroadcastJobHistory(w http.ResponseWriter, r *http.Request) {
	h.serveBroadcastJobHistory(w, r, true)
}

func (h *Handler) serveBroadcastJobHistory(w http.ResponseWriter, r *http.Request, detail bool) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.broadcastJobHistory) {
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_broadcast_job_history_query"})
		return
	}
	writeStaticHistory(w, r, detail, id, limit, offset, h.broadcastJobHistory.GetHistoricalBroadcastJob, func(ctx context.Context) ([]outboundport.HistoricalBroadcastJob, int64, error) {
		return h.broadcastJobHistory.ListHistoricalBroadcastJobs(ctx, outboundport.BroadcastJobHistoryQuery{Limit: limit, Offset: offset})
	}, func(v outboundport.HistoricalBroadcastJob) (int64, error) {
		_, err := outboundapp.HistoricalBroadcastJobDigest(v)
		return v.ID, err
	})
}
