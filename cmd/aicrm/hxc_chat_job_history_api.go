package main

import (
	"context"
	"net/http"

	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
)

func (h *Handler) ListHXCHistoryChatJob(w http.ResponseWriter, r *http.Request) {
	h.serveHXCChatJobHistory(w, r, false)
}

func (h *Handler) GetHXCHistoryChatJob(w http.ResponseWriter, r *http.Request) {
	h.serveHXCChatJobHistory(w, r, true)
}

func (h *Handler) serveHXCChatJobHistory(w http.ResponseWriter, r *http.Request, detail bool) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.hxcChatJobHistory) {
		hxcHistoryUnavailable(w)
		return
	}
	query, id, valid := parseHXCHistoryQuery(r, "runtime", detail)
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_hxc_history_query"})
		return
	}
	serveHXCRuntimeValue(w, r, query, id, detail,
		h.hxcChatJobHistory.GetHistoricalHXCChatJob,
		func(ctx context.Context, q hxcport.HXCHistoryQuery) ([]hxcport.HistoricalHXCChatJob, int64, error) {
			return h.hxcChatJobHistory.ListHistoricalHXCChatJob(ctx, hxcport.HXCChatJobHistoryQuery{Limit: q.Limit, Offset: q.Offset})
		},
		hxcapp.HistoricalHXCChatJobDigest,
		func(v hxcport.HistoricalHXCChatJob) int64 { return v.ID })
}
