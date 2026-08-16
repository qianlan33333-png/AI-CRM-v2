package domain

import (
	"strings"
	"testing"
	"time"

	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

func TestMiniProgramCreationDefaultsAndFrozenRuneLimits(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	title := strings.Repeat("界", MaxMiniProgramTitleRunes+1)
	appid, page := " wx_demo ", " pages/home "
	item, err := NewMiniProgram(mediaport.MiniProgramUpsert{Title: &title, AppID: &appid, PagePath: &page}, 7, now)
	if err != nil || item.Name != strings.Repeat("界", MaxMiniProgramNameRunes) || item.Title != strings.Repeat("界", MaxMiniProgramTitleRunes) || item.AppID != "wx_demo" || item.PagePath != "pages/home" || !item.Enabled {
		t.Fatalf("item=%#v err=%v", item, err)
	}
	if !ValidMiniProgram(item, false) {
		t.Fatalf("new item invalid=%#v", item)
	}
}

func TestMiniProgramPatchClearsCachedThumbOnImageChange(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	title, appid, page := "卡片", "wx_demo", "pages/home"
	item, err := NewMiniProgram(mediaport.MiniProgramUpsert{Title: &title, AppID: &appid, PagePath: &page}, 7, now)
	if err != nil {
		t.Fatal(err)
	}
	item.ID, item.ThumbMediaID = 9, "old-cache"
	imageID, directMedia := int64(19), "direct-value-must-not-win"
	updated, err := ApplyMiniProgramPatch(item, mediaport.MiniProgramUpsert{ThumbImageID: &imageID, ThumbMediaID: &directMedia}, 8, now.Add(time.Minute))
	if err != nil || updated.ThumbImageID == nil || *updated.ThumbImageID != imageID || updated.ThumbMediaID != "" || updated.Version != 2 || updated.UpdatedBy != 8 {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
}
