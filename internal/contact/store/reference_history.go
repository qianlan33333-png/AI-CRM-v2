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

type ReferenceHistoryStore struct{}
type ReferenceHistoryReader struct{ db contactdb.DBTX }

var _ contact.ReferenceHistoryStore = (*ReferenceHistoryStore)(nil)
var _ contact.ReferenceHistoryReader = (*ReferenceHistoryReader)(nil)

func NewReferenceHistoryStore() *ReferenceHistoryStore { return &ReferenceHistoryStore{} }
func NewReferenceHistoryReader(db contactdb.DBTX) *ReferenceHistoryReader {
	return &ReferenceHistoryReader{db: db}
}

func (s *ReferenceHistoryStore) q(ctx context.Context) (*contactdb.Queries, error) {
	if s == nil || ctx == nil || ctx.Err() != nil {
		return nil, contact.ErrReferenceHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, contact.ErrReferenceHistoryUnavailable
	}
	return contactdb.New(tx), nil
}

func (r *ReferenceHistoryReader) q(ctx context.Context) (*contactdb.Queries, error) {
	if r == nil || ctx == nil || ctx.Err() != nil {
		return nil, contact.ErrReferenceHistoryUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return contactdb.New(tx), nil
	}
	if nilReferenceHistoryDB(r.db) {
		return nil, contact.ErrReferenceHistoryUnavailable
	}
	return contactdb.New(r.db), nil
}

func nilReferenceHistoryDB(value contactdb.DBTX) bool {
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

func (s *ReferenceHistoryStore) CreateHistoricalExternalContactBinding(ctx context.Context, value contact.HistoricalExternalContactBinding) (contact.HistoricalExternalContactBinding, error) {
	if value.ID != 0 || invalidExternalContactBinding(value) {
		return contact.HistoricalExternalContactBinding{}, contact.ErrReferenceHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return contact.HistoricalExternalContactBinding{}, err
	}
	row, err := q.CreateHistoricalExternalContactBinding(ctx, contactdb.CreateHistoricalExternalContactBindingParams{
		SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], ExternalUserIDDigest: value.ExternalUserIDDigest[:], SourcePersonID: value.SourcePersonID,
		PersonHistoryID: referenceOptionalInt8(value.PersonHistoryID), IdentityID: referenceOptionalInt8(value.IdentityID), IdentityAssurance: value.IdentityAssurance,
		FirstBoundByUserIDDigest: value.FirstBoundByUserIDDigest[:], FirstOwnerUserIDDigest: value.FirstOwnerUserIDDigest[:], LastOwnerUserIDDigest: value.LastOwnerUserIDDigest[:],
		CreatedAt: referenceTimestamp(value.CreatedAt), UpdatedAt: referenceTimestamp(value.UpdatedAt),
	})
	if err != nil {
		return contact.HistoricalExternalContactBinding{}, referenceHistoryDBError(err)
	}
	return externalContactBindingValue(row)
}

func (s *ReferenceHistoryStore) GetHistoricalExternalContactBinding(ctx context.Context, id int64) (contact.HistoricalExternalContactBinding, error) {
	return externalContactBindingGet(ctx, s.q, id)
}
func (r *ReferenceHistoryReader) GetHistoricalExternalContactBinding(ctx context.Context, id int64) (contact.HistoricalExternalContactBinding, error) {
	return externalContactBindingGet(ctx, r.q, id)
}
func externalContactBindingGet(ctx context.Context, query func(context.Context) (*contactdb.Queries, error), id int64) (contact.HistoricalExternalContactBinding, error) {
	if id < 1 {
		return contact.HistoricalExternalContactBinding{}, contact.ErrReferenceHistoryInvalid
	}
	q, err := query(ctx)
	if err != nil {
		return contact.HistoricalExternalContactBinding{}, err
	}
	row, err := q.GetHistoricalExternalContactBinding(ctx, id)
	if err != nil {
		return contact.HistoricalExternalContactBinding{}, referenceHistoryDBError(err)
	}
	return externalContactBindingValue(row)
}
func (r *ReferenceHistoryReader) ListHistoricalExternalContactBinding(ctx context.Context, page contact.ReferenceHistoryQuery) ([]contact.HistoricalExternalContactBinding, int64, error) {
	if invalidReferenceHistoryPage(page) {
		return nil, 0, contact.ErrReferenceHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalExternalContactBinding(ctx)
	if err != nil {
		return nil, 0, referenceHistoryDBError(err)
	}
	rows, err := q.ListHistoricalExternalContactBinding(ctx, contactdb.ListHistoricalExternalContactBindingParams{RowLimit: page.Limit, RowOffset: page.Offset})
	if err != nil {
		return nil, 0, referenceHistoryDBError(err)
	}
	values := make([]contact.HistoricalExternalContactBinding, 0, len(rows))
	for _, row := range rows {
		value, err := externalContactBindingValue(row)
		if err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	return values, total, nil
}

func (s *ReferenceHistoryStore) CreateHistoricalWeComDirectoryMember(ctx context.Context, value contact.HistoricalWeComDirectoryMember) (contact.HistoricalWeComDirectoryMember, error) {
	if value.ID != 0 || invalidWeComDirectoryMember(value) {
		return contact.HistoricalWeComDirectoryMember{}, contact.ErrReferenceHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return contact.HistoricalWeComDirectoryMember{}, err
	}
	row, err := q.CreateHistoricalWeComDirectoryMember(ctx, contactdb.CreateHistoricalWeComDirectoryMemberParams{
		SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], SourceID: value.SourceID,
		WecomCorpIDDigest: value.WeComCorpIDDigest[:], CorpIDDigest: value.CorpIDDigest[:], WecomUserIDDigest: value.WeComUserIDDigest[:], CorpAttribution: value.CorpAttribution, MatchedStaffID: referenceOptionalInt8(value.MatchedStaffID),
		DisplayName: value.DisplayName, DepartmentIdsDigest: value.DepartmentIDsDigest[:], DepartmentName: value.DepartmentName, Position: value.Position, WecomStatus: referenceOptionalInt4(value.WeComStatus), IsActive: value.IsActive,
		SyncedAt: referenceTimestamp(value.SyncedAt), RawPayloadDigest: value.RawPayloadDigest[:], MobileDigest: value.MobileDigest[:], AvatarUrlDigest: value.AvatarURLDigest[:], UpdatedByDigest: value.UpdatedByDigest[:],
		FirstSeenAt: referenceTimestamp(value.FirstSeenAt), LastSyncedAt: referenceTimestamp(value.LastSyncedAt), CreatedAt: referenceTimestamp(value.CreatedAt), UpdatedAt: referenceTimestamp(value.UpdatedAt),
	})
	if err != nil {
		return contact.HistoricalWeComDirectoryMember{}, referenceHistoryDBError(err)
	}
	return wecomDirectoryMemberValue(row)
}

func (s *ReferenceHistoryStore) GetHistoricalWeComDirectoryMember(ctx context.Context, id int64) (contact.HistoricalWeComDirectoryMember, error) {
	return wecomDirectoryMemberGet(ctx, s.q, id)
}
func (r *ReferenceHistoryReader) GetHistoricalWeComDirectoryMember(ctx context.Context, id int64) (contact.HistoricalWeComDirectoryMember, error) {
	return wecomDirectoryMemberGet(ctx, r.q, id)
}
func wecomDirectoryMemberGet(ctx context.Context, query func(context.Context) (*contactdb.Queries, error), id int64) (contact.HistoricalWeComDirectoryMember, error) {
	if id < 1 {
		return contact.HistoricalWeComDirectoryMember{}, contact.ErrReferenceHistoryInvalid
	}
	q, err := query(ctx)
	if err != nil {
		return contact.HistoricalWeComDirectoryMember{}, err
	}
	row, err := q.GetHistoricalWeComDirectoryMember(ctx, id)
	if err != nil {
		return contact.HistoricalWeComDirectoryMember{}, referenceHistoryDBError(err)
	}
	return wecomDirectoryMemberValue(row)
}
func (r *ReferenceHistoryReader) ListHistoricalWeComDirectoryMember(ctx context.Context, page contact.ReferenceHistoryQuery) ([]contact.HistoricalWeComDirectoryMember, int64, error) {
	if invalidReferenceHistoryPage(page) {
		return nil, 0, contact.ErrReferenceHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalWeComDirectoryMember(ctx)
	if err != nil {
		return nil, 0, referenceHistoryDBError(err)
	}
	rows, err := q.ListHistoricalWeComDirectoryMember(ctx, contactdb.ListHistoricalWeComDirectoryMemberParams{RowLimit: page.Limit, RowOffset: page.Offset})
	if err != nil {
		return nil, 0, referenceHistoryDBError(err)
	}
	values := make([]contact.HistoricalWeComDirectoryMember, 0, len(rows))
	for _, row := range rows {
		value, err := wecomDirectoryMemberValue(row)
		if err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	return values, total, nil
}

func externalContactBindingValue(row contactdb.ContactV1ExternalBindingHistory) (contact.HistoricalExternalContactBinding, error) {
	digests := [][]byte{row.SourceKeyDigest, row.SourcePayloadDigest, row.SourceFieldDigest, row.ExternalUserIDDigest, row.FirstBoundByUserIDDigest, row.FirstOwnerUserIDDigest, row.LastOwnerUserIDDigest}
	values := make([][32]byte, len(digests))
	for i := range digests {
		var ok bool
		if values[i], ok = referenceDigest(digests[i]); !ok {
			return contact.HistoricalExternalContactBinding{}, contact.ErrReferenceHistoryUnavailable
		}
	}
	personHistoryID, personOK := referenceOptionalInt8Value(row.PersonHistoryID)
	identityID, identityOK := referenceOptionalInt8Value(row.IdentityID)
	created, createdOK := referenceTime(row.CreatedAt)
	updated, updatedOK := referenceTime(row.UpdatedAt)
	value := contact.HistoricalExternalContactBinding{ID: row.ID, SourceKeyDigest: values[0], SourcePayloadDigest: values[1], SourceFieldDigest: values[2], ExternalUserIDDigest: values[3], SourcePersonID: row.SourcePersonID, PersonHistoryID: personHistoryID, IdentityID: identityID, IdentityAssurance: row.IdentityAssurance, FirstBoundByUserIDDigest: values[4], FirstOwnerUserIDDigest: values[5], LastOwnerUserIDDigest: values[6], CreatedAt: created, UpdatedAt: updated}
	if !personOK || !identityOK || !createdOK || !updatedOK || invalidExternalContactBindingStored(value) {
		return contact.HistoricalExternalContactBinding{}, contact.ErrReferenceHistoryUnavailable
	}
	return value, nil
}

func wecomDirectoryMemberValue(row contactdb.ContactV1DirectoryMemberHistory) (contact.HistoricalWeComDirectoryMember, error) {
	digests := [][]byte{row.SourceKeyDigest, row.SourcePayloadDigest, row.SourceFieldDigest, row.WecomCorpIDDigest, row.CorpIDDigest, row.WecomUserIDDigest, row.DepartmentIdsDigest, row.RawPayloadDigest, row.MobileDigest, row.AvatarUrlDigest, row.UpdatedByDigest}
	values := make([][32]byte, len(digests))
	for i := range digests {
		var ok bool
		if values[i], ok = referenceDigest(digests[i]); !ok {
			return contact.HistoricalWeComDirectoryMember{}, contact.ErrReferenceHistoryUnavailable
		}
	}
	staffID, staffOK := referenceOptionalInt8Value(row.MatchedStaffID)
	status, statusOK := referenceOptionalInt4Value(row.WecomStatus)
	synced, syncedOK := referenceTime(row.SyncedAt)
	firstSeen, firstSeenOK := referenceTime(row.FirstSeenAt)
	lastSynced, lastSyncedOK := referenceTime(row.LastSyncedAt)
	created, createdOK := referenceTime(row.CreatedAt)
	updated, updatedOK := referenceTime(row.UpdatedAt)
	value := contact.HistoricalWeComDirectoryMember{ID: row.ID, SourceKeyDigest: values[0], SourcePayloadDigest: values[1], SourceFieldDigest: values[2], SourceID: row.SourceID, WeComCorpIDDigest: values[3], CorpIDDigest: values[4], WeComUserIDDigest: values[5], CorpAttribution: row.CorpAttribution, MatchedStaffID: staffID, DisplayName: row.DisplayName, DepartmentIDsDigest: values[6], DepartmentName: row.DepartmentName, Position: row.Position, WeComStatus: status, IsActive: row.IsActive, SyncedAt: synced, RawPayloadDigest: values[7], MobileDigest: values[8], AvatarURLDigest: values[9], UpdatedByDigest: values[10], FirstSeenAt: firstSeen, LastSyncedAt: lastSynced, CreatedAt: created, UpdatedAt: updated}
	if !staffOK || !statusOK || !syncedOK || !firstSeenOK || !lastSyncedOK || !createdOK || !updatedOK || invalidWeComDirectoryMemberStored(value) {
		return contact.HistoricalWeComDirectoryMember{}, contact.ErrReferenceHistoryUnavailable
	}
	return value, nil
}

func invalidExternalContactBinding(value contact.HistoricalExternalContactBinding) bool {
	value.ID = 1
	_, err := contactapp.HistoricalExternalContactBindingDigest(value)
	return err != nil
}
func invalidExternalContactBindingStored(value contact.HistoricalExternalContactBinding) bool {
	_, err := contactapp.HistoricalExternalContactBindingDigest(value)
	return err != nil
}
func invalidWeComDirectoryMember(value contact.HistoricalWeComDirectoryMember) bool {
	value.ID = 1
	_, err := contactapp.HistoricalWeComDirectoryMemberDigest(value)
	return err != nil
}
func invalidWeComDirectoryMemberStored(value contact.HistoricalWeComDirectoryMember) bool {
	_, err := contactapp.HistoricalWeComDirectoryMemberDigest(value)
	return err != nil
}

func invalidReferenceHistoryPage(value contact.ReferenceHistoryQuery) bool {
	return value.Limit < 1 || value.Limit > 100 || value.Offset < 0
}

func referenceDigest(value []byte) ([32]byte, bool) {
	var result [32]byte
	if len(value) != len(result) {
		return result, false
	}
	copy(result[:], value)
	return result, result != ([32]byte{})
}
func referenceOptionalInt8(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}
func referenceOptionalInt8Value(value pgtype.Int8) (*int64, bool) {
	if !value.Valid {
		return nil, true
	}
	result := value.Int64
	return &result, true
}
func referenceOptionalInt4(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}
func referenceOptionalInt4Value(value pgtype.Int4) (*int32, bool) {
	if !value.Valid {
		return nil, true
	}
	result := value.Int32
	return &result, true
}
func referenceTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func referenceTime(value pgtype.Timestamptz) (time.Time, bool) {
	if !value.Valid || value.InfinityModifier != pgtype.Finite {
		return time.Time{}, false
	}
	return value.Time.UTC().Truncate(time.Microsecond), true
}
func referenceHistoryDBError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return contact.ErrReferenceHistoryConflict
	}
	return contact.ErrReferenceHistoryUnavailable
}
