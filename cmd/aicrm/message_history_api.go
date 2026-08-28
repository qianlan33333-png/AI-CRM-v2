package main

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

// ListMessageHistory exposes only the immutable, masked V1 message projection.
// It neither performs a current sync nor provides a path to dispatch a message.
func (h *Handler) ListMessageHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.messageHistory) {
		messageHistoryUnavailable(w)
		return
	}
	query, ok := messageHistoryQuery(r.URL.RawQuery)
	if !ok {
		messageHistoryInvalid(w)
		return
	}
	items, total, err := h.messageHistory.ListHistoricalMessages(r.Context(), query)
	if err != nil || !validMessageHistoryPage(items, total, query) {
		messageHistoryUnavailable(w)
		return
	}
	if items == nil {
		items = []wecomport.HistoricalMessage{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"source": "v1_history", "read_only": true, "real_external_call_executed": false,
		"items": items, "total": total, "limit": query.Limit, "offset": query.Offset,
	})
}

// GetMessageHistory reads one actual V2 history row. Missing and malformed
// rows fail closed as dependency unavailable; this frozen port has no not-found
// distinction.
func (h *Handler) GetMessageHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.messageHistory) {
		messageHistoryUnavailable(w)
		return
	}
	id, ok := messageHistoryID(chi.URLParam(r, "history_id"))
	if !ok || r.URL.RawQuery != "" {
		messageHistoryInvalid(w)
		return
	}
	item, err := h.messageHistory.GetHistoricalMessage(r.Context(), id)
	if err != nil || item.ID != id || !validMessageHistoryItem(item) {
		messageHistoryUnavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"source": "v1_history", "read_only": true, "real_external_call_executed": false, "item": item,
	})
}

func messageHistoryQuery(raw string) (wecomport.MessageHistoryQuery, bool) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return wecomport.MessageHistoryQuery{}, false
	}
	result := wecomport.MessageHistoryQuery{Limit: 50}
	for name, value := range values {
		if len(value) != 1 {
			return wecomport.MessageHistoryQuery{}, false
		}
		switch name {
		case "customer_id":
			id, ok := messageHistoryID(value[0])
			if !ok {
				return wecomport.MessageHistoryQuery{}, false
			}
			result.CustomerID = &id
		case "chat_type":
			if value[0] != "private" && value[0] != "group" {
				return wecomport.MessageHistoryQuery{}, false
			}
			result.ChatType = value[0]
		case "limit":
			parsed, ok := messageHistoryInt32(value[0], 1, 100)
			if !ok {
				return wecomport.MessageHistoryQuery{}, false
			}
			result.Limit = parsed
		case "offset":
			parsed, ok := messageHistoryInt32(value[0], 0, 2147483647)
			if !ok {
				return wecomport.MessageHistoryQuery{}, false
			}
			result.Offset = parsed
		default:
			return wecomport.MessageHistoryQuery{}, false
		}
	}
	return result, true
}

func messageHistoryID(raw string) (int64, bool) {
	id, err := strconv.ParseInt(raw, 10, 64)
	return id, err == nil && id > 0 && strconv.FormatInt(id, 10) == raw
}

func messageHistoryInt32(raw string, minimum, maximum int64) (int32, bool) {
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || value < minimum || value > maximum || strconv.FormatInt(value, 10) != raw {
		return 0, false
	}
	return int32(value), true
}

func validMessageHistoryPage(items []wecomport.HistoricalMessage, total int64, query wecomport.MessageHistoryQuery) bool {
	if total < 0 || len(items) > int(query.Limit) {
		return false
	}
	expected := total - int64(query.Offset)
	if expected < 0 {
		expected = 0
	}
	if expected > int64(query.Limit) {
		expected = int64(query.Limit)
	}
	if int64(len(items)) != expected {
		return false
	}
	for _, item := range items {
		if !validMessageHistoryItem(item) || query.CustomerID != nil && (item.CustomerID == nil || *item.CustomerID != *query.CustomerID) || query.ChatType != "" && item.ChatType != query.ChatType {
			return false
		}
	}
	return true
}

func validMessageHistoryItem(item wecomport.HistoricalMessage) bool {
	_, err := wecomapp.HistoricalMessageDigest(item)
	return err == nil
}

func messageHistoryInvalid(w http.ResponseWriter) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_message_history_query"})
}

func messageHistoryUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "message_history_unavailable"})
}
