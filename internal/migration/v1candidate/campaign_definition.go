// Package v1candidate makes fail-closed, side-effect-free V1 migration
// decisions. It has no SQL, target repository, Provider, queue, or command
// dependency.
package v1candidate

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
)

type Disposition string

const (
	CanonicalCandidate Disposition = "canonical_candidate"
	Quarantine         Disposition = "quarantine"
	Archive            Disposition = "archive"
)

const (
	ReasonInvalidSource               = "invalid_source"
	ReasonActorUnresolved             = "campaign_actor_unresolved"
	ReasonRuntimeRequiresArchive      = "campaign_runtime_requires_archive"
	ReasonStepCampaignUnresolved      = "campaign_step_campaign_fk_unresolved"
	ReasonStepOrderInvalid            = "campaign_step_order_invalid"
	ReasonMultipleSegments            = "campaign_segments_require_archive"
	ReasonScheduleRequiresArchive     = "campaign_step_schedule_requires_archive"
	ReasonRuntimeTableRequiresArchive = "campaign_runtime_table_requires_archive"
)

type Decision[T any] struct {
	Disposition Disposition
	Reason      string
	Candidate   *T
}

// ActorIDs is the approved V1 owner_userid -> V2 actor mapping. No source
// actor is created or guessed by this package.
type ActorIDs map[string]int64

type CampaignRow struct {
	ID           int64
	CampaignCode string
	DisplayName  string
	ReviewStatus string
	RunStatus    string
	OwnerUserID  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type CampaignStepRow struct {
	ID                int64
	CampaignID        int64
	CampaignSegmentID int64
	StepIndex         int
	DayOffset         int
	SendTime          string
	Timezone          string
	ContentText       string
}

// CampaignDefinitionCandidate is a local value only. In particular, it is
// not a command, plan, dispatch, receipt, or Provider request.
type CampaignDefinitionCandidate struct {
	Campaign                campaign.Campaign
	Steps                   []campaign.Step
	SourceCampaignID        int64
	SourceCampaignSegmentID int64
}

// ConvertCampaignDefinition accepts only a non-running, single-segment V1
// definition. V1 segment/member/job/event state is kept archive-only rather
// than being collapsed into a V2 campaign action.
func ConvertCampaignDefinition(row CampaignRow, rows []CampaignStepRow, actors ActorIDs) Decision[CampaignDefinitionCandidate] {
	approval, approvalOK := approvalStatus(row.ReviewStatus)
	runtime, runtimeOK, runtimeArchivable := runtimeStatus(row.RunStatus)
	if !validCampaignRow(row) || !approvalOK || (!runtimeOK && !runtimeArchivable) {
		return quarantine[CampaignDefinitionCandidate](ReasonInvalidSource)
	}
	if !runtimeOK {
		return archive[CampaignDefinitionCandidate](ReasonRuntimeRequiresArchive)
	}
	actorID, found := actors[row.OwnerUserID]
	if !found || actorID < 1 {
		return quarantine[CampaignDefinitionCandidate](ReasonActorUnresolved)
	}
	candidate := CampaignDefinitionCandidate{
		Campaign: campaign.Campaign{
			Code: row.CampaignCode, Name: row.DisplayName, ApprovalStatus: approval, RuntimeStatus: runtime,
			Version: 1, CreatedBy: actorID, UpdatedBy: actorID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		},
		SourceCampaignID: row.ID,
	}
	for index, step := range rows {
		if step.ID < 1 || step.CampaignID != row.ID || step.CampaignSegmentID < 1 {
			return quarantine[CampaignDefinitionCandidate](ReasonStepCampaignUnresolved)
		}
		if step.StepIndex != index {
			return quarantine[CampaignDefinitionCandidate](ReasonStepOrderInvalid)
		}
		if index == 0 {
			candidate.SourceCampaignSegmentID = step.CampaignSegmentID
		} else if step.CampaignSegmentID != candidate.SourceCampaignSegmentID {
			return archive[CampaignDefinitionCandidate](ReasonMultipleSegments)
		}
		delay, ok := delayMinutes(step.DayOffset, step.SendTime)
		if !ok || (step.Timezone != "" && step.Timezone != "Asia/Shanghai") || !validStepContent(step.ContentText) {
			return archive[CampaignDefinitionCandidate](ReasonScheduleRequiresArchive)
		}
		candidate.Steps = append(candidate.Steps, campaign.Step{Index: int32(index + 1), DelayMinutes: delay, Content: step.ContentText})
	}
	if len(candidate.Steps) > campaign.MaximumSteps {
		return archive[CampaignDefinitionCandidate](ReasonScheduleRequiresArchive)
	}
	return canonical(candidate)
}

// ArchiveCampaignRuntimeTable makes the excluded runtime scope explicit.
func ArchiveCampaignRuntimeTable(table string) Decision[struct{}] {
	switch table {
	case "campaign_segments", "campaign_members", "broadcast_jobs", "broadcast_job_events", "internal_event", "events":
		return archive[struct{}](ReasonRuntimeTableRequiresArchive)
	default:
		return quarantine[struct{}](ReasonInvalidSource)
	}
}

func approvalStatus(value string) (campaign.ApprovalStatus, bool) {
	switch value {
	case "pending_review":
		return campaign.ApprovalDraft, true
	case "approved":
		return campaign.ApprovalApproved, true
	case "rejected":
		return campaign.ApprovalRejected, true
	default:
		return "", false
	}
}

// runtimeStatus returns whether the state has a non-running V2 equivalent and
// whether it is a known V1 terminal/running state that must remain archived.
func runtimeStatus(value string) (campaign.RuntimeStatus, bool, bool) {
	switch value {
	case "draft":
		return campaign.RuntimeIdle, true, false
	case "paused":
		return campaign.RuntimePaused, true, false
	case "active", "finished", "completed", "cancelled":
		return "", false, true
	default:
		return "", false, false
	}
}

func validCampaignRow(row CampaignRow) bool {
	return row.ID > 0 && campaign.ValidCampaignCode(row.CampaignCode) && strings.TrimSpace(row.DisplayName) != "" && utf8.RuneCountInString(row.DisplayName) <= campaign.MaximumCampaignNameRunes && row.OwnerUserID != "" && row.OwnerUserID == strings.TrimSpace(row.OwnerUserID) && !row.CreatedAt.IsZero() && !row.UpdatedAt.IsZero() && !row.UpdatedAt.Before(row.CreatedAt)
}

func validStepContent(value string) bool {
	return strings.TrimSpace(value) != "" && utf8.RuneCountInString(value) <= campaign.MaximumStepContentRunes
}

// delayMinutes accepts only the V1 HH:MM representation and reconstructs it
// after conversion, so day_offset/send_time cannot be silently rounded.
func delayMinutes(dayOffset int, sendTime string) (int32, bool) {
	if dayOffset < 0 || len(sendTime) != 5 || sendTime[2] != ':' {
		return 0, false
	}
	hour, hourOK := twoDigits(sendTime[:2])
	minute, minuteOK := twoDigits(sendTime[3:])
	if !hourOK || !minuteOK || hour > 23 || minute > 59 {
		return 0, false
	}
	minutes := int64(dayOffset)*24*60 + int64(hour*60+minute)
	if minutes > 525600 || minutes/1440 != int64(dayOffset) || int(minutes%1440)/60 != hour || int(minutes%60) != minute {
		return 0, false
	}
	return int32(minutes), true
}

func twoDigits(value string) (int, bool) {
	if len(value) != 2 || value[0] < '0' || value[0] > '9' || value[1] < '0' || value[1] > '9' {
		return 0, false
	}
	return int(value[0]-'0')*10 + int(value[1]-'0'), true
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
