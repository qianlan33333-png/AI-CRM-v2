// Package v1candidate makes fail-closed, side-effect-free decisions for V1
// customer profile and tag/segment rows. It deliberately has no SQL, target
// repository, Provider, queue, or execution dependency.
package v1candidate

import (
	"encoding/json"
	"strings"
	"time"
)

type Disposition string

const (
	CanonicalCandidate Disposition = "canonical_candidate"
	Quarantine         Disposition = "quarantine"
	Archive            Disposition = "archive"
)

const (
	ReasonInvalidSource              = "invalid_source"
	ReasonCustomerRootUnresolved     = "customer_root_unresolved"
	ReasonTagGroupUnresolved         = "tag_group_unresolved"
	ReasonTagUnresolved              = "tag_unresolved"
	ReasonStaffUnresolved            = "staff_unresolved"
	ReasonProfileFieldRequiresSchema = "profile_field_requires_target_schema"
	ReasonRawPayloadRequiresArchive  = "raw_payload_requires_archive"
	ReasonUnmappedLegacyFields       = "unmapped_legacy_fields_requires_archive"
	ReasonDeletedTag                 = "deleted_tag_requires_archive"
	ReasonLegacySQLRequiresArchive   = "legacy_sql_requires_archive"
	ReasonSegmentDefinitionDeferred  = "segment_definition_requires_approved_dsl"
)

// Decision holds a proposed canonical fact only when Disposition is
// CanonicalCandidate. Archive and quarantine decisions never carry source
// payloads, so this package cannot accidentally turn them into active facts.
type Decision[T any] struct {
	Disposition Disposition
	Reason      string
	Candidate   *T
}

type CustomerRoots map[string]int64
type TagGroups map[string]int64
type Tags map[string]int64
type Staff map[string]int64

type ProfileFieldsRow struct {
	UnionID               string
	Source                string
	Industry              string
	IndustryDescription   string
	NeedsBlockersFollowup string
	UpdatedBy             string
	UpdatedAt             time.Time
	CreatedAt             time.Time
}

// ProfileCandidate is only a crosswalk proposal. It must be applied later by
// a Contact-owned importer after target-field approval; this package never
// updates a customer or sidebar profile.
type ProfileCandidate struct {
	CustomerID  int64
	Source      string
	Industry    string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func ConvertProfile(row ProfileFieldsRow, roots CustomerRoots) Decision[ProfileCandidate] {
	if !validRequiredText(row.UnionID) || !validText(row.Source) || !validText(row.Industry) || !validText(row.IndustryDescription) || !validText(row.UpdatedBy) || row.CreatedAt.IsZero() || row.UpdatedAt.IsZero() || row.UpdatedAt.Before(row.CreatedAt) {
		return quarantine[ProfileCandidate](ReasonInvalidSource)
	}
	customerID, found := roots[row.UnionID]
	if !found || customerID < 1 {
		return quarantine[ProfileCandidate](ReasonCustomerRootUnresolved)
	}
	if row.NeedsBlockersFollowup != "" {
		// V1 mixes needs, blockers, and follow-up in one field. There is no
		// approved lossless mapping to the V2 needs/pain-points split.
		return archive[ProfileCandidate](ReasonProfileFieldRequiresSchema)
	}
	return canonical(ProfileCandidate{CustomerID: customerID, Source: row.Source, Industry: row.Industry, Description: row.IndustryDescription, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt})
}

type TagGroupRow struct {
	GroupID    string
	GroupName  string
	GroupKey   string
	TagCount   int
	RawPayload json.RawMessage
	SyncedAt   *time.Time
	UpdatedAt  time.Time
}

type TagGroupCandidate struct {
	LegacyGroupID string
	Name          string
}

func ConvertTagGroup(row TagGroupRow) Decision[TagGroupCandidate] {
	if !validRequiredText(row.GroupID) || !validRequiredText(row.GroupName) || !validText(row.GroupKey) || row.TagCount < 0 || row.UpdatedAt.IsZero() || (row.SyncedAt != nil && row.SyncedAt.IsZero()) || !validJSON(row.RawPayload) {
		return quarantine[TagGroupCandidate](ReasonInvalidSource)
	}
	if len(row.RawPayload) != 0 {
		return archive[TagGroupCandidate](ReasonRawPayloadRequiresArchive)
	}
	return canonical(TagGroupCandidate{LegacyGroupID: row.GroupID, Name: row.GroupName})
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

type TagCandidate struct {
	LegacyTagID   string
	Name          string
	TagGroupID    int64
	WeComTagID    string
	LegacySortKey int
}

func ConvertTag(row TagRow, groups TagGroups) Decision[TagCandidate] {
	if !validRequiredText(row.TagID) || !validRequiredText(row.TagName) || !validRequiredText(row.GroupID) || !validText(row.GroupName) || row.OrderIndex < 0 || row.UpdatedAt.IsZero() || (row.DeletedAt != nil && row.DeletedAt.IsZero()) || (row.SyncedAt != nil && row.SyncedAt.IsZero()) || !validJSON(row.RawPayload) {
		return quarantine[TagCandidate](ReasonInvalidSource)
	}
	if row.DeletedAt != nil {
		return archive[TagCandidate](ReasonDeletedTag)
	}
	if len(row.RawPayload) != 0 {
		return archive[TagCandidate](ReasonRawPayloadRequiresArchive)
	}
	groupID, found := groups[row.GroupID]
	if !found || groupID < 1 {
		return quarantine[TagCandidate](ReasonTagGroupUnresolved)
	}
	return canonical(TagCandidate{LegacyTagID: row.TagID, Name: row.TagName, TagGroupID: groupID, WeComTagID: row.TagID, LegacySortKey: row.OrderIndex})
}

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

type CustomerTagCandidate struct {
	LegacyID        int64
	CustomerID      int64
	TagID           int64
	TaggedByStaffID int64
	TaggedAt        time.Time
}

func ConvertContactTag(row ContactTagRow, roots CustomerRoots, tags Tags, staff Staff) Decision[CustomerTagCandidate] {
	if row.ID < 1 || !validRequiredText(row.UnionID) || !validRequiredText(row.UserID) || !validRequiredText(row.TagID) || !validText(row.TagName) || !validText(row.Source) || !validText(row.QuestionnaireID) || !validText(row.SubmissionID) || !validText(row.IdempotencyKey) || row.CreatedAt.IsZero() || row.UpdatedAt.IsZero() || row.UpdatedAt.Before(row.CreatedAt) || !validJSON(row.RawPayload) {
		return quarantine[CustomerTagCandidate](ReasonInvalidSource)
	}
	if row.TagName != "" || row.Source != "" || row.QuestionnaireID != "" || row.SubmissionID != "" || row.IdempotencyKey != "" || len(row.RawPayload) != 0 {
		return archive[CustomerTagCandidate](ReasonUnmappedLegacyFields)
	}
	customerID, found := roots[row.UnionID]
	if !found || customerID < 1 {
		return quarantine[CustomerTagCandidate](ReasonCustomerRootUnresolved)
	}
	tagID, found := tags[row.TagID]
	if !found || tagID < 1 {
		return quarantine[CustomerTagCandidate](ReasonTagUnresolved)
	}
	staffID, found := staff[row.UserID]
	if !found || staffID < 1 {
		return quarantine[CustomerTagCandidate](ReasonStaffUnresolved)
	}
	return canonical(CustomerTagCandidate{LegacyID: row.ID, CustomerID: customerID, TagID: tagID, TaggedByStaffID: staffID, TaggedAt: row.CreatedAt})
}

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

type SegmentCandidate struct {
	LegacyID  int64
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func ConvertSegment(row SegmentRow) Decision[SegmentCandidate] {
	if row.ID < 1 || !validRequiredText(row.SegmentCode) || !validRequiredText(row.DisplayName) || !validRequiredText(row.SQLDialect) || row.CreatedAt.IsZero() || row.UpdatedAt.IsZero() || row.UpdatedAt.Before(row.CreatedAt) || !validJSON(row.SQLParamsJSON) {
		return quarantine[SegmentCandidate](ReasonInvalidSource)
	}
	// Legacy SQL and its parameters are intentionally not parsed or executed.
	if row.SQLQuery != "" || len(row.SQLParamsJSON) != 0 {
		return archive[SegmentCandidate](ReasonLegacySQLRequiresArchive)
	}
	return quarantine[SegmentCandidate](ReasonSegmentDefinitionDeferred)
}

func canonical[T any](candidate T) Decision[T] {
	return Decision[T]{Disposition: CanonicalCandidate, Candidate: &candidate}
}

func quarantine[T any](reason string) Decision[T] {
	return Decision[T]{Disposition: Quarantine, Reason: reason}
}

func archive[T any](reason string) Decision[T] {
	return Decision[T]{Disposition: Archive, Reason: reason}
}

func validText(value string) bool { return value == strings.TrimSpace(value) }

func validRequiredText(value string) bool { return value != "" && validText(value) }

func validJSON(value json.RawMessage) bool { return len(value) == 0 || json.Valid(value) }
