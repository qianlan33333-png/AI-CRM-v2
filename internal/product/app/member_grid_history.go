package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

// MemberGridHistoryWriter writes only immutable V1 history in the caller's
// transaction. It has no current Member Grid, permission, share, event, or
// Provider dependency.
type MemberGridHistoryWriter struct {
	store   productport.MemberGridHistoryStore
	journal productport.MemberGridHistoryJournal
}

func NewMemberGridHistoryWriter(store productport.MemberGridHistoryStore, journal productport.MemberGridHistoryJournal) *MemberGridHistoryWriter {
	return &MemberGridHistoryWriter{store: store, journal: journal}
}

func (writer *MemberGridHistoryWriter) WriteMemberView(ctx context.Context, sourceIdentifier string, payloadDigest [sha256.Size]byte, value productport.HistoricalMemberView) (productport.MemberGridHistoryReceipt, error) {
	empty := productport.MemberGridHistoryReceipt{}
	if !memberGridHistoryReady(writer, ctx) {
		return empty, productport.ErrMemberGridHistoryUnavailable
	}
	sourceKey, ok := memberGridHistorySourceKey(sourceIdentifier)
	if !ok || payloadDigest == ([sha256.Size]byte{}) || value.ID != 0 || value.SourceKeyDigest != sourceKey || value.SourcePayloadDigest != payloadDigest || !validHistoricalMemberView(value, false) {
		return empty, productport.ErrMemberGridHistoryInvalid
	}
	value = normalizeHistoricalMemberView(value)
	if _, err := HistoricalMemberViewDigest(withHistoricalMemberViewID(value, 1)); err != nil {
		return empty, productport.ErrMemberGridHistoryInvalid
	}

	receipt, found, err := writer.journal.LoadMemberGridHistory(ctx, productport.MemberGridHistoryView, sourceIdentifier)
	if err != nil {
		return empty, memberGridHistoryError(err)
	}
	if found {
		if !validMemberGridHistoryReceipt(receipt, productport.MemberGridHistoryView, sourceIdentifier, payloadDigest) {
			return empty, productport.ErrMemberGridHistoryConflict
		}
		actual, getErr := writer.store.GetHistoricalMemberView(ctx, receipt.TargetID)
		if getErr != nil {
			return empty, memberGridHistoryError(getErr)
		}
		expected := withHistoricalMemberViewID(value, receipt.TargetID)
		actualDigest, actualErr := HistoricalMemberViewDigest(actual)
		expectedDigest, expectedErr := HistoricalMemberViewDigest(expected)
		if actualErr != nil || expectedErr != nil || actual.ID != receipt.TargetID || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, productport.ErrMemberGridHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}

	actual, err := writer.store.CreateHistoricalMemberView(ctx, value)
	if err != nil {
		return empty, memberGridHistoryError(err)
	}
	actualDigest, actualErr := HistoricalMemberViewDigest(actual)
	expectedDigest, expectedErr := HistoricalMemberViewDigest(withHistoricalMemberViewID(value, actual.ID))
	if actualErr != nil || expectedErr != nil || actual.ID < 1 || actualDigest != expectedDigest {
		return empty, productport.ErrMemberGridHistoryConflict
	}
	receipt = productport.MemberGridHistoryReceipt{Kind: productport.MemberGridHistoryView, SourceIdentifier: sourceIdentifier, PayloadDigest: payloadDigest, TargetID: actual.ID, TargetDigest: actualDigest}
	if err = writer.journal.RecordMemberGridHistory(ctx, receipt); err != nil {
		return empty, memberGridHistoryError(err)
	}
	return receipt, nil
}

func (writer *MemberGridHistoryWriter) WriteMemberUsage(ctx context.Context, sourceIdentifier string, payloadDigest [sha256.Size]byte, value productport.HistoricalMemberUsage) (productport.MemberGridHistoryReceipt, error) {
	empty := productport.MemberGridHistoryReceipt{}
	if !memberGridHistoryReady(writer, ctx) {
		return empty, productport.ErrMemberGridHistoryUnavailable
	}
	sourceKey, ok := memberGridHistorySourceKey(sourceIdentifier)
	if !ok || payloadDigest == ([sha256.Size]byte{}) || value.ID != 0 || value.SourceKeyDigest != sourceKey || value.SourcePayloadDigest != payloadDigest || !validHistoricalMemberUsage(value, false) {
		return empty, productport.ErrMemberGridHistoryInvalid
	}
	value = normalizeHistoricalMemberUsage(value)
	if _, err := HistoricalMemberUsageDigest(withHistoricalMemberUsageID(value, 1)); err != nil {
		return empty, productport.ErrMemberGridHistoryInvalid
	}

	receipt, found, err := writer.journal.LoadMemberGridHistory(ctx, productport.MemberGridHistoryUsage, sourceIdentifier)
	if err != nil {
		return empty, memberGridHistoryError(err)
	}
	if found {
		if !validMemberGridHistoryReceipt(receipt, productport.MemberGridHistoryUsage, sourceIdentifier, payloadDigest) {
			return empty, productport.ErrMemberGridHistoryConflict
		}
		actual, getErr := writer.store.GetHistoricalMemberUsage(ctx, receipt.TargetID)
		if getErr != nil {
			return empty, memberGridHistoryError(getErr)
		}
		expected := withHistoricalMemberUsageID(value, receipt.TargetID)
		actualDigest, actualErr := HistoricalMemberUsageDigest(actual)
		expectedDigest, expectedErr := HistoricalMemberUsageDigest(expected)
		if actualErr != nil || expectedErr != nil || actual.ID != receipt.TargetID || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, productport.ErrMemberGridHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}

	actual, err := writer.store.CreateHistoricalMemberUsage(ctx, value)
	if err != nil {
		return empty, memberGridHistoryError(err)
	}
	actualDigest, actualErr := HistoricalMemberUsageDigest(actual)
	expectedDigest, expectedErr := HistoricalMemberUsageDigest(withHistoricalMemberUsageID(value, actual.ID))
	if actualErr != nil || expectedErr != nil || actual.ID < 1 || actualDigest != expectedDigest {
		return empty, productport.ErrMemberGridHistoryConflict
	}
	receipt = productport.MemberGridHistoryReceipt{Kind: productport.MemberGridHistoryUsage, SourceIdentifier: sourceIdentifier, PayloadDigest: payloadDigest, TargetID: actual.ID, TargetDigest: actualDigest}
	if err = writer.journal.RecordMemberGridHistory(ctx, receipt); err != nil {
		return empty, memberGridHistoryError(err)
	}
	return receipt, nil
}

// HistoricalMemberViewDigest covers every stored typed field, including the
// generated target ID. Its input must already match PostgreSQL UTC microsecond
// representation so target drift cannot be normalized away during replay.
func HistoricalMemberViewDigest(value productport.HistoricalMemberView) ([sha256.Size]byte, error) {
	if !validHistoricalMemberView(value, true) {
		return [sha256.Size]byte{}, productport.ErrMemberGridHistoryInvalid
	}
	encoded, err := json.Marshal(struct {
		Kind                                                        string `json:"kind"`
		ID, SourceViewID, SourceServiceProductID, Position, Version int64
		SourceKeyDigest, ConfigDigest, SourcePayloadDigest          [sha256.Size]byte
		ProductID                                                   *int64
		Name                                                        string
		IsDefault                                                   bool
		SchemaVersion                                               int16
		CreatedAt, UpdatedAt                                        string
	}{
		Kind: "v1.member_grid_view", ID: value.ID, SourceViewID: value.SourceViewID, SourceServiceProductID: value.SourceServiceProductID,
		Position: value.Position, Version: value.Version, SourceKeyDigest: value.SourceKeyDigest, ConfigDigest: value.ConfigDigest,
		SourcePayloadDigest: value.SourcePayloadDigest, ProductID: value.ProductID, Name: value.Name, IsDefault: value.IsDefault,
		SchemaVersion: value.SchemaVersion, CreatedAt: memberGridHistoryTime(value.CreatedAt), UpdatedAt: memberGridHistoryTime(value.UpdatedAt),
	})
	if err != nil {
		return [sha256.Size]byte{}, productport.ErrMemberGridHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

// HistoricalMemberUsageDigest covers only historical usage facts and the
// recovery proof; it deliberately represents no current entitlement or login.
func HistoricalMemberUsageDigest(value productport.HistoricalMemberUsage) ([sha256.Size]byte, error) {
	if !validHistoricalMemberUsage(value, true) {
		return [sha256.Size]byte{}, productport.ErrMemberGridHistoryInvalid
	}
	encoded, err := json.Marshal(struct {
		Kind                                                      string `json:"kind"`
		ID, OpenCount7D                                           int64
		SourceKeyDigest, SourcePayloadDigest, RecoveryEntryDigest [sha256.Size]byte
		CustomerID, LearningPlanCurrent, LearningPlanTotal        *int64
		FormallyLoggedIn, HasTokenUsage                           bool
		LearningPlanID                                            string
		LastOpenAt                                                *string
		RefreshedAt                                               string
	}{
		Kind: "v1.member_grid_usage", ID: value.ID, OpenCount7D: value.OpenCount7D, SourceKeyDigest: value.SourceKeyDigest,
		SourcePayloadDigest: value.SourcePayloadDigest, RecoveryEntryDigest: value.RecoveryEntryDigest, CustomerID: value.CustomerID,
		LearningPlanCurrent: value.LearningPlanCurrent, LearningPlanTotal: value.LearningPlanTotal, FormallyLoggedIn: value.FormallyLoggedIn,
		HasTokenUsage: value.HasTokenUsage, LearningPlanID: value.LearningPlanID, LastOpenAt: memberGridHistoryTimePointer(value.LastOpenAt),
		RefreshedAt: memberGridHistoryTime(value.RefreshedAt),
	})
	if err != nil {
		return [sha256.Size]byte{}, productport.ErrMemberGridHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func memberGridHistoryReady(writer *MemberGridHistoryWriter, ctx context.Context) bool {
	return writer != nil && ctx != nil && ctx.Err() == nil && !nilServicePeriodDependency(writer.store) && !nilServicePeriodDependency(writer.journal)
}

func memberGridHistorySourceKey(value string) ([sha256.Size]byte, bool) {
	if len(value) != hex.EncodedLen(sha256.Size) || value != strings.ToLower(value) {
		return [sha256.Size]byte{}, false
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return [sha256.Size]byte{}, false
	}
	var result [sha256.Size]byte
	copy(result[:], decoded)
	return result, result != ([sha256.Size]byte{})
}

func validHistoricalMemberView(value productport.HistoricalMemberView, stored bool) bool {
	return (stored && value.ID > 0 || !stored && value.ID == 0) && value.SourceKeyDigest != ([sha256.Size]byte{}) &&
		value.SourcePayloadDigest != ([sha256.Size]byte{}) && value.SourceViewID > 0 && value.SourceServiceProductID > 0 &&
		validMemberGridHistoryOptionalID(value.ProductID) && value.ConfigDigest != ([sha256.Size]byte{}) && validMemberGridHistoryText(value.Name) &&
		validMemberGridHistoryTime(value.CreatedAt, stored) && validMemberGridHistoryTime(value.UpdatedAt, stored) && !value.UpdatedAt.Before(value.CreatedAt)
}

func validHistoricalMemberUsage(value productport.HistoricalMemberUsage, stored bool) bool {
	return (stored && value.ID > 0 || !stored && value.ID == 0) && value.SourceKeyDigest != ([sha256.Size]byte{}) &&
		value.SourcePayloadDigest != ([sha256.Size]byte{}) && value.RecoveryEntryDigest != ([sha256.Size]byte{}) &&
		validMemberGridHistoryOptionalID(value.CustomerID) && validMemberGridHistoryOptionalCount(value.LearningPlanCurrent) &&
		validMemberGridHistoryOptionalCount(value.LearningPlanTotal) && value.OpenCount7D >= 0 && validMemberGridHistoryText(value.LearningPlanID) &&
		validMemberGridHistoryOptionalTime(value.LastOpenAt, stored) && validMemberGridHistoryTime(value.RefreshedAt, stored)
}

func validMemberGridHistoryOptionalID(value *int64) bool { return value == nil || *value > 0 }

func validMemberGridHistoryOptionalCount(value *int64) bool { return value == nil || *value >= 0 }

func validMemberGridHistoryText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}

func validMemberGridHistoryTime(value time.Time, canonical bool) bool {
	return !value.IsZero() && (!canonical || value.Location() == time.UTC && value.Equal(value.UTC().Truncate(time.Microsecond)))
}

func validMemberGridHistoryOptionalTime(value *time.Time, canonical bool) bool {
	return value == nil || validMemberGridHistoryTime(*value, canonical)
}

func normalizeHistoricalMemberView(value productport.HistoricalMemberView) productport.HistoricalMemberView {
	value.CreatedAt, value.UpdatedAt = normalizeMemberGridHistoryTime(value.CreatedAt), normalizeMemberGridHistoryTime(value.UpdatedAt)
	if value.ProductID != nil {
		productID := *value.ProductID
		value.ProductID = &productID
	}
	return value
}

func normalizeHistoricalMemberUsage(value productport.HistoricalMemberUsage) productport.HistoricalMemberUsage {
	value.RefreshedAt = normalizeMemberGridHistoryTime(value.RefreshedAt)
	if value.CustomerID != nil {
		customerID := *value.CustomerID
		value.CustomerID = &customerID
	}
	if value.LearningPlanCurrent != nil {
		current := *value.LearningPlanCurrent
		value.LearningPlanCurrent = &current
	}
	if value.LearningPlanTotal != nil {
		total := *value.LearningPlanTotal
		value.LearningPlanTotal = &total
	}
	if value.LastOpenAt != nil {
		lastOpenAt := normalizeMemberGridHistoryTime(*value.LastOpenAt)
		value.LastOpenAt = &lastOpenAt
	}
	return value
}

func normalizeMemberGridHistoryTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}

func memberGridHistoryTime(value time.Time) string { return value.Format(time.RFC3339Nano) }

func memberGridHistoryTimePointer(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := memberGridHistoryTime(*value)
	return &formatted
}

func withHistoricalMemberViewID(value productport.HistoricalMemberView, id int64) productport.HistoricalMemberView {
	value.ID = id
	return normalizeHistoricalMemberView(value)
}

func withHistoricalMemberUsageID(value productport.HistoricalMemberUsage, id int64) productport.HistoricalMemberUsage {
	value.ID = id
	return normalizeHistoricalMemberUsage(value)
}

func validMemberGridHistoryReceipt(receipt productport.MemberGridHistoryReceipt, kind, source string, payload [sha256.Size]byte) bool {
	return receipt.Kind == kind && receipt.SourceIdentifier == source && receipt.PayloadDigest == payload && receipt.TargetID > 0 && receipt.TargetDigest != ([sha256.Size]byte{})
}

func memberGridHistoryError(err error) error {
	switch {
	case errors.Is(err, productport.ErrMemberGridHistoryInvalid):
		return productport.ErrMemberGridHistoryInvalid
	case errors.Is(err, productport.ErrMemberGridHistoryConflict):
		return productport.ErrMemberGridHistoryConflict
	default:
		return productport.ErrMemberGridHistoryUnavailable
	}
}
