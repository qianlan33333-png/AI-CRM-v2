package main

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	couponport "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/port"
)

func (h *Handler) ListCouponHistoryDefinitions(w http.ResponseWriter, r *http.Request) {
	h.listCouponHistory(w, r, "definitions")
}
func (h *Handler) ListCouponHistoryClaims(w http.ResponseWriter, r *http.Request) {
	h.listCouponHistory(w, r, "claims")
}
func (h *Handler) ListCouponHistoryRedemptions(w http.ResponseWriter, r *http.Request) {
	h.listCouponHistory(w, r, "redemptions")
}

func (h *Handler) listCouponHistory(w http.ResponseWriter, r *http.Request, kind string) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.couponHistory) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "coupon_history_unavailable"})
		return
	}
	limit, offset, ok := couponHistoryPage(r.URL.RawQuery)
	id := int64(0)
	if kind != "definitions" {
		var err error
		id, err = strconv.ParseInt(chi.URLParam(r, "coupon_id"), 10, 64)
		ok = ok && err == nil && id > 0
	}
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_coupon_history_query"})
		return
	}
	var items any
	var total int64
	var err error
	switch kind {
	case "definitions":
		var rows []couponport.HistoricalDefinition
		rows, total, err = h.couponHistory.ListHistoricalDefinitions(r.Context(), limit, offset)
		if rows == nil {
			rows = []couponport.HistoricalDefinition{}
		}
		items = rows
	case "claims":
		var rows []couponport.HistoricalClaim
		rows, total, err = h.couponHistory.ListHistoricalClaims(r.Context(), id, limit, offset)
		if rows == nil {
			rows = []couponport.HistoricalClaim{}
		}
		items = rows
	case "redemptions":
		var rows []couponport.HistoricalRedemption
		rows, total, err = h.couponHistory.ListHistoricalRedemptions(r.Context(), id, limit, offset)
		if rows == nil {
			rows = []couponport.HistoricalRedemption{}
		}
		items = rows
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "coupon_history_unavailable"})
		return
	}
	response := map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "items": items, "total": total, "limit": limit, "offset": offset}
	if kind != "definitions" {
		response["coupon_id"] = id
	}
	writeJSON(w, http.StatusOK, response)
}

func couponHistoryPage(raw string) (int32, int32, bool) {
	query, err := url.ParseQuery(raw)
	if err != nil {
		return 0, 0, false
	}
	limit, offset := int64(50), int64(0)
	for key, values := range query {
		if len(values) != 1 || key != "limit" && key != "offset" {
			return 0, 0, false
		}
		value, err := strconv.ParseInt(values[0], 10, 32)
		if err != nil {
			return 0, 0, false
		}
		if key == "limit" {
			limit = value
		} else {
			offset = value
		}
	}
	return int32(limit), int32(offset), limit >= 1 && limit <= 100 && offset >= 0
}
