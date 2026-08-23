// Package app implements the non-shared Campaign initiation snapshot seam.
package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"time"

	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
)

var (
	ErrInvalidCommand = errors.New("invalid campaign initiation command")
	ErrUnavailable    = errors.New("campaign initiation unavailable")
	ErrConflict       = errors.New("campaign initiation conflict")
)

const createDraftOperation = "campaign.draft_touch_plan.created"

type Service struct {
	uow         campaignport.UnitOfWork
	campaigns   campaignport.CampaignDraftReader
	sources     campaignport.TargetSourceResolver
	eligibility campaignport.EligibilityChecker
	repository  campaignport.Repository
	events      campaignport.EventAppender
	now         func() time.Time
}

func NewService(
	uow campaignport.UnitOfWork,
	campaigns campaignport.CampaignDraftReader,
	sources campaignport.TargetSourceResolver,
	eligibility campaignport.EligibilityChecker,
	repository campaignport.Repository,
	events campaignport.EventAppender,
) (*Service, error) {
	if isNil(uow) || isNil(campaigns) || isNil(eligibility) || isNil(repository) || isNil(events) {
		return nil, ErrUnavailable
	}
	return &Service{uow: uow, campaigns: campaigns, sources: sources, eligibility: eligibility, repository: repository, events: events, now: time.Now}, nil
}

func (service *Service) CreateDraftTouchPlan(ctx context.Context, command campaign.CreateDraftTouchPlanCommand) (campaign.DraftTouchPlan, error) {
	if !ready(service, ctx) {
		return campaign.DraftTouchPlan{}, ErrUnavailable
	}
	if command.Source.Kind == campaign.InitiationSourceCustomerSelection {
		canonicalCustomerIDs, valid := campaign.CanonicalCustomerSelection(command.Source.CustomerIDs)
		if !valid {
			return campaign.DraftTouchPlan{}, ErrInvalidCommand
		}
		command.Source.CustomerIDs = canonicalCustomerIDs
	}
	if !campaign.ValidateCreateDraftTouchPlanCommand(command) {
		return campaign.DraftTouchPlan{}, ErrInvalidCommand
	}
	if command.Source.Kind == campaign.InitiationSourceCustomerFilter {
		return campaign.DraftTouchPlan{}, campaign.ErrBlockedRedline
	}
	if command.Source.Kind != campaign.InitiationSourceCustomerSelection && isNil(service.sources) {
		return campaign.DraftTouchPlan{}, ErrUnavailable
	}
	now := service.now().UTC().Truncate(time.Microsecond)
	if now.IsZero() {
		return campaign.DraftTouchPlan{}, ErrUnavailable
	}
	reservation := campaignport.CreateReservation{
		ActorID:       command.Owner.ID,
		KeyDigest:     sha256.Sum256([]byte(command.IdempotencyKey)),
		PayloadDigest: payloadDigest(command),
		PlanID:        campaign.DraftTouchPlanID(command.Owner.ID, command.CampaignCode, command.IdempotencyKey),
	}
	var result campaign.DraftTouchPlan
	err := service.uow.Within(ctx, func(tx context.Context) error {
		receipt, created, err := service.repository.ReserveDraftCreate(tx, reservation)
		if err != nil {
			return ErrUnavailable
		}
		if !sameReceipt(receipt, reservation) {
			return ErrConflict
		}
		if !created {
			if !receipt.Completed {
				return ErrUnavailable
			}
			readback, err := service.readStrict(tx, reservation.PlanID, command, nil)
			if err != nil {
				return err
			}
			result = readback
			return nil
		}

		draft, err := service.campaigns.LockDraftCampaign(tx, command.CampaignCode)
		if err != nil {
			return classifyFactsError(err)
		}
		if draft.CampaignCode != command.CampaignCode || draft.Version != command.ExpectedCampaignVersion {
			return ErrConflict
		}
		content := campaign.CanonicalContentSnapshot(draft.Steps)
		if draft.ApprovalStatus != campaign.ApprovalDraft || draft.RuntimeStatus != campaign.RuntimeIdle || !campaign.ValidContentSnapshot(content) {
			return campaign.ErrBlockedRedline
		}

		var source campaign.InitiationSourceRef
		var candidates []int64
		var valid bool
		if command.Source.Kind == campaign.InitiationSourceCustomerSelection {
			source, valid = campaign.NewCustomerSelectionSourceRef(command.Source.CustomerIDs)
			candidates = append([]int64(nil), command.Source.CustomerIDs...)
		} else {
			resolved, resolveErr := service.sources.ResolveCampaignTargets(tx, command.Source)
			if resolveErr != nil {
				return classifyFactsError(resolveErr)
			}
			source = campaign.CanonicalInitiationSourceRef(resolved.Source)
			if !command.Source.Matches(source) || resolved.CustomerIDs == nil {
				return campaign.ErrBlockedRedline
			}
			candidates, valid = canonicalCustomerIDs(resolved.CustomerIDs)
		}
		if !valid || len(candidates) == 0 || len(candidates) > campaign.MaximumDraftTouchTargets {
			return campaign.ErrBlockedRedline
		}
		decisions, err := service.eligibility.CheckCampaignEligibility(tx, campaignport.EligibilityRequest{
			Checkpoint:     campaignport.EligibilityCheckpointPreview,
			MaximumTargets: campaignport.MaximumEligibilityTargets,
			CustomerIDs:    candidates,
		})
		if err != nil {
			return classifyFactsError(err)
		}
		targets, exclusions, valid := canonicalTargets(candidates, decisions)
		if !valid {
			return campaign.ErrBlockedRedline
		}
		if len(targets) == 0 {
			return campaign.ErrBlockedRedline
		}
		plan := campaign.DraftTouchPlan{
			ID:              reservation.PlanID,
			CampaignCode:    command.CampaignCode,
			CampaignVersion: draft.Version,
			Source:          source,
			Targets:         campaign.CustomerTargetSnapshot{CustomerIDs: targets, Digest: campaign.CanonicalTargetDigest(source, targets)},
			Content:         content,
			OwnerActorID:    command.Owner.ID,
			Exclusions:      exclusions,
			Review:          campaign.ReviewDraft,
			Version:         1,
			CreatedAt:       now,
			Safety:          campaign.LocalInitiationSafety(),
		}
		if !campaign.ValidDraftTouchPlan(plan) {
			return ErrUnavailable
		}
		if err = service.repository.SaveDraftTouchPlan(tx, plan); err != nil {
			return ErrUnavailable
		}
		event := campaignport.CampaignEvent{
			Type:           createDraftOperation,
			PlanID:         plan.ID,
			CampaignCode:   plan.CampaignCode,
			OwnerActorID:   plan.OwnerActorID,
			TargetDigest:   plan.Targets.Digest,
			TargetCount:    int32(len(plan.Targets.CustomerIDs)),
			OccurredAt:     now,
			IdempotencyKey: createDraftOperation + ":" + plan.ID,
		}
		if err = service.events.AppendCampaignEvent(tx, event); err != nil {
			return ErrUnavailable
		}
		if err = service.repository.CompleteDraftCreate(tx, receipt); err != nil {
			return ErrUnavailable
		}
		readback, err := service.readStrict(tx, reservation.PlanID, command, &plan)
		if err != nil {
			return err
		}
		result = readback
		return nil
	})
	if err != nil {
		return campaign.DraftTouchPlan{}, err
	}
	return campaign.CloneDraftTouchPlan(result), nil
}

func (service *Service) readStrict(tx context.Context, planID string, command campaign.CreateDraftTouchPlanCommand, expected *campaign.DraftTouchPlan) (campaign.DraftTouchPlan, error) {
	readback, err := service.repository.ReadDraftTouchPlan(tx, planID)
	if err != nil || !campaign.ValidDraftTouchPlan(readback) || !matchesCommand(readback, command) {
		return campaign.DraftTouchPlan{}, ErrUnavailable
	}
	if expected != nil && !reflect.DeepEqual(readback, *expected) {
		return campaign.DraftTouchPlan{}, ErrUnavailable
	}
	return campaign.CloneDraftTouchPlan(readback), nil
}

func canonicalCustomerIDs(ids []int64) ([]int64, bool) {
	items := append([]int64(nil), ids...)
	for _, id := range items {
		if id < 1 {
			return nil, false
		}
	}
	sort.Slice(items, func(left, right int) bool { return items[left] < items[right] })
	result := make([]int64, 0, len(items))
	for _, id := range items {
		if len(result) == 0 || result[len(result)-1] != id {
			result = append(result, id)
		}
	}
	return result, true
}

func canonicalTargets(candidates []int64, decisions []campaignport.EligibilityDecision) ([]int64, campaign.PreviewExclusionSummary, bool) {
	if len(candidates) != len(decisions) {
		return nil, campaign.PreviewExclusionSummary{}, false
	}
	result := make([]int64, 0, len(candidates))
	summary := campaign.PreviewExclusionSummary{CandidateCount: int32(len(candidates))}
	for index, candidate := range candidates {
		decision := decisions[index]
		if decision.CustomerID != candidate {
			return nil, campaign.PreviewExclusionSummary{}, false
		}
		if decision.CustomerActive {
			summary.ActiveCustomerCount++
		}
		switch decision.Exclusion {
		case campaignport.EligibilityExclusionNone:
			if !decision.CustomerActive || !decision.Eligible {
				return nil, campaign.PreviewExclusionSummary{}, false
			}
			result = append(result, candidate)
		case campaignport.EligibilityExclusionInactiveCustomer:
			if decision.CustomerActive || decision.Eligible {
				return nil, campaign.PreviewExclusionSummary{}, false
			}
			summary.InactiveExcludedCount++
		case campaignport.EligibilityExclusionContactPolicy:
			if !decision.CustomerActive || decision.Eligible {
				return nil, campaign.PreviewExclusionSummary{}, false
			}
			summary.PolicyExcludedCount++
		default:
			return nil, campaign.PreviewExclusionSummary{}, false
		}
	}
	return result, summary, true
}

func payloadDigest(command campaign.CreateDraftTouchPlanCommand) [sha256.Size]byte {
	payload := struct {
		CampaignCode            string                           `json:"campaign_code"`
		ExpectedCampaignVersion int64                            `json:"expected_campaign_version"`
		Source                  campaign.InitiationSourceRequest `json:"source"`
		OwnerActorID            int64                            `json:"owner_actor_id"`
	}{command.CampaignCode, command.ExpectedCampaignVersion, command.Source, command.Owner.ID}
	raw, err := json.Marshal(payload)
	if err != nil {
		return [sha256.Size]byte{}
	}
	return sha256.Sum256(raw)
}

func sameReceipt(receipt campaignport.CreateReceipt, reservation campaignport.CreateReservation) bool {
	return receipt.ActorID == reservation.ActorID && receipt.PlanID == reservation.PlanID &&
		subtle.ConstantTimeCompare(receipt.KeyDigest[:], reservation.KeyDigest[:]) == 1 &&
		subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) == 1
}

func matchesCommand(plan campaign.DraftTouchPlan, command campaign.CreateDraftTouchPlanCommand) bool {
	return plan.ID == campaign.DraftTouchPlanID(command.Owner.ID, command.CampaignCode, command.IdempotencyKey) &&
		plan.CampaignCode == command.CampaignCode && plan.CampaignVersion == command.ExpectedCampaignVersion &&
		command.Source.Matches(plan.Source) && plan.OwnerActorID == command.Owner.ID
}

func classifyFactsError(err error) error {
	if errors.Is(err, campaignport.ErrSourceFactsUnavailable) {
		return campaign.ErrBlockedRedline
	}
	return ErrUnavailable
}

func ready(service *Service, ctx context.Context) bool {
	return service != nil && service.uow != nil && service.campaigns != nil && service.eligibility != nil && service.repository != nil && service.events != nil && service.now != nil && ctx != nil && ctx.Err() == nil
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	valueOf := reflect.ValueOf(value)
	return (valueOf.Kind() == reflect.Chan || valueOf.Kind() == reflect.Func || valueOf.Kind() == reflect.Interface || valueOf.Kind() == reflect.Map || valueOf.Kind() == reflect.Pointer || valueOf.Kind() == reflect.Slice) && valueOf.IsNil()
}
