package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

const (
	wecomContactEventLogKind   = "wecom_contact_event_log"
	wecomContactFollowUserKind = "wecom_contact_follow_user"
)

// WeComContactHistoryWriter writes only inert V1 observations and their
// caller-transaction journal receipts. It has no current-contact or Provider
// dependency.
type WeComContactHistoryWriter struct {
	store   contact.WeComContactHistoryStore
	journal contact.WeComContactHistoryJournal
}

func NewWeComContactHistoryWriter(store contact.WeComContactHistoryStore, journal contact.WeComContactHistoryJournal) (*WeComContactHistoryWriter, error) {
	if nilWeComContactHistory(store) || nilWeComContactHistory(journal) {
		return nil, contact.ErrWeComContactHistoryUnavailable
	}
	return &WeComContactHistoryWriter{store: store, journal: journal}, nil
}

func (w *WeComContactHistoryWriter) ImportHistoricalWeComExternalContactEventLog(ctx context.Context, source string, value contact.HistoricalWeComExternalContactEventLog) (contact.WeComContactHistoryReceipt, error) {
	value = normalizeWeComEventLog(value)
	return importWeComContactHistory(w, ctx, wecomContactEventLogKind, source, value, HistoricalWeComExternalContactEventLogDigest,
		func(v contact.HistoricalWeComExternalContactEventLog, id int64) contact.HistoricalWeComExternalContactEventLog {
			v.ID = id
			return v
		},
		func() (contact.HistoricalWeComExternalContactEventLog, error) {
			return w.store.CreateHistoricalWeComExternalContactEventLog(ctx, value)
		},
		func(id int64) (contact.HistoricalWeComExternalContactEventLog, error) {
			return w.store.GetHistoricalWeComExternalContactEventLog(ctx, id)
		})
}

func (w *WeComContactHistoryWriter) ImportHistoricalWeComExternalContactFollowUser(ctx context.Context, source string, value contact.HistoricalWeComExternalContactFollowUser) (contact.WeComContactHistoryReceipt, error) {
	value = normalizeWeComFollowUser(value)
	return importWeComContactHistory(w, ctx, wecomContactFollowUserKind, source, value, HistoricalWeComExternalContactFollowUserDigest,
		func(v contact.HistoricalWeComExternalContactFollowUser, id int64) contact.HistoricalWeComExternalContactFollowUser {
			v.ID = id
			return v
		},
		func() (contact.HistoricalWeComExternalContactFollowUser, error) {
			return w.store.CreateHistoricalWeComExternalContactFollowUser(ctx, value)
		},
		func(id int64) (contact.HistoricalWeComExternalContactFollowUser, error) {
			return w.store.GetHistoricalWeComExternalContactFollowUser(ctx, id)
		})
}

func importWeComContactHistory[T any](w *WeComContactHistoryWriter, ctx context.Context, kind, source string, value T, digest func(T) ([32]byte, error), withID func(T, int64) T, create func() (T, error), get func(int64) (T, error)) (contact.WeComContactHistoryReceipt, error) {
	var empty contact.WeComContactHistoryReceipt
	if w == nil || ctx == nil || ctx.Err() != nil || nilWeComContactHistory(w.store) || nilWeComContactHistory(w.journal) {
		return empty, contact.ErrWeComContactHistoryUnavailable
	}
	key, payload, id, ok := wecomContactIdentity(value)
	if !ok || id != 0 || key == ([32]byte{}) || payload == ([32]byte{}) || source != hex.EncodeToString(key[:]) {
		return empty, contact.ErrWeComContactHistoryInvalid
	}
	if _, err := digest(withID(value, 1)); err != nil {
		return empty, contact.ErrWeComContactHistoryInvalid
	}
	receipt, found, err := w.journal.LoadWeComContactHistory(ctx, kind, source)
	if err != nil {
		return empty, wecomContactHistoryError(err)
	}
	if found {
		if !validWeComContactReceipt(receipt, kind, source, payload) {
			return empty, contact.ErrWeComContactHistoryConflict
		}
		actual, err := get(receipt.TargetID)
		if err != nil {
			return empty, wecomContactHistoryError(err)
		}
		actualDigest, actualErr := digest(actual)
		expectedDigest, expectedErr := digest(withID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, contact.ErrWeComContactHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := create()
	if err != nil {
		return empty, wecomContactHistoryError(err)
	}
	_, _, targetID, ok := wecomContactIdentity(actual)
	if !ok || targetID < 1 {
		return empty, contact.ErrWeComContactHistoryConflict
	}
	actualDigest, actualErr := digest(actual)
	expectedDigest, expectedErr := digest(withID(value, targetID))
	if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest {
		return empty, contact.ErrWeComContactHistoryConflict
	}
	receipt = contact.WeComContactHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetDigest: actualDigest, TargetID: targetID}
	if err = w.journal.RecordWeComContactHistory(ctx, receipt); err != nil {
		return empty, wecomContactHistoryError(err)
	}
	return receipt, nil
}

// Digest encodings deliberately list every private source fact. Marshaling the
// Port value directly would omit json:"-" fields and permit silent drift.
func HistoricalWeComExternalContactEventLogDigest(v contact.HistoricalWeComExternalContactEventLog) ([32]byte, error) {
	if !validWeComEventLog(v, true) {
		return [32]byte{}, contact.ErrWeComContactHistoryInvalid
	}
	return digestWeComContact(wecomContactEventLogKind, struct {
		ID                                                                                                                             int64
		SourceKey, SourcePayload, SourceField, Corp, ExternalUser, User, EventKey, XML, JSON, Error, ErrorCode, ErrorMessage, Response [32]byte
		SourceID                                                                                                                       int64
		EventType, ChangeType, ProcessStatus, IdentitySyncStatus                                                                       string
		EventTime                                                                                                                      *int64
		RetryCount                                                                                                                     int32
		CreatedAt, UpdatedAt                                                                                                           time.Time
	}{v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.CorpIDDigest, v.ExternalUserIDDigest, v.UserIDDigest, v.EventKeyDigest, v.PayloadXMLDigest, v.PayloadJSONDigest, v.ErrorMessageDigest, v.IdentitySyncErrorCodeDigest, v.IdentitySyncErrorMessageDigest, v.IdentitySyncResponseDigest, v.SourceID, v.EventType, v.ChangeType, v.ProcessStatus, v.IdentitySyncStatus, v.EventTime, v.RetryCount, v.CreatedAt, v.UpdatedAt})
}

func HistoricalWeComExternalContactFollowUserDigest(v contact.HistoricalWeComExternalContactFollowUser) ([32]byte, error) {
	if !validWeComFollowUser(v, true) {
		return [32]byte{}, contact.ErrWeComContactHistoryInvalid
	}
	return digestWeComContact(wecomContactFollowUserKind, struct {
		ID                                                                                                        int64
		SourceKey, SourcePayload, SourceField, Corp, ExternalUser, User, Remark, Description, OperUser, RawFollow [32]byte
		SourceID                                                                                                  int64
		RelationStatus, State                                                                                     string
		IsPrimary                                                                                                 bool
		AddWay                                                                                                    *int32
		CreateTime                                                                                                *int64
		FirstSeenAt, LastSeenAt, CreatedAt, UpdatedAt                                                             time.Time
	}{v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.CorpIDDigest, v.ExternalUserIDDigest, v.UserIDDigest, v.RemarkDigest, v.DescriptionDigest, v.OperUserIDDigest, v.RawFollowUserDigest, v.SourceID, v.RelationStatus, v.State, v.IsPrimary, v.AddWay, v.CreateTime, v.FirstSeenAt, v.LastSeenAt, v.CreatedAt, v.UpdatedAt})
}

func digestWeComContact(kind string, value any) ([32]byte, error) {
	encoded, err := json.Marshal(struct {
		Kind  string
		Value any
	}{kind, value})
	if err != nil {
		return [32]byte{}, contact.ErrWeComContactHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func validWeComEventLog(v contact.HistoricalWeComExternalContactEventLog, stored bool) bool {
	return validWeComContactIdentity(v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, stored) &&
		allWeComDigests(v.CorpIDDigest, v.ExternalUserIDDigest, v.UserIDDigest, v.EventKeyDigest, v.PayloadXMLDigest, v.PayloadJSONDigest, v.ErrorMessageDigest, v.IdentitySyncErrorCodeDigest, v.IdentitySyncErrorMessageDigest, v.IdentitySyncResponseDigest) &&
		validWeComText(v.EventType) && validWeComText(v.ChangeType) && validWeComText(v.ProcessStatus) && validWeComText(v.IdentitySyncStatus) && validWeComTime(v.CreatedAt, stored) && validWeComTime(v.UpdatedAt, stored)
}
func validWeComFollowUser(v contact.HistoricalWeComExternalContactFollowUser, stored bool) bool {
	return validWeComContactIdentity(v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, stored) &&
		allWeComDigests(v.CorpIDDigest, v.ExternalUserIDDigest, v.UserIDDigest, v.RemarkDigest, v.DescriptionDigest, v.OperUserIDDigest, v.RawFollowUserDigest) &&
		validWeComText(v.RelationStatus) && validWeComText(v.State) && validWeComTime(v.FirstSeenAt, stored) && validWeComTime(v.LastSeenAt, stored) && validWeComTime(v.CreatedAt, stored) && validWeComTime(v.UpdatedAt, stored)
}
func validWeComContactIdentity(id int64, key, payload, field [32]byte, stored bool) bool {
	return (stored && id > 0 || !stored && id == 0) && allWeComDigests(key, payload, field)
}
func allWeComDigests(values ...[32]byte) bool {
	for _, value := range values {
		if value == ([32]byte{}) {
			return false
		}
	}
	return true
}
func validWeComText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}
func validWeComTime(value time.Time, stored bool) bool {
	return !value.IsZero() && (!stored || value.Location() == time.UTC && value.Equal(value.UTC().Truncate(time.Microsecond)))
}
func normalizeWeComTime(value time.Time) time.Time { return value.UTC().Truncate(time.Microsecond) }
func normalizeWeComEventLog(v contact.HistoricalWeComExternalContactEventLog) contact.HistoricalWeComExternalContactEventLog {
	v.CreatedAt, v.UpdatedAt = normalizeWeComTime(v.CreatedAt), normalizeWeComTime(v.UpdatedAt)
	v.EventTime = copyWeComInt64(v.EventTime)
	return v
}
func normalizeWeComFollowUser(v contact.HistoricalWeComExternalContactFollowUser) contact.HistoricalWeComExternalContactFollowUser {
	v.FirstSeenAt, v.LastSeenAt, v.CreatedAt, v.UpdatedAt = normalizeWeComTime(v.FirstSeenAt), normalizeWeComTime(v.LastSeenAt), normalizeWeComTime(v.CreatedAt), normalizeWeComTime(v.UpdatedAt)
	v.AddWay, v.CreateTime = copyWeComInt32(v.AddWay), copyWeComInt64(v.CreateTime)
	return v
}
func copyWeComInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
func copyWeComInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
func wecomContactIdentity(value any) ([32]byte, [32]byte, int64, bool) {
	switch v := value.(type) {
	case contact.HistoricalWeComExternalContactEventLog:
		return v.SourceKeyDigest, v.SourcePayloadDigest, v.ID, true
	case contact.HistoricalWeComExternalContactFollowUser:
		return v.SourceKeyDigest, v.SourcePayloadDigest, v.ID, true
	}
	return [32]byte{}, [32]byte{}, 0, false
}
func validWeComContactReceipt(v contact.WeComContactHistoryReceipt, kind, source string, payload [32]byte) bool {
	return v.Kind == kind && v.SourceIdentifier == source && v.PayloadDigest == payload && v.TargetID > 0 && v.TargetDigest != ([32]byte{})
}
func wecomContactHistoryError(err error) error {
	if errors.Is(err, contact.ErrWeComContactHistoryInvalid) {
		return contact.ErrWeComContactHistoryInvalid
	}
	if errors.Is(err, contact.ErrWeComContactHistoryConflict) {
		return contact.ErrWeComContactHistoryConflict
	}
	return contact.ErrWeComContactHistoryUnavailable
}
func nilWeComContactHistory(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return (v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface) && v.IsNil()
}
