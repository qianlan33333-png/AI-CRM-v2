package main

import (
	"github.com/go-chi/chi/v5"
	app "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/app"
	port "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	"net/http"
)

func (h *Handler) ListMarketingConfigHistoryConfigs(w http.ResponseWriter, r *http.Request) {
	h.serveMarketingConfigHistory(w, r, "marketing_config", false)
}
func (h *Handler) GetMarketingConfigHistoryConfig(w http.ResponseWriter, r *http.Request) {
	h.serveMarketingConfigHistory(w, r, "marketing_config", true)
}
func (h *Handler) ListMarketingConfigHistoryRules(w http.ResponseWriter, r *http.Request) {
	h.serveMarketingConfigHistory(w, r, "marketing_rule", false)
}
func (h *Handler) GetMarketingConfigHistoryRule(w http.ResponseWriter, r *http.Request) {
	h.serveMarketingConfigHistory(w, r, "marketing_rule", true)
}
func (h *Handler) serveMarketingConfigHistory(w http.ResponseWriter, r *http.Request, kind string, detail bool) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.marketingConfigHistory) {
		writeJSON(w, 503, map[string]string{"code": "marketing_config_history_unavailable"})
		return
	}
	limit, offset, valid := audienceHistoryPage(r.URL.RawQuery)
	id := int64(0)
	if detail {
		id, valid = audienceHistoryID(chi.URLParam(r, "history_id"))
		valid = valid && r.URL.RawQuery == ""
	}
	if !valid {
		writeJSON(w, 400, map[string]string{"code": "invalid_marketing_config_history_query"})
		return
	}
	query := port.MarketingConfigHistoryQuery{Limit: limit, Offset: offset}
	var result any
	var total int64
	var count int
	var err error
	switch kind {
	case "marketing_config":
		if detail {
			var item port.HistoricalMarketingAutomationConfig
			item, err = h.marketingConfigHistory.GetHistoricalMarketingAutomationConfig(r.Context(), id)
			if err == nil {
				_, err = app.HistoricalMarketingAutomationConfigDigest(item)
			}
			if err == nil && item.ID != id {
				err = port.ErrMarketingConfigHistoryUnavailable
			}
			result = item
		} else {
			var items []port.HistoricalMarketingAutomationConfig
			items, total, err = h.marketingConfigHistory.ListHistoricalMarketingAutomationConfig(r.Context(), query)
			if items == nil {
				items = []port.HistoricalMarketingAutomationConfig{}
			}
			for _, item := range items {
				if err != nil {
					break
				}
				_, err = app.HistoricalMarketingAutomationConfigDigest(item)
			}
			result, count = items, len(items)
		}
	case "marketing_rule":
		if detail {
			var item port.HistoricalMarketingAutomationRule
			item, err = h.marketingConfigHistory.GetHistoricalMarketingAutomationRule(r.Context(), id)
			if err == nil {
				_, err = app.HistoricalMarketingAutomationRuleDigest(item)
			}
			if err == nil && item.ID != id {
				err = port.ErrMarketingConfigHistoryUnavailable
			}
			result = item
		} else {
			var items []port.HistoricalMarketingAutomationRule
			items, total, err = h.marketingConfigHistory.ListHistoricalMarketingAutomationRule(r.Context(), query)
			if items == nil {
				items = []port.HistoricalMarketingAutomationRule{}
			}
			for _, item := range items {
				if err != nil {
					break
				}
				_, err = app.HistoricalMarketingAutomationRuleDigest(item)
			}
			result, count = items, len(items)
		}
	default:
		err = port.ErrMarketingConfigHistoryUnavailable
	}
	if err != nil || (!detail && (total < 0 || count > int(limit) || int64(count) > total)) {
		writeJSON(w, 503, map[string]string{"code": "marketing_config_history_unavailable"})
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
	writeJSON(w, 200, response)
}
