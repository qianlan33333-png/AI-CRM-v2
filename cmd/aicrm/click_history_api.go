package main

import (
	"github.com/go-chi/chi/v5"
	app "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/app"
	port "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
	"net/http"
)

func (h *Handler) ListRadarClickHistory(w http.ResponseWriter, r *http.Request) {
	h.serveRadarClickHistory(w, r, "radar_click", false)
}
func (h *Handler) GetRadarClickHistory(w http.ResponseWriter, r *http.Request) {
	h.serveRadarClickHistory(w, r, "radar_click", true)
}
func (h *Handler) serveRadarClickHistory(w http.ResponseWriter, r *http.Request, kind string, detail bool) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.radarClickHistory) {
		writeJSON(w, 503, map[string]string{"code": "click_history_unavailable"})
		return
	}
	limit, offset, valid := audienceHistoryPage(r.URL.RawQuery)
	id := int64(0)
	if detail {
		id, valid = audienceHistoryID(chi.URLParam(r, "history_id"))
		valid = valid && r.URL.RawQuery == ""
	}
	if !valid {
		writeJSON(w, 400, map[string]string{"code": "invalid_click_history_query"})
		return
	}
	query := port.RadarClickHistoryQuery{Limit: limit, Offset: offset}
	var result any
	var total int64
	var count int
	var err error
	switch kind {
	case "radar_click":
		if detail {
			var item port.HistoricalRadarClick
			item, err = h.radarClickHistory.GetHistoricalRadarClick(r.Context(), id)
			if err == nil {
				_, err = app.HistoricalRadarClickDigest(item)
			}
			if err == nil && item.ID != id {
				err = port.ErrRadarClickHistoryUnavailable
			}
			result = item
		} else {
			var items []port.HistoricalRadarClick
			items, total, err = h.radarClickHistory.ListHistoricalRadarClick(r.Context(), query)
			if items == nil {
				items = []port.HistoricalRadarClick{}
			}
			for _, item := range items {
				if err != nil {
					break
				}
				_, err = app.HistoricalRadarClickDigest(item)
			}
			result, count = items, len(items)
		}
	default:
		err = port.ErrRadarClickHistoryUnavailable
	}
	if err != nil || (!detail && (total < 0 || count > int(limit) || int64(count) > total)) {
		writeJSON(w, 503, map[string]string{"code": "click_history_unavailable"})
		return
	}
	response := map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false}
	if detail {
		response["item"] = result
	} else {
		response["items"] = result
		response["total"] = total
		response["limit"] = limit
		response["offset"] = offset
	}
	writeJSON(w, 200, response)
}
