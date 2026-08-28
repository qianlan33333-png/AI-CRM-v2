package main

import (
	"github.com/go-chi/chi/v5"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"net/http"
)

func (h *Handler) ListWeComContactHistoryEvents(w http.ResponseWriter, r *http.Request) {
	if !h.wecomContactHistoryReady(w) {
		return
	}
	limit, offset, _, ok := campaignHistoryQuery(r.URL.RawQuery, "")
	if !ok {
		wecomContactHistoryInvalid(w)
		return
	}
	rows, total, err := h.wecomContactHistory.ListHistoricalWeComExternalContactEventLog(r.Context(), contactport.WeComContactHistoryQuery{Limit: limit, Offset: offset})
	writeWeComContactHistoryPage(w, rows, total, limit, offset, err, func(row contactport.HistoricalWeComExternalContactEventLog) bool {
		_, e := contactapp.HistoricalWeComExternalContactEventLogDigest(row)
		return e == nil && row.ID > 0
	})
}

func (h *Handler) GetWeComContactHistoryEvent(w http.ResponseWriter, r *http.Request) {
	if !h.wecomContactHistoryReady(w) {
		return
	}
	id, ok := campaignHistoryID(chi.URLParam(r, "history_id"))
	if !ok || r.URL.RawQuery != "" {
		wecomContactHistoryInvalid(w)
		return
	}
	row, err := h.wecomContactHistory.GetHistoricalWeComExternalContactEventLog(r.Context(), id)
	_, digestErr := contactapp.HistoricalWeComExternalContactEventLogDigest(row)
	if err != nil || digestErr != nil || row.ID != id {
		wecomContactHistoryUnavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "item": row})
}

func (h *Handler) ListWeComContactHistoryRelations(w http.ResponseWriter, r *http.Request) {
	if !h.wecomContactHistoryReady(w) {
		return
	}
	limit, offset, _, ok := campaignHistoryQuery(r.URL.RawQuery, "")
	if !ok {
		wecomContactHistoryInvalid(w)
		return
	}
	rows, total, err := h.wecomContactHistory.ListHistoricalWeComExternalContactFollowUser(r.Context(), contactport.WeComContactHistoryQuery{Limit: limit, Offset: offset})
	writeWeComContactHistoryPage(w, rows, total, limit, offset, err, func(row contactport.HistoricalWeComExternalContactFollowUser) bool {
		_, e := contactapp.HistoricalWeComExternalContactFollowUserDigest(row)
		return e == nil && row.ID > 0
	})
}

func (h *Handler) GetWeComContactHistoryRelation(w http.ResponseWriter, r *http.Request) {
	if !h.wecomContactHistoryReady(w) {
		return
	}
	id, ok := campaignHistoryID(chi.URLParam(r, "history_id"))
	if !ok || r.URL.RawQuery != "" {
		wecomContactHistoryInvalid(w)
		return
	}
	row, err := h.wecomContactHistory.GetHistoricalWeComExternalContactFollowUser(r.Context(), id)
	_, digestErr := contactapp.HistoricalWeComExternalContactFollowUserDigest(row)
	if err != nil || digestErr != nil || row.ID != id {
		wecomContactHistoryUnavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "item": row})
}

func (h *Handler) wecomContactHistoryReady(w http.ResponseWriter) bool {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || nilLegacyDependency(h.wecomContactHistory) {
		wecomContactHistoryUnavailable(w)
		return false
	}
	return true
}
func writeWeComContactHistoryPage[T any](w http.ResponseWriter, rows []T, total int64, limit, offset int32, err error, valid func(T) bool) {
	if err != nil || total < 0 || int64(len(rows)) != min(int64(limit), max(int64(0), total-int64(offset))) {
		wecomContactHistoryUnavailable(w)
		return
	}
	for _, row := range rows {
		if !valid(row) {
			wecomContactHistoryUnavailable(w)
			return
		}
	}
	if rows == nil {
		rows = []T{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "items": rows, "total": total, "limit": limit, "offset": offset})
}
func wecomContactHistoryInvalid(w http.ResponseWriter) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_wecom_contact_history_query"})
}
func wecomContactHistoryUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "wecom_contact_history_unavailable"})
}
