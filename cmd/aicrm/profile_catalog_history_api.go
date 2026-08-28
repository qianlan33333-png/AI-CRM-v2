package main

import (
	"github.com/go-chi/chi/v5"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	"net/http"
)

func profileHistoryInvalid(w http.ResponseWriter) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_profile_history_query"})
}
func profileHistoryUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "profile_history_unavailable"})
}

func (h *Handler) ListProfileHistoryTemplates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.profileCatalogHistory) {
		profileHistoryUnavailable(w)
		return
	}
	limit, offset, ok := audienceHistoryPage(r.URL.RawQuery)
	if !ok {
		profileHistoryInvalid(w)
		return
	}
	query := segmentport.ProfileCatalogHistoryQuery{Limit: limit, Offset: offset}
	items, total, err := h.profileCatalogHistory.ListHistoricalProfileTemplates(r.Context(), query)
	if err != nil || total < 0 || int64(len(items)) != min(int64(limit), max(0, total-int64(offset))) {
		profileHistoryUnavailable(w)
		return
	}
	for _, item := range items {
		if _, err := segmentapp.HistoricalProfileTemplateDigest(item); err != nil {
			profileHistoryUnavailable(w)
			return
		}
	}
	if items == nil {
		items = []segmentport.HistoricalProfileTemplate{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "items": items, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) GetProfileHistoryTemplate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.profileCatalogHistory) {
		profileHistoryUnavailable(w)
		return
	}
	template_id, ok := audienceHistoryID(chi.URLParam(r, "template_id"))
	if !ok {
		profileHistoryInvalid(w)
		return
	}
	if r.URL.RawQuery != "" {
		profileHistoryInvalid(w)
		return
	}
	item, err := h.profileCatalogHistory.GetHistoricalProfileTemplate(r.Context(), template_id)
	if err != nil || item.ID != template_id {
		profileHistoryUnavailable(w)
		return
	}
	if _, err := segmentapp.HistoricalProfileTemplateDigest(item); err != nil {
		profileHistoryUnavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "item": item})
}

func (h *Handler) ListProfileHistoryCategories(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.profileCatalogHistory) {
		profileHistoryUnavailable(w)
		return
	}
	template_id, ok := audienceHistoryID(chi.URLParam(r, "template_id"))
	if !ok {
		profileHistoryInvalid(w)
		return
	}
	limit, offset, ok := audienceHistoryPage(r.URL.RawQuery)
	if !ok {
		profileHistoryInvalid(w)
		return
	}
	query := segmentport.ProfileCatalogHistoryQuery{Limit: limit, Offset: offset}
	query.TemplateHistoryID = &template_id
	items, total, err := h.profileCatalogHistory.ListHistoricalProfileCategories(r.Context(), query)
	if err != nil || total < 0 || int64(len(items)) != min(int64(limit), max(0, total-int64(offset))) {
		profileHistoryUnavailable(w)
		return
	}
	for _, item := range items {
		if _, err := segmentapp.HistoricalProfileCategoryDigest(item); err != nil {
			profileHistoryUnavailable(w)
			return
		}
		if item.TemplateHistoryID != template_id {
			profileHistoryUnavailable(w)
			return
		}
	}
	if items == nil {
		items = []segmentport.HistoricalProfileCategory{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "items": items, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) ListProfileHistoryOptionMappings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.profileCatalogHistory) {
		profileHistoryUnavailable(w)
		return
	}
	template_id, ok := audienceHistoryID(chi.URLParam(r, "template_id"))
	if !ok {
		profileHistoryInvalid(w)
		return
	}
	category_id, ok := audienceHistoryID(chi.URLParam(r, "category_id"))
	if !ok {
		profileHistoryInvalid(w)
		return
	}
	limit, offset, ok := audienceHistoryPage(r.URL.RawQuery)
	if !ok {
		profileHistoryInvalid(w)
		return
	}
	query := segmentport.ProfileCatalogHistoryQuery{Limit: limit, Offset: offset}
	query.TemplateHistoryID = &template_id
	query.CategoryHistoryID = &category_id
	items, total, err := h.profileCatalogHistory.ListHistoricalProfileOptionMappings(r.Context(), query)
	if err != nil || total < 0 || int64(len(items)) != min(int64(limit), max(0, total-int64(offset))) {
		profileHistoryUnavailable(w)
		return
	}
	for _, item := range items {
		if _, err := segmentapp.HistoricalProfileOptionMappingDigest(item); err != nil {
			profileHistoryUnavailable(w)
			return
		}
		if item.TemplateHistoryID != template_id {
			profileHistoryUnavailable(w)
			return
		}
		if item.CategoryHistoryID != category_id {
			profileHistoryUnavailable(w)
			return
		}
	}
	if items == nil {
		items = []segmentport.HistoricalProfileOptionMapping{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "items": items, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) ListSignupTagHistoryRules(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.signupTagHistory) {
		profileHistoryUnavailable(w)
		return
	}
	limit, offset, ok := audienceHistoryPage(r.URL.RawQuery)
	if !ok {
		profileHistoryInvalid(w)
		return
	}
	items, total, err := h.signupTagHistory.ListHistoricalSignupTagRules(r.Context(), limit, offset)
	if err != nil || total < 0 || int64(len(items)) != min(int64(limit), max(0, total-int64(offset))) {
		profileHistoryUnavailable(w)
		return
	}
	for _, item := range items {
		if _, err := contactapp.HistoricalSignupTagRuleDigest(item); err != nil {
			profileHistoryUnavailable(w)
			return
		}
	}
	if items == nil {
		items = []contactport.HistoricalSignupTagRule{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "items": items, "total": total, "limit": limit, "offset": offset})
}
