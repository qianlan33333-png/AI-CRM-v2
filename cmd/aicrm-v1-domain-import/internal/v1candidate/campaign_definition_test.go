package v1candidate

import (
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
)

var campaignTime = time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

func TestConvertCampaignDefinitionCreatesOnlyLocalDefinitionCandidate(t *testing.T) {
	decision := ConvertCampaignDefinition(campaignRow(), []CampaignStepRow{{
		ID: 31, CampaignID: 11, CampaignSegmentID: 21, StepIndex: 0, DayOffset: 1, SendTime: "09:30", Timezone: "Asia/Shanghai", ContentText: "hello",
	}}, ActorIDs{"owner-a": 7})
	if decision.Disposition != CanonicalCandidate || decision.Candidate == nil || decision.Reason != "" {
		t.Fatalf("decision = %#v", decision)
	}
	got := *decision.Candidate
	if got.Campaign.Code != "welcome-v1" || got.Campaign.ApprovalStatus != campaign.ApprovalDraft || got.Campaign.RuntimeStatus != campaign.RuntimePaused || got.Campaign.CreatedBy != 7 || got.Campaign.UpdatedBy != 7 || got.SourceCampaignID != 11 || got.SourceCampaignSegmentID != 21 {
		t.Fatalf("candidate campaign = %#v", got)
	}
	if len(got.Steps) != 1 || got.Steps[0] != (campaign.Step{Index: 1, DelayMinutes: 2010, Content: "hello"}) {
		t.Fatalf("candidate steps = %#v", got.Steps)
	}
}

func TestConvertCampaignDefinitionFailsClosedForActorAndRuntime(t *testing.T) {
	actor := ConvertCampaignDefinition(campaignRow(), nil, ActorIDs{})
	assertDefinitionDecision(t, actor, Quarantine, ReasonActorUnresolved)

	running := campaignRow()
	running.RunStatus = "active"
	decision := ConvertCampaignDefinition(running, nil, ActorIDs{"owner-a": 7})
	assertDefinitionDecision(t, decision, Archive, ReasonRuntimeRequiresArchive)

	unknown := campaignRow()
	unknown.ReviewStatus = "surprise"
	decision = ConvertCampaignDefinition(unknown, nil, ActorIDs{"owner-a": 7})
	assertDefinitionDecision(t, decision, Quarantine, ReasonInvalidSource)
}

func TestConvertCampaignDefinitionValidatesCampaignFKAndStepOrder(t *testing.T) {
	wrongCampaign := ConvertCampaignDefinition(campaignRow(), []CampaignStepRow{{ID: 31, CampaignID: 12, CampaignSegmentID: 21, StepIndex: 0, SendTime: "09:00", ContentText: "hello"}}, ActorIDs{"owner-a": 7})
	assertDefinitionDecision(t, wrongCampaign, Quarantine, ReasonStepCampaignUnresolved)

	badOrder := ConvertCampaignDefinition(campaignRow(), []CampaignStepRow{{ID: 31, CampaignID: 11, CampaignSegmentID: 21, StepIndex: 1, SendTime: "09:00", ContentText: "hello"}}, ActorIDs{"owner-a": 7})
	assertDefinitionDecision(t, badOrder, Quarantine, ReasonStepOrderInvalid)

	multipleSegments := ConvertCampaignDefinition(campaignRow(), []CampaignStepRow{
		{ID: 31, CampaignID: 11, CampaignSegmentID: 21, StepIndex: 0, SendTime: "09:00", ContentText: "one"},
		{ID: 32, CampaignID: 11, CampaignSegmentID: 22, StepIndex: 1, SendTime: "09:00", ContentText: "two"},
	}, ActorIDs{"owner-a": 7})
	assertDefinitionDecision(t, multipleSegments, Archive, ReasonMultipleSegments)
}

func TestConvertCampaignDefinitionRejectsLossySchedule(t *testing.T) {
	seconds := ConvertCampaignDefinition(campaignRow(), []CampaignStepRow{{ID: 31, CampaignID: 11, CampaignSegmentID: 21, StepIndex: 0, SendTime: "09:30:00", ContentText: "hello"}}, ActorIDs{"owner-a": 7})
	assertDefinitionDecision(t, seconds, Archive, ReasonScheduleRequiresArchive)

	timezone := ConvertCampaignDefinition(campaignRow(), []CampaignStepRow{{ID: 31, CampaignID: 11, CampaignSegmentID: 21, StepIndex: 0, SendTime: "09:30", Timezone: "UTC", ContentText: "hello"}}, ActorIDs{"owner-a": 7})
	assertDefinitionDecision(t, timezone, Archive, ReasonScheduleRequiresArchive)
}

func TestArchiveCampaignRuntimeTables(t *testing.T) {
	for _, table := range []string{"campaign_segments", "campaign_members", "broadcast_jobs", "broadcast_job_events", "internal_event", "events"} {
		t.Run(table, func(t *testing.T) {
			decision := ArchiveCampaignRuntimeTable(table)
			if decision.Disposition != Archive || decision.Reason != ReasonRuntimeTableRequiresArchive || decision.Candidate != nil {
				t.Fatalf("decision = %#v", decision)
			}
		})
	}
}

func assertDefinitionDecision(t *testing.T, got Decision[CampaignDefinitionCandidate], disposition Disposition, reason string) {
	t.Helper()
	if got.Disposition != disposition || got.Reason != reason || got.Candidate != nil {
		t.Fatalf("decision = %#v, want disposition=%q reason=%q", got, disposition, reason)
	}
}

func campaignRow() CampaignRow {
	return CampaignRow{ID: 11, CampaignCode: "welcome-v1", DisplayName: "Welcome", ReviewStatus: "pending_review", RunStatus: "paused", OwnerUserID: "owner-a", CreatedAt: campaignTime, UpdatedAt: campaignTime}
}
