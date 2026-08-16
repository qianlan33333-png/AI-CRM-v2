package app

import (
	"errors"
	"testing"
	"time"

	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

func TestMiniProgramCompatibilityEnvelopeAndEmptyTimes(t *testing.T) {
	item := mediaport.MiniProgramCard{ID: 7, Name: "卡片", Title: "卡片", AppID: "wx_demo", PagePath: "pages/index", Enabled: true}
	encoded := MiniProgramCompatibilityItem(item)
	if encoded["pagepath"] != "pages/index" || encoded["page_path"] != "pages/index" || encoded["created_at"] != "" || encoded["updated_at"] != "" {
		t.Fatalf("item=%#v", encoded)
	}
	item.CreatedAt = time.Date(2026, 8, 16, 12, 0, 0, 123456000, time.UTC)
	page := MiniProgramCompatibilityList(mediaport.MiniProgramPage{Items: []mediaport.MiniProgramCard{item}, Total: 2, Limit: 100, Offset: 0}, MiniProgramCompatibilityMeta{AdapterMode: "production"})
	if page["count"] != 1 || page["has_more"] != true || page["next_offset"] != int32(1) || page["storage_adapter_mode"] != "fake" || page["real_external_call_executed"] != false {
		t.Fatalf("page=%#v", page)
	}
}

func TestMiniProgramCompatibilityMutationFailureDoesNotClaimSendable(t *testing.T) {
	item := mediaport.MiniProgramCard{ID: 7, Name: "卡片", Title: "卡片", AppID: "wx_demo", PagePath: "pages/index", Enabled: true}
	resolution := mediaport.MiniProgramThumbResolution{OK: false, Error: "real_wecom_media_resolve_failed"}
	payload := MiniProgramCompatibilityMutation(mediaport.MiniProgramMutationResult{Item: item, ThumbResolve: &resolution}, "key", MiniProgramCompatibilityMeta{AdapterMode: "disabled"})
	thumb, ok := payload["thumb_resolve"].(map[string]any)
	if !ok || thumb["ok"] != false || thumb["side_effect_executed"] != false || thumb["real_external_call_executed"] != false || payload["real_external_call_executed"] != false {
		t.Fatalf("payload=%#v", payload)
	}
	status, notFound := MiniProgramCompatibilityError(ErrMiniProgramNotFound, MiniProgramCompatibilityMeta{})
	if status != 404 || notFound["ok"] != false {
		t.Fatalf("notFound=%d %#v", status, notFound)
	}
	status, generic := MiniProgramCompatibilityError(errors.New("invalid"), MiniProgramCompatibilityMeta{})
	if status != 400 || generic["source_status"] != "next_media_library_error" {
		t.Fatalf("generic=%d %#v", status, generic)
	}
	status, validation := MiniProgramCompatibilityValidationError("invalid body")
	if status != 422 || validation["detail"] == nil {
		t.Fatalf("validation=%d %#v", status, validation)
	}
}
