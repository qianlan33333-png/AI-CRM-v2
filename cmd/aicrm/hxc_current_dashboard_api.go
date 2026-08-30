package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

const hxcCurrentDashboardPath = "/api/admin/hxc-current"

type hxcCurrentDashboardItem struct {
	UserRef           string  `json:"user_ref"`
	MatchState        string  `json:"match_state"`
	SubscriptionTier  string  `json:"subscription_tier"`
	CurrentPeriodUsed int32   `json:"current_period_used"`
	MonthlyChatQuota  int32   `json:"monthly_chat_quota"`
	UserMessages7D    int64   `json:"user_messages_7d"`
	UserMessages30D   int64   `json:"user_messages_30d"`
	LastUsedAt        *string `json:"last_used_at"`
	LastCapability    *string `json:"last_capability"`
	BusinessStage     *string `json:"business_stage"`
	UserSegment       *string `json:"user_segment"`
	SourceUpdatedAt   string  `json:"source_updated_at"`
	SyncedAt          string  `json:"synced_at"`
}

func (handler *Handler) ListHXCCurrentDashboard(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if handler == nil || handler.hxcCurrentDashboard == nil {
		writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"code": "hxc_current_unavailable"})
		return
	}
	query := request.URL.Query()
	limit := int64(100)
	if values := query["limit"]; len(values) > 1 {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"code": "invalid_hxc_current_query"})
		return
	}
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || parsed < 1 || parsed > 200 {
			writeJSON(writer, http.StatusBadRequest, map[string]string{"code": "invalid_hxc_current_query"})
			return
		}
		limit = parsed
	}
	if len(query) > 1 || (len(query) == 1 && !query.Has("limit")) {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"code": "invalid_hxc_current_query"})
		return
	}
	snapshot, err := handler.hxcCurrentDashboard.ReadDashboard(request.Context(), int32(limit))
	if err != nil {
		writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"code": "hxc_current_unavailable"})
		return
	}
	items := make([]hxcCurrentDashboardItem, 0, len(snapshot.Rows))
	for _, row := range snapshot.Rows {
		items = append(items, hxcCurrentDashboardItem{
			UserRef: maskHXCUserID(row.HXCUserID), MatchState: string(row.MatchState), SubscriptionTier: row.SubscriptionTier,
			CurrentPeriodUsed: row.CurrentPeriodUsed, MonthlyChatQuota: row.MonthlyChatQuota,
			UserMessages7D: row.UserMessages7D, UserMessages30D: row.UserMessages30D,
			LastUsedAt: formatOptionalTime(row.LastUsedAt), LastCapability: row.LastCapability,
			BusinessStage: row.BusinessStage, UserSegment: row.UserSegment,
			SourceUpdatedAt: row.SourceUpdatedAt.UTC().Format(time.RFC3339), SyncedAt: row.SyncedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"source": "hxc_current_sync", "read_only": true, "real_external_call_executed": false,
		"total": snapshot.Total, "matched_count": snapshot.MatchedCount, "unmatched_count": snapshot.UnmatchedCount,
		"conflict_count": snapshot.ConflictCount, "last_synced_at": formatOptionalTime(snapshot.LastSyncedAt), "items": items,
	})
}

func maskHXCUserID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 4 {
		return "HXC-****"
	}
	return "HXC-****" + value[len(value)-4:]
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339)
	return &formatted
}
