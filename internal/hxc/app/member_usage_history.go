package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	hxc "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
)

// HXCMemberUsageHistoryWriter preserves generation observations only. It never
// derives current membership, owner, Customer, or Staff state.
type HXCMemberUsageHistoryWriter struct {
	store   hxc.HXCMemberUsageHistoryStore
	journal hxc.HXCHistoryJournal
}

func NewHXCMemberUsageHistoryWriter(store hxc.HXCMemberUsageHistoryStore, journal hxc.HXCHistoryJournal) (*HXCMemberUsageHistoryWriter, error) {
	if nilHXC(store) || nilHXC(journal) {
		return nil, hxc.ErrHXCHistoryUnavailable
	}
	return &HXCMemberUsageHistoryWriter{store: store, journal: journal}, nil
}

func (w *HXCMemberUsageHistoryWriter) ImportMemberUsage(ctx context.Context, source string, value hxc.HistoricalHXCMemberUsage) (hxc.HXCHistoryReceipt, error) {
	var empty hxc.HXCHistoryReceipt
	if w == nil || ctx == nil || ctx.Err() != nil || nilHXC(w.store) || nilHXC(w.journal) {
		return empty, hxc.ErrHXCHistoryUnavailable
	}
	value = normalizeHXCMemberUsage(value)
	if !validHXCMemberUsage(value, false) || source != hex.EncodeToString(value.SourceKeyDigest[:]) {
		return empty, hxc.ErrHXCHistoryInvalid
	}
	if _, err := HistoricalHXCMemberUsageDigest(withHXCMemberUsageID(value, 1)); err != nil {
		return empty, hxc.ErrHXCHistoryInvalid
	}
	receipt, found, err := w.journal.LoadHXCHistory(ctx, hxc.HXCHistoryMemberUsage, source)
	if err != nil {
		return empty, hxcHistoryError(err)
	}
	if found {
		if !validReceipt(receipt, hxc.HXCHistoryMemberUsage, source, value.SourcePayloadDigest) {
			return empty, hxc.ErrHXCHistoryConflict
		}
		actual, err := w.store.GetHistoricalHXCMemberUsage(ctx, receipt.TargetID)
		if err != nil {
			return empty, hxcHistoryError(err)
		}
		actualDigest, actualErr := HistoricalHXCMemberUsageDigest(actual)
		expectedDigest, expectedErr := HistoricalHXCMemberUsageDigest(withHXCMemberUsageID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, hxc.ErrHXCHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := w.store.CreateHistoricalHXCMemberUsage(ctx, value)
	if err != nil {
		return empty, hxcHistoryError(err)
	}
	if actual.ID < 1 {
		return empty, hxc.ErrHXCHistoryConflict
	}
	actualDigest, actualErr := HistoricalHXCMemberUsageDigest(actual)
	expectedDigest, expectedErr := HistoricalHXCMemberUsageDigest(withHXCMemberUsageID(value, actual.ID))
	if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest {
		return empty, hxc.ErrHXCHistoryConflict
	}
	receipt = hxc.HXCHistoryReceipt{
		Kind:             hxc.HXCHistoryMemberUsage,
		SourceIdentifier: source,
		PayloadDigest:    value.SourcePayloadDigest,
		TargetID:         actual.ID,
		TargetDigest:     actualDigest,
	}
	if err := w.journal.RecordHXCHistory(ctx, receipt); err != nil {
		return empty, hxcHistoryError(err)
	}
	return receipt, nil
}

func HistoricalHXCMemberUsageDigest(value hxc.HistoricalHXCMemberUsage) ([32]byte, error) {
	if !validHXCMemberUsage(value, true) {
		return [32]byte{}, hxc.ErrHXCHistoryInvalid
	}
	encoded, err := json.Marshal(struct {
		Kind  string                    `json:"kind"`
		Value hxcMemberUsageDigestValue `json:"value"`
	}{Kind: hxc.HXCHistoryMemberUsage, Value: hxcMemberUsageDigestValue{
		ID: value.ID, SourceKeyDigest: value.SourceKeyDigest, SourcePayloadDigest: value.SourcePayloadDigest, SourceFieldDigest: value.SourceFieldDigest,
		Generation: value.Generation, UnionID: value.UnionID, OwnerUserID: value.OwnerUserID, MobileHash: value.MobileHash,
		IsMember: value.IsMember, IsRegistered: value.IsRegistered, HasRealUsage: value.HasRealUsage,
		RegisteredAt: value.RegisteredAt, FirstUsedAt: value.FirstUsedAt, LastUsedAt: value.LastUsedAt, MemberSince: value.MemberSince, MembershipExpiresAt: value.MembershipExpiresAt,
		MembershipTier: value.MembershipTier, MembershipStatus: value.MembershipStatus, MembershipSource: value.MembershipSource, RegistrationSource: value.RegistrationSource, UsageSource: value.UsageSource,
		UpdatedAt: value.UpdatedAt, PayloadJSON: string(value.PayloadJSON), ProjectedAt: value.ProjectedAt,
	}})
	if err != nil {
		return [32]byte{}, hxc.ErrHXCHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

type hxcMemberUsageDigestValue struct {
	ID                                                                                  int64
	SourceKeyDigest, SourcePayloadDigest, SourceFieldDigest                             [32]byte
	Generation                                                                          int64
	UnionID, OwnerUserID, MobileHash                                                    string
	IsMember, IsRegistered, HasRealUsage                                                bool
	RegisteredAt, FirstUsedAt, LastUsedAt, MemberSince, MembershipExpiresAt             *time.Time
	MembershipTier, MembershipStatus, MembershipSource, RegistrationSource, UsageSource string
	UpdatedAt                                                                           *time.Time
	PayloadJSON                                                                         string
	ProjectedAt                                                                         time.Time
}

func normalizeHXCMemberUsage(value hxc.HistoricalHXCMemberUsage) hxc.HistoricalHXCMemberUsage {
	value.RegisteredAt = normalizePTime(value.RegisteredAt)
	value.FirstUsedAt = normalizePTime(value.FirstUsedAt)
	value.LastUsedAt = normalizePTime(value.LastUsedAt)
	value.MemberSince = normalizePTime(value.MemberSince)
	value.MembershipExpiresAt = normalizePTime(value.MembershipExpiresAt)
	value.UpdatedAt = normalizePTime(value.UpdatedAt)
	value.ProjectedAt = normalizeTime(value.ProjectedAt)
	value.PayloadJSON = append(json.RawMessage(nil), value.PayloadJSON...)
	return value
}

func withHXCMemberUsageID(value hxc.HistoricalHXCMemberUsage, id int64) hxc.HistoricalHXCMemberUsage {
	value.ID = id
	return value
}

func validHXCMemberUsage(value hxc.HistoricalHXCMemberUsage, stored bool) bool {
	if (stored && value.ID < 1 || !stored && value.ID != 0) || value.SourceKeyDigest == ([32]byte{}) || value.SourcePayloadDigest == ([32]byte{}) || value.SourceFieldDigest == ([32]byte{}) || !validHXCMemberUsageText(value.UnionID, value.OwnerUserID, value.MobileHash, value.MembershipTier, value.MembershipStatus, value.MembershipSource, value.RegistrationSource, value.UsageSource) || len(value.PayloadJSON) == 0 || !json.Valid(value.PayloadJSON) || !validTime(value.ProjectedAt, stored) {
		return false
	}
	for _, value := range []*time.Time{value.RegisteredAt, value.FirstUsedAt, value.LastUsedAt, value.MemberSince, value.MembershipExpiresAt, value.UpdatedAt} {
		if !validOptionalTime(value, stored) {
			return false
		}
	}
	return true
}

func validHXCMemberUsageText(values ...string) bool {
	for _, value := range values {
		if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return false
		}
	}
	return true
}
