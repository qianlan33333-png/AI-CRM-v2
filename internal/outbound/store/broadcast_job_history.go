package store

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
	outbounddb "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type BroadcastJobHistoryStore struct{}
type BroadcastJobHistoryReader struct{ db outbounddb.DBTX }

var _ outboundport.BroadcastJobHistoryStore = (*BroadcastJobHistoryStore)(nil)
var _ outboundport.BroadcastJobHistoryReader = (*BroadcastJobHistoryReader)(nil)

func NewBroadcastJobHistoryStore() *BroadcastJobHistoryStore { return &BroadcastJobHistoryStore{} }
func NewBroadcastJobHistoryReader(db outbounddb.DBTX) *BroadcastJobHistoryReader {
	return &BroadcastJobHistoryReader{db: db}
}

func (store *BroadcastJobHistoryStore) queries(ctx context.Context) (*outbounddb.Queries, error) {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return nil, outboundport.ErrBroadcastJobHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, outboundport.ErrBroadcastJobHistoryUnavailable
	}
	return outbounddb.New(tx), nil
}

func (reader *BroadcastJobHistoryReader) queries(ctx context.Context) (*outbounddb.Queries, error) {
	if reader == nil || ctx == nil || ctx.Err() != nil {
		return nil, outboundport.ErrBroadcastJobHistoryUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return outbounddb.New(tx), nil
	}
	if reader.db == nil {
		return nil, outboundport.ErrBroadcastJobHistoryUnavailable
	}
	value := reflect.ValueOf(reader.db)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return nil, outboundport.ErrBroadcastJobHistoryUnavailable
	}
	return outbounddb.New(reader.db), nil
}

func (store *BroadcastJobHistoryStore) CreateHistoricalBroadcastJob(ctx context.Context, value outboundport.HistoricalBroadcastJob) (outboundport.HistoricalBroadcastJob, error) {
	if value.ID != 0 {
		return outboundport.HistoricalBroadcastJob{}, outboundport.ErrBroadcastJobHistoryInvalid
	}
	check := value
	check.ID = 1
	if _, err := outboundapp.HistoricalBroadcastJobDigest(check); err != nil {
		return outboundport.HistoricalBroadcastJob{}, outboundport.ErrBroadcastJobHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return outboundport.HistoricalBroadcastJob{}, err
	}
	row, err := q.CreateHistoricalBroadcastJob(ctx, broadcastJobHistoryParams(value))
	if err != nil {
		return outboundport.HistoricalBroadcastJob{}, broadcastJobHistoryStoreError(err)
	}
	return broadcastJobHistoryValue(row)
}

func (store *BroadcastJobHistoryStore) GetHistoricalBroadcastJob(ctx context.Context, id int64) (outboundport.HistoricalBroadcastJob, error) {
	if id < 1 {
		return outboundport.HistoricalBroadcastJob{}, outboundport.ErrBroadcastJobHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return outboundport.HistoricalBroadcastJob{}, err
	}
	row, err := q.GetHistoricalBroadcastJob(ctx, id)
	if err != nil {
		return outboundport.HistoricalBroadcastJob{}, broadcastJobHistoryStoreError(err)
	}
	return broadcastJobHistoryValue(row)
}

func (reader *BroadcastJobHistoryReader) GetHistoricalBroadcastJob(ctx context.Context, id int64) (outboundport.HistoricalBroadcastJob, error) {
	if id < 1 {
		return outboundport.HistoricalBroadcastJob{}, outboundport.ErrBroadcastJobHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return outboundport.HistoricalBroadcastJob{}, err
	}
	row, err := q.GetHistoricalBroadcastJob(ctx, id)
	if err != nil {
		return outboundport.HistoricalBroadcastJob{}, broadcastJobHistoryStoreError(err)
	}
	return broadcastJobHistoryValue(row)
}

func (reader *BroadcastJobHistoryReader) ListHistoricalBroadcastJobs(ctx context.Context, query outboundport.BroadcastJobHistoryQuery) ([]outboundport.HistoricalBroadcastJob, int64, error) {
	if query.Limit == 0 {
		query.Limit = 20
	}
	if query.Limit < 1 || query.Limit > 100 || query.Offset < 0 {
		return nil, 0, outboundport.ErrBroadcastJobHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalBroadcastJobs(ctx)
	if err != nil {
		return nil, 0, broadcastJobHistoryStoreError(err)
	}
	rows, err := q.ListHistoricalBroadcastJobs(ctx, outbounddb.ListHistoricalBroadcastJobsParams{Limit: query.Limit, Offset: query.Offset})
	if err != nil {
		return nil, 0, broadcastJobHistoryStoreError(err)
	}
	values := make([]outboundport.HistoricalBroadcastJob, 0, len(rows))
	for _, row := range rows {
		value, err := broadcastJobHistoryValue(row)
		if err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	return values, total, nil
}

func broadcastJobHistoryParams(value outboundport.HistoricalBroadcastJob) outbounddb.CreateHistoricalBroadcastJobParams {
	return outbounddb.CreateHistoricalBroadcastJobParams{
		SourceID: value.SourceID, OriginalSourceType: value.OriginalSourceType, SourceReferenceDigest: value.SourceReferenceDigest[:], SourceTable: value.SourceTable,
		ScheduledFor: broadcastJobHistoryTimestamp(&value.ScheduledFor), Priority: value.Priority, BatchKeyDigest: value.BatchKeyDigest[:], OriginalStatus: value.OriginalStatus,
		RequiresApproval: value.RequiresApproval, ApprovedByDigest: value.ApprovedByDigest[:], ApprovedAt: broadcastJobHistoryTimestamp(value.ApprovedAt),
		CancelledByDigest: value.CancelledByDigest[:], CancelledAt: broadcastJobHistoryTimestamp(value.CancelledAt), CancelReasonDigest: value.CancelReasonDigest[:],
		TargetCount: value.TargetCount, TargetSummaryDigest: value.TargetSummaryDigest[:], ContentType: value.ContentType, ContentPayloadDigest: value.ContentPayloadDigest[:],
		ContentSummaryDigest: value.ContentSummaryDigest[:], AttemptCount: value.AttemptCount, LastErrorDigest: value.LastErrorDigest[:], LegacyOutboundTaskID: broadcastJobHistoryInt(value.LegacyOutboundTaskID),
		SentCount: value.SentCount, FailedCount: value.FailedCount, TraceIDDigest: value.TraceIDDigest[:], CreatedByDigest: value.CreatedByDigest[:],
		CreatedAt: broadcastJobHistoryTimestamp(&value.CreatedAt), UpdatedAt: broadcastJobHistoryTimestamp(&value.UpdatedAt), ClaimedAt: broadcastJobHistoryTimestamp(value.ClaimedAt), SentAt: broadcastJobHistoryTimestamp(value.SentAt),
		ClaimTokenDigest: value.ClaimTokenDigest[:], LeaseExpiresAt: broadcastJobHistoryTimestamp(value.LeaseExpiresAt), BusinessDomain: broadcastJobHistoryText(value.BusinessDomain),
		IdempotencyKeyDigest: broadcastJobHistoryDigest(value.IdempotencyKeyDigest), Channel: broadcastJobHistoryText(value.Channel), TargetKind: broadcastJobHistoryText(value.TargetKind), FailureType: broadcastJobHistoryText(value.FailureType),
		RetryPolicyDigest: value.RetryPolicyDigest[:], MetadataDigest: value.MetadataDigest[:], TargetUnionIDsDigest: value.TargetUnionIDsDigest[:], MaxAttempts: value.MaxAttempts,
		NextRetryAt: broadcastJobHistoryTimestamp(value.NextRetryAt), DispatchStartedAt: broadcastJobHistoryTimestamp(value.DispatchStartedAt), OriginalSideEffectExecuted: value.SideEffectExecuted,
		OriginalProviderResultReceived: value.ProviderResultReceived, ResultSummaryDigest: value.ResultSummaryDigest[:], OriginalReconciliationRequired: value.ReconciliationRequired,
		CompletedAt: broadcastJobHistoryTimestamp(value.CompletedAt), HoldReasonDigest: value.HoldReasonDigest[:], HoldAt: broadcastJobHistoryTimestamp(value.HoldAt), LegacyExternalEffectJobID: broadcastJobHistoryInt(value.LegacyExternalEffectJobID),
		ExecutionIDDigest: value.ExecutionIDDigest[:], ExecutionOwnerDigest: value.ExecutionOwnerDigest[:], SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], RedactedRoots: append([]string{}, value.RedactedRoots...),
	}
}

func broadcastJobHistoryValue(row outbounddb.OutboundV1BroadcastJobHistory) (outboundport.HistoricalBroadcastJob, error) {
	scheduled, created, updated, err := broadcastJobHistoryRequiredTime(row.ScheduledFor, row.CreatedAt, row.UpdatedAt)
	if err != nil {
		return outboundport.HistoricalBroadcastJob{}, outboundport.ErrBroadcastJobHistoryUnavailable
	}
	value := outboundport.HistoricalBroadcastJob{
		ID: row.ID, SourceID: row.SourceID, OriginalSourceType: row.OriginalSourceType, SourceTable: row.SourceTable, ScheduledFor: scheduled, Priority: row.Priority,
		OriginalStatus: row.OriginalStatus, RequiresApproval: row.RequiresApproval, TargetCount: row.TargetCount, ContentType: row.ContentType, AttemptCount: row.AttemptCount,
		SentCount: row.SentCount, FailedCount: row.FailedCount, CreatedAt: created, UpdatedAt: updated, MaxAttempts: row.MaxAttempts,
		SideEffectExecuted: row.OriginalSideEffectExecuted, ProviderResultReceived: row.OriginalProviderResultReceived, ReconciliationRequired: row.OriginalReconciliationRequired,
		ApprovedAt: broadcastJobHistoryOptionalTime(row.ApprovedAt), CancelledAt: broadcastJobHistoryOptionalTime(row.CancelledAt), ClaimedAt: broadcastJobHistoryOptionalTime(row.ClaimedAt), SentAt: broadcastJobHistoryOptionalTime(row.SentAt), LeaseExpiresAt: broadcastJobHistoryOptionalTime(row.LeaseExpiresAt),
		BusinessDomain: broadcastJobHistoryOptionalText(row.BusinessDomain), Channel: broadcastJobHistoryOptionalText(row.Channel), TargetKind: broadcastJobHistoryOptionalText(row.TargetKind), FailureType: broadcastJobHistoryOptionalText(row.FailureType),
		NextRetryAt: broadcastJobHistoryOptionalTime(row.NextRetryAt), DispatchStartedAt: broadcastJobHistoryOptionalTime(row.DispatchStartedAt), CompletedAt: broadcastJobHistoryOptionalTime(row.CompletedAt), HoldAt: broadcastJobHistoryOptionalTime(row.HoldAt),
		LegacyOutboundTaskID: broadcastJobHistoryOptionalInt(row.LegacyOutboundTaskID), LegacyExternalEffectJobID: broadcastJobHistoryOptionalInt(row.LegacyExternalEffectJobID), RedactedRoots: append([]string{}, row.RedactedRoots...),
	}
	for _, digest := range []struct {
		source []byte
		target *[32]byte
	}{
		{row.SourceReferenceDigest, &value.SourceReferenceDigest}, {row.BatchKeyDigest, &value.BatchKeyDigest}, {row.ApprovedByDigest, &value.ApprovedByDigest}, {row.CancelledByDigest, &value.CancelledByDigest},
		{row.CancelReasonDigest, &value.CancelReasonDigest}, {row.TargetSummaryDigest, &value.TargetSummaryDigest}, {row.ContentPayloadDigest, &value.ContentPayloadDigest}, {row.ContentSummaryDigest, &value.ContentSummaryDigest},
		{row.LastErrorDigest, &value.LastErrorDigest}, {row.TraceIDDigest, &value.TraceIDDigest}, {row.CreatedByDigest, &value.CreatedByDigest}, {row.ClaimTokenDigest, &value.ClaimTokenDigest},
		{row.RetryPolicyDigest, &value.RetryPolicyDigest}, {row.MetadataDigest, &value.MetadataDigest}, {row.TargetUnionIDsDigest, &value.TargetUnionIDsDigest}, {row.ResultSummaryDigest, &value.ResultSummaryDigest},
		{row.HoldReasonDigest, &value.HoldReasonDigest}, {row.ExecutionIDDigest, &value.ExecutionIDDigest}, {row.ExecutionOwnerDigest, &value.ExecutionOwnerDigest}, {row.SourceKeyDigest, &value.SourceKeyDigest},
		{row.SourcePayloadDigest, &value.SourcePayloadDigest}, {row.SourceFieldDigest, &value.SourceFieldDigest},
	} {
		if len(digest.source) != 32 {
			return outboundport.HistoricalBroadcastJob{}, outboundport.ErrBroadcastJobHistoryUnavailable
		}
		copy(digest.target[:], digest.source)
	}
	if row.IdempotencyKeyDigest != nil {
		if len(row.IdempotencyKeyDigest) != 32 {
			return outboundport.HistoricalBroadcastJob{}, outboundport.ErrBroadcastJobHistoryUnavailable
		}
		var digest [32]byte
		copy(digest[:], row.IdempotencyKeyDigest)
		value.IdempotencyKeyDigest = &digest
	}
	if _, err := outboundapp.HistoricalBroadcastJobDigest(value); err != nil {
		return outboundport.HistoricalBroadcastJob{}, outboundport.ErrBroadcastJobHistoryUnavailable
	}
	return value, nil
}

func broadcastJobHistoryTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC().Truncate(time.Microsecond), Valid: true}
}
func broadcastJobHistoryInt(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}
func broadcastJobHistoryText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}
func broadcastJobHistoryDigest(value *[32]byte) []byte {
	if value == nil {
		return nil
	}
	return value[:]
}
func broadcastJobHistoryOptionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid || value.InfinityModifier != pgtype.Finite {
		return nil
	}
	at := value.Time.UTC().Truncate(time.Microsecond)
	return &at
}
func broadcastJobHistoryOptionalInt(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	copy := value.Int64
	return &copy
}
func broadcastJobHistoryOptionalText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}
func broadcastJobHistoryRequiredTime(values ...pgtype.Timestamptz) (time.Time, time.Time, time.Time, error) {
	if len(values) != 3 || !values[0].Valid || !values[1].Valid || !values[2].Valid || values[0].InfinityModifier != pgtype.Finite || values[1].InfinityModifier != pgtype.Finite || values[2].InfinityModifier != pgtype.Finite {
		return time.Time{}, time.Time{}, time.Time{}, errors.New("invalid required time")
	}
	return values[0].Time.UTC().Truncate(time.Microsecond), values[1].Time.UTC().Truncate(time.Microsecond), values[2].Time.UTC().Truncate(time.Microsecond), nil
}

func broadcastJobHistoryStoreError(err error) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		return outboundport.ErrBroadcastJobHistoryConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return outboundport.ErrBroadcastJobHistoryConflict
	}
	return outboundport.ErrBroadcastJobHistoryUnavailable
}
