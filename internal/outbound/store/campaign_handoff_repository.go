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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
	outbounddb "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type CampaignHandoffRepository struct{}

var _ outboundport.CampaignHandoffRepository = (*CampaignHandoffRepository)(nil)

func NewCampaignHandoffRepository() *CampaignHandoffRepository { return &CampaignHandoffRepository{} }

func (*CampaignHandoffRepository) queries(ctx context.Context) (*outbounddb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return outbounddb.New(tx), nil
}

func (repository *CampaignHandoffRepository) ReserveCampaignHandoff(ctx context.Context, reservation outboundport.CampaignHandoffReservation) (outboundport.CampaignHandoffReceipt, bool, error) {
	if repository == nil || reservation.ActorID < 1 || !outbound.ValidCampaignHandoffIdentity(reservation.CampaignCode, reservation.PlanID) || !validOutboundHandoffTime(reservation.CreatedAt) {
		return outboundport.CampaignHandoffReceipt{}, false, outbound.ErrCampaignHandoffInvalid
	}
	queries, err := repository.queries(ctx)
	if err != nil {
		return outboundport.CampaignHandoffReceipt{}, false, err
	}
	row, err := queries.ReserveOutboundCampaignHandoffReceipt(ctx, outbounddb.ReserveOutboundCampaignHandoffReceiptParams{
		ActorID: reservation.ActorID, KeyDigest: reservation.KeyDigest[:], PayloadDigest: reservation.PayloadDigest[:],
		CampaignCode: reservation.CampaignCode, PlanID: reservation.PlanID, CreatedAt: pgtype.Timestamptz{Time: reservation.CreatedAt, Valid: true},
	})
	if err == nil {
		receipt, valid := campaignHandoffReceiptFromRow(row.ID, row.ActorID, row.KeyDigest, row.PayloadDigest, row.CampaignCode, row.PlanID, row.HandoffID, row.EventID, row.State, row.ResultSnapshot)
		if !valid {
			return outboundport.CampaignHandoffReceipt{}, false, outbound.ErrCampaignHandoffUnavailable
		}
		return receipt, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return outboundport.CampaignHandoffReceipt{}, false, err
	}
	existing, err := queries.GetOutboundCampaignHandoffReceiptForUpdate(ctx, outbounddb.GetOutboundCampaignHandoffReceiptForUpdateParams{ActorID: reservation.ActorID, KeyDigest: reservation.KeyDigest[:]})
	if err != nil {
		return outboundport.CampaignHandoffReceipt{}, false, err
	}
	if existing.CampaignCode != reservation.CampaignCode || existing.PlanID != reservation.PlanID ||
		subtle.ConstantTimeCompare(existing.PayloadDigest, reservation.PayloadDigest[:]) != 1 {
		return outboundport.CampaignHandoffReceipt{}, false, outbound.ErrCampaignHandoffIdempotencyConflict
	}
	receipt, valid := campaignHandoffReceiptFromRow(existing.ID, existing.ActorID, existing.KeyDigest, existing.PayloadDigest, existing.CampaignCode, existing.PlanID, existing.HandoffID, existing.EventID, existing.State, existing.ResultSnapshot)
	if !valid {
		return outboundport.CampaignHandoffReceipt{}, false, outbound.ErrCampaignHandoffUnavailable
	}
	return receipt, false, nil
}

func (repository *CampaignHandoffRepository) CreateAcceptedCampaignHandoff(ctx context.Context, snapshot outboundport.ApprovedCampaignHandoffSnapshot, actorID int64, acceptedAt time.Time) (int64, error) {
	if repository == nil || actorID < 1 || !validOutboundHandoffTime(acceptedAt) {
		return 0, outbound.ErrCampaignHandoffInvalid
	}
	links, valid := outbound.CanonicalCampaignHandoffLinks(snapshot.CustomerIDs)
	if !valid {
		return 0, outbound.ErrCampaignHandoffInvalid
	}
	probe := outbound.AcceptedCampaignHandoff{
		ID: 1, CampaignCode: snapshot.CampaignCode, PlanID: snapshot.PlanID, ReviewVersion: snapshot.ReviewVersion,
		SourceDigest: snapshot.SourceDigest, TargetDigest: snapshot.TargetDigest, ContentDigest: snapshot.ContentDigest,
		TargetCount: int32(len(links)), StepCount: int32(len(snapshot.Steps)), Status: outbound.CampaignHandoffHeld,
		AcceptedBy: actorID, AcceptedAt: acceptedAt, Safety: outbound.LocalCampaignHandoffSafety(),
		Steps: snapshot.Steps, Links: links,
	}
	if !outbound.ValidAcceptedCampaignHandoff(probe) {
		return 0, outbound.ErrCampaignHandoffInvalid
	}
	sourceDigest, ok := decodeOutboundHandoffDigest(snapshot.SourceDigest)
	if !ok {
		return 0, outbound.ErrCampaignHandoffInvalid
	}
	targetDigest, ok := decodeOutboundHandoffDigest(snapshot.TargetDigest)
	if !ok {
		return 0, outbound.ErrCampaignHandoffInvalid
	}
	contentDigest, ok := decodeOutboundHandoffDigest(snapshot.ContentDigest)
	if !ok {
		return 0, outbound.ErrCampaignHandoffInvalid
	}
	queries, err := repository.queries(ctx)
	if err != nil {
		return 0, err
	}
	id, err := queries.InsertOutboundCampaignHandoff(ctx, outbounddb.InsertOutboundCampaignHandoffParams{
		CampaignCode: snapshot.CampaignCode, PlanID: snapshot.PlanID, ReviewVersion: snapshot.ReviewVersion,
		SourceDigest: sourceDigest, TargetDigest: targetDigest, ContentDigest: contentDigest,
		TargetCount: int32(len(links)), StepCount: int32(len(snapshot.Steps)), AcceptedByActorID: actorID,
		AcceptedAt: pgtype.Timestamptz{Time: acceptedAt, Valid: true},
	})
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return 0, outbound.ErrCampaignHandoffConflict
		}
		return 0, err
	}
	for _, step := range snapshot.Steps {
		if err = queries.InsertOutboundCampaignHandoffStep(ctx, outbounddb.InsertOutboundCampaignHandoffStepParams{HandoffID: id, StepIndex: step.Index, DelayMinutes: step.DelayMinutes, Content: step.Content}); err != nil {
			return 0, err
		}
	}
	customerIDs := make([]int64, len(links))
	for index, link := range links {
		customerIDs[index] = link.CustomerID
	}
	if err = queries.InsertOutboundCampaignHandoffCustomerLinks(ctx, outbounddb.InsertOutboundCampaignHandoffCustomerLinksParams{HandoffID: id, CustomerIds: customerIDs}); err != nil {
		return 0, err
	}
	return id, nil
}

func (repository *CampaignHandoffRepository) ReadAcceptedCampaignHandoff(ctx context.Context, campaignCode, planID string) (outbound.AcceptedCampaignHandoff, error) {
	queries, err := repository.queries(ctx)
	if err != nil {
		return outbound.AcceptedCampaignHandoff{}, err
	}
	header, err := queries.GetOutboundCampaignHandoffHeader(ctx, outbounddb.GetOutboundCampaignHandoffHeaderParams{CampaignCode: campaignCode, PlanID: planID})
	if errors.Is(err, pgx.ErrNoRows) {
		return outbound.AcceptedCampaignHandoff{}, outbound.ErrCampaignHandoffNotFound
	}
	if err != nil {
		return outbound.AcceptedCampaignHandoff{}, err
	}
	steps, err := queries.ListOutboundCampaignHandoffSteps(ctx, header.ID)
	if err != nil {
		return outbound.AcceptedCampaignHandoff{}, err
	}
	links, err := queries.ListOutboundCampaignHandoffCustomerLinks(ctx, header.ID)
	if err != nil {
		return outbound.AcceptedCampaignHandoff{}, err
	}
	result, valid := acceptedCampaignHandoffFromRows(header, steps, links)
	if !valid {
		return outbound.AcceptedCampaignHandoff{}, outbound.ErrCampaignHandoffUnavailable
	}
	return result, nil
}

func (repository *CampaignHandoffRepository) ReadCampaignHandoffSummary(ctx context.Context, campaignCode, planID string) (outbound.CampaignHandoffSummary, error) {
	queries, err := repository.queries(ctx)
	if err != nil {
		return outbound.CampaignHandoffSummary{}, err
	}
	row, err := queries.GetOutboundCampaignHandoffSummary(ctx, outbounddb.GetOutboundCampaignHandoffSummaryParams{CampaignCode: campaignCode, PlanID: planID})
	if errors.Is(err, pgx.ErrNoRows) {
		return outbound.CampaignHandoffSummary{}, outbound.ErrCampaignHandoffNotFound
	}
	if err != nil {
		return outbound.CampaignHandoffSummary{}, err
	}
	result := campaignHandoffSummaryFromRow(row)
	if !outbound.ValidCampaignHandoffSummary(result) {
		return outbound.CampaignHandoffSummary{}, outbound.ErrCampaignHandoffUnavailable
	}
	return result, nil
}

func (repository *CampaignHandoffRepository) CompleteCampaignHandoffReceipt(ctx context.Context, receiptID, eventID int64, result outbound.CampaignHandoffSummary, completedAt time.Time) error {
	if repository == nil || receiptID < 1 || eventID < 1 || !outbound.ValidCampaignHandoffSummary(result) || !validOutboundHandoffTime(completedAt) {
		return outbound.ErrCampaignHandoffInvalid
	}
	snapshot, err := marshalCampaignHandoffResult(result)
	if err != nil {
		return outbound.ErrCampaignHandoffUnavailable
	}
	queries, err := repository.queries(ctx)
	if err != nil {
		return err
	}
	row, err := queries.CompleteOutboundCampaignHandoffReceipt(ctx, outbounddb.CompleteOutboundCampaignHandoffReceiptParams{
		HandoffID: pgtype.Int8{Int64: result.ID, Valid: true}, EventID: pgtype.Int8{Int64: eventID, Valid: true},
		ResultSnapshot: snapshot, CompletedAt: pgtype.Timestamptz{Time: completedAt, Valid: true}, ID: receiptID,
	})
	if err != nil {
		return outbound.ErrCampaignHandoffUnavailable
	}
	receipt, valid := campaignHandoffReceiptFromRow(row.ID, row.ActorID, row.KeyDigest, row.PayloadDigest, row.CampaignCode, row.PlanID, row.HandoffID, row.EventID, row.State, row.ResultSnapshot)
	if !valid || receipt.ID != receiptID || receipt.State != "completed" || receipt.Result == nil || !reflect.DeepEqual(*receipt.Result, result) || !row.EventID.Valid || row.EventID.Int64 != eventID || !row.HandoffID.Valid || row.HandoffID.Int64 != result.ID {
		return outbound.ErrCampaignHandoffUnavailable
	}
	return nil
}

func acceptedCampaignHandoffFromRows(header outbounddb.OutboundCampaignHandoff, stepRows []outbounddb.ListOutboundCampaignHandoffStepsRow, linkRows []outbounddb.ListOutboundCampaignHandoffCustomerLinksRow) (outbound.AcceptedCampaignHandoff, bool) {
	result := outbound.AcceptedCampaignHandoff{
		ID: header.ID, CampaignCode: header.CampaignCode, PlanID: header.PlanID, ReviewVersion: header.ReviewVersion,
		TargetCount: header.TargetCount, StepCount: header.StepCount, Status: header.Status,
		AcceptedBy: header.AcceptedByActorID, Safety: outbound.CampaignHandoffSafety{LocalOnly: header.LocalOnly, ProviderExecutionEligible: header.ProviderExecutionEligible, RealExternalCallExecuted: header.RealExternalCallExecuted, DeliveryProven: header.DeliveryProven},
	}
	if !header.AcceptedAt.Valid {
		return outbound.AcceptedCampaignHandoff{}, false
	}
	result.AcceptedAt = header.AcceptedAt.Time.UTC()
	var valid bool
	if result.SourceDigest, valid = encodeOutboundHandoffDigest(header.SourceDigest); !valid {
		return outbound.AcceptedCampaignHandoff{}, false
	}
	if result.TargetDigest, valid = encodeOutboundHandoffDigest(header.TargetDigest); !valid {
		return outbound.AcceptedCampaignHandoff{}, false
	}
	if result.ContentDigest, valid = encodeOutboundHandoffDigest(header.ContentDigest); !valid {
		return outbound.AcceptedCampaignHandoff{}, false
	}
	result.Steps = make([]outbound.CampaignHandoffStep, len(stepRows))
	for index, row := range stepRows {
		result.Steps[index] = outbound.CampaignHandoffStep{Index: row.StepIndex, DelayMinutes: row.DelayMinutes, Content: row.Content}
	}
	result.Links = make([]outbound.CampaignHandoffLink, len(linkRows))
	for index, row := range linkRows {
		var taskID *int64
		if row.OutboundTaskID.Valid {
			value := row.OutboundTaskID.Int64
			taskID = &value
		}
		result.Links[index] = outbound.CampaignHandoffLink{CustomerID: row.CustomerID, State: row.State, Eligibility: row.Eligibility, OutboundTaskID: taskID}
	}
	return result, outbound.ValidAcceptedCampaignHandoff(result)
}

func campaignHandoffSummaryFromRow(row outbounddb.GetOutboundCampaignHandoffSummaryRow) outbound.CampaignHandoffSummary {
	result := outbound.CampaignHandoffSummary{
		ID: row.ID, CampaignCode: row.CampaignCode, PlanID: row.PlanID, ReviewVersion: row.ReviewVersion,
		Status: row.Status, TargetCount: row.TargetCount, StepCount: row.StepCount,
		HeldCount: row.HeldCount, BlockedCount: row.BlockedCount, PendingCount: row.PendingCount,
		NotEvaluatedCount: row.NotEvaluatedCount, EligibleCount: row.EligibleCount,
		InactiveCount: row.InactiveCount, ContactPolicyCount: row.ContactPolicyCount,
		Safety: outbound.CampaignHandoffSafety{LocalOnly: row.LocalOnly, ProviderExecutionEligible: row.ProviderExecutionEligible, RealExternalCallExecuted: row.RealExternalCallExecuted, DeliveryProven: row.DeliveryProven},
	}
	if row.AcceptedAt.Valid {
		result.AcceptedAt = row.AcceptedAt.Time.UTC()
	}
	return result
}

func campaignHandoffReceiptFromRow(id, actorID int64, keyDigest, payloadDigest []byte, campaignCode, planID string, handoffID, eventID pgtype.Int8, state string, snapshot []byte) (outboundport.CampaignHandoffReceipt, bool) {
	if id < 1 || actorID < 1 || len(keyDigest) != sha256.Size || len(payloadDigest) != sha256.Size || !outbound.ValidCampaignHandoffIdentity(campaignCode, planID) {
		return outboundport.CampaignHandoffReceipt{}, false
	}
	result := outboundport.CampaignHandoffReceipt{ID: id, ActorID: actorID, CampaignCode: campaignCode, PlanID: planID, State: state}
	copy(result.KeyDigest[:], keyDigest)
	copy(result.PayloadDigest[:], payloadDigest)
	switch state {
	case "reserved":
		if handoffID.Valid || eventID.Valid || snapshot != nil {
			return outboundport.CampaignHandoffReceipt{}, false
		}
	case "completed":
		if !handoffID.Valid || handoffID.Int64 < 1 || !eventID.Valid || eventID.Int64 < 1 || len(snapshot) == 0 {
			return outboundport.CampaignHandoffReceipt{}, false
		}
		value, valid := unmarshalCampaignHandoffResult(snapshot)
		if !valid || value.ID != handoffID.Int64 || value.CampaignCode != campaignCode || value.PlanID != planID {
			return outboundport.CampaignHandoffReceipt{}, false
		}
		result.Result = &value
	default:
		return outboundport.CampaignHandoffReceipt{}, false
	}
	return result, true
}

type campaignHandoffResultSnapshot struct {
	ID                   int64  `json:"id"`
	CampaignCode         string `json:"campaign_code"`
	PlanID               string `json:"plan_id"`
	ReviewVersion        int64  `json:"review_version"`
	Status               string `json:"status"`
	TargetCount          int32  `json:"target_count"`
	StepCount            int32  `json:"step_count"`
	HeldCount            int32  `json:"held_count"`
	BlockedCount         int32  `json:"blocked_count"`
	PendingCount         int32  `json:"pending_count"`
	NotEvaluatedCount    int32  `json:"not_evaluated_count"`
	EligibleCount        int32  `json:"eligible_count"`
	InactiveCount        int32  `json:"inactive_count"`
	ContactPolicyCount   int32  `json:"contact_policy_count"`
	AcceptedAtUnixMicro  int64  `json:"accepted_at_unix_micro"`
	LocalOnly            bool   `json:"local_only"`
	ProviderEligible     bool   `json:"provider_execution_eligible"`
	ExternalCallExecuted bool   `json:"real_external_call_executed"`
	DeliveryProven       bool   `json:"delivery_proven"`
}

func marshalCampaignHandoffResult(value outbound.CampaignHandoffSummary) ([]byte, error) {
	return json.Marshal(campaignHandoffResultSnapshot{
		ID: value.ID, CampaignCode: value.CampaignCode, PlanID: value.PlanID, ReviewVersion: value.ReviewVersion,
		Status: value.Status, TargetCount: value.TargetCount, StepCount: value.StepCount,
		HeldCount: value.HeldCount, BlockedCount: value.BlockedCount, PendingCount: value.PendingCount,
		NotEvaluatedCount: value.NotEvaluatedCount, EligibleCount: value.EligibleCount,
		InactiveCount: value.InactiveCount, ContactPolicyCount: value.ContactPolicyCount,
		AcceptedAtUnixMicro: value.AcceptedAt.UnixMicro(), LocalOnly: value.Safety.LocalOnly,
		ProviderEligible: value.Safety.ProviderExecutionEligible, ExternalCallExecuted: value.Safety.RealExternalCallExecuted,
		DeliveryProven: value.Safety.DeliveryProven,
	})
}

func unmarshalCampaignHandoffResult(data []byte) (outbound.CampaignHandoffSummary, bool) {
	var snapshot campaignHandoffResultSnapshot
	if json.Unmarshal(data, &snapshot) != nil {
		return outbound.CampaignHandoffSummary{}, false
	}
	result := outbound.CampaignHandoffSummary{
		ID: snapshot.ID, CampaignCode: snapshot.CampaignCode, PlanID: snapshot.PlanID, ReviewVersion: snapshot.ReviewVersion,
		Status: snapshot.Status, TargetCount: snapshot.TargetCount, StepCount: snapshot.StepCount,
		HeldCount: snapshot.HeldCount, BlockedCount: snapshot.BlockedCount, PendingCount: snapshot.PendingCount,
		NotEvaluatedCount: snapshot.NotEvaluatedCount, EligibleCount: snapshot.EligibleCount,
		InactiveCount: snapshot.InactiveCount, ContactPolicyCount: snapshot.ContactPolicyCount,
		AcceptedAt: time.UnixMicro(snapshot.AcceptedAtUnixMicro).UTC(),
		Safety:     outbound.CampaignHandoffSafety{LocalOnly: snapshot.LocalOnly, ProviderExecutionEligible: snapshot.ProviderEligible, RealExternalCallExecuted: snapshot.ExternalCallExecuted, DeliveryProven: snapshot.DeliveryProven},
	}
	return result, outbound.ValidCampaignHandoffSummary(result)
}

func decodeOutboundHandoffDigest(value string) ([]byte, bool) {
	decoded, err := hex.DecodeString(value)
	return decoded, err == nil && len(decoded) == sha256.Size
}

func encodeOutboundHandoffDigest(value []byte) (string, bool) {
	return hex.EncodeToString(value), len(value) == sha256.Size
}

func validOutboundHandoffTime(value time.Time) bool {
	return !value.IsZero() && value.Location() == time.UTC && value.Equal(value.Truncate(time.Microsecond))
}
