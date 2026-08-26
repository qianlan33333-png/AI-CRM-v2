package store

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	campaigndb "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/store/generated"
)

var _ campaignport.RecipientReviewRepository = (*Repository)(nil)

func (r *Repository) ReserveTouchPlanRecipientReviewReceipt(ctx context.Context, reservation campaignport.RecipientReviewReceiptReservation) (campaign.TouchPlanRecipientReviewReceipt, bool, error) {
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return campaign.TouchPlanRecipientReviewReceipt{}, false, err
	}
	row, err := queries.ReserveCampaignTouchPlanRecipientReviewReceipt(ctx, campaigndb.ReserveCampaignTouchPlanRecipientReviewReceiptParams{
		ActorID: reservation.ActorID, Operation: reservation.Operation, KeyDigest: reservation.KeyDigest[:], PayloadDigest: reservation.PayloadDigest[:],
		PlanID: reservation.PlanID, CampaignCode: reservation.CampaignCode, CustomerID: reservation.CustomerID, CreatedAt: pgTime(reservation.CreatedAt),
	})
	if err == nil {
		value, ok := recipientReviewReceipt(row.ID, row.ActorID, row.Operation, row.KeyDigest, row.PayloadDigest, row.PlanID, row.CampaignCode, row.CustomerID, row.EventID, row.State, row.ResultSnapshot)
		if !ok {
			return campaign.TouchPlanRecipientReviewReceipt{}, false, campaign.ErrUnavailable
		}
		return value, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return campaign.TouchPlanRecipientReviewReceipt{}, false, err
	}
	existing, err := queries.GetCampaignTouchPlanRecipientReviewReceiptForUpdate(ctx, campaigndb.GetCampaignTouchPlanRecipientReviewReceiptForUpdateParams{ActorID: reservation.ActorID, KeyDigest: reservation.KeyDigest[:]})
	if err != nil {
		return campaign.TouchPlanRecipientReviewReceipt{}, false, err
	}
	value, ok := recipientReviewReceipt(existing.ID, existing.ActorID, existing.Operation, existing.KeyDigest, existing.PayloadDigest, existing.PlanID, existing.CampaignCode, existing.CustomerID, existing.EventID, existing.State, existing.ResultSnapshot)
	if !ok || value.Operation != reservation.Operation || value.PlanID != reservation.PlanID || value.CampaignCode != reservation.CampaignCode || value.CustomerID != reservation.CustomerID || subtle.ConstantTimeCompare(value.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
		return campaign.TouchPlanRecipientReviewReceipt{}, false, campaign.ErrIdempotencyConflict
	}
	return value, false, nil
}

func (r *Repository) LockTouchPlanRecipientReview(ctx context.Context, campaignCode, planID string, customerID int64) (campaign.TouchPlanRecipientReview, bool, error) {
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return campaign.TouchPlanRecipientReview{}, false, err
	}
	row, err := queries.LockCampaignTouchPlanRecipientReview(ctx, campaigndb.LockCampaignTouchPlanRecipientReviewParams{CampaignCode: campaignCode, PlanID: planID, CustomerID: customerID})
	if errors.Is(err, pgx.ErrNoRows) {
		return campaign.TouchPlanRecipientReview{}, false, nil
	}
	if err != nil {
		return campaign.TouchPlanRecipientReview{}, false, err
	}
	value, ok := recipientReviewValue(row.PlanID, row.CampaignCode, row.CustomerID, row.MessageOverride, row.Status, row.Version, row.UpdatedByActorID, row.UpdatedAt)
	if !ok {
		return campaign.TouchPlanRecipientReview{}, false, campaign.ErrUnavailable
	}
	return value, true, nil
}

func (r *Repository) SaveTouchPlanRecipientReview(ctx context.Context, value campaign.TouchPlanRecipientReview, expectedVersion int64) error {
	if !campaign.ValidTouchPlanRecipientReview(value) || expectedVersion < 0 {
		return campaign.ErrUnavailable
	}
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return err
	}
	row, err := queries.SaveCampaignTouchPlanRecipientReview(ctx, campaigndb.SaveCampaignTouchPlanRecipientReviewParams{
		PlanID: value.PlanID, CampaignCode: value.CampaignCode, CustomerID: value.CustomerID, MessageOverride: value.MessageOverride,
		Status: string(value.Status), Version: value.Version, UpdatedByActorID: value.UpdatedByActorID, UpdatedAt: pgTime(value.UpdatedAt), ExpectedVersion: expectedVersion,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return campaign.ErrConflict
	}
	if err != nil {
		return err
	}
	stored, ok := recipientReviewValue(row.PlanID, row.CampaignCode, row.CustomerID, row.MessageOverride, row.Status, row.Version, row.UpdatedByActorID, row.UpdatedAt)
	if !ok || !reflect.DeepEqual(stored, value) {
		return campaign.ErrUnavailable
	}
	return nil
}

func (r *Repository) ReadTouchPlanRecipientReview(ctx context.Context, campaignCode, planID string, customerID int64) (campaign.TouchPlanRecipientReview, error) {
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return campaign.TouchPlanRecipientReview{}, err
	}
	row, err := queries.GetCampaignTouchPlanRecipientReview(ctx, campaigndb.GetCampaignTouchPlanRecipientReviewParams{CampaignCode: campaignCode, PlanID: planID, CustomerID: customerID})
	if errors.Is(err, pgx.ErrNoRows) {
		return campaign.TouchPlanRecipientReview{}, campaign.ErrNotFound
	}
	if err != nil {
		return campaign.TouchPlanRecipientReview{}, err
	}
	value, ok := recipientReviewValue(row.PlanID, row.CampaignCode, row.CustomerID, row.MessageOverride, row.Status, row.Version, row.UpdatedByActorID, row.UpdatedAt)
	if !ok {
		return campaign.TouchPlanRecipientReview{}, campaign.ErrUnavailable
	}
	return value, nil
}

func (r *Repository) CompleteTouchPlanRecipientReviewReceipt(ctx context.Context, id int64, result campaign.TouchPlanRecipientReviewResult, completedAt time.Time) error {
	if id < 1 || !validStoredTime(completedAt) || !validRecipientReviewResult(result) {
		return campaign.ErrUnavailable
	}
	snapshot, err := json.Marshal(result)
	if err != nil {
		return campaign.ErrUnavailable
	}
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return err
	}
	row, err := queries.CompleteCampaignTouchPlanRecipientReviewReceipt(ctx, campaigndb.CompleteCampaignTouchPlanRecipientReviewReceiptParams{EventID: pgtype.Int8{Int64: result.EventID, Valid: true}, ResultSnapshot: snapshot, CompletedAt: pgTime(completedAt), ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return campaign.ErrUnavailable
	}
	if err != nil {
		return err
	}
	receipt, ok := recipientReviewReceipt(row.ID, row.ActorID, row.Operation, row.KeyDigest, row.PayloadDigest, row.PlanID, row.CampaignCode, row.CustomerID, row.EventID, row.State, row.ResultSnapshot)
	if !ok || receipt.ID != id || receipt.Result == nil || !reflect.DeepEqual(*receipt.Result, result) {
		return campaign.ErrUnavailable
	}
	return nil
}

func recipientReviewValue(planID, campaignCode string, customerID int64, messageOverride, status string, version, actorID int64, updatedAt pgtype.Timestamptz) (campaign.TouchPlanRecipientReview, bool) {
	if !updatedAt.Valid {
		return campaign.TouchPlanRecipientReview{}, false
	}
	value := campaign.TouchPlanRecipientReview{PlanID: planID, CampaignCode: campaignCode, CustomerID: customerID, MessageOverride: messageOverride, Status: campaign.TouchPlanRecipientReviewStatus(status), Version: version, UpdatedByActorID: actorID, UpdatedAt: updatedAt.Time.UTC(), Safety: campaign.LocalInitiationSafety()}
	return value, campaign.ValidTouchPlanRecipientReview(value)
}

func recipientReviewReceipt(id, actorID int64, operation string, keyDigest, payloadDigest []byte, planID, campaignCode string, customerID int64, eventID pgtype.Int8, state string, snapshot []byte) (campaign.TouchPlanRecipientReviewReceipt, bool) {
	if id < 1 || actorID < 1 || len(keyDigest) != sha256.Size || len(payloadDigest) != sha256.Size {
		return campaign.TouchPlanRecipientReviewReceipt{}, false
	}
	value := campaign.TouchPlanRecipientReviewReceipt{ID: id, ActorID: actorID, Operation: operation, PlanID: planID, CampaignCode: campaignCode, CustomerID: customerID, State: campaign.TouchPlanRecipientReviewReceiptState(state)}
	copy(value.KeyDigest[:], keyDigest)
	copy(value.PayloadDigest[:], payloadDigest)
	if value.State == campaign.TouchPlanRecipientReviewReceiptCompleted {
		var result campaign.TouchPlanRecipientReviewResult
		if !eventID.Valid || json.Unmarshal(snapshot, &result) != nil || result.EventID != eventID.Int64 || !validRecipientReviewResult(result) {
			return campaign.TouchPlanRecipientReviewReceipt{}, false
		}
		value.Result = &result
	} else if value.State != campaign.TouchPlanRecipientReviewReceiptReserved || eventID.Valid || len(snapshot) != 0 {
		return campaign.TouchPlanRecipientReviewReceipt{}, false
	}
	return value, true
}

func validRecipientReviewResult(value campaign.TouchPlanRecipientReviewResult) bool {
	return value.EventID > 0 && campaign.ValidTouchPlanRecipientReview(value.Review)
}
