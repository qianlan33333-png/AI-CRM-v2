package main

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

func (h *Handler) ListOutboundTaskHistory(w http.ResponseWriter, r *http.Request) {
	h.serveOutboundTaskHistory(w, r, false)
}

func (h *Handler) GetOutboundTaskHistory(w http.ResponseWriter, r *http.Request) {
	h.serveOutboundTaskHistory(w, r, true)
}

func (h *Handler) serveOutboundTaskHistory(w http.ResponseWriter, r *http.Request, detail bool) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.outboundTaskHistory) {
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_outbound_task_history_query"})
		return
	}
	writeStaticHistory(w, r, detail, id, limit, offset, h.outboundTaskHistory.GetHistoricalOutboundTask, func(ctx context.Context) ([]outboundport.HistoricalOutboundTask, int64, error) {
		return h.outboundTaskHistory.ListHistoricalOutboundTasks(ctx, outboundport.OutboundTaskHistoryQuery{Limit: limit, Offset: offset})
	}, func(value outboundport.HistoricalOutboundTask) (int64, error) {
		_, err := outboundapp.HistoricalOutboundTaskDigest(value)
		return value.ID, err
	})
}
