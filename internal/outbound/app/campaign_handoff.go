package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	campaignHandoffReceiptReserved  = "reserved"
	campaignHandoffReceiptCompleted = "completed"
)

type AcceptCampaignHandoffCommand struct {
	CampaignCode          string
	PlanID                string
	ExpectedReviewVersion int64
	ActorID               int64
	IdempotencyKey        string
}

type CampaignHandoffService struct {
	uow    platformport.UnitOfWork
	source outboundport.ApprovedCampaignHandoffSource
	repo   outboundport.CampaignHandoffRepository
	events outboundport.CampaignHandoffEventAppender
	now    func() time.Time
}

func NewCampaignHandoffService(
	uow platformport.UnitOfWork,
	source outboundport.ApprovedCampaignHandoffSource,
	repo outboundport.CampaignHandoffRepository,
	events outboundport.CampaignHandoffEventAppender,
) (*CampaignHandoffService, error) {
	if nilDependency(uow) || nilDependency(source) || nilDependency(repo) || nilDependency(events) {
		return nil, outbound.ErrCampaignHandoffUnavailable
	}
	return &CampaignHandoffService{uow: uow, source: source, repo: repo, events: events, now: time.Now}, nil
}

func (service *CampaignHandoffService) Accept(ctx context.Context, command AcceptCampaignHandoffCommand) (outbound.CampaignHandoffSummary, error) {
	if ctx == nil || !validAcceptCampaignHandoff(command) || service == nil || nilDependency(service.uow) ||
		nilDependency(service.source) || nilDependency(service.repo) || nilDependency(service.events) || service.now == nil {
		return outbound.CampaignHandoffSummary{}, outbound.ErrCampaignHandoffInvalid
	}
	now := service.now().UTC().Truncate(time.Microsecond)
	keyDigest := sha256.Sum256([]byte(command.IdempotencyKey))
	payloadDigest := outbound.CampaignHandoffPayloadDigest(command.CampaignCode, command.PlanID, command.ExpectedReviewVersion)
	reservation := outboundport.CampaignHandoffReservation{
		ActorID: command.ActorID, KeyDigest: keyDigest, PayloadDigest: payloadDigest,
		CampaignCode: command.CampaignCode, PlanID: command.PlanID, CreatedAt: now,
	}
	var result outbound.CampaignHandoffSummary
	err := service.uow.Within(ctx, func(tx context.Context) error {
		receipt, fresh, err := service.repo.ReserveCampaignHandoff(tx, reservation)
		if err != nil {
			return err
		}
		if !sameCampaignHandoffReceipt(receipt, reservation) {
			return outbound.ErrCampaignHandoffUnavailable
		}
		if !fresh {
			if receipt.State != campaignHandoffReceiptCompleted || receipt.Result == nil || !outbound.ValidCampaignHandoffSummary(*receipt.Result) {
				return outbound.ErrCampaignHandoffUnavailable
			}
			result = cloneCampaignHandoffSummary(*receipt.Result)
			return nil
		}
		if receipt.State != campaignHandoffReceiptReserved || receipt.Result != nil {
			return outbound.ErrCampaignHandoffUnavailable
		}
		snapshot, err := service.source.LockApprovedCampaignHandoff(tx, command.CampaignCode, command.PlanID)
		if err != nil {
			return err
		}
		if !validApprovedCampaignHandoffSnapshot(snapshot, command) {
			return outbound.ErrCampaignHandoffConflict
		}
		handoffID, err := service.repo.CreateAcceptedCampaignHandoff(tx, cloneApprovedCampaignHandoffSnapshot(snapshot), command.ActorID, now)
		if err != nil {
			return err
		}
		if handoffID < 1 {
			return outbound.ErrCampaignHandoffUnavailable
		}
		eventID, err := service.events.AppendCampaignHandoffFact(tx, outboundport.CampaignHandoffEvent{
			HandoffID: handoffID, CampaignCode: command.CampaignCode, PlanID: command.PlanID,
			ReviewVersion: command.ExpectedReviewVersion, TargetDigest: snapshot.TargetDigest,
			ContentDigest: snapshot.ContentDigest, TargetCount: int32(len(snapshot.CustomerIDs)), StepCount: int32(len(snapshot.Steps)),
			ActorID: command.ActorID, OccurredAt: now,
			IdempotencyKey: "outbound.campaign_handoff.accepted:" + strconv.FormatInt(handoffID, 10),
		})
		if err != nil || eventID < 1 {
			return errors.Join(outbound.ErrCampaignHandoffUnavailable, err)
		}
		stored, err := service.repo.ReadAcceptedCampaignHandoff(tx, command.CampaignCode, command.PlanID)
		if err != nil || !sameAcceptedCampaignHandoff(stored, snapshot, handoffID, command.ActorID, now) {
			return errors.Join(outbound.ErrCampaignHandoffUnavailable, err)
		}
		result = outbound.SummaryOf(stored)
		if err = service.repo.CompleteCampaignHandoffReceipt(tx, receipt.ID, eventID, result, now); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, outbound.ErrCampaignHandoffInvalid), errors.Is(err, outbound.ErrCampaignHandoffNotFound),
			errors.Is(err, outbound.ErrCampaignHandoffConflict), errors.Is(err, outbound.ErrCampaignHandoffIdempotencyConflict):
			return outbound.CampaignHandoffSummary{}, err
		default:
			return outbound.CampaignHandoffSummary{}, errors.Join(outbound.ErrCampaignHandoffUnavailable, err)
		}
	}
	if !outbound.ValidCampaignHandoffSummary(result) || result.CampaignCode != command.CampaignCode || result.PlanID != command.PlanID {
		return outbound.CampaignHandoffSummary{}, outbound.ErrCampaignHandoffUnavailable
	}
	return cloneCampaignHandoffSummary(result), nil
}

func (service *CampaignHandoffService) Get(ctx context.Context, campaignCode, planID string) (outbound.CampaignHandoffSummary, error) {
	if ctx == nil || service == nil || nilDependency(service.uow) || nilDependency(service.repo) || !outbound.ValidCampaignHandoffIdentity(campaignCode, planID) {
		return outbound.CampaignHandoffSummary{}, outbound.ErrCampaignHandoffInvalid
	}
	var result outbound.CampaignHandoffSummary
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		result, err = service.repo.ReadCampaignHandoffSummary(tx, campaignCode, planID)
		return err
	})
	if err != nil {
		if errors.Is(err, outbound.ErrCampaignHandoffNotFound) {
			return outbound.CampaignHandoffSummary{}, err
		}
		return outbound.CampaignHandoffSummary{}, errors.Join(outbound.ErrCampaignHandoffUnavailable, err)
	}
	if !outbound.ValidCampaignHandoffSummary(result) || result.CampaignCode != campaignCode || result.PlanID != planID {
		return outbound.CampaignHandoffSummary{}, outbound.ErrCampaignHandoffUnavailable
	}
	return cloneCampaignHandoffSummary(result), nil
}

func validAcceptCampaignHandoff(command AcceptCampaignHandoffCommand) bool {
	return outbound.ValidCampaignHandoffIdentity(command.CampaignCode, command.PlanID) && command.ExpectedReviewVersion >= 3 &&
		command.ActorID > 0 && len(command.IdempotencyKey) >= 16 && len(command.IdempotencyKey) <= 128 &&
		strings.TrimSpace(command.IdempotencyKey) == command.IdempotencyKey
}

func validApprovedCampaignHandoffSnapshot(snapshot outboundport.ApprovedCampaignHandoffSnapshot, command AcceptCampaignHandoffCommand) bool {
	if snapshot.CampaignCode != command.CampaignCode || snapshot.PlanID != command.PlanID || snapshot.ReviewVersion != command.ExpectedReviewVersion ||
		len(snapshot.CustomerIDs) < 1 || len(snapshot.CustomerIDs) > outbound.MaximumCampaignHandoffTargets ||
		len(snapshot.Steps) < 1 || len(snapshot.Steps) > outbound.MaximumCampaignHandoffSteps || snapshot.ApprovedAt.IsZero() {
		return false
	}
	links, valid := outbound.CanonicalCampaignHandoffLinks(snapshot.CustomerIDs)
	if !valid || len(links) != len(snapshot.CustomerIDs) {
		return false
	}
	probe := outbound.AcceptedCampaignHandoff{
		ID: 1, CampaignCode: snapshot.CampaignCode, PlanID: snapshot.PlanID, ReviewVersion: snapshot.ReviewVersion,
		SourceDigest: snapshot.SourceDigest, TargetDigest: snapshot.TargetDigest, ContentDigest: snapshot.ContentDigest,
		TargetCount: int32(len(snapshot.CustomerIDs)), StepCount: int32(len(snapshot.Steps)), Status: outbound.CampaignHandoffHeld,
		AcceptedBy: 1, AcceptedAt: time.Unix(1, 0).UTC(), Safety: outbound.LocalCampaignHandoffSafety(), Steps: snapshot.Steps, Links: links,
	}
	return outbound.ValidAcceptedCampaignHandoff(probe)
}

func sameAcceptedCampaignHandoff(value outbound.AcceptedCampaignHandoff, snapshot outboundport.ApprovedCampaignHandoffSnapshot, id, actorID int64, acceptedAt time.Time) bool {
	if !outbound.ValidAcceptedCampaignHandoff(value) || value.ID != id || value.CampaignCode != snapshot.CampaignCode || value.PlanID != snapshot.PlanID ||
		value.ReviewVersion != snapshot.ReviewVersion || value.SourceDigest != snapshot.SourceDigest || value.TargetDigest != snapshot.TargetDigest ||
		value.ContentDigest != snapshot.ContentDigest || value.AcceptedBy != actorID || !value.AcceptedAt.Equal(acceptedAt) || !reflect.DeepEqual(value.Steps, snapshot.Steps) {
		return false
	}
	links, valid := outbound.CanonicalCampaignHandoffLinks(snapshot.CustomerIDs)
	return valid && reflect.DeepEqual(value.Links, links)
}

func sameCampaignHandoffReceipt(value outboundport.CampaignHandoffReceipt, reservation outboundport.CampaignHandoffReservation) bool {
	return value.ID > 0 && value.ActorID == reservation.ActorID && value.KeyDigest == reservation.KeyDigest && value.PayloadDigest == reservation.PayloadDigest &&
		value.CampaignCode == reservation.CampaignCode && value.PlanID == reservation.PlanID
}

func cloneApprovedCampaignHandoffSnapshot(value outboundport.ApprovedCampaignHandoffSnapshot) outboundport.ApprovedCampaignHandoffSnapshot {
	value.CustomerIDs = append([]int64(nil), value.CustomerIDs...)
	value.Steps = append([]outbound.CampaignHandoffStep(nil), value.Steps...)
	return value
}

func cloneCampaignHandoffSummary(value outbound.CampaignHandoffSummary) outbound.CampaignHandoffSummary {
	return value
}

func nilDependency(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	switch ref.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return ref.IsNil()
	default:
		return false
	}
}
