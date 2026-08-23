package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
)

const (
	reviewSubmit  = "submit"
	reviewApprove = "approve"
	reviewReject  = "reject"
)

type ReviewHandoffService struct {
	uow    campaignport.UnitOfWork
	repo   campaignport.ReviewRepository
	events campaignport.ReviewEventAppender
	now    func() time.Time
}

func NewReviewHandoffService(uow campaignport.UnitOfWork, repo campaignport.ReviewRepository, events campaignport.ReviewEventAppender) (*ReviewHandoffService, error) {
	if isNil(uow) || isNil(repo) || isNil(events) {
		return nil, campaign.ErrUnavailable
	}
	return &ReviewHandoffService{uow: uow, repo: repo, events: events, now: time.Now}, nil
}
func (s *ReviewHandoffService) Submit(ctx context.Context, command campaign.SubmitTouchPlanReviewCommand) (campaign.TouchPlanReview, error) {
	if !validSubmit(command) {
		return campaign.TouchPlanReview{}, campaign.ErrInvalidArgument
	}
	result, err := s.execute(ctx, reviewSubmit, command.CampaignCode, command.PlanID, command.ExpectedVersion, command.Actor, command.IdempotencyKey, "", func(tx context.Context, review campaign.TouchPlanReview, now time.Time) (campaign.TouchPlanReviewResult, error) {
		if review.Version != command.ExpectedVersion {
			return campaign.TouchPlanReviewResult{}, campaign.ErrConflict
		}
		if review.Status != campaign.TouchPlanReviewDraft {
			return campaign.TouchPlanReviewResult{}, campaign.ErrStateConflict
		}
		review.Status, review.Version, review.SubmittedByActorID, review.SubmittedAt = campaign.TouchPlanReviewPending, review.Version+1, command.Actor.ID, now
		if err := s.repo.SaveTouchPlanReview(tx, review, command.ExpectedVersion); err != nil {
			return campaign.TouchPlanReviewResult{}, err
		}
		return s.finish(tx, reviewSubmit, []string{campaign.ReviewAuditSubmitted}, review, nil, now)
	})
	if err != nil {
		return campaign.TouchPlanReview{}, err
	}
	return result.Review, nil
}
func (s *ReviewHandoffService) Approve(ctx context.Context, command campaign.DecideTouchPlanReviewCommand) (campaign.TouchPlanReviewResult, error) {
	return s.decide(ctx, reviewApprove, campaign.TouchPlanReviewApproved, command)
}
func (s *ReviewHandoffService) Reject(ctx context.Context, command campaign.DecideTouchPlanReviewCommand) (campaign.TouchPlanReviewResult, error) {
	return s.decide(ctx, reviewReject, campaign.TouchPlanReviewRejected, command)
}
func (s *ReviewHandoffService) decide(ctx context.Context, operation string, status campaign.TouchPlanReviewStatus, command campaign.DecideTouchPlanReviewCommand) (campaign.TouchPlanReviewResult, error) {
	if !validDecision(operation, command) {
		return campaign.TouchPlanReviewResult{}, campaign.ErrInvalidArgument
	}
	return s.execute(ctx, operation, command.CampaignCode, command.PlanID, command.ExpectedVersion, command.Actor, command.IdempotencyKey, command.Confirmation, func(tx context.Context, review campaign.TouchPlanReview, now time.Time) (campaign.TouchPlanReviewResult, error) {
		if review.Version != command.ExpectedVersion {
			return campaign.TouchPlanReviewResult{}, campaign.ErrConflict
		}
		if review.Status != campaign.TouchPlanReviewPending {
			return campaign.TouchPlanReviewResult{}, campaign.ErrStateConflict
		}
		review.Status, review.Version, review.ReviewedByActorID, review.ReviewedAt, review.ConfirmationDigest = status, review.Version+1, command.Actor.ID, now, campaign.ReviewConfirmationDigest(command.Confirmation)
		if err := s.repo.SaveTouchPlanReview(tx, review, command.ExpectedVersion); err != nil {
			return campaign.TouchPlanReviewResult{}, err
		}
		var handoff *campaign.TouchPlanHandoff
		auditTypes := []string{campaign.ReviewAuditRejected}
		if status == campaign.TouchPlanReviewApproved {
			value := campaign.TouchPlanHandoff{PlanID: review.PlanID, CampaignCode: review.CampaignCode, ReviewVersion: review.Version, Status: campaign.HandoffPendingOutboundAccept, CreatedAt: now, LocalOnly: true}
			if !campaign.ValidTouchPlanHandoff(value) {
				return campaign.TouchPlanReviewResult{}, campaign.ErrUnavailable
			}
			if err := s.repo.CreateTouchPlanHandoff(tx, value); err != nil {
				return campaign.TouchPlanReviewResult{}, err
			}
			handoff = &value
			auditTypes = []string{campaign.ReviewAuditApproved, campaign.ReviewAuditHandoffCreated}
		}
		return s.finish(tx, operation, auditTypes, review, handoff, now)
	})
}
func (s *ReviewHandoffService) execute(ctx context.Context, operation, campaignCode, planID string, expectedVersion int64, actor campaign.Actor, key, confirmation string, transition func(context.Context, campaign.TouchPlanReview, time.Time) (campaign.TouchPlanReviewResult, error)) (out campaign.TouchPlanReviewResult, err error) {
	if s == nil || s.uow == nil || s.repo == nil || s.events == nil || s.now == nil || ctx == nil || ctx.Err() != nil {
		return out, campaign.ErrUnavailable
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	if now.IsZero() {
		return out, campaign.ErrUnavailable
	}
	reservation := campaignport.ReviewReceiptReservation{ActorID: actor.ID, Operation: operation, KeyDigest: sha256.Sum256([]byte(key)), PayloadDigest: campaign.ReviewPayloadDigest(operation, campaignCode, planID, expectedVersion, confirmation), PlanID: planID, CampaignCode: campaignCode, CreatedAt: now}
	err = s.uow.Within(ctx, func(tx context.Context) error {
		receipt, created, reserveErr := s.repo.ReserveReviewReceipt(tx, reservation)
		if reserveErr != nil {
			return reserveErr
		}
		if !sameReviewReceipt(receipt, reservation) {
			return campaign.ErrIdempotencyConflict
		}
		if !created {
			if receipt.State != campaign.TouchPlanReviewReceiptCompleted || receipt.Result == nil || !validResult(*receipt.Result) {
				return campaign.ErrUnavailable
			}
			out = cloneReviewResult(*receipt.Result)
			return nil
		}
		review, lockErr := s.repo.LockTouchPlanReview(tx, campaignCode, planID)
		if lockErr != nil {
			return lockErr
		}
		if !campaign.ValidTouchPlanReview(review) || review.PlanID != planID || review.CampaignCode != campaignCode {
			return campaign.ErrUnavailable
		}
		result, transitionErr := transition(tx, review, now)
		if transitionErr != nil {
			return transitionErr
		}
		if !validResult(result) {
			return campaign.ErrUnavailable
		}
		if completeErr := s.repo.CompleteReviewReceipt(tx, receipt.ID, result, now); completeErr != nil {
			return completeErr
		}
		out = cloneReviewResult(result)
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
func (s *ReviewHandoffService) finish(ctx context.Context, operation string, auditTypes []string, review campaign.TouchPlanReview, handoff *campaign.TouchPlanHandoff, now time.Time) (campaign.TouchPlanReviewResult, error) {
	ids := make([]int64, 0, len(auditTypes))
	for _, auditType := range auditTypes {
		if !campaign.ValidReviewAuditType(auditType) {
			return campaign.TouchPlanReviewResult{}, campaign.ErrUnavailable
		}
		id, err := s.events.AppendTouchPlanReviewEvent(ctx, campaign.TouchPlanReviewEvent{AuditType: auditType, PlanID: review.PlanID, CampaignCode: review.CampaignCode, ReviewVersion: review.Version, ActorID: reviewActor(review), OccurredAt: now, IdempotencyKey: operation + ":" + review.CampaignCode + ":" + review.PlanID + ":" + string(auditType) + ":" + strconv64(review.Version)})
		if err != nil || id < 1 {
			return campaign.TouchPlanReviewResult{}, campaign.ErrUnavailable
		}
		ids = append(ids, id)
	}
	stored, err := s.repo.ReadTouchPlanReview(ctx, review.CampaignCode, review.PlanID)
	if err != nil || !reflect.DeepEqual(stored, review) {
		return campaign.TouchPlanReviewResult{}, campaign.ErrUnavailable
	}
	if handoff != nil {
		storedHandoff, readErr := s.repo.ReadTouchPlanHandoff(ctx, handoff.CampaignCode, handoff.PlanID)
		if readErr != nil || !reflect.DeepEqual(storedHandoff, *handoff) {
			return campaign.TouchPlanReviewResult{}, campaign.ErrUnavailable
		}
	}
	return campaign.TouchPlanReviewResult{Review: review, Handoff: cloneHandoff(handoff), EventIDs: ids}, nil
}
func (s *ReviewHandoffService) ListRecipients(ctx context.Context, campaignCode, planID string, cursor *campaign.TouchPlanRecipientKeyset, limit int32) (campaign.TouchPlanRecipientPage, error) {
	if s == nil || s.uow == nil || s.repo == nil || ctx == nil || !campaign.ValidCampaignCode(campaignCode) || !campaign.ValidTouchPlanReviewID(planID) || limit < 1 || limit > campaign.MaximumReviewRecipientPage || cursor != nil && (cursor.PlanID != planID || cursor.CustomerID < 1) {
		return campaign.TouchPlanRecipientPage{}, campaign.ErrInvalidArgument
	}
	after := int64(0)
	if cursor != nil {
		after = cursor.CustomerID
	}
	var values []campaign.TouchPlanRecipient
	if err := s.uow.Within(ctx, func(tx context.Context) error {
		review, readErr := s.repo.ReadTouchPlanReview(tx, campaignCode, planID)
		if readErr != nil {
			return readErr
		}
		if !campaign.ValidTouchPlanReview(review) || review.CampaignCode != campaignCode {
			return campaign.ErrUnavailable
		}
		var err error
		values, err = s.repo.ListTouchPlanRecipients(tx, campaignCode, planID, after, limit+1)
		return err
	}); err != nil {
		if errors.Is(err, campaign.ErrNotFound) {
			return campaign.TouchPlanRecipientPage{}, campaign.ErrNotFound
		}
		return campaign.TouchPlanRecipientPage{}, campaign.ErrUnavailable
	}
	for i, value := range values {
		if value.PlanID != planID || value.CustomerID <= after || i > 0 && values[i-1].CustomerID >= value.CustomerID {
			return campaign.TouchPlanRecipientPage{}, campaign.ErrUnavailable
		}
	}
	page := campaign.TouchPlanRecipientPage{Items: append([]campaign.TouchPlanRecipient(nil), values...)}
	if len(page.Items) > int(limit) {
		page.Items = page.Items[:limit]
		last := page.Items[len(page.Items)-1]
		page.Next = &campaign.TouchPlanRecipientKeyset{PlanID: planID, CustomerID: last.CustomerID}
	}
	return page, nil
}
func (s *ReviewHandoffService) GetRecipient(ctx context.Context, campaignCode, planID string, customerID int64) (campaign.TouchPlanRecipient, error) {
	if s == nil || s.uow == nil || s.repo == nil || ctx == nil || !campaign.ValidCampaignCode(campaignCode) || !campaign.ValidTouchPlanReviewID(planID) || customerID < 1 {
		return campaign.TouchPlanRecipient{}, campaign.ErrInvalidArgument
	}
	var value campaign.TouchPlanRecipient
	if err := s.uow.Within(ctx, func(tx context.Context) error {
		var err error
		value, err = s.repo.GetTouchPlanRecipient(tx, campaignCode, planID, customerID)
		return err
	}); err != nil {
		if errors.Is(err, campaign.ErrNotFound) {
			return value, err
		}
		return value, campaign.ErrUnavailable
	}
	if value.PlanID != planID || value.CustomerID != customerID {
		return campaign.TouchPlanRecipient{}, campaign.ErrUnavailable
	}
	return value, nil
}
func (s *ReviewHandoffService) GetReview(ctx context.Context, campaignCode, planID string) (campaign.TouchPlanReviewResult, error) {
	if s == nil || s.uow == nil || s.repo == nil || ctx == nil || !campaign.ValidCampaignCode(campaignCode) || !campaign.ValidTouchPlanReviewID(planID) {
		return campaign.TouchPlanReviewResult{}, campaign.ErrInvalidArgument
	}
	var result campaign.TouchPlanReviewResult
	if err := s.uow.Within(ctx, func(tx context.Context) error {
		review, err := s.repo.ReadTouchPlanReview(tx, campaignCode, planID)
		if err != nil {
			return err
		}
		result.Review = review
		if review.Status == campaign.TouchPlanReviewApproved {
			handoff, err := s.repo.ReadTouchPlanHandoff(tx, campaignCode, planID)
			if err != nil {
				return err
			}
			result.Handoff = &handoff
		}
		return nil
	}); err != nil {
		if errors.Is(err, campaign.ErrNotFound) {
			return campaign.TouchPlanReviewResult{}, err
		}
		return campaign.TouchPlanReviewResult{}, campaign.ErrUnavailable
	}
	if !campaign.ValidTouchPlanReview(result.Review) || result.Review.CampaignCode != campaignCode || result.Handoff != nil && !campaign.ValidTouchPlanHandoff(*result.Handoff) {
		return campaign.TouchPlanReviewResult{}, campaign.ErrUnavailable
	}
	return cloneReviewResult(result), nil
}
func validSubmit(value campaign.SubmitTouchPlanReviewCommand) bool {
	return campaign.ValidCampaignCode(value.CampaignCode) && campaign.ValidTouchPlanReviewID(value.PlanID) && value.ExpectedVersion > 0 && value.Actor.ID > 0 && validReviewKey(value.IdempotencyKey)
}
func validDecision(operation string, value campaign.DecideTouchPlanReviewCommand) bool {
	return validSubmit(campaign.SubmitTouchPlanReviewCommand{CampaignCode: value.CampaignCode, PlanID: value.PlanID, ExpectedVersion: value.ExpectedVersion, Actor: value.Actor, IdempotencyKey: value.IdempotencyKey}) && value.Confirmation == campaign.ReviewConfirmation(operation, value.PlanID)
}
func validReviewKey(value string) bool {
	return len(value) >= 16 && len(value) <= 128 && strings.TrimSpace(value) == value
}
func sameReviewReceipt(value campaign.TouchPlanReviewReceipt, reservation campaignport.ReviewReceiptReservation) bool {
	return value.ID > 0 && value.ActorID == reservation.ActorID && value.Operation == reservation.Operation && value.PlanID == reservation.PlanID && value.CampaignCode == reservation.CampaignCode && value.KeyDigest == reservation.KeyDigest && value.PayloadDigest == reservation.PayloadDigest
}
func validResult(value campaign.TouchPlanReviewResult) bool {
	if !campaign.ValidTouchPlanReview(value.Review) || len(value.EventIDs) < 1 {
		return false
	}
	for i, id := range value.EventIDs {
		if id < 1 || i > 0 && value.EventIDs[i-1] >= id {
			return false
		}
	}
	return value.Handoff == nil && len(value.EventIDs) == 1 || value.Handoff != nil && len(value.EventIDs) == 2 && campaign.ValidTouchPlanHandoff(*value.Handoff) && value.Handoff.PlanID == value.Review.PlanID && value.Handoff.ReviewVersion == value.Review.Version && value.Review.Status == campaign.TouchPlanReviewApproved
}
func cloneReviewResult(value campaign.TouchPlanReviewResult) campaign.TouchPlanReviewResult {
	value.EventIDs = append([]int64(nil), value.EventIDs...)
	value.Handoff = cloneHandoff(value.Handoff)
	return value
}
func cloneHandoff(value *campaign.TouchPlanHandoff) *campaign.TouchPlanHandoff {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
func reviewActor(value campaign.TouchPlanReview) int64 {
	if value.ReviewedByActorID > 0 {
		return value.ReviewedByActorID
	}
	return value.SubmittedByActorID
}
func strconv64(value int64) string { return fmt.Sprintf("%d", value) }
