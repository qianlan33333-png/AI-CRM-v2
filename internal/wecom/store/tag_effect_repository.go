package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	wecomdb "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store/generated"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/tag"
)

type TagEffectRepository struct {
	pool *pgxpool.Pool
	uow  *platformstore.UnitOfWork
}

var _ tag.Store = (*TagEffectRepository)(nil)

func NewTagEffectRepository(pool *pgxpool.Pool) *TagEffectRepository {
	return &TagEffectRepository{pool: pool, uow: platformstore.NewUnitOfWork(pool)}
}

func (repository *TagEffectRepository) Reserve(ctx context.Context, candidate tag.Effect) (tag.Effect, bool, error) {
	effectID, err := parseTagEffectID(candidate.EffectID)
	if err != nil {
		return tag.Effect{}, false, err
	}
	acceptReceiptID, err := parseTagReceiptID(candidate.AcceptReceiptID)
	if err != nil || candidate.LegacyReceiptID < 1 || candidate.Actor < 1 ||
		candidate.Generation < 1 || candidate.UpdatedAt.IsZero() {
		return tag.Effect{}, false, tag.ErrInvalidCommand
	}
	var row wecomdb.WecomTagEffect
	inserted := false
	err = repository.within(ctx, func(queries *wecomdb.Queries) error {
		row, err = queries.InsertWeComTagEffect(ctx, wecomdb.InsertWeComTagEffectParams{
			EffectID: effectID, LegacyReceiptID: candidate.LegacyReceiptID, ActorID: candidate.Actor,
			CorpID: candidate.CorpID, Operation: string(candidate.Operation), SyncTrigger: string(candidate.SyncTrigger),
			ExternalUserid: candidate.ExternalUserID, ProviderTagIds: append([]string{}, candidate.ProviderTagIDs...),
			IdempotencyDigest: string(candidate.IdempotencyDigest), EnvelopeFingerprint: string(candidate.EnvelopeFingerprint),
			AcceptReceiptID: acceptReceiptID, Generation: candidate.Generation, UpdatedAt: timestamp(candidate.UpdatedAt),
		})
		if err == nil {
			inserted = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		row, err = queries.GetWeComTagEffectByIdempotency(ctx, wecomdb.GetWeComTagEffectByIdempotencyParams{
			ActorID: candidate.Actor, IdempotencyDigest: string(candidate.IdempotencyDigest),
		})
		return err
	})
	if err != nil {
		return tag.Effect{}, false, tagStoreError(err)
	}
	return tagEffect(row), inserted, nil
}

func (repository *TagEffectRepository) GetByIdempotency(ctx context.Context, actor int64, idempotencyDigest eer.Digest) (tag.Effect, error) {
	if repository == nil || repository.pool == nil || ctx == nil || actor < 1 || idempotencyDigest == "" {
		return tag.Effect{}, tag.ErrEffectUnavailable
	}
	queries := wecomdb.New(repository.pool)
	if tx, txErr := platformstore.TxFromContext(ctx); txErr == nil {
		queries = wecomdb.New(tx)
	}
	row, err := queries.GetWeComTagEffectByIdempotency(ctx, wecomdb.GetWeComTagEffectByIdempotencyParams{
		ActorID: actor, IdempotencyDigest: string(idempotencyDigest),
	})
	if err != nil {
		return tag.Effect{}, tagStoreError(err)
	}
	return tagEffect(row), nil
}

func (repository *TagEffectRepository) MarkQueued(ctx context.Context, opaqueEffectID string, link eer.RiverJobLink, opaqueReceiptID string, at time.Time) (tag.Effect, error) {
	effectID, err := parseTagEffectID(opaqueEffectID)
	if err != nil {
		return tag.Effect{}, err
	}
	receiptID, err := parseTagReceiptID(opaqueReceiptID)
	if err != nil || link.JobID < 1 || link.Generation < 1 || at.IsZero() {
		return tag.Effect{}, tag.ErrInvalidCommand
	}
	var row wecomdb.WecomTagEffect
	err = repository.within(ctx, func(queries *wecomdb.Queries) error {
		row, err = queries.MarkWeComTagEffectQueued(ctx, wecomdb.MarkWeComTagEffectQueuedParams{
			QueueReceiptID: int8Value(receiptID), RiverJobID: int8Value(link.JobID), Generation: link.Generation,
			UpdatedAt: timestamp(at), EffectID: effectID,
		})
		return err
	})
	if err != nil {
		return tag.Effect{}, tagStoreError(err)
	}
	return tagEffect(row), nil
}

func (repository *TagEffectRepository) Get(ctx context.Context, opaqueEffectID string) (tag.Effect, error) {
	effectID, err := parseTagEffectID(opaqueEffectID)
	if err != nil {
		return tag.Effect{}, err
	}
	if repository == nil || repository.pool == nil || ctx == nil {
		return tag.Effect{}, tag.ErrEffectUnavailable
	}
	queries := wecomdb.New(repository.pool)
	if tx, txErr := platformstore.TxFromContext(ctx); txErr == nil {
		queries = wecomdb.New(tx)
	}
	row, err := queries.GetWeComTagEffect(ctx, effectID)
	if err != nil {
		return tag.Effect{}, tagStoreError(err)
	}
	return tagEffect(row), nil
}

func (repository *TagEffectRepository) RecordClaim(ctx context.Context, opaqueEffectID string, lease eer.Lease, at time.Time) (tag.Effect, error) {
	effectID, err := parseTagEffectID(opaqueEffectID)
	if err != nil || lease.EffectID != opaqueEffectID || lease.Generation < 1 || lease.Fence < 1 || lease.ExpiresAt.IsZero() || at.IsZero() {
		return tag.Effect{}, tag.ErrInvalidCommand
	}
	var row wecomdb.WecomTagEffect
	err = repository.within(ctx, func(queries *wecomdb.Queries) error {
		row, err = queries.RecordWeComTagEffectClaim(ctx, wecomdb.RecordWeComTagEffectClaimParams{
			Generation: lease.Generation, Fence: lease.Fence, LeaseExpiresAt: timestamp(lease.ExpiresAt),
			UpdatedAt: timestamp(at), EffectID: effectID,
		})
		return err
	})
	if err != nil {
		return tag.Effect{}, tagStoreError(err)
	}
	return tagEffect(row), nil
}

func (repository *TagEffectRepository) CompleteAttempt(ctx context.Context, completion tag.AttemptCompletion) (tag.Effect, error) {
	effectID, err := parseTagEffectID(completion.EffectID)
	if err != nil || completion.Lease.EffectID != completion.EffectID || completion.Lease.Generation < 1 ||
		completion.Lease.Fence < 1 || completion.Lease.ExpiresAt.IsZero() || completion.CompletedAt.IsZero() {
		return tag.Effect{}, tag.ErrInvalidCommand
	}
	receiptID, err := parseTagReceiptID(completion.ReceiptID)
	if err != nil || (completion.State != eer.StateExecuted && completion.State != eer.StateOutcomeUnknown && completion.State != eer.StateFinalFailed) {
		return tag.Effect{}, tag.ErrInvalidCommand
	}
	var row wecomdb.WecomTagEffect
	err = repository.within(ctx, func(queries *wecomdb.Queries) error {
		row, err = queries.CompleteWeComTagEffectAttempt(ctx, wecomdb.CompleteWeComTagEffectAttemptParams{
			State: string(completion.State), AttemptReceiptID: int8Value(receiptID),
			AttemptReceiptDigest: textValue(string(completion.Receipt)), UpdatedAt: timestamp(completion.CompletedAt),
			EffectID: effectID, Generation: completion.Lease.Generation, Fence: completion.Lease.Fence,
		})
		if err != nil {
			return err
		}
		if !completion.Catalog.Observed {
			if len(completion.Catalog.Groups) != 0 || len(completion.Catalog.Tags) != 0 ||
				(row.Operation == string(tag.OperationCatalogSync) && completion.State == eer.StateExecuted) {
				return tag.ErrInvalidCommand
			}
			return nil
		}
		if row.Operation != string(tag.OperationCatalogSync) || completion.State != eer.StateExecuted {
			return tag.ErrInvalidCommand
		}
		snapshotID, err := queries.InsertWeComTagCatalogSnapshot(ctx, wecomdb.InsertWeComTagCatalogSnapshotParams{
			ReceiptDigest: string(completion.Receipt), ObservedAt: timestamp(completion.CompletedAt), EffectID: effectID,
		})
		if err != nil {
			return err
		}
		for _, group := range completion.Catalog.Groups {
			if err = queries.InsertWeComTagCatalogGroup(ctx, wecomdb.InsertWeComTagCatalogGroupParams{
				SnapshotID: snapshotID, ProviderGroupID: group.ProviderGroupID, Name: group.Name, ProviderOrder: group.Order,
			}); err != nil {
				return err
			}
		}
		groupIDs := make(map[string]int64, len(completion.Catalog.Groups))
		providerGroupIDs := make([]string, 0, len(completion.Catalog.Groups))
		for _, group := range completion.Catalog.Groups {
			groupID, groupErr := queries.UpsertWeComTagGroupProjection(ctx, wecomdb.UpsertWeComTagGroupProjectionParams{
				ProviderGroupID: textValue(group.ProviderGroupID), Name: group.Name, ProviderOrder: group.Order,
			})
			if groupErr != nil {
				return groupErr
			}
			groupIDs[group.ProviderGroupID] = groupID
			providerGroupIDs = append(providerGroupIDs, group.ProviderGroupID)
		}
		providerTagIDs := make([]string, 0, len(completion.Catalog.Tags))
		for _, providerTag := range completion.Catalog.Tags {
			if err = queries.InsertWeComTagCatalogTag(ctx, wecomdb.InsertWeComTagCatalogTagParams{
				SnapshotID: snapshotID, ProviderTagID: providerTag.ProviderTagID,
				ProviderGroupID: providerTag.ProviderGroupID, Name: providerTag.Name, ProviderOrder: providerTag.Order,
			}); err != nil {
				return err
			}
			groupID, ok := groupIDs[providerTag.ProviderGroupID]
			if !ok {
				return tag.ErrInvalidCommand
			}
			if err = queries.UpsertWeComTagProjection(ctx, wecomdb.UpsertWeComTagProjectionParams{
				GroupID: int8Value(groupID), ProviderTagID: textValue(providerTag.ProviderTagID), Name: providerTag.Name, ProviderOrder: providerTag.Order,
			}); err != nil {
				return err
			}
			providerTagIDs = append(providerTagIDs, providerTag.ProviderTagID)
		}
		if err = queries.ArchiveMissingWeComTagProjections(ctx, providerTagIDs); err != nil {
			return err
		}
		if err = queries.ArchiveMissingWeComTagGroupProjections(ctx, providerGroupIDs); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return tag.Effect{}, tagStoreError(err)
	}
	return tagEffect(row), nil
}

func (repository *TagEffectRepository) CompleteReconcile(ctx context.Context, completion tag.ReconcileCompletion) (tag.Effect, error) {
	effectID, err := parseTagEffectID(completion.EffectID)
	if err != nil || completion.Lease.EffectID != completion.EffectID || completion.Lease.Generation < 1 ||
		completion.Lease.Fence < 1 || completion.Lease.ExpiresAt.IsZero() || completion.CompletedAt.IsZero() {
		return tag.Effect{}, tag.ErrReconcileRequired
	}
	receiptID, err := parseTagReceiptID(completion.ReceiptID)
	if err != nil || (completion.Resolution != tag.ResolutionProviderApplied && completion.Resolution != tag.ResolutionProviderNotApplied) {
		return tag.Effect{}, tag.ErrReconcileRequired
	}
	var row wecomdb.WecomTagEffect
	err = repository.within(ctx, func(queries *wecomdb.Queries) error {
		row, err = queries.CompleteWeComTagEffectReconcile(ctx, wecomdb.CompleteWeComTagEffectReconcileParams{
			ReconcileReceiptID: int8Value(receiptID), ReconcileReceiptDigest: textValue(string(completion.Receipt)),
			ReconcileEvidenceDigest: textValue(string(completion.EvidenceDigest)), ReconcileResolution: textValue(string(completion.Resolution)),
			UpdatedAt: timestamp(completion.CompletedAt), EffectID: effectID,
			Generation: completion.Lease.Generation, Fence: completion.Lease.Fence,
		})
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		row, err = queries.GetWeComTagEffect(ctx, effectID)
		if err != nil {
			return err
		}
		if row.State != string(eer.StateReconciled) || !row.ReconcileReceiptID.Valid || row.ReconcileReceiptID.Int64 != receiptID ||
			!row.ReconcileReceiptDigest.Valid || row.ReconcileReceiptDigest.String != string(completion.Receipt) ||
			!row.ReconcileEvidenceDigest.Valid || row.ReconcileEvidenceDigest.String != string(completion.EvidenceDigest) ||
			!row.ReconcileResolution.Valid || row.ReconcileResolution.String != string(completion.Resolution) ||
			row.Generation != completion.Lease.Generation || row.Fence != completion.Lease.Fence ||
			!row.LeaseExpiresAt.Valid || !row.LeaseExpiresAt.Time.Equal(completion.Lease.ExpiresAt) {
			return tag.ErrReconcileRequired
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, tag.ErrReconcileRequired) {
			return tag.Effect{}, tag.ErrReconcileRequired
		}
		return tag.Effect{}, tagStoreError(err)
	}
	return tagEffect(row), nil
}

func (repository *TagEffectRepository) within(ctx context.Context, callback func(*wecomdb.Queries) error) error {
	if repository == nil || repository.pool == nil || repository.uow == nil || ctx == nil || callback == nil {
		return tag.ErrEffectUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return callback(wecomdb.New(tx))
	}
	return repository.uow.Within(ctx, func(txCtx context.Context) error {
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		return callback(wecomdb.New(tx))
	})
}

func tagEffect(row wecomdb.WecomTagEffect) tag.Effect {
	return tag.Effect{
		EffectID: formatTagID("eer_", row.EffectID), LegacyReceiptID: row.LegacyReceiptID, Actor: row.ActorID,
		CorpID: row.CorpID, Operation: tag.Operation(row.Operation), SyncTrigger: tag.SyncTrigger(row.SyncTrigger),
		ExternalUserID: row.ExternalUserid, ProviderTagIDs: append([]string(nil), row.ProviderTagIds...),
		IdempotencyDigest: eer.Digest(row.IdempotencyDigest), EnvelopeFingerprint: eer.Digest(row.EnvelopeFingerprint),
		State: eer.State(row.State), AcceptReceiptID: formatTagID("eerop_", row.AcceptReceiptID),
		QueueReceiptID: formatOptionalTagID("eerop_", row.QueueReceiptID), RiverJobID: row.RiverJobID.Int64,
		Generation: row.Generation, Fence: row.Fence, LeaseExpiresAt: timeValue(row.LeaseExpiresAt),
		AttemptReceiptID:     formatOptionalTagID("eerop_", row.AttemptReceiptID),
		AttemptReceiptDigest: eer.Digest(row.AttemptReceiptDigest.String), AttemptCompletedAt: timeValue(row.AttemptCompletedAt),
		ReconcileReceiptID:     formatOptionalTagID("eerop_", row.ReconcileReceiptID),
		ReconcileReceiptDigest: eer.Digest(row.ReconcileReceiptDigest.String),
		ReconcileResolution:    tag.ReconcileResolution(row.ReconcileResolution.String),
		ReconcileEvidenceHash:  eer.Digest(row.ReconcileEvidenceDigest.String), ReconciledAt: timeValue(row.ReconciledAt),
		UpdatedAt: timeValue(row.UpdatedAt),
	}
}

func parseTagEffectID(value string) (int64, error)  { return parseTagID(value, "eer_") }
func parseTagReceiptID(value string) (int64, error) { return parseTagID(value, "eerop_") }

func parseTagID(value, prefix string) (int64, error) {
	if !strings.HasPrefix(value, prefix) {
		return 0, tag.ErrInvalidCommand
	}
	digits := strings.TrimPrefix(value, prefix)
	if digits == "" || digits[0] == '0' || strings.TrimSpace(digits) != digits {
		return 0, tag.ErrInvalidCommand
	}
	id, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || id < 1 || strconv.FormatInt(id, 10) != digits {
		return 0, tag.ErrInvalidCommand
	}
	return id, nil
}

func formatTagID(prefix string, id int64) string {
	if id < 1 {
		return ""
	}
	return prefix + strconv.FormatInt(id, 10)
}

func formatOptionalTagID(prefix string, id pgtype.Int8) string {
	if !id.Valid {
		return ""
	}
	return formatTagID(prefix, id.Int64)
}

func tagStoreError(err error) error {
	if errors.Is(err, tag.ErrInvalidCommand) || errors.Is(err, tag.ErrReconcileRequired) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return tag.ErrEffectUnavailable
	}
	return fmt.Errorf("%w: %v", tag.ErrEffectUnavailable, err)
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: !value.IsZero()}
}

func int8Value(value int64) pgtype.Int8  { return pgtype.Int8{Int64: value, Valid: value > 0} }
func textValue(value string) pgtype.Text { return pgtype.Text{String: value, Valid: value != ""} }

func timeValue(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}
