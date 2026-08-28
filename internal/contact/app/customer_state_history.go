package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

const (
	customerStatusSnapshotKind = "customer_status_snapshot"
	customerStatusChangeKind   = "customer_status_change"
	classTermTagMappingKind    = "class_term_tag_mapping"
)

type CustomerStateHistoryWriter struct {
	store   contact.CustomerStateHistoryStore
	journal contact.CustomerStateHistoryJournal
}

func NewCustomerStateHistoryWriter(store contact.CustomerStateHistoryStore, journal contact.CustomerStateHistoryJournal) (*CustomerStateHistoryWriter, error) {
	if nilCustomerState(store) || nilCustomerState(journal) {
		return nil, contact.ErrCustomerStateHistoryUnavailable
	}
	return &CustomerStateHistoryWriter{store: store, journal: journal}, nil
}
func (w *CustomerStateHistoryWriter) ImportCustomerStatusSnapshot(ctx context.Context, source string, value contact.HistoricalCustomerStatusSnapshot) (contact.CustomerStateHistoryReceipt, error) {
	value = normalizeCustomerStatusSnapshot(value)
	return importCustomerState(w, ctx, customerStatusSnapshotKind, source, value, HistoricalCustomerStatusSnapshotDigest, func(v contact.HistoricalCustomerStatusSnapshot, id int64) contact.HistoricalCustomerStatusSnapshot {
		v.ID = id
		return v
	}, func() (contact.HistoricalCustomerStatusSnapshot, error) {
		return w.store.CreateHistoricalCustomerStatusSnapshot(ctx, value)
	}, func(id int64) (contact.HistoricalCustomerStatusSnapshot, error) {
		return w.store.GetHistoricalCustomerStatusSnapshot(ctx, id)
	})
}
func (w *CustomerStateHistoryWriter) ImportCustomerStatusChange(ctx context.Context, source string, value contact.HistoricalCustomerStatusChange) (contact.CustomerStateHistoryReceipt, error) {
	value = normalizeCustomerStatusChange(value)
	return importCustomerState(w, ctx, customerStatusChangeKind, source, value, HistoricalCustomerStatusChangeDigest, func(v contact.HistoricalCustomerStatusChange, id int64) contact.HistoricalCustomerStatusChange {
		v.ID = id
		return v
	}, func() (contact.HistoricalCustomerStatusChange, error) {
		return w.store.CreateHistoricalCustomerStatusChange(ctx, value)
	}, func(id int64) (contact.HistoricalCustomerStatusChange, error) {
		return w.store.GetHistoricalCustomerStatusChange(ctx, id)
	})
}
func (w *CustomerStateHistoryWriter) ImportClassTermTagMapping(ctx context.Context, source string, value contact.HistoricalClassTermTagMapping) (contact.CustomerStateHistoryReceipt, error) {
	value = normalizeClassTermTagMapping(value)
	return importCustomerState(w, ctx, classTermTagMappingKind, source, value, HistoricalClassTermTagMappingDigest, func(v contact.HistoricalClassTermTagMapping, id int64) contact.HistoricalClassTermTagMapping {
		v.ID = id
		return v
	}, func() (contact.HistoricalClassTermTagMapping, error) {
		return w.store.CreateHistoricalClassTermTagMapping(ctx, value)
	}, func(id int64) (contact.HistoricalClassTermTagMapping, error) {
		return w.store.GetHistoricalClassTermTagMapping(ctx, id)
	})
}

func importCustomerState[T any](w *CustomerStateHistoryWriter, ctx context.Context, kind, source string, value T, digest func(T) ([32]byte, error), withID func(T, int64) T, create func() (T, error), get func(int64) (T, error)) (contact.CustomerStateHistoryReceipt, error) {
	var empty contact.CustomerStateHistoryReceipt
	if w == nil || ctx == nil || ctx.Err() != nil || nilCustomerState(w.store) || nilCustomerState(w.journal) {
		return empty, contact.ErrCustomerStateHistoryUnavailable
	}
	key, payload, id, ok := customerStateIdentity(value)
	if !ok || id != 0 || key == ([32]byte{}) || payload == ([32]byte{}) || source != hex.EncodeToString(key[:]) {
		return empty, contact.ErrCustomerStateHistoryInvalid
	}
	if _, err := digest(withID(value, 1)); err != nil {
		return empty, contact.ErrCustomerStateHistoryInvalid
	}
	receipt, found, err := w.journal.LoadCustomerStateHistory(ctx, kind, source)
	if err != nil {
		return empty, customerStateError(err)
	}
	if found {
		if !validCustomerStateReceipt(receipt, kind, source, payload) {
			return empty, contact.ErrCustomerStateHistoryConflict
		}
		actual, err := get(receipt.TargetID)
		if err != nil {
			return empty, customerStateError(err)
		}
		actualDigest, actualErr := digest(actual)
		expectedDigest, expectedErr := digest(withID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, contact.ErrCustomerStateHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := create()
	if err != nil {
		return empty, customerStateError(err)
	}
	_, _, targetID, ok := customerStateIdentity(actual)
	if !ok || targetID < 1 {
		return empty, contact.ErrCustomerStateHistoryConflict
	}
	actualDigest, actualErr := digest(actual)
	expectedDigest, expectedErr := digest(withID(value, targetID))
	if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest {
		return empty, contact.ErrCustomerStateHistoryConflict
	}
	receipt = contact.CustomerStateHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetDigest: actualDigest, TargetID: targetID}
	if err := w.journal.RecordCustomerStateHistory(ctx, receipt); err != nil {
		return empty, customerStateError(err)
	}
	return receipt, nil
}

// These encodings deliberately name private fields: json.Marshal(value) would
// omit every json:"-" source fact from the target digest.
func HistoricalCustomerStatusSnapshotDigest(v contact.HistoricalCustomerStatusSnapshot) ([32]byte, error) {
	if !validCustomerStatusSnapshot(v, true) {
		return [32]byte{}, contact.ErrCustomerStateHistoryInvalid
	}
	return digestCustomerState(customerStatusSnapshotKind, struct {
		ID                                         int64
		Key, Payload, Field                        [32]byte
		SignupStatus, SignupLabel, Customer, Owner string
		SetBy                                      [32]byte
		SetAt                                      time.Time
		SyncStatus                                 string
		SyncError, Flags                           [32]byte
		CreatedAt, UpdatedAt                       time.Time
		UnionID                                    string
	}{v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.SignupStatus, v.SignupLabelName, v.CustomerNameSnapshot, v.OwnerUserIDSnapshot, v.SetByUserIDDigest, v.SetAt, v.WeComTagSyncStatus, v.WeComTagSyncErrorHash, v.StatusFlagsDigest, v.CreatedAt, v.UpdatedAt, v.UnionID})
}
func HistoricalCustomerStatusChangeDigest(v contact.HistoricalCustomerStatusChange) ([32]byte, error) {
	if !validCustomerStatusChange(v, true) {
		return [32]byte{}, contact.ErrCustomerStateHistoryInvalid
	}
	return digestCustomerState(customerStatusChangeKind, struct {
		ID                                                        int64
		Key, Payload, Field                                       [32]byte
		SourceID                                                  int64
		OldStatus, NewStatus, OldLabel, NewLabel, Customer, Owner string
		SetBy                                                     [32]byte
		SetAt                                                     time.Time
		SyncStatus                                                string
		SyncError, Flags                                          [32]byte
		CreatedAt                                                 time.Time
		UnionID                                                   string
	}{v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.SourceID, v.OldSignupStatus, v.NewSignupStatus, v.OldLabelName, v.NewLabelName, v.CustomerNameSnapshot, v.OwnerUserIDSnapshot, v.SetByUserIDDigest, v.SetAt, v.WeComTagSyncStatus, v.WeComTagSyncErrorHash, v.StatusFlagsDigest, v.CreatedAt, v.UnionID})
}
func HistoricalClassTermTagMappingDigest(v contact.HistoricalClassTermTagMapping) ([32]byte, error) {
	if !validClassTermTagMapping(v, true) {
		return [32]byte{}, contact.ErrCustomerStateHistoryInvalid
	}
	return digestCustomerState(classTermTagMappingKind, struct {
		ID                         int64
		Key, Payload, Field        [32]byte
		SourceID                   int64
		Group, Tag                 string
		TermNo                     int32
		Label                      string
		Active                     bool
		CreatedAt, UpdatedAt       time.Time
		StrategyID, GroupID, TagID string
	}{v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.SourceID, v.TagGroupName, v.TagName, v.ClassTermNo, v.ClassTermLabel, v.OriginalActive, v.CreatedAt, v.UpdatedAt, v.StrategySourceID, v.GroupSourceID, v.TagSourceID})
}
func digestCustomerState(kind string, value any) ([32]byte, error) {
	encoded, err := json.Marshal(struct {
		Kind  string `json:"kind"`
		Value any    `json:"value"`
	}{kind, value})
	if err != nil {
		return [32]byte{}, contact.ErrCustomerStateHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func validCustomerStateIdentity(id int64, key, payload, field [32]byte, stored bool) bool {
	return (stored && id > 0 || !stored && id == 0) && key != ([32]byte{}) && payload != ([32]byte{}) && field != ([32]byte{})
}
func validCustomerStateTime(value time.Time, stored bool) bool {
	return !value.IsZero() && (!stored || value.Location() == time.UTC && value.Equal(value.UTC().Truncate(time.Microsecond)))
}
func validCustomerStatusSnapshot(v contact.HistoricalCustomerStatusSnapshot, stored bool) bool {
	return validCustomerStateIdentity(v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, stored) && v.SetByUserIDDigest != ([32]byte{}) && v.WeComTagSyncErrorHash != ([32]byte{}) && v.StatusFlagsDigest != ([32]byte{}) && validCustomerStateTime(v.SetAt, stored) && validCustomerStateTime(v.CreatedAt, stored) && validCustomerStateTime(v.UpdatedAt, stored)
}
func validCustomerStatusChange(v contact.HistoricalCustomerStatusChange, stored bool) bool {
	return validCustomerStateIdentity(v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, stored) && v.SetByUserIDDigest != ([32]byte{}) && v.WeComTagSyncErrorHash != ([32]byte{}) && v.StatusFlagsDigest != ([32]byte{}) && validCustomerStateTime(v.SetAt, stored) && validCustomerStateTime(v.CreatedAt, stored)
}
func validClassTermTagMapping(v contact.HistoricalClassTermTagMapping, stored bool) bool {
	return validCustomerStateIdentity(v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, stored) && validCustomerStateTime(v.CreatedAt, stored) && validCustomerStateTime(v.UpdatedAt, stored)
}
func normalizeCustomerStateTime(v time.Time) time.Time { return v.UTC().Truncate(time.Microsecond) }
func normalizeCustomerStatusSnapshot(v contact.HistoricalCustomerStatusSnapshot) contact.HistoricalCustomerStatusSnapshot {
	v.SetAt, v.CreatedAt, v.UpdatedAt = normalizeCustomerStateTime(v.SetAt), normalizeCustomerStateTime(v.CreatedAt), normalizeCustomerStateTime(v.UpdatedAt)
	return v
}
func normalizeCustomerStatusChange(v contact.HistoricalCustomerStatusChange) contact.HistoricalCustomerStatusChange {
	v.SetAt, v.CreatedAt = normalizeCustomerStateTime(v.SetAt), normalizeCustomerStateTime(v.CreatedAt)
	return v
}
func normalizeClassTermTagMapping(v contact.HistoricalClassTermTagMapping) contact.HistoricalClassTermTagMapping {
	v.CreatedAt, v.UpdatedAt = normalizeCustomerStateTime(v.CreatedAt), normalizeCustomerStateTime(v.UpdatedAt)
	return v
}
func customerStateIdentity(value any) ([32]byte, [32]byte, int64, bool) {
	switch v := value.(type) {
	case contact.HistoricalCustomerStatusSnapshot:
		return v.SourceKeyDigest, v.SourcePayloadDigest, v.ID, true
	case contact.HistoricalCustomerStatusChange:
		return v.SourceKeyDigest, v.SourcePayloadDigest, v.ID, true
	case contact.HistoricalClassTermTagMapping:
		return v.SourceKeyDigest, v.SourcePayloadDigest, v.ID, true
	}
	return [32]byte{}, [32]byte{}, 0, false
}
func validCustomerStateReceipt(v contact.CustomerStateHistoryReceipt, kind, source string, payload [32]byte) bool {
	return v.Kind == kind && v.SourceIdentifier == source && v.PayloadDigest == payload && v.TargetID > 0 && v.TargetDigest != ([32]byte{})
}
func customerStateError(err error) error {
	if errors.Is(err, contact.ErrCustomerStateHistoryInvalid) {
		return contact.ErrCustomerStateHistoryInvalid
	}
	if errors.Is(err, contact.ErrCustomerStateHistoryConflict) {
		return contact.ErrCustomerStateHistoryConflict
	}
	return contact.ErrCustomerStateHistoryUnavailable
}
func nilCustomerState(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return (v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface) && v.IsNil()
}
