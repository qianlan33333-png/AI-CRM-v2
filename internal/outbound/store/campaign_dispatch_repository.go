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
}

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
	return &CampaignDispatchRepository{client: client}, nil
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
	if repository == nil || binding.HandoffID < 1 || binding.CustomerID < 1 || binding.StepIndex < 1 || !outbound.ValidCampaignDispatchDigest(binding.RecipientDigest) || !outbound.ValidCampaignDispatchDigest(binding.PayloadDigest) {
		return outboundport.CampaignDispatchBinding{}, outbound.ErrCampaignDispatchInvalid
	}
	queries, err := dispatchQueries(ctx)
	if err != nil {
		return outboundport.CampaignDispatchBinding{}, err
	}
	params := outbounddb.InsertOutboundCampaignDispatchParams{HandoffID: binding.HandoffID, CustomerID: binding.CustomerID, StepIndex: binding.StepIndex, RecipientDigest: binding.RecipientDigest, PayloadDigest: binding.PayloadDigest, State: string(binding.State)}
	if binding.ExternalEffectID != "" {
		id, parseErr := parseCampaignExternalEffectID(binding.ExternalEffectID)
		if parseErr != nil || id < 1 {
			return outboundport.CampaignDispatchBinding{}, outbound.ErrCampaignDispatchInvalid
		}
		params.ExternalEffectID = pgtype.Int8{Int64: id, Valid: true}
	}
	if binding.BlockReason != "" {
		params.BlockReason = pgtype.Text{String: binding.BlockReason, Valid: true}
	}
	row, err := queries.InsertOutboundCampaignDispatch(ctx, params)
	if err != nil {
		return outboundport.CampaignDispatchBinding{}, err
	}
	stored, valid := dispatchBindingFromRow(row)
	if !valid {
		return outboundport.CampaignDispatchBinding{}, outbound.ErrCampaignDispatchUnavailable
	}
	if stored.HandoffID != binding.HandoffID || stored.CustomerID != binding.CustomerID || stored.StepIndex != binding.StepIndex || stored.RecipientDigest != binding.RecipientDigest || stored.PayloadDigest != binding.PayloadDigest {
		return outboundport.CampaignDispatchBinding{}, outbound.ErrCampaignDispatchConflict
	}
	return stored, nil
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
	return result, nil
}

func (repository *CampaignDispatchRepository) RecordCampaignProviderAttemptReceipt(ctx context.Context, effectID string, attempt int32, completion string, receipt eer.Digest) error {
	id, err := parseCampaignExternalEffectID(effectID)
	if repository == nil || err != nil || id < 1 || attempt < 1 || !validCampaignDispatchCompletion(completion) || !outbound.ValidCampaignDispatchDigest(string(receipt)) {
		return outbound.ErrCampaignDispatchInvalid
	}
	queries, err := dispatchQueries(ctx)
	if err != nil {
		return err
	}
	return queries.InsertOutboundCampaignProviderAttemptReceipt(ctx, outbounddb.InsertOutboundCampaignProviderAttemptReceiptParams{ExternalEffectID: id, AttemptNumber: attempt, Completion: completion, ProviderReceiptDigest: string(receipt)})
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
