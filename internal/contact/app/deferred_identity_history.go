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
	deferredPersonKind   = "deferred_person"
	deferredConflictKind = "deferred_identity_conflict"
	missingRootKind      = "missing_root_identity"
)

// DeferredIdentityHistoryWriter records inert identity evidence only. It does
// not create customers, bind identities, or assign assurance.
type DeferredIdentityHistoryWriter struct {
	store   contact.DeferredIdentityHistoryStore
	journal contact.DeferredIdentityHistoryJournal
}

func NewDeferredIdentityHistoryWriter(store contact.DeferredIdentityHistoryStore, journal contact.DeferredIdentityHistoryJournal) (*DeferredIdentityHistoryWriter, error) {
	if nilDeferredIdentity(store) || nilDeferredIdentity(journal) {
		return nil, contact.ErrDeferredIdentityHistoryUnavailable
	}
	return &DeferredIdentityHistoryWriter{store: store, journal: journal}, nil
}

func (w *DeferredIdentityHistoryWriter) ImportHistoricalDeferredPerson(ctx context.Context, source string, value contact.HistoricalDeferredPerson) (contact.DeferredIdentityHistoryReceipt, error) {
	value = normalizeDeferredPerson(value)
	return importDeferredIdentity(w, ctx, deferredPersonKind, source, value, HistoricalDeferredPersonDigest,
		func(v contact.HistoricalDeferredPerson, id int64) contact.HistoricalDeferredPerson {
			v.ID = id
			return v
		},
		func() (contact.HistoricalDeferredPerson, error) {
			return w.store.CreateHistoricalDeferredPerson(ctx, value)
		},
		func(id int64) (contact.HistoricalDeferredPerson, error) {
			return w.store.GetHistoricalDeferredPerson(ctx, id)
		})
}

func (w *DeferredIdentityHistoryWriter) ImportHistoricalDeferredIdentityConflict(ctx context.Context, source string, value contact.HistoricalDeferredIdentityConflict) (contact.DeferredIdentityHistoryReceipt, error) {
	value = normalizeDeferredConflict(value)
	return importDeferredIdentity(w, ctx, deferredConflictKind, source, value, HistoricalDeferredIdentityConflictDigest,
		func(v contact.HistoricalDeferredIdentityConflict, id int64) contact.HistoricalDeferredIdentityConflict {
			v.ID = id
			return v
		},
		func() (contact.HistoricalDeferredIdentityConflict, error) {
			return w.store.CreateHistoricalDeferredIdentityConflict(ctx, value)
		},
		func(id int64) (contact.HistoricalDeferredIdentityConflict, error) {
			return w.store.GetHistoricalDeferredIdentityConflict(ctx, id)
		})
}

func (w *DeferredIdentityHistoryWriter) ImportHistoricalMissingRootIdentity(ctx context.Context, source string, value contact.HistoricalMissingRootIdentity) (contact.DeferredIdentityHistoryReceipt, error) {
	value = normalizeMissingRoot(value)
	return importDeferredIdentity(w, ctx, missingRootKind, source, value, HistoricalMissingRootIdentityDigest,
		func(v contact.HistoricalMissingRootIdentity, id int64) contact.HistoricalMissingRootIdentity {
			v.ID = id
			return v
		},
		func() (contact.HistoricalMissingRootIdentity, error) {
			return w.store.CreateHistoricalMissingRootIdentity(ctx, value)
		},
		func(id int64) (contact.HistoricalMissingRootIdentity, error) {
			return w.store.GetHistoricalMissingRootIdentity(ctx, id)
		})
}

func importDeferredIdentity[T any](w *DeferredIdentityHistoryWriter, ctx context.Context, kind, source string, value T, digest func(T) ([32]byte, error), withID func(T, int64) T, create func() (T, error), get func(int64) (T, error)) (contact.DeferredIdentityHistoryReceipt, error) {
	var empty contact.DeferredIdentityHistoryReceipt
	if w == nil || ctx == nil || ctx.Err() != nil || nilDeferredIdentity(w.store) || nilDeferredIdentity(w.journal) {
		return empty, contact.ErrDeferredIdentityHistoryUnavailable
	}
	key, payload, id, ok := deferredIdentity(value)
	if !ok || id != 0 || key == ([32]byte{}) || payload == ([32]byte{}) || source != hex.EncodeToString(key[:]) {
		return empty, contact.ErrDeferredIdentityHistoryInvalid
	}
	if _, err := digest(withID(value, 1)); err != nil {
		return empty, contact.ErrDeferredIdentityHistoryInvalid
	}
	receipt, found, err := w.journal.LoadDeferredIdentityHistory(ctx, kind, source)
	if err != nil {
		return empty, deferredIdentityError(err)
	}
	if found {
		if !validDeferredReceipt(receipt, kind, source, payload) {
			return empty, contact.ErrDeferredIdentityHistoryConflict
		}
		actual, err := get(receipt.TargetID)
		if err != nil {
			return empty, deferredIdentityError(err)
		}
		actualDigest, actualErr := digest(actual)
		expectedDigest, expectedErr := digest(withID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, contact.ErrDeferredIdentityHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := create()
	if err != nil {
		return empty, deferredIdentityError(err)
	}
	_, _, targetID, ok := deferredIdentity(actual)
	if !ok || targetID < 1 {
		return empty, contact.ErrDeferredIdentityHistoryConflict
	}
	actualDigest, actualErr := digest(actual)
	expectedDigest, expectedErr := digest(withID(value, targetID))
	if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest {
		return empty, contact.ErrDeferredIdentityHistoryConflict
	}
	receipt = contact.DeferredIdentityHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetDigest: actualDigest, TargetID: targetID}
	if err = w.journal.RecordDeferredIdentityHistory(ctx, receipt); err != nil {
		return empty, deferredIdentityError(err)
	}
	return receipt, nil
}

// Each encoding deliberately names every private field. Marshaling a Port
// value directly would silently omit all json:"-" identity evidence.
func HistoricalDeferredPersonDigest(v contact.HistoricalDeferredPerson) ([32]byte, error) {
	v.RedactedRoots = cloneDeferredRoots(v.RedactedRoots)
	if !validDeferredPerson(v, true) {
		return [32]byte{}, contact.ErrDeferredIdentityHistoryInvalid
	}
	return digestDeferredIdentity(deferredPersonKind, struct {
		ID                                                                 int64
		SourceKey, SourcePayload, SourceField, Mobile, ThirdParty, Private [32]byte
		SourceID                                                           int64
		Roots                                                              []string
		CreatedAt, UpdatedAt                                               time.Time
	}{v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.MobileDigest, v.ThirdPartyUserIDDigest, v.PrivateDigest, v.SourceID, v.RedactedRoots, v.CreatedAt, v.UpdatedAt})
}

func HistoricalDeferredIdentityConflictDigest(v contact.HistoricalDeferredIdentityConflict) ([32]byte, error) {
	v.RedactedRoots = cloneDeferredRoots(v.RedactedRoots)
	if !validDeferredConflict(v, true) {
		return [32]byte{}, contact.ErrDeferredIdentityHistoryInvalid
	}
	return digestDeferredIdentity(deferredConflictKind, struct {
		ID                                                                                                                int64
		SourceKey, SourcePayload, SourceField, UnionID, CandidateUnionID, ExternalUserID, OpenID, Mobile, LegacySourceKey [32]byte
		PayloadJSON, SourcePayloadJSON, ResolutionNote, Private                                                           [32]byte
		SourceID                                                                                                          int64
		ConflictType, SourceType, Status, ResolutionStatus                                                                string
		Roots                                                                                                             []string
		CreatedAt, UpdatedAt                                                                                              time.Time
		ResolvedAt                                                                                                        *time.Time
	}{v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.UnionIDDigest, v.CandidateUnionIDDigest, v.ExternalUserIDDigest, v.OpenIDDigest, v.MobileDigest, v.LegacySourceKeyDigest, v.PayloadJSONDigest, v.SourcePayloadJSONDigest, v.ResolutionNoteDigest, v.PrivateDigest, v.SourceID, v.ConflictType, v.SourceType, v.Status, v.ResolutionStatus, v.RedactedRoots, v.CreatedAt, v.UpdatedAt, v.ResolvedAt})
}

func HistoricalMissingRootIdentityDigest(v contact.HistoricalMissingRootIdentity) ([32]byte, error) {
	v.RedactedRoots = cloneDeferredRoots(v.RedactedRoots)
	if !validMissingRoot(v, true) {
		return [32]byte{}, contact.ErrDeferredIdentityHistoryInvalid
	}
	return digestDeferredIdentity(missingRootKind, struct {
		ID                                                                                            int64
		SourceKey, SourcePayload, SourceField, DM01SourceKey, CorpID, ExternalUserID, UnionID, OpenID [32]byte
		FollowUserID, Name, Avatar, RawProfile, Private                                               [32]byte
		Gender                                                                                        *[32]byte
		SourceID, DM01RunID                                                                           int64
		DM01SourceHMACKeyVersion, QuarantineReason, Status                                            string
		Type                                                                                          *int32
		Roots                                                                                         []string
		FirstSeenAt, LastSeenAt, CreatedAt, UpdatedAt                                                 time.Time
	}{v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.DM01SourceKeyDigest, v.CorpIDDigest, v.ExternalUserIDDigest, v.UnionIDDigest, v.OpenIDDigest, v.FollowUserIDDigest, v.NameDigest, v.AvatarDigest, v.RawProfileDigest, v.PrivateDigest, v.GenderDigest, v.SourceID, v.DM01RunID, v.DM01SourceHMACKeyVersion, v.QuarantineReason, v.Status, v.Type, v.RedactedRoots, v.FirstSeenAt, v.LastSeenAt, v.CreatedAt, v.UpdatedAt})
}

func digestDeferredIdentity(kind string, value any) ([32]byte, error) {
	encoded, err := json.Marshal(struct {
		Kind  string `json:"kind"`
		Value any    `json:"value"`
	}{kind, value})
	if err != nil {
		return [32]byte{}, contact.ErrDeferredIdentityHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func validDeferredIdentity(id int64, key, payload, field [32]byte, stored bool) bool {
	return (stored && id > 0 || !stored && id == 0) && allDeferredDigests(key, payload, field)
}

func allDeferredDigests(values ...[32]byte) bool {
	for _, value := range values {
		if value == ([32]byte{}) {
			return false
		}
	}
	return true
}

func validDeferredTime(value time.Time, stored bool) bool {
	return !value.IsZero() && (!stored || value.Location() == time.UTC && value.Equal(value.UTC().Truncate(time.Microsecond)))
}

func validDeferredText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}

func validDeferredRoots(values []string) bool {
	for _, value := range values {
		if !validDeferredText(value) {
			return false
		}
	}
	return true
}

func validDeferredPerson(v contact.HistoricalDeferredPerson, stored bool) bool {
	return validDeferredIdentity(v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, stored) &&
		allDeferredDigests(v.MobileDigest, v.ThirdPartyUserIDDigest, v.PrivateDigest) &&
		validDeferredRoots(v.RedactedRoots) && validDeferredTime(v.CreatedAt, stored) && validDeferredTime(v.UpdatedAt, stored)
}

func validDeferredConflict(v contact.HistoricalDeferredIdentityConflict, stored bool) bool {
	return validDeferredIdentity(v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, stored) &&
		allDeferredDigests(v.UnionIDDigest, v.CandidateUnionIDDigest, v.ExternalUserIDDigest, v.OpenIDDigest, v.MobileDigest, v.LegacySourceKeyDigest, v.PayloadJSONDigest, v.SourcePayloadJSONDigest, v.ResolutionNoteDigest, v.PrivateDigest) &&
		validDeferredText(v.ConflictType) && validDeferredText(v.SourceType) && validDeferredText(v.Status) && validDeferredText(v.ResolutionStatus) && validDeferredRoots(v.RedactedRoots) &&
		validDeferredTime(v.CreatedAt, stored) && validDeferredTime(v.UpdatedAt, stored) && (v.ResolvedAt == nil || validDeferredTime(*v.ResolvedAt, stored))
}

func validMissingRoot(v contact.HistoricalMissingRootIdentity, stored bool) bool {
	if !validDeferredIdentity(v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, stored) ||
		v.DM01RunID < 1 || v.DM01SourceHMACKeyVersion == "" || !validDeferredText(v.DM01SourceHMACKeyVersion) || v.QuarantineReason != "missing_customer_root" || !validDeferredText(v.QuarantineReason) || !validDeferredText(v.Status) || !validDeferredRoots(v.RedactedRoots) ||
		!allDeferredDigests(v.DM01SourceKeyDigest, v.CorpIDDigest, v.ExternalUserIDDigest, v.UnionIDDigest, v.OpenIDDigest, v.FollowUserIDDigest, v.NameDigest, v.AvatarDigest, v.RawProfileDigest, v.PrivateDigest) {
		return false
	}
	if v.GenderDigest != nil && *v.GenderDigest == ([32]byte{}) {
		return false
	}
	return validDeferredTime(v.FirstSeenAt, stored) && validDeferredTime(v.LastSeenAt, stored) && validDeferredTime(v.CreatedAt, stored) && validDeferredTime(v.UpdatedAt, stored)
}

func normalizeDeferredTime(value time.Time) time.Time { return value.UTC().Truncate(time.Microsecond) }

func normalizeDeferredPerson(v contact.HistoricalDeferredPerson) contact.HistoricalDeferredPerson {
	v.RedactedRoots = cloneDeferredRoots(v.RedactedRoots)
	v.CreatedAt, v.UpdatedAt = normalizeDeferredTime(v.CreatedAt), normalizeDeferredTime(v.UpdatedAt)
	return v
}

func normalizeDeferredConflict(v contact.HistoricalDeferredIdentityConflict) contact.HistoricalDeferredIdentityConflict {
	v.RedactedRoots = cloneDeferredRoots(v.RedactedRoots)
	v.CreatedAt, v.UpdatedAt, v.ResolvedAt = normalizeDeferredTime(v.CreatedAt), normalizeDeferredTime(v.UpdatedAt), cloneDeferredTime(v.ResolvedAt)
	return v
}

func normalizeMissingRoot(v contact.HistoricalMissingRootIdentity) contact.HistoricalMissingRootIdentity {
	v.RedactedRoots = cloneDeferredRoots(v.RedactedRoots)
	v.Type, v.GenderDigest = cloneDeferredInt32(v.Type), cloneDeferredDigest(v.GenderDigest)
	v.FirstSeenAt, v.LastSeenAt, v.CreatedAt, v.UpdatedAt = normalizeDeferredTime(v.FirstSeenAt), normalizeDeferredTime(v.LastSeenAt), normalizeDeferredTime(v.CreatedAt), normalizeDeferredTime(v.UpdatedAt)
	return v
}

func cloneDeferredRoots(value []string) []string {
	cloned := make([]string, len(value))
	copy(cloned, value)
	return cloned
}
func cloneDeferredTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := normalizeDeferredTime(*value)
	return &copied
}
func cloneDeferredInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
func cloneDeferredDigest(value *[32]byte) *[32]byte {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func deferredIdentity(value any) ([32]byte, [32]byte, int64, bool) {
	switch v := value.(type) {
	case contact.HistoricalDeferredPerson:
		return v.SourceKeyDigest, v.SourcePayloadDigest, v.ID, true
	case contact.HistoricalDeferredIdentityConflict:
		return v.SourceKeyDigest, v.SourcePayloadDigest, v.ID, true
	case contact.HistoricalMissingRootIdentity:
		return v.SourceKeyDigest, v.SourcePayloadDigest, v.ID, true
	}
	return [32]byte{}, [32]byte{}, 0, false
}

func validDeferredReceipt(v contact.DeferredIdentityHistoryReceipt, kind, source string, payload [32]byte) bool {
	return v.Kind == kind && v.SourceIdentifier == source && v.PayloadDigest == payload && v.TargetID > 0 && v.TargetDigest != ([32]byte{})
}

func deferredIdentityError(err error) error {
	if errors.Is(err, contact.ErrDeferredIdentityHistoryInvalid) {
		return contact.ErrDeferredIdentityHistoryInvalid
	}
	if errors.Is(err, contact.ErrDeferredIdentityHistoryConflict) {
		return contact.ErrDeferredIdentityHistoryConflict
	}
	return contact.ErrDeferredIdentityHistoryUnavailable
}

func nilDeferredIdentity(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return (v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface) && v.IsNil()
}
