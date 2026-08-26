package store

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
	outbounddb "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store/generated"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type CampaignDispatchRepository struct {
	client *platformjobqueue.InsertOnlyClient
	pool   *pgxpool.Pool
}

const insertCampaignDispatchWithAudienceSnapshot = `
INSERT INTO public.outbound_campaign_dispatches(
  handoff_id,customer_id,step_index,external_effect_id,recipient_digest,payload_digest,state,block_reason,
  sender_userid_snapshot,external_userid_snapshot
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT(handoff_id,customer_id,step_index) DO UPDATE
SET updated_at=public.outbound_campaign_dispatches.updated_at
RETURNING id,handoff_id,customer_id,step_index,external_effect_id,recipient_digest,payload_digest,state,block_reason,
          sender_userid_snapshot,external_userid_snapshot,created_at,updated_at`

var _ outboundport.CampaignDispatchRepository = (*CampaignDispatchRepository)(nil)
var _ outboundport.CampaignDispatchEnqueuer = (*CampaignDispatchRepository)(nil)

func NewCampaignDispatchRepository(pool *pgxpool.Pool) (*CampaignDispatchRepository, error) {
	if pool == nil {
		return nil, outbound.ErrCampaignDispatchUnavailable
	}
	client, err := platformjobqueue.NewInsertOnlyClient(pool)
	if err != nil {
		return nil, errors.Join(outbound.ErrCampaignDispatchUnavailable, err)
	}
	return &CampaignDispatchRepository{client: client, pool: pool}, nil
}

func dispatchQueries(ctx context.Context) (*outbounddb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return outbounddb.New(tx), nil
}

func (repository *CampaignDispatchRepository) LockCampaignHandoffForDispatch(ctx context.Context, campaignCode, planID string) (int64, error) {
	if repository == nil || !outbound.ValidCampaignHandoffIdentity(campaignCode, planID) {
		return 0, outbound.ErrCampaignDispatchInvalid
	}
	queries, err := dispatchQueries(ctx)
	if err != nil {
		return 0, err
	}
	row, err := queries.LockOutboundCampaignHandoffForDispatch(ctx, outbounddb.LockOutboundCampaignHandoffForDispatchParams{CampaignCode: campaignCode, PlanID: planID})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, outbound.ErrCampaignHandoffNotFound
	}
	if err != nil || row.ID < 1 {
		return 0, errors.Join(outbound.ErrCampaignDispatchUnavailable, err)
	}
	return row.ID, nil
}

func (repository *CampaignDispatchRepository) ReadCampaignHandoffForDispatch(ctx context.Context, campaignCode, planID string) (int64, error) {
	if repository == nil || !outbound.ValidCampaignHandoffIdentity(campaignCode, planID) {
		return 0, outbound.ErrCampaignDispatchInvalid
	}
	queries, err := dispatchQueries(ctx)
	if err != nil {
		return 0, err
	}
	row, err := queries.ReadOutboundCampaignHandoffForDispatch(ctx, outbounddb.ReadOutboundCampaignHandoffForDispatchParams{CampaignCode: campaignCode, PlanID: planID})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, outbound.ErrCampaignHandoffNotFound
	}
	if err != nil || row < 1 {
		return 0, errors.Join(outbound.ErrCampaignDispatchUnavailable, err)
	}
	return row, nil
}

func (repository *CampaignDispatchRepository) ListCampaignDispatchCandidates(ctx context.Context, handoffID int64) ([]outboundport.CampaignDispatchCandidate, error) {
	if repository == nil || handoffID < 1 {
		return nil, outbound.ErrCampaignDispatchInvalid
	}
	queries, err := dispatchQueries(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListOutboundCampaignDispatchCandidates(ctx, handoffID)
	if err != nil {
		return nil, err
	}
	result := make([]outboundport.CampaignDispatchCandidate, len(rows))
	for i, row := range rows {
		result[i] = outboundport.CampaignDispatchCandidate{CustomerID: row.CustomerID, StepIndex: row.StepIndex, Content: row.Content}
	}
	return result, nil
}

func (repository *CampaignDispatchRepository) InsertCampaignDispatchBinding(ctx context.Context, binding outboundport.CampaignDispatchBinding) (outboundport.CampaignDispatchBinding, error) {
	if repository == nil || binding.HandoffID < 1 || binding.CustomerID < 1 || binding.StepIndex < 1 || !outbound.ValidCampaignDispatchDigest(binding.RecipientDigest) || !outbound.ValidCampaignDispatchDigest(binding.PayloadDigest) || !validCampaignTargetSnapshot(binding.SenderUserIDSnapshot, binding.ExternalUserIDSnapshot) {
		return outboundport.CampaignDispatchBinding{}, outbound.ErrCampaignDispatchInvalid
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return outboundport.CampaignDispatchBinding{}, err
	}
	var effectID pgtype.Int8
	var blockReason, sender, external pgtype.Text
	if binding.ExternalEffectID != "" {
		id, parseErr := parseCampaignExternalEffectID(binding.ExternalEffectID)
		if parseErr != nil || id < 1 {
			return outboundport.CampaignDispatchBinding{}, outbound.ErrCampaignDispatchInvalid
		}
		effectID = pgtype.Int8{Int64: id, Valid: true}
	}
	if binding.BlockReason != "" {
		blockReason = pgtype.Text{String: binding.BlockReason, Valid: true}
	}
	if binding.SenderUserIDSnapshot != "" {
		sender, external = pgtype.Text{String: binding.SenderUserIDSnapshot, Valid: true}, pgtype.Text{String: binding.ExternalUserIDSnapshot, Valid: true}
	}
	var row campaignDispatchBindingRow
	err = tx.QueryRow(ctx, insertCampaignDispatchWithAudienceSnapshot, binding.HandoffID, binding.CustomerID, binding.StepIndex, effectID, binding.RecipientDigest, binding.PayloadDigest, string(binding.State), blockReason, sender, external).Scan(
		&row.ID, &row.HandoffID, &row.CustomerID, &row.StepIndex, &row.ExternalEffectID, &row.RecipientDigest, &row.PayloadDigest, &row.State, &row.BlockReason, &row.SenderUserIDSnapshot, &row.ExternalUserIDSnapshot, &row.CreatedAt, &row.UpdatedAt,
	)
	if err != nil {
		return outboundport.CampaignDispatchBinding{}, err
	}
	stored, valid := dispatchBindingFromAudienceRow(row)
	if !valid {
		return outboundport.CampaignDispatchBinding{}, outbound.ErrCampaignDispatchUnavailable
	}
	if stored.HandoffID != binding.HandoffID || stored.CustomerID != binding.CustomerID || stored.StepIndex != binding.StepIndex || stored.RecipientDigest != binding.RecipientDigest || stored.PayloadDigest != binding.PayloadDigest || stored.SenderUserIDSnapshot != binding.SenderUserIDSnapshot || stored.ExternalUserIDSnapshot != binding.ExternalUserIDSnapshot {
		return outboundport.CampaignDispatchBinding{}, outbound.ErrCampaignDispatchConflict
	}
	return stored, nil
}

type campaignDispatchBindingRow struct {
	ID, HandoffID, CustomerID                                 int64
	StepIndex                                                 int32
	ExternalEffectID                                          pgtype.Int8
	RecipientDigest, PayloadDigest, State                     string
	BlockReason, SenderUserIDSnapshot, ExternalUserIDSnapshot pgtype.Text
	CreatedAt, UpdatedAt                                      pgtype.Timestamptz
}

func dispatchBindingFromAudienceRow(row campaignDispatchBindingRow) (outboundport.CampaignDispatchBinding, bool) {
	if row.ID < 1 || row.HandoffID < 1 || row.CustomerID < 1 || row.StepIndex < 1 || !outbound.ValidCampaignDispatchDigest(row.RecipientDigest) || !outbound.ValidCampaignDispatchDigest(row.PayloadDigest) || !row.CreatedAt.Valid || !row.UpdatedAt.Valid || !validCampaignTargetSnapshot(textOrEmpty(row.SenderUserIDSnapshot), textOrEmpty(row.ExternalUserIDSnapshot)) {
		return outboundport.CampaignDispatchBinding{}, false
	}
	result := outboundport.CampaignDispatchBinding{ID: row.ID, HandoffID: row.HandoffID, CustomerID: row.CustomerID, StepIndex: row.StepIndex, RecipientDigest: row.RecipientDigest, PayloadDigest: row.PayloadDigest, State: outbound.CampaignDispatchState(row.State), SenderUserIDSnapshot: textOrEmpty(row.SenderUserIDSnapshot), ExternalUserIDSnapshot: textOrEmpty(row.ExternalUserIDSnapshot), CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}
	if row.ExternalEffectID.Valid {
		result.ExternalEffectID = formatCampaignExternalEffectID(row.ExternalEffectID.Int64)
	}
	if row.BlockReason.Valid {
		result.BlockReason = row.BlockReason.String
	}
	return result, true
}

func textOrEmpty(value pgtype.Text) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func (repository *CampaignDispatchRepository) LoadCampaignDispatchByEffect(ctx context.Context, effectID string) (outboundport.CampaignDispatchBinding, error) {
	id, err := parseCampaignExternalEffectID(effectID)
	if repository == nil || err != nil || id < 1 {
		return outboundport.CampaignDispatchBinding{}, outbound.ErrCampaignDispatchInvalid
	}
	queries, err := dispatchQueries(ctx)
	if err != nil {
		return outboundport.CampaignDispatchBinding{}, err
	}
	row, err := queries.LoadOutboundCampaignDispatchByEffect(ctx, pgtype.Int8{Int64: id, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return outboundport.CampaignDispatchBinding{}, outbound.ErrCampaignHandoffNotFound
	}
	if err != nil {
		return outboundport.CampaignDispatchBinding{}, err
	}
	result, valid := dispatchBindingFromRow(row)
	if !valid || result.ExternalEffectID != effectID {
		return outboundport.CampaignDispatchBinding{}, outbound.ErrCampaignDispatchUnavailable
	}
	return result, nil
}

func (repository *CampaignDispatchRepository) LoadCampaignDispatchProviderRequest(ctx context.Context, payloadDigest string) (outboundport.CampaignDispatchProviderRequest, error) {
	if repository == nil || !outbound.ValidCampaignDispatchDigest(payloadDigest) {
		return outboundport.CampaignDispatchProviderRequest{}, outbound.ErrCampaignDispatchInvalid
	}
	if repository.pool == nil {
		return outboundport.CampaignDispatchProviderRequest{}, outbound.ErrCampaignDispatchUnavailable
	}
	const query = `
SELECT dispatch.id,dispatch.handoff_id,dispatch.customer_id,dispatch.step_index,dispatch.payload_digest,step.content,
       COALESCE(plan.source_kind,''),plan.audience_package_id,dispatch.sender_userid_snapshot,dispatch.external_userid_snapshot
FROM public.outbound_campaign_dispatches AS dispatch
JOIN public.outbound_campaign_handoffs AS handoff ON handoff.id=dispatch.handoff_id
LEFT JOIN public.cloud_campaign_touch_plans AS plan ON plan.id=handoff.plan_id
JOIN public.outbound_campaign_handoff_steps AS step ON step.handoff_id=dispatch.handoff_id AND step.step_index=dispatch.step_index
WHERE dispatch.payload_digest=$1`
	var row struct {
		ID, HandoffID, CustomerID                    int64
		StepIndex                                    int32
		PayloadDigest, Content, SourceKind           string
		AudiencePackageID                            pgtype.Int8
		SenderUserIDSnapshot, ExternalUserIDSnapshot pgtype.Text
	}
	err := repository.pool.QueryRow(ctx, query, payloadDigest).Scan(&row.ID, &row.HandoffID, &row.CustomerID, &row.StepIndex, &row.PayloadDigest, &row.Content, &row.SourceKind, &row.AudiencePackageID, &row.SenderUserIDSnapshot, &row.ExternalUserIDSnapshot)
	if errors.Is(err, pgx.ErrNoRows) {
		return outboundport.CampaignDispatchProviderRequest{}, outbound.ErrCampaignHandoffNotFound
	}
	if err != nil || row.ID < 1 || row.HandoffID < 1 || row.CustomerID < 1 || row.StepIndex < 1 || strings.TrimSpace(row.Content) == "" || !outbound.ValidCampaignDispatchDigest(row.PayloadDigest) || (row.SourceKind != "" && row.SourceKind != "ai_audience_package_members" && row.SourceKind != "customer_selection" && row.SourceKind != "segment_members") {
		return outboundport.CampaignDispatchProviderRequest{}, errors.Join(outbound.ErrCampaignDispatchUnavailable, err)
	}
	sender, external := textOrEmpty(row.SenderUserIDSnapshot), textOrEmpty(row.ExternalUserIDSnapshot)
	if !validCampaignTargetSnapshot(sender, external) {
		return outboundport.CampaignDispatchProviderRequest{}, outbound.ErrCampaignDispatchUnavailable
	}
	if row.SourceKind == "ai_audience_package_members" {
		if !row.AudiencePackageID.Valid || row.AudiencePackageID.Int64 < 1 || sender == "" || row.PayloadDigest != outbound.AudienceCampaignDispatchPayloadDigest(row.HandoffID, row.CustomerID, row.StepIndex, row.Content, sender, external) {
			return outboundport.CampaignDispatchProviderRequest{}, outbound.ErrCampaignDispatchUnavailable
		}
	} else if sender != "" || row.PayloadDigest != outbound.CampaignDispatchPayloadDigest(row.HandoffID, row.CustomerID, row.StepIndex, row.Content) {
		return outboundport.CampaignDispatchProviderRequest{}, outbound.ErrCampaignDispatchUnavailable
	}
	return outboundport.CampaignDispatchProviderRequest{DispatchID: row.ID, HandoffID: row.HandoffID, CustomerID: row.CustomerID, StepIndex: row.StepIndex, Content: row.Content, PayloadDigest: row.PayloadDigest, AudiencePackageID: row.AudiencePackageID.Int64, SenderUserIDSnapshot: sender, ExternalUserIDSnapshot: external}, nil
}

func (repository *CampaignDispatchRepository) AudiencePackageForCampaignHandoff(ctx context.Context, handoffID int64) (int64, bool, error) {
	if repository == nil || handoffID < 1 {
		return 0, false, outbound.ErrCampaignDispatchInvalid
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, false, err
	}
	const query = `SELECT COALESCE(plan.source_kind,''),plan.audience_package_id
FROM public.outbound_campaign_handoffs AS handoff
LEFT JOIN public.cloud_campaign_touch_plans AS plan ON plan.id=handoff.plan_id
WHERE handoff.id=$1 FOR KEY SHARE OF handoff`
	var sourceKind string
	var packageID pgtype.Int8
	if err = tx.QueryRow(ctx, query, handoffID).Scan(&sourceKind, &packageID); errors.Is(err, pgx.ErrNoRows) {
		return 0, false, outbound.ErrCampaignHandoffNotFound
	} else if err != nil {
		return 0, false, errors.Join(outbound.ErrCampaignDispatchUnavailable, err)
	}
	if sourceKind == "ai_audience_package_members" {
		if !packageID.Valid || packageID.Int64 < 1 {
			return 0, false, outbound.ErrCampaignDispatchUnavailable
		}
		return packageID.Int64, true, nil
	}
	return 0, false, nil
}

func (repository *CampaignDispatchRepository) ReserveCampaignDispatchReceipt(ctx context.Context, actorID, handoffID int64, key, payload [32]byte, summary outbound.CampaignDispatchSummary) (outboundport.CampaignDispatchReceipt, error) {
	if repository == nil || actorID < 1 || handoffID < 1 || !outbound.ValidCampaignDispatchSummary(summary) {
		return outboundport.CampaignDispatchReceipt{}, outbound.ErrCampaignDispatchInvalid
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		return outboundport.CampaignDispatchReceipt{}, outbound.ErrCampaignDispatchUnavailable
	}
	queries, err := dispatchQueries(ctx)
	if err != nil {
		return outboundport.CampaignDispatchReceipt{}, err
	}
	row, err := queries.ReserveOutboundCampaignDispatchReceipt(ctx, outbounddb.ReserveOutboundCampaignDispatchReceiptParams{ActorID: actorID, HandoffID: handoffID, KeyDigest: key[:], PayloadDigest: payload[:], ResultSnapshot: encoded})
	if err != nil {
		return outboundport.CampaignDispatchReceipt{}, err
	}
	if row.ActorID != actorID || row.HandoffID != handoffID || subtle.ConstantTimeCompare(row.KeyDigest, key[:]) != 1 || subtle.ConstantTimeCompare(row.PayloadDigest, payload[:]) != 1 {
		return outboundport.CampaignDispatchReceipt{}, outbound.ErrCampaignDispatchConflict
	}
	var stored outbound.CampaignDispatchSummary
	if err = json.Unmarshal(row.ResultSnapshot, &stored); err != nil || !outbound.ValidCampaignDispatchSummary(stored) {
		return outboundport.CampaignDispatchReceipt{}, outbound.ErrCampaignDispatchUnavailable
	}
	return outboundport.CampaignDispatchReceipt{ID: row.ID, ActorID: row.ActorID, HandoffID: row.HandoffID, KeyDigest: key, PayloadDigest: payload, Result: stored}, nil
}

func (repository *CampaignDispatchRepository) LoadCampaignDispatchReceipt(ctx context.Context, actorID int64, key [32]byte) (outboundport.CampaignDispatchReceipt, bool, error) {
	if repository == nil || actorID < 1 {
		return outboundport.CampaignDispatchReceipt{}, false, outbound.ErrCampaignDispatchInvalid
	}
	queries, err := dispatchQueries(ctx)
	if err != nil {
		return outboundport.CampaignDispatchReceipt{}, false, err
	}
	row, err := queries.LoadOutboundCampaignDispatchReceipt(ctx, outbounddb.LoadOutboundCampaignDispatchReceiptParams{ActorID: actorID, KeyDigest: key[:]})
	if errors.Is(err, pgx.ErrNoRows) {
		return outboundport.CampaignDispatchReceipt{}, false, nil
	}
	if err != nil {
		return outboundport.CampaignDispatchReceipt{}, false, err
	}
	if row.ID < 1 || row.ActorID != actorID || row.HandoffID < 1 || subtle.ConstantTimeCompare(row.KeyDigest, key[:]) != 1 || len(row.PayloadDigest) != sha256.Size {
		return outboundport.CampaignDispatchReceipt{}, false, outbound.ErrCampaignDispatchUnavailable
	}
	var payloadDigest [32]byte
	copy(payloadDigest[:], row.PayloadDigest)
	var result outbound.CampaignDispatchSummary
	if err = json.Unmarshal(row.ResultSnapshot, &result); err != nil || !outbound.ValidCampaignDispatchSummary(result) || result.HandoffID != row.HandoffID {
		return outboundport.CampaignDispatchReceipt{}, false, outbound.ErrCampaignDispatchUnavailable
	}
	return outboundport.CampaignDispatchReceipt{
		ID: row.ID, ActorID: row.ActorID, HandoffID: row.HandoffID, KeyDigest: key, PayloadDigest: payloadDigest, Result: result,
	}, true, nil
}

func (repository *CampaignDispatchRepository) UpdateCampaignDispatchState(ctx context.Context, effectID string, state outbound.CampaignDispatchState) error {
	id, err := parseCampaignExternalEffectID(effectID)
	if repository == nil || err != nil || id < 1 {
		return outbound.ErrCampaignDispatchInvalid
	}
	queries, err := dispatchQueries(ctx)
	if err != nil {
		return err
	}
	return queries.UpdateOutboundCampaignDispatchState(ctx, outbounddb.UpdateOutboundCampaignDispatchStateParams{ExternalEffectID: pgtype.Int8{Int64: id, Valid: true}, State: string(state)})
}

func (repository *CampaignDispatchRepository) ReadCampaignDispatchSummary(ctx context.Context, handoffID int64) (outbound.CampaignDispatchSummary, error) {
	if repository == nil || handoffID < 1 {
		return outbound.CampaignDispatchSummary{}, outbound.ErrCampaignDispatchInvalid
	}
	queries, err := dispatchQueries(ctx)
	if err != nil {
		return outbound.CampaignDispatchSummary{}, err
	}
	rows, err := queries.ListOutboundCampaignDispatchReconciliation(ctx, handoffID)
	if err != nil {
		return outbound.CampaignDispatchSummary{}, err
	}
	result := outbound.CampaignDispatchSummary{HandoffID: handoffID, UpdatedAt: time.Now().UTC()}
	for _, row := range rows {
		switch outbound.CampaignDispatchState(row.State) {
		case outbound.CampaignDispatchBlocked:
			result.Blocked = row.Count
		case outbound.CampaignDispatchAccepted:
			result.Accepted = row.Count
		case outbound.CampaignDispatchQueued:
			result.Queued = row.Count
		case outbound.CampaignDispatchAttempted:
			result.Attempted = row.Count
		case outbound.CampaignDispatchExecuted:
			result.Executed = row.Count
		case outbound.CampaignDispatchOutcomeUnknown:
			result.OutcomeUnknown = row.Count
		case outbound.CampaignDispatchReconciled:
			result.Reconciled = row.Count
		case outbound.CampaignDispatchRetryableFailed:
			result.RetryableFailed = row.Count
		case outbound.CampaignDispatchFinalFailed:
			result.FinalFailed = row.Count
		default:
			return outbound.CampaignDispatchSummary{}, outbound.ErrCampaignDispatchUnavailable
		}
	}
	evidence, err := queries.ReadOutboundCampaignDispatchEvidence(ctx, handoffID)
	if err != nil || (evidence.RealExternalCallExecuted && !evidence.BusinessCallDispatched) {
		return outbound.CampaignDispatchSummary{}, errors.Join(outbound.ErrCampaignDispatchUnavailable, err)
	}
	result.BusinessCallDispatched = evidence.BusinessCallDispatched
	result.RealExternalCallExecuted = evidence.RealExternalCallExecuted
	const deliveryQuery = `
SELECT COALESCE(bool_or(receipt.delivery_proven), FALSE)::boolean
FROM public.outbound_campaign_dispatches AS dispatch
JOIN public.outbound_campaign_provider_attempt_receipts AS receipt ON receipt.external_effect_id=dispatch.external_effect_id
WHERE dispatch.handoff_id=$1`
	tx, txErr := platformstore.TxFromContext(ctx)
	if txErr != nil {
		return outbound.CampaignDispatchSummary{}, txErr
	}
	if err = tx.QueryRow(ctx, deliveryQuery, handoffID).Scan(&result.DeliveryProven); err != nil {
		return outbound.CampaignDispatchSummary{}, errors.Join(outbound.ErrCampaignDispatchUnavailable, err)
	}
	if result.DeliveryProven && (!result.BusinessCallDispatched || !result.RealExternalCallExecuted) {
		return outbound.CampaignDispatchSummary{}, outbound.ErrCampaignDispatchUnavailable
	}
	return result, nil
}

func (repository *CampaignDispatchRepository) RecordCampaignProviderAttemptReceipt(ctx context.Context, effectID string, attempt int32, evidence outboundport.CampaignDispatchProviderAttemptReceipt) error {
	id, err := parseCampaignExternalEffectID(effectID)
	if repository == nil || err != nil || id < 1 || attempt < 1 || !validCampaignDispatchCompletion(evidence.Completion) || !outbound.ValidCampaignDispatchDigest(string(evidence.ReceiptDigest)) || (evidence.RealExternalCallExecuted && !evidence.BusinessCallDispatched) || !validCampaignProviderReceipt(evidence) {
		return outbound.ErrCampaignDispatchInvalid
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	const query = `
INSERT INTO public.outbound_campaign_provider_attempt_receipts(
 external_effect_id,attempt_number,completion,provider_receipt_digest,business_call_dispatched,real_external_call_executed,
 provider_message_id,provider_code,provider_result_received,delivery_proven,reconciliation_evidence_digest
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT(external_effect_id,attempt_number,completion) DO NOTHING`
	var messageID, providerCode, reconciliation pgtype.Text
	if evidence.ProviderMessageID != "" {
		messageID = pgtype.Text{String: evidence.ProviderMessageID, Valid: true}
	}
	if evidence.ProviderCode != "" {
		providerCode = pgtype.Text{String: evidence.ProviderCode, Valid: true}
	}
	if evidence.ReconciliationEvidenceDigest != "" {
		reconciliation = pgtype.Text{String: string(evidence.ReconciliationEvidenceDigest), Valid: true}
	}
	_, err = tx.Exec(ctx, query, id, attempt, evidence.Completion, string(evidence.ReceiptDigest), evidence.BusinessCallDispatched, evidence.RealExternalCallExecuted, messageID, providerCode, evidence.ProviderResultReceived, evidence.DeliveryProven, reconciliation)
	return err
}

func (repository *CampaignDispatchRepository) LoadAudienceCampaignDispatchReconciliationEvidence(ctx context.Context, effectID string) (outboundport.CampaignDispatchReconciliationEvidence, bool, error) {
	id, err := parseCampaignExternalEffectID(effectID)
	if repository == nil || err != nil || id < 1 {
		return outboundport.CampaignDispatchReconciliationEvidence{}, false, outbound.ErrCampaignDispatchInvalid
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return outboundport.CampaignDispatchReconciliationEvidence{}, false, err
	}
	const sourceQuery = `
SELECT COALESCE(plan.source_kind,'')
FROM public.outbound_campaign_dispatches AS dispatch
JOIN public.outbound_campaign_handoffs AS handoff ON handoff.id=dispatch.handoff_id
LEFT JOIN public.cloud_campaign_touch_plans AS plan ON plan.id=handoff.plan_id
WHERE dispatch.external_effect_id=$1 FOR KEY SHARE OF dispatch`
	var sourceKind string
	if err = tx.QueryRow(ctx, sourceQuery, id).Scan(&sourceKind); errors.Is(err, pgx.ErrNoRows) {
		return outboundport.CampaignDispatchReconciliationEvidence{}, false, outbound.ErrCampaignHandoffNotFound
	} else if err != nil {
		return outboundport.CampaignDispatchReconciliationEvidence{}, false, errors.Join(outbound.ErrCampaignDispatchUnavailable, err)
	}
	if sourceKind != "ai_audience_package_members" {
		return outboundport.CampaignDispatchReconciliationEvidence{}, false, nil
	}
	const query = `
SELECT receipt.provider_message_id,dispatch.sender_userid_snapshot,dispatch.external_userid_snapshot,receipt.provider_receipt_digest,
       receipt.business_call_dispatched,receipt.real_external_call_executed
FROM public.outbound_campaign_dispatches AS dispatch
JOIN public.outbound_campaign_handoffs AS handoff ON handoff.id=dispatch.handoff_id
JOIN public.cloud_campaign_touch_plans AS plan ON plan.id=handoff.plan_id
JOIN public.outbound_campaign_provider_attempt_receipts AS receipt ON receipt.external_effect_id=dispatch.external_effect_id
WHERE dispatch.external_effect_id=$1 AND plan.source_kind='ai_audience_package_members'
  AND receipt.provider_result_received AND receipt.provider_message_id IS NOT NULL
ORDER BY receipt.created_at DESC, receipt.id DESC
LIMIT 1 FOR KEY SHARE OF dispatch,receipt`
	var messageID, sender, external, digest string
	var businessCallDispatched, realExternalCallExecuted bool
	err = tx.QueryRow(ctx, query, id).Scan(&messageID, &sender, &external, &digest, &businessCallDispatched, &realExternalCallExecuted)
	if errors.Is(err, pgx.ErrNoRows) {
		return outboundport.CampaignDispatchReconciliationEvidence{}, true, nil
	}
	if err != nil || !validCampaignTargetSnapshot(sender, external) || !validCampaignProviderText(messageID, 1024) || !outbound.ValidCampaignDispatchDigest(digest) || !businessCallDispatched || !realExternalCallExecuted {
		return outboundport.CampaignDispatchReconciliationEvidence{}, false, errors.Join(outbound.ErrCampaignDispatchUnavailable, err)
	}
	return outboundport.CampaignDispatchReconciliationEvidence{ExternalEffectID: effectID, ProviderMessageID: messageID, SenderUserID: sender, ExternalUserID: external, ProviderReceiptDigest: eer.Digest(digest), BusinessCallDispatched: businessCallDispatched, RealExternalCallExecuted: realExternalCallExecuted}, true, nil
}

func validCampaignTargetSnapshot(sender, external string) bool {
	return (sender == "" && external == "") || (validCampaignProviderText(sender, 128) && validCampaignProviderText(external, 1024))
}

func validCampaignProviderReceipt(value outboundport.CampaignDispatchProviderAttemptReceipt) bool {
	if !validCampaignProviderTextOptional(value.ProviderMessageID, 1024) || !validCampaignProviderTextOptional(value.ProviderCode, 128) {
		return false
	}
	if !value.ProviderResultReceived && (value.ProviderMessageID != "" || value.ProviderCode != "") {
		return false
	}
	if value.DeliveryProven && (value.Completion != string(eer.StateReconciled) || !value.ProviderResultReceived || value.ProviderMessageID == "" || !value.BusinessCallDispatched || !value.RealExternalCallExecuted || !outbound.ValidCampaignDispatchDigest(string(value.ReconciliationEvidenceDigest))) {
		return false
	}
	return value.ReconciliationEvidenceDigest == "" || outbound.ValidCampaignDispatchDigest(string(value.ReconciliationEvidenceDigest))
}

func validCampaignProviderText(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && strings.TrimSpace(value) == value
}
func validCampaignProviderTextOptional(value string, maximum int) bool {
	return value == "" || validCampaignProviderText(value, maximum)
}

type CampaignDispatchArgs struct {
	EffectID string `json:"effect_id"`
}

func (CampaignDispatchArgs) Kind() string { return "outbound_campaign_dispatch" }

func (repository *CampaignDispatchRepository) EnqueueCampaignDispatch(ctx context.Context, effectID string) (eer.RiverJobLink, error) {
	if repository == nil || repository.client == nil || effectID == "" {
		return eer.RiverJobLink{}, outbound.ErrCampaignDispatchInvalid
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return eer.RiverJobLink{}, err
	}
	jobID, err := repository.client.InsertTx(ctx, tx, CampaignDispatchArgs{EffectID: effectID}, string(platformjobqueue.QueueOutbound))
	if err != nil || jobID < 1 {
		return eer.RiverJobLink{}, errors.Join(outbound.ErrCampaignDispatchUnavailable, err)
	}
	return eer.RiverJobLink{JobID: jobID, Generation: 1, Queue: string(platformjobqueue.QueueOutbound), ArgsDigest: digestCampaignDispatchArgs(effectID), ScheduledAt: time.Now().UTC()}, nil
}

func digestCampaignDispatchArgs(effectID string) eer.Digest {
	sum := sha256.Sum256([]byte("outbound.campaign_dispatch.args.v1\x00" + effectID))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}

func validCampaignDispatchCompletion(value string) bool {
	switch value {
	case string(eer.StateExecuted), string(eer.StateRetryableFailed), string(eer.StateFinalFailed), string(eer.StateOutcomeUnknown), string(eer.StateReconciled):
		return true
	default:
		return false
	}
}

func dispatchBindingFromRow(row outbounddb.OutboundCampaignDispatch) (outboundport.CampaignDispatchBinding, bool) {
	if row.ID < 1 || row.HandoffID < 1 || row.CustomerID < 1 || row.StepIndex < 1 || !outbound.ValidCampaignDispatchDigest(row.RecipientDigest) || !outbound.ValidCampaignDispatchDigest(row.PayloadDigest) || !row.CreatedAt.Valid || !row.UpdatedAt.Valid {
		return outboundport.CampaignDispatchBinding{}, false
	}
	result := outboundport.CampaignDispatchBinding{ID: row.ID, HandoffID: row.HandoffID, CustomerID: row.CustomerID, StepIndex: row.StepIndex, RecipientDigest: row.RecipientDigest, PayloadDigest: row.PayloadDigest, State: outbound.CampaignDispatchState(row.State), CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}
	if row.ExternalEffectID.Valid {
		result.ExternalEffectID = formatCampaignExternalEffectID(row.ExternalEffectID.Int64)
	}
	if row.BlockReason.Valid {
		result.BlockReason = row.BlockReason.String
	}
	return result, true
}

func parseCampaignExternalEffectID(value string) (int64, error) {
	if !strings.HasPrefix(value, "eer_") {
		return 0, outbound.ErrCampaignDispatchInvalid
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(value, "eer_"), 10, 64)
	if err != nil || id < 1 {
		return 0, outbound.ErrCampaignDispatchInvalid
	}
	return id, nil
}

func formatCampaignExternalEffectID(id int64) string {
	if id < 1 {
		return ""
	}
	return "eer_" + strconv.FormatInt(id, 10)
}
