package v1candidate

import (
	"encoding/json"
	"testing"
	"time"
)

func TestConvertProfileUsesExistingCustomerRootOnly(t *testing.T) {
	decision := ConvertProfile(profileRow(), CustomerRoots{"union-1": 101})
	if decision.Disposition != CanonicalCandidate || decision.Candidate == nil {
		t.Fatalf("profile decision = %#v", decision)
	}
	if got := *decision.Candidate; got.CustomerID != 101 || got.Source != "wecom" || got.Industry != "education" || got.Description != "course" {
		t.Fatalf("profile candidate = %#v", got)
	}

	missing := ConvertProfile(profileRow(), CustomerRoots{})
	if missing.Disposition != Quarantine || missing.Reason != ReasonCustomerRootUnresolved || missing.Candidate != nil {
		t.Fatalf("missing root = %#v", missing)
	}
}

func TestConvertProfileArchivesCombinedLegacyField(t *testing.T) {
	row := profileRow()
	row.NeedsBlockersFollowup = "need: demo; blocker: time"
	decision := ConvertProfile(row, CustomerRoots{"union-1": 101})
	if decision.Disposition != Archive || decision.Reason != ReasonProfileFieldRequiresSchema || decision.Candidate != nil {
		t.Fatalf("profile archive = %#v", decision)
	}
}

func TestConvertTagGroupAndTagRequireCleanResolvedCatalog(t *testing.T) {
	group := ConvertTagGroup(TagGroupRow{GroupID: "group-1", GroupName: "课程", GroupKey: "course", TagCount: 1, UpdatedAt: testTime})
	if group.Disposition != CanonicalCandidate || group.Candidate == nil || group.Candidate.Name != "课程" {
		t.Fatalf("group = %#v", group)
	}

	tag := ConvertTag(TagRow{TagID: "tag-1", TagName: "已购", GroupID: "group-1", GroupName: "课程", UpdatedAt: testTime}, TagGroups{"group-1": 201})
	if tag.Disposition != CanonicalCandidate || tag.Candidate == nil || tag.Candidate.TagGroupID != 201 || tag.Candidate.WeComTagID != "tag-1" {
		t.Fatalf("tag = %#v", tag)
	}

	missingGroup := ConvertTag(TagRow{TagID: "tag-1", TagName: "已购", GroupID: "missing", GroupName: "课程", UpdatedAt: testTime}, TagGroups{})
	if missingGroup.Disposition != Quarantine || missingGroup.Reason != ReasonTagGroupUnresolved {
		t.Fatalf("missing group = %#v", missingGroup)
	}

	deletedAt := testTime
	deleted := ConvertTag(TagRow{TagID: "tag-1", TagName: "已购", GroupID: "group-1", GroupName: "课程", DeletedAt: &deletedAt, UpdatedAt: testTime}, TagGroups{"group-1": 201})
	if deleted.Disposition != Archive || deleted.Reason != ReasonDeletedTag {
		t.Fatalf("deleted tag = %#v", deleted)
	}
}

func TestConvertRawPayloadArchivesInsteadOfDroppingIt(t *testing.T) {
	decision := ConvertTagGroup(TagGroupRow{GroupID: "group-1", GroupName: "课程", GroupKey: "course", TagCount: 1, RawPayload: json.RawMessage(`{"secret":"value"}`), UpdatedAt: testTime})
	if decision.Disposition != Archive || decision.Reason != ReasonRawPayloadRequiresArchive || decision.Candidate != nil {
		t.Fatalf("raw group = %#v", decision)
	}
}

func TestConvertContactTagRequiresAllDM01Crosswalks(t *testing.T) {
	row := ContactTagRow{ID: 9, UnionID: "union-1", UserID: "staff-1", TagID: "tag-1", CreatedAt: testTime, UpdatedAt: testTime.Add(time.Minute)}
	decision := ConvertContactTag(row, CustomerRoots{"union-1": 101}, Tags{"tag-1": 201}, Staff{"staff-1": 301})
	if decision.Disposition != CanonicalCandidate || decision.Candidate == nil {
		t.Fatalf("contact tag = %#v", decision)
	}
	if got := *decision.Candidate; got.CustomerID != 101 || got.TagID != 201 || got.TaggedByStaffID != 301 || got.TaggedAt != testTime {
		t.Fatalf("contact tag candidate = %#v", got)
	}

	for name, testCase := range map[string]struct {
		roots  CustomerRoots
		tags   Tags
		staff  Staff
		reason string
	}{
		"customer": {CustomerRoots{}, Tags{"tag-1": 201}, Staff{"staff-1": 301}, ReasonCustomerRootUnresolved},
		"tag":      {CustomerRoots{"union-1": 101}, Tags{}, Staff{"staff-1": 301}, ReasonTagUnresolved},
		"staff":    {CustomerRoots{"union-1": 101}, Tags{"tag-1": 201}, Staff{}, ReasonStaffUnresolved},
	} {
		t.Run(name, func(t *testing.T) {
			got := ConvertContactTag(row, testCase.roots, testCase.tags, testCase.staff)
			if got.Disposition != Quarantine || got.Reason != testCase.reason || got.Candidate != nil {
				t.Fatalf("decision = %#v", got)
			}
		})
	}
}

func TestConvertContactTagArchivesUnmappedLegacyFields(t *testing.T) {
	row := ContactTagRow{ID: 9, UnionID: "union-1", UserID: "staff-1", TagID: "tag-1", Source: "questionnaire", CreatedAt: testTime, UpdatedAt: testTime}
	decision := ConvertContactTag(row, CustomerRoots{"union-1": 101}, Tags{"tag-1": 201}, Staff{"staff-1": 301})
	if decision.Disposition != Archive || decision.Reason != ReasonUnmappedLegacyFields || decision.Candidate != nil {
		t.Fatalf("unmapped contact tag = %#v", decision)
	}
}

func TestConvertersQuarantineMalformedSource(t *testing.T) {
	group := ConvertTagGroup(TagGroupRow{GroupID: " group-1", GroupName: "课程", UpdatedAt: testTime})
	if group.Disposition != Quarantine || group.Reason != ReasonInvalidSource || group.Candidate != nil {
		t.Fatalf("malformed group = %#v", group)
	}
	segment := ConvertSegment(SegmentRow{ID: 11, SegmentCode: "old-audience", DisplayName: "旧人群", SQLDialect: "postgres", SQLParamsJSON: json.RawMessage("not-json"), CreatedAt: testTime, UpdatedAt: testTime})
	if segment.Disposition != Quarantine || segment.Reason != ReasonInvalidSource || segment.Candidate != nil {
		t.Fatalf("malformed segment = %#v", segment)
	}
}

func TestConvertSegmentNeverExecutesLegacySQL(t *testing.T) {
	row := SegmentRow{ID: 11, SegmentCode: "old-audience", DisplayName: "旧人群", SQLQuery: "SELECT * FROM customers", SQLDialect: "postgres", CreatedAt: testTime, UpdatedAt: testTime}
	decision := ConvertSegment(row)
	if decision.Disposition != Archive || decision.Reason != ReasonLegacySQLRequiresArchive || decision.Candidate != nil {
		t.Fatalf("legacy SQL decision = %#v", decision)
	}

	deferred := ConvertSegment(SegmentRow{ID: 11, SegmentCode: "old-audience", DisplayName: "旧人群", SQLDialect: "postgres", CreatedAt: testTime, UpdatedAt: testTime})
	if deferred.Disposition != Quarantine || deferred.Reason != ReasonSegmentDefinitionDeferred || deferred.Candidate != nil {
		t.Fatalf("deferred segment = %#v", deferred)
	}
}

var testTime = time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

func profileRow() ProfileFieldsRow {
	return ProfileFieldsRow{UnionID: "union-1", Source: "wecom", Industry: "education", IndustryDescription: "course", UpdatedBy: "staff-1", CreatedAt: testTime, UpdatedAt: testTime.Add(time.Minute)}
}
