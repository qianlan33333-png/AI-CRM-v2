// Package v1campaignhistory parses frozen V1 Campaign history only. It has
// no persistence, current Campaign writer, event, queue or Provider dependency.
package v1campaignhistory

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
)

const (
	CampaignTable = "public/campaigns"
	SegmentTable  = "public/campaign_segments"
	MemberTable   = "public/campaign_members"
)

type Disposition string

const (
	Candidate Disposition = "historical_candidate"
	Pending   Disposition = "pending"
	Invalid   Disposition = "invalid"
)

// All IDs are source IDs. None identifies a V2 Campaign, customer or segment.
// Source references are private migration inputs, excluded from JSON output.
type CampaignSource struct {
	OwnerUserID, CreatedByAgent, CreatedBySession, ApprovedBy, TraceID string
}

type CampaignFact struct {
	SourceID     int64          `json:"id"`
	Code         string         `json:"campaign_code"`
	DisplayName  string         `json:"display_name"`
	Intent       string         `json:"intent"`
	AnchorMode   string         `json:"anchor_mode"`
	AnchorDate   string         `json:"anchor_date"`
	ReviewStatus string         `json:"review_status"`
	RunStatus    string         `json:"run_status"`
	ApprovedAt   *time.Time     `json:"approved_at"`
	StartedAt    *time.Time     `json:"started_at"`
	FinishedAt   *time.Time     `json:"finished_at"`
	PausedAt     *time.Time     `json:"paused_at"`
	PausedReason string         `json:"paused_reason"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	Source       CampaignSource `json:"-"`
}

type SegmentFact struct {
	SourceID         int64     `json:"id"`
	CampaignSourceID int64     `json:"campaign_id"`
	SegmentSourceID  int64     `json:"segment_id"`
	Code             string    `json:"segment_code"`
	Priority         int32     `json:"priority"`
	Label            string    `json:"label"`
	CreatedAt        time.Time `json:"created_at"`
}

type MemberSource struct {
	UnionID, TraceID, LastErrorText string
}

type MemberFact struct {
	SourceID                int64        `json:"id"`
	CampaignSourceID        int64        `json:"campaign_id"`
	CampaignSegmentSourceID int64        `json:"campaign_segment_id"`
	SegmentSourceID         int64        `json:"segment_id"`
	MemberSourceID          int64        `json:"member_id"`
	JoinedAt                time.Time    `json:"joined_at"`
	AnchorDate              string       `json:"anchor_date"`
	CurrentStepIndex        int32        `json:"current_step_index"`
	NextDueAt               *time.Time   `json:"next_due_at"`
	Status                  string       `json:"status"`
	StopReason              string       `json:"stop_reason"`
	LastStepSentAt          *time.Time   `json:"last_step_sent_at"`
	RetryCount              int32        `json:"retry_count"`
	CreatedAt               time.Time    `json:"created_at"`
	UpdatedAt               time.Time    `json:"updated_at"`
	Source                  MemberSource `json:"-"`
}

type Result[T any] struct {
	Disposition Disposition
	Reason      string
	Fact        *T
}

type History struct {
	Campaigns []Result[CampaignFact]
	Segments  []Result[SegmentFact]
	Members   []Result[MemberFact]
}

const campaignRequired = "id campaign_code display_name intent anchor_mode anchor_date review_status run_status created_by_agent created_by_session trace_id owner_userid approved_by paused_reason created_at updated_at"
const campaignNullable = "approved_at started_at finished_at paused_at"
const segmentRequired = "id campaign_id segment_id segment_code priority label created_at"
const memberRequired = "id campaign_id campaign_segment_id segment_id member_id joined_at anchor_date current_step_index status stop_reason last_error_text retry_count trace_id created_at updated_at unionid"
const memberNullable = "next_due_at last_step_sent_at"

// AdaptHistory preserves input order/count and original statuses (including
// active) without reconstructing runtime. Parent checks use source IDs only;
// repeated codes and repeated members remain distinct source rows.
func AdaptHistory(campaigns, segments, members []json.RawMessage) History {
	h := History{Campaigns: make([]Result[CampaignFact], len(campaigns)), Segments: make([]Result[SegmentFact], len(segments)), Members: make([]Result[MemberFact], len(members))}
	for i, payload := range campaigns {
		h.Campaigns[i] = AdaptCampaign(payload)
	}
	parents := indexFacts(h.Campaigns, func(f *CampaignFact) int64 { return f.SourceID })
	for i, payload := range segments {
		h.Segments[i] = AdaptSegment(payload)
	}
	segmentParents := indexFacts(h.Segments, func(f *SegmentFact) int64 { return f.SourceID })
	for i, row := range h.Segments {
		if row.Fact != nil && parents[row.Fact.CampaignSourceID] == nil {
			delete(segmentParents, row.Fact.SourceID)
			h.Segments[i] = Result[SegmentFact]{Disposition: Pending, Reason: "segment_campaign_unresolved"}
		}
	}
	for i, payload := range members {
		h.Members[i] = AdaptMember(payload)
	}
	indexFacts(h.Members, func(f *MemberFact) int64 { return f.SourceID })
	for i, row := range h.Members {
		if row.Fact == nil {
			continue
		}
		f := row.Fact
		reason := ""
		if parents[f.CampaignSourceID] == nil {
			reason = "member_campaign_unresolved"
		} else if s := segmentParents[f.CampaignSegmentSourceID]; s == nil {
			reason = "member_campaign_segment_unresolved"
		} else if s.CampaignSourceID != f.CampaignSourceID || s.SegmentSourceID != f.SegmentSourceID {
			reason = "member_campaign_segment_mismatch"
		}
		if reason != "" {
			h.Members[i] = Result[MemberFact]{Disposition: Pending, Reason: reason}
		}
	}
	return h
}

func AdaptCampaign(payload json.RawMessage) Result[CampaignFact] {
	var source struct {
		CampaignFact
		OwnerUserID      string `json:"owner_userid"`
		CreatedByAgent   string `json:"created_by_agent"`
		CreatedBySession string `json:"created_by_session"`
		ApprovedBy       string `json:"approved_by"`
		TraceID          string `json:"trace_id"`
	}
	if !decode(payload, &source, campaignRequired, campaignNullable) {
		return Result[CampaignFact]{Disposition: Invalid, Reason: "campaign_json_invalid"}
	}
	f := source.CampaignFact
	if f.SourceID < 1 || !timesPresent(f.CreatedAt, f.UpdatedAt) || !optionalTimesPresent(f.ApprovedAt, f.StartedAt, f.FinishedAt, f.PausedAt) {
		return Result[CampaignFact]{Disposition: Invalid, Reason: "campaign_fact_invalid"}
	}
	f.Source = CampaignSource{source.OwnerUserID, source.CreatedByAgent, source.CreatedBySession, source.ApprovedBy, source.TraceID}
	return Result[CampaignFact]{Disposition: Candidate, Fact: &f}
}

// AdaptSegment checks row shape only; AdaptHistory verifies the source parent.
func AdaptSegment(payload json.RawMessage) Result[SegmentFact] {
	var f SegmentFact
	if !decode(payload, &f, segmentRequired, "") {
		return Result[SegmentFact]{Disposition: Invalid, Reason: "segment_json_invalid"}
	}
	if f.SourceID < 1 || f.CampaignSourceID < 1 || !timesPresent(f.CreatedAt) {
		return Result[SegmentFact]{Disposition: Invalid, Reason: "segment_fact_invalid"}
	}
	return Result[SegmentFact]{Disposition: Candidate, Fact: &f}
}

// AdaptMember checks row shape only; AdaptHistory verifies source parents.
// Neither function establishes a V2 customer identity.
func AdaptMember(payload json.RawMessage) Result[MemberFact] {
	var source struct {
		MemberFact
		UnionID       string `json:"unionid"`
		TraceID       string `json:"trace_id"`
		LastErrorText string `json:"last_error_text"`
	}
	if !decode(payload, &source, memberRequired, memberNullable) {
		return Result[MemberFact]{Disposition: Invalid, Reason: "member_json_invalid"}
	}
	f := source.MemberFact
	if f.SourceID < 1 || f.CampaignSourceID < 1 || f.CampaignSegmentSourceID < 1 || !timesPresent(f.JoinedAt, f.CreatedAt, f.UpdatedAt) || !optionalTimesPresent(f.NextDueAt, f.LastStepSentAt) {
		return Result[MemberFact]{Disposition: Invalid, Reason: "member_fact_invalid"}
	}
	f.Source = MemberSource{source.UnionID, source.TraceID, source.LastErrorText}
	return Result[MemberFact]{Disposition: Candidate, Fact: &f}
}

func decode(payload json.RawMessage, target any, required, nullable string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(payload, &fields) != nil || fields == nil {
		return false
	}
	for _, name := range strings.Fields(required) {
		if value, found := fields[name]; !found || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return false
		}
	}
	for _, name := range strings.Fields(nullable) {
		if _, found := fields[name]; !found {
			return false
		}
	}
	return json.Unmarshal(payload, target) == nil
}

// Duplicate source IDs are not selected arbitrarily, even for identical rows.
func indexFacts[T any](rows []Result[T], id func(*T) int64) map[int64]*T {
	counts := map[int64]int{}
	for _, row := range rows {
		if row.Fact != nil {
			counts[id(row.Fact)]++
		}
	}
	index := map[int64]*T{}
	for i, row := range rows {
		if row.Fact != nil {
			key := id(row.Fact)
			if counts[key] != 1 {
				rows[i] = Result[T]{Disposition: Invalid, Reason: "duplicate_source_id"}
			} else {
				index[key] = row.Fact
			}
		}
	}
	return index
}

func timesPresent(values ...time.Time) bool {
	for _, value := range values {
		if value.IsZero() {
			return false
		}
	}
	return true
}

func optionalTimesPresent(values ...*time.Time) bool {
	for _, value := range values {
		if value != nil && value.IsZero() {
			return false
		}
	}
	return true
}
