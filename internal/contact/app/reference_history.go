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
	externalContactBindingKind = "external_contact_binding"
	wecomDirectoryMemberKind   = "wecom_directory_member"
)

// ReferenceHistoryWriter stores inert source references only. It never binds a
// customer, creates staff, or changes an identity or permission.
type ReferenceHistoryWriter struct {
	store   contact.ReferenceHistoryStore
	journal contact.ReferenceHistoryJournal
}

func NewReferenceHistoryWriter(store contact.ReferenceHistoryStore, journal contact.ReferenceHistoryJournal) (*ReferenceHistoryWriter, error) {
	if nilReferenceHistory(store) || nilReferenceHistory(journal) {
		return nil, contact.ErrReferenceHistoryUnavailable
	}
	return &ReferenceHistoryWriter{store: store, journal: journal}, nil
}

func (w *ReferenceHistoryWriter) ImportHistoricalExternalContactBinding(ctx context.Context, source string, value contact.HistoricalExternalContactBinding) (contact.ReferenceHistoryReceipt, error) {
	value = normalizeExternalContactBinding(value)
	return importReferenceHistory(w, ctx, externalContactBindingKind, source, value, HistoricalExternalContactBindingDigest,
		func(v contact.HistoricalExternalContactBinding, id int64) contact.HistoricalExternalContactBinding {
			v.ID = id
			return v
		},
		func() (contact.HistoricalExternalContactBinding, error) {
			return w.store.CreateHistoricalExternalContactBinding(ctx, value)
		},
		func(id int64) (contact.HistoricalExternalContactBinding, error) {
			return w.store.GetHistoricalExternalContactBinding(ctx, id)
		})
}

func (w *ReferenceHistoryWriter) ImportHistoricalWeComDirectoryMember(ctx context.Context, source string, value contact.HistoricalWeComDirectoryMember) (contact.ReferenceHistoryReceipt, error) {
	value = normalizeWeComDirectoryMember(value)
	return importReferenceHistory(w, ctx, wecomDirectoryMemberKind, source, value, HistoricalWeComDirectoryMemberDigest,
		func(v contact.HistoricalWeComDirectoryMember, id int64) contact.HistoricalWeComDirectoryMember {
			v.ID = id
			return v
		},
		func() (contact.HistoricalWeComDirectoryMember, error) {
			return w.store.CreateHistoricalWeComDirectoryMember(ctx, value)
		},
		func(id int64) (contact.HistoricalWeComDirectoryMember, error) {
			return w.store.GetHistoricalWeComDirectoryMember(ctx, id)
		})
}

func importReferenceHistory[T any](w *ReferenceHistoryWriter, ctx context.Context, kind, source string, value T, digest func(T) ([32]byte, error), withID func(T, int64) T, create func() (T, error), get func(int64) (T, error)) (contact.ReferenceHistoryReceipt, error) {
	var empty contact.ReferenceHistoryReceipt
	if w == nil || ctx == nil || ctx.Err() != nil || nilReferenceHistory(w.store) || nilReferenceHistory(w.journal) {
		return empty, contact.ErrReferenceHistoryUnavailable
	}
	key, payload, id, ok := referenceHistoryIdentity(value)
	if !ok || id != 0 || key == ([32]byte{}) || payload == ([32]byte{}) || source != hex.EncodeToString(key[:]) {
		return empty, contact.ErrReferenceHistoryInvalid
	}
	if _, err := digest(withID(value, 1)); err != nil {
		return empty, contact.ErrReferenceHistoryInvalid
	}
	receipt, found, err := w.journal.LoadReferenceHistory(ctx, kind, source)
	if err != nil {
		return empty, referenceHistoryError(err)
	}
	if found {
		if !validReferenceReceipt(receipt, kind, source, payload) {
			return empty, contact.ErrReferenceHistoryConflict
		}
		actual, err := get(receipt.TargetID)
		if err != nil {
			return empty, referenceHistoryError(err)
		}
		actualDigest, actualErr := digest(actual)
		expectedDigest, expectedErr := digest(withID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, contact.ErrReferenceHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := create()
	if err != nil {
		return empty, referenceHistoryError(err)
	}
	_, _, targetID, ok := referenceHistoryIdentity(actual)
	if !ok || targetID < 1 {
		return empty, contact.ErrReferenceHistoryConflict
	}
	actualDigest, actualErr := digest(actual)
	expectedDigest, expectedErr := digest(withID(value, targetID))
	if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest {
		return empty, contact.ErrReferenceHistoryConflict
	}
	receipt = contact.ReferenceHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetDigest: actualDigest, TargetID: targetID}
	if err = w.journal.RecordReferenceHistory(ctx, receipt); err != nil {
		return empty, referenceHistoryError(err)
	}
	return receipt, nil
}

// Digest encodings list every private source fact. Marshaling Port values
// directly would omit json:"-" fields and allow reference evidence to drift.
func HistoricalExternalContactBindingDigest(v contact.HistoricalExternalContactBinding) ([32]byte, error) {
	if !validExternalContactBinding(v, true) {
		return [32]byte{}, contact.ErrReferenceHistoryInvalid
	}
	return digestReferenceHistory(externalContactBindingKind, struct {
		ID                                                                          int64
		SourceKey, SourcePayload, SourceField, ExternalUser, FirstBound, FirstOwner [32]byte
		LastOwner                                                                   [32]byte
		SourcePersonID                                                              int64
		PersonHistoryID, IdentityID                                                 *int64
		IdentityAssurance                                                           string
		CreatedAt, UpdatedAt                                                        time.Time
	}{v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.ExternalUserIDDigest, v.FirstBoundByUserIDDigest, v.FirstOwnerUserIDDigest, v.LastOwnerUserIDDigest, v.SourcePersonID, v.PersonHistoryID, v.IdentityID, v.IdentityAssurance, v.CreatedAt, v.UpdatedAt})
}

func HistoricalWeComDirectoryMemberDigest(v contact.HistoricalWeComDirectoryMember) ([32]byte, error) {
	if !validWeComDirectoryMember(v, true) {
		return [32]byte{}, contact.ErrReferenceHistoryInvalid
	}
	return digestReferenceHistory(wecomDirectoryMemberKind, struct {
		ID                                                                             int64
		SourceKey, SourcePayload, SourceField, WeComCorp, Corp, WeComUser, Departments [32]byte
		RawPayload, Mobile, AvatarURL, UpdatedBy                                       [32]byte
		SourceID                                                                       int64
		CorpAttribution, DisplayName, DepartmentName, Position                         string
		MatchedStaffID                                                                 *int64
		WeComStatus                                                                    *int32
		IsActive                                                                       bool
		SyncedAt, FirstSeenAt, LastSyncedAt, CreatedAt, UpdatedAt                      time.Time
	}{v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.WeComCorpIDDigest, v.CorpIDDigest, v.WeComUserIDDigest, v.DepartmentIDsDigest, v.RawPayloadDigest, v.MobileDigest, v.AvatarURLDigest, v.UpdatedByDigest, v.SourceID, v.CorpAttribution, v.DisplayName, v.DepartmentName, v.Position, v.MatchedStaffID, v.WeComStatus, v.IsActive, v.SyncedAt, v.FirstSeenAt, v.LastSyncedAt, v.CreatedAt, v.UpdatedAt})
}

func digestReferenceHistory(kind string, value any) ([32]byte, error) {
	encoded, err := json.Marshal(struct {
		Kind  string
		Value any
	}{kind, value})
	if err != nil {
		return [32]byte{}, contact.ErrReferenceHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func validExternalContactBinding(v contact.HistoricalExternalContactBinding, stored bool) bool {
	if !validReferenceIdentity(v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, stored) ||
		!allReferenceDigests(v.ExternalUserIDDigest, v.FirstBoundByUserIDDigest, v.FirstOwnerUserIDDigest, v.LastOwnerUserIDDigest) ||
		!validReferenceText(v.IdentityAssurance) || !validReferenceTime(v.CreatedAt, stored) || !validReferenceTime(v.UpdatedAt, stored) {
		return false
	}
	if v.PersonHistoryID != nil && *v.PersonHistoryID < 1 || v.IdentityID != nil && *v.IdentityID < 1 {
		return false
	}
	return (v.IdentityID == nil && v.IdentityAssurance == "unresolved") || (v.IdentityID != nil && (v.IdentityAssurance == "declared" || v.IdentityAssurance == "verified"))
}

func validWeComDirectoryMember(v contact.HistoricalWeComDirectoryMember, stored bool) bool {
	if !validReferenceIdentity(v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, stored) ||
		!allReferenceDigests(v.WeComCorpIDDigest, v.CorpIDDigest, v.WeComUserIDDigest, v.DepartmentIDsDigest, v.RawPayloadDigest, v.MobileDigest, v.AvatarURLDigest, v.UpdatedByDigest) ||
		!validReferenceText(v.CorpAttribution) || !validReferenceText(v.DisplayName) || !validReferenceText(v.DepartmentName) || !validReferenceText(v.Position) ||
		!validReferenceTime(v.SyncedAt, stored) || !validReferenceTime(v.FirstSeenAt, stored) || !validReferenceTime(v.LastSyncedAt, stored) || !validReferenceTime(v.CreatedAt, stored) || !validReferenceTime(v.UpdatedAt, stored) {
		return false
	}
	if v.MatchedStaffID != nil && *v.MatchedStaffID < 1 {
		return false
	}
	return (v.CorpAttribution == "matched") || (v.CorpAttribution == "unattributable" && v.MatchedStaffID == nil)
}

func validReferenceIdentity(id int64, key, payload, field [32]byte, stored bool) bool {
	return (stored && id > 0 || !stored && id == 0) && allReferenceDigests(key, payload, field)
}

func allReferenceDigests(values ...[32]byte) bool {
	for _, value := range values {
		if value == ([32]byte{}) {
			return false
		}
	}
	return true
}

func validReferenceText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}

func validReferenceTime(value time.Time, stored bool) bool {
	return !value.IsZero() && (!stored || value.Location() == time.UTC && value.Equal(value.UTC().Truncate(time.Microsecond)))
}

func normalizeReferenceTime(value time.Time) time.Time { return value.UTC().Truncate(time.Microsecond) }

func normalizeExternalContactBinding(v contact.HistoricalExternalContactBinding) contact.HistoricalExternalContactBinding {
	v.PersonHistoryID, v.IdentityID = cloneReferenceInt64(v.PersonHistoryID), cloneReferenceInt64(v.IdentityID)
	v.CreatedAt, v.UpdatedAt = normalizeReferenceTime(v.CreatedAt), normalizeReferenceTime(v.UpdatedAt)
	return v
}

func normalizeWeComDirectoryMember(v contact.HistoricalWeComDirectoryMember) contact.HistoricalWeComDirectoryMember {
	v.MatchedStaffID, v.WeComStatus = cloneReferenceInt64(v.MatchedStaffID), cloneReferenceInt32(v.WeComStatus)
	v.SyncedAt, v.FirstSeenAt, v.LastSyncedAt, v.CreatedAt, v.UpdatedAt = normalizeReferenceTime(v.SyncedAt), normalizeReferenceTime(v.FirstSeenAt), normalizeReferenceTime(v.LastSyncedAt), normalizeReferenceTime(v.CreatedAt), normalizeReferenceTime(v.UpdatedAt)
	return v
}

func cloneReferenceInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneReferenceInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func referenceHistoryIdentity(value any) ([32]byte, [32]byte, int64, bool) {
	switch v := value.(type) {
	case contact.HistoricalExternalContactBinding:
		return v.SourceKeyDigest, v.SourcePayloadDigest, v.ID, true
	case contact.HistoricalWeComDirectoryMember:
		return v.SourceKeyDigest, v.SourcePayloadDigest, v.ID, true
	}
	return [32]byte{}, [32]byte{}, 0, false
}

func validReferenceReceipt(v contact.ReferenceHistoryReceipt, kind, source string, payload [32]byte) bool {
	return v.Kind == kind && v.SourceIdentifier == source && v.PayloadDigest == payload && v.TargetID > 0 && v.TargetDigest != ([32]byte{})
}

func referenceHistoryError(err error) error {
	if errors.Is(err, contact.ErrReferenceHistoryInvalid) {
		return contact.ErrReferenceHistoryInvalid
	}
	if errors.Is(err, contact.ErrReferenceHistoryConflict) {
		return contact.ErrReferenceHistoryConflict
	}
	return contact.ErrReferenceHistoryUnavailable
}

func nilReferenceHistory(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return (v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface) && v.IsNil()
}
