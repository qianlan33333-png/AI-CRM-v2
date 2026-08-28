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

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

// SignupTagHistoryService writes only immutable V1 signup-tag-rule facts. It
// has no dependency on the current tag catalogue or any Provider operation.
type SignupTagHistoryService struct {
	store   contactport.SignupTagHistoryStore
	journal contactport.SignupTagHistoryJournal
}

func NewSignupTagHistoryService(store contactport.SignupTagHistoryStore, journal contactport.SignupTagHistoryJournal) *SignupTagHistoryService {
	return &SignupTagHistoryService{store: store, journal: journal}
}

func (service *SignupTagHistoryService) ImportRule(ctx context.Context, sourceIdentifier string, payloadDigest [sha256.Size]byte, fact contactport.HistoricalSignupTagRule) (contactport.SignupTagHistoryReceipt, error) {
	empty := contactport.SignupTagHistoryReceipt{}
	if !signupTagHistoryReady(service, ctx) {
		return empty, contactport.ErrSignupTagHistoryUnavailable
	}
	sourceKey, ok := signupTagHistorySourceKey(sourceIdentifier)
	if !ok || payloadDigest == ([sha256.Size]byte{}) || fact.ID != 0 || fact.SourceKeyDigest != sourceKey || fact.SourcePayloadDigest != payloadDigest || !validHistoricalSignupTagRule(fact, false) {
		return empty, contactport.ErrSignupTagHistoryInvalid
	}
	fact = normalizeHistoricalSignupTagRule(fact)
	if _, err := HistoricalSignupTagRuleDigest(withHistoricalSignupTagRuleID(fact, 1)); err != nil {
		return empty, contactport.ErrSignupTagHistoryInvalid
	}

	receipt, found, err := service.journal.LoadSignupTagHistory(ctx, sourceIdentifier)
	if err != nil {
		return empty, signupTagHistoryError(err)
	}
	if found {
		if !validSignupTagHistoryReceipt(receipt, sourceIdentifier, payloadDigest) {
			return empty, contactport.ErrSignupTagHistoryConflict
		}
		actual, getErr := service.store.GetHistoricalSignupTagRule(ctx, receipt.TargetID)
		if getErr != nil {
			return empty, signupTagHistoryError(getErr)
		}
		expected := withHistoricalSignupTagRuleID(fact, receipt.TargetID)
		actualDigest, actualErr := HistoricalSignupTagRuleDigest(actual)
		expectedDigest, expectedErr := HistoricalSignupTagRuleDigest(expected)
		if actualErr != nil || expectedErr != nil || actual.ID != receipt.TargetID || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, contactport.ErrSignupTagHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}

	actual, err := service.store.CreateHistoricalSignupTagRule(ctx, fact)
	if err != nil {
		return empty, signupTagHistoryError(err)
	}
	expected := withHistoricalSignupTagRuleID(fact, actual.ID)
	actualDigest, actualErr := HistoricalSignupTagRuleDigest(actual)
	expectedDigest, expectedErr := HistoricalSignupTagRuleDigest(expected)
	if actualErr != nil || expectedErr != nil || actual.ID < 1 || actualDigest != expectedDigest {
		return empty, contactport.ErrSignupTagHistoryConflict
	}
	receipt = contactport.SignupTagHistoryReceipt{SourceIdentifier: sourceIdentifier, PayloadDigest: payloadDigest, TargetID: actual.ID, TargetDigest: actualDigest}
	if err = service.journal.RecordSignupTagHistory(ctx, receipt); err != nil {
		return empty, signupTagHistoryError(err)
	}
	return receipt, nil
}

// HistoricalSignupTagRuleDigest covers each stored history field, including
// the generated target ID, so replay and reconciliation detect target drift.
func HistoricalSignupTagRuleDigest(value contactport.HistoricalSignupTagRule) ([sha256.Size]byte, error) {
	if !validHistoricalSignupTagRule(value, true) {
		return [sha256.Size]byte{}, contactport.ErrSignupTagHistoryInvalid
	}
	encoded, err := json.Marshal(struct {
		Kind                                 string `json:"kind"`
		ID                                   int64  `json:"id"`
		SourceKeyDigest, SourcePayloadDigest [32]byte
		TagSourceID, TagName, SignupStatus   string
		OriginalActive                       bool
		UpdatedAt                            string
	}{
		Kind: "v1.signup_tag_rule", ID: value.ID, SourceKeyDigest: value.SourceKeyDigest, SourcePayloadDigest: value.SourcePayloadDigest,
		TagSourceID: value.TagSourceID, TagName: value.TagName, SignupStatus: value.SignupStatus, OriginalActive: value.OriginalActive,
		UpdatedAt: signupTagHistoryTime(value.UpdatedAt),
	})
	if err != nil {
		return [sha256.Size]byte{}, contactport.ErrSignupTagHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func signupTagHistoryReady(service *SignupTagHistoryService, ctx context.Context) bool {
	return service != nil && ctx != nil && ctx.Err() == nil && !signupTagHistoryNil(service.store) && !signupTagHistoryNil(service.journal)
}

func signupTagHistorySourceKey(value string) ([sha256.Size]byte, bool) {
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

func validHistoricalSignupTagRule(value contactport.HistoricalSignupTagRule, stored bool) bool {
	return (stored && value.ID > 0 || !stored && value.ID == 0) && value.SourceKeyDigest != ([sha256.Size]byte{}) &&
		value.SourcePayloadDigest != ([sha256.Size]byte{}) && validSignupTagHistoryText(value.TagSourceID) &&
		validSignupTagHistoryText(value.TagName) && validSignupTagHistoryText(value.SignupStatus) &&
		validSignupTagHistoryTime(value.UpdatedAt, stored)
}

func validSignupTagHistoryText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}

func validSignupTagHistoryTime(value time.Time, canonical bool) bool {
	return !value.IsZero() && (!canonical || value.Location() == time.UTC && value.Equal(value.UTC().Truncate(time.Microsecond)))
}

func normalizeHistoricalSignupTagRule(value contactport.HistoricalSignupTagRule) contactport.HistoricalSignupTagRule {
	value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
	return value
}

func withHistoricalSignupTagRuleID(value contactport.HistoricalSignupTagRule, id int64) contactport.HistoricalSignupTagRule {
	value.ID = id
	return normalizeHistoricalSignupTagRule(value)
}

func signupTagHistoryTime(value time.Time) string { return value.Format(time.RFC3339Nano) }

func validSignupTagHistoryReceipt(receipt contactport.SignupTagHistoryReceipt, source string, payload [sha256.Size]byte) bool {
	return receipt.SourceIdentifier == source && receipt.PayloadDigest == payload && receipt.TargetID > 0 && receipt.TargetDigest != ([sha256.Size]byte{})
}

func signupTagHistoryError(err error) error {
	switch {
	case errors.Is(err, contactport.ErrSignupTagHistoryInvalid):
		return contactport.ErrSignupTagHistoryInvalid
	case errors.Is(err, contactport.ErrSignupTagHistoryConflict):
		return contactport.ErrSignupTagHistoryConflict
	default:
		return contactport.ErrSignupTagHistoryUnavailable
	}
}

func signupTagHistoryNil(value any) bool {
	if value == nil {
		return true
	}
	raw := reflect.ValueOf(value)
	return raw.Kind() == reflect.Ptr && raw.IsNil()
}
