package main

import (
	"github.com/go-chi/chi/v5"
	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	"net/http"
	"net/url"
)

// These reads expose historical observations, never live funnel refresh or tasks.
func (h *Handler) ListHXCHistoryMeta(w http.ResponseWriter, r *http.Request) {
	h.serveHXCHistory(w, r, "meta", false)
}
func (h *Handler) GetHXCHistoryMeta(w http.ResponseWriter, r *http.Request) {
	h.serveHXCHistory(w, r, "meta", true)
}
func (h *Handler) ListHXCHistorySnapshot(w http.ResponseWriter, r *http.Request) {
	h.serveHXCHistory(w, r, "snapshot", false)
}
func (h *Handler) GetHXCHistorySnapshot(w http.ResponseWriter, r *http.Request) {
	h.serveHXCHistory(w, r, "snapshot", true)
}
func (h *Handler) ListHXCHistoryActivation(w http.ResponseWriter, r *http.Request) {
	h.serveHXCHistory(w, r, "activation", false)
}
func (h *Handler) GetHXCHistoryActivation(w http.ResponseWriter, r *http.Request) {
	h.serveHXCHistory(w, r, "activation", true)
}
func (h *Handler) ListHXCHistoryLead(w http.ResponseWriter, r *http.Request) {
	h.serveHXCHistory(w, r, "lead", false)
}
func (h *Handler) GetHXCHistoryLead(w http.ResponseWriter, r *http.Request) {
	h.serveHXCHistory(w, r, "lead", true)
}
func (h *Handler) ListHXCHistoryBatch(w http.ResponseWriter, r *http.Request) {
	h.serveHXCHistory(w, r, "batch", false)
}
func (h *Handler) GetHXCHistoryBatch(w http.ResponseWriter, r *http.Request) {
	h.serveHXCHistory(w, r, "batch", true)
}
func (h *Handler) serveHXCHistory(w http.ResponseWriter, r *http.Request, kind string, detail bool) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.hxcHistory) {
		hxcHistoryUnavailable(w)
		return
	}
	query, id, valid := parseHXCHistoryQuery(r, kind, detail)
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_hxc_history_query"})
		return
	}
	var result any
	var total int64
	var count int
	var err error
	switch kind {
	case "meta":
		if detail {
			var item hxcport.HistoricalHXCMeta
			item, err = h.hxcHistory.GetHistoricalHXCMeta(r.Context(), id)
			if err == nil {
				_, err = hxcapp.HistoricalHXCMetaDigest(item)
			}
			if err == nil && item.ID != id {
				err = hxcport.ErrHXCHistoryUnavailable
			}
			result = item
		} else {
			var items []hxcport.HistoricalHXCMeta
			items, total, err = h.hxcHistory.ListHistoricalHXCMeta(r.Context(), query)
			if items == nil {
				items = []hxcport.HistoricalHXCMeta{}
			}
			for _, item := range items {
				if err != nil {
					break
				}
				_, err = hxcapp.HistoricalHXCMetaDigest(item)

			}
			result, count = items, len(items)
		}
	case "snapshot":
		if detail {
			var item hxcport.HistoricalHXCSnapshot
			item, err = h.hxcHistory.GetHistoricalHXCSnapshot(r.Context(), id)
			if err == nil {
				_, err = hxcapp.HistoricalHXCSnapshotDigest(item)
			}
			if err == nil && item.ID != id {
				err = hxcport.ErrHXCHistoryUnavailable
			}
			result = item
		} else {
			var items []hxcport.HistoricalHXCSnapshot
			items, total, err = h.hxcHistory.ListHistoricalHXCSnapshot(r.Context(), query)
			if items == nil {
				items = []hxcport.HistoricalHXCSnapshot{}
			}
			for _, item := range items {
				if err != nil {
					break
				}
				_, err = hxcapp.HistoricalHXCSnapshotDigest(item)
				if err == nil && query.CustomerID != nil && (item.CustomerID == nil || *item.CustomerID != *query.CustomerID) {
					err = hxcport.ErrHXCHistoryUnavailable
				}
			}
			result, count = items, len(items)
		}
	case "activation":
		if detail {
			var item hxcport.HistoricalHXCActivation
			item, err = h.hxcHistory.GetHistoricalHXCActivation(r.Context(), id)
			if err == nil {
				_, err = hxcapp.HistoricalHXCActivationDigest(item)
			}
			if err == nil && item.ID != id {
				err = hxcport.ErrHXCHistoryUnavailable
			}
			result = item
		} else {
			var items []hxcport.HistoricalHXCActivation
			items, total, err = h.hxcHistory.ListHistoricalHXCActivation(r.Context(), query)
			if items == nil {
				items = []hxcport.HistoricalHXCActivation{}
			}
			for _, item := range items {
				if err != nil {
					break
				}
				_, err = hxcapp.HistoricalHXCActivationDigest(item)
				if err == nil && query.SourceTable != "" && item.SourceTable != query.SourceTable {
					err = hxcport.ErrHXCHistoryUnavailable
				}
			}
			result, count = items, len(items)
		}
	case "lead":
		if detail {
			var item hxcport.HistoricalHXCLead
			item, err = h.hxcHistory.GetHistoricalHXCLead(r.Context(), id)
			if err == nil {
				_, err = hxcapp.HistoricalHXCLeadDigest(item)
			}
			if err == nil && item.ID != id {
				err = hxcport.ErrHXCHistoryUnavailable
			}
			result = item
		} else {
			var items []hxcport.HistoricalHXCLead
			items, total, err = h.hxcHistory.ListHistoricalHXCLead(r.Context(), query)
			if items == nil {
				items = []hxcport.HistoricalHXCLead{}
			}
			for _, item := range items {
				if err != nil {
					break
				}
				_, err = hxcapp.HistoricalHXCLeadDigest(item)

			}
			result, count = items, len(items)
		}
	case "batch":
		if detail {
			var item hxcport.HistoricalHXCBatch
			item, err = h.hxcHistory.GetHistoricalHXCBatch(r.Context(), id)
			if err == nil {
				_, err = hxcapp.HistoricalHXCBatchDigest(item)
			}
			if err == nil && item.ID != id {
				err = hxcport.ErrHXCHistoryUnavailable
			}
			result = item
		} else {
			var items []hxcport.HistoricalHXCBatch
			items, total, err = h.hxcHistory.ListHistoricalHXCBatch(r.Context(), query)
			if items == nil {
				items = []hxcport.HistoricalHXCBatch{}
			}
			for _, item := range items {
				if err != nil {
					break
				}
				_, err = hxcapp.HistoricalHXCBatchDigest(item)

			}
			result, count = items, len(items)
		}
	default:
		hxcHistoryUnavailable(w)
		return
	}
	if err != nil || !detail && (total < 0 || int64(count) != min(int64(query.Limit), max(0, total-int64(query.Offset)))) {
		hxcHistoryUnavailable(w)
		return
	}
	response := map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false}
	if detail {
		response["item"] = result
	} else {
		response["items"] = result
		response["total"] = total
		response["limit"] = query.Limit
		response["offset"] = query.Offset
	}
	writeJSON(w, http.StatusOK, response)
}
func parseHXCHistoryQuery(r *http.Request, kind string, detail bool) (hxcport.HXCHistoryQuery, int64, bool) {
	var query hxcport.HXCHistoryQuery
	if detail {
		id, ok := audienceHistoryID(chi.URLParam(r, "history_id"))
		return query, id, ok && r.URL.RawQuery == ""
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return query, 0, false
	}
	if raw, found := values["customer_id"]; found {
		if kind != "snapshot" || len(raw) != 1 {
			return query, 0, false
		}
		id, ok := audienceHistoryID(raw[0])
		if !ok {
			return query, 0, false
		}
		query.CustomerID = &id
		values.Del("customer_id")
	}
	if raw, found := values["source_table"]; found {
		if kind != "activation" || len(raw) != 1 || raw[0] != "public/user_ops_activation_status_source" && raw[0] != "public/user_ops_huangxiaocan_activation_source" {
			return query, 0, false
		}
		query.SourceTable = raw[0]
		values.Del("source_table")
	}
	var ok bool
	query.Limit, query.Offset, ok = audienceHistoryPage(values.Encode())
	return query, 0, ok
}
func hxcHistoryUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "hxc_history_unavailable"})
}
