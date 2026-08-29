package main

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

// Only immutable Segment-owned history is exposed. Registration requires
// human authentication and admin.read; no mutation or Provider dependency.
func (h *Handler) ListAudienceHistoryGroups(w http.ResponseWriter, r *http.Request) {
	h.listAudienceHistory(w, r, "Groups", "")
}
func (h *Handler) ListAudienceHistoryPackages(w http.ResponseWriter, r *http.Request) {
	h.listAudienceHistory(w, r, "Packages", "")
}
func (h *Handler) ListAudienceHistoryVersions(w http.ResponseWriter, r *http.Request) {
	h.listAudienceHistory(w, r, "Versions", "package_id")
}
func (h *Handler) ListAudienceHistorySenders(w http.ResponseWriter, r *http.Request) {
	h.listAudienceHistory(w, r, "Senders", "package_id")
}
func (h *Handler) ListAudienceHistoryRules(w http.ResponseWriter, r *http.Request) {
	h.listAudienceHistory(w, r, "Rules", "")
}
func (h *Handler) ListAudienceHistoryRuleVersions(w http.ResponseWriter, r *http.Request) {
	h.listAudienceHistory(w, r, "RuleVersions", "rule_id")
}
func (h *Handler) ListAudienceHistoryDefinitions(w http.ResponseWriter, r *http.Request) {
	h.listAudienceHistory(w, r, "Definitions", "")
}
func (h *Handler) ListAudienceHistoryMembers(w http.ResponseWriter, r *http.Request) {
	h.listAudienceHistory(w, r, "Members", "package_id")
}

// ListAudienceActivityHistoryRuns exposes only immutable V1 activity facts;
// it is deliberately distinct from current Audience execution state.
func (h *Handler) ListAudienceActivityHistoryRuns(w http.ResponseWriter, r *http.Request) {
	h.listAudienceActivityHistory(w, r, false)
}

// ListAudienceActivityHistoryMemberEvents exposes read-only historical event
// observations, never current members or an executable queue.
func (h *Handler) ListAudienceActivityHistoryMemberEvents(w http.ResponseWriter, r *http.Request) {
	h.listAudienceActivityHistory(w, r, true)
}

func (h *Handler) listAudienceActivityHistory(w http.ResponseWriter, r *http.Request, events bool) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.audienceActivityHistory) {
		audienceHistoryUnavailable(w)
		return
	}
	limit, offset, ok := audienceHistoryPage(r.URL.RawQuery)
	if !ok {
		audienceHistoryInvalid(w)
		return
	}
	var items any
	var total int64
	var count int
	var err error
	if events {
		rows, found, readErr := h.audienceActivityHistory.ListAudienceActivityMemberEvents(r.Context(), 0, limit, offset)
		if rows == nil {
			rows = []segmentport.AudienceActivityMemberEventView{}
		}
		items, total, count, err = rows, found, len(rows), readErr
	} else {
		rows, found, readErr := h.audienceActivityHistory.ListAudienceActivityRuns(r.Context(), 0, limit, offset)
		if rows == nil {
			rows = []segmentport.AudienceActivityRunView{}
		}
		items, total, count, err = rows, found, len(rows), readErr
	}
	if err != nil || total < 0 || int64(count) != min(int64(limit), max(0, total-int64(offset))) {
		audienceHistoryUnavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "items": items, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) listAudienceHistory(w http.ResponseWriter, r *http.Request, kind, parent string) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.audienceHistory) {
		audienceHistoryUnavailable(w)
		return
	}
	limit, offset, ok := audienceHistoryPage(r.URL.RawQuery)
	id := int64(0)
	if parent != "" {
		var valid bool
		id, valid = audienceHistoryID(chi.URLParam(r, parent))
		ok = ok && valid
	}
	if !ok {
		audienceHistoryInvalid(w)
		return
	}
	var items any
	var total int64
	var count int
	var err error
	switch kind {
	case "Groups":
		var rows []segmentport.HistoricalAudienceGroup
		rows, total, err = h.audienceHistory.ListHistoricalAudienceGroups(r.Context(), limit, offset)
		if rows == nil {
			rows = []segmentport.HistoricalAudienceGroup{}
		}
		items, count = rows, len(rows)
	case "Packages":
		var rows []segmentport.HistoricalAudiencePackage
		rows, total, err = h.audienceHistory.ListHistoricalAudiencePackages(r.Context(), limit, offset)
		if rows == nil {
			rows = []segmentport.HistoricalAudiencePackage{}
		}
		items, count = rows, len(rows)
	case "Versions":
		var rows []segmentport.HistoricalAudienceVersion
		rows, total, err = h.audienceHistory.ListHistoricalAudienceVersions(r.Context(), id, limit, offset)
		if rows == nil {
			rows = []segmentport.HistoricalAudienceVersion{}
		}
		items, count = rows, len(rows)
	case "Senders":
		var rows []segmentport.HistoricalAudienceSender
		rows, total, err = h.audienceHistory.ListHistoricalAudienceSenders(r.Context(), id, limit, offset)
		if rows == nil {
			rows = []segmentport.HistoricalAudienceSender{}
		}
		items, count = rows, len(rows)
	case "Rules":
		var rows []segmentport.HistoricalAudienceRule
		rows, total, err = h.audienceHistory.ListHistoricalAudienceRules(r.Context(), limit, offset)
		if rows == nil {
			rows = []segmentport.HistoricalAudienceRule{}
		}
		items, count = rows, len(rows)
	case "RuleVersions":
		var rows []segmentport.HistoricalAudienceRuleVersion
		rows, total, err = h.audienceHistory.ListHistoricalAudienceRuleVersions(r.Context(), id, limit, offset)
		if rows == nil {
			rows = []segmentport.HistoricalAudienceRuleVersion{}
		}
		items, count = rows, len(rows)
	case "Definitions":
		var rows []segmentport.HistoricalAudienceDefinition
		rows, total, err = h.audienceHistory.ListHistoricalAudienceDefinitions(r.Context(), limit, offset)
		if rows == nil {
			rows = []segmentport.HistoricalAudienceDefinition{}
		}
		items, count = rows, len(rows)
	case "Members":
		var rows []segmentport.HistoricalAudienceMember
		rows, total, err = h.audienceHistory.ListHistoricalAudienceMembers(r.Context(), id, limit, offset)
		if rows == nil {
			rows = []segmentport.HistoricalAudienceMember{}
		}
		items, count = rows, len(rows)
	default:
		audienceHistoryInvalid(w)
		return
	}
	if err != nil || total < 0 || int64(count) != min(int64(limit), max(0, total-int64(offset))) {
		audienceHistoryUnavailable(w)
		return
	}
	response := map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "items": items, "total": total, "limit": limit, "offset": offset}
	if parent != "" {
		response[parent] = id
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) GetAudienceHistoryPackage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.audienceHistory) {
		audienceHistoryUnavailable(w)
		return
	}
	id, ok := audienceHistoryID(chi.URLParam(r, "package_id"))
	if !ok || r.URL.RawQuery != "" {
		audienceHistoryInvalid(w)
		return
	}
	item, err := h.audienceHistory.GetHistoricalAudiencePackage(r.Context(), id)
	if err != nil || item.ID != id {
		audienceHistoryUnavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "item": item})
}

func (h *Handler) GetAudienceHistoryDefinition(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.audienceHistory) {
		audienceHistoryUnavailable(w)
		return
	}
	id, ok := audienceHistoryID(chi.URLParam(r, "definition_id"))
	if !ok || r.URL.RawQuery != "" {
		audienceHistoryInvalid(w)
		return
	}
	item, err := h.audienceHistory.GetHistoricalAudienceDefinition(r.Context(), id)
	if err != nil || item.ID != id {
		audienceHistoryUnavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "item": item})
}

func audienceHistoryInvalid(w http.ResponseWriter) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_audience_history_query"})
}
func audienceHistoryUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "audience_history_unavailable"})
}
func audienceHistoryID(raw string) (int64, bool) {
	id, err := strconv.ParseInt(raw, 10, 64)
	return id, err == nil && id > 0 && strconv.FormatInt(id, 10) == raw
}
func audienceHistoryPage(raw string) (int32, int32, bool) {
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
