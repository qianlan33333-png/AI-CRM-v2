package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	campaigndb "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/store/generated"
)

var _ campaignport.ApprovedTouchPlanHandoffReader = (*Repository)(nil)

func (repository *Repository) LockApprovedTouchPlanHandoff(ctx context.Context, campaignCode, planID string) (campaignport.ApprovedTouchPlanHandoffSnapshot, error) {
	if !campaign.ValidCampaignCode(campaignCode) || !campaign.ValidTouchPlanReviewID(planID) {
		return campaignport.ApprovedTouchPlanHandoffSnapshot{}, campaign.ErrInvalidArgument
	}
	queries, err := repository.initiationQueries(ctx)
	if err != nil {
		return campaignport.ApprovedTouchPlanHandoffSnapshot{}, err
	}
	row, err := queries.LockApprovedCampaignTouchPlanHandoff(ctx, campaigndb.LockApprovedCampaignTouchPlanHandoffParams{CampaignCode: campaignCode, PlanID: planID})
	if errors.Is(err, pgx.ErrNoRows) {
		return campaignport.ApprovedTouchPlanHandoffSnapshot{}, campaign.ErrNotFound
	}
	if err != nil {
		return campaignport.ApprovedTouchPlanHandoffSnapshot{}, err
	}
	targets, err := queries.ListApprovedCampaignTouchPlanTargets(ctx, campaigndb.ListApprovedCampaignTouchPlanTargetsParams{CampaignCode: campaignCode, PlanID: planID})
	if err != nil {
		return campaignport.ApprovedTouchPlanHandoffSnapshot{}, err
	}
	stepRows, err := queries.ListApprovedCampaignTouchPlanSteps(ctx, campaigndb.ListApprovedCampaignTouchPlanStepsParams{CampaignCode: campaignCode, PlanID: planID})
	if err != nil {
		return campaignport.ApprovedTouchPlanHandoffSnapshot{}, err
	}
	steps := make([]campaigndb.ListCampaignTouchPlanStepsRow, len(stepRows))
	for index, step := range stepRows {
		steps[index] = campaigndb.ListCampaignTouchPlanStepsRow{StepIndex: step.StepIndex, DelayMinutes: step.DelayMinutes, Content: step.Content}
	}
	header := touchPlanHeader{
		id: row.ID, campaignCode: row.CampaignCode, campaignVersion: row.CampaignVersion, sourceKind: row.SourceKind,
		customerSelectionID: row.CustomerSelectionID, customerSelectionVersion: row.CustomerSelectionVersion,
		segmentID: row.SegmentID, audiencePackageID: row.AudiencePackageID, audiencePackageVersion: row.AudiencePackageVersion,
		memberSnapshotWatermark: row.MemberSnapshotWatermark, sourceDigest: row.SourceDigest, targetDigest: row.TargetDigest,
		contentDigest: row.ContentDigest, targetCount: int64(row.TargetCount), contentStepCount: int64(row.ContentStepCount),
		candidateCount: int64(row.CandidateCount), activeCustomerCount: int64(row.ActiveCustomerCount),
		inactiveExcludedCount: int64(row.InactiveExcludedCount), policyExcludedCount: int64(row.PolicyExcludedCount),
		ownerActorID: row.OwnerActorID, createdAt: row.PlanCreatedAt, localOnly: row.LocalOnly,
		providerExecutionEligible: row.ProviderExecutionEligible, runtimeExecuted: row.RuntimeExecuted,
		realExternalCallExecuted: row.RealExternalCallExecuted, deliveryProven: row.DeliveryProven,
	}
	plan, valid := planFromHeader(header, targets, steps)
	handoff := campaign.TouchPlanHandoff{
		PlanID: row.ID, CampaignCode: row.CampaignCode, ReviewVersion: row.ReviewVersion, Status: row.HandoffStatus,
		CreatedAt: row.HandoffCreatedAt.Time.UTC(), LocalOnly: row.HandoffLocalOnly,
		ProviderExecutionEligible: row.HandoffProviderExecutionEligible,
		RealExternalCallExecuted:  row.HandoffRealExternalCallExecuted, DeliveryProven: row.HandoffDeliveryProven,
	}
	sourceDigest, digestValid := encodedDigest(row.SourceDigest)
	if !valid || !campaign.ValidDraftTouchPlan(plan) || !digestValid || !row.ReviewedAt.Valid || !row.HandoffCreatedAt.Valid ||
		!campaign.ValidTouchPlanHandoff(handoff) || handoff.CampaignCode != campaignCode || handoff.PlanID != planID ||
		row.TargetCount != int32(len(targets)) || row.ContentStepCount != int32(len(stepRows)) {
		return campaignport.ApprovedTouchPlanHandoffSnapshot{}, campaign.ErrUnavailable
	}
	return campaignport.ApprovedTouchPlanHandoffSnapshot{
		CampaignCode: campaignCode, PlanID: planID, ReviewVersion: row.ReviewVersion,
		SourceDigest: sourceDigest, TargetDigest: plan.Targets.Digest, ContentDigest: plan.Content.Digest,
		CustomerIDs: append([]int64(nil), plan.Targets.CustomerIDs...), Steps: append([]campaign.Step(nil), plan.Content.Steps...),
		ApprovedAt: row.ReviewedAt.Time.UTC(), HandoffCreatedAt: row.HandoffCreatedAt.Time.UTC(),
	}, nil
}
