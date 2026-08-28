package main

import (
	"github.com/go-chi/chi/v5"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"net/http"
)

func (h *Handler) ListDeferredPeople(w http.ResponseWriter, r *http.Request) {
	if !h.deferredIdentityHistoryReady(w) {
		return
	}
	limit, offset, _, ok := campaignHistoryQuery(r.URL.RawQuery, "")
	if !ok {
		deferredIdentityHistoryInvalid(w)
		return
	}
	rows, total, err := h.deferredIdentityHistory.ListHistoricalDeferredPerson(r.Context(), contactport.DeferredIdentityHistoryQuery{Limit: limit, Offset: offset})
	writeDeferredIdentityHistoryPage(w, rows, total, limit, offset, err, deferredPersonPublic)
}
func (h *Handler) GetDeferredPerson(w http.ResponseWriter, r *http.Request) {
	if !h.deferredIdentityHistoryReady(w) {
		return
	}
	id, ok := campaignHistoryID(chi.URLParam(r, "history_id"))
	if !ok || r.URL.RawQuery != "" {
		deferredIdentityHistoryInvalid(w)
		return
	}
	row, err := h.deferredIdentityHistory.GetHistoricalDeferredPerson(r.Context(), id)
	value, valid := deferredPersonPublic(row)
	if err != nil || !valid || row.ID != id {
		deferredIdentityHistoryUnavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "item": value})
}
func deferredPersonPublic(row contactport.HistoricalDeferredPerson) (map[string]any, bool) {
	if _, err := contactapp.HistoricalDeferredPersonDigest(row); err != nil {
		return nil, false
	}
	return map[string]any{"id": row.ID, "source_id": row.SourceID, "created_at": row.CreatedAt, "updated_at": row.UpdatedAt}, true
}

func (h *Handler) ListDeferredIdentityConflicts(w http.ResponseWriter, r *http.Request) {
	if !h.deferredIdentityHistoryReady(w) {
		return
	}
	limit, offset, _, ok := campaignHistoryQuery(r.URL.RawQuery, "")
	if !ok {
		deferredIdentityHistoryInvalid(w)
		return
	}
	rows, total, err := h.deferredIdentityHistory.ListHistoricalDeferredIdentityConflict(r.Context(), contactport.DeferredIdentityHistoryQuery{Limit: limit, Offset: offset})
	writeDeferredIdentityHistoryPage(w, rows, total, limit, offset, err, deferredIdentityConflictPublic)
}
func (h *Handler) GetDeferredIdentityConflict(w http.ResponseWriter, r *http.Request) {
	if !h.deferredIdentityHistoryReady(w) {
		return
	}
	id, ok := campaignHistoryID(chi.URLParam(r, "history_id"))
	if !ok || r.URL.RawQuery != "" {
		deferredIdentityHistoryInvalid(w)
		return
	}
	row, err := h.deferredIdentityHistory.GetHistoricalDeferredIdentityConflict(r.Context(), id)
	value, valid := deferredIdentityConflictPublic(row)
	if err != nil || !valid || row.ID != id {
		deferredIdentityHistoryUnavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "item": value})
}
func deferredIdentityConflictPublic(row contactport.HistoricalDeferredIdentityConflict) (map[string]any, bool) {
	if _, err := contactapp.HistoricalDeferredIdentityConflictDigest(row); err != nil {
		return nil, false
	}
	return map[string]any{"id": row.ID, "source_id": row.SourceID, "conflict_type": row.ConflictType, "source_type": row.SourceType, "status": row.Status, "resolution_status": row.ResolutionStatus, "created_at": row.CreatedAt, "updated_at": row.UpdatedAt, "resolved_at": row.ResolvedAt}, true
}

func (h *Handler) ListMissingRootIdentities(w http.ResponseWriter, r *http.Request) {
	if !h.deferredIdentityHistoryReady(w) {
		return
	}
	limit, offset, _, ok := campaignHistoryQuery(r.URL.RawQuery, "")
	if !ok {
		deferredIdentityHistoryInvalid(w)
		return
	}
	rows, total, err := h.deferredIdentityHistory.ListHistoricalMissingRootIdentity(r.Context(), contactport.DeferredIdentityHistoryQuery{Limit: limit, Offset: offset})
	writeDeferredIdentityHistoryPage(w, rows, total, limit, offset, err, missingRootIdentityPublic)
}
func (h *Handler) GetMissingRootIdentity(w http.ResponseWriter, r *http.Request) {
	if !h.deferredIdentityHistoryReady(w) {
		return
	}
	id, ok := campaignHistoryID(chi.URLParam(r, "history_id"))
	if !ok || r.URL.RawQuery != "" {
		deferredIdentityHistoryInvalid(w)
		return
	}
	row, err := h.deferredIdentityHistory.GetHistoricalMissingRootIdentity(r.Context(), id)
	value, valid := missingRootIdentityPublic(row)
	if err != nil || !valid || row.ID != id {
		deferredIdentityHistoryUnavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "item": value})
}
func missingRootIdentityPublic(row contactport.HistoricalMissingRootIdentity) (map[string]any, bool) {
	if _, err := contactapp.HistoricalMissingRootIdentityDigest(row); err != nil {
		return nil, false
	}
	return map[string]any{"id": row.ID, "source_id": row.SourceID, "quarantine_reason": row.QuarantineReason, "type": row.Type, "status": row.Status, "first_seen_at": row.FirstSeenAt, "last_seen_at": row.LastSeenAt, "created_at": row.CreatedAt, "updated_at": row.UpdatedAt}, true
}

func (h *Handler) deferredIdentityHistoryReady(w http.ResponseWriter) bool {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || nilLegacyDependency(h.deferredIdentityHistory) {
		deferredIdentityHistoryUnavailable(w)
		return false
	}
	return true
}
func writeDeferredIdentityHistoryPage[T any](w http.ResponseWriter, rows []T, total int64, limit, offset int32, err error, public func(T) (map[string]any, bool)) {
	if err != nil || total < 0 || int64(len(rows)) != min(int64(limit), max(int64(0), total-int64(offset))) {
		deferredIdentityHistoryUnavailable(w)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		value, ok := public(row)
		if !ok {
			deferredIdentityHistoryUnavailable(w)
			return
		}
		items = append(items, value)
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "items": items, "total": total, "limit": limit, "offset": offset})
}
func deferredIdentityHistoryInvalid(w http.ResponseWriter) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_deferred_identity_history_query"})
}
func deferredIdentityHistoryUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "deferred_identity_history_unavailable"})
}
