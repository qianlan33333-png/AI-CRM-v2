package main

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
)

func (h *Handler) ListHXCHistoryMemberUsage(w http.ResponseWriter, r *http.Request) {
	h.serveHXCMemberUsageHistory(w, r, false)
}

func (h *Handler) GetHXCHistoryMemberUsage(w http.ResponseWriter, r *http.Request) {
	h.serveHXCMemberUsageHistory(w, r, true)
}

func (h *Handler) serveHXCMemberUsageHistory(w http.ResponseWriter, r *http.Request, detail bool) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.hxcMemberUsageHistory) {
		hxcHistoryUnavailable(w)
		return
	}
	query, id, generation, valid := parseHXCMemberUsageQuery(r, detail)
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_hxc_history_query"})
		return
	}
	serveHXCRuntimeValue(w, r, query, id, detail,
		h.hxcMemberUsageHistory.GetHistoricalHXCMemberUsage,
		func(ctx context.Context, q hxcport.HXCHistoryQuery) ([]hxcport.HistoricalHXCMemberUsage, int64, error) {
			items, total, err := h.hxcMemberUsageHistory.ListHistoricalHXCMemberUsage(ctx, hxcport.HXCMemberUsageHistoryQuery{Generation: generation, Limit: q.Limit, Offset: q.Offset})
			for _, item := range items {
				if generation != nil && item.Generation != *generation {
					return nil, 0, hxcport.ErrHXCHistoryConflict
				}
			}
			return items, total, err
		},
		hxcapp.HistoricalHXCMemberUsageDigest,
		func(v hxcport.HistoricalHXCMemberUsage) int64 { return v.ID })
}

func parseHXCMemberUsageQuery(r *http.Request, detail bool) (hxcport.HXCHistoryQuery, int64, *int64, bool) {
	if detail {
		query, id, valid := parseHXCHistoryQuery(r, "runtime", true)
		return query, id, nil, valid
	}
	var query hxcport.HXCHistoryQuery
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return query, 0, nil, false
	}
	var generation *int64
	if raw, found := values["generation"]; found {
		if len(raw) != 1 {
			return query, 0, nil, false
		}
		value, err := strconv.ParseInt(raw[0], 10, 64)
		if err != nil || strconv.FormatInt(value, 10) != raw[0] {
			return query, 0, nil, false
		}
		generation = &value
		values.Del("generation")
	}
	var valid bool
	query.Limit, query.Offset, valid = audienceHistoryPage(values.Encode())
	return query, 0, generation, valid
}
