package main

import (
	"github.com/go-chi/chi/v5"
	automationapp "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/app"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	"net/http"
)

// These GETs read frozen configuration only, never a live agent or executable prompt.
func (h *Handler) ListAutomationHistorySOPs(w http.ResponseWriter, r *http.Request) {
	h.serveAutomationHistory(w, r, "sop", false)
}
func (h *Handler) GetAutomationHistorySOP(w http.ResponseWriter, r *http.Request) {
	h.serveAutomationHistory(w, r, "sop", true)
}
func (h *Handler) ListAutomationHistoryConfigs(w http.ResponseWriter, r *http.Request) {
	h.serveAutomationHistory(w, r, "config", false)
}
func (h *Handler) GetAutomationHistoryConfig(w http.ResponseWriter, r *http.Request) {
	h.serveAutomationHistory(w, r, "config", true)
}
func (h *Handler) ListAutomationHistoryPrompts(w http.ResponseWriter, r *http.Request) {
	h.serveAutomationHistory(w, r, "prompt", false)
}
func (h *Handler) GetAutomationHistoryPrompt(w http.ResponseWriter, r *http.Request) {
	h.serveAutomationHistory(w, r, "prompt", true)
}
func (h *Handler) ListAutomationHistoryAgents(w http.ResponseWriter, r *http.Request) {
	h.serveAutomationHistory(w, r, "agent", false)
}
func (h *Handler) GetAutomationHistoryAgent(w http.ResponseWriter, r *http.Request) {
	h.serveAutomationHistory(w, r, "agent", true)
}

func (h *Handler) serveAutomationHistory(w http.ResponseWriter, r *http.Request, kind string, detail bool) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.automationHistory) {
		automationHistoryUnavailable(w)
		return
	}
	limit, offset, valid := audienceHistoryPage(r.URL.RawQuery)
	id := int64(0)
	if detail {
		id, valid = audienceHistoryID(chi.URLParam(r, "history_id"))
		valid = valid && r.URL.RawQuery == ""
	}
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_automation_history_query"})
		return
	}
	query := automationport.AutomationHistoryQuery{Limit: limit, Offset: offset}
	var result any
	var total int64
	var count int
	var err error
	switch kind {
	case "sop":
		if detail {
			var item automationport.HistoricalAutomationSOP
			item, err = h.automationHistory.GetHistoricalAutomationSOP(r.Context(), id)
			if err == nil {
				_, err = automationapp.HistoricalAutomationSOPDigest(item)
			}
			if err == nil && item.ID != id {
				err = automationport.ErrAutomationHistoryUnavailable
			}
			result = item
		} else {
			var items []automationport.HistoricalAutomationSOP
			items, total, err = h.automationHistory.ListHistoricalAutomationSOPs(r.Context(), query)
			if items == nil {
				items = []automationport.HistoricalAutomationSOP{}
			}
			for _, item := range items {
				if err != nil {
					break
				}
				_, err = automationapp.HistoricalAutomationSOPDigest(item)
			}
			result, count = items, len(items)
		}
	case "config":
		if detail {
			var item automationport.HistoricalAutomationConfig
			item, err = h.automationHistory.GetHistoricalAutomationConfig(r.Context(), id)
			if err == nil {
				_, err = automationapp.HistoricalAutomationConfigDigest(item)
			}
			if err == nil && item.ID != id {
				err = automationport.ErrAutomationHistoryUnavailable
			}
			result = item
		} else {
			var items []automationport.HistoricalAutomationConfig
			items, total, err = h.automationHistory.ListHistoricalAutomationConfigs(r.Context(), query)
			if items == nil {
				items = []automationport.HistoricalAutomationConfig{}
			}
			for _, item := range items {
				if err != nil {
					break
				}
				_, err = automationapp.HistoricalAutomationConfigDigest(item)
			}
			result, count = items, len(items)
		}
	case "prompt":
		if detail {
			var item automationport.HistoricalAutomationPrompt
			item, err = h.automationHistory.GetHistoricalAutomationPrompt(r.Context(), id)
			if err == nil {
				_, err = automationapp.HistoricalAutomationPromptDigest(item)
			}
			if err == nil && item.ID != id {
				err = automationport.ErrAutomationHistoryUnavailable
			}
			result = item
		} else {
			var items []automationport.HistoricalAutomationPrompt
			items, total, err = h.automationHistory.ListHistoricalAutomationPrompts(r.Context(), query)
			if items == nil {
				items = []automationport.HistoricalAutomationPrompt{}
			}
			for _, item := range items {
				if err != nil {
					break
				}
				_, err = automationapp.HistoricalAutomationPromptDigest(item)
			}
			result, count = items, len(items)
		}
	case "agent":
		if detail {
			var item automationport.HistoricalAutomationAgent
			item, err = h.automationHistory.GetHistoricalAutomationAgent(r.Context(), id)
			if err == nil {
				_, err = automationapp.HistoricalAutomationAgentDigest(item)
			}
			if err == nil && item.ID != id {
				err = automationport.ErrAutomationHistoryUnavailable
			}
			result = item
		} else {
			var items []automationport.HistoricalAutomationAgent
			items, total, err = h.automationHistory.ListHistoricalAutomationAgents(r.Context(), query)
			if items == nil {
				items = []automationport.HistoricalAutomationAgent{}
			}
			for _, item := range items {
				if err != nil {
					break
				}
				_, err = automationapp.HistoricalAutomationAgentDigest(item)
			}
			result, count = items, len(items)
		}
	default:
		automationHistoryUnavailable(w)
		return
	}
	if err != nil || !detail && (total < 0 || int64(count) != min(int64(limit), max(0, total-int64(offset)))) {
		automationHistoryUnavailable(w)
		return
	}
	response := map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false}
	if detail {
		response["item"] = result
	} else {
		response["items"] = result
		response["total"] = total
		response["limit"] = limit
		response["offset"] = offset
	}
	writeJSON(w, http.StatusOK, response)
}
func automationHistoryUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "automation_history_unavailable"})
}
