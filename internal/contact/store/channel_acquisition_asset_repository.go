package store

import (
	"context"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
)

// ChannelAcquisitionAssetRepository persists only Contact's typed acquisition
// facts. The composition root supplies the shared EER runtime separately.
type ChannelAcquisitionAssetRepository struct{}

var _ contactapp.ChannelAcquisitionAssetStore = (*ChannelAcquisitionAssetRepository)(nil)
var _ contactapp.ChannelAcquisitionAssetReadStore = (*ChannelAcquisitionAssetRepository)(nil)

func NewChannelAcquisitionAssetRepository() *ChannelAcquisitionAssetRepository {
	return &ChannelAcquisitionAssetRepository{}
}

func (r *ChannelAcquisitionAssetRepository) LockSnapshot(ctx context.Context, channelID int64) (contactport.AcquisitionAssetSnapshot, error) {
	q, err := channelQueries(ctx)
	if r == nil || err != nil || channelID < 1 {
		return contactport.AcquisitionAssetSnapshot{}, channelAcquisitionStoreError(err)
	}
	row, err := q.LockChannelAcquisitionSnapshot(ctx, channelID)
	if err != nil {
		return contactport.AcquisitionAssetSnapshot{}, channelAcquisitionStoreError(err)
	}
	revision, ok := row.ChannelRevision.(int64)
	if !ok || revision < 1 {
		return contactport.AcquisitionAssetSnapshot{}, contactapp.ErrChannelAcquisitionAssetUnavailable
	}
	return contactport.AcquisitionAssetSnapshot{ChannelID: row.ChannelID, ChannelRevision: revision, ChannelCode: row.ChannelCode,
		ChannelName: row.ChannelName, ChannelStatus: row.Status, SceneValue: row.SceneValue,
		AssigneeWeComUserIDs: append([]string(nil), row.AssigneeWecomUserids...)}, nil
}

func (r *ChannelAcquisitionAssetRepository) ReserveActorReceipt(ctx context.Context, candidate contactapp.ChannelAcquisitionAssetActorReceipt) (contactapp.ChannelAcquisitionAssetActorReceipt, bool, error) {
	q, err := channelQueries(ctx)
	if r == nil || err != nil {
		return contactapp.ChannelAcquisitionAssetActorReceipt{}, false, channelAcquisitionStoreError(err)
	}
	params := contactdb.ReserveChannelAcquisitionAssetActorReceiptParams{Operation: string(candidate.Operation), ActorID: candidate.Actor,
		KeyDigest: string(candidate.KeyDigest), PayloadDigest: string(candidate.PayloadDigest), CreatedAt: acquisitionTime(candidate.CreatedAt)}
	row, err := q.ReserveChannelAcquisitionAssetActorReceipt(ctx, params)
	if err == nil {
		return channelAcquisitionActorReceipt(row), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return contactapp.ChannelAcquisitionAssetActorReceipt{}, false, channelAcquisitionStoreError(err)
	}
	row, err = q.LockChannelAcquisitionAssetActorReceipt(ctx, contactdb.LockChannelAcquisitionAssetActorReceiptParams{Operation: params.Operation, ActorID: params.ActorID, KeyDigest: params.KeyDigest})
	if err != nil {
		return contactapp.ChannelAcquisitionAssetActorReceipt{}, false, channelAcquisitionStoreError(err)
	}
	return channelAcquisitionActorReceipt(row), false, nil
}

func (r *ChannelAcquisitionAssetRepository) CompleteActorReceipt(ctx context.Context, id int64, effectID, replacementID string, at time.Time) (contactapp.ChannelAcquisitionAssetActorReceipt, error) {
	q, err := channelQueries(ctx)
	resultID, parseErr := channelAcquisitionEffectID(effectID)
	if r == nil || err != nil || parseErr != nil || id < 1 {
		return contactapp.ChannelAcquisitionAssetActorReceipt{}, channelAcquisitionStoreError(errors.Join(err, parseErr))
	}
	replacement := pgtype.Int8{}
	if replacementID != "" {
		value, valueErr := channelAcquisitionEffectID(replacementID)
		if valueErr != nil {
			return contactapp.ChannelAcquisitionAssetActorReceipt{}, contactapp.ErrChannelAcquisitionAssetUnavailable
		}
		replacement = pgtype.Int8{Int64: value, Valid: true}
	}
	row, err := q.CompleteChannelAcquisitionAssetActorReceipt(ctx, contactdb.CompleteChannelAcquisitionAssetActorReceiptParams{ID: id, ResultEffectID: resultID, ReplacementEffectID: replacement, CompletedAt: acquisitionTime(at)})
	if err != nil {
		return contactapp.ChannelAcquisitionAssetActorReceipt{}, channelAcquisitionStoreError(err)
	}
	return channelAcquisitionActorReceipt(row), nil
}

func (r *ChannelAcquisitionAssetRepository) NextAssetVersion(ctx context.Context, channelID, supersedes int64, kind contactport.AcquisitionAssetKind) (int64, error) {
	q, err := channelQueries(ctx)
	if r == nil || err != nil || channelID < 1 || supersedes < 0 {
		return 0, channelAcquisitionStoreError(err)
	}
	version, err := q.NextChannelAcquisitionAssetVersion(ctx, contactdb.NextChannelAcquisitionAssetVersionParams{ChannelID: channelID, AssetKind: string(kind)})
	if err != nil || version < 1 || int64(version) <= supersedes {
		return 0, channelAcquisitionStoreError(err)
	}
	return int64(version), nil
}

func (r *ChannelAcquisitionAssetRepository) InsertAccepted(ctx context.Context, binding contactapp.ChannelAcquisitionAssetBinding) (contactapp.ChannelAcquisitionAssetBinding, error) {
	q, err := channelQueries(ctx)
	effectID, effectErr := channelAcquisitionEffectID(binding.EffectID)
	receiptID, receiptErr := channelAcquisitionReceiptID(binding.AcceptReceiptID)
	if r == nil || err != nil || effectErr != nil || receiptErr != nil {
		return contactapp.ChannelAcquisitionAssetBinding{}, channelAcquisitionStoreError(errors.Join(err, effectErr, receiptErr))
	}
	row, err := q.InsertChannelAcquisitionAssetBinding(ctx, contactdb.InsertChannelAcquisitionAssetBindingParams{
		EffectID: effectID, ChannelID: binding.ChannelID, AssetKind: string(binding.Kind), AssetVersion: binding.AssetVersion, SupersedesVersion: binding.SupersedesVersion,
		ChannelRevision: binding.Snapshot.ChannelRevision, ChannelCode: binding.Snapshot.ChannelCode, ChannelName: binding.Snapshot.ChannelName, SceneValue: binding.Snapshot.SceneValue,
		AssigneeWecomUserids: append([]string(nil), binding.Snapshot.AssigneeWeComUserIDs...), SnapshotDigest: string(channelAcquisitionDigest(binding.SnapshotDigest)),
		IdempotencyDigest: string(binding.IdempotencyDigest), EnvelopeFingerprint: string(binding.EnvelopeFingerprint), AcceptReceiptID: receiptID, AcceptReceiptDigest: string(binding.AcceptReceiptDigest),
		CorpID: binding.CorpID, CorrelationKey: binding.CorrelationKey,
		Generation: binding.Generation, CreatedAt: acquisitionTime(binding.CreatedAt), UpdatedAt: acquisitionTime(binding.UpdatedAt),
	})
	if err != nil {
		return contactapp.ChannelAcquisitionAssetBinding{}, channelAcquisitionStoreError(err)
	}
	if err := q.UpsertCurrentChannelAcquisitionAsset(ctx, contactdb.UpsertCurrentChannelAcquisitionAssetParams{ChannelID: row.ChannelID, AssetKind: row.AssetKind, EffectID: row.EffectID, AssetVersion: row.AssetVersion, UpdatedAt: row.UpdatedAt}); err != nil {
		return contactapp.ChannelAcquisitionAssetBinding{}, channelAcquisitionStoreError(err)
	}
	return channelAcquisitionBinding(row), nil
}

func (r *ChannelAcquisitionAssetRepository) MarkQueued(ctx context.Context, effectID string, link eer.RiverJobLink, receipt eer.OperationReceipt, at time.Time) (contactapp.ChannelAcquisitionAssetBinding, error) {
	q, err := channelQueries(ctx)
	id, idErr := channelAcquisitionEffectID(effectID)
	receiptID, receiptErr := channelAcquisitionReceiptID(receipt.ID)
	if r == nil || err != nil || idErr != nil || receiptErr != nil {
		return contactapp.ChannelAcquisitionAssetBinding{}, channelAcquisitionStoreError(errors.Join(err, idErr, receiptErr))
	}
	row, err := q.MarkChannelAcquisitionAssetQueued(ctx, contactdb.MarkChannelAcquisitionAssetQueuedParams{EffectID: id, QueueReceiptID: receiptID, QueueReceiptDigest: string(receipt.CommandDigest), RiverJobID: link.JobID, Generation: link.Generation, UpdatedAt: acquisitionTime(at)})
	if err != nil {
		return contactapp.ChannelAcquisitionAssetBinding{}, channelAcquisitionStoreError(err)
	}
	return channelAcquisitionBinding(row), nil
}

func (r *ChannelAcquisitionAssetRepository) LockBinding(ctx context.Context, effectID string) (contactapp.ChannelAcquisitionAssetBinding, error) {
	q, err := channelQueries(ctx)
	id, idErr := channelAcquisitionEffectID(effectID)
	if r == nil || err != nil || idErr != nil {
		return contactapp.ChannelAcquisitionAssetBinding{}, channelAcquisitionStoreError(errors.Join(err, idErr))
	}
	row, err := q.LockChannelAcquisitionAssetBinding(ctx, id)
	if err != nil {
		return contactapp.ChannelAcquisitionAssetBinding{}, channelAcquisitionStoreError(err)
	}
	return channelAcquisitionBinding(row), nil
}

func (r *ChannelAcquisitionAssetRepository) LockBindingForChannel(ctx context.Context, channelID int64, effectID string) (contactapp.ChannelAcquisitionAssetBinding, error) {
	q, err := channelQueries(ctx)
	id, idErr := channelAcquisitionEffectID(effectID)
	if r == nil || err != nil || idErr != nil || channelID < 1 {
		return contactapp.ChannelAcquisitionAssetBinding{}, channelAcquisitionStoreError(errors.Join(err, idErr))
	}
	row, err := q.LockChannelAcquisitionAssetBindingForChannel(ctx, contactdb.LockChannelAcquisitionAssetBindingForChannelParams{ChannelID: channelID, EffectID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.ChannelAcquisitionAssetBinding{}, contactapp.ErrChannelAcquisitionAssetNotFound
	}
	if err != nil {
		return contactapp.ChannelAcquisitionAssetBinding{}, channelAcquisitionStoreError(err)
	}
	return channelAcquisitionBinding(row), nil
}

func (r *ChannelAcquisitionAssetRepository) ReadChannelAcquisitionAssetChannel(ctx context.Context, channelID int64) (bool, error) {
	q, err := channelQueries(ctx)
	if r == nil || err != nil || channelID < 1 {
		return false, channelAcquisitionStoreError(err)
	}
	exists, err := q.ReadChannelAcquisitionAssetChannel(ctx, channelID)
	if err != nil {
		return false, channelAcquisitionStoreError(err)
	}
	return exists, nil
}

func (r *ChannelAcquisitionAssetRepository) GetChannelAcquisitionAsset(ctx context.Context, channelID int64, effectID string) (contactapp.ChannelAcquisitionAssetItem, error) {
	q, err := channelQueries(ctx)
	id, idErr := channelAcquisitionEffectID(effectID)
	if r == nil || err != nil || idErr != nil || channelID < 1 {
		return contactapp.ChannelAcquisitionAssetItem{}, channelAcquisitionStoreError(errors.Join(err, idErr))
	}
	row, err := q.GetChannelAcquisitionAsset(ctx, contactdb.GetChannelAcquisitionAssetParams{ChannelID: channelID, EffectID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.ChannelAcquisitionAssetItem{}, contactapp.ErrChannelAcquisitionAssetNotFound
	}
	if err != nil {
		return contactapp.ChannelAcquisitionAssetItem{}, channelAcquisitionStoreError(err)
	}
	return channelAcquisitionAssetItem(row.EffectID, row.ChannelID, row.AssetKind, row.AssetVersion, row.SupersedesVersion, row.State,
		row.AcceptReceiptID, row.QueueReceiptID, row.AttemptReceiptDigest, row.ReconcileReceiptID, row.CreatedAt, row.UpdatedAt, row.ReconciledAt), nil
}

func (r *ChannelAcquisitionAssetRepository) ListChannelAcquisitionAssets(ctx context.Context, channelID, afterEffectID int64, limit int) ([]contactapp.ChannelAcquisitionAssetItem, error) {
	q, err := channelQueries(ctx)
	if r == nil || err != nil || channelID < 1 || afterEffectID < 0 || limit < 1 || limit > contactapp.ChannelAcquisitionAssetMaximumLimit+1 {
		return nil, channelAcquisitionStoreError(err)
	}
	rows, err := q.ListChannelAcquisitionAssets(ctx, contactdb.ListChannelAcquisitionAssetsParams{ChannelID: channelID, AfterEffectID: afterEffectID, ResultLimit: int32(limit)})
	if err != nil {
		return nil, channelAcquisitionStoreError(err)
	}
	items := make([]contactapp.ChannelAcquisitionAssetItem, len(rows))
	for index, row := range rows {
		items[index] = channelAcquisitionAssetItem(row.EffectID, row.ChannelID, row.AssetKind, row.AssetVersion, row.SupersedesVersion, row.State,
			row.AcceptReceiptID, row.QueueReceiptID, row.AttemptReceiptDigest, row.ReconcileReceiptID, row.CreatedAt, row.UpdatedAt, row.ReconciledAt)
	}
	return items, nil
}

func (r *ChannelAcquisitionAssetRepository) ListExpiredAttempts(ctx context.Context, expiredAt time.Time, limit int32) ([]contactapp.ChannelAcquisitionAssetRecoveryCandidate, error) {
	q, err := channelQueries(ctx)
	if r == nil || err != nil || expiredAt.IsZero() || limit < 1 || limit > contactapp.ChannelAcquisitionAssetRecoveryLimit {
		return nil, channelAcquisitionStoreError(err)
	}
	rows, err := q.ListExpiredChannelAcquisitionAssetAttempts(ctx, contactdb.ListExpiredChannelAcquisitionAssetAttemptsParams{
		ExpiredAt: acquisitionTime(expiredAt), CandidateLimit: limit,
	})
	if err != nil {
		return nil, channelAcquisitionStoreError(err)
	}
	result := make([]contactapp.ChannelAcquisitionAssetRecoveryCandidate, len(rows))
	for index, row := range rows {
		if row.EffectID < 1 || row.Generation < 1 {
			return nil, contactapp.ErrChannelAcquisitionAssetUnavailable
		}
		result[index] = contactapp.ChannelAcquisitionAssetRecoveryCandidate{
			EffectID: channelAcquisitionFormatEffectID(row.EffectID), Generation: row.Generation,
		}
	}
	return result, nil
}

func (r *ChannelAcquisitionAssetRepository) MarkAttempted(ctx context.Context, effectID string, lease eer.Lease, at time.Time) (contactapp.ChannelAcquisitionAssetBinding, error) {
	q, err := channelQueries(ctx)
	id, idErr := channelAcquisitionEffectID(effectID)
	if r == nil || err != nil || idErr != nil || lease.EffectID != effectID {
		return contactapp.ChannelAcquisitionAssetBinding{}, channelAcquisitionStoreError(errors.Join(err, idErr))
	}
	row, err := q.MarkChannelAcquisitionAssetAttempted(ctx, contactdb.MarkChannelAcquisitionAssetAttemptedParams{EffectID: id, Generation: lease.Generation, Fence: lease.Fence, LeaseExpiresAt: acquisitionTime(lease.ExpiresAt), UpdatedAt: acquisitionTime(at)})
	if err != nil {
		return contactapp.ChannelAcquisitionAssetBinding{}, channelAcquisitionStoreError(err)
	}
	return channelAcquisitionBinding(row), nil
}

func (r *ChannelAcquisitionAssetRepository) CompleteAttempt(ctx context.Context, completion contactapp.ChannelAcquisitionAssetAttemptCompletion) (contactapp.ChannelAcquisitionAssetBinding, error) {
	q, err := channelQueries(ctx)
	id, idErr := channelAcquisitionEffectID(completion.EffectID)
	receiptID, receiptErr := channelAcquisitionReceiptID(completion.Receipt.ID)
	if r == nil || err != nil || idErr != nil || receiptErr != nil || completion.Lease.EffectID != completion.EffectID {
		return contactapp.ChannelAcquisitionAssetBinding{}, channelAcquisitionStoreError(errors.Join(err, idErr, receiptErr))
	}
	completedAt := acquisitionTime(completion.CompletedAt)
	factID, err := q.InsertChannelAcquisitionAssetAttemptFact(ctx, contactdb.InsertChannelAcquisitionAssetAttemptFactParams{EffectID: id, Generation: completion.Lease.Generation, Fence: completion.Lease.Fence, State: string(completion.State), ReceiptID: receiptID, ReceiptDigest: string(completion.Receipt.CommandDigest), ProviderCallAttempted: completion.ProviderCallAttempted, RealExternalCallExecuted: completion.RealExternalCallExecuted, CompletedAt: completedAt})
	if err != nil {
		return contactapp.ChannelAcquisitionAssetBinding{}, channelAcquisitionStoreError(err)
	}
	reference := channelAcquisitionOptionalDigest(completion.ProviderAssetReferenceDigest)
	if err := q.InsertChannelAcquisitionAssetObservedResult(ctx, contactdb.InsertChannelAcquisitionAssetObservedResultParams{AttemptFactID: factID, EffectID: id, Outcome: string(completion.State), AssetReferenceDigest: reference, ObservedAt: completedAt}); err != nil {
		return contactapp.ChannelAcquisitionAssetBinding{}, channelAcquisitionStoreError(err)
	}
	row, err := q.CompleteChannelAcquisitionAssetAttempt(ctx, contactdb.CompleteChannelAcquisitionAssetAttemptParams{EffectID: id, Generation: completion.Lease.Generation, Fence: completion.Lease.Fence, State: string(completion.State), AttemptReceiptID: receiptID, AttemptReceiptDigest: string(completion.Receipt.CommandDigest), ProviderAssetReferenceDigest: reference, ProviderCallAttempted: completion.ProviderCallAttempted, RealExternalCallExecuted: completion.RealExternalCallExecuted, UpdatedAt: completedAt})
	if err != nil {
		return contactapp.ChannelAcquisitionAssetBinding{}, channelAcquisitionStoreError(err)
	}
	return channelAcquisitionBinding(row), nil
}

func (r *ChannelAcquisitionAssetRepository) CompleteReconcile(ctx context.Context, completion contactapp.ChannelAcquisitionAssetReconcileCompletion) (contactapp.ChannelAcquisitionAssetBinding, error) {
	q, err := channelQueries(ctx)
	id, idErr := channelAcquisitionEffectID(completion.EffectID)
	receiptID, receiptErr := channelAcquisitionReceiptID(completion.Receipt.ID)
	if r == nil || err != nil || idErr != nil || receiptErr != nil || completion.Lease.EffectID != completion.EffectID {
		return contactapp.ChannelAcquisitionAssetBinding{}, channelAcquisitionStoreError(errors.Join(err, idErr, receiptErr))
	}
	at := acquisitionTime(completion.CompletedAt)
	if err := q.InsertChannelAcquisitionAssetReconciliationFact(ctx, contactdb.InsertChannelAcquisitionAssetReconciliationFactParams{EffectID: id, Generation: completion.Lease.Generation, Fence: completion.Lease.Fence, ReceiptID: receiptID, ReceiptDigest: string(completion.Receipt.CommandDigest), EvidenceDigest: string(completion.EvidenceDigest), Resolution: string(completion.Resolution), ReconciledAt: at}); err != nil {
		return contactapp.ChannelAcquisitionAssetBinding{}, channelAcquisitionStoreError(err)
	}
	row, err := q.CompleteChannelAcquisitionAssetReconcile(ctx, contactdb.CompleteChannelAcquisitionAssetReconcileParams{EffectID: id, Generation: completion.Lease.Generation, Fence: completion.Lease.Fence, ReconcileReceiptID: receiptID, ReconcileReceiptDigest: string(completion.Receipt.CommandDigest), ReconcileEvidenceDigest: string(completion.EvidenceDigest), ReconcileResolution: string(completion.Resolution), ReconciledAt: at, UpdatedAt: at})
	if err != nil {
		return contactapp.ChannelAcquisitionAssetBinding{}, channelAcquisitionStoreError(err)
	}
	return channelAcquisitionBinding(row), nil
}

func channelAcquisitionActorReceipt(row contactdb.ChannelAcquisitionAssetActorReceipt) contactapp.ChannelAcquisitionAssetActorReceipt {
	result := contactapp.ChannelAcquisitionAssetActorReceipt{ID: row.ID, Operation: contactapp.ChannelAcquisitionAssetOperation(row.Operation), Actor: row.ActorID, KeyDigest: eer.Digest(row.KeyDigest), PayloadDigest: eer.Digest(row.PayloadDigest), State: contactapp.ChannelAcquisitionAssetReceiptState(row.State), CreatedAt: row.CreatedAt.Time, CompletedAt: row.CompletedAt.Time}
	if row.ResultEffectID.Valid {
		result.ResultEffectID = channelAcquisitionFormatEffectID(row.ResultEffectID.Int64)
	}
	if row.ReplacementEffectID.Valid {
		result.ReplacementEffectID = channelAcquisitionFormatEffectID(row.ReplacementEffectID.Int64)
	}
	return result
}

func channelAcquisitionBinding(row contactdb.ChannelAcquisitionAssetBinding) contactapp.ChannelAcquisitionAssetBinding {
	result := contactapp.ChannelAcquisitionAssetBinding{EffectID: channelAcquisitionFormatEffectID(row.EffectID), CorpID: row.CorpID.String, CorrelationKey: row.CorrelationKey.String, ChannelID: row.ChannelID, Kind: contactport.AcquisitionAssetKind(row.AssetKind), AssetVersion: row.AssetVersion, SupersedesVersion: row.SupersedesVersion,
		Snapshot:          contactport.AcquisitionAssetSnapshot{ChannelID: row.ChannelID, ChannelRevision: row.ChannelRevision, ChannelCode: row.ChannelCode, ChannelName: row.ChannelName, ChannelStatus: row.ChannelStatus, SceneValue: row.SceneValue, AssigneeWeComUserIDs: append([]string(nil), row.AssigneeWecomUserids...)},
		IdempotencyDigest: eer.Digest(row.IdempotencyDigest), EnvelopeFingerprint: eer.Digest(row.EnvelopeFingerprint), State: eer.State(row.State), AcceptReceiptID: channelAcquisitionFormatReceiptID(row.AcceptReceiptID), AcceptReceiptDigest: eer.Digest(row.AcceptReceiptDigest), Generation: row.Generation, Fence: row.Fence, LeaseExpiresAt: row.LeaseExpiresAt.Time, ProviderCallAttempted: row.ProviderCallAttempted, RealExternalCallExecuted: row.RealExternalCallExecuted, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}
	result.SnapshotDigest = channelAcquisitionDigestBytes(row.SnapshotDigest)
	if row.QueueReceiptID.Valid {
		result.QueueReceiptID = channelAcquisitionFormatReceiptID(row.QueueReceiptID.Int64)
	}
	if row.QueueReceiptDigest.Valid {
		result.QueueReceiptDigest = eer.Digest(row.QueueReceiptDigest.String)
	}
	if row.RiverJobID.Valid {
		result.RiverJobID = row.RiverJobID.Int64
	}
	if row.AttemptReceiptID.Valid {
		result.AttemptReceiptID = channelAcquisitionFormatReceiptID(row.AttemptReceiptID.Int64)
	}
	if row.AttemptReceiptDigest.Valid {
		result.AttemptReceiptDigest = eer.Digest(row.AttemptReceiptDigest.String)
	}
	if row.ProviderAssetReferenceDigest.Valid {
		result.ProviderAssetReferenceDigest = channelAcquisitionDigestBytes(row.ProviderAssetReferenceDigest.String)
	}
	if row.ReconcileReceiptID.Valid {
		result.ReconcileReceiptID = channelAcquisitionFormatReceiptID(row.ReconcileReceiptID.Int64)
	}
	if row.ReconcileReceiptDigest.Valid {
		result.ReconcileReceiptDigest = eer.Digest(row.ReconcileReceiptDigest.String)
	}
	if row.ReconcileEvidenceDigest.Valid {
		result.ReconcileEvidenceDigest = eer.Digest(row.ReconcileEvidenceDigest.String)
	}
	if row.ReconcileResolution.Valid {
		result.ReconcileResolution = contactapp.ChannelAcquisitionAssetReconcileResolution(row.ReconcileResolution.String)
	}
	if row.ReconciledAt.Valid {
		result.ReconciledAt = row.ReconciledAt.Time
	}
	return result
}

func channelAcquisitionAssetItem(effectID, channelID int64, kind string, version, supersedes int64, state string, acceptID int64, queueID pgtype.Int8, attemptDigest pgtype.Text, reconcileID pgtype.Int8, createdAt, updatedAt, reconciledAt pgtype.Timestamptz) contactapp.ChannelAcquisitionAssetItem {
	item := contactapp.ChannelAcquisitionAssetItem{
		EffectID: channelAcquisitionFormatEffectID(effectID), ChannelID: channelID, Kind: contactport.AcquisitionAssetKind(kind), AssetVersion: version,
		SupersedesVersion: supersedes, State: eer.State(state), AcceptReceiptID: channelAcquisitionFormatReceiptID(acceptID),
		CreatedAt: createdAt.Time.UTC(), UpdatedAt: updatedAt.Time.UTC(), EntrantReady: false,
	}
	if queueID.Valid {
		item.QueueReceiptID = channelAcquisitionFormatReceiptID(queueID.Int64)
	}
	if attemptDigest.Valid {
		item.AttemptReceiptDigest = eer.Digest(attemptDigest.String)
	}
	if reconcileID.Valid {
		item.ReconcileReceiptID = channelAcquisitionFormatReceiptID(reconcileID.Int64)
	}
	if reconciledAt.Valid {
		value := reconciledAt.Time.UTC()
		item.ReconciledAt = &value
	}
	return item
}

func channelAcquisitionEffectID(value string) (int64, error) {
	return channelAcquisitionID(value, "eer_")
}
func channelAcquisitionReceiptID(value string) (int64, error) {
	return channelAcquisitionID(value, "eerop_")
}
func channelAcquisitionID(value, prefix string) (int64, error) {
	if !strings.HasPrefix(value, prefix) {
		return 0, errors.New("invalid external-effect identifier")
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(value, prefix), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid external-effect identifier")
	}
	return id, nil
}
func channelAcquisitionFormatEffectID(id int64) string  { return "eer_" + strconv.FormatInt(id, 10) }
func channelAcquisitionFormatReceiptID(id int64) string { return "eerop_" + strconv.FormatInt(id, 10) }
func channelAcquisitionDigest(value [32]byte) eer.Digest {
	return eer.Digest("sha256:" + hex.EncodeToString(value[:]))
}
func channelAcquisitionDigestBytes(value string) (result [32]byte) {
	raw, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	if err == nil && len(raw) == len(result) {
		copy(result[:], raw)
	}
	return result
}
func channelAcquisitionOptionalDigest(value [32]byte) pgtype.Text {
	if value == [32]byte{} {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(channelAcquisitionDigest(value)), Valid: true}
}
func acquisitionTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: !value.IsZero()}
}
func channelAcquisitionStoreError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.ErrChannelAcquisitionAssetUnavailable
	}
	if err != nil {
		return errors.Join(contactapp.ErrChannelAcquisitionAssetUnavailable, err)
	}
	return contactapp.ErrChannelAcquisitionAssetUnavailable
}
