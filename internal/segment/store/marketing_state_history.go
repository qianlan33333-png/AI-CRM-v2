package store

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segment "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentdb "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store/generated"
)

// MarketingStateHistoryStore deliberately has no pool: historical writes must
// share the caller transaction with the import journal.
type MarketingStateHistoryStore struct{}
type MarketingStateHistoryReader struct{ db segmentdb.DBTX }

var _ segment.MarketingStateHistoryStore = (*MarketingStateHistoryStore)(nil)
var _ segment.MarketingStateHistoryReader = (*MarketingStateHistoryReader)(nil)

func NewMarketingStateHistoryStore() *MarketingStateHistoryStore {
	return &MarketingStateHistoryStore{}
}
func NewMarketingStateHistoryReader(db segmentdb.DBTX) *MarketingStateHistoryReader {
	return &MarketingStateHistoryReader{db: db}
}

func (s *MarketingStateHistoryStore) q(ctx context.Context) (*segmentdb.Queries, error) {
	if s == nil || ctx == nil || ctx.Err() != nil {
		return nil, segment.ErrMarketingStateHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil || tx == nil {
		return nil, segment.ErrMarketingStateHistoryUnavailable
	}
	return segmentdb.New(tx), nil
}
func (r *MarketingStateHistoryReader) q(ctx context.Context) (*segmentdb.Queries, error) {
	if r == nil || ctx == nil || ctx.Err() != nil {
		return nil, segment.ErrMarketingStateHistoryUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil && tx != nil {
		return segmentdb.New(tx), nil
	}
	if nilMarketingStateDB(r.db) {
		return nil, segment.ErrMarketingStateHistoryUnavailable
	}
	return segmentdb.New(r.db), nil
}
func nilMarketingStateDB(v segmentdb.DBTX) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return r.IsNil()
	}
	return false
}

func (s *MarketingStateHistoryStore) CreateHistoricalMarketingStateSnapshot(ctx context.Context, v segment.HistoricalMarketingStateSnapshot) (segment.HistoricalMarketingStateSnapshot, error) {
	if v.ID != 0 || badMarketingStateSnapshot(v) {
		return segment.HistoricalMarketingStateSnapshot{}, segment.ErrMarketingStateHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return segment.HistoricalMarketingStateSnapshot{}, err
	}
	r, err := q.CreateHistoricalMarketingStateSnapshot(ctx, segmentdb.CreateHistoricalMarketingStateSnapshotParams{SourceKeyDigest: v.SourceKeyDigest[:], SourcePayloadDigest: v.SourcePayloadDigest[:], SourceFieldDigest: v.SourceFieldDigest[:], SourceID: v.SourceID, PersonSourceID: mi(v.PersonSourceID), ExternalUseridDigest: v.ExternalUserIDDigest[:], AutomationKey: v.AutomationKey, MainStage: v.MainStage, SubStage: v.SubStage, Activated: v.Activated, Converted: v.Converted, EligibleForConversion: v.EligibleForConversion, LifecycleStatus: v.LifecycleStatus, LastActivationAt: v.LastActivationAt, LastConversionMarkedAt: v.LastConversionMarkedAt, LastMessageAt: v.LastMessageAt, LastBatchSourceID: mi(v.LastBatchSourceID), LastBatchStatus: v.LastBatchStatus, LastBatchWindowStart: v.LastBatchWindowStart, LastBatchWindowEnd: v.LastBatchWindowEnd, LastTriggerMessageAt: v.LastTriggerMessageAt, EnteredAt: mt(v.EnteredAt), ExitedAt: mt(v.ExitedAt), ExitReason: v.ExitReason, StatePayloadDigest: v.StatePayloadDigest[:], CreatedAt: mt(&v.CreatedAt), UpdatedAt: mt(&v.UpdatedAt)})
	if err != nil {
		return segment.HistoricalMarketingStateSnapshot{}, marketingStateDBError(err)
	}
	return marketingStateSnapshotValue(r)
}
func (s *MarketingStateHistoryStore) GetHistoricalMarketingStateSnapshot(ctx context.Context, id int64) (segment.HistoricalMarketingStateSnapshot, error) {
	if id < 1 {
		return segment.HistoricalMarketingStateSnapshot{}, segment.ErrMarketingStateHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return segment.HistoricalMarketingStateSnapshot{}, err
	}
	r, err := q.GetHistoricalMarketingStateSnapshot(ctx, id)
	if err != nil {
		return segment.HistoricalMarketingStateSnapshot{}, marketingStateDBError(err)
	}
	return marketingStateSnapshotValue(r)
}
func (r *MarketingStateHistoryReader) GetHistoricalMarketingStateSnapshot(ctx context.Context, id int64) (segment.HistoricalMarketingStateSnapshot, error) {
	if id < 1 {
		return segment.HistoricalMarketingStateSnapshot{}, segment.ErrMarketingStateHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return segment.HistoricalMarketingStateSnapshot{}, err
	}
	x, err := q.GetHistoricalMarketingStateSnapshot(ctx, id)
	if err != nil {
		return segment.HistoricalMarketingStateSnapshot{}, marketingStateDBError(err)
	}
	return marketingStateSnapshotValue(x)
}
func (r *MarketingStateHistoryReader) ListHistoricalMarketingStateSnapshot(ctx context.Context, x segment.MarketingStateHistoryQuery) ([]segment.HistoricalMarketingStateSnapshot, int64, error) {
	if badMarketingStateQuery(x) {
		return nil, 0, segment.ErrMarketingStateHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	n, err := q.CountHistoricalMarketingStateSnapshot(ctx)
	if err != nil {
		return nil, 0, marketingStateDBError(err)
	}
	rows, err := q.ListHistoricalMarketingStateSnapshot(ctx, segmentdb.ListHistoricalMarketingStateSnapshotParams{Limit: x.Limit, Offset: x.Offset})
	if err != nil {
		return nil, 0, marketingStateDBError(err)
	}
	out := make([]segment.HistoricalMarketingStateSnapshot, 0, len(rows))
	for _, row := range rows {
		v, e := marketingStateSnapshotValue(row)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, v)
	}
	return out, n, nil
}

func (s *MarketingStateHistoryStore) CreateHistoricalMarketingStateChange(ctx context.Context, v segment.HistoricalMarketingStateChange) (segment.HistoricalMarketingStateChange, error) {
	if v.ID != 0 || badMarketingStateChange(v) {
		return segment.HistoricalMarketingStateChange{}, segment.ErrMarketingStateHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return segment.HistoricalMarketingStateChange{}, err
	}
	r, err := q.CreateHistoricalMarketingStateChange(ctx, segmentdb.CreateHistoricalMarketingStateChangeParams{SourceKeyDigest: v.SourceKeyDigest[:], SourcePayloadDigest: v.SourcePayloadDigest[:], SourceFieldDigest: v.SourceFieldDigest[:], SourceID: v.SourceID, PersonSourceID: mi(v.PersonSourceID), BatchSourceID: mi(v.BatchSourceID), ExternalUseridDigest: v.ExternalUserIDDigest[:], AutomationKey: v.AutomationKey, MainStage: v.MainStage, SubStage: v.SubStage, Activated: v.Activated, Converted: v.Converted, EligibleForConversion: v.EligibleForConversion, LifecycleStatus: v.LifecycleStatus, LastActivationAt: v.LastActivationAt, LastConversionMarkedAt: v.LastConversionMarkedAt, LastMessageAt: v.LastMessageAt, ExitReason: v.ExitReason, ChangeReason: v.ChangeReason, StatePayloadDigest: v.StatePayloadDigest[:], RecordedAt: mt(&v.RecordedAt), CreatedAt: mt(&v.CreatedAt)})
	if err != nil {
		return segment.HistoricalMarketingStateChange{}, marketingStateDBError(err)
	}
	return marketingStateChangeValue(r)
}
func (s *MarketingStateHistoryStore) GetHistoricalMarketingStateChange(ctx context.Context, id int64) (segment.HistoricalMarketingStateChange, error) {
	if id < 1 {
		return segment.HistoricalMarketingStateChange{}, segment.ErrMarketingStateHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return segment.HistoricalMarketingStateChange{}, err
	}
	r, err := q.GetHistoricalMarketingStateChange(ctx, id)
	if err != nil {
		return segment.HistoricalMarketingStateChange{}, marketingStateDBError(err)
	}
	return marketingStateChangeValue(r)
}
func (r *MarketingStateHistoryReader) GetHistoricalMarketingStateChange(ctx context.Context, id int64) (segment.HistoricalMarketingStateChange, error) {
	if id < 1 {
		return segment.HistoricalMarketingStateChange{}, segment.ErrMarketingStateHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return segment.HistoricalMarketingStateChange{}, err
	}
	x, err := q.GetHistoricalMarketingStateChange(ctx, id)
	if err != nil {
		return segment.HistoricalMarketingStateChange{}, marketingStateDBError(err)
	}
	return marketingStateChangeValue(x)
}
func (r *MarketingStateHistoryReader) ListHistoricalMarketingStateChange(ctx context.Context, x segment.MarketingStateHistoryQuery) ([]segment.HistoricalMarketingStateChange, int64, error) {
	if badMarketingStateQuery(x) {
		return nil, 0, segment.ErrMarketingStateHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	n, err := q.CountHistoricalMarketingStateChange(ctx)
	if err != nil {
		return nil, 0, marketingStateDBError(err)
	}
	rows, err := q.ListHistoricalMarketingStateChange(ctx, segmentdb.ListHistoricalMarketingStateChangeParams{Limit: x.Limit, Offset: x.Offset})
	if err != nil {
		return nil, 0, marketingStateDBError(err)
	}
	out := make([]segment.HistoricalMarketingStateChange, 0, len(rows))
	for _, row := range rows {
		v, e := marketingStateChangeValue(row)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, v)
	}
	return out, n, nil
}

func (s *MarketingStateHistoryStore) CreateHistoricalValueSegmentSnapshot(ctx context.Context, v segment.HistoricalValueSegmentSnapshot) (segment.HistoricalValueSegmentSnapshot, error) {
	if v.ID != 0 || badValueSegmentSnapshot(v) {
		return segment.HistoricalValueSegmentSnapshot{}, segment.ErrMarketingStateHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return segment.HistoricalValueSegmentSnapshot{}, err
	}
	r, err := q.CreateHistoricalValueSegmentSnapshot(ctx, segmentdb.CreateHistoricalValueSegmentSnapshotParams{SourceKeyDigest: v.SourceKeyDigest[:], SourcePayloadDigest: v.SourcePayloadDigest[:], SourceFieldDigest: v.SourceFieldDigest[:], SourceID: v.SourceID, ExternalUseridDigest: v.ExternalUserIDDigest[:], Segment: v.Segment, SegmentRank: v.SegmentRank, Score: v.Score, ScoringVersion: v.ScoringVersion, SubmissionSourceID: mi(v.SubmissionSourceID), MatchedQuestionIdsDigest: v.MatchedQuestionIDsDigest[:], StatePayloadDigest: v.StatePayloadDigest[:], ComputedReason: v.ComputedReason, EvaluatedAt: mt(&v.EvaluatedAt), ComputedAt: mt(&v.ComputedAt), CreatedAt: mt(&v.CreatedAt), UpdatedAt: mt(&v.UpdatedAt)})
	if err != nil {
		return segment.HistoricalValueSegmentSnapshot{}, marketingStateDBError(err)
	}
	return valueSegmentSnapshotValue(r)
}
func (s *MarketingStateHistoryStore) GetHistoricalValueSegmentSnapshot(ctx context.Context, id int64) (segment.HistoricalValueSegmentSnapshot, error) {
	if id < 1 {
		return segment.HistoricalValueSegmentSnapshot{}, segment.ErrMarketingStateHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return segment.HistoricalValueSegmentSnapshot{}, err
	}
	x, err := q.GetHistoricalValueSegmentSnapshot(ctx, id)
	if err != nil {
		return segment.HistoricalValueSegmentSnapshot{}, marketingStateDBError(err)
	}
	return valueSegmentSnapshotValue(x)
}
func (r *MarketingStateHistoryReader) GetHistoricalValueSegmentSnapshot(ctx context.Context, id int64) (segment.HistoricalValueSegmentSnapshot, error) {
	if id < 1 {
		return segment.HistoricalValueSegmentSnapshot{}, segment.ErrMarketingStateHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return segment.HistoricalValueSegmentSnapshot{}, err
	}
	x, err := q.GetHistoricalValueSegmentSnapshot(ctx, id)
	if err != nil {
		return segment.HistoricalValueSegmentSnapshot{}, marketingStateDBError(err)
	}
	return valueSegmentSnapshotValue(x)
}
func (r *MarketingStateHistoryReader) ListHistoricalValueSegmentSnapshot(ctx context.Context, x segment.MarketingStateHistoryQuery) ([]segment.HistoricalValueSegmentSnapshot, int64, error) {
	if badMarketingStateQuery(x) {
		return nil, 0, segment.ErrMarketingStateHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	n, err := q.CountHistoricalValueSegmentSnapshot(ctx)
	if err != nil {
		return nil, 0, marketingStateDBError(err)
	}
	rows, err := q.ListHistoricalValueSegmentSnapshot(ctx, segmentdb.ListHistoricalValueSegmentSnapshotParams{Limit: x.Limit, Offset: x.Offset})
	if err != nil {
		return nil, 0, marketingStateDBError(err)
	}
	out := make([]segment.HistoricalValueSegmentSnapshot, 0, len(rows))
	for _, row := range rows {
		v, e := valueSegmentSnapshotValue(row)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, v)
	}
	return out, n, nil
}

func (s *MarketingStateHistoryStore) CreateHistoricalValueSegmentChange(ctx context.Context, v segment.HistoricalValueSegmentChange) (segment.HistoricalValueSegmentChange, error) {
	if v.ID != 0 || badValueSegmentChange(v) {
		return segment.HistoricalValueSegmentChange{}, segment.ErrMarketingStateHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return segment.HistoricalValueSegmentChange{}, err
	}
	x, err := q.CreateHistoricalValueSegmentChange(ctx, segmentdb.CreateHistoricalValueSegmentChangeParams{SourceKeyDigest: v.SourceKeyDigest[:], SourcePayloadDigest: v.SourcePayloadDigest[:], SourceFieldDigest: v.SourceFieldDigest[:], SourceID: v.SourceID, ExternalUseridDigest: v.ExternalUserIDDigest[:], Segment: v.Segment, SegmentRank: v.SegmentRank, Score: v.Score, ScoringVersion: v.ScoringVersion, SubmissionSourceID: mi(v.SubmissionSourceID), MatchedQuestionIdsDigest: v.MatchedQuestionIDsDigest[:], StatePayloadDigest: v.StatePayloadDigest[:], ChangeReason: v.ChangeReason, EvaluatedAt: mt(&v.EvaluatedAt), RecordedAt: mt(&v.RecordedAt), CreatedAt: mt(&v.CreatedAt)})
	if err != nil {
		return segment.HistoricalValueSegmentChange{}, marketingStateDBError(err)
	}
	return valueSegmentChangeValue(x)
}
func (s *MarketingStateHistoryStore) GetHistoricalValueSegmentChange(ctx context.Context, id int64) (segment.HistoricalValueSegmentChange, error) {
	if id < 1 {
		return segment.HistoricalValueSegmentChange{}, segment.ErrMarketingStateHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return segment.HistoricalValueSegmentChange{}, err
	}
	x, err := q.GetHistoricalValueSegmentChange(ctx, id)
	if err != nil {
		return segment.HistoricalValueSegmentChange{}, marketingStateDBError(err)
	}
	return valueSegmentChangeValue(x)
}
func (r *MarketingStateHistoryReader) GetHistoricalValueSegmentChange(ctx context.Context, id int64) (segment.HistoricalValueSegmentChange, error) {
	if id < 1 {
		return segment.HistoricalValueSegmentChange{}, segment.ErrMarketingStateHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return segment.HistoricalValueSegmentChange{}, err
	}
	x, err := q.GetHistoricalValueSegmentChange(ctx, id)
	if err != nil {
		return segment.HistoricalValueSegmentChange{}, marketingStateDBError(err)
	}
	return valueSegmentChangeValue(x)
}
func (r *MarketingStateHistoryReader) ListHistoricalValueSegmentChange(ctx context.Context, x segment.MarketingStateHistoryQuery) ([]segment.HistoricalValueSegmentChange, int64, error) {
	if badMarketingStateQuery(x) {
		return nil, 0, segment.ErrMarketingStateHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	n, err := q.CountHistoricalValueSegmentChange(ctx)
	if err != nil {
		return nil, 0, marketingStateDBError(err)
	}
	rows, err := q.ListHistoricalValueSegmentChange(ctx, segmentdb.ListHistoricalValueSegmentChangeParams{Limit: x.Limit, Offset: x.Offset})
	if err != nil {
		return nil, 0, marketingStateDBError(err)
	}
	out := make([]segment.HistoricalValueSegmentChange, 0, len(rows))
	for _, row := range rows {
		v, e := valueSegmentChangeValue(row)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, v)
	}
	return out, n, nil
}

func mi(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
func mt(v *time.Time) pgtype.Timestamptz {
	if v == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *v, Valid: true}
}
func mtime(v pgtype.Timestamptz) (time.Time, bool) {
	if !v.Valid || v.InfinityModifier != pgtype.Finite {
		return time.Time{}, false
	}
	return v.Time.UTC().Truncate(time.Microsecond), true
}
func mint(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}
func mdigest(values ...[]byte) ([][32]byte, bool) {
	out := make([][32]byte, len(values))
	for i, v := range values {
		if len(v) != 32 {
			return nil, false
		}
		copy(out[i][:], v)
		if out[i] == ([32]byte{}) {
			return nil, false
		}
	}
	return out, true
}

func marketingStateSnapshotValue(r segmentdb.SegmentV1MarketingStateSnapshot) (segment.HistoricalMarketingStateSnapshot, error) {
	d, ok := mdigest(r.SourceKeyDigest, r.SourcePayloadDigest, r.SourceFieldDigest, r.ExternalUseridDigest, r.StatePayloadDigest)
	created, a := mtime(r.CreatedAt)
	updated, b := mtime(r.UpdatedAt)
	entered, c := mtime(r.EnteredAt)
	exited, e := mtime(r.ExitedAt)
	var ep, xp *time.Time
	if r.EnteredAt.Valid {
		ep = &entered
	}
	if r.ExitedAt.Valid {
		xp = &exited
	}
	v := segment.HistoricalMarketingStateSnapshot{ID: r.ID, SourceKeyDigest: dAt(d, 0), SourcePayloadDigest: dAt(d, 1), SourceFieldDigest: dAt(d, 2), SourceID: r.SourceID, PersonSourceID: mint(r.PersonSourceID), ExternalUserIDDigest: dAt(d, 3), AutomationKey: r.AutomationKey, MainStage: r.MainStage, SubStage: r.SubStage, Activated: r.Activated, Converted: r.Converted, EligibleForConversion: r.EligibleForConversion, LifecycleStatus: r.LifecycleStatus, LastActivationAt: r.LastActivationAt, LastConversionMarkedAt: r.LastConversionMarkedAt, LastMessageAt: r.LastMessageAt, LastBatchSourceID: mint(r.LastBatchSourceID), LastBatchStatus: r.LastBatchStatus, LastBatchWindowStart: r.LastBatchWindowStart, LastBatchWindowEnd: r.LastBatchWindowEnd, LastTriggerMessageAt: r.LastTriggerMessageAt, EnteredAt: ep, ExitedAt: xp, ExitReason: r.ExitReason, StatePayloadDigest: dAt(d, 4), CreatedAt: created, UpdatedAt: updated}
	if !ok || !a || !b || (r.EnteredAt.Valid && !c) || (r.ExitedAt.Valid && !e) || badMarketingStateSnapshot(v) {
		return segment.HistoricalMarketingStateSnapshot{}, segment.ErrMarketingStateHistoryUnavailable
	}
	return v, nil
}
func marketingStateChangeValue(r segmentdb.SegmentV1MarketingStateChange) (segment.HistoricalMarketingStateChange, error) {
	d, ok := mdigest(r.SourceKeyDigest, r.SourcePayloadDigest, r.SourceFieldDigest, r.ExternalUseridDigest, r.StatePayloadDigest)
	recorded, a := mtime(r.RecordedAt)
	created, b := mtime(r.CreatedAt)
	v := segment.HistoricalMarketingStateChange{ID: r.ID, SourceKeyDigest: dAt(d, 0), SourcePayloadDigest: dAt(d, 1), SourceFieldDigest: dAt(d, 2), SourceID: r.SourceID, PersonSourceID: mint(r.PersonSourceID), BatchSourceID: mint(r.BatchSourceID), ExternalUserIDDigest: dAt(d, 3), AutomationKey: r.AutomationKey, MainStage: r.MainStage, SubStage: r.SubStage, Activated: r.Activated, Converted: r.Converted, EligibleForConversion: r.EligibleForConversion, LifecycleStatus: r.LifecycleStatus, LastActivationAt: r.LastActivationAt, LastConversionMarkedAt: r.LastConversionMarkedAt, LastMessageAt: r.LastMessageAt, ExitReason: r.ExitReason, ChangeReason: r.ChangeReason, StatePayloadDigest: dAt(d, 4), RecordedAt: recorded, CreatedAt: created}
	if !ok || !a || !b || badMarketingStateChange(v) {
		return segment.HistoricalMarketingStateChange{}, segment.ErrMarketingStateHistoryUnavailable
	}
	return v, nil
}
func valueSegmentSnapshotValue(r segmentdb.SegmentV1ValueSegmentSnapshot) (segment.HistoricalValueSegmentSnapshot, error) {
	d, ok := mdigest(r.SourceKeyDigest, r.SourcePayloadDigest, r.SourceFieldDigest, r.ExternalUseridDigest, r.MatchedQuestionIdsDigest, r.StatePayloadDigest)
	evaluated, a := mtime(r.EvaluatedAt)
	computed, b := mtime(r.ComputedAt)
	created, c := mtime(r.CreatedAt)
	updated, e := mtime(r.UpdatedAt)
	v := segment.HistoricalValueSegmentSnapshot{ID: r.ID, SourceKeyDigest: dAt(d, 0), SourcePayloadDigest: dAt(d, 1), SourceFieldDigest: dAt(d, 2), SourceID: r.SourceID, ExternalUserIDDigest: dAt(d, 3), Segment: r.Segment, SegmentRank: r.SegmentRank, Score: r.Score, ScoringVersion: r.ScoringVersion, SubmissionSourceID: mint(r.SubmissionSourceID), MatchedQuestionIDsDigest: dAt(d, 4), StatePayloadDigest: dAt(d, 5), ComputedReason: r.ComputedReason, EvaluatedAt: evaluated, ComputedAt: computed, CreatedAt: created, UpdatedAt: updated}
	if !ok || !a || !b || !c || !e || badValueSegmentSnapshot(v) {
		return segment.HistoricalValueSegmentSnapshot{}, segment.ErrMarketingStateHistoryUnavailable
	}
	return v, nil
}
func valueSegmentChangeValue(r segmentdb.SegmentV1ValueSegmentChange) (segment.HistoricalValueSegmentChange, error) {
	d, ok := mdigest(r.SourceKeyDigest, r.SourcePayloadDigest, r.SourceFieldDigest, r.ExternalUseridDigest, r.MatchedQuestionIdsDigest, r.StatePayloadDigest)
	evaluated, a := mtime(r.EvaluatedAt)
	recorded, b := mtime(r.RecordedAt)
	created, c := mtime(r.CreatedAt)
	v := segment.HistoricalValueSegmentChange{ID: r.ID, SourceKeyDigest: dAt(d, 0), SourcePayloadDigest: dAt(d, 1), SourceFieldDigest: dAt(d, 2), SourceID: r.SourceID, ExternalUserIDDigest: dAt(d, 3), Segment: r.Segment, SegmentRank: r.SegmentRank, Score: r.Score, ScoringVersion: r.ScoringVersion, SubmissionSourceID: mint(r.SubmissionSourceID), MatchedQuestionIDsDigest: dAt(d, 4), StatePayloadDigest: dAt(d, 5), ChangeReason: r.ChangeReason, EvaluatedAt: evaluated, RecordedAt: recorded, CreatedAt: created}
	if !ok || !a || !b || !c || badValueSegmentChange(v) {
		return segment.HistoricalValueSegmentChange{}, segment.ErrMarketingStateHistoryUnavailable
	}
	return v, nil
}
func dAt(v [][32]byte, n int) [32]byte {
	if n >= len(v) {
		return [32]byte{}
	}
	return v[n]
}
func badMarketingStateSnapshot(v segment.HistoricalMarketingStateSnapshot) bool {
	if v.ID == 0 {
		v.ID = 1
	}
	_, err := segmentapp.HistoricalMarketingStateSnapshotDigest(v)
	return err != nil
}
func badMarketingStateChange(v segment.HistoricalMarketingStateChange) bool {
	if v.ID == 0 {
		v.ID = 1
	}
	_, err := segmentapp.HistoricalMarketingStateChangeDigest(v)
	return err != nil
}
func badValueSegmentSnapshot(v segment.HistoricalValueSegmentSnapshot) bool {
	if v.ID == 0 {
		v.ID = 1
	}
	_, err := segmentapp.HistoricalValueSegmentSnapshotDigest(v)
	return err != nil
}
func badValueSegmentChange(v segment.HistoricalValueSegmentChange) bool {
	if v.ID == 0 {
		v.ID = 1
	}
	_, err := segmentapp.HistoricalValueSegmentChangeDigest(v)
	return err != nil
}
func badMarketingStateQuery(v segment.MarketingStateHistoryQuery) bool {
	return v.Limit < 1 || v.Limit > 100 || v.Offset < 0
}
func marketingStateDBError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return segment.ErrMarketingStateHistoryConflict
	}
	return segment.ErrMarketingStateHistoryUnavailable
}
