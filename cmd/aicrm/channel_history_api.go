package main

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
)

// GetChannelHistory reads immutable historical facts. These are not the
// current channel attribution, staff permissions or Provider execution state.
func (handler *Handler) GetChannelHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if handler == nil || r == nil || nilLegacyDependency(handler.channels) || nilLegacyDependency(handler.channelHistory) {
		writeLegacyChannelError(w, contactapp.ErrChannelUnavailable)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "channel_id"), 10, 64)
	if err != nil || id < 1 {
		writeLegacyChannelError(w, contactapp.ErrInvalidChannel)
		return
	}
	limit, offset := int64(50), int64(0)
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeLegacyChannelError(w, contactapp.ErrInvalidChannel)
		return
	}
	for key, values := range query {
		if len(values) != 1 || key != "limit" && key != "offset" {
			writeLegacyChannelError(w, contactapp.ErrInvalidChannel)
			return
		}
		var value int64
		value, err = strconv.ParseInt(values[0], 10, 32)
		if err != nil {
			writeLegacyChannelError(w, contactapp.ErrInvalidChannel)
			return
		}
		if key == "limit" {
			limit = value
		} else {
			offset = value
		}
	}
	if limit < 1 || limit > 100 || offset < 0 {
		writeLegacyChannelError(w, contactapp.ErrInvalidChannel)
		return
	}
	if _, err = handler.channels.GetChannel(r.Context(), id); err != nil {
		writeLegacyChannelError(w, err)
		return
	}
	contacts, total, err := handler.channelHistory.ListHistoricalChannelContacts(r.Context(), id, int32(limit), int32(offset))
	if err != nil {
		writeLegacyChannelError(w, contactapp.ErrChannelUnavailable)
		return
	}
	assignees, err := handler.channelHistory.ListHistoricalChannelAssignees(r.Context(), id)
	if err != nil {
		writeLegacyChannelError(w, contactapp.ErrChannelUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "source": "v1_history", "read_only": true,
		"real_external_call_executed": false, "channel_id": id, "contacts": contacts, "total": total,
		"limit": limit, "offset": offset, "assignees": assignees})
}
