package main

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

func (h *Handler) ListMemberViewHistory(w http.ResponseWriter, r *http.Request) {
	h.readMemberGridHistory(w, r, true, false)
}
func (h *Handler) GetMemberViewHistory(w http.ResponseWriter, r *http.Request) {
	h.readMemberGridHistory(w, r, true, true)
}
func (h *Handler) ListMemberUsageHistory(w http.ResponseWriter, r *http.Request) {
	h.readMemberGridHistory(w, r, false, false)
}
func (h *Handler) GetMemberUsageHistory(w http.ResponseWriter, r *http.Request) {
	h.readMemberGridHistory(w, r, false, true)
}

func (h *Handler) readMemberGridHistory(w http.ResponseWriter, r *http.Request, view, detail bool) {
	w.Header().Set("Cache-Control", "no-store")
	unavailable := func() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "member_grid_history_unavailable"})
	}
	invalid := func() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_member_grid_history_query"})
	}
	if h == nil || r == nil || nilLegacyDependency(h.memberGridHistory) {
		unavailable()
		return
	}
	query, ok := memberGridHistoryQuery(r.URL.RawQuery, view)
	var id int64
	if detail {
		id, ok = memberGridHistoryPositiveID(chi.URLParam(r, "history_id"))
		ok = ok && r.URL.RawQuery == ""
	}
	if !ok {
		invalid()
		return
	}
	response := map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false}
	if detail {
		if view {
			item, err := h.memberGridHistory.GetHistoricalMemberView(r.Context(), id)
			if err != nil || item.ID != id || !validMemberViewHistory(item) {
				unavailable()
				return
			}
			response["item"] = item
		} else {
			item, err := h.memberGridHistory.GetHistoricalMemberUsage(r.Context(), id)
			if err != nil || item.ID != id || !validMemberUsageHistory(item) {
				unavailable()
				return
			}
			response["item"] = item
		}
	} else {
		var rows any
		var total int64
		if view {
			items, count, err := h.memberGridHistory.ListHistoricalMemberViews(r.Context(), query)
			if err != nil || !validMemberGridHistoryPageSize(len(items), count, query) {
				unavailable()
				return
			}
			ids := map[int64]bool{}
			for _, item := range items {
				if !validMemberViewHistory(item) || ids[item.ID] || query.ProductID != nil && (item.ProductID == nil || *item.ProductID != *query.ProductID) {
					unavailable()
					return
				}
				ids[item.ID] = true
			}
			if items == nil {
				items = []productport.HistoricalMemberView{}
			}
			rows, total = items, count
		} else {
			items, count, err := h.memberGridHistory.ListHistoricalMemberUsage(r.Context(), query)
			if err != nil || !validMemberGridHistoryPageSize(len(items), count, query) {
				unavailable()
				return
			}
			ids := map[int64]bool{}
			for _, item := range items {
				if !validMemberUsageHistory(item) || ids[item.ID] || query.CustomerID != nil && (item.CustomerID == nil || *item.CustomerID != *query.CustomerID) {
					unavailable()
					return
				}
				ids[item.ID] = true
			}
			if items == nil {
				items = []productport.HistoricalMemberUsage{}
			}
			rows, total = items, count
		}
		response["items"], response["total"], response["limit"], response["offset"] = rows, total, query.Limit, query.Offset
	}
	writeJSON(w, http.StatusOK, response)
}

func memberGridHistoryQuery(raw string, view bool) (productport.MemberGridHistoryQuery, bool) {
	query := productport.MemberGridHistoryQuery{Limit: 50}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return query, false
	}
	for key, values := range values {
		if len(values) != 1 {
			return query, false
		}
		if key == "product_id" && view || key == "customer_id" && !view {
			id, ok := memberGridHistoryPositiveID(values[0])
			if !ok {
				return query, false
			}
			if view {
				query.ProductID = &id
			} else {
				query.CustomerID = &id
			}
			continue
		}
		value, err := strconv.ParseInt(values[0], 10, 32)
		if err != nil || strconv.FormatInt(value, 10) != values[0] {
			return query, false
		}
		switch key {
		case "limit":
			if value < 1 || value > 100 {
				return query, false
			}
			query.Limit = int32(value)
		case "offset":
			if value < 0 {
				return query, false
			}
			query.Offset = int32(value)
		default:
			return query, false
		}
	}
	return query, true
}

func memberGridHistoryPositiveID(raw string) (int64, bool) {
	id, err := strconv.ParseInt(raw, 10, 64)
	return id, err == nil && id > 0 && strconv.FormatInt(id, 10) == raw
}

func validMemberGridHistoryPageSize(size int, total int64, query productport.MemberGridHistoryQuery) bool {
	if total < 0 {
		return false
	}
	expected := total - int64(query.Offset)
	if expected < 0 {
		expected = 0
	}
	if expected > int64(query.Limit) {
		expected = int64(query.Limit)
	}
	return int64(size) == expected
}

func validMemberViewHistory(value productport.HistoricalMemberView) bool {
	_, err := productapp.HistoricalMemberViewDigest(value)
	return value.ID > 0 && err == nil
}

func validMemberUsageHistory(value productport.HistoricalMemberUsage) bool {
	_, err := productapp.HistoricalMemberUsageDigest(value)
	return value.ID > 0 && err == nil
}
