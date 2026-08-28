package main

import (
	"context"
	"net/http"

	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
)

func (h *Handler) ListHXCHistorySenderConfig(w http.ResponseWriter, r *http.Request) {
	h.serveHXCRuntimeHistory(w, r, false, false)
}
func (h *Handler) GetHXCHistorySenderConfig(w http.ResponseWriter, r *http.Request) {
	h.serveHXCRuntimeHistory(w, r, false, true)
}
func (h *Handler) ListHXCHistorySendRecord(w http.ResponseWriter, r *http.Request) {
	h.serveHXCRuntimeHistory(w, r, true, false)
}
func (h *Handler) GetHXCHistorySendRecord(w http.ResponseWriter, r *http.Request) {
	h.serveHXCRuntimeHistory(w, r, true, true)
}

func (h *Handler) serveHXCRuntimeHistory(w http.ResponseWriter, r *http.Request, records, detail bool) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.hxcRuntimeHistory) {
		hxcHistoryUnavailable(w)
		return
	}
	query, id, valid := parseHXCHistoryQuery(r, "runtime", detail)
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_hxc_history_query"})
		return
	}
	if records {
		serveHXCRuntimeValue(w, r, query, id, detail,
			h.hxcRuntimeHistory.GetHistoricalHXCSendRecord,
			h.hxcRuntimeHistory.ListHistoricalHXCSendRecord,
			hxcapp.HistoricalHXCSendRecordDigest,
			func(v hxcport.HistoricalHXCSendRecord) int64 { return v.ID })
	} else {
		serveHXCRuntimeValue(w, r, query, id, detail,
			h.hxcRuntimeHistory.GetHistoricalHXCSenderConfig,
			h.hxcRuntimeHistory.ListHistoricalHXCSenderConfig,
			hxcapp.HistoricalHXCSenderConfigDigest,
			func(v hxcport.HistoricalHXCSenderConfig) int64 { return v.ID })
	}
}

func serveHXCRuntimeValue[T any](w http.ResponseWriter, r *http.Request, query hxcport.HXCHistoryQuery, id int64, detail bool,
	get func(context.Context, int64) (T, error),
	list func(context.Context, hxcport.HXCHistoryQuery) ([]T, int64, error),
	digest func(T) ([32]byte, error), getID func(T) int64,
) {
	response := map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false}
	if detail {
		item, err := get(r.Context(), id)
		if err != nil || getID(item) != id {
			hxcHistoryUnavailable(w)
			return
		}
		if _, err := digest(item); err != nil {
			hxcHistoryUnavailable(w)
			return
		}
		response["item"] = item
	} else {
		items, total, err := list(r.Context(), query)
		if err != nil || total < 0 || int64(len(items)) != min(int64(query.Limit), max(0, total-int64(query.Offset))) {
			hxcHistoryUnavailable(w)
			return
		}
		if items == nil {
			items = []T{}
		}
		var previous int64
		for _, item := range items {
			if _, err := digest(item); err != nil || getID(item) <= previous {
				hxcHistoryUnavailable(w)
				return
			}
			previous = getID(item)
		}
		response["items"], response["total"], response["limit"], response["offset"] = items, total, query.Limit, query.Offset
	}
	writeJSON(w, http.StatusOK, response)
}
