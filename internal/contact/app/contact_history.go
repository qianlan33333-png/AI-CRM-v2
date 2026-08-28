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

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

// ContactHistoryWriter persists only immutable V1 historical projections and
// their caller-transaction receipts. It has no current-profile, event, queue,
// or Provider dependency.
type ContactHistoryWriter struct {
	store   contactport.ContactHistoryStore
	journal contactport.ContactHistoryJournal
}

func NewContactHistoryWriter(store contactport.ContactHistoryStore, journal contactport.ContactHistoryJournal) *ContactHistoryWriter {
	return &ContactHistoryWriter{store: store, journal: journal}
}

func (writer *ContactHistoryWriter) WriteSidebarProfile(ctx context.Context, sourceIdentifier string, payloadDigest [sha256.Size]byte, value contactport.HistoricalSidebarProfile) (contactport.ContactHistoryReceipt, error) {
	empty := contactport.ContactHistoryReceipt{}
	if !contactHistoryReady(writer, ctx) {
		return empty, contactport.ErrContactHistoryUnavailable
	}
	sourceKey, ok := contactHistorySourceKey(sourceIdentifier)
	if !ok || payloadDigest == ([sha256.Size]byte{}) || value.ID != 0 || value.SourceKeyDigest != sourceKey || value.SourcePayloadDigest != payloadDigest || !validHistoricalSidebarProfile(value, false) {
		return empty, contactport.ErrContactHistoryInvalid
	}
	value = normalizeHistoricalSidebarProfile(value)
	if _, err := HistoricalSidebarProfileDigest(withHistoricalSidebarProfileID(value, 1)); err != nil {
		return empty, contactport.ErrContactHistoryInvalid
	}

	receipt, found, err := writer.journal.LoadContactHistory(ctx, contactport.ContactHistorySidebar, sourceIdentifier)
	if err != nil {
		return empty, contactHistoryError(err)
	}
	if found {
		if !validContactHistoryReceipt(receipt, contactport.ContactHistorySidebar, sourceIdentifier, payloadDigest) {
			return empty, contactport.ErrContactHistoryConflict
		}
		actual, getErr := writer.store.GetHistoricalSidebarProfile(ctx, receipt.TargetID)
		if getErr != nil {
			return empty, contactHistoryError(getErr)
		}
		expected := withHistoricalSidebarProfileID(value, receipt.TargetID)
		actualDigest, actualErr := HistoricalSidebarProfileDigest(actual)
		expectedDigest, expectedErr := HistoricalSidebarProfileDigest(expected)
		if actualErr != nil || expectedErr != nil || actual.ID != receipt.TargetID || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, contactport.ErrContactHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}

	actual, err := writer.store.CreateHistoricalSidebarProfile(ctx, value)
	if err != nil {
		return empty, contactHistoryError(err)
	}
	actualDigest, actualErr := HistoricalSidebarProfileDigest(actual)
	expectedDigest, expectedErr := HistoricalSidebarProfileDigest(withHistoricalSidebarProfileID(value, actual.ID))
	if actualErr != nil || expectedErr != nil || actual.ID < 1 || actualDigest != expectedDigest {
		return empty, contactport.ErrContactHistoryConflict
	}
	receipt = contactport.ContactHistoryReceipt{Kind: contactport.ContactHistorySidebar, SourceIdentifier: sourceIdentifier, PayloadDigest: payloadDigest, TargetID: actual.ID, TargetDigest: actualDigest}
	if err = writer.journal.RecordContactHistory(ctx, receipt); err != nil {
		return empty, contactHistoryError(err)
	}
	return receipt, nil
}

func (writer *ContactHistoryWriter) WriteOwnerMigrationResult(ctx context.Context, sourceIdentifier string, payloadDigest [sha256.Size]byte, value contactport.HistoricalOwnerMigrationResult) (contactport.ContactHistoryReceipt, error) {
	empty := contactport.ContactHistoryReceipt{}
	if !contactHistoryReady(writer, ctx) {
		return empty, contactport.ErrContactHistoryUnavailable
	}
	sourceKey, ok := contactHistorySourceKey(sourceIdentifier)
	if !ok || payloadDigest == ([sha256.Size]byte{}) || value.ID != 0 || value.SourceKeyDigest != sourceKey || value.SourcePayloadDigest != payloadDigest || !validHistoricalOwnerMigrationResult(value, false) {
		return empty, contactport.ErrContactHistoryInvalid
	}
	value = normalizeHistoricalOwnerMigrationResult(value)
	if _, err := HistoricalOwnerMigrationResultDigest(withHistoricalOwnerMigrationResultID(value, 1)); err != nil {
		return empty, contactport.ErrContactHistoryInvalid
	}

	receipt, found, err := writer.journal.LoadContactHistory(ctx, contactport.ContactHistoryOwnerResult, sourceIdentifier)
	if err != nil {
		return empty, contactHistoryError(err)
	}
	if found {
		if !validContactHistoryReceipt(receipt, contactport.ContactHistoryOwnerResult, sourceIdentifier, payloadDigest) {
			return empty, contactport.ErrContactHistoryConflict
		}
		actual, getErr := writer.store.GetHistoricalOwnerMigrationResult(ctx, receipt.TargetID)
		if getErr != nil {
			return empty, contactHistoryError(getErr)
		}
		expected := withHistoricalOwnerMigrationResultID(value, receipt.TargetID)
		actualDigest, actualErr := HistoricalOwnerMigrationResultDigest(actual)
		expectedDigest, expectedErr := HistoricalOwnerMigrationResultDigest(expected)
		if actualErr != nil || expectedErr != nil || actual.ID != receipt.TargetID || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, contactport.ErrContactHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}

	actual, err := writer.store.CreateHistoricalOwnerMigrationResult(ctx, value)
	if err != nil {
		return empty, contactHistoryError(err)
	}
	actualDigest, actualErr := HistoricalOwnerMigrationResultDigest(actual)
	expectedDigest, expectedErr := HistoricalOwnerMigrationResultDigest(withHistoricalOwnerMigrationResultID(value, actual.ID))
	if actualErr != nil || expectedErr != nil || actual.ID < 1 || actualDigest != expectedDigest {
		return empty, contactport.ErrContactHistoryConflict
	}
	receipt = contactport.ContactHistoryReceipt{Kind: contactport.ContactHistoryOwnerResult, SourceIdentifier: sourceIdentifier, PayloadDigest: payloadDigest, TargetID: actual.ID, TargetDigest: actualDigest}
	if err = writer.journal.RecordContactHistory(ctx, receipt); err != nil {
		return empty, contactHistoryError(err)
	}
	return receipt, nil
}

// HistoricalSidebarProfileDigest covers every stored history field, including
// the generated target ID, so replay and reconciliation detect target drift.
func HistoricalSidebarProfileDigest(value contactport.HistoricalSidebarProfile) ([sha256.Size]byte, error) {
	if !validHistoricalSidebarProfile(value, true) {
		return [sha256.Size]byte{}, contactport.ErrContactHistoryInvalid
	}
	encoded, err := json.Marshal(struct {
		Kind                                  string `json:"kind"`
		ID                                    int64  `json:"id"`
		SourceKeyDigest, SourcePayloadDigest  [32]byte
		CustomerID                            *int64 `json:"customer_id"`
		Source, Industry, IndustryDescription string
		NeedsBlockersFollowup                 string
		UpdatedAt                             string
	}{
		Kind: "v1.sidebar_profile", ID: value.ID, SourceKeyDigest: value.SourceKeyDigest, SourcePayloadDigest: value.SourcePayloadDigest,
		CustomerID: value.CustomerID, Source: value.Source, Industry: value.Industry, IndustryDescription: value.IndustryDescription,
		NeedsBlockersFollowup: value.NeedsBlockersFollowup, UpdatedAt: contactHistoryTime(value.UpdatedAt),
	})
	if err != nil {
		return [sha256.Size]byte{}, contactport.ErrContactHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

// HistoricalOwnerMigrationResultDigest covers only the frozen, non-executable
// result projection and its generated V2 target ID.
func HistoricalOwnerMigrationResultDigest(value contactport.HistoricalOwnerMigrationResult) ([sha256.Size]byte, error) {
	if !validHistoricalOwnerMigrationResult(value, true) {
		return [sha256.Size]byte{}, contactport.ErrContactHistoryInvalid
	}
	encoded, err := json.Marshal(struct {
		Kind                                  string `json:"kind"`
		ID                                    int64  `json:"id"`
		SourceKeyDigest, SourcePayloadDigest  [32]byte
		ScopeType, FileHash, PreviewHash      string
		TotalRows, EligibleCount              int64
		WeComSuccess, WeComFailed, CRMUpdated int64
		IncludeWeComTransfer                  bool
		TransferWelcomeMessage                string
		SessionRelation, PreviewRelation      string
		CreatedAt, ExecutedAt                 string
	}{
		Kind: "v1.owner_migration_result", ID: value.ID, SourceKeyDigest: value.SourceKeyDigest, SourcePayloadDigest: value.SourcePayloadDigest,
		ScopeType: value.ScopeType, FileHash: value.FileHash, PreviewHash: value.PreviewHash, TotalRows: value.TotalRows,
		EligibleCount: value.EligibleCount, WeComSuccess: value.WeComSuccess, WeComFailed: value.WeComFailed, CRMUpdated: value.CRMUpdated,
		IncludeWeComTransfer: value.IncludeWeComTransfer, TransferWelcomeMessage: value.TransferWelcomeMessage,
		SessionRelation: value.SessionRelation, PreviewRelation: value.PreviewRelation,
		CreatedAt: contactHistoryTime(value.CreatedAt), ExecutedAt: contactHistoryTime(value.ExecutedAt),
	})
	if err != nil {
		return [sha256.Size]byte{}, contactport.ErrContactHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func contactHistoryReady(writer *ContactHistoryWriter, ctx context.Context) bool {
	return writer != nil && ctx != nil && ctx.Err() == nil && !historicalChannelNil(writer.store) && !historicalChannelNil(writer.journal)
}

func contactHistorySourceKey(value string) ([sha256.Size]byte, bool) {
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

func validHistoricalSidebarProfile(value contactport.HistoricalSidebarProfile, stored bool) bool {
	return (stored && value.ID > 0 || !stored && value.ID == 0) && value.SourceKeyDigest != ([sha256.Size]byte{}) &&
		value.SourcePayloadDigest != ([sha256.Size]byte{}) && (value.CustomerID == nil || *value.CustomerID > 0) &&
		validContactHistoryText(value.Source) && validContactHistoryText(value.Industry) && validContactHistoryText(value.IndustryDescription) &&
		validContactHistoryText(value.NeedsBlockersFollowup) && validContactHistoryTime(value.UpdatedAt, stored)
}

func validHistoricalOwnerMigrationResult(value contactport.HistoricalOwnerMigrationResult, stored bool) bool {
	return (stored && value.ID > 0 || !stored && value.ID == 0) && value.SourceKeyDigest != ([sha256.Size]byte{}) &&
		value.SourcePayloadDigest != ([sha256.Size]byte{}) && value.TotalRows >= 0 && value.EligibleCount >= 0 && value.WeComSuccess >= 0 &&
		value.WeComFailed >= 0 && value.CRMUpdated >= 0 && validContactHistoryText(value.ScopeType) && validContactHistoryText(value.FileHash) &&
		validContactHistoryText(value.PreviewHash) && validContactHistoryText(value.TransferWelcomeMessage) &&
		(value.SessionRelation == "resolved" || value.SessionRelation == "unresolved") && (value.PreviewRelation == "resolved" || value.PreviewRelation == "unresolved") &&
		validContactHistoryTime(value.CreatedAt, stored) && validContactHistoryTime(value.ExecutedAt, stored) && !value.ExecutedAt.Before(value.CreatedAt)
}

func validContactHistoryText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}

func validContactHistoryTime(value time.Time, canonical bool) bool {
	return !value.IsZero() && (!canonical || value.Location() == time.UTC && value.Equal(value.UTC().Truncate(time.Microsecond)))
}

func normalizeHistoricalSidebarProfile(value contactport.HistoricalSidebarProfile) contactport.HistoricalSidebarProfile {
	value.UpdatedAt = normalizeContactHistoryTime(value.UpdatedAt)
	if value.CustomerID != nil {
		customerID := *value.CustomerID
		value.CustomerID = &customerID
	}
	return value
}

func normalizeHistoricalOwnerMigrationResult(value contactport.HistoricalOwnerMigrationResult) contactport.HistoricalOwnerMigrationResult {
	value.CreatedAt = normalizeContactHistoryTime(value.CreatedAt)
	value.ExecutedAt = normalizeContactHistoryTime(value.ExecutedAt)
	return value
}

func normalizeContactHistoryTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}
func contactHistoryTime(value time.Time) string { return value.Format(time.RFC3339Nano) }

func withHistoricalSidebarProfileID(value contactport.HistoricalSidebarProfile, id int64) contactport.HistoricalSidebarProfile {
	value.ID = id
	return normalizeHistoricalSidebarProfile(value)
}

func withHistoricalOwnerMigrationResultID(value contactport.HistoricalOwnerMigrationResult, id int64) contactport.HistoricalOwnerMigrationResult {
	value.ID = id
	return normalizeHistoricalOwnerMigrationResult(value)
}

func validContactHistoryReceipt(receipt contactport.ContactHistoryReceipt, kind, source string, payload [sha256.Size]byte) bool {
	return receipt.Kind == kind && receipt.SourceIdentifier == source && receipt.PayloadDigest == payload && receipt.TargetID > 0 && receipt.TargetDigest != ([sha256.Size]byte{})
}

func contactHistoryError(err error) error {
	switch {
	case errors.Is(err, contactport.ErrContactHistoryInvalid):
		return contactport.ErrContactHistoryInvalid
	case errors.Is(err, contactport.ErrContactHistoryConflict):
		return contactport.ErrContactHistoryConflict
	default:
		return contactport.ErrContactHistoryUnavailable
	}
}
