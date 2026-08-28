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

type DeferredIdentityHistoryStore struct{}
type DeferredIdentityHistoryReader struct{ db contactdb.DBTX }

var _ contact.DeferredIdentityHistoryStore = (*DeferredIdentityHistoryStore)(nil)
var _ contact.DeferredIdentityHistoryReader = (*DeferredIdentityHistoryReader)(nil)

func NewDeferredIdentityHistoryStore() *DeferredIdentityHistoryStore {
	return &DeferredIdentityHistoryStore{}
}
func NewDeferredIdentityHistoryReader(db contactdb.DBTX) *DeferredIdentityHistoryReader {
	return &DeferredIdentityHistoryReader{db: db}
}

func (s *DeferredIdentityHistoryStore) q(ctx context.Context) (*contactdb.Queries, error) {
	if s == nil || ctx == nil || ctx.Err() != nil {
		return nil, contact.ErrDeferredIdentityHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, contact.ErrDeferredIdentityHistoryUnavailable
	}
	return contactdb.New(tx), nil
}

func (r *DeferredIdentityHistoryReader) q(ctx context.Context) (*contactdb.Queries, error) {
	if r == nil || ctx == nil || ctx.Err() != nil {
		return nil, contact.ErrDeferredIdentityHistoryUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return contactdb.New(tx), nil
	}
	if nilDeferredIdentityHistoryDB(r.db) {
		return nil, contact.ErrDeferredIdentityHistoryUnavailable
	}
	return contactdb.New(r.db), nil
}

func nilDeferredIdentityHistoryDB(value contactdb.DBTX) bool {
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

func (s *DeferredIdentityHistoryStore) CreateHistoricalDeferredPerson(ctx context.Context, value contact.HistoricalDeferredPerson) (contact.HistoricalDeferredPerson, error) {
	if value.ID != 0 || invalidDeferredPerson(value) {
		return contact.HistoricalDeferredPerson{}, contact.ErrDeferredIdentityHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return contact.HistoricalDeferredPerson{}, err
	}
	value.RedactedRoots = deferredRoots(value.RedactedRoots)
	row, err := q.CreateHistoricalDeferredPerson(ctx, contactdb.CreateHistoricalDeferredPersonParams{
		SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], SourceID: value.SourceID,
		MobileDigest: value.MobileDigest[:], ThirdPartyUserIDDigest: value.ThirdPartyUserIDDigest[:], PrivateDigest: value.PrivateDigest[:], RedactedRoots: value.RedactedRoots,
		CreatedAt: deferredTimestamp(value.CreatedAt), UpdatedAt: deferredTimestamp(value.UpdatedAt),
	})
	if err != nil {
		return contact.HistoricalDeferredPerson{}, deferredIdentityHistoryDBError(err)
	}
	return deferredPersonValue(row)
}

func (s *DeferredIdentityHistoryStore) GetHistoricalDeferredPerson(ctx context.Context, id int64) (contact.HistoricalDeferredPerson, error) {
	return deferredPersonGet(ctx, s.q, id)
}
func (r *DeferredIdentityHistoryReader) GetHistoricalDeferredPerson(ctx context.Context, id int64) (contact.HistoricalDeferredPerson, error) {
	return deferredPersonGet(ctx, r.q, id)
}
func deferredPersonGet(ctx context.Context, query func(context.Context) (*contactdb.Queries, error), id int64) (contact.HistoricalDeferredPerson, error) {
	if id < 1 {
		return contact.HistoricalDeferredPerson{}, contact.ErrDeferredIdentityHistoryInvalid
	}
	q, err := query(ctx)
	if err != nil {
		return contact.HistoricalDeferredPerson{}, err
	}
	row, err := q.GetHistoricalDeferredPerson(ctx, id)
	if err != nil {
		return contact.HistoricalDeferredPerson{}, deferredIdentityHistoryDBError(err)
	}
	return deferredPersonValue(row)
}
func (r *DeferredIdentityHistoryReader) ListHistoricalDeferredPerson(ctx context.Context, page contact.DeferredIdentityHistoryQuery) ([]contact.HistoricalDeferredPerson, int64, error) {
	if invalidDeferredIdentityHistoryPage(page) {
		return nil, 0, contact.ErrDeferredIdentityHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalDeferredPerson(ctx)
	if err != nil {
		return nil, 0, deferredIdentityHistoryDBError(err)
	}
	rows, err := q.ListHistoricalDeferredPerson(ctx, contactdb.ListHistoricalDeferredPersonParams{RowLimit: page.Limit, RowOffset: page.Offset})
	if err != nil {
		return nil, 0, deferredIdentityHistoryDBError(err)
	}
	values := make([]contact.HistoricalDeferredPerson, 0, len(rows))
	for _, row := range rows {
		value, err := deferredPersonValue(row)
		if err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	return values, total, nil
}

func (s *DeferredIdentityHistoryStore) CreateHistoricalDeferredIdentityConflict(ctx context.Context, value contact.HistoricalDeferredIdentityConflict) (contact.HistoricalDeferredIdentityConflict, error) {
	if value.ID != 0 || invalidDeferredConflict(value) {
		return contact.HistoricalDeferredIdentityConflict{}, contact.ErrDeferredIdentityHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return contact.HistoricalDeferredIdentityConflict{}, err
	}
	value.RedactedRoots = deferredRoots(value.RedactedRoots)
	row, err := q.CreateHistoricalDeferredIdentityConflict(ctx, contactdb.CreateHistoricalDeferredIdentityConflictParams{
		SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], SourceID: value.SourceID,
		ConflictType: value.ConflictType, SourceType: value.SourceType, Status: value.Status, ResolutionStatus: value.ResolutionStatus,
		UnionIDDigest: value.UnionIDDigest[:], CandidateUnionIDDigest: value.CandidateUnionIDDigest[:], ExternalUserIDDigest: value.ExternalUserIDDigest[:], OpenIDDigest: value.OpenIDDigest[:], MobileDigest: value.MobileDigest[:], LegacySourceKeyDigest: value.LegacySourceKeyDigest[:], PayloadJsonDigest: value.PayloadJSONDigest[:], SourcePayloadJsonDigest: value.SourcePayloadJSONDigest[:], ResolutionNoteDigest: value.ResolutionNoteDigest[:], PrivateDigest: value.PrivateDigest[:], RedactedRoots: value.RedactedRoots,
		CreatedAt: deferredTimestamp(value.CreatedAt), UpdatedAt: deferredTimestamp(value.UpdatedAt), ResolvedAt: deferredOptionalTimestamp(value.ResolvedAt),
	})
	if err != nil {
		return contact.HistoricalDeferredIdentityConflict{}, deferredIdentityHistoryDBError(err)
	}
	return deferredConflictValue(row)
}

func (s *DeferredIdentityHistoryStore) GetHistoricalDeferredIdentityConflict(ctx context.Context, id int64) (contact.HistoricalDeferredIdentityConflict, error) {
	return deferredConflictGet(ctx, s.q, id)
}
func (r *DeferredIdentityHistoryReader) GetHistoricalDeferredIdentityConflict(ctx context.Context, id int64) (contact.HistoricalDeferredIdentityConflict, error) {
	return deferredConflictGet(ctx, r.q, id)
}
func deferredConflictGet(ctx context.Context, query func(context.Context) (*contactdb.Queries, error), id int64) (contact.HistoricalDeferredIdentityConflict, error) {
	if id < 1 {
		return contact.HistoricalDeferredIdentityConflict{}, contact.ErrDeferredIdentityHistoryInvalid
	}
	q, err := query(ctx)
	if err != nil {
		return contact.HistoricalDeferredIdentityConflict{}, err
	}
	row, err := q.GetHistoricalDeferredIdentityConflict(ctx, id)
	if err != nil {
		return contact.HistoricalDeferredIdentityConflict{}, deferredIdentityHistoryDBError(err)
	}
	return deferredConflictValue(row)
}
func (r *DeferredIdentityHistoryReader) ListHistoricalDeferredIdentityConflict(ctx context.Context, page contact.DeferredIdentityHistoryQuery) ([]contact.HistoricalDeferredIdentityConflict, int64, error) {
	if invalidDeferredIdentityHistoryPage(page) {
		return nil, 0, contact.ErrDeferredIdentityHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalDeferredIdentityConflict(ctx)
	if err != nil {
		return nil, 0, deferredIdentityHistoryDBError(err)
	}
	rows, err := q.ListHistoricalDeferredIdentityConflict(ctx, contactdb.ListHistoricalDeferredIdentityConflictParams{RowLimit: page.Limit, RowOffset: page.Offset})
	if err != nil {
		return nil, 0, deferredIdentityHistoryDBError(err)
	}
	values := make([]contact.HistoricalDeferredIdentityConflict, 0, len(rows))
	for _, row := range rows {
		value, err := deferredConflictValue(row)
		if err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	return values, total, nil
}

func (s *DeferredIdentityHistoryStore) CreateHistoricalMissingRootIdentity(ctx context.Context, value contact.HistoricalMissingRootIdentity) (contact.HistoricalMissingRootIdentity, error) {
	if value.ID != 0 || invalidMissingRootIdentity(value) {
		return contact.HistoricalMissingRootIdentity{}, contact.ErrDeferredIdentityHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return contact.HistoricalMissingRootIdentity{}, err
	}
	value.RedactedRoots = deferredRoots(value.RedactedRoots)
	row, err := q.CreateHistoricalMissingRootIdentity(ctx, contactdb.CreateHistoricalMissingRootIdentityParams{
		SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], SourceID: value.SourceID,
		Dm01RunID: value.DM01RunID, Dm01SourceKeyDigest: value.DM01SourceKeyDigest[:], Dm01SourceHmacKeyVersion: value.DM01SourceHMACKeyVersion, QuarantineReason: value.QuarantineReason, Type: deferredOptionalInt4(value.Type), Status: value.Status,
		CorpIDDigest: value.CorpIDDigest[:], ExternalUserIDDigest: value.ExternalUserIDDigest[:], UnionIDDigest: value.UnionIDDigest[:], OpenIDDigest: value.OpenIDDigest[:], FollowUserIDDigest: value.FollowUserIDDigest[:], NameDigest: value.NameDigest[:], AvatarDigest: value.AvatarDigest[:], GenderDigest: deferredOptionalDigest(value.GenderDigest), RawProfileDigest: value.RawProfileDigest[:], PrivateDigest: value.PrivateDigest[:], RedactedRoots: value.RedactedRoots,
		FirstSeenAt: deferredTimestamp(value.FirstSeenAt), LastSeenAt: deferredTimestamp(value.LastSeenAt), CreatedAt: deferredTimestamp(value.CreatedAt), UpdatedAt: deferredTimestamp(value.UpdatedAt),
	})
	if err != nil {
		return contact.HistoricalMissingRootIdentity{}, deferredIdentityHistoryDBError(err)
	}
	return missingRootIdentityValue(row)
}

func (s *DeferredIdentityHistoryStore) GetHistoricalMissingRootIdentity(ctx context.Context, id int64) (contact.HistoricalMissingRootIdentity, error) {
	return missingRootIdentityGet(ctx, s.q, id)
}
func (r *DeferredIdentityHistoryReader) GetHistoricalMissingRootIdentity(ctx context.Context, id int64) (contact.HistoricalMissingRootIdentity, error) {
	return missingRootIdentityGet(ctx, r.q, id)
}
func missingRootIdentityGet(ctx context.Context, query func(context.Context) (*contactdb.Queries, error), id int64) (contact.HistoricalMissingRootIdentity, error) {
	if id < 1 {
		return contact.HistoricalMissingRootIdentity{}, contact.ErrDeferredIdentityHistoryInvalid
	}
	q, err := query(ctx)
	if err != nil {
		return contact.HistoricalMissingRootIdentity{}, err
	}
	row, err := q.GetHistoricalMissingRootIdentity(ctx, id)
	if err != nil {
		return contact.HistoricalMissingRootIdentity{}, deferredIdentityHistoryDBError(err)
	}
	return missingRootIdentityValue(row)
}
func (r *DeferredIdentityHistoryReader) ListHistoricalMissingRootIdentity(ctx context.Context, page contact.DeferredIdentityHistoryQuery) ([]contact.HistoricalMissingRootIdentity, int64, error) {
	if invalidDeferredIdentityHistoryPage(page) {
		return nil, 0, contact.ErrDeferredIdentityHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalMissingRootIdentity(ctx)
	if err != nil {
		return nil, 0, deferredIdentityHistoryDBError(err)
	}
	rows, err := q.ListHistoricalMissingRootIdentity(ctx, contactdb.ListHistoricalMissingRootIdentityParams{RowLimit: page.Limit, RowOffset: page.Offset})
	if err != nil {
		return nil, 0, deferredIdentityHistoryDBError(err)
	}
	values := make([]contact.HistoricalMissingRootIdentity, 0, len(rows))
	for _, row := range rows {
		value, err := missingRootIdentityValue(row)
		if err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	return values, total, nil
}

func deferredPersonValue(row contactdb.ContactV1DeferredPersonHistory) (contact.HistoricalDeferredPerson, error) {
	key, ok1 := deferredDigest(row.SourceKeyDigest)
	payload, ok2 := deferredDigest(row.SourcePayloadDigest)
	field, ok3 := deferredDigest(row.SourceFieldDigest)
	mobile, ok4 := deferredDigest(row.MobileDigest)
	thirdParty, ok5 := deferredDigest(row.ThirdPartyUserIDDigest)
	private, ok6 := deferredDigest(row.PrivateDigest)
	created, ok7 := deferredTime(row.CreatedAt)
	updated, ok8 := deferredTime(row.UpdatedAt)
	value := contact.HistoricalDeferredPerson{ID: row.ID, SourceID: row.SourceID, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, MobileDigest: mobile, ThirdPartyUserIDDigest: thirdParty, PrivateDigest: private, RedactedRoots: deferredRoots(row.RedactedRoots), CreatedAt: created, UpdatedAt: updated}
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 || !ok8 || invalidDeferredPersonStored(value) {
		return contact.HistoricalDeferredPerson{}, contact.ErrDeferredIdentityHistoryUnavailable
	}
	return value, nil
}

func deferredConflictValue(row contactdb.ContactV1DeferredIdentityConflictHistory) (contact.HistoricalDeferredIdentityConflict, error) {
	digests := [][]byte{row.SourceKeyDigest, row.SourcePayloadDigest, row.SourceFieldDigest, row.UnionIDDigest, row.CandidateUnionIDDigest, row.ExternalUserIDDigest, row.OpenIDDigest, row.MobileDigest, row.LegacySourceKeyDigest, row.PayloadJsonDigest, row.SourcePayloadJsonDigest, row.ResolutionNoteDigest, row.PrivateDigest}
	values := make([][32]byte, len(digests))
	for i := range digests {
		var ok bool
		if values[i], ok = deferredDigest(digests[i]); !ok {
			return contact.HistoricalDeferredIdentityConflict{}, contact.ErrDeferredIdentityHistoryUnavailable
		}
	}
	created, createdOK := deferredTime(row.CreatedAt)
	updated, updatedOK := deferredTime(row.UpdatedAt)
	resolved, resolvedOK := deferredOptionalTime(row.ResolvedAt)
	value := contact.HistoricalDeferredIdentityConflict{ID: row.ID, SourceID: row.SourceID, SourceKeyDigest: values[0], SourcePayloadDigest: values[1], SourceFieldDigest: values[2], ConflictType: row.ConflictType, SourceType: row.SourceType, Status: row.Status, ResolutionStatus: row.ResolutionStatus, UnionIDDigest: values[3], CandidateUnionIDDigest: values[4], ExternalUserIDDigest: values[5], OpenIDDigest: values[6], MobileDigest: values[7], LegacySourceKeyDigest: values[8], PayloadJSONDigest: values[9], SourcePayloadJSONDigest: values[10], ResolutionNoteDigest: values[11], PrivateDigest: values[12], RedactedRoots: deferredRoots(row.RedactedRoots), CreatedAt: created, UpdatedAt: updated, ResolvedAt: resolved}
	if !createdOK || !updatedOK || !resolvedOK || invalidDeferredConflictStored(value) {
		return contact.HistoricalDeferredIdentityConflict{}, contact.ErrDeferredIdentityHistoryUnavailable
	}
	return value, nil
}

func missingRootIdentityValue(row contactdb.ContactV1MissingRootIdentityHistory) (contact.HistoricalMissingRootIdentity, error) {
	digests := [][]byte{row.SourceKeyDigest, row.SourcePayloadDigest, row.SourceFieldDigest, row.Dm01SourceKeyDigest, row.CorpIDDigest, row.ExternalUserIDDigest, row.UnionIDDigest, row.OpenIDDigest, row.FollowUserIDDigest, row.NameDigest, row.AvatarDigest, row.RawProfileDigest, row.PrivateDigest}
	values := make([][32]byte, len(digests))
	for i := range digests {
		var ok bool
		if values[i], ok = deferredDigest(digests[i]); !ok {
			return contact.HistoricalMissingRootIdentity{}, contact.ErrDeferredIdentityHistoryUnavailable
		}
	}
	typeValue, typeOK := deferredOptionalInt(row.Type)
	gender, genderOK := deferredOptionalDigestValue(row.GenderDigest)
	first, firstOK := deferredTime(row.FirstSeenAt)
	last, lastOK := deferredTime(row.LastSeenAt)
	created, createdOK := deferredTime(row.CreatedAt)
	updated, updatedOK := deferredTime(row.UpdatedAt)
	value := contact.HistoricalMissingRootIdentity{ID: row.ID, SourceID: row.SourceID, SourceKeyDigest: values[0], SourcePayloadDigest: values[1], SourceFieldDigest: values[2], DM01RunID: row.Dm01RunID, DM01SourceKeyDigest: values[3], DM01SourceHMACKeyVersion: row.Dm01SourceHmacKeyVersion, QuarantineReason: row.QuarantineReason, Type: typeValue, Status: row.Status, CorpIDDigest: values[4], ExternalUserIDDigest: values[5], UnionIDDigest: values[6], OpenIDDigest: values[7], FollowUserIDDigest: values[8], NameDigest: values[9], AvatarDigest: values[10], GenderDigest: gender, RawProfileDigest: values[11], PrivateDigest: values[12], RedactedRoots: deferredRoots(row.RedactedRoots), FirstSeenAt: first, LastSeenAt: last, CreatedAt: created, UpdatedAt: updated}
	if !typeOK || !genderOK || !firstOK || !lastOK || !createdOK || !updatedOK || invalidMissingRootIdentityStored(value) {
		return contact.HistoricalMissingRootIdentity{}, contact.ErrDeferredIdentityHistoryUnavailable
	}
	return value, nil
}

func invalidDeferredPerson(value contact.HistoricalDeferredPerson) bool {
	value.ID = 1
	_, err := contactapp.HistoricalDeferredPersonDigest(value)
	return err != nil
}
func invalidDeferredPersonStored(value contact.HistoricalDeferredPerson) bool {
	_, err := contactapp.HistoricalDeferredPersonDigest(value)
	return err != nil
}
func invalidDeferredConflict(value contact.HistoricalDeferredIdentityConflict) bool {
	value.ID = 1
	_, err := contactapp.HistoricalDeferredIdentityConflictDigest(value)
	return err != nil
}
func invalidDeferredConflictStored(value contact.HistoricalDeferredIdentityConflict) bool {
	_, err := contactapp.HistoricalDeferredIdentityConflictDigest(value)
	return err != nil
}
func invalidMissingRootIdentity(value contact.HistoricalMissingRootIdentity) bool {
	value.ID = 1
	_, err := contactapp.HistoricalMissingRootIdentityDigest(value)
	return err != nil
}
func invalidMissingRootIdentityStored(value contact.HistoricalMissingRootIdentity) bool {
	_, err := contactapp.HistoricalMissingRootIdentityDigest(value)
	return err != nil
}
func invalidDeferredIdentityHistoryPage(value contact.DeferredIdentityHistoryQuery) bool {
	return value.Limit < 1 || value.Limit > 100 || value.Offset < 0
}

func deferredDigest(value []byte) ([32]byte, bool) {
	var result [32]byte
	if len(value) != len(result) {
		return result, false
	}
	copy(result[:], value)
	return result, result != ([32]byte{})
}
func deferredOptionalDigestValue(value []byte) (*[32]byte, bool) {
	if value == nil {
		return nil, true
	}
	digest, ok := deferredDigest(value)
	if !ok {
		return nil, false
	}
	return &digest, true
}
func deferredOptionalDigest(value *[32]byte) []byte {
	if value == nil {
		return nil
	}
	return value[:]
}
func deferredTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func deferredOptionalTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return deferredTimestamp(*value)
}
func deferredTime(value pgtype.Timestamptz) (time.Time, bool) {
	if !value.Valid || value.InfinityModifier != pgtype.Finite {
		return time.Time{}, false
	}
	return value.Time.UTC().Truncate(time.Microsecond), true
}
func deferredOptionalTime(value pgtype.Timestamptz) (*time.Time, bool) {
	if !value.Valid {
		return nil, true
	}
	parsed, ok := deferredTime(value)
	if !ok {
		return nil, false
	}
	return &parsed, true
}
func deferredOptionalInt(value pgtype.Int4) (*int32, bool) {
	if !value.Valid {
		return nil, true
	}
	result := value.Int32
	return &result, true
}
func deferredOptionalInt4(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}
func deferredRoots(value []string) []string {
	if value == nil {
		return []string{}
	}
	return append([]string{}, value...)
}
func deferredIdentityHistoryDBError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return contact.ErrDeferredIdentityHistoryConflict
	}
	return contact.ErrDeferredIdentityHistoryUnavailable
}
