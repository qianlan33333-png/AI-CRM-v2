// Package v1groupops classifies V1 Group Ops definitions as non-executable
// historical facts. It has no target store, command, queue, or Provider code.
package v1groupops

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

const (
	GroupChatsTableID     = "public/group_chats"
	GroupSnapshotsTableID = "public/wecom_group_chat_snapshots"
	PlansTableID          = "public/automation_group_ops_plans"
	PlanGroupsTableID     = "public/automation_group_ops_plan_groups"
	PlanNodesTableID      = "public/automation_group_ops_plan_nodes"
)

type Disposition string

const (
	DispositionCandidate  Disposition = "historical_candidate"
	DispositionQuarantine Disposition = "quarantine"
)

// Fact values deliberately keep sensitive V1 source material out of JSON.
// The immutable archive remains the evidence for omitted payloads and secrets.
type GroupChatFact struct {
	SourceID       int64
	ChatID         string  `json:"-"`
	GroupName      *string `json:"-"`
	OwnerUserID    *string `json:"-"`
	MemberCount    int32
	OriginalStatus string
	UpdatedAt      time.Time
}

type GroupSnapshotFact struct {
	ChatID              string `json:"-"`
	GroupName           string `json:"-"`
	OwnerUserID         string `json:"-"`
	OwnerName           string `json:"-"`
	InternalMemberCount int32
	ExternalMemberCount int32
	OriginalStatus      string
	SyncedAt            time.Time
}

type PlanFact struct {
	SourceID          int64
	PlanCode          string `json:"-"`
	PlanName          string `json:"-"`
	PlanType          string `json:"-"`
	SourceOwnerUserID string `json:"-"`
	OriginalStatus    string
	Archived          bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ArchivedAt        *time.Time
}

type PlanGroupFact struct {
	SourceID                    int64
	PlanSourceID                int64
	ChatID                      string `json:"-"`
	GroupName                   string `json:"-"`
	OwnerUserID                 string `json:"-"`
	InternalMemberCountSnapshot int32
	ExternalMemberCountSnapshot int32
	OriginalStatus              string
	CreatedAt                   time.Time
	RemovedAt                   *time.Time
}

type PlanNodeFact struct {
	SourceID       int64
	PlanSourceID   int64
	DayIndex       int32
	TriggerTime    string `json:"-"`
	SortOrder      int32
	OriginalStatus string
	ContentPackage json.RawMessage `json:"-"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type GroupChatResult struct {
	Disposition Disposition
	Reason      string
	Fact        *GroupChatFact
}

type GroupSnapshotResult struct {
	Disposition Disposition
	Reason      string
	Fact        *GroupSnapshotFact
}

type PlanResult struct {
	Disposition Disposition
	Reason      string
	Fact        *PlanFact
}

type PlanGroupResult struct {
	Disposition Disposition
	Reason      string
	Fact        *PlanGroupFact
}

type PlanNodeResult struct {
	Disposition Disposition
	Reason      string
	Fact        *PlanNodeFact
}

// History intentionally preserves directory sources separately. The two
// sources have overlapping chat references but no safe merge authority here.
type History struct {
	GroupChats []GroupChatResult
	Snapshots  []GroupSnapshotResult
	Plans      []PlanResult
	PlanGroups []PlanGroupResult
	PlanNodes  []PlanNodeResult
}

type groupChatJSON struct {
	ID          int64     `json:"id"`
	ChatID      string    `json:"chat_id"`
	GroupName   *string   `json:"group_name"`
	OwnerUserID *string   `json:"owner_userid"`
	MemberCount int32     `json:"member_count"`
	Status      string    `json:"status"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type groupSnapshotJSON struct {
	ChatID              string    `json:"chat_id"`
	GroupName           string    `json:"group_name"`
	OwnerUserID         string    `json:"owner_userid"`
	OwnerName           string    `json:"owner_name"`
	InternalMemberCount int32     `json:"internal_member_count"`
	ExternalMemberCount int32     `json:"external_member_count"`
	SyncedAt            time.Time `json:"synced_at"`
	Status              string    `json:"status"`
}

type planJSON struct {
	ID          int64      `json:"id"`
	PlanCode    string     `json:"plan_code"`
	PlanName    string     `json:"plan_name"`
	PlanType    string     `json:"plan_type"`
	OwnerUserID string     `json:"owner_userid"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ArchivedAt  *time.Time `json:"archived_at"`
}

type planGroupJSON struct {
	ID                  int64      `json:"id"`
	PlanID              int64      `json:"plan_id"`
	ChatID              string     `json:"chat_id"`
	GroupNameSnapshot   string     `json:"group_name_snapshot"`
	OwnerUserIDSnapshot string     `json:"owner_userid_snapshot"`
	InternalMemberCount int32      `json:"internal_member_count_snapshot"`
	ExternalMemberCount int32      `json:"external_member_count_snapshot"`
	Status              string     `json:"status"`
	CreatedAt           time.Time  `json:"created_at"`
	RemovedAt           *time.Time `json:"removed_at"`
}

type planNodeJSON struct {
	ID               int64           `json:"id"`
	PlanID           int64           `json:"plan_id"`
	DayIndex         int32           `json:"day_index"`
	TriggerTimeLabel string          `json:"trigger_time_label"`
	SortOrder        int32           `json:"sort_order"`
	Status           string          `json:"status"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	ContentPackage   json.RawMessage `json:"content_package_json"`
}

func AdaptHistory(groupChats, snapshots, plans, planGroups, planNodes []json.RawMessage) History {
	history := History{
		GroupChats: make([]GroupChatResult, len(groupChats)), Snapshots: make([]GroupSnapshotResult, len(snapshots)),
		Plans: make([]PlanResult, len(plans)), PlanGroups: make([]PlanGroupResult, len(planGroups)), PlanNodes: make([]PlanNodeResult, len(planNodes)),
	}
	for index, value := range groupChats {
		history.GroupChats[index] = adaptGroupChat(value)
	}
	for index, value := range snapshots {
		history.Snapshots[index] = adaptGroupSnapshot(value)
	}
	for index, value := range plans {
		history.Plans[index] = adaptPlan(value)
	}
	plansByID := uniquePlans(history.Plans)
	for index, value := range planGroups {
		history.PlanGroups[index] = adaptPlanGroup(value, plansByID)
	}
	for index, value := range planNodes {
		history.PlanNodes[index] = adaptPlanNode(value, plansByID)
	}
	return history
}

func adaptGroupChat(value json.RawMessage) GroupChatResult {
	var source groupChatJSON
	if !decode(value, &source, "id", "chat_id", "member_count", "status", "updated_at") || source.ID < 1 || !text(source.ChatID) || source.MemberCount < 0 || !text(source.Status) || source.UpdatedAt.IsZero() {
		return quarantineGroupChat("group_chat_shape_invalid")
	}
	return GroupChatResult{Disposition: DispositionCandidate, Fact: &GroupChatFact{SourceID: source.ID, ChatID: source.ChatID,
		GroupName: cloneString(source.GroupName), OwnerUserID: cloneString(source.OwnerUserID),
		MemberCount: source.MemberCount, OriginalStatus: source.Status, UpdatedAt: source.UpdatedAt}}
}

func adaptGroupSnapshot(value json.RawMessage) GroupSnapshotResult {
	var source groupSnapshotJSON
	if !decode(value, &source, "chat_id", "group_name", "owner_userid", "owner_name", "internal_member_count", "external_member_count", "synced_at", "status") || !text(source.ChatID) || !text(source.GroupName) || !text(source.OwnerUserID) || !text(source.OwnerName) || source.InternalMemberCount < 0 || source.ExternalMemberCount < 0 || !text(source.Status) || source.SyncedAt.IsZero() {
		return GroupSnapshotResult{Disposition: DispositionQuarantine, Reason: "group_snapshot_shape_invalid"}
	}
	return GroupSnapshotResult{Disposition: DispositionCandidate, Fact: &GroupSnapshotFact{ChatID: source.ChatID, GroupName: source.GroupName, OwnerUserID: source.OwnerUserID, OwnerName: source.OwnerName,
		InternalMemberCount: source.InternalMemberCount, ExternalMemberCount: source.ExternalMemberCount, OriginalStatus: source.Status, SyncedAt: source.SyncedAt}}
}

func adaptPlan(value json.RawMessage) PlanResult {
	var source planJSON
	if !decode(value, &source, "id", "plan_code", "plan_name", "plan_type", "owner_userid", "status", "created_at", "updated_at") || source.ID < 1 || !text(source.PlanCode) || !text(source.PlanName) || !text(source.PlanType) || !text(source.OwnerUserID) || !text(source.Status) || source.CreatedAt.IsZero() || source.UpdatedAt.IsZero() || source.UpdatedAt.Before(source.CreatedAt) || invalidOptionalTime(source.ArchivedAt) {
		return PlanResult{Disposition: DispositionQuarantine, Reason: "group_ops_plan_shape_invalid"}
	}
	// The webhook key, signature and action type have no safe V2 historical
	// equivalent. The fact is always archived and cannot be executed.
	return PlanResult{Disposition: DispositionCandidate, Fact: &PlanFact{SourceID: source.ID, PlanCode: source.PlanCode, PlanName: source.PlanName, PlanType: source.PlanType,
		// This is an unverified V1 source reference, never a V2 owner mapping.
		SourceOwnerUserID: source.OwnerUserID, OriginalStatus: source.Status, Archived: true, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt, ArchivedAt: cloneTime(source.ArchivedAt)}}
}

func adaptPlanGroup(value json.RawMessage, plans map[int64]PlanFact) PlanGroupResult {
	var source planGroupJSON
	if !decode(value, &source, "id", "plan_id", "chat_id", "group_name_snapshot", "owner_userid_snapshot", "internal_member_count_snapshot", "external_member_count_snapshot", "status", "created_at") || source.ID < 1 || source.PlanID < 1 || !text(source.ChatID) || !text(source.GroupNameSnapshot) || !text(source.OwnerUserIDSnapshot) || source.InternalMemberCount < 0 || source.ExternalMemberCount < 0 || !text(source.Status) || source.CreatedAt.IsZero() || invalidOptionalTime(source.RemovedAt) {
		return PlanGroupResult{Disposition: DispositionQuarantine, Reason: "group_ops_plan_group_shape_invalid"}
	}
	if source.RemovedAt != nil && source.RemovedAt.Before(source.CreatedAt) {
		return PlanGroupResult{Disposition: DispositionQuarantine, Reason: "group_ops_plan_group_time_invalid"}
	}
	if _, found := plans[source.PlanID]; !found {
		return PlanGroupResult{Disposition: DispositionQuarantine, Reason: "group_ops_plan_group_plan_unresolved"}
	}
	return PlanGroupResult{Disposition: DispositionCandidate, Fact: &PlanGroupFact{SourceID: source.ID, PlanSourceID: source.PlanID, ChatID: source.ChatID,
		GroupName: source.GroupNameSnapshot, OwnerUserID: source.OwnerUserIDSnapshot, InternalMemberCountSnapshot: source.InternalMemberCount,
		ExternalMemberCountSnapshot: source.ExternalMemberCount, OriginalStatus: source.Status, CreatedAt: source.CreatedAt, RemovedAt: cloneTime(source.RemovedAt)}}
}

func adaptPlanNode(value json.RawMessage, plans map[int64]PlanFact) PlanNodeResult {
	var source planNodeJSON
	if !decode(value, &source, "id", "plan_id", "day_index", "trigger_time_label", "sort_order", "status", "created_at", "updated_at", "content_package_json") || source.ID < 1 || source.PlanID < 1 || source.DayIndex < 0 || source.SortOrder < 0 || !text(source.TriggerTimeLabel) || !text(source.Status) || source.CreatedAt.IsZero() || source.UpdatedAt.IsZero() || source.UpdatedAt.Before(source.CreatedAt) || !jsonObject(source.ContentPackage) {
		return PlanNodeResult{Disposition: DispositionQuarantine, Reason: "group_ops_plan_node_shape_invalid"}
	}
	if _, found := plans[source.PlanID]; !found {
		return PlanNodeResult{Disposition: DispositionQuarantine, Reason: "group_ops_plan_node_plan_unresolved"}
	}
	return PlanNodeResult{Disposition: DispositionCandidate, Fact: &PlanNodeFact{SourceID: source.ID, PlanSourceID: source.PlanID, DayIndex: source.DayIndex,
		TriggerTime: source.TriggerTimeLabel, SortOrder: source.SortOrder, OriginalStatus: source.Status, ContentPackage: append(json.RawMessage(nil), source.ContentPackage...), CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt}}
}

func uniquePlans(values []PlanResult) map[int64]PlanFact {
	result, duplicates := make(map[int64]PlanFact, len(values)), map[int64]bool{}
	for index := range values {
		value := &values[index]
		if value.Disposition != DispositionCandidate || value.Fact == nil {
			continue
		}
		if _, found := result[value.Fact.SourceID]; found {
			duplicates[value.Fact.SourceID] = true
			delete(result, value.Fact.SourceID)
		} else if !duplicates[value.Fact.SourceID] {
			result[value.Fact.SourceID] = *value.Fact
		}
	}
	for index := range values {
		value := &values[index]
		if value.Fact != nil && duplicates[value.Fact.SourceID] {
			*value = PlanResult{Disposition: DispositionQuarantine, Reason: "group_ops_plan_source_ambiguous"}
		}
	}
	return result
}

func quarantineGroupChat(reason string) GroupChatResult {
	return GroupChatResult{Disposition: DispositionQuarantine, Reason: reason}
}

func text(value string) bool { return value != "" && value == strings.TrimSpace(value) }

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func invalidOptionalTime(value *time.Time) bool { return value != nil && value.IsZero() }

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func jsonObject(value json.RawMessage) bool {
	return json.Valid(value) && len(bytes.TrimSpace(value)) > 1 && bytes.HasPrefix(bytes.TrimSpace(value), []byte("{")) && bytes.HasSuffix(bytes.TrimSpace(value), []byte("}"))
}

func decode(value json.RawMessage, target any, required ...string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(value, &fields) != nil || fields == nil {
		return false
	}
	for _, field := range required {
		raw, found := fields[field]
		if !found || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return false
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	if decoder.Decode(target) != nil {
		return false
	}
	var extra any
	return errors.Is(decoder.Decode(&extra), io.EOF)
}
