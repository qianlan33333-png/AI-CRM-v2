package main

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

func (h *Handler) ListSidebarProfileHistory(w http.ResponseWriter, r *http.Request) {
	h.readContactHistory(w, r, true, false)
}

func (h *Handler) GetSidebarProfileHistory(w http.ResponseWriter, r *http.Request) {
	h.readContactHistory(w, r, true, true)
}

func (h *Handler) ListOwnerMigrationResultHistory(w http.ResponseWriter, r *http.Request) {
	h.readContactHistory(w, r, false, false)
}

func (h *Handler) GetOwnerMigrationResultHistory(w http.ResponseWriter, r *http.Request) {
	h.readContactHistory(w, r, false, true)
}

// These GETs never invoke current profile or owner-reassignment commands.
func (h *Handler) readContactHistory(w http.ResponseWriter, r *http.Request, sidebar, detail bool) {
	w.Header().Set("Cache-Control", "no-store")
	unavailable := func() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "contact_history_unavailable"})
	}
	invalid := func() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_contact_history_query"})
	}
	if h == nil || r == nil || nilLegacyDependency(h.contactHistory) {
		unavailable()
		return
	}
	query, ok := contactHistoryQuery(r.URL.RawQuery, sidebar)
	var id int64
	if detail {
		id, ok = contactHistoryPositiveID(chi.URLParam(r, "history_id"))
		ok = ok && r.URL.RawQuery == ""
	}
	if !ok {
		invalid()
		return
	}
	response := map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false}
	if detail {
		if sidebar {
			item, err := h.contactHistory.GetHistoricalSidebarProfile(r.Context(), id)
			if err != nil || item.ID != id || !validSidebarProfileHistory(item) {
				unavailable()
				return
			}
			response["item"] = item
		} else {
			item, err := h.contactHistory.GetHistoricalOwnerMigrationResult(r.Context(), id)
			if err != nil || item.ID != id || !validOwnerMigrationResultHistory(item) {
				unavailable()
				return
			}
			response["item"] = item
		}
	} else {
		var rows any
		var total int64
		if sidebar {
			items, count, err := h.contactHistory.ListHistoricalSidebarProfiles(r.Context(), query)
			if err != nil || !validContactHistoryPageSize(len(items), count, query) {
				unavailable()
				return
			}
			for _, item := range items {
				if !validSidebarProfileHistory(item) || query.CustomerID != nil && (item.CustomerID == nil || *item.CustomerID != *query.CustomerID) {
					unavailable()
					return
				}
			}
			if items == nil {
				items = []contactport.HistoricalSidebarProfile{}
			}
			rows, total = items, count
		} else {
			items, count, err := h.contactHistory.ListHistoricalOwnerMigrationResults(r.Context(), query)
			if err != nil || !validContactHistoryPageSize(len(items), count, query) {
				unavailable()
				return
			}
			for _, item := range items {
				if !validOwnerMigrationResultHistory(item) {
					unavailable()
					return
				}
			}
			if items == nil {
				items = []contactport.HistoricalOwnerMigrationResult{}
			}
			rows, total = items, count
		}
		response["items"], response["total"], response["limit"], response["offset"] = rows, total, query.Limit, query.Offset
	}
	writeJSON(w, http.StatusOK, response)
}

func contactHistoryQuery(raw string, sidebar bool) (contactport.ContactHistoryQuery, bool) {
	query := contactport.ContactHistoryQuery{Limit: 50}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return query, false
	}
	for key, values := range values {
		if len(values) != 1 {
			return query, false
		}
		if key == "customer_id" && sidebar {
			id, ok := contactHistoryPositiveID(values[0])
			if !ok {
				return query, false
			}
			query.CustomerID = &id
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

func contactHistoryPositiveID(raw string) (int64, bool) {
	id, err := strconv.ParseInt(raw, 10, 64)
	return id, err == nil && id > 0 && strconv.FormatInt(id, 10) == raw
}

func validContactHistoryPageSize(size int, total int64, query contactport.ContactHistoryQuery) bool {
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

func validSidebarProfileHistory(value contactport.HistoricalSidebarProfile) bool {
	_, err := contactapp.HistoricalSidebarProfileDigest(value)
	return err == nil
}

func validOwnerMigrationResultHistory(value contactport.HistoricalOwnerMigrationResult) bool {
	_, err := contactapp.HistoricalOwnerMigrationResultDigest(value)
	return err == nil
}
