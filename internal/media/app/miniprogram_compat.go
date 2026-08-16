package app

import (
	"errors"
	"strings"
	"time"

	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

// MiniProgramCompatibilityMeta is deliberately transport-neutral so the
// later central router can apply the frozen envelope without duplicating card
// serialization or accidentally reporting a real external effect.
type MiniProgramCompatibilityMeta struct{ AdapterMode string }

func (meta MiniProgramCompatibilityMeta) normalizedMode() string {
	switch strings.ToLower(strings.TrimSpace(meta.AdapterMode)) {
	case "fake", "disabled", "staging":
		return strings.ToLower(strings.TrimSpace(meta.AdapterMode))
	default:
		return "fake"
	}
}

func MiniProgramCompatibilityItem(item mediaport.MiniProgramCard) map[string]any {
	return map[string]any{
		"id":                        item.ID,
		"name":                      item.Name,
		"appid":                     item.AppID,
		"pagepath":                  item.PagePath,
		"page_path":                 item.PagePath,
		"title":                     item.Title,
		"thumb_image_id":            item.ThumbImageID,
		"thumb_media_id":            item.ThumbMediaID,
		"thumb_media_id_expires_at": item.ThumbMediaIDExpiresAt,
		"thumb_image_url":           item.ThumbImageURL,
		"thumb_image_base64":        item.ThumbImageBase64,
		"enabled":                   item.Enabled,
		"created_at":                miniProgramLegacyTime(item.CreatedAt),
		"updated_at":                miniProgramLegacyTime(item.UpdatedAt),
	}
}

func MiniProgramCompatibilityList(page mediaport.MiniProgramPage, meta MiniProgramCompatibilityMeta) map[string]any {
	items := make([]any, len(page.Items))
	for index, item := range page.Items {
		items[index] = MiniProgramCompatibilityItem(item)
	}
	nextOffset := page.Offset + int32(len(items))
	payload := map[string]any{"ok": true, "items": items, "total": page.Total, "limit": page.Limit, "offset": page.Offset,
		"count": len(items), "has_more": int64(nextOffset) < page.Total, "next_offset": nil}
	if int64(nextOffset) < page.Total {
		payload["next_offset"] = nextOffset
	}
	return miniProgramCompatibilityEnvelope(payload, "next_media_library", meta)
}

func MiniProgramCompatibilityMutation(result mediaport.MiniProgramMutationResult, idempotencyKey string, meta MiniProgramCompatibilityMeta) map[string]any {
	payload := map[string]any{"ok": true, "item": MiniProgramCompatibilityItem(result.Item),
		"side_effect_plan": miniProgramSideEffectPlan("miniprogram_upsert", idempotencyKey, "local_repository_write_only")}
	if result.ThumbResolve != nil {
		payload["thumb_resolve"] = MiniProgramCompatibilityResolution(*result.ThumbResolve, &result.Item)
	}
	return miniProgramCompatibilityEnvelope(payload, "local_repository_write", meta)
}

func MiniProgramCompatibilityDelete(result mediaport.MiniProgramDeleteResult, idempotencyKey string, meta MiniProgramCompatibilityMeta) map[string]any {
	payload := map[string]any{"ok": result.Deleted, "deleted": result.Deleted, "hard_deleted": result.HardDeleted, "id": result.ID,
		"side_effect_plan": miniProgramSideEffectPlan("miniprogram_delete", idempotencyKey, "delete is a local repository mutation; external storage and WeCom media references are not deleted by this route")}
	return miniProgramCompatibilityEnvelope(payload, "local_delete", meta)
}

func MiniProgramCompatibilityResolution(result mediaport.MiniProgramThumbResolution, item *mediaport.MiniProgramCard) map[string]any {
	payload := map[string]any{"ok": result.OK, "side_effect_executed": false, "real_external_call_executed": false}
	if result.ThumbMediaID != "" {
		payload["thumb_media_id"] = result.ThumbMediaID
	}
	if result.Source != "" {
		payload["source"] = result.Source
	}
	if result.Error != "" {
		payload["error"] = result.Error
	}
	if result.ErrorMessage != "" {
		payload["error_message"] = result.ErrorMessage
	}
	if result.ThumbImageID != nil {
		payload["thumb_image_id"] = *result.ThumbImageID
	}
	if result.OK && item != nil {
		payload["item"] = MiniProgramCompatibilityItem(*item)
	}
	return payload
}

func MiniProgramCompatibilityError(err error, meta MiniProgramCompatibilityMeta) (int, map[string]any) {
	status := 400
	if errors.Is(err, ErrMiniProgramNotFound) {
		status = 404
	}
	return status, miniProgramCompatibilityEnvelope(map[string]any{"ok": false, "error": err.Error()}, "next_media_library_error", meta)
}

// MiniProgramCompatibilityValidationError reserves FastAPI's 422 seam for a
// transport decoder/type error. Domain contract violations remain 400.
func MiniProgramCompatibilityValidationError(detail string) (int, map[string]any) {
	return 422, map[string]any{"detail": []map[string]any{{"loc": []string{"body"}, "msg": detail}}}
}

func miniProgramCompatibilityEnvelope(payload map[string]any, source string, meta MiniProgramCompatibilityMeta) map[string]any {
	payload["source_status"] = source
	payload["route_owner"] = "ai_crm_next"
	payload["fallback_used"] = false
	payload["real_external_call_executed"] = false
	payload["storage_adapter_mode"] = meta.normalizedMode()
	payload["adapter_mode"] = meta.normalizedMode()
	return payload
}

func miniProgramSideEffectPlan(operation, key, reason string) map[string]any {
	return map[string]any{"operation": operation, "external_storage": "not_executed", "wecom_media_upload": "not_executed",
		"real_external_call": "not_executed", "database_write": "executed", "audit": "response_side_effect_plan",
		"idempotency_key": key, "idempotency_required": false, "idempotency_reason": reason}
}

func miniProgramLegacyTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02T15:04:05.999999-07:00")
}
