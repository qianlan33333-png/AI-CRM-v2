// Package app implements the non-shared Campaign initiation snapshot seam.
package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"sort"
	"strings"
	"time"

	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
)

var (
	ErrInvalidCommand = campaign.ErrInvalidArgument
	ErrUnavailable    = campaign.ErrUnavailable
	ErrConflict       = campaign.ErrConflict
)

const createDraftOperation = "campaign.draft_touch_plan.created"

const draftTouchPlanCursorMaximumBytes = 512

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
			if errors.Is(err, campaign.ErrIdempotencyConflict) {
				return ErrConflict
			}
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
			EvaluatedAt:    now,
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
			ContentDigest:  plan.Content.Digest,
			OccurredAt:     now,
			IdempotencyKey: createDraftOperation + ":" + plan.ID,
		}
		eventID, appendErr := service.events.AppendCampaignEvent(tx, event)
		if appendErr != nil || eventID < 1 {
			return ErrUnavailable
		}
		if err = service.repository.CompleteDraftCreate(tx, receipt, eventID); err != nil {
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

// ListDraftTouchPlans reads only recipient-safe snapshot summaries. It still
// runs through the UnitOfWork so repository reads cannot bypass transaction
// context, matching the strict-readback boundary used by create/replay.
func (service *Service) ListDraftTouchPlans(ctx context.Context, campaignCode, cursor string, limit int32) (campaign.DraftTouchPlanPage, error) {
	if !ready(service, ctx) {
		return campaign.DraftTouchPlanPage{}, ErrUnavailable
	}
	if !validCampaignCode(campaignCode) {
		return campaign.DraftTouchPlanPage{}, ErrInvalidCommand
	}
	if limit == 0 {
		limit = campaignport.DefaultDraftTouchPlanPageLimit
	}
	if limit < 1 || limit > campaignport.MaximumDraftTouchPlanPageLimit {
		return campaign.DraftTouchPlanPage{}, ErrInvalidCommand
	}
	after, err := decodeDraftTouchPlanCursor(cursor, campaignCode)
	if err != nil {
		return campaign.DraftTouchPlanPage{}, ErrInvalidCommand
	}
	result := campaign.DraftTouchPlanPage{Items: []campaign.DraftTouchPlanSummary{}}
	err = service.uow.Within(ctx, func(tx context.Context) error {
		plans, err := service.repository.ListDraftTouchPlanSummaries(tx, campaignCode, after, limit+1)
		if err != nil {
			return err
		}
		for _, plan := range plans {
			if !campaign.ValidDraftTouchPlanSummary(plan) || plan.CampaignCode != campaignCode {
				return ErrUnavailable
			}
		}
		if len(plans) > int(limit) {
			page := plans[:limit]
			next, encodeErr := encodeDraftTouchPlanCursor(campaignCode, page[len(page)-1])
			if encodeErr != nil {
				return ErrUnavailable
			}
			result.Items = campaign.CloneDraftTouchPlanSummaries(page)
			result.NextCursor = next
			return nil
		}
		result.Items = campaign.CloneDraftTouchPlanSummaries(plans)
		return nil
	})
	if err != nil {
		return campaign.DraftTouchPlanPage{}, err
	}
	result.Items = campaign.CloneDraftTouchPlanSummaries(result.Items)
	return result, nil
}

// GetDraftTouchPlan retains the full immutable snapshot for a trusted
// Campaign transport projection. HTTP deliberately omits Targets.CustomerIDs
// until the separate recipient-read contract is introduced.
func (service *Service) GetDraftTouchPlan(ctx context.Context, campaignCode, planID string) (campaign.DraftTouchPlan, error) {
	if !ready(service, ctx) {
		return campaign.DraftTouchPlan{}, ErrUnavailable
	}
	if !validCampaignCode(campaignCode) || !validDraftTouchPlanID(planID) {
		return campaign.DraftTouchPlan{}, ErrInvalidCommand
	}
	var result campaign.DraftTouchPlan
	err := service.uow.Within(ctx, func(tx context.Context) error {
		plan, err := service.repository.ReadDraftTouchPlan(tx, campaignCode, planID)
		if err != nil {
			return err
		}
		if !campaign.ValidDraftTouchPlan(plan) || plan.CampaignCode != campaignCode {
			return ErrUnavailable
		}
		result = campaign.CloneDraftTouchPlan(plan)
		return nil
	})
	if err != nil {
		return campaign.DraftTouchPlan{}, err
	}
	return campaign.CloneDraftTouchPlan(result), nil
}

func (service *Service) readStrict(tx context.Context, planID string, command campaign.CreateDraftTouchPlanCommand, expected *campaign.DraftTouchPlan) (campaign.DraftTouchPlan, error) {
	readback, err := service.repository.ReadDraftTouchPlan(tx, command.CampaignCode, planID)
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
	for index := 1; index < len(items); index++ {
		if items[index-1] == items[index] {
			return nil, false
		}
	}
	return items, true
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
	return receipt.ID > 0 && receipt.ActorID == reservation.ActorID && receipt.PlanID == reservation.PlanID &&
		subtle.ConstantTimeCompare(receipt.KeyDigest[:], reservation.KeyDigest[:]) == 1 &&
		subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) == 1
}

func matchesCommand(plan campaign.DraftTouchPlan, command campaign.CreateDraftTouchPlanCommand) bool {
	return plan.ID == campaign.DraftTouchPlanID(command.Owner.ID, command.CampaignCode, command.IdempotencyKey) &&
		plan.CampaignCode == command.CampaignCode && plan.CampaignVersion == command.ExpectedCampaignVersion &&
		command.Source.Matches(plan.Source) && plan.OwnerActorID == command.Owner.ID
}

func classifyFactsError(err error) error {
	if errors.Is(err, campaign.ErrNotFound) {
		return campaign.ErrNotFound
	}
	if errors.Is(err, campaignport.ErrSourceFactsUnavailable) {
		return campaign.ErrBlockedRedline
	}
	return ErrUnavailable
}

func ready(service *Service, ctx context.Context) bool {
	return service != nil && service.uow != nil && service.campaigns != nil && service.eligibility != nil && service.repository != nil && service.events != nil && service.now != nil && ctx != nil && ctx.Err() == nil
}

func validCampaignCode(value string) bool {
	return campaign.ValidCampaignCode(value)
}

func validDraftTouchPlanID(value string) bool {
	return campaign.ValidDraftTouchPlanID(value)
}

type draftTouchPlanCursor struct {
	Version      int    `json:"v"`
	Operation    string `json:"op"`
	CampaignCode string `json:"campaign_code"`
	CreatedAt    string `json:"created_at"`
	PlanID       string `json:"plan_id"`
}

func encodeDraftTouchPlanCursor(campaignCode string, plan campaign.DraftTouchPlanSummary) (string, error) {
	if !validCampaignCode(campaignCode) || plan.CampaignCode != campaignCode || !campaign.ValidDraftTouchPlanSummary(plan) {
		return "", ErrInvalidCommand
	}
	raw, err := json.Marshal(draftTouchPlanCursor{Version: 1, Operation: "listDraftTouchPlans", CampaignCode: campaignCode,
		CreatedAt: plan.CreatedAt.UTC().Format(time.RFC3339Nano), PlanID: plan.ID})
	if err != nil {
		return "", ErrUnavailable
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeDraftTouchPlanCursor(raw, campaignCode string) (*campaignport.DraftTouchPlanKeyset, error) {
	if raw == "" {
		return nil, nil
	}
	if len(raw) > draftTouchPlanCursorMaximumBytes || strings.Contains(raw, "=") {
		return nil, ErrInvalidCommand
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != raw {
		return nil, ErrInvalidCommand
	}
	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.DisallowUnknownFields()
	var cursor draftTouchPlanCursor
	if decoder.Decode(&cursor) != nil || decoder.Decode(&struct{}{}) != io.EOF || cursor.Version != 1 ||
		cursor.Operation != "listDraftTouchPlans" || cursor.CampaignCode != campaignCode || !campaign.ValidDraftTouchPlanID(cursor.PlanID) {
		return nil, ErrInvalidCommand
	}
	createdAt, err := time.Parse(time.RFC3339Nano, cursor.CreatedAt)
	if err != nil || createdAt.IsZero() || createdAt.Location() != time.UTC || createdAt.UTC().Format(time.RFC3339Nano) != cursor.CreatedAt {
		return nil, ErrInvalidCommand
	}
	return &campaignport.DraftTouchPlanKeyset{CreatedAt: createdAt.UTC(), PlanID: cursor.PlanID}, nil
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	valueOf := reflect.ValueOf(value)
	return (valueOf.Kind() == reflect.Chan || valueOf.Kind() == reflect.Func || valueOf.Kind() == reflect.Interface || valueOf.Kind() == reflect.Map || valueOf.Kind() == reflect.Pointer || valueOf.Kind() == reflect.Slice) && valueOf.IsNil()
}
