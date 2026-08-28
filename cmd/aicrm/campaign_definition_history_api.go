package main

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	campaignapp "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/app"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
)

func (h *Handler) campaignDefinitionHistoryReady(w http.ResponseWriter) bool {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || nilLegacyDependency(h.campaignDefinitionHistory) {
		campaignHistoryUnavailable(w)
		return false
	}
	return true
}

func (h *Handler) ListCampaignHistoryDefinitions(w http.ResponseWriter, r *http.Request) {
	if !h.campaignDefinitionHistoryReady(w) {
		return
	}
	limit, offset, _, ok := campaignHistoryQuery(r.URL.RawQuery, "")
	if !ok {
		campaignHistoryInvalid(w)
		return
	}
	rows, total, err := h.campaignDefinitionHistory.ListHistoricalCampaignDefinitions(r.Context(), limit, offset)
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if _, digestErr := campaignapp.HistoricalCampaignDefinitionDigest(row); digestErr != nil {
			campaignHistoryUnavailable(w)
			return
		}
		items = append(items, campaignDefinitionPublic(row))
	}
	writeCampaignHistoryPage(w, items, total, limit, offset, err, "", 0, func(map[string]any) bool { return true })
}

func (h *Handler) GetCampaignHistoryDefinition(w http.ResponseWriter, r *http.Request) {
	if !h.campaignDefinitionHistoryReady(w) {
		return
	}
	id, ok := campaignHistoryID(chi.URLParam(r, "history_id"))
	if !ok || r.URL.RawQuery != "" {
		campaignHistoryInvalid(w)
		return
	}
	row, err := h.campaignDefinitionHistory.GetHistoricalCampaignDefinition(r.Context(), id)
	_, digestErr := campaignapp.HistoricalCampaignDefinitionDigest(row)
	if err != nil || digestErr != nil || row.ID != id {
		campaignHistoryUnavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "item": campaignDefinitionPublic(row)})
}

func (h *Handler) ListCampaignHistoryDefinitionSteps(w http.ResponseWriter, r *http.Request) {
	if !h.campaignDefinitionHistoryReady(w) {
		return
	}
	// V1 IDs are signed historical references; the current V2 ID parser is
	// deliberately not used for this filter.
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		campaignHistoryInvalid(w)
		return
	}
	var sourceID *int64
	if raw, found := values["campaign_source_id"]; found {
		if len(raw) != 1 {
			campaignHistoryInvalid(w)
			return
		}
		id, parseErr := strconv.ParseInt(raw[0], 10, 64)
		if parseErr != nil || strconv.FormatInt(id, 10) != raw[0] {
			campaignHistoryInvalid(w)
			return
		}
		sourceID = &id
		values.Del("campaign_source_id")
	}
	limit, offset, _, ok := campaignHistoryQuery(values.Encode(), "")
	if !ok {
		campaignHistoryInvalid(w)
		return
	}
	rows, total, err := h.campaignDefinitionHistory.ListHistoricalCampaignDefinitionSteps(r.Context(), sourceID, limit, offset)
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if _, digestErr := campaignapp.HistoricalCampaignDefinitionStepDigest(row); digestErr != nil || (sourceID != nil && row.CampaignSourceID != *sourceID) {
			campaignHistoryUnavailable(w)
			return
		}
		items = append(items, campaignDefinitionStepPublic(row))
	}
	writeCampaignHistoryPage(w, items, total, limit, offset, err, "", 0, func(map[string]any) bool { return true })
}

// Explicit response allowlists keep private migration digests and redacted
// source material out of HTTP, even if the internal facts gain new fields.
func campaignDefinitionPublic(v campaignport.HistoricalCampaignDefinition) map[string]any {
	return map[string]any{"id": v.ID, "source_id": v.SourceID, "code": v.Code, "display_name": v.DisplayName,
		"intent": v.Intent, "anchor_mode": v.AnchorMode, "anchor_date": v.AnchorDate, "review_status": v.ReviewStatus,
		"run_status": v.RunStatus, "approved_at": v.ApprovedAt, "started_at": v.StartedAt, "finished_at": v.FinishedAt,
		"paused_at": v.PausedAt, "paused_reason": v.PausedReason, "created_at": v.CreatedAt, "updated_at": v.UpdatedAt,
		"original_disposition": v.OriginalDisposition, "original_reason": v.OriginalReason}
}

func campaignDefinitionStepPublic(v campaignport.HistoricalCampaignDefinitionStep) map[string]any {
	return map[string]any{"id": v.ID, "source_id": v.SourceID, "campaign_source_id": v.CampaignSourceID, "segment_source_id": v.SegmentSourceID,
		"history_definition_id": v.HistoryDefinitionID, "current_campaign_code": v.CurrentCampaignCode, "source_parent_state": v.SourceParentState,
		"step_index": v.StepIndex, "day_offset": v.DayOffset, "send_time": v.SendTime, "timezone": v.Timezone, "content_masked": v.ContentMasked,
		"stop_on_reply": v.StopOnReply, "skip_recent_days": v.SkipRecentDays, "created_at": v.CreatedAt, "updated_at": v.UpdatedAt,
		"original_disposition": v.OriginalDisposition, "original_reason": v.OriginalReason}
}
