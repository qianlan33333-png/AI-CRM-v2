package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

const (
	MaxMiniProgramNameRunes         = 200
	MaxMiniProgramAppIDRunes        = 120
	MaxMiniProgramPagePathRunes     = 500
	MaxMiniProgramTitleRunes        = 200
	MaxMiniProgramThumbMediaIDRunes = 255
)

var ErrInvalidMiniProgram = errors.New("invalid miniprogram material")

func NewMiniProgram(input mediaport.MiniProgramUpsert, actor int64, now time.Time) (mediaport.MiniProgramCard, error) {
	if actor < 1 || now.IsZero() {
		return mediaport.MiniProgramCard{}, ErrInvalidMiniProgram
	}
	name := miniProgramText(input.Name, MaxMiniProgramNameRunes)
	title := miniProgramText(input.Title, MaxMiniProgramTitleRunes)
	if title == "" {
		title = truncateRunes(name, MaxMiniProgramTitleRunes)
	}
	if name == "" {
		name = truncateRunes(title, MaxMiniProgramNameRunes)
	}
	item := mediaport.MiniProgramCard{
		Name: name, Title: title,
		AppID:        miniProgramText(input.AppID, MaxMiniProgramAppIDRunes),
		PagePath:     miniProgramText(input.PagePath, MaxMiniProgramPagePathRunes),
		ThumbImageID: cloneID(input.ThumbImageID),
		ThumbMediaID: miniProgramText(input.ThumbMediaID, MaxMiniProgramThumbMediaIDRunes),
		Enabled:      enabledOrDefault(input.Enabled, true), CreatedBy: actor, UpdatedBy: actor,
		Version: 1, CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}
	if !ValidMiniProgram(item, false) {
		return mediaport.MiniProgramCard{}, ErrInvalidMiniProgram
	}
	return item, nil
}

func ApplyMiniProgramPatch(current mediaport.MiniProgramCard, patch mediaport.MiniProgramUpsert, actor int64, now time.Time) (mediaport.MiniProgramCard, error) {
	if !ValidMiniProgram(current, true) || EmptyMiniProgramPatch(patch) || actor < 1 || now.IsZero() {
		return mediaport.MiniProgramCard{}, ErrInvalidMiniProgram
	}
	if patch.Name != nil {
		current.Name = miniProgramText(patch.Name, MaxMiniProgramNameRunes)
	}
	if patch.Title != nil {
		current.Title = miniProgramText(patch.Title, MaxMiniProgramTitleRunes)
	}
	if patch.AppID != nil {
		current.AppID = miniProgramText(patch.AppID, MaxMiniProgramAppIDRunes)
	}
	if patch.PagePath != nil {
		current.PagePath = miniProgramText(patch.PagePath, MaxMiniProgramPagePathRunes)
	}
	if patch.ThumbImageID != nil {
		current.ThumbImageID = cloneID(patch.ThumbImageID)
		// Frozen legacy behavior clears a previously cached media id whenever
		// the selected thumbnail image changes, even if both fields are sent.
		current.ThumbMediaID = ""
	} else if patch.ThumbMediaID != nil {
		current.ThumbMediaID = miniProgramText(patch.ThumbMediaID, MaxMiniProgramThumbMediaIDRunes)
	}
	if patch.Enabled != nil {
		current.Enabled = *patch.Enabled
	}
	current.UpdatedBy, current.UpdatedAt, current.Version = actor, now.UTC(), current.Version+1
	if !ValidMiniProgram(current, true) {
		return mediaport.MiniProgramCard{}, ErrInvalidMiniProgram
	}
	return current, nil
}

func EmptyMiniProgramPatch(patch mediaport.MiniProgramUpsert) bool {
	return patch.Name == nil && patch.Title == nil && patch.AppID == nil && patch.PagePath == nil && patch.ThumbImageID == nil &&
		patch.ThumbMediaID == nil && patch.Enabled == nil
}

func ValidMiniProgram(item mediaport.MiniProgramCard, persisted bool) bool {
	if persisted && (item.ID < 1 || item.CreatedBy < 1 || item.UpdatedBy < 1 || item.Version < 1 || item.CreatedAt.IsZero() || item.UpdatedAt.IsZero()) {
		return false
	}
	if item.AppID == "" || item.PagePath == "" || item.Title == "" || (!persisted && item.Name == "") || item.CreatedBy < 1 || item.UpdatedBy < 1 || item.UpdatedAt.IsZero() ||
		!utf8.ValidString(item.Name) || !utf8.ValidString(item.AppID) || !utf8.ValidString(item.PagePath) || !utf8.ValidString(item.Title) || !utf8.ValidString(item.ThumbMediaID) ||
		utf8.RuneCountInString(item.Name) > MaxMiniProgramNameRunes || utf8.RuneCountInString(item.AppID) > MaxMiniProgramAppIDRunes ||
		utf8.RuneCountInString(item.PagePath) > MaxMiniProgramPagePathRunes || utf8.RuneCountInString(item.Title) > MaxMiniProgramTitleRunes ||
		utf8.RuneCountInString(item.ThumbMediaID) > MaxMiniProgramThumbMediaIDRunes {
		return false
	}
	return item.ThumbImageID == nil || *item.ThumbImageID >= 0
}

func miniProgramText(value *string, maximum int) string {
	if value == nil {
		return ""
	}
	return truncateRunes(strings.TrimSpace(*value), maximum)
}

func truncateRunes(value string, maximum int) string {
	if maximum < 1 || utf8.RuneCountInString(value) <= maximum {
		return value
	}
	return string([]rune(value)[:maximum])
}

func enabledOrDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func cloneID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
