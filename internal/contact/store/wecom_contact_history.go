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

type WeComContactHistoryStore struct{}
type WeComContactHistoryReader struct{ db contactdb.DBTX }

var _ contact.WeComContactHistoryStore = (*WeComContactHistoryStore)(nil)
var _ contact.WeComContactHistoryReader = (*WeComContactHistoryReader)(nil)

func NewWeComContactHistoryStore() *WeComContactHistoryStore { return &WeComContactHistoryStore{} }
func NewWeComContactHistoryReader(db contactdb.DBTX) *WeComContactHistoryReader {
	return &WeComContactHistoryReader{db: db}
}

func (s *WeComContactHistoryStore) q(ctx context.Context) (*contactdb.Queries, error) {
	if s == nil || ctx == nil || ctx.Err() != nil {
		return nil, contact.ErrWeComContactHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, contact.ErrWeComContactHistoryUnavailable
	}
	return contactdb.New(tx), nil
}
func (r *WeComContactHistoryReader) q(ctx context.Context) (*contactdb.Queries, error) {
	if r == nil || ctx == nil || ctx.Err() != nil {
		return nil, contact.ErrWeComContactHistoryUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return contactdb.New(tx), nil
	}
	if nilWeComContactHistoryDB(r.db) {
		return nil, contact.ErrWeComContactHistoryUnavailable
	}
	return contactdb.New(r.db), nil
}
func nilWeComContactHistoryDB(value contactdb.DBTX) bool {
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

func (s *WeComContactHistoryStore) CreateHistoricalWeComExternalContactEventLog(ctx context.Context, value contact.HistoricalWeComExternalContactEventLog) (contact.HistoricalWeComExternalContactEventLog, error) {
	if value.ID != 0 || invalidWeComEvent(value) {
		return contact.HistoricalWeComExternalContactEventLog{}, contact.ErrWeComContactHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return contact.HistoricalWeComExternalContactEventLog{}, err
	}
	row, err := q.CreateHistoricalWeComExternalContactEventLog(ctx, contactdb.CreateHistoricalWeComExternalContactEventLogParams{
		SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], SourceID: value.SourceID, CorpIDDigest: value.CorpIDDigest[:], EventType: value.EventType, ChangeType: value.ChangeType, ExternalUserIDDigest: value.ExternalUserIDDigest[:], UserIDDigest: value.UserIDDigest[:], EventTime: wecomInt8(value.EventTime), EventKeyDigest: value.EventKeyDigest[:], PayloadXmlDigest: value.PayloadXMLDigest[:], PayloadJsonDigest: value.PayloadJSONDigest[:], ProcessStatus: value.ProcessStatus, RetryCount: value.RetryCount, ErrorMessageDigest: value.ErrorMessageDigest[:], CreatedAt: wecomTimestamp(value.CreatedAt), UpdatedAt: wecomTimestamp(value.UpdatedAt), IdentitySyncStatus: value.IdentitySyncStatus, IdentitySyncErrorCodeDigest: value.IdentitySyncErrorCodeDigest[:], IdentitySyncErrorMessageDigest: value.IdentitySyncErrorMessageDigest[:], IdentitySyncResponseDigest: value.IdentitySyncResponseDigest[:],
	})
	if err != nil {
		return contact.HistoricalWeComExternalContactEventLog{}, wecomContactHistoryDBError(err)
	}
	return wecomEventValue(row)
}
func (s *WeComContactHistoryStore) GetHistoricalWeComExternalContactEventLog(ctx context.Context, id int64) (contact.HistoricalWeComExternalContactEventLog, error) {
	return wecomEventGet(ctx, s.q, id)
}
func (r *WeComContactHistoryReader) GetHistoricalWeComExternalContactEventLog(ctx context.Context, id int64) (contact.HistoricalWeComExternalContactEventLog, error) {
	return wecomEventGet(ctx, r.q, id)
}
func wecomEventGet(ctx context.Context, query func(context.Context) (*contactdb.Queries, error), id int64) (contact.HistoricalWeComExternalContactEventLog, error) {
	if id < 1 {
		return contact.HistoricalWeComExternalContactEventLog{}, contact.ErrWeComContactHistoryInvalid
	}
	q, err := query(ctx)
	if err != nil {
		return contact.HistoricalWeComExternalContactEventLog{}, err
	}
	row, err := q.GetHistoricalWeComExternalContactEventLog(ctx, id)
	if err != nil {
		return contact.HistoricalWeComExternalContactEventLog{}, wecomContactHistoryDBError(err)
	}
	return wecomEventValue(row)
}
func (r *WeComContactHistoryReader) ListHistoricalWeComExternalContactEventLog(ctx context.Context, query contact.WeComContactHistoryQuery) ([]contact.HistoricalWeComExternalContactEventLog, int64, error) {
	if invalidWeComQuery(query) {
		return nil, 0, contact.ErrWeComContactHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalWeComExternalContactEventLog(ctx)
	if err != nil {
		return nil, 0, wecomContactHistoryDBError(err)
	}
	rows, err := q.ListHistoricalWeComExternalContactEventLog(ctx, contactdb.ListHistoricalWeComExternalContactEventLogParams{RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, wecomContactHistoryDBError(err)
	}
	values := make([]contact.HistoricalWeComExternalContactEventLog, 0, len(rows))
	for _, row := range rows {
		value, err := wecomEventValue(row)
		if err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	return values, total, nil
}

func (s *WeComContactHistoryStore) CreateHistoricalWeComExternalContactFollowUser(ctx context.Context, value contact.HistoricalWeComExternalContactFollowUser) (contact.HistoricalWeComExternalContactFollowUser, error) {
	if value.ID != 0 || invalidWeComFollow(value) {
		return contact.HistoricalWeComExternalContactFollowUser{}, contact.ErrWeComContactHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return contact.HistoricalWeComExternalContactFollowUser{}, err
	}
	row, err := q.CreateHistoricalWeComExternalContactFollowUser(ctx, contactdb.CreateHistoricalWeComExternalContactFollowUserParams{
		SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], SourceID: value.SourceID, CorpIDDigest: value.CorpIDDigest[:], ExternalUserIDDigest: value.ExternalUserIDDigest[:], UserIDDigest: value.UserIDDigest[:], RelationStatus: value.RelationStatus, IsPrimary: value.IsPrimary, RemarkDigest: value.RemarkDigest[:], DescriptionDigest: value.DescriptionDigest[:], AddWay: wecomInt4(value.AddWay), State: value.State, OperUserIDDigest: value.OperUserIDDigest[:], CreateTime: wecomInt8(value.CreateTime), RawFollowUserDigest: value.RawFollowUserDigest[:], FirstSeenAt: wecomTimestamp(value.FirstSeenAt), LastSeenAt: wecomTimestamp(value.LastSeenAt), CreatedAt: wecomTimestamp(value.CreatedAt), UpdatedAt: wecomTimestamp(value.UpdatedAt),
	})
	if err != nil {
		return contact.HistoricalWeComExternalContactFollowUser{}, wecomContactHistoryDBError(err)
	}
	return wecomFollowValue(row)
}
func (s *WeComContactHistoryStore) GetHistoricalWeComExternalContactFollowUser(ctx context.Context, id int64) (contact.HistoricalWeComExternalContactFollowUser, error) {
	return wecomFollowGet(ctx, s.q, id)
}
func (r *WeComContactHistoryReader) GetHistoricalWeComExternalContactFollowUser(ctx context.Context, id int64) (contact.HistoricalWeComExternalContactFollowUser, error) {
	return wecomFollowGet(ctx, r.q, id)
}
func wecomFollowGet(ctx context.Context, query func(context.Context) (*contactdb.Queries, error), id int64) (contact.HistoricalWeComExternalContactFollowUser, error) {
	if id < 1 {
		return contact.HistoricalWeComExternalContactFollowUser{}, contact.ErrWeComContactHistoryInvalid
	}
	q, err := query(ctx)
	if err != nil {
		return contact.HistoricalWeComExternalContactFollowUser{}, err
	}
	row, err := q.GetHistoricalWeComExternalContactFollowUser(ctx, id)
	if err != nil {
		return contact.HistoricalWeComExternalContactFollowUser{}, wecomContactHistoryDBError(err)
	}
	return wecomFollowValue(row)
}
func (r *WeComContactHistoryReader) ListHistoricalWeComExternalContactFollowUser(ctx context.Context, query contact.WeComContactHistoryQuery) ([]contact.HistoricalWeComExternalContactFollowUser, int64, error) {
	if invalidWeComQuery(query) {
		return nil, 0, contact.ErrWeComContactHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalWeComExternalContactFollowUser(ctx)
	if err != nil {
		return nil, 0, wecomContactHistoryDBError(err)
	}
	rows, err := q.ListHistoricalWeComExternalContactFollowUser(ctx, contactdb.ListHistoricalWeComExternalContactFollowUserParams{RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, wecomContactHistoryDBError(err)
	}
	values := make([]contact.HistoricalWeComExternalContactFollowUser, 0, len(rows))
	for _, row := range rows {
		value, err := wecomFollowValue(row)
		if err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	return values, total, nil
}

func wecomEventValue(row contactdb.ContactV1WecomEventLogHistory) (contact.HistoricalWeComExternalContactEventLog, error) {
	key, ok := wecomDigest(row.SourceKeyDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactEventLog{}, contact.ErrWeComContactHistoryUnavailable
	}
	payload, ok := wecomDigest(row.SourcePayloadDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactEventLog{}, contact.ErrWeComContactHistoryUnavailable
	}
	field, ok := wecomDigest(row.SourceFieldDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactEventLog{}, contact.ErrWeComContactHistoryUnavailable
	}
	corp, ok := wecomDigest(row.CorpIDDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactEventLog{}, contact.ErrWeComContactHistoryUnavailable
	}
	external, ok := wecomDigest(row.ExternalUserIDDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactEventLog{}, contact.ErrWeComContactHistoryUnavailable
	}
	user, ok := wecomDigest(row.UserIDDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactEventLog{}, contact.ErrWeComContactHistoryUnavailable
	}
	eventKey, ok := wecomDigest(row.EventKeyDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactEventLog{}, contact.ErrWeComContactHistoryUnavailable
	}
	xml, ok := wecomDigest(row.PayloadXmlDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactEventLog{}, contact.ErrWeComContactHistoryUnavailable
	}
	jsonDigest, ok := wecomDigest(row.PayloadJsonDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactEventLog{}, contact.ErrWeComContactHistoryUnavailable
	}
	errDigest, ok := wecomDigest(row.ErrorMessageDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactEventLog{}, contact.ErrWeComContactHistoryUnavailable
	}
	code, ok := wecomDigest(row.IdentitySyncErrorCodeDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactEventLog{}, contact.ErrWeComContactHistoryUnavailable
	}
	message, ok := wecomDigest(row.IdentitySyncErrorMessageDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactEventLog{}, contact.ErrWeComContactHistoryUnavailable
	}
	response, ok := wecomDigest(row.IdentitySyncResponseDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactEventLog{}, contact.ErrWeComContactHistoryUnavailable
	}
	created, createdOK := wecomTime(row.CreatedAt)
	updated, updatedOK := wecomTime(row.UpdatedAt)
	value := contact.HistoricalWeComExternalContactEventLog{ID: row.ID, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, SourceID: row.SourceID, CorpIDDigest: corp, EventType: row.EventType, ChangeType: row.ChangeType, ExternalUserIDDigest: external, UserIDDigest: user, EventTime: wecomValueInt8(row.EventTime), EventKeyDigest: eventKey, PayloadXMLDigest: xml, PayloadJSONDigest: jsonDigest, ProcessStatus: row.ProcessStatus, RetryCount: row.RetryCount, ErrorMessageDigest: errDigest, CreatedAt: created, UpdatedAt: updated, IdentitySyncStatus: row.IdentitySyncStatus, IdentitySyncErrorCodeDigest: code, IdentitySyncErrorMessageDigest: message, IdentitySyncResponseDigest: response}
	if !createdOK || !updatedOK || invalidWeComEvent(value) {
		return contact.HistoricalWeComExternalContactEventLog{}, contact.ErrWeComContactHistoryUnavailable
	}
	return value, nil
}
func wecomFollowValue(row contactdb.ContactV1WecomFollowUserHistory) (contact.HistoricalWeComExternalContactFollowUser, error) {
	key, ok := wecomDigest(row.SourceKeyDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactFollowUser{}, contact.ErrWeComContactHistoryUnavailable
	}
	payload, ok := wecomDigest(row.SourcePayloadDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactFollowUser{}, contact.ErrWeComContactHistoryUnavailable
	}
	field, ok := wecomDigest(row.SourceFieldDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactFollowUser{}, contact.ErrWeComContactHistoryUnavailable
	}
	corp, ok := wecomDigest(row.CorpIDDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactFollowUser{}, contact.ErrWeComContactHistoryUnavailable
	}
	external, ok := wecomDigest(row.ExternalUserIDDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactFollowUser{}, contact.ErrWeComContactHistoryUnavailable
	}
	user, ok := wecomDigest(row.UserIDDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactFollowUser{}, contact.ErrWeComContactHistoryUnavailable
	}
	remark, ok := wecomDigest(row.RemarkDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactFollowUser{}, contact.ErrWeComContactHistoryUnavailable
	}
	description, ok := wecomDigest(row.DescriptionDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactFollowUser{}, contact.ErrWeComContactHistoryUnavailable
	}
	oper, ok := wecomDigest(row.OperUserIDDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactFollowUser{}, contact.ErrWeComContactHistoryUnavailable
	}
	raw, ok := wecomDigest(row.RawFollowUserDigest)
	if !ok {
		return contact.HistoricalWeComExternalContactFollowUser{}, contact.ErrWeComContactHistoryUnavailable
	}
	first, firstOK := wecomTime(row.FirstSeenAt)
	last, lastOK := wecomTime(row.LastSeenAt)
	created, createdOK := wecomTime(row.CreatedAt)
	updated, updatedOK := wecomTime(row.UpdatedAt)
	value := contact.HistoricalWeComExternalContactFollowUser{ID: row.ID, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, SourceID: row.SourceID, CorpIDDigest: corp, ExternalUserIDDigest: external, UserIDDigest: user, RelationStatus: row.RelationStatus, IsPrimary: row.IsPrimary, RemarkDigest: remark, DescriptionDigest: description, AddWay: wecomValueInt4(row.AddWay), State: row.State, OperUserIDDigest: oper, CreateTime: wecomValueInt8(row.CreateTime), RawFollowUserDigest: raw, FirstSeenAt: first, LastSeenAt: last, CreatedAt: created, UpdatedAt: updated}
	if !firstOK || !lastOK || !createdOK || !updatedOK || invalidWeComFollow(value) {
		return contact.HistoricalWeComExternalContactFollowUser{}, contact.ErrWeComContactHistoryUnavailable
	}
	return value, nil
}

func invalidWeComEvent(value contact.HistoricalWeComExternalContactEventLog) bool {
	if value.ID == 0 {
		value.ID = 1
	}
	_, err := contactapp.HistoricalWeComExternalContactEventLogDigest(value)
	return err != nil
}
func invalidWeComFollow(value contact.HistoricalWeComExternalContactFollowUser) bool {
	if value.ID == 0 {
		value.ID = 1
	}
	_, err := contactapp.HistoricalWeComExternalContactFollowUserDigest(value)
	return err != nil
}
func invalidWeComQuery(query contact.WeComContactHistoryQuery) bool {
	return query.Limit < 1 || query.Limit > 100 || query.Offset < 0
}
func wecomDigest(value []byte) ([32]byte, bool) {
	var digest [32]byte
	if len(value) != len(digest) {
		return digest, false
	}
	copy(digest[:], value)
	return digest, digest != ([32]byte{})
}
func wecomTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func wecomTime(value pgtype.Timestamptz) (time.Time, bool) {
	if !value.Valid || value.InfinityModifier != pgtype.Finite {
		return time.Time{}, false
	}
	return value.Time.UTC().Truncate(time.Microsecond), true
}
func wecomInt8(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}
func wecomInt4(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}
func wecomValueInt8(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}
func wecomValueInt4(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	result := value.Int32
	return &result
}
func wecomContactHistoryDBError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return contact.ErrWeComContactHistoryConflict
	}
	return contact.ErrWeComContactHistoryUnavailable
}
