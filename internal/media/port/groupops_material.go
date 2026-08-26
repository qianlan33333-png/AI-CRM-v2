package port

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

var ErrInvalidGroupOpsMaterialSnapshot = errors.New("invalid group ops material snapshot")

// GroupOpsMaterialSnapshot is the immutable, provider-ready part of one
// accepted Group Ops execution. Provider media IDs and link/card fields must
// be resolved before the execution is accepted; workers may only submit this
// stored payload and never reopen a mutable content package.
type GroupOpsMaterialSnapshot struct {
	SchemaVersion int                               `json:"schema_version"`
	NodeKind      string                            `json:"node_kind"`
	Attachments   []GroupOpsProviderReadyAttachment `json:"attachments,omitempty"`
}

// GroupOpsMaterialPlan is persisted by Group Ops as an ordered list of stable
// local Media IDs. It is intentionally not a mutable content-package pointer.
// The Media freezer locks and resolves it into GroupOpsMaterialSnapshot before
// an execution is accepted.
type GroupOpsMaterialPlan struct {
	References []GroupOpsMaterialReference `json:"references"`
}

type GroupOpsMaterialReference struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
}

// GroupOpsProviderReadyAttachment is intentionally the small intersection of
// the WeCom group-message template contract and the legacy P4 content package.
// It contains no local media record IDs, credentials, or mutable URLs that a
// worker would need to resolve later.
type GroupOpsProviderReadyAttachment struct {
	MsgType     string `json:"msgtype"`
	MediaID     string `json:"media_id,omitempty"`
	AppID       string `json:"appid,omitempty"`
	PagePath    string `json:"pagepath,omitempty"`
	Title       string `json:"title,omitempty"`
	URL         string `json:"url,omitempty"`
	Description string `json:"description,omitempty"`
	PicURL      string `json:"picurl,omitempty"`
}

// GroupOpsMaterialSnapshotFreezer belongs to Media. The Group Ops acceptance
// flow calls it before persisting the execution snapshot, and must treat any
// error as a local acceptance failure rather than queue work that a worker
// would need to resolve later.
type GroupOpsMaterialSnapshotFreezer interface {
	FreezeGroupOpsMaterial(context.Context, GroupOpsMaterialPlan) (GroupOpsMaterialSnapshot, error)
}

func ValidateGroupOpsMaterialSnapshot(value GroupOpsMaterialSnapshot) error {
	if value.SchemaVersion != 2 || value.NodeKind != "message" || len(value.Attachments) > 9 {
		return ErrInvalidGroupOpsMaterialSnapshot
	}
	return ValidateGroupOpsProviderReadyAttachments(value.Attachments)
}

func ValidateGroupOpsMaterialPlan(value GroupOpsMaterialPlan) error {
	if len(value.References) == 0 || len(value.References) > 9 {
		return ErrInvalidGroupOpsMaterialSnapshot
	}
	images, minis, attachments, invites := 0, 0, 0, 0
	seen := make(map[string]struct{}, len(value.References))
	for _, reference := range value.References {
		if reference.ID < 1 {
			return ErrInvalidGroupOpsMaterialSnapshot
		}
		key := reference.Kind + ":" + strconv.FormatInt(reference.ID, 10)
		if _, exists := seen[key]; exists {
			return ErrInvalidGroupOpsMaterialSnapshot
		}
		seen[key] = struct{}{}
		switch reference.Kind {
		case "image":
			images++
		case "miniprogram":
			minis++
		case "attachment":
			attachments++
		case "group_invite":
			invites++
		default:
			return ErrInvalidGroupOpsMaterialSnapshot
		}
	}
	if images > 3 || minis > 1 || attachments > 9 || invites > 1 {
		return ErrInvalidGroupOpsMaterialSnapshot
	}
	return nil
}

func ValidateGroupOpsProviderReadyAttachments(attachments []GroupOpsProviderReadyAttachment) error {
	if len(attachments) > 9 {
		return ErrInvalidGroupOpsMaterialSnapshot
	}
	images, minis, files, invites := 0, 0, 0, 0
	for _, attachment := range attachments {
		switch attachment.MsgType {
		case "image":
			images++
			if !validGroupOpsText(attachment.MediaID, 1024) || !emptyAttachmentFields(attachment, "MediaID") {
				return ErrInvalidGroupOpsMaterialSnapshot
			}
		case "file":
			files++
			if !validGroupOpsText(attachment.MediaID, 1024) || !emptyAttachmentFields(attachment, "MediaID") {
				return ErrInvalidGroupOpsMaterialSnapshot
			}
		case "miniprogram":
			minis++
			if !validGroupOpsText(attachment.AppID, 128) || !validGroupOpsText(attachment.PagePath, 1024) ||
				!validGroupOpsText(attachment.Title, 128) || !validGroupOpsText(attachment.MediaID, 1024) ||
				!emptyAttachmentFields(attachment, "MediaID", "AppID", "PagePath", "Title") {
				return ErrInvalidGroupOpsMaterialSnapshot
			}
		case "link":
			invites++
			if !validGroupOpsText(attachment.Title, 128) || !validGroupInviteURL(attachment.URL) ||
				!validOptionalGroupOpsText(attachment.Description, 512) ||
				(attachment.PicURL != "" && !validHTTPURL(attachment.PicURL)) ||
				!emptyAttachmentFields(attachment, "Title", "URL", "Description", "PicURL") {
				return ErrInvalidGroupOpsMaterialSnapshot
			}
		default:
			return ErrInvalidGroupOpsMaterialSnapshot
		}
	}
	if images > 3 || minis > 1 || files > 9 || invites > 1 {
		return ErrInvalidGroupOpsMaterialSnapshot
	}
	return nil
}

func validGroupOpsText(value string, limit int) bool {
	return value != "" && validOptionalGroupOpsText(value, limit)
}

func validOptionalGroupOpsText(value string, limit int) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) == value && len(value) <= limit
}

func validGroupInviteURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host == "work.weixin.qq.com" && strings.HasPrefix(parsed.Path, "/gm/") && parsed.User == nil && parsed.Fragment == ""
}

func validHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" && parsed.User == nil && parsed.Fragment == ""
}

func emptyAttachmentFields(value GroupOpsProviderReadyAttachment, allowed ...string) bool {
	permitted := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		permitted[field] = struct{}{}
	}
	for field, current := range map[string]string{
		"MediaID": value.MediaID, "AppID": value.AppID, "PagePath": value.PagePath, "Title": value.Title,
		"URL": value.URL, "Description": value.Description, "PicURL": value.PicURL,
	} {
		if _, ok := permitted[field]; !ok && current != "" {
			return false
		}
	}
	return true
}
