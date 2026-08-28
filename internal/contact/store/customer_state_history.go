package store

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type CustomerStateHistoryStore struct{}
type CustomerStateHistoryReader struct{ db contactdb.DBTX }

var _ contact.CustomerStateHistoryStore = (*CustomerStateHistoryStore)(nil)
var _ contact.CustomerStateHistoryReader = (*CustomerStateHistoryReader)(nil)

func NewCustomerStateHistoryStore() *CustomerStateHistoryStore { return &CustomerStateHistoryStore{} }
func NewCustomerStateHistoryReader(db contactdb.DBTX) *CustomerStateHistoryReader {
	return &CustomerStateHistoryReader{db: db}
}
func (s *CustomerStateHistoryStore) q(ctx context.Context) (*contactdb.Queries, error) {
	if s == nil || ctx == nil || ctx.Err() != nil {
		return nil, contact.ErrCustomerStateHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, contact.ErrCustomerStateHistoryUnavailable
	}
	return contactdb.New(tx), nil
}
func (r *CustomerStateHistoryReader) q(ctx context.Context) (*contactdb.Queries, error) {
	if r == nil || ctx == nil || ctx.Err() != nil {
		return nil, contact.ErrCustomerStateHistoryUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return contactdb.New(tx), nil
	}
	if nilCustomerStateDB(r.db) {
		return nil, contact.ErrCustomerStateHistoryUnavailable
	}
	return contactdb.New(r.db), nil
}
func nilCustomerStateDB(value contactdb.DBTX) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

func (s *CustomerStateHistoryStore) CreateHistoricalCustomerStatusSnapshot(ctx context.Context, v contact.HistoricalCustomerStatusSnapshot) (contact.HistoricalCustomerStatusSnapshot, error) {
	if v.ID != 0 || badSnapshot(v) {
		return contact.HistoricalCustomerStatusSnapshot{}, contact.ErrCustomerStateHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return contact.HistoricalCustomerStatusSnapshot{}, err
	}
	row, err := q.CreateHistoricalCustomerStatusSnapshot(ctx, contactdb.CreateHistoricalCustomerStatusSnapshotParams{SourceKeyDigest: v.SourceKeyDigest[:], SourcePayloadDigest: v.SourcePayloadDigest[:], SourceFieldDigest: v.SourceFieldDigest[:], SignupStatus: v.SignupStatus, SignupLabelName: v.SignupLabelName, CustomerNameSnapshot: v.CustomerNameSnapshot, OwnerUseridSnapshot: v.OwnerUserIDSnapshot, SetByUseridDigest: v.SetByUserIDDigest[:], SetAt: cts(v.SetAt), WecomTagSyncStatus: v.WeComTagSyncStatus, WecomTagSyncErrorHash: v.WeComTagSyncErrorHash[:], StatusFlagsDigest: v.StatusFlagsDigest[:], CreatedAt: cts(v.CreatedAt), UpdatedAt: cts(v.UpdatedAt), Unionid: v.UnionID})
	if err != nil {
		return contact.HistoricalCustomerStatusSnapshot{}, customerStateDBError(err)
	}
	return snapshotValue(row)
}
func (s *CustomerStateHistoryStore) GetHistoricalCustomerStatusSnapshot(ctx context.Context, id int64) (contact.HistoricalCustomerStatusSnapshot, error) {
	if id < 1 {
		return contact.HistoricalCustomerStatusSnapshot{}, contact.ErrCustomerStateHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return contact.HistoricalCustomerStatusSnapshot{}, err
	}
	row, err := q.GetHistoricalCustomerStatusSnapshot(ctx, id)
	if err != nil {
		return contact.HistoricalCustomerStatusSnapshot{}, customerStateDBError(err)
	}
	return snapshotValue(row)
}
func (r *CustomerStateHistoryReader) GetHistoricalCustomerStatusSnapshot(ctx context.Context, id int64) (contact.HistoricalCustomerStatusSnapshot, error) {
	if id < 1 {
		return contact.HistoricalCustomerStatusSnapshot{}, contact.ErrCustomerStateHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return contact.HistoricalCustomerStatusSnapshot{}, err
	}
	row, err := q.GetHistoricalCustomerStatusSnapshot(ctx, id)
	if err != nil {
		return contact.HistoricalCustomerStatusSnapshot{}, customerStateDBError(err)
	}
	return snapshotValue(row)
}
func (r *CustomerStateHistoryReader) ListHistoricalCustomerStatusSnapshot(ctx context.Context, x contact.CustomerStateHistoryQuery) ([]contact.HistoricalCustomerStatusSnapshot, int64, error) {
	if badCustomerStateQuery(x) {
		return nil, 0, contact.ErrCustomerStateHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	n, err := q.CountHistoricalCustomerStatusSnapshot(ctx)
	if err != nil {
		return nil, 0, customerStateDBError(err)
	}
	rows, err := q.ListHistoricalCustomerStatusSnapshot(ctx, contactdb.ListHistoricalCustomerStatusSnapshotParams{RowLimit: x.Limit, RowOffset: x.Offset})
	if err != nil {
		return nil, 0, customerStateDBError(err)
	}
	out := make([]contact.HistoricalCustomerStatusSnapshot, 0, len(rows))
	for _, row := range rows {
		v, e := snapshotValue(row)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, v)
	}
	return out, n, nil
}

func (s *CustomerStateHistoryStore) CreateHistoricalCustomerStatusChange(ctx context.Context, v contact.HistoricalCustomerStatusChange) (contact.HistoricalCustomerStatusChange, error) {
	if v.ID != 0 || badChange(v) {
		return contact.HistoricalCustomerStatusChange{}, contact.ErrCustomerStateHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return contact.HistoricalCustomerStatusChange{}, err
	}
	row, err := q.CreateHistoricalCustomerStatusChange(ctx, contactdb.CreateHistoricalCustomerStatusChangeParams{SourceKeyDigest: v.SourceKeyDigest[:], SourcePayloadDigest: v.SourcePayloadDigest[:], SourceFieldDigest: v.SourceFieldDigest[:], SourceID: v.SourceID, OldSignupStatus: v.OldSignupStatus, NewSignupStatus: v.NewSignupStatus, OldLabelName: v.OldLabelName, NewLabelName: v.NewLabelName, CustomerNameSnapshot: v.CustomerNameSnapshot, OwnerUseridSnapshot: v.OwnerUserIDSnapshot, SetByUseridDigest: v.SetByUserIDDigest[:], SetAt: cts(v.SetAt), WecomTagSyncStatus: v.WeComTagSyncStatus, WecomTagSyncErrorHash: v.WeComTagSyncErrorHash[:], StatusFlagsDigest: v.StatusFlagsDigest[:], CreatedAt: cts(v.CreatedAt), Unionid: v.UnionID})
	if err != nil {
		return contact.HistoricalCustomerStatusChange{}, customerStateDBError(err)
	}
	return changeValue(row)
}
func (s *CustomerStateHistoryStore) GetHistoricalCustomerStatusChange(ctx context.Context, id int64) (contact.HistoricalCustomerStatusChange, error) {
	if id < 1 {
		return contact.HistoricalCustomerStatusChange{}, contact.ErrCustomerStateHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return contact.HistoricalCustomerStatusChange{}, err
	}
	row, err := q.GetHistoricalCustomerStatusChange(ctx, id)
	if err != nil {
		return contact.HistoricalCustomerStatusChange{}, customerStateDBError(err)
	}
	return changeValue(row)
}
func (r *CustomerStateHistoryReader) GetHistoricalCustomerStatusChange(ctx context.Context, id int64) (contact.HistoricalCustomerStatusChange, error) {
	if id < 1 {
		return contact.HistoricalCustomerStatusChange{}, contact.ErrCustomerStateHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return contact.HistoricalCustomerStatusChange{}, err
	}
	row, err := q.GetHistoricalCustomerStatusChange(ctx, id)
	if err != nil {
		return contact.HistoricalCustomerStatusChange{}, customerStateDBError(err)
	}
	return changeValue(row)
}
func (r *CustomerStateHistoryReader) ListHistoricalCustomerStatusChange(ctx context.Context, x contact.CustomerStateHistoryQuery) ([]contact.HistoricalCustomerStatusChange, int64, error) {
	if badCustomerStateQuery(x) {
		return nil, 0, contact.ErrCustomerStateHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	n, err := q.CountHistoricalCustomerStatusChange(ctx)
	if err != nil {
		return nil, 0, customerStateDBError(err)
	}
	rows, err := q.ListHistoricalCustomerStatusChange(ctx, contactdb.ListHistoricalCustomerStatusChangeParams{RowLimit: x.Limit, RowOffset: x.Offset})
	if err != nil {
		return nil, 0, customerStateDBError(err)
	}
	out := make([]contact.HistoricalCustomerStatusChange, 0, len(rows))
	for _, row := range rows {
		v, e := changeValue(row)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, v)
	}
	return out, n, nil
}

func (s *CustomerStateHistoryStore) CreateHistoricalClassTermTagMapping(ctx context.Context, v contact.HistoricalClassTermTagMapping) (contact.HistoricalClassTermTagMapping, error) {
	if v.ID != 0 || badTerm(v) {
		return contact.HistoricalClassTermTagMapping{}, contact.ErrCustomerStateHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return contact.HistoricalClassTermTagMapping{}, err
	}
	row, err := q.CreateHistoricalClassTermTagMapping(ctx, contactdb.CreateHistoricalClassTermTagMappingParams{SourceKeyDigest: v.SourceKeyDigest[:], SourcePayloadDigest: v.SourcePayloadDigest[:], SourceFieldDigest: v.SourceFieldDigest[:], SourceID: v.SourceID, TagGroupName: v.TagGroupName, TagName: v.TagName, ClassTermNo: v.ClassTermNo, ClassTermLabel: v.ClassTermLabel, OriginalActive: v.OriginalActive, CreatedAt: cts(v.CreatedAt), UpdatedAt: cts(v.UpdatedAt), StrategySourceID: v.StrategySourceID, GroupSourceID: v.GroupSourceID, TagSourceID: v.TagSourceID})
	if err != nil {
		return contact.HistoricalClassTermTagMapping{}, customerStateDBError(err)
	}
	return termValue(row)
}
func (s *CustomerStateHistoryStore) GetHistoricalClassTermTagMapping(ctx context.Context, id int64) (contact.HistoricalClassTermTagMapping, error) {
	if id < 1 {
		return contact.HistoricalClassTermTagMapping{}, contact.ErrCustomerStateHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return contact.HistoricalClassTermTagMapping{}, err
	}
	row, err := q.GetHistoricalClassTermTagMapping(ctx, id)
	if err != nil {
		return contact.HistoricalClassTermTagMapping{}, customerStateDBError(err)
	}
	return termValue(row)
}
func (r *CustomerStateHistoryReader) GetHistoricalClassTermTagMapping(ctx context.Context, id int64) (contact.HistoricalClassTermTagMapping, error) {
	if id < 1 {
		return contact.HistoricalClassTermTagMapping{}, contact.ErrCustomerStateHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return contact.HistoricalClassTermTagMapping{}, err
	}
	row, err := q.GetHistoricalClassTermTagMapping(ctx, id)
	if err != nil {
		return contact.HistoricalClassTermTagMapping{}, customerStateDBError(err)
	}
	return termValue(row)
}
func (r *CustomerStateHistoryReader) ListHistoricalClassTermTagMapping(ctx context.Context, x contact.CustomerStateHistoryQuery) ([]contact.HistoricalClassTermTagMapping, int64, error) {
	if badCustomerStateQuery(x) {
		return nil, 0, contact.ErrCustomerStateHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	n, err := q.CountHistoricalClassTermTagMapping(ctx)
	if err != nil {
		return nil, 0, customerStateDBError(err)
	}
	rows, err := q.ListHistoricalClassTermTagMapping(ctx, contactdb.ListHistoricalClassTermTagMappingParams{RowLimit: x.Limit, RowOffset: x.Offset})
	if err != nil {
		return nil, 0, customerStateDBError(err)
	}
	out := make([]contact.HistoricalClassTermTagMapping, 0, len(rows))
	for _, row := range rows {
		v, e := termValue(row)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, v)
	}
	return out, n, nil
}

func cts(v time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: v, Valid: true} }
func ctv(v pgtype.Timestamptz) (time.Time, bool) {
	if !v.Valid || v.InfinityModifier != pgtype.Finite {
		return time.Time{}, false
	}
	return v.Time.UTC().Truncate(time.Microsecond), true
}
func cdigest(values ...[]byte) ([32]byte, [32]byte, [32]byte, [32]byte, [32]byte, [32]byte, bool) {
	var a, b, c, d, e, f [32]byte
	if len(values) != 6 {
		return a, b, c, d, e, f, false
	}
	for i, x := range values {
		if len(x) != 32 {
			return a, b, c, d, e, f, false
		}
		switch i {
		case 0:
			copy(a[:], x)
		case 1:
			copy(b[:], x)
		case 2:
			copy(c[:], x)
		case 3:
			copy(d[:], x)
		case 4:
			copy(e[:], x)
		case 5:
			copy(f[:], x)
		}
	}
	return a, b, c, d, e, f, a != ([32]byte{}) && b != ([32]byte{}) && c != ([32]byte{}) && d != ([32]byte{}) && e != ([32]byte{}) && f != ([32]byte{})
}
func cdigest3(values ...[]byte) ([32]byte, [32]byte, [32]byte, bool) {
	var a, b, c [32]byte
	if len(values) != 3 || len(values[0]) != 32 || len(values[1]) != 32 || len(values[2]) != 32 {
		return a, b, c, false
	}
	copy(a[:], values[0])
	copy(b[:], values[1])
	copy(c[:], values[2])
	return a, b, c, a != ([32]byte{}) && b != ([32]byte{}) && c != ([32]byte{})
}
func snapshotValue(r contactdb.ContactV1CustomerStatusSnapshot) (contact.HistoricalCustomerStatusSnapshot, error) {
	key, payload, field, setby, syncerr, flags, ok := cdigest(r.SourceKeyDigest, r.SourcePayloadDigest, r.SourceFieldDigest, r.SetByUseridDigest, r.WecomTagSyncErrorHash, r.StatusFlagsDigest)
	set, setOK := ctv(r.SetAt)
	created, createdOK := ctv(r.CreatedAt)
	updated, updatedOK := ctv(r.UpdatedAt)
	v := contact.HistoricalCustomerStatusSnapshot{ID: r.ID, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, SignupStatus: r.SignupStatus, SignupLabelName: r.SignupLabelName, CustomerNameSnapshot: r.CustomerNameSnapshot, OwnerUserIDSnapshot: r.OwnerUseridSnapshot, SetByUserIDDigest: setby, SetAt: set, WeComTagSyncStatus: r.WecomTagSyncStatus, WeComTagSyncErrorHash: syncerr, StatusFlagsDigest: flags, CreatedAt: created, UpdatedAt: updated, UnionID: r.Unionid}
	if !ok || !setOK || !createdOK || !updatedOK || badSnapshot(v) {
		return contact.HistoricalCustomerStatusSnapshot{}, contact.ErrCustomerStateHistoryUnavailable
	}
	return v, nil
}
func changeValue(r contactdb.ContactV1CustomerStatusChange) (contact.HistoricalCustomerStatusChange, error) {
	key, payload, field, setby, syncerr, flags, ok := cdigest(r.SourceKeyDigest, r.SourcePayloadDigest, r.SourceFieldDigest, r.SetByUseridDigest, r.WecomTagSyncErrorHash, r.StatusFlagsDigest)
	set, setOK := ctv(r.SetAt)
	created, createdOK := ctv(r.CreatedAt)
	v := contact.HistoricalCustomerStatusChange{ID: r.ID, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, SourceID: r.SourceID, OldSignupStatus: r.OldSignupStatus, NewSignupStatus: r.NewSignupStatus, OldLabelName: r.OldLabelName, NewLabelName: r.NewLabelName, CustomerNameSnapshot: r.CustomerNameSnapshot, OwnerUserIDSnapshot: r.OwnerUseridSnapshot, SetByUserIDDigest: setby, SetAt: set, WeComTagSyncStatus: r.WecomTagSyncStatus, WeComTagSyncErrorHash: syncerr, StatusFlagsDigest: flags, CreatedAt: created, UnionID: r.Unionid}
	if !ok || !setOK || !createdOK || badChange(v) {
		return contact.HistoricalCustomerStatusChange{}, contact.ErrCustomerStateHistoryUnavailable
	}
	return v, nil
}
func termValue(r contactdb.ContactV1ClassTermTagHistory) (contact.HistoricalClassTermTagMapping, error) {
	key, payload, field, ok := cdigest3(r.SourceKeyDigest, r.SourcePayloadDigest, r.SourceFieldDigest)
	created, createdOK := ctv(r.CreatedAt)
	updated, updatedOK := ctv(r.UpdatedAt)
	v := contact.HistoricalClassTermTagMapping{ID: r.ID, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, SourceID: r.SourceID, TagGroupName: r.TagGroupName, TagName: r.TagName, ClassTermNo: r.ClassTermNo, ClassTermLabel: r.ClassTermLabel, OriginalActive: r.OriginalActive, CreatedAt: created, UpdatedAt: updated, StrategySourceID: r.StrategySourceID, GroupSourceID: r.GroupSourceID, TagSourceID: r.TagSourceID}
	if !ok || !createdOK || !updatedOK || badTerm(v) {
		return contact.HistoricalClassTermTagMapping{}, contact.ErrCustomerStateHistoryUnavailable
	}
	return v, nil
}
func badSnapshot(v contact.HistoricalCustomerStatusSnapshot) bool {
	if v.ID == 0 {
		v.ID = 1
	}
	_, e := contactapp.HistoricalCustomerStatusSnapshotDigest(v)
	return e != nil
}
func badChange(v contact.HistoricalCustomerStatusChange) bool {
	if v.ID == 0 {
		v.ID = 1
	}
	_, e := contactapp.HistoricalCustomerStatusChangeDigest(v)
	return e != nil
}
func badTerm(v contact.HistoricalClassTermTagMapping) bool {
	if v.ID == 0 {
		v.ID = 1
	}
	_, e := contactapp.HistoricalClassTermTagMappingDigest(v)
	return e != nil
}
func badCustomerStateQuery(x contact.CustomerStateHistoryQuery) bool {
	return x.Limit < 1 || x.Limit > 100 || x.Offset < 0
}
func customerStateDBError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return contact.ErrCustomerStateHistoryConflict
	}
	return contact.ErrCustomerStateHistoryUnavailable
}
