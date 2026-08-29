package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"reflect"
	"time"

	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
)

const (
	recipientReviewMessageOverride = "recipient_message_override"
	recipientReviewApprove         = "recipient_approve"
	recipientReviewReject          = "recipient_reject"
)

type RecipientReviewService struct {
	uow    campaignport.UnitOfWork
	repo   campaignport.RecipientReviewRepository
	events campaignport.RecipientReviewEventAppender
	now    func() time.Time
}

func NewRecipientReviewService(uow campaignport.UnitOfWork, repo campaignport.RecipientReviewRepository, events campaignport.RecipientReviewEventAppender) (*RecipientReviewService, error) {
	if isNil(uow) || isNil(repo) || isNil(events) {
		return nil, campaign.ErrUnavailable
	}
	return &RecipientReviewService{uow: uow, repo: repo, events: events, now: time.Now}, nil
}

func (s *RecipientReviewService) ListCampaignMembers(ctx context.Context, campaignCode string, status campaign.TouchPlanRecipientReviewStatus, limit, offset int32) (campaign.CampaignMemberStatusPage, error) {
	if s == nil || s.uow == nil || s.repo == nil || ctx == nil || ctx.Err() != nil {
		return campaign.CampaignMemberStatusPage{}, campaign.ErrUnavailable
	}
	if !campaign.ValidCampaignCode(campaignCode) || status != "" && !status.Valid() || limit < 1 || limit > campaign.MaximumCampaignMemberPage || offset < 0 {
		return campaign.CampaignMemberStatusPage{}, campaign.ErrInvalidArgument
	}
	var snapshot campaign.CampaignMemberStatusSnapshot
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var readErr error
		snapshot, readErr = s.repo.ListLatestCampaignMemberStatuses(tx, campaignCode, status, limit, offset)
		return readErr
	})
	if err != nil {
		if errors.Is(err, campaign.ErrNotFound) {
			return campaign.CampaignMemberStatusPage{}, campaign.ErrNotFound
		}
		return campaign.CampaignMemberStatusPage{}, campaign.ErrUnavailable
	}
	if !validCampaignMemberStatusSnapshot(snapshot, status, limit) {
		return campaign.CampaignMemberStatusPage{}, campaign.ErrUnavailable
	}
	return campaign.CampaignMemberStatusPage{
		PlanID: snapshot.PlanID,
		Items:  append([]campaign.CampaignMemberStatus(nil), snapshot.Items...),
		Total:  snapshot.Total,
		Limit:  limit,
		Offset: offset,
		Safety: campaign.LocalInitiationSafety(),
	}, nil
}

func validCampaignMemberStatusSnapshot(value campaign.CampaignMemberStatusSnapshot, filter campaign.TouchPlanRecipientReviewStatus, limit int32) bool {
	if value.Total < 0 || len(value.Items) > int(limit) || value.PlanID == "" && (value.Total != 0 || len(value.Items) != 0) || value.PlanID != "" && !campaign.ValidTouchPlanReviewID(value.PlanID) {
		return false
	}
	for index, item := range value.Items {
		if item.PlanID != value.PlanID || item.CustomerID < 1 || !item.Status.Valid() || filter != "" && item.Status != filter || index > 0 && value.Items[index-1].CustomerID >= item.CustomerID {
			return false
		}
	}
	return int64(len(value.Items)) <= value.Total
}

func (s *RecipientReviewService) Get(ctx context.Context, campaignCode, planID string, customerID int64) (campaign.TouchPlanRecipientReview, error) {
	if s == nil || s.repo == nil || ctx == nil || ctx.Err() != nil {
		return campaign.TouchPlanRecipientReview{}, campaign.ErrUnavailable
	}
	if !campaign.ValidCampaignCode(campaignCode) || !campaign.ValidTouchPlanReviewID(planID) || customerID < 1 {
		return campaign.TouchPlanRecipientReview{}, campaign.ErrInvalidArgument
	}
	value, err := s.repo.ReadTouchPlanRecipientReview(ctx, campaignCode, planID, customerID)
	if err != nil {
		if errors.Is(err, campaign.ErrNotFound) {
			return campaign.TouchPlanRecipientReview{}, err
		}
		return campaign.TouchPlanRecipientReview{}, campaign.ErrUnavailable
	}
	if !campaign.ValidTouchPlanRecipientReview(value) || value.CampaignCode != campaignCode || value.PlanID != planID || value.CustomerID != customerID {
		return campaign.TouchPlanRecipientReview{}, campaign.ErrUnavailable
	}
	return value, nil
}

func (s *RecipientReviewService) SaveMessageOverride(ctx context.Context, command campaign.SaveTouchPlanRecipientMessageOverrideCommand) (campaign.TouchPlanRecipientReviewResult, error) {
	if !validRecipientOverride(command) {
		return campaign.TouchPlanRecipientReviewResult{}, campaign.ErrInvalidArgument
	}
	return s.execute(ctx, recipientReviewMessageOverride, command.CampaignCode, command.PlanID, command.CustomerID, command.ExpectedPlanVersion, command.ExpectedRecipientVersion, command.Actor, command.IdempotencyKey, command.MessageOverride, func(review campaign.TouchPlanRecipientReview, found bool, now time.Time) (campaign.TouchPlanRecipientReview, error) {
		if found && review.Status != campaign.TouchPlanRecipientReviewPending {
			return campaign.TouchPlanRecipientReview{}, campaign.ErrStateConflict
		}
		review.MessageOverride = command.MessageOverride
		review.Status = campaign.TouchPlanRecipientReviewPending
		review.Version++
		review.UpdatedByActorID, review.UpdatedAt, review.Safety = command.Actor.ID, now, campaign.LocalInitiationSafety()
		return review, nil
	})
}

func (s *RecipientReviewService) Approve(ctx context.Context, command campaign.DecideTouchPlanRecipientCommand) (campaign.TouchPlanRecipientReviewResult, error) {
	return s.decide(ctx, recipientReviewApprove, campaign.TouchPlanRecipientReviewApproved, command)
}

func (s *RecipientReviewService) Reject(ctx context.Context, command campaign.DecideTouchPlanRecipientCommand) (campaign.TouchPlanRecipientReviewResult, error) {
	return s.decide(ctx, recipientReviewReject, campaign.TouchPlanRecipientReviewRejected, command)
}

func (s *RecipientReviewService) decide(ctx context.Context, operation string, status campaign.TouchPlanRecipientReviewStatus, command campaign.DecideTouchPlanRecipientCommand) (campaign.TouchPlanRecipientReviewResult, error) {
	if !validRecipientDecision(command) {
		return campaign.TouchPlanRecipientReviewResult{}, campaign.ErrInvalidArgument
	}
	return s.execute(ctx, operation, command.CampaignCode, command.PlanID, command.CustomerID, command.ExpectedPlanVersion, command.ExpectedRecipientVersion, command.Actor, command.IdempotencyKey, "", func(review campaign.TouchPlanRecipientReview, found bool, now time.Time) (campaign.TouchPlanRecipientReview, error) {
		if found && review.Status != campaign.TouchPlanRecipientReviewPending {
			return campaign.TouchPlanRecipientReview{}, campaign.ErrStateConflict
		}
		review.Status, review.Version = status, review.Version+1
		review.UpdatedByActorID, review.UpdatedAt, review.Safety = command.Actor.ID, now, campaign.LocalInitiationSafety()
		return review, nil
	})
}

func (s *RecipientReviewService) execute(ctx context.Context, operation, campaignCode, planID string, customerID, expectedPlanVersion, expectedRecipientVersion int64, actor campaign.Actor, key, messageOverride string, transition func(campaign.TouchPlanRecipientReview, bool, time.Time) (campaign.TouchPlanRecipientReview, error)) (out campaign.TouchPlanRecipientReviewResult, err error) {
	if s == nil || s.uow == nil || s.repo == nil || s.events == nil || s.now == nil || ctx == nil || ctx.Err() != nil {
		return out, campaign.ErrUnavailable
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	if now.IsZero() {
		return out, campaign.ErrUnavailable
	}
	reservation := campaignport.RecipientReviewReceiptReservation{ActorID: actor.ID, Operation: operation, KeyDigest: sha256.Sum256([]byte(key)), PayloadDigest: campaign.TouchPlanRecipientReviewPayloadDigest(operation, campaignCode, planID, customerID, expectedPlanVersion, expectedRecipientVersion, messageOverride), PlanID: planID, CampaignCode: campaignCode, CustomerID: customerID, CreatedAt: now}
	err = s.uow.Within(ctx, func(tx context.Context) error {
		receipt, created, reserveErr := s.repo.ReserveTouchPlanRecipientReviewReceipt(tx, reservation)
		if reserveErr != nil {
			return reserveErr
		}
		if !sameRecipientReviewReceipt(receipt, reservation) {
			return campaign.ErrIdempotencyConflict
		}
		if !created {
			if receipt.State != campaign.TouchPlanRecipientReviewReceiptCompleted || receipt.Result == nil || !validRecipientReviewResult(*receipt.Result) {
				return campaign.ErrUnavailable
			}
			out = *receipt.Result
			return nil
		}
		planReview, lockErr := s.repo.LockTouchPlanReview(tx, campaignCode, planID)
		if lockErr != nil {
			return lockErr
		}
		if !campaign.ValidTouchPlanReview(planReview) || planReview.PlanID != planID || planReview.CampaignCode != campaignCode {
			return campaign.ErrUnavailable
		}
		if planReview.Version != expectedPlanVersion {
			return campaign.ErrConflict
		}
		if planReview.Status != campaign.TouchPlanReviewDraft && planReview.Status != campaign.TouchPlanReviewPending {
			return campaign.ErrStateConflict
		}
		if operation != recipientReviewMessageOverride && planReview.Status != campaign.TouchPlanReviewPending {
			return campaign.ErrStateConflict
		}
		recipient, recipientErr := s.repo.GetTouchPlanRecipient(tx, campaignCode, planID, customerID)
		if recipientErr != nil {
			return recipientErr
		}
		if recipient.PlanID != planID || recipient.CustomerID != customerID {
			return campaign.ErrUnavailable
		}
		value, found, reviewErr := s.repo.LockTouchPlanRecipientReview(tx, campaignCode, planID, customerID)
		if reviewErr != nil {
			return reviewErr
		}
		if !found {
			value = campaign.TouchPlanRecipientReview{PlanID: planID, CampaignCode: campaignCode, CustomerID: customerID}
		} else if !campaign.ValidTouchPlanRecipientReview(value) || value.PlanID != planID || value.CampaignCode != campaignCode || value.CustomerID != customerID {
			return campaign.ErrUnavailable
		}
		if value.Version != expectedRecipientVersion {
			return campaign.ErrConflict
		}
		value, transitionErr := transition(value, found, now)
		if transitionErr != nil {
			return transitionErr
		}
		if !campaign.ValidTouchPlanRecipientReview(value) {
			return campaign.ErrUnavailable
		}
		if saveErr := s.repo.SaveTouchPlanRecipientReview(tx, value, expectedRecipientVersion); saveErr != nil {
			return saveErr
		}
		eventID, eventErr := s.events.AppendTouchPlanRecipientReviewEvent(tx, campaign.TouchPlanRecipientReviewEvent{AuditType: recipientReviewAudit(operation), PlanID: planID, CampaignCode: campaignCode, CustomerID: customerID, RecipientVersion: value.Version, ActorID: actor.ID, OccurredAt: now, IdempotencyKey: operation + ":" + campaignCode + ":" + planID + ":" + strconv64(customerID) + ":" + strconv64(value.Version)})
		if eventErr != nil || eventID < 1 {
			return campaign.ErrUnavailable
		}
		stored, readErr := s.repo.ReadTouchPlanRecipientReview(tx, campaignCode, planID, customerID)
		if readErr != nil || !reflect.DeepEqual(stored, value) {
			return campaign.ErrUnavailable
		}
		result := campaign.TouchPlanRecipientReviewResult{Review: value, EventID: eventID}
		if completeErr := s.repo.CompleteTouchPlanRecipientReviewReceipt(tx, receipt.ID, result, now); completeErr != nil {
			return completeErr
		}
		out = result
		return nil
	})
	if err != nil {
		if errors.Is(err, campaign.ErrConflict) || errors.Is(err, campaign.ErrStateConflict) || errors.Is(err, campaign.ErrIdempotencyConflict) || errors.Is(err, campaign.ErrNotFound) {
			return out, err
		}
		return out, campaign.ErrUnavailable
	}
	return out, nil
}

func validRecipientOverride(value campaign.SaveTouchPlanRecipientMessageOverrideCommand) bool {
	return validRecipientCommand(value.CampaignCode, value.PlanID, value.CustomerID, value.ExpectedPlanVersion, value.ExpectedRecipientVersion, value.Actor, value.IdempotencyKey) && campaign.ValidTouchPlanRecipientMessageOverride(value.MessageOverride)
}

func validRecipientDecision(value campaign.DecideTouchPlanRecipientCommand) bool {
	return validRecipientCommand(value.CampaignCode, value.PlanID, value.CustomerID, value.ExpectedPlanVersion, value.ExpectedRecipientVersion, value.Actor, value.IdempotencyKey)
}

func validRecipientCommand(campaignCode, planID string, customerID, expectedPlanVersion, expectedRecipientVersion int64, actor campaign.Actor, key string) bool {
	return campaign.ValidCampaignCode(campaignCode) && campaign.ValidTouchPlanReviewID(planID) && customerID > 0 && expectedPlanVersion > 0 && expectedRecipientVersion >= 0 && actor.ID > 0 && validReviewKey(key)
}

func sameRecipientReviewReceipt(value campaign.TouchPlanRecipientReviewReceipt, reservation campaignport.RecipientReviewReceiptReservation) bool {
	return value.ID > 0 && value.ActorID == reservation.ActorID && value.Operation == reservation.Operation && value.KeyDigest == reservation.KeyDigest && value.PayloadDigest == reservation.PayloadDigest && value.PlanID == reservation.PlanID && value.CampaignCode == reservation.CampaignCode && value.CustomerID == reservation.CustomerID
}

func validRecipientReviewResult(value campaign.TouchPlanRecipientReviewResult) bool {
	return campaign.ValidTouchPlanRecipientReview(value.Review) && value.EventID > 0
}

func recipientReviewAudit(operation string) string {
	switch operation {
	case recipientReviewMessageOverride:
		return campaign.RecipientReviewAuditMessageOverridden
	case recipientReviewApprove:
		return campaign.RecipientReviewAuditApproved
	case recipientReviewReject:
		return campaign.RecipientReviewAuditRejected
	default:
		return ""
	}
}
