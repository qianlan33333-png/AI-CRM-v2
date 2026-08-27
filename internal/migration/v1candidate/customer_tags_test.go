package v1candidate

import (
	"encoding/json"
	"testing"
	"time"
)

func TestProfileArchivesWithoutTargetSchema(t *testing.T) {
	decision := ConvertProfile(ProfileFieldsRow{
		UnionID: "union-1", Source: "wecom", Industry: "education", IndustryDescription: "course", UpdatedBy: "staff-1", UpdatedAt: testTime,
	})
	assertDecision(t, decision, Archive, ReasonProfileRequiresArchive)
}

func TestWeComCatalogArchivesWithoutInventedTargetFacts(t *testing.T) {
	group := ConvertTagGroup(TagGroupRow{GroupID: "group-1", GroupName: "课程", GroupKey: "course", TagCount: 1, UpdatedAt: testTime})
	assertDecision(t, group, Archive, ReasonTagCatalogRequiresArchive)

	deletedAt := testTime
	tag := ConvertTag(TagRow{TagID: "tag-1", TagName: "已购", GroupID: "group-1", GroupName: "课程", DeletedAt: &deletedAt, RawPayload: json.RawMessage(`{"legacy":"payload"}`), UpdatedAt: testTime})
	assertDecision(t, tag, Archive, ReasonTagCatalogRequiresArchive)
}

func TestContactTagArchivesUntilTaggedByCrosswalkIsApproved(t *testing.T) {
	decision := ConvertContactTag(ContactTagRow{
		ID: 9, UnionID: "union-1", UserID: "staff-1", TagID: "tag-1", CreatedAt: testTime, UpdatedAt: testTime.Add(time.Minute),
	})
	assertDecision(t, decision, Archive, ReasonContactTagRequiresArchive)
}

func TestMalformedRowsQuarantine(t *testing.T) {
	profile := ConvertProfile(ProfileFieldsRow{UnionID: " union-1", UpdatedAt: testTime})
	assertDecision(t, profile, Quarantine, ReasonInvalidSource)

	tag := ConvertTag(TagRow{TagID: "tag-1", TagName: "已购", GroupID: "group-1", GroupName: "课程", RawPayload: json.RawMessage("not-json"), UpdatedAt: testTime})
	assertDecision(t, tag, Quarantine, ReasonInvalidSource)
}

func TestSegmentAlwaysArchivesLegacySQLWithoutParsing(t *testing.T) {
	decision := ConvertSegment(SegmentRow{SQLQuery: "SELECT * FROM customers", SQLParamsJSON: json.RawMessage("not-json")})
	assertDecision(t, decision, Archive, ReasonLegacySQLRequiresArchive)
}

func assertDecision[T any](t *testing.T, got Decision[T], disposition Disposition, reason string) {
	t.Helper()
	if got.Disposition != disposition || got.Reason != reason {
		t.Fatalf("decision = %#v, want disposition=%q reason=%q", got, disposition, reason)
	}
}

var testTime = time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
