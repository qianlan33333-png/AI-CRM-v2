package main

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
)

const (
	adminGroupOpsHistoryPlansPath     = "/api/admin/automation-conversion/group-ops/history/plans"
	adminGroupOpsHistoryDirectoryPath = "/api/admin/automation-conversion/group-ops/history/directory"
	adminGroupOpsHistoryGroupsPath    = adminGroupOpsHistoryPlansPath + "/{plan_id}/groups"
	adminGroupOpsHistoryNodesPath     = adminGroupOpsHistoryPlansPath + "/{plan_id}/nodes"
)

// Production composition must apply Authenticate and admin.read authorization.
// This reader has no current-plan, directory-sync or execution write dependency.
type adminGroupOpsHistory struct{ reader groupopsport.HistoricalReader }

func newAdminGroupOpsHistory(reader groupopsport.HistoricalReader) *adminGroupOpsHistory {
	return &adminGroupOpsHistory{reader: reader}
}
func (h *adminGroupOpsHistory) ListPlans(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, "plans")
}
func (h *adminGroupOpsHistory) ListDirectory(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, "directory")
}
func (h *adminGroupOpsHistory) ListGroups(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, "groups")
}
func (h *adminGroupOpsHistory) ListNodes(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, "nodes")
}

func (h *adminGroupOpsHistory) list(w http.ResponseWriter, r *http.Request, kind string) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.reader) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "group_ops_history_unavailable"})
		return
	}
	limit, offset, ok := adminGroupOpsHistoryPage(r.URL.RawQuery)
	planID := int64(0)
	if kind == "groups" || kind == "nodes" {
		raw := chi.URLParam(r, "plan_id")
		var err error
		planID, err = strconv.ParseInt(raw, 10, 64)
		ok = ok && err == nil && planID > 0 && raw == strconv.FormatInt(planID, 10)
	}
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_group_ops_history_query"})
		return
	}
	var items any
	var total int64
	var count int
	var err error
	switch kind {
	case "plans":
		var rows []groupopsport.HistoricalPlan
		rows, total, err = h.reader.ListHistoricalPlans(r.Context(), limit, offset)
		if rows == nil {
			rows = []groupopsport.HistoricalPlan{}
		}
		items, count = rows, len(rows)
	case "directory":
		var rows []groupopsport.HistoricalDirectory
		rows, total, err = h.reader.ListHistoricalDirectory(r.Context(), limit, offset)
		if rows == nil {
			rows = []groupopsport.HistoricalDirectory{}
		}
		items, count = rows, len(rows)
	case "groups":
		var rows []groupopsport.HistoricalGroup
		rows, total, err = h.reader.ListHistoricalGroups(r.Context(), planID, limit, offset)
		if rows == nil {
			rows = []groupopsport.HistoricalGroup{}
		}
		items, count = rows, len(rows)
	case "nodes":
		var rows []groupopsport.HistoricalNode
		rows, total, err = h.reader.ListHistoricalNodes(r.Context(), planID, limit, offset)
		if rows == nil {
			rows = []groupopsport.HistoricalNode{}
		}
		items, count = rows, len(rows)
	}
	if err != nil || total < 0 || int64(count) != min(int64(limit), max(0, total-int64(offset))) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "group_ops_history_unavailable"})
		return
	}
	response := map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "items": items, "total": total, "limit": limit, "offset": offset}
	if kind == "groups" || kind == "nodes" {
		response["plan_id"] = strconv.FormatInt(planID, 10)
	}
	writeJSON(w, http.StatusOK, response)
}

func adminGroupOpsHistoryPage(raw string) (int32, int32, bool) {
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
