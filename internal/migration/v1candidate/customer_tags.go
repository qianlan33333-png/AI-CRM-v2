// Package v1candidate makes fail-closed, side-effect-free archive decisions
// for V1 customer profile and tag/segment rows. It deliberately has no SQL,
// target repository, Provider, queue, or execution dependency.
package v1candidate

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	ReasonProfileRequiresArchive    = "profile_requires_approved_target_schema"
	ReasonTagCatalogRequiresArchive = "wecom_tag_catalog_requires_target_crosswalk"
	ReasonContactTagRequiresArchive = "contact_tag_tagged_by_requires_target_crosswalk"
	ReasonLegacySQLRequiresArchive  = "legacy_sql_requires_archive"
)

// V1 profile fields have no created_at and there is no approved V2 profile
// target schema, so every valid row is archive-only.
type ProfileFieldsRow struct {
	UnionID               string
	Source                string
	Industry              string
	IndustryDescription   string
	NeedsBlockersFollowup string
	UpdatedBy             string
	UpdatedAt             time.Time
}

func ConvertProfile(row ProfileFieldsRow) Decision[struct{}] {
	if !validRequiredText(row.UnionID) || !validText(row.Source) || !validText(row.Industry) || !validText(row.IndustryDescription) || !validText(row.UpdatedBy) || row.UpdatedAt.IsZero() {
		return quarantine[struct{}](ReasonInvalidSource)
	}
	return archive[struct{}](ReasonProfileRequiresArchive)
}

// V1 WeCom group/tag catalogs are retained for future approved crosswalks;
// this conversion does not invent V2 catalog facts.
type TagGroupRow struct {
	GroupID    string
	GroupName  string
	GroupKey   string
	TagCount   int
	RawPayload json.RawMessage
	SyncedAt   *time.Time
	UpdatedAt  time.Time
}

func ConvertTagGroup(row TagGroupRow) Decision[struct{}] {
	if !validRequiredText(row.GroupID) || !validRequiredText(row.GroupName) || !validText(row.GroupKey) || row.TagCount < 0 || row.UpdatedAt.IsZero() || (row.SyncedAt != nil && row.SyncedAt.IsZero()) || !validJSON(row.RawPayload) {
		return quarantine[struct{}](ReasonInvalidSource)
	}
	return archive[struct{}](ReasonTagCatalogRequiresArchive)
}

type TagRow struct {
	TagID      string
	TagName    string
	GroupID    string
	GroupName  string
	OrderIndex int
	DeletedAt  *time.Time
	RawPayload json.RawMessage
	SyncedAt   *time.Time
	UpdatedAt  time.Time
}

func ConvertTag(row TagRow) Decision[struct{}] {
	if !validRequiredText(row.TagID) || !validRequiredText(row.TagName) || !validRequiredText(row.GroupID) || !validText(row.GroupName) || row.OrderIndex < 0 || row.UpdatedAt.IsZero() || (row.DeletedAt != nil && row.DeletedAt.IsZero()) || (row.SyncedAt != nil && row.SyncedAt.IsZero()) || !validJSON(row.RawPayload) {
		return quarantine[struct{}](ReasonInvalidSource)
	}
	return archive[struct{}](ReasonTagCatalogRequiresArchive)
}

// V2 customer_tags.tagged_by is text while V1 records a staff ID. Until an
// approved crosswalk exists, valid contact-tag rows are archive-only.
type ContactTagRow struct {
	ID              int64
	UnionID         string
	UserID          string
	TagID           string
	TagName         string
	Source          string
	QuestionnaireID string
	SubmissionID    string
	IdempotencyKey  string
	RawPayload      json.RawMessage
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func ConvertContactTag(row ContactTagRow) Decision[struct{}] {
	if row.ID < 1 || !validRequiredText(row.UnionID) || !validRequiredText(row.UserID) || !validRequiredText(row.TagID) || !validText(row.TagName) || !validText(row.Source) || !validText(row.QuestionnaireID) || !validText(row.SubmissionID) || !validText(row.IdempotencyKey) || row.CreatedAt.IsZero() || row.UpdatedAt.IsZero() || row.UpdatedAt.Before(row.CreatedAt) || !validJSON(row.RawPayload) {
		return quarantine[struct{}](ReasonInvalidSource)
	}
	return archive[struct{}](ReasonContactTagRequiresArchive)
}

// V1 segment SQL is never parsed or executed. The complete source record is
// retained only as encrypted archive history.
type SegmentRow struct {
	ID            int64
	SegmentCode   string
	DisplayName   string
	SQLQuery      string
	SQLParamsJSON json.RawMessage
	SQLDialect    string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func ConvertSegment(SegmentRow) Decision[struct{}] {
	return archive[struct{}](ReasonLegacySQLRequiresArchive)
}

func validText(value string) bool { return value == strings.TrimSpace(value) }

func validRequiredText(value string) bool { return value != "" && validText(value) }

func validJSON(value json.RawMessage) bool { return len(value) == 0 || json.Valid(value) }
