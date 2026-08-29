package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segment "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentdb "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store/generated"
)

// AudienceActivityHistoryStore writes only through the caller transaction.
type AudienceActivityHistoryStore struct{}
type AudienceActivityHistoryReader struct{ db segmentdb.DBTX }

var _ segment.AudienceActivityHistoryStore = (*AudienceActivityHistoryStore)(nil)
var _ segment.AudienceActivityHistoryReader = (*AudienceActivityHistoryReader)(nil)
var _ segment.AudienceActivityHistoryReferences = (*AudienceActivityHistoryReader)(nil)

func NewAudienceActivityHistoryStore() *AudienceActivityHistoryStore {
	return &AudienceActivityHistoryStore{}
}
func NewAudienceActivityHistoryReader(db segmentdb.DBTX) *AudienceActivityHistoryReader {
	return &AudienceActivityHistoryReader{db: db}
}

func (store *AudienceActivityHistoryStore) q(ctx context.Context) (*segmentdb.Queries, error) {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return nil, segment.ErrAudienceActivityHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil || tx == nil {
		return nil, segment.ErrAudienceActivityHistoryUnavailable
	}
	return segmentdb.New(tx), nil
}

func (reader *AudienceActivityHistoryReader) q(ctx context.Context) (*segmentdb.Queries, error) {
	if reader == nil || ctx == nil || ctx.Err() != nil {
		return nil, segment.ErrAudienceActivityHistoryUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil && tx != nil {
		return segmentdb.New(tx), nil
	}
	if nilAudienceActivityDB(reader.db) {
		return nil, segment.ErrAudienceActivityHistoryUnavailable
	}
	return segmentdb.New(reader.db), nil
}

func nilAudienceActivityDB(value segmentdb.DBTX) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	return (ref.Kind() == reflect.Chan || ref.Kind() == reflect.Func || ref.Kind() == reflect.Interface || ref.Kind() == reflect.Map || ref.Kind() == reflect.Pointer || ref.Kind() == reflect.Slice) && ref.IsNil()
}

func (store *AudienceActivityHistoryStore) CreateHistoricalAudienceActivityRun(ctx context.Context, value segment.HistoricalAudienceActivityRun) (segment.HistoricalAudienceActivityRun, error) {
	if value.ID != 0 || !validAudienceActivityRun(value) {
		return segment.HistoricalAudienceActivityRun{}, segment.ErrAudienceActivityHistoryInvalid
	}
	q, err := store.q(ctx)
	if err != nil {
		return segment.HistoricalAudienceActivityRun{}, err
	}
	row, err := q.CreateHistoricalAudienceActivityRun(ctx, audienceActivityRunParams(value))
	if err != nil {
		return segment.HistoricalAudienceActivityRun{}, audienceActivityStoreError(err)
	}
	return audienceActivityRunValue(row)
}

func (store *AudienceActivityHistoryStore) GetHistoricalAudienceActivityRun(ctx context.Context, id int64) (segment.HistoricalAudienceActivityRun, error) {
	if id < 1 {
		return segment.HistoricalAudienceActivityRun{}, segment.ErrAudienceActivityHistoryInvalid
	}
	q, err := store.q(ctx)
	if err != nil {
		return segment.HistoricalAudienceActivityRun{}, err
	}
	row, err := q.GetHistoricalAudienceActivityRun(ctx, id)
	if err != nil {
		return segment.HistoricalAudienceActivityRun{}, audienceActivityStoreError(err)
	}
	return audienceActivityRunValue(row)
}

func (store *AudienceActivityHistoryStore) CreateHistoricalAudienceActivityMemberEvent(ctx context.Context, value segment.HistoricalAudienceActivityMemberEvent) (segment.HistoricalAudienceActivityMemberEvent, error) {
	if value.ID != 0 || !validAudienceActivityEvent(value) {
		return segment.HistoricalAudienceActivityMemberEvent{}, segment.ErrAudienceActivityHistoryInvalid
	}
	q, err := store.q(ctx)
	if err != nil {
		return segment.HistoricalAudienceActivityMemberEvent{}, err
	}
	row, err := q.CreateHistoricalAudienceActivityMemberEvent(ctx, audienceActivityEventParams(value))
	if err != nil {
		return segment.HistoricalAudienceActivityMemberEvent{}, audienceActivityStoreError(err)
	}
	return audienceActivityEventValue(row)
}

func (store *AudienceActivityHistoryStore) GetHistoricalAudienceActivityMemberEvent(ctx context.Context, id int64) (segment.HistoricalAudienceActivityMemberEvent, error) {
	if id < 1 {
		return segment.HistoricalAudienceActivityMemberEvent{}, segment.ErrAudienceActivityHistoryInvalid
	}
	q, err := store.q(ctx)
	if err != nil {
		return segment.HistoricalAudienceActivityMemberEvent{}, err
	}
	row, err := q.GetHistoricalAudienceActivityMemberEvent(ctx, id)
	if err != nil {
		return segment.HistoricalAudienceActivityMemberEvent{}, audienceActivityStoreError(err)
	}
	return audienceActivityEventValue(row)
}

func (store *AudienceActivityHistoryStore) GetHistoricalAudienceActivityPackage(ctx context.Context, id int64) (segment.AudienceActivityPackageReference, error) {
	return NewAudienceActivityHistoryReader(nil).getPackage(ctx, id)
}
func (store *AudienceActivityHistoryStore) GetHistoricalAudienceActivityVersion(ctx context.Context, id int64) (segment.AudienceActivityVersionReference, error) {
	return NewAudienceActivityHistoryReader(nil).getVersion(ctx, id)
}
func (store *AudienceActivityHistoryStore) GetHistoricalAudienceActivityMember(ctx context.Context, id int64) (segment.AudienceActivityMemberReference, error) {
	return NewAudienceActivityHistoryReader(nil).getMember(ctx, id)
}

func (reader *AudienceActivityHistoryReader) getPackage(ctx context.Context, id int64) (segment.AudienceActivityPackageReference, error) {
	if id < 1 {
		return segment.AudienceActivityPackageReference{}, segment.ErrAudienceActivityHistoryInvalid
	}
	q, err := reader.q(ctx)
	if err != nil {
		return segment.AudienceActivityPackageReference{}, err
	}
	row, err := q.GetHistoricalAudiencePackage(ctx, id)
	if err != nil {
		return segment.AudienceActivityPackageReference{}, audienceActivityStoreError(err)
	}
	if row.ID != id {
		return segment.AudienceActivityPackageReference{}, segment.ErrAudienceActivityHistoryConflict
	}
	return segment.AudienceActivityPackageReference{ID: row.ID}, nil
}
func (reader *AudienceActivityHistoryReader) getVersion(ctx context.Context, id int64) (segment.AudienceActivityVersionReference, error) {
	if id < 1 {
		return segment.AudienceActivityVersionReference{}, segment.ErrAudienceActivityHistoryInvalid
	}
	q, err := reader.q(ctx)
	if err != nil {
		return segment.AudienceActivityVersionReference{}, err
	}
	row, err := q.GetHistoricalAudienceVersion(ctx, id)
	if err != nil {
		return segment.AudienceActivityVersionReference{}, audienceActivityStoreError(err)
	}
	if row.ID != id || row.PackageHistoryID < 1 {
		return segment.AudienceActivityVersionReference{}, segment.ErrAudienceActivityHistoryConflict
	}
	return segment.AudienceActivityVersionReference{ID: row.ID, PackageHistoryID: row.PackageHistoryID}, nil
}
func (reader *AudienceActivityHistoryReader) getMember(ctx context.Context, id int64) (segment.AudienceActivityMemberReference, error) {
	if id < 1 {
		return segment.AudienceActivityMemberReference{}, segment.ErrAudienceActivityHistoryInvalid
	}
	q, err := reader.q(ctx)
	if err != nil {
		return segment.AudienceActivityMemberReference{}, err
	}
	row, err := q.GetHistoricalAudienceMember(ctx, id)
	if err != nil {
		return segment.AudienceActivityMemberReference{}, audienceActivityStoreError(err)
	}
	if row.ID != id || row.PackageHistoryID < 1 {
		return segment.AudienceActivityMemberReference{}, segment.ErrAudienceActivityHistoryConflict
	}
	return segment.AudienceActivityMemberReference{ID: row.ID, PackageHistoryID: row.PackageHistoryID}, nil
}

func (reader *AudienceActivityHistoryReader) ResolveAudienceActivityPackage(ctx context.Context, sourceID int64) (segment.AudienceActivityPackageReference, error) {
	if sourceID < 1 {
		return segment.AudienceActivityPackageReference{}, segment.ErrAudienceActivityHistoryInvalid
	}
	q, err := reader.q(ctx)
	if err != nil {
		return segment.AudienceActivityPackageReference{}, err
	}
	id, err := q.GetHistoricalAudienceActivityPackageBySourceID(ctx, sourceID)
	if err != nil {
		return segment.AudienceActivityPackageReference{}, audienceActivityStoreError(err)
	}
	return segment.AudienceActivityPackageReference{ID: id}, nil
}
func (reader *AudienceActivityHistoryReader) ResolveAudienceActivityVersion(ctx context.Context, sourceID int64) (segment.AudienceActivityVersionReference, error) {
	if sourceID < 1 {
		return segment.AudienceActivityVersionReference{}, segment.ErrAudienceActivityHistoryInvalid
	}
	q, err := reader.q(ctx)
	if err != nil {
		return segment.AudienceActivityVersionReference{}, err
	}
	row, err := q.GetHistoricalAudienceActivityVersionBySourceID(ctx, sourceID)
	if err != nil {
		return segment.AudienceActivityVersionReference{}, audienceActivityStoreError(err)
	}
	if row.ID < 1 || row.PackageHistoryID < 1 {
		return segment.AudienceActivityVersionReference{}, segment.ErrAudienceActivityHistoryConflict
	}
	return segment.AudienceActivityVersionReference{ID: row.ID, PackageHistoryID: row.PackageHistoryID}, nil
}
func (reader *AudienceActivityHistoryReader) ResolveAudienceActivityMember(ctx context.Context, sourceID int64) (segment.AudienceActivityMemberReference, error) {
	if sourceID < 1 {
		return segment.AudienceActivityMemberReference{}, segment.ErrAudienceActivityHistoryInvalid
	}
	q, err := reader.q(ctx)
	if err != nil {
		return segment.AudienceActivityMemberReference{}, err
	}
	row, err := q.GetHistoricalAudienceActivityMemberBySourceID(ctx, sourceID)
	if err != nil {
		return segment.AudienceActivityMemberReference{}, audienceActivityStoreError(err)
	}
	if row.ID < 1 || row.PackageHistoryID < 1 {
		return segment.AudienceActivityMemberReference{}, segment.ErrAudienceActivityHistoryConflict
	}
	return segment.AudienceActivityMemberReference{ID: row.ID, PackageHistoryID: row.PackageHistoryID}, nil
}
func (reader *AudienceActivityHistoryReader) ResolveAudienceActivityRun(ctx context.Context, sourceID int64) (segment.HistoricalAudienceActivityRun, error) {
	if sourceID < 1 {
		return segment.HistoricalAudienceActivityRun{}, segment.ErrAudienceActivityHistoryInvalid
	}
	q, err := reader.q(ctx)
	if err != nil {
		return segment.HistoricalAudienceActivityRun{}, err
	}
	row, err := q.GetHistoricalAudienceActivityRunBySourceID(ctx, sourceID)
	if err != nil {
		return segment.HistoricalAudienceActivityRun{}, audienceActivityStoreError(err)
	}
	return audienceActivityRunValue(row)
}

func (reader *AudienceActivityHistoryReader) ListAudienceActivityRuns(ctx context.Context, _ int64, limit, offset int32) ([]segment.AudienceActivityRunView, int64, error) {
	if !validAudienceActivityPage(limit, offset) {
		return nil, 0, segment.ErrAudienceActivityHistoryInvalid
	}
	q, err := reader.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalAudienceActivityRuns(ctx)
	if err != nil {
		return nil, 0, audienceActivityStoreError(err)
	}
	rows, err := q.ListHistoricalAudienceActivityRuns(ctx, segmentdb.ListHistoricalAudienceActivityRunsParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, audienceActivityStoreError(err)
	}
	items := make([]segment.AudienceActivityRunView, 0, len(rows))
	for _, row := range rows {
		value, err := audienceActivityRunValue(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, audienceActivityRunView(value))
	}
	return items, total, nil
}
func (reader *AudienceActivityHistoryReader) ListAudienceActivityMemberEvents(ctx context.Context, _ int64, limit, offset int32) ([]segment.AudienceActivityMemberEventView, int64, error) {
	if !validAudienceActivityPage(limit, offset) {
		return nil, 0, segment.ErrAudienceActivityHistoryInvalid
	}
	q, err := reader.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalAudienceActivityMemberEvents(ctx)
	if err != nil {
		return nil, 0, audienceActivityStoreError(err)
	}
	rows, err := q.ListHistoricalAudienceActivityMemberEvents(ctx, segmentdb.ListHistoricalAudienceActivityMemberEventsParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, audienceActivityStoreError(err)
	}
	items := make([]segment.AudienceActivityMemberEventView, 0, len(rows))
	for _, row := range rows {
		value, err := audienceActivityEventValue(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, audienceActivityEventView(value))
	}
	return items, total, nil
}

func audienceActivityRunParams(value segment.HistoricalAudienceActivityRun) segmentdb.CreateHistoricalAudienceActivityRunParams {
	return segmentdb.CreateHistoricalAudienceActivityRunParams{SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], SourceID: value.SourceID, PackageHistoryID: value.PackageHistoryID, VersionHistoryID: audienceActivityInt(value.VersionHistoryID), RunType: value.RunType, OriginalStatus: value.OriginalStatus, RefreshStartedAt: audienceActivityTimestamp(&value.RefreshStartedAt), RefreshFinishedAt: audienceActivityTimestamp(value.RefreshFinishedAt), LastWatermarkAt: audienceActivityTimestamp(value.LastWatermarkAt), NextWatermarkAt: audienceActivityTimestamp(value.NextWatermarkAt), ReturnedCount: value.ReturnedCount, EnteredCount: value.EnteredCount, UpdatedCount: value.UpdatedCount, ExitedCount: value.ExitedCount, MemberEventCount: value.MemberEventCount, DurationMs: value.DurationMS, CreatedAt: audienceActivityTimestamp(&value.CreatedAt), PrivateDigest: value.PrivateDigest[:]}
}
func audienceActivityEventParams(value segment.HistoricalAudienceActivityMemberEvent) segmentdb.CreateHistoricalAudienceActivityMemberEventParams {
	return segmentdb.CreateHistoricalAudienceActivityMemberEventParams{SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], SourceID: value.SourceID, PackageHistoryID: value.PackageHistoryID, RunHistoryID: audienceActivityInt(value.RunHistoryID), MemberHistoryID: audienceActivityInt(value.MemberHistoryID), EventType: value.EventType, IdentityKind: value.IdentityKind, OccurredAt: audienceActivityTimestamp(&value.OccurredAt), CreatedAt: audienceActivityTimestamp(&value.CreatedAt), PrivateDigest: value.PrivateDigest[:]}
}

func audienceActivityRunValue(row segmentdb.SegmentV1AudienceActivityRun) (segment.HistoricalAudienceActivityRun, error) {
	key, ok := audienceActivityDigest(row.SourceKeyDigest)
	payload, payloadOK := audienceActivityDigest(row.SourcePayloadDigest)
	field, fieldOK := audienceActivityDigest(row.SourceFieldDigest)
	private, privateOK := audienceActivityDigest(row.PrivateDigest)
	if !ok || !payloadOK || !fieldOK || !privateOK || !row.RefreshStartedAt.Valid || !row.CreatedAt.Valid {
		return segment.HistoricalAudienceActivityRun{}, segment.ErrAudienceActivityHistoryUnavailable
	}
	value := segment.HistoricalAudienceActivityRun{ID: row.ID, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, SourceID: row.SourceID, PackageHistoryID: row.PackageHistoryID, VersionHistoryID: audienceActivityIntPtr(row.VersionHistoryID), RunType: row.RunType, OriginalStatus: row.OriginalStatus, RefreshStartedAt: row.RefreshStartedAt.Time.UTC(), RefreshFinishedAt: audienceActivityTimePtr(row.RefreshFinishedAt), LastWatermarkAt: audienceActivityTimePtr(row.LastWatermarkAt), NextWatermarkAt: audienceActivityTimePtr(row.NextWatermarkAt), ReturnedCount: row.ReturnedCount, EnteredCount: row.EnteredCount, UpdatedCount: row.UpdatedCount, ExitedCount: row.ExitedCount, MemberEventCount: row.MemberEventCount, DurationMS: row.DurationMs, CreatedAt: row.CreatedAt.Time.UTC(), PrivateDigest: private}
	if !validAudienceActivityRun(value) {
		return segment.HistoricalAudienceActivityRun{}, segment.ErrAudienceActivityHistoryUnavailable
	}
	return value, nil
}
func audienceActivityEventValue(row segmentdb.SegmentV1AudienceActivityMemberEvent) (segment.HistoricalAudienceActivityMemberEvent, error) {
	key, ok := audienceActivityDigest(row.SourceKeyDigest)
	payload, payloadOK := audienceActivityDigest(row.SourcePayloadDigest)
	field, fieldOK := audienceActivityDigest(row.SourceFieldDigest)
	private, privateOK := audienceActivityDigest(row.PrivateDigest)
	if !ok || !payloadOK || !fieldOK || !privateOK || !row.OccurredAt.Valid || !row.CreatedAt.Valid {
		return segment.HistoricalAudienceActivityMemberEvent{}, segment.ErrAudienceActivityHistoryUnavailable
	}
	value := segment.HistoricalAudienceActivityMemberEvent{ID: row.ID, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, SourceID: row.SourceID, PackageHistoryID: row.PackageHistoryID, RunHistoryID: audienceActivityIntPtr(row.RunHistoryID), MemberHistoryID: audienceActivityIntPtr(row.MemberHistoryID), EventType: row.EventType, IdentityKind: row.IdentityKind, OccurredAt: row.OccurredAt.Time.UTC(), CreatedAt: row.CreatedAt.Time.UTC(), PrivateDigest: private}
	if !validAudienceActivityEvent(value) {
		return segment.HistoricalAudienceActivityMemberEvent{}, segment.ErrAudienceActivityHistoryUnavailable
	}
	return value, nil
}
func audienceActivityRunView(v segment.HistoricalAudienceActivityRun) segment.AudienceActivityRunView {
	return segment.AudienceActivityRunView{ID: v.ID, PackageHistoryID: v.PackageHistoryID, VersionHistoryID: v.VersionHistoryID, RunType: v.RunType, OriginalStatus: v.OriginalStatus, RefreshStartedAt: v.RefreshStartedAt, RefreshFinishedAt: v.RefreshFinishedAt, LastWatermarkAt: v.LastWatermarkAt, NextWatermarkAt: v.NextWatermarkAt, ReturnedCount: v.ReturnedCount, EnteredCount: v.EnteredCount, UpdatedCount: v.UpdatedCount, ExitedCount: v.ExitedCount, MemberEventCount: v.MemberEventCount, DurationMS: v.DurationMS, CreatedAt: v.CreatedAt}
}
func audienceActivityEventView(v segment.HistoricalAudienceActivityMemberEvent) segment.AudienceActivityMemberEventView {
	return segment.AudienceActivityMemberEventView{ID: v.ID, PackageHistoryID: v.PackageHistoryID, RunHistoryID: v.RunHistoryID, MemberHistoryID: v.MemberHistoryID, EventType: v.EventType, OccurredAt: v.OccurredAt, CreatedAt: v.CreatedAt}
}
func audienceActivityDigest(value []byte) ([32]byte, bool) {
	var out [32]byte
	if len(value) != 32 {
		return out, false
	}
	copy(out[:], value)
	return out, true
}
func audienceActivityInt(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
func audienceActivityIntPtr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	value := v.Int64
	return &value
}
func audienceActivityTimestamp(v *time.Time) pgtype.Timestamptz {
	if v == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: v.UTC().Truncate(time.Microsecond), Valid: true}
}
func audienceActivityTimePtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	value := v.Time.UTC().Truncate(time.Microsecond)
	return &value
}
func validAudienceActivityPage(limit, offset int32) bool {
	return limit >= 1 && limit <= 100 && offset >= 0
}
func validAudienceActivityRun(v segment.HistoricalAudienceActivityRun) bool {
	return v.ID >= 0 && v.SourceKeyDigest != ([32]byte{}) && v.SourcePayloadDigest != ([32]byte{}) && v.SourceFieldDigest != ([32]byte{}) && v.PrivateDigest != ([32]byte{}) && v.SourceID > 0 && v.PackageHistoryID > 0 && (v.VersionHistoryID == nil || *v.VersionHistoryID > 0) && audienceActivityText(v.RunType) && audienceActivityText(v.OriginalStatus) && !v.RefreshStartedAt.IsZero() && !v.CreatedAt.IsZero()
}
func validAudienceActivityEvent(v segment.HistoricalAudienceActivityMemberEvent) bool {
	return v.ID >= 0 && v.SourceKeyDigest != ([32]byte{}) && v.SourcePayloadDigest != ([32]byte{}) && v.SourceFieldDigest != ([32]byte{}) && v.PrivateDigest != ([32]byte{}) && v.SourceID > 0 && v.PackageHistoryID > 0 && (v.RunHistoryID == nil || *v.RunHistoryID > 0) && (v.MemberHistoryID == nil || *v.MemberHistoryID > 0) && audienceActivityText(v.EventType) && audienceActivityText(v.IdentityKind) && !v.OccurredAt.IsZero() && !v.CreatedAt.IsZero()
}
func audienceActivityText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}
func audienceActivityStoreError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return segment.ErrAudienceActivityHistoryConflict
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "23503" || pgErr.Code == "23514") {
		return segment.ErrAudienceActivityHistoryConflict
	}
	return segment.ErrAudienceActivityHistoryUnavailable
}
