package store

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	campaigndb "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/store/generated"
)

var (
	_ campaignport.CampaignDraftReader = (*Repository)(nil)
	_ campaignport.Repository          = (*Repository)(nil)
)

func (r *Repository) initiationQueries(ctx context.Context) (*campaigndb.Queries, error) {
	tx, err := r.transaction(ctx)
	if err != nil {
		return nil, err
	}
	return campaigndb.New(tx), nil
}

func (r *Repository) LockDraftCampaign(ctx context.Context, campaignCode string) (campaignport.CampaignDraftFact, error) {
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return campaignport.CampaignDraftFact{}, err
	}
	row, err := queries.LockCampaignDraftForTouchPlan(ctx, campaignCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return campaignport.CampaignDraftFact{}, campaign.ErrNotFound
	}
	if err != nil {
		return campaignport.CampaignDraftFact{}, err
	}
	steps, err := queries.ListCampaignStepsForTouchPlan(ctx, campaignCode)
	if err != nil {
		return campaignport.CampaignDraftFact{}, err
	}
	result := campaignport.CampaignDraftFact{
		CampaignCode:   row.CampaignCode,
		Version:        row.Version,
		ApprovalStatus: campaign.ApprovalStatus(row.ApprovalStatus),
		RuntimeStatus:  campaign.RuntimeStatus(row.RuntimeStatus),
		Steps:          make([]campaign.Step, len(steps)),
	}
	for index, step := range steps {
		result.Steps[index] = campaign.Step{Index: step.StepIndex, DelayMinutes: step.DelayMinutes, Content: step.Content}
	}
	return result, nil
}

func canonicalSourceMembers(customerIDs []int64) bool {
	if len(customerIDs) > campaign.MaximumDraftTouchTargets {
		return false
	}
	for index, customerID := range customerIDs {
		if customerID < 1 || index > 0 && customerIDs[index-1] >= customerID {
			return false
		}
	}
	return true
}

func (r *Repository) ReserveDraftCreate(ctx context.Context, reservation campaignport.CreateReservation) (campaignport.CreateReceipt, bool, error) {
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return campaignport.CreateReceipt{}, false, err
	}
	row, err := queries.ReserveCampaignTouchPlanReceipt(ctx, campaigndb.ReserveCampaignTouchPlanReceiptParams{
		ActorID: reservation.ActorID, KeyDigest: reservation.KeyDigest[:], PayloadDigest: reservation.PayloadDigest[:], PlanID: reservation.PlanID,
	})
	if err == nil {
		receipt, valid := receiptFromRow(row.ID, row.ActorID, row.KeyDigest, row.PayloadDigest, row.PlanID, row.EventID, row.State)
		if !valid {
			return campaignport.CreateReceipt{}, false, campaign.ErrUnavailable
		}
		return receipt, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return campaignport.CreateReceipt{}, false, err
	}
	existing, err := queries.GetCampaignTouchPlanReceiptForUpdate(ctx, campaigndb.GetCampaignTouchPlanReceiptForUpdateParams{
		ActorID: reservation.ActorID, KeyDigest: reservation.KeyDigest[:],
	})
	if err != nil {
		return campaignport.CreateReceipt{}, false, err
	}
	if existing.PlanID != reservation.PlanID || len(existing.PayloadDigest) != len(reservation.PayloadDigest) ||
		subtle.ConstantTimeCompare(existing.PayloadDigest, reservation.PayloadDigest[:]) != 1 {
		return campaignport.CreateReceipt{}, false, campaign.ErrIdempotencyConflict
	}
	receipt, valid := receiptFromRow(existing.ID, existing.ActorID, existing.KeyDigest, existing.PayloadDigest, existing.PlanID, existing.EventID, existing.State)
	if !valid {
		return campaignport.CreateReceipt{}, false, campaign.ErrUnavailable
	}
	return receipt, false, nil
}

func (r *Repository) SaveDraftTouchPlan(ctx context.Context, plan campaign.DraftTouchPlan) error {
	if !campaign.ValidDraftTouchPlan(plan) {
		return campaign.ErrUnavailable
	}
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return err
	}
	params, valid := touchPlanInsertParams(plan)
	if !valid {
		return campaign.ErrUnavailable
	}
	if err = queries.InsertCampaignTouchPlan(ctx, params); err != nil {
		return err
	}
	if err = queries.InsertCampaignTouchPlanTargets(ctx, campaigndb.InsertCampaignTouchPlanTargetsParams{
		PlanID: plan.ID, CustomerIds: append([]int64(nil), plan.Targets.CustomerIDs...),
	}); err != nil {
		return err
	}
	for _, step := range plan.Content.Steps {
		if err = queries.InsertCampaignTouchPlanStep(ctx, campaigndb.InsertCampaignTouchPlanStepParams{
			PlanID: plan.ID, StepIndex: step.Index, DelayMinutes: step.DelayMinutes, Content: step.Content,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) CompleteDraftCreate(ctx context.Context, receipt campaignport.CreateReceipt, eventID int64) error {
	if receipt.ID < 1 || receipt.Completed || eventID < 1 {
		return campaign.ErrUnavailable
	}
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return err
	}
	row, err := queries.CompleteCampaignTouchPlanReceipt(ctx, campaigndb.CompleteCampaignTouchPlanReceiptParams{
		ID: receipt.ID, EventID: pgtype.Int8{Int64: eventID, Valid: true},
	})
	if err != nil {
		return campaign.ErrUnavailable
	}
	completed, valid := receiptFromRow(row.ID, row.ActorID, row.KeyDigest, row.PayloadDigest, row.PlanID, row.EventID, row.State)
	if !valid || !completed.Completed || completed.EventID != eventID || !sameStoredReceipt(completed, receipt) {
		return campaign.ErrUnavailable
	}
	return nil
}

func (r *Repository) ListDraftTouchPlanSummaries(ctx context.Context, campaignCode string, after *campaignport.DraftTouchPlanKeyset, limit int32) ([]campaign.DraftTouchPlanSummary, error) {
	if limit < 1 || limit > campaignport.MaximumDraftTouchPlanPageLimit+1 {
		return nil, campaign.ErrUnavailable
	}
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return nil, err
	}
	params := campaigndb.ListCampaignTouchPlanSummariesParams{CampaignCode: campaignCode, PageLimit: limit}
	if after != nil {
		if after.CreatedAt.IsZero() || !campaign.ValidDraftTouchPlanID(after.PlanID) {
			return nil, campaign.ErrUnavailable
		}
		params.AfterCreatedAt = pgtype.Timestamptz{Time: after.CreatedAt.UTC(), Valid: true}
		params.AfterID = pgtype.Text{String: after.PlanID, Valid: true}
	}
	rows, err := queries.ListCampaignTouchPlanSummaries(ctx, params)
	if err != nil {
		return nil, err
	}
	result := make([]campaign.DraftTouchPlanSummary, len(rows))
	for index, row := range rows {
		summary, valid := summaryFromHeader(headerFromSummaryRow(row))
		if !valid {
			return nil, campaign.ErrUnavailable
		}
		result[index] = summary
	}
	return result, nil
}

func (r *Repository) ListTouchPlanIndex(ctx context.Context, reviewStatus campaign.TouchPlanReviewStatus, after *campaignport.DraftTouchPlanKeyset, limit int32) ([]campaign.TouchPlanIndexItem, error) {
	if limit < 1 || limit > campaignport.MaximumDraftTouchPlanPageLimit+1 || reviewStatus != "" && !reviewStatus.Valid() {
		return nil, campaign.ErrUnavailable
	}
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return nil, err
	}
	params := campaigndb.ListCampaignTouchPlanIndexParams{PageLimit: limit}
	if reviewStatus != "" {
		params.ReviewStatus = pgtype.Text{String: string(reviewStatus), Valid: true}
	}
	if after != nil {
		if after.CreatedAt.IsZero() || !campaign.ValidDraftTouchPlanID(after.PlanID) {
			return nil, campaign.ErrUnavailable
		}
		params.AfterCreatedAt = pgtype.Timestamptz{Time: after.CreatedAt.UTC(), Valid: true}
		params.AfterID = pgtype.Text{String: after.PlanID, Valid: true}
	}
	rows, err := queries.ListCampaignTouchPlanIndex(ctx, params)
	if err != nil {
		return nil, err
	}
	result := make([]campaign.TouchPlanIndexItem, len(rows))
	for index, row := range rows {
		summary, valid := summaryFromHeader(headerFromIndexRow(row))
		item := campaign.TouchPlanIndexItem{Plan: summary, ReviewStatus: campaign.TouchPlanReviewStatus(row.ReviewStatus), ReviewVersion: row.ReviewVersion}
		if !valid || !campaign.ValidTouchPlanIndexItem(item) {
			return nil, campaign.ErrUnavailable
		}
		result[index] = item
	}
	return result, nil
}

func (r *Repository) ReadDraftTouchPlan(ctx context.Context, campaignCode, planID string) (campaign.DraftTouchPlan, error) {
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return campaign.DraftTouchPlan{}, err
	}
	row, err := queries.GetCampaignTouchPlan(ctx, campaigndb.GetCampaignTouchPlanParams{CampaignCode: campaignCode, ID: planID})
	if errors.Is(err, pgx.ErrNoRows) {
		return campaign.DraftTouchPlan{}, campaign.ErrNotFound
	}
	if err != nil {
		return campaign.DraftTouchPlan{}, err
	}
	targets, err := queries.ListCampaignTouchPlanTargets(ctx, planID)
	if err != nil {
		return campaign.DraftTouchPlan{}, err
	}
	steps, err := queries.ListCampaignTouchPlanSteps(ctx, planID)
	if err != nil {
		return campaign.DraftTouchPlan{}, err
	}
	plan, valid := planFromHeader(headerFromPlanRow(row), targets, steps)
	if !valid {
		return campaign.DraftTouchPlan{}, campaign.ErrUnavailable
	}
	return plan, nil
}

type touchPlanHeader struct {
	id, campaignCode, sourceKind                               string
	campaignVersion, targetCount, contentStepCount             int64
	candidateCount, activeCustomerCount, inactiveExcludedCount int64
	policyExcludedCount, ownerActorID                          int64
	customerSelectionID, customerSelectionVersion              pgtype.Text
	segmentID, audiencePackageID, audiencePackageVersion       pgtype.Int8
	memberSnapshotWatermark                                    pgtype.Timestamptz
	sourceDigest, targetDigest, contentDigest                  []byte
	createdAt                                                  pgtype.Timestamptz
	localOnly, providerExecutionEligible, runtimeExecuted      bool
	realExternalCallExecuted, deliveryProven                   bool
}

func headerFromPlanRow(row campaigndb.GetCampaignTouchPlanRow) touchPlanHeader {
	return touchPlanHeader{
		id: row.ID, campaignCode: row.CampaignCode, campaignVersion: row.CampaignVersion, sourceKind: row.SourceKind,
		customerSelectionID: row.CustomerSelectionID, customerSelectionVersion: row.CustomerSelectionVersion,
		segmentID: row.SegmentID, audiencePackageID: row.AudiencePackageID, audiencePackageVersion: row.AudiencePackageVersion,
		memberSnapshotWatermark: row.MemberSnapshotWatermark, sourceDigest: row.SourceDigest, targetDigest: row.TargetDigest,
		contentDigest: row.ContentDigest, targetCount: int64(row.TargetCount), contentStepCount: int64(row.ContentStepCount),
		candidateCount: int64(row.CandidateCount), activeCustomerCount: int64(row.ActiveCustomerCount),
		inactiveExcludedCount: int64(row.InactiveExcludedCount), policyExcludedCount: int64(row.PolicyExcludedCount),
		ownerActorID: row.OwnerActorID, createdAt: row.CreatedAt,
		localOnly: row.LocalOnly, providerExecutionEligible: row.ProviderExecutionEligible, runtimeExecuted: row.RuntimeExecuted,
		realExternalCallExecuted: row.RealExternalCallExecuted, deliveryProven: row.DeliveryProven,
	}
}

func headerFromSummaryRow(row campaigndb.ListCampaignTouchPlanSummariesRow) touchPlanHeader {
	return touchPlanHeader{
		id: row.ID, campaignCode: row.CampaignCode, campaignVersion: row.CampaignVersion, sourceKind: row.SourceKind,
		customerSelectionID: row.CustomerSelectionID, customerSelectionVersion: row.CustomerSelectionVersion,
		segmentID: row.SegmentID, audiencePackageID: row.AudiencePackageID, audiencePackageVersion: row.AudiencePackageVersion,
		memberSnapshotWatermark: row.MemberSnapshotWatermark, sourceDigest: row.SourceDigest, targetDigest: row.TargetDigest,
		contentDigest: row.ContentDigest, targetCount: int64(row.TargetCount), contentStepCount: int64(row.ContentStepCount),
		candidateCount: int64(row.CandidateCount), activeCustomerCount: int64(row.ActiveCustomerCount),
		inactiveExcludedCount: int64(row.InactiveExcludedCount), policyExcludedCount: int64(row.PolicyExcludedCount),
		ownerActorID: row.OwnerActorID, createdAt: row.CreatedAt,
		localOnly: row.LocalOnly, providerExecutionEligible: row.ProviderExecutionEligible, runtimeExecuted: row.RuntimeExecuted,
		realExternalCallExecuted: row.RealExternalCallExecuted, deliveryProven: row.DeliveryProven,
	}
}

func headerFromIndexRow(row campaigndb.ListCampaignTouchPlanIndexRow) touchPlanHeader {
	return touchPlanHeader{
		id: row.ID, campaignCode: row.CampaignCode, campaignVersion: row.CampaignVersion, sourceKind: row.SourceKind,
		customerSelectionID: row.CustomerSelectionID, customerSelectionVersion: row.CustomerSelectionVersion,
		segmentID: row.SegmentID, audiencePackageID: row.AudiencePackageID, audiencePackageVersion: row.AudiencePackageVersion,
		memberSnapshotWatermark: row.MemberSnapshotWatermark, sourceDigest: row.SourceDigest, targetDigest: row.TargetDigest,
		contentDigest: row.ContentDigest, targetCount: int64(row.TargetCount), contentStepCount: int64(row.ContentStepCount),
		candidateCount: int64(row.CandidateCount), activeCustomerCount: int64(row.ActiveCustomerCount),
		inactiveExcludedCount: int64(row.InactiveExcludedCount), policyExcludedCount: int64(row.PolicyExcludedCount),
		ownerActorID: row.OwnerActorID, createdAt: row.CreatedAt,
		localOnly: row.LocalOnly, providerExecutionEligible: row.ProviderExecutionEligible, runtimeExecuted: row.RuntimeExecuted,
		realExternalCallExecuted: row.RealExternalCallExecuted, deliveryProven: row.DeliveryProven,
	}
}

func touchPlanInsertParams(plan campaign.DraftTouchPlan) (campaigndb.InsertCampaignTouchPlanParams, bool) {
	params := campaigndb.InsertCampaignTouchPlanParams{
		ID: plan.ID, CampaignCode: plan.CampaignCode, CampaignVersion: plan.CampaignVersion, SourceKind: string(plan.Source.Kind),
		TargetCount: int32(len(plan.Targets.CustomerIDs)), ContentStepCount: int32(len(plan.Content.Steps)),
		CandidateCount: plan.Exclusions.CandidateCount, ActiveCustomerCount: plan.Exclusions.ActiveCustomerCount,
		InactiveExcludedCount: plan.Exclusions.InactiveExcludedCount, PolicyExcludedCount: plan.Exclusions.PolicyExcludedCount,
		OwnerActorID: plan.OwnerActorID,
		CreatedAt:    pgtype.Timestamptz{Time: plan.CreatedAt.UTC(), Valid: true},
	}
	var sourceDigest string
	switch plan.Source.Kind {
	case campaign.InitiationSourceCustomerSelection:
		fact := plan.Source.CustomerSelection
		if fact == nil {
			return params, false
		}
		params.CustomerSelectionID = pgtype.Text{String: fact.ID, Valid: true}
		params.CustomerSelectionVersion = pgtype.Text{String: fact.Version, Valid: true}
		sourceDigest = fact.Digest
	case campaign.InitiationSourceSegmentMembers:
		fact := plan.Source.Segment
		if fact == nil {
			return params, false
		}
		params.SegmentID = pgtype.Int8{Int64: fact.SegmentID, Valid: true}
		params.MemberSnapshotWatermark = pgtype.Timestamptz{Time: fact.MemberSnapshotWatermark.UTC(), Valid: true}
		sourceDigest = fact.Digest
	case campaign.InitiationSourceAudiencePackageMembers:
		fact := plan.Source.AudiencePackage
		if fact == nil {
			return params, false
		}
		params.AudiencePackageID = pgtype.Int8{Int64: fact.PackageID, Valid: true}
		params.AudiencePackageVersion = pgtype.Int8{Int64: fact.PackageVersion, Valid: true}
		params.MemberSnapshotWatermark = pgtype.Timestamptz{Time: fact.MemberSnapshotWatermark.UTC(), Valid: true}
		sourceDigest = fact.Digest
	default:
		return params, false
	}
	var valid bool
	if params.SourceDigest, valid = decodedDigest(sourceDigest); !valid {
		return params, false
	}
	if params.TargetDigest, valid = decodedDigest(plan.Targets.Digest); !valid {
		return params, false
	}
	params.ContentDigest, valid = decodedDigest(plan.Content.Digest)
	return params, valid
}

func planFromHeader(header touchPlanHeader, targets []int64, steps []campaigndb.ListCampaignTouchPlanStepsRow) (campaign.DraftTouchPlan, bool) {
	if header.targetCount != int64(len(targets)) || header.contentStepCount != int64(len(steps)) || !canonicalSourceMembers(targets) ||
		header.createdAt.Valid == false || header.createdAt.Time.IsZero() {
		return campaign.DraftTouchPlan{}, false
	}
	source, valid := sourceFromHeader(header)
	if !valid {
		return campaign.DraftTouchPlan{}, false
	}
	targetDigest, valid := encodedDigest(header.targetDigest)
	if !valid {
		return campaign.DraftTouchPlan{}, false
	}
	contentDigest, valid := encodedDigest(header.contentDigest)
	if !valid {
		return campaign.DraftTouchPlan{}, false
	}
	content := make([]campaign.Step, len(steps))
	for index, step := range steps {
		content[index] = campaign.Step{Index: step.StepIndex, DelayMinutes: step.DelayMinutes, Content: step.Content}
	}
	return campaign.DraftTouchPlan{
		ID: header.id, CampaignCode: header.campaignCode, CampaignVersion: header.campaignVersion, Source: source,
		Targets: campaign.CustomerTargetSnapshot{CustomerIDs: append([]int64(nil), targets...), Digest: targetDigest},
		Content: campaign.ContentSnapshot{Steps: content, Digest: contentDigest}, OwnerActorID: header.ownerActorID,
		Exclusions: campaign.PreviewExclusionSummary{CandidateCount: int32(header.candidateCount), ActiveCustomerCount: int32(header.activeCustomerCount),
			InactiveExcludedCount: int32(header.inactiveExcludedCount), PolicyExcludedCount: int32(header.policyExcludedCount)},
		CreatedAt: header.createdAt.Time.UTC(),
		Safety: campaign.InitiationSafety{LocalOnly: header.localOnly, ProviderExecutionEligible: header.providerExecutionEligible,
			RuntimeExecuted: header.runtimeExecuted, RealExternalCallExecuted: header.realExternalCallExecuted, DeliveryProven: header.deliveryProven},
	}, true
}

func summaryFromHeader(header touchPlanHeader) (campaign.DraftTouchPlanSummary, bool) {
	if header.targetCount < 1 || header.targetCount > campaign.MaximumDraftTouchTargets ||
		header.contentStepCount < 1 || header.contentStepCount > campaign.MaximumSteps || !header.createdAt.Valid || header.createdAt.Time.IsZero() {
		return campaign.DraftTouchPlanSummary{}, false
	}
	source, valid := sourceFromHeader(header)
	if !valid {
		return campaign.DraftTouchPlanSummary{}, false
	}
	targetDigest, valid := encodedDigest(header.targetDigest)
	if !valid {
		return campaign.DraftTouchPlanSummary{}, false
	}
	contentDigest, valid := encodedDigest(header.contentDigest)
	if !valid {
		return campaign.DraftTouchPlanSummary{}, false
	}
	return campaign.DraftTouchPlanSummary{
		ID: header.id, CampaignCode: header.campaignCode, CampaignVersion: header.campaignVersion, Source: source,
		TargetCount: int32(header.targetCount), TargetDigest: targetDigest, ContentStepCount: int32(header.contentStepCount), ContentDigest: contentDigest,
		OwnerActorID: header.ownerActorID, Exclusions: campaign.PreviewExclusionSummary{CandidateCount: int32(header.candidateCount),
			ActiveCustomerCount: int32(header.activeCustomerCount), InactiveExcludedCount: int32(header.inactiveExcludedCount), PolicyExcludedCount: int32(header.policyExcludedCount)},
		CreatedAt: header.createdAt.Time.UTC(),
		Safety: campaign.InitiationSafety{LocalOnly: header.localOnly, ProviderExecutionEligible: header.providerExecutionEligible,
			RuntimeExecuted: header.runtimeExecuted, RealExternalCallExecuted: header.realExternalCallExecuted, DeliveryProven: header.deliveryProven},
	}, true
}

func sourceFromHeader(header touchPlanHeader) (campaign.InitiationSourceRef, bool) {
	sourceDigest, valid := encodedDigest(header.sourceDigest)
	if !valid {
		return campaign.InitiationSourceRef{}, false
	}
	switch campaign.InitiationSourceKind(header.sourceKind) {
	case campaign.InitiationSourceCustomerSelection:
		if !header.customerSelectionID.Valid || !header.customerSelectionVersion.Valid || header.segmentID.Valid ||
			header.audiencePackageID.Valid || header.audiencePackageVersion.Valid || header.memberSnapshotWatermark.Valid {
			return campaign.InitiationSourceRef{}, false
		}
		return campaign.InitiationSourceRef{Kind: campaign.InitiationSourceCustomerSelection,
			CustomerSelection: &campaign.CustomerSelectionSourceFact{ID: header.customerSelectionID.String, Version: header.customerSelectionVersion.String, Digest: sourceDigest}}, true
	case campaign.InitiationSourceSegmentMembers:
		if header.customerSelectionID.Valid || header.customerSelectionVersion.Valid || !header.segmentID.Valid ||
			header.audiencePackageID.Valid || header.audiencePackageVersion.Valid || !header.memberSnapshotWatermark.Valid {
			return campaign.InitiationSourceRef{}, false
		}
		return campaign.InitiationSourceRef{Kind: campaign.InitiationSourceSegmentMembers,
			Segment: &campaign.SegmentMemberSourceFact{SegmentID: header.segmentID.Int64, MemberSnapshotWatermark: header.memberSnapshotWatermark.Time.UTC(), Digest: sourceDigest}}, true
	case campaign.InitiationSourceAudiencePackageMembers:
		if header.customerSelectionID.Valid || header.customerSelectionVersion.Valid || header.segmentID.Valid ||
			!header.audiencePackageID.Valid || !header.audiencePackageVersion.Valid || !header.memberSnapshotWatermark.Valid {
			return campaign.InitiationSourceRef{}, false
		}
		return campaign.InitiationSourceRef{Kind: campaign.InitiationSourceAudiencePackageMembers,
			AudiencePackage: &campaign.AudiencePackageMemberSourceFact{PackageID: header.audiencePackageID.Int64,
				PackageVersion: header.audiencePackageVersion.Int64, MemberSnapshotWatermark: header.memberSnapshotWatermark.Time.UTC(), Digest: sourceDigest}}, true
	default:
		return campaign.InitiationSourceRef{}, false
	}
}

func receiptFromRow(id, actorID int64, keyDigest, payloadDigest []byte, planID string, eventID pgtype.Int8, state string) (campaignport.CreateReceipt, bool) {
	if id < 1 || actorID < 1 || len(keyDigest) != 32 || len(payloadDigest) != 32 || !campaign.ValidDraftTouchPlanID(planID) {
		return campaignport.CreateReceipt{}, false
	}
	result := campaignport.CreateReceipt{ID: id, ActorID: actorID, PlanID: planID, Completed: state == "completed"}
	copy(result.KeyDigest[:], keyDigest)
	copy(result.PayloadDigest[:], payloadDigest)
	if state != "reserved" && state != "completed" || state == "reserved" && eventID.Valid || state == "completed" && (!eventID.Valid || eventID.Int64 < 1) {
		return campaignport.CreateReceipt{}, false
	}
	if eventID.Valid {
		result.EventID = eventID.Int64
	}
	return result, true
}

func sameStoredReceipt(left, right campaignport.CreateReceipt) bool {
	return left.ID == right.ID && left.ActorID == right.ActorID && left.PlanID == right.PlanID &&
		subtle.ConstantTimeCompare(left.KeyDigest[:], right.KeyDigest[:]) == 1 &&
		subtle.ConstantTimeCompare(left.PayloadDigest[:], right.PayloadDigest[:]) == 1
}

func decodedDigest(value string) ([]byte, bool) {
	decoded, err := hex.DecodeString(value)
	return decoded, err == nil && len(decoded) == 32
}

func encodedDigest(value []byte) (string, bool) {
	if len(value) != 32 {
		return "", false
	}
	return hex.EncodeToString(value), true
}
