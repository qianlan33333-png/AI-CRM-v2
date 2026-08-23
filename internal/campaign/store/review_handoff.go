package store

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
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

var _ campaignport.ReviewRepository = (*Repository)(nil)

func (r *Repository) ReserveReviewReceipt(ctx context.Context, reservation campaignport.ReviewReceiptReservation) (campaign.TouchPlanReviewReceipt, bool, error) {
	if reservation.ActorID < 1 || !campaign.ValidTouchPlanReviewID(reservation.PlanID) || !validStoredReviewOperation(reservation.Operation) || !validStoredTime(reservation.CreatedAt) {
		return campaign.TouchPlanReviewReceipt{}, false, campaign.ErrUnavailable
	}
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return campaign.TouchPlanReviewReceipt{}, false, err
	}
	row, err := queries.ReserveCampaignTouchPlanReviewReceipt(ctx, campaigndb.ReserveCampaignTouchPlanReviewReceiptParams{ActorID: reservation.ActorID, Operation: reservation.Operation, KeyDigest: reservation.KeyDigest[:], PayloadDigest: reservation.PayloadDigest[:], PlanID: reservation.PlanID, CampaignCode: reservation.CampaignCode, CreatedAt: pgTime(reservation.CreatedAt)})
	if err == nil {
		value, ok := reviewReceiptFromRow(row.ID, row.ActorID, row.Operation, row.KeyDigest, row.PayloadDigest, row.PlanID, row.CampaignCode, row.EventID, row.HandoffEventID, row.State, row.ResultSnapshot)
		if !ok {
			return campaign.TouchPlanReviewReceipt{}, false, campaign.ErrUnavailable
		}
		return value, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return campaign.TouchPlanReviewReceipt{}, false, err
	}
	existing, err := queries.GetCampaignTouchPlanReviewReceiptForUpdate(ctx, campaigndb.GetCampaignTouchPlanReviewReceiptForUpdateParams{ActorID: reservation.ActorID, KeyDigest: reservation.KeyDigest[:]})
	if err != nil {
		return campaign.TouchPlanReviewReceipt{}, false, err
	}
	value, ok := reviewReceiptFromRow(existing.ID, existing.ActorID, existing.Operation, existing.KeyDigest, existing.PayloadDigest, existing.PlanID, existing.CampaignCode, existing.EventID, existing.HandoffEventID, existing.State, existing.ResultSnapshot)
	if !ok {
		return campaign.TouchPlanReviewReceipt{}, false, campaign.ErrUnavailable
	}
	if value.Operation != reservation.Operation || value.PlanID != reservation.PlanID || value.CampaignCode != reservation.CampaignCode || subtle.ConstantTimeCompare(value.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
		return campaign.TouchPlanReviewReceipt{}, false, campaign.ErrIdempotencyConflict
	}
	if value.State == campaign.TouchPlanReviewReceiptCompleted && value.Result == nil {
		return campaign.TouchPlanReviewReceipt{}, false, campaign.ErrUnavailable
	}
	return value, false, nil
}
func (r *Repository) LockTouchPlanReview(ctx context.Context, campaignCode, planID string) (campaign.TouchPlanReview, error) {
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return campaign.TouchPlanReview{}, err
	}
	row, err := queries.LockCampaignTouchPlanReview(ctx, campaigndb.LockCampaignTouchPlanReviewParams{CampaignCode: campaignCode, PlanID: planID})
	if errors.Is(err, pgx.ErrNoRows) {
		return campaign.TouchPlanReview{}, campaign.ErrNotFound
	}
	if err != nil {
		return campaign.TouchPlanReview{}, err
	}
	value, ok := reviewFromRow(row)
	if !ok {
		return campaign.TouchPlanReview{}, campaign.ErrUnavailable
	}
	return value, nil
}
func (r *Repository) ReadTouchPlanReview(ctx context.Context, campaignCode, planID string) (campaign.TouchPlanReview, error) {
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return campaign.TouchPlanReview{}, err
	}
	row, err := queries.GetCampaignTouchPlanReview(ctx, campaigndb.GetCampaignTouchPlanReviewParams{CampaignCode: campaignCode, PlanID: planID})
	if errors.Is(err, pgx.ErrNoRows) {
		return campaign.TouchPlanReview{}, campaign.ErrNotFound
	}
	if err != nil {
		return campaign.TouchPlanReview{}, err
	}
	value, ok := reviewFromRow(row)
	if !ok {
		return campaign.TouchPlanReview{}, campaign.ErrUnavailable
	}
	return value, nil
}
func (r *Repository) SaveTouchPlanReview(ctx context.Context, value campaign.TouchPlanReview, expectedVersion int64) error {
	if !campaign.ValidTouchPlanReview(value) || expectedVersion < 1 {
		return campaign.ErrUnavailable
	}
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return err
	}
	row, err := queries.SaveCampaignTouchPlanReview(ctx, reviewSaveParams(value, expectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return campaign.ErrConflict
	}
	if err != nil {
		return err
	}
	saved, ok := reviewFromRow(row)
	if !ok || !reflect.DeepEqual(saved, value) {
		return campaign.ErrUnavailable
	}
	return nil
}
func (r *Repository) CreateTouchPlanHandoff(ctx context.Context, value campaign.TouchPlanHandoff) error {
	if !campaign.ValidTouchPlanHandoff(value) {
		return campaign.ErrUnavailable
	}
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return err
	}
	err = queries.InsertCampaignTouchPlanHandoff(ctx, campaigndb.InsertCampaignTouchPlanHandoffParams{PlanID: value.PlanID, ReviewVersion: value.ReviewVersion, Status: value.Status, LocalOnly: value.LocalOnly, ProviderExecutionEligible: value.ProviderExecutionEligible, RealExternalCallExecuted: value.RealExternalCallExecuted, DeliveryProven: value.DeliveryProven, CreatedAt: pgTime(value.CreatedAt)})
	if err != nil {
		return err
	}
	return nil
}
func (r *Repository) ReadTouchPlanHandoff(ctx context.Context, campaignCode, planID string) (campaign.TouchPlanHandoff, error) {
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return campaign.TouchPlanHandoff{}, err
	}
	row, err := queries.GetCampaignTouchPlanHandoff(ctx, campaigndb.GetCampaignTouchPlanHandoffParams{CampaignCode: campaignCode, PlanID: planID})
	if errors.Is(err, pgx.ErrNoRows) {
		return campaign.TouchPlanHandoff{}, campaign.ErrNotFound
	}
	if err != nil {
		return campaign.TouchPlanHandoff{}, err
	}
	value := campaign.TouchPlanHandoff{PlanID: row.PlanID, CampaignCode: campaignCode, ReviewVersion: row.ReviewVersion, Status: row.Status, LocalOnly: row.LocalOnly, ProviderExecutionEligible: row.ProviderExecutionEligible, RealExternalCallExecuted: row.RealExternalCallExecuted, DeliveryProven: row.DeliveryProven}
	if !row.CreatedAt.Valid {
		return campaign.TouchPlanHandoff{}, campaign.ErrUnavailable
	}
	value.CreatedAt = row.CreatedAt.Time.UTC()
	if !campaign.ValidTouchPlanHandoff(value) {
		return campaign.TouchPlanHandoff{}, campaign.ErrUnavailable
	}
	return value, nil
}
func (r *Repository) CompleteReviewReceipt(ctx context.Context, id int64, result campaign.TouchPlanReviewResult, completedAt time.Time) error {
	if id < 1 || !validStoredTime(completedAt) || !validReviewResult(result) {
		return campaign.ErrUnavailable
	}
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return err
	}
	snapshot, marshalErr := marshalReviewReceiptResult(result)
	if marshalErr != nil {
		return campaign.ErrUnavailable
	}
	params := campaigndb.CompleteCampaignTouchPlanReviewReceiptParams{ID: id, EventID: pgtype.Int8{Int64: result.EventIDs[0], Valid: true}, ResultSnapshot: snapshot, CompletedAt: pgTime(completedAt)}
	if result.Handoff != nil {
		params.HandoffEventID = pgtype.Int8{Int64: result.EventIDs[1], Valid: true}
	}
	row, err := queries.CompleteCampaignTouchPlanReviewReceipt(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return campaign.ErrUnavailable
	}
	if err != nil {
		return err
	}
	receipt, ok := reviewReceiptFromRow(row.ID, row.ActorID, row.Operation, row.KeyDigest, row.PayloadDigest, row.PlanID, row.CampaignCode, row.EventID, row.HandoffEventID, row.State, row.ResultSnapshot)
	if !ok || receipt.ID != id || receipt.State != campaign.TouchPlanReviewReceiptCompleted || receipt.Result == nil || !reflect.DeepEqual(*receipt.Result, result) {
		return campaign.ErrUnavailable
	}
	return nil
}
func (r *Repository) ListTouchPlanRecipients(ctx context.Context, campaignCode, planID string, after int64, limit int32) ([]campaign.TouchPlanRecipient, error) {
	if !campaign.ValidTouchPlanReviewID(planID) || after < 0 || limit < 1 || limit > campaign.MaximumReviewRecipientPage+1 {
		return nil, campaign.ErrUnavailable
	}
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListCampaignTouchPlanReviewRecipients(ctx, campaigndb.ListCampaignTouchPlanReviewRecipientsParams{CampaignCode: campaignCode, PlanID: planID, AfterCustomerID: after, PageLimit: limit})
	if err != nil {
		return nil, err
	}
	values := make([]campaign.TouchPlanRecipient, len(rows))
	for i, row := range rows {
		values[i] = campaign.TouchPlanRecipient{PlanID: row.PlanID, CustomerID: row.CustomerID}
		if values[i].PlanID != planID || values[i].CustomerID <= after || i > 0 && values[i-1].CustomerID >= values[i].CustomerID {
			return nil, campaign.ErrUnavailable
		}
	}
	return values, nil
}
func (r *Repository) GetTouchPlanRecipient(ctx context.Context, campaignCode, planID string, customerID int64) (campaign.TouchPlanRecipient, error) {
	queries, err := r.initiationQueries(ctx)
	if err != nil {
		return campaign.TouchPlanRecipient{}, err
	}
	row, err := queries.GetCampaignTouchPlanReviewRecipient(ctx, campaigndb.GetCampaignTouchPlanReviewRecipientParams{CampaignCode: campaignCode, PlanID: planID, CustomerID: customerID})
	if errors.Is(err, pgx.ErrNoRows) {
		return campaign.TouchPlanRecipient{}, campaign.ErrNotFound
	}
	if err != nil {
		return campaign.TouchPlanRecipient{}, err
	}
	value := campaign.TouchPlanRecipient{PlanID: row.PlanID, CustomerID: row.CustomerID}
	if value.PlanID != planID || value.CustomerID != customerID {
		return campaign.TouchPlanRecipient{}, campaign.ErrUnavailable
	}
	return value, nil
}
func reviewFromRow(row campaigndb.CloudCampaignTouchPlanReview) (campaign.TouchPlanReview, bool) {
	value := campaign.TouchPlanReview{PlanID: row.PlanID, CampaignCode: row.CampaignCode, Status: campaign.TouchPlanReviewStatus(row.Status), Version: row.Version}
	if row.SubmittedByActorID.Valid {
		value.SubmittedByActorID = row.SubmittedByActorID.Int64
	}
	if row.SubmittedAt.Valid {
		value.SubmittedAt = row.SubmittedAt.Time.UTC()
	}
	if row.ReviewedByActorID.Valid {
		value.ReviewedByActorID = row.ReviewedByActorID.Int64
	}
	if row.ReviewedAt.Valid {
		value.ReviewedAt = row.ReviewedAt.Time.UTC()
	}
	if row.ConfirmationDigest != nil {
		if len(row.ConfirmationDigest) != sha256.Size {
			return campaign.TouchPlanReview{}, false
		}
		copy(value.ConfirmationDigest[:], row.ConfirmationDigest)
	}
	return value, campaign.ValidTouchPlanReview(value)
}
func reviewSaveParams(value campaign.TouchPlanReview, expected int64) campaigndb.SaveCampaignTouchPlanReviewParams {
	params := campaigndb.SaveCampaignTouchPlanReviewParams{PlanID: value.PlanID, Status: string(value.Status), Version: value.Version, ExpectedVersion: expected}
	if value.SubmittedByActorID > 0 {
		params.SubmittedByActorID = pgtype.Int8{Int64: value.SubmittedByActorID, Valid: true}
		params.SubmittedAt = pgTime(value.SubmittedAt)
	}
	if value.ReviewedByActorID > 0 {
		params.ReviewedByActorID = pgtype.Int8{Int64: value.ReviewedByActorID, Valid: true}
		params.ReviewedAt = pgTime(value.ReviewedAt)
		params.ConfirmationDigest = append([]byte(nil), value.ConfirmationDigest[:]...)
	}
	return params
}
func reviewReceiptFromRow(id, actorID int64, operation string, key, payload []byte, planID, campaignCode string, eventID, handoffEventID pgtype.Int8, state string, snapshot []byte) (campaign.TouchPlanReviewReceipt, bool) {
	if id < 1 || actorID < 1 || !validStoredReviewOperation(operation) || len(key) != sha256.Size || len(payload) != sha256.Size || !campaign.ValidTouchPlanReviewID(planID) || !campaign.ValidCampaignCode(campaignCode) {
		return campaign.TouchPlanReviewReceipt{}, false
	}
	value := campaign.TouchPlanReviewReceipt{ID: id, ActorID: actorID, Operation: operation, PlanID: planID, CampaignCode: campaignCode, State: campaign.TouchPlanReviewReceiptState(state)}
	copy(value.KeyDigest[:], key)
	copy(value.PayloadDigest[:], payload)
	if value.State != campaign.TouchPlanReviewReceiptReserved && value.State != campaign.TouchPlanReviewReceiptCompleted {
		return campaign.TouchPlanReviewReceipt{}, false
	}
	if value.State == campaign.TouchPlanReviewReceiptReserved && (eventID.Valid || handoffEventID.Valid || snapshot != nil) {
		return campaign.TouchPlanReviewReceipt{}, false
	}
	if value.State == campaign.TouchPlanReviewReceiptCompleted && (!eventID.Valid || eventID.Int64 < 1 || (operation == "approve") != handoffEventID.Valid || (handoffEventID.Valid && handoffEventID.Int64 < 1) || len(snapshot) == 0) {
		return campaign.TouchPlanReviewReceipt{}, false
	}
	if eventID.Valid {
		result, ok := unmarshalReviewReceiptResult(snapshot)
		if !ok || !validReviewResult(result) || result.Review.PlanID != planID || result.Review.CampaignCode != campaignCode || len(result.EventIDs) == 0 || result.EventIDs[0] != eventID.Int64 || handoffEventID.Valid && (len(result.EventIDs) != 2 || result.EventIDs[1] != handoffEventID.Int64) {
			return campaign.TouchPlanReviewReceipt{}, false
		}
		value.Result = &result
	}
	return value, true
}
func validStoredReviewOperation(value string) bool {
	return value == "submit" || value == "approve" || value == "reject"
}
func validStoredTime(value time.Time) bool {
	return !value.IsZero() && value.Equal(value.UTC().Truncate(time.Microsecond))
}
func pgTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC().Truncate(time.Microsecond), Valid: true}
}
func validReviewResult(value campaign.TouchPlanReviewResult) bool {
	if !campaign.ValidTouchPlanReview(value.Review) || len(value.EventIDs) < 1 {
		return false
	}
	for i, id := range value.EventIDs {
		if id < 1 || i > 0 && value.EventIDs[i-1] >= id {
			return false
		}
	}
	return value.Handoff == nil && len(value.EventIDs) == 1 || value.Handoff != nil && len(value.EventIDs) == 2 && campaign.ValidTouchPlanHandoff(*value.Handoff) && value.Handoff.PlanID == value.Review.PlanID && value.Handoff.ReviewVersion == value.Review.Version
}

type reviewReceiptResultSnapshot struct {
	Review   reviewReceiptReviewSnapshot   `json:"review"`
	Handoff  *reviewReceiptHandoffSnapshot `json:"handoff"`
	EventIDs []int64                       `json:"event_ids"`
}

type reviewReceiptReviewSnapshot struct {
	PlanID                string `json:"plan_id"`
	CampaignCode          string `json:"campaign_code"`
	Status                string `json:"status"`
	Version               int64  `json:"version"`
	SubmittedByActorID    int64  `json:"submitted_by_actor_id"`
	SubmittedAtUnixMicro  int64  `json:"submitted_at_unix_micro"`
	ReviewedByActorID     int64  `json:"reviewed_by_actor_id"`
	ReviewedAtUnixMicro   int64  `json:"reviewed_at_unix_micro"`
	ConfirmationDigestHex string `json:"confirmation_digest"`
}

type reviewReceiptHandoffSnapshot struct {
	PlanID                    string `json:"plan_id"`
	CampaignCode              string `json:"campaign_code"`
	ReviewVersion             int64  `json:"review_version"`
	Status                    string `json:"status"`
	CreatedAtUnixMicro        int64  `json:"created_at_unix_micro"`
	LocalOnly                 bool   `json:"local_only"`
	ProviderExecutionEligible bool   `json:"provider_execution_eligible"`
	RealExternalCallExecuted  bool   `json:"real_external_call_executed"`
	DeliveryProven            bool   `json:"delivery_proven"`
}

func marshalReviewReceiptResult(value campaign.TouchPlanReviewResult) ([]byte, error) {
	if !validReviewResult(value) {
		return nil, errors.New("invalid review receipt result")
	}
	confirmationDigest := ""
	if value.Review.ConfirmationDigest != [sha256.Size]byte{} {
		confirmationDigest = hex.EncodeToString(value.Review.ConfirmationDigest[:])
	}
	snapshot := reviewReceiptResultSnapshot{
		Review: reviewReceiptReviewSnapshot{
			PlanID: value.Review.PlanID, CampaignCode: value.Review.CampaignCode, Status: string(value.Review.Status), Version: value.Review.Version,
			SubmittedByActorID: value.Review.SubmittedByActorID, SubmittedAtUnixMicro: value.Review.SubmittedAt.UnixMicro(),
			ReviewedByActorID: value.Review.ReviewedByActorID, ReviewedAtUnixMicro: reviewUnixMicro(value.Review.ReviewedAt),
			ConfirmationDigestHex: confirmationDigest,
		},
		EventIDs: append([]int64(nil), value.EventIDs...),
	}
	if value.Handoff != nil {
		snapshot.Handoff = &reviewReceiptHandoffSnapshot{
			PlanID: value.Handoff.PlanID, CampaignCode: value.Handoff.CampaignCode, ReviewVersion: value.Handoff.ReviewVersion, Status: value.Handoff.Status,
			CreatedAtUnixMicro: value.Handoff.CreatedAt.UnixMicro(), LocalOnly: value.Handoff.LocalOnly,
			ProviderExecutionEligible: value.Handoff.ProviderExecutionEligible, RealExternalCallExecuted: value.Handoff.RealExternalCallExecuted, DeliveryProven: value.Handoff.DeliveryProven,
		}
	}
	return json.Marshal(snapshot)
}

func unmarshalReviewReceiptResult(data []byte) (campaign.TouchPlanReviewResult, bool) {
	var snapshot reviewReceiptResultSnapshot
	if json.Unmarshal(data, &snapshot) != nil {
		return campaign.TouchPlanReviewResult{}, false
	}
	digest, err := hex.DecodeString(snapshot.Review.ConfirmationDigestHex)
	if err != nil || len(digest) != 0 && len(digest) != sha256.Size {
		return campaign.TouchPlanReviewResult{}, false
	}
	result := campaign.TouchPlanReviewResult{Review: campaign.TouchPlanReview{
		PlanID: snapshot.Review.PlanID, CampaignCode: snapshot.Review.CampaignCode, Status: campaign.TouchPlanReviewStatus(snapshot.Review.Status), Version: snapshot.Review.Version,
		SubmittedByActorID: snapshot.Review.SubmittedByActorID, SubmittedAt: reviewTime(snapshot.Review.SubmittedAtUnixMicro),
		ReviewedByActorID: snapshot.Review.ReviewedByActorID, ReviewedAt: reviewTime(snapshot.Review.ReviewedAtUnixMicro),
	}, EventIDs: append([]int64(nil), snapshot.EventIDs...)}
	copy(result.Review.ConfirmationDigest[:], digest)
	if snapshot.Handoff != nil {
		result.Handoff = &campaign.TouchPlanHandoff{
			PlanID: snapshot.Handoff.PlanID, CampaignCode: snapshot.Handoff.CampaignCode, ReviewVersion: snapshot.Handoff.ReviewVersion, Status: snapshot.Handoff.Status,
			CreatedAt: reviewTime(snapshot.Handoff.CreatedAtUnixMicro), LocalOnly: snapshot.Handoff.LocalOnly,
			ProviderExecutionEligible: snapshot.Handoff.ProviderExecutionEligible, RealExternalCallExecuted: snapshot.Handoff.RealExternalCallExecuted, DeliveryProven: snapshot.Handoff.DeliveryProven,
		}
	}
	return result, validReviewResult(result)
}

func reviewUnixMicro(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UTC().UnixMicro()
}

func reviewTime(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.UnixMicro(value).UTC()
}
