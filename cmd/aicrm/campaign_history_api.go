package main

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	campaignapp "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/app"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
)

// These handlers expose immutable V1 facts; they never call the current dispatcher.

func (h *Handler) ListCampaignHistorySegments(w http.ResponseWriter, r *http.Request) {
	if !h.campaignHistoryReady(w) {
		return
	}
	limit, offset, filters, ok := campaignHistoryQuery(r.URL.RawQuery, "campaign_source_id")
	if !ok {
		campaignHistoryInvalid(w)
		return
	}
	rows, total, err := h.campaignHistory.ListHistoricalCampaignSegments(r.Context(), filters["campaign_source_id"], limit, offset)
	writeCampaignHistoryPage(w, rows, total, limit, offset, err, "", 0, func(row campaignport.HistoricalCampaignSegment) bool {
		_, digestErr := campaignapp.HistoricalCampaignSegmentDigest(row)
		return digestErr == nil && (filters["campaign_source_id"] == nil || row.CampaignSourceID == *filters["campaign_source_id"])
	})
}

func (h *Handler) ListCampaignHistoryMembers(w http.ResponseWriter, r *http.Request) {
	if !h.campaignHistoryReady(w) {
		return
	}
	limit, offset, filters, ok := campaignHistoryQuery(r.URL.RawQuery, "segment_history_id customer_id")
	if !ok {
		campaignHistoryInvalid(w)
		return
	}
	rows, total, err := h.campaignHistory.ListHistoricalCampaignMembers(r.Context(), filters["segment_history_id"], filters["customer_id"], limit, offset)
	writeCampaignHistoryPage(w, rows, total, limit, offset, err, "", 0, func(row campaignport.HistoricalCampaignMember) bool {
		_, digestErr := campaignapp.HistoricalCampaignMemberDigest(row)
		return digestErr == nil && (filters["segment_history_id"] == nil || row.SegmentHistoryID == *filters["segment_history_id"]) && (filters["customer_id"] == nil || row.CustomerID != nil && *row.CustomerID == *filters["customer_id"])
	})
}

func (h *Handler) ListCampaignHistoryBroadcastPlans(w http.ResponseWriter, r *http.Request) {
	if !h.campaignHistoryReady(w) {
		return
	}
	limit, offset, filters, ok := campaignHistoryQuery(r.URL.RawQuery, "")
	_ = filters
	if !ok {
		campaignHistoryInvalid(w)
		return
	}
	rows, total, err := h.campaignHistory.ListHistoricalBroadcastPlans(r.Context(), limit, offset)
	writeCampaignHistoryPage(w, rows, total, limit, offset, err, "", 0, func(row campaignport.HistoricalBroadcastPlan) bool {
		_, digestErr := campaignapp.HistoricalBroadcastPlanDigest(row)
		return digestErr == nil && true
	})
}

func (h *Handler) ListCampaignHistoryBroadcastRecipients(w http.ResponseWriter, r *http.Request) {
	if !h.campaignHistoryReady(w) {
		return
	}
	limit, offset, filters, ok := campaignHistoryQuery(r.URL.RawQuery, "")
	_ = filters
	id, valid := campaignHistoryID(chi.URLParam(r, "plan_history_id"))
	ok = ok && valid
	if !ok {
		campaignHistoryInvalid(w)
		return
	}
	rows, total, err := h.campaignHistory.ListHistoricalBroadcastRecipients(r.Context(), id, limit, offset)
	writeCampaignHistoryPage(w, rows, total, limit, offset, err, "plan_history_id", id, func(row campaignport.HistoricalBroadcastRecipient) bool {
		_, digestErr := campaignapp.HistoricalBroadcastRecipientDigest(row)
		return digestErr == nil && row.PlanHistoryID == id
	})
}

func (h *Handler) ListCampaignHistoryBroadcastMessages(w http.ResponseWriter, r *http.Request) {
	if !h.campaignHistoryReady(w) {
		return
	}
	limit, offset, filters, ok := campaignHistoryQuery(r.URL.RawQuery, "")
	_ = filters
	id, valid := campaignHistoryID(chi.URLParam(r, "recipient_history_id"))
	ok = ok && valid
	if !ok {
		campaignHistoryInvalid(w)
		return
	}
	rows, total, err := h.campaignHistory.ListHistoricalBroadcastMessages(r.Context(), id, limit, offset)
	writeCampaignHistoryPage(w, rows, total, limit, offset, err, "recipient_history_id", id, func(row campaignport.HistoricalBroadcastMessage) bool {
		_, digestErr := campaignapp.HistoricalBroadcastMessageDigest(row)
		return digestErr == nil && row.RecipientHistoryID == id
	})
}

func (h *Handler) GetCampaignHistorySegment(w http.ResponseWriter, r *http.Request) {
	if !h.campaignHistoryReady(w) {
		return
	}
	id, ok := campaignHistoryID(chi.URLParam(r, "segment_history_id"))
	if !ok || r.URL.RawQuery != "" {
		campaignHistoryInvalid(w)
		return
	}
	row, err := h.campaignHistory.GetHistoricalCampaignSegment(r.Context(), id)
	_, digestErr := campaignapp.HistoricalCampaignSegmentDigest(row)
	if err != nil || digestErr != nil || row.ID != id {
		campaignHistoryUnavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "item": row})
}

func (h *Handler) GetCampaignHistoryBroadcastPlan(w http.ResponseWriter, r *http.Request) {
	if !h.campaignHistoryReady(w) {
		return
	}
	id, ok := campaignHistoryID(chi.URLParam(r, "plan_history_id"))
	if !ok || r.URL.RawQuery != "" {
		campaignHistoryInvalid(w)
		return
	}
	row, err := h.campaignHistory.GetHistoricalBroadcastPlan(r.Context(), id)
	_, digestErr := campaignapp.HistoricalBroadcastPlanDigest(row)
	if err != nil || digestErr != nil || row.ID != id {
		campaignHistoryUnavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "item": row})
}

func (h *Handler) campaignHistoryReady(w http.ResponseWriter) bool {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || nilLegacyDependency(h.campaignHistory) {
		campaignHistoryUnavailable(w)
		return false
	}
	return true
}

func writeCampaignHistoryPage[T any](w http.ResponseWriter, rows []T, total int64, limit, offset int32, err error, parent string, id int64, valid func(T) bool) {
	if err != nil || total < 0 || int64(len(rows)) != min(int64(limit), max(int64(0), total-int64(offset))) {
		campaignHistoryUnavailable(w)
		return
	}
	for _, row := range rows {
		if !valid(row) {
			campaignHistoryUnavailable(w)
			return
		}
	}
	if rows == nil {
		rows = []T{}
	}
	response := map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "items": rows, "total": total, "limit": limit, "offset": offset}
	if parent != "" {
		response[parent] = id
	}
	writeJSON(w, http.StatusOK, response)
}

func campaignHistoryQuery(raw, allowed string) (int32, int32, map[string]*int64, bool) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return 0, 0, nil, false
	}
	limit, offset := int32(50), int32(0)
	filters := map[string]*int64{}
	for name, values := range values {
		if len(values) != 1 {
			return 0, 0, nil, false
		}
		if name == "limit" || name == "offset" {
			value, parseErr := strconv.ParseInt(values[0], 10, 32)
			if parseErr != nil || strconv.FormatInt(value, 10) != values[0] || value < 0 {
				return 0, 0, nil, false
			}
			if name == "limit" {
				if value < 1 || value > 100 {
					return 0, 0, nil, false
				}
				limit = int32(value)
			} else {
				offset = int32(value)
			}
			continue
		}
		found := false
		for _, filter := range strings.Fields(allowed) {
			found = found || name == filter
		}
		if !found {
			return 0, 0, nil, false
		}
		id, ok := campaignHistoryID(values[0])
		if !ok {
			return 0, 0, nil, false
		}
		filters[name] = &id
	}
	return limit, offset, filters, true
}

func campaignHistoryID(raw string) (int64, bool) {
	id, err := strconv.ParseInt(raw, 10, 64)
	return id, err == nil && id > 0 && strconv.FormatInt(id, 10) == raw
}
func campaignHistoryInvalid(w http.ResponseWriter) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_campaign_history_query"})
}
func campaignHistoryUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "campaign_history_unavailable"})
}
