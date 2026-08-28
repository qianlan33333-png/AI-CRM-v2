package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

func (h *Handler) ListContactReferenceHistoryBindings(w http.ResponseWriter, r *http.Request) {
	if !h.contactReferenceHistoryReady(w) {
		return
	}
	limit, offset, _, ok := campaignHistoryQuery(r.URL.RawQuery, "")
	if !ok {
		contactReferenceHistoryInvalid(w)
		return
	}
	rows, total, err := h.contactReferenceHistory.ListHistoricalExternalContactBinding(r.Context(), contactport.ReferenceHistoryQuery{Limit: limit, Offset: offset})
	writeContactReferenceHistoryPage(w, rows, total, limit, offset, err, func(row contactport.HistoricalExternalContactBinding) bool {
		_, err := contactapp.HistoricalExternalContactBindingDigest(row)
		return err == nil && row.ID > 0
	})
}

func (h *Handler) GetContactReferenceHistoryBinding(w http.ResponseWriter, r *http.Request) {
	if !h.contactReferenceHistoryReady(w) {
		return
	}
	id, ok := campaignHistoryID(chi.URLParam(r, "history_id"))
	if !ok || r.URL.RawQuery != "" {
		contactReferenceHistoryInvalid(w)
		return
	}
	row, err := h.contactReferenceHistory.GetHistoricalExternalContactBinding(r.Context(), id)
	_, digestErr := contactapp.HistoricalExternalContactBindingDigest(row)
	if err != nil || digestErr != nil || row.ID != id {
		contactReferenceHistoryUnavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "item": row})
}

func (h *Handler) ListContactReferenceHistoryDirectory(w http.ResponseWriter, r *http.Request) {
	if !h.contactReferenceHistoryReady(w) {
		return
	}
	limit, offset, _, ok := campaignHistoryQuery(r.URL.RawQuery, "")
	if !ok {
		contactReferenceHistoryInvalid(w)
		return
	}
	rows, total, err := h.contactReferenceHistory.ListHistoricalWeComDirectoryMember(r.Context(), contactport.ReferenceHistoryQuery{Limit: limit, Offset: offset})
	writeContactReferenceHistoryPage(w, rows, total, limit, offset, err, func(row contactport.HistoricalWeComDirectoryMember) bool {
		_, err := contactapp.HistoricalWeComDirectoryMemberDigest(row)
		return err == nil && row.ID > 0
	})
}

func (h *Handler) GetContactReferenceHistoryDirectory(w http.ResponseWriter, r *http.Request) {
	if !h.contactReferenceHistoryReady(w) {
		return
	}
	id, ok := campaignHistoryID(chi.URLParam(r, "history_id"))
	if !ok || r.URL.RawQuery != "" {
		contactReferenceHistoryInvalid(w)
		return
	}
	row, err := h.contactReferenceHistory.GetHistoricalWeComDirectoryMember(r.Context(), id)
	_, digestErr := contactapp.HistoricalWeComDirectoryMemberDigest(row)
	if err != nil || digestErr != nil || row.ID != id {
		contactReferenceHistoryUnavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "item": row})
}

func (h *Handler) contactReferenceHistoryReady(w http.ResponseWriter) bool {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || nilLegacyDependency(h.contactReferenceHistory) {
		contactReferenceHistoryUnavailable(w)
		return false
	}
	return true
}

func writeContactReferenceHistoryPage[T any](w http.ResponseWriter, rows []T, total int64, limit, offset int32, err error, valid func(T) bool) {
	if err != nil || total < 0 || int64(len(rows)) != min(int64(limit), max(int64(0), total-int64(offset))) {
		contactReferenceHistoryUnavailable(w)
		return
	}
	for _, row := range rows {
		if !valid(row) {
			contactReferenceHistoryUnavailable(w)
			return
		}
	}
	if rows == nil {
		rows = []T{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "items": rows, "total": total, "limit": limit, "offset": offset})
}

func contactReferenceHistoryInvalid(w http.ResponseWriter) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_contact_reference_history_query"})
}
func contactReferenceHistoryUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "contact_reference_history_unavailable"})
}
