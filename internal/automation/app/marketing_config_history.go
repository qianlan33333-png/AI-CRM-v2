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

	automation "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
)

const (
	marketingConfigHistoryConfigKind = "marketing_config"
	marketingConfigHistoryRuleKind   = "marketing_rule"
)

// MarketingConfigHistoryWriter writes inert V1 marketing facts in the caller
// transaction. It never creates or enables a current automation or rule.
type MarketingConfigHistoryWriter struct {
	store   automation.MarketingConfigHistoryStore
	journal automation.MarketingConfigHistoryJournal
}

func NewMarketingConfigHistoryWriter(store automation.MarketingConfigHistoryStore, journal automation.MarketingConfigHistoryJournal) (*MarketingConfigHistoryWriter, error) {
	if nilMarketingConfigHistory(store) || nilMarketingConfigHistory(journal) {
		return nil, automation.ErrMarketingConfigHistoryUnavailable
	}
	return &MarketingConfigHistoryWriter{store: store, journal: journal}, nil
}

func (w *MarketingConfigHistoryWriter) ImportHistoricalMarketingAutomationConfig(ctx context.Context, source string, value automation.HistoricalMarketingAutomationConfig) (automation.MarketingConfigHistoryReceipt, error) {
	value = normalizeMarketingConfig(value)
	if !validMarketingConfig(value, false) || !validMarketingConfigSource(source, value.SourceKeyDigest) {
		return automation.MarketingConfigHistoryReceipt{}, automation.ErrMarketingConfigHistoryInvalid
	}
	return importMarketingConfigHistory(w, ctx, marketingConfigHistoryConfigKind, source, value.SourceKeyDigest, value.SourcePayloadDigest, value.ID,
		func(id int64) ([32]byte, error) {
			expected := value
			expected.ID = id
			return HistoricalMarketingAutomationConfigDigest(expected)
		},
		func() (int64, [32]byte, error) {
			actual, err := w.store.CreateHistoricalMarketingAutomationConfig(ctx, value)
			if err != nil {
				return 0, [32]byte{}, err
			}
			digest, err := HistoricalMarketingAutomationConfigDigest(actual)
			return actual.ID, digest, err
		},
		func(id int64) (int64, [32]byte, error) {
			actual, err := w.store.GetHistoricalMarketingAutomationConfig(ctx, id)
			if err != nil {
				return 0, [32]byte{}, err
			}
			digest, err := HistoricalMarketingAutomationConfigDigest(actual)
			return actual.ID, digest, err
		})
}

func (w *MarketingConfigHistoryWriter) ImportHistoricalMarketingAutomationRule(ctx context.Context, source string, value automation.HistoricalMarketingAutomationRule) (automation.MarketingConfigHistoryReceipt, error) {
	value = normalizeMarketingRule(value)
	if !validMarketingRule(value, false) || !validMarketingConfigSource(source, value.SourceKeyDigest) {
		return automation.MarketingConfigHistoryReceipt{}, automation.ErrMarketingConfigHistoryInvalid
	}
	return importMarketingConfigHistory(w, ctx, marketingConfigHistoryRuleKind, source, value.SourceKeyDigest, value.SourcePayloadDigest, value.ID,
		func(id int64) ([32]byte, error) {
			expected := value
			expected.ID = id
			return HistoricalMarketingAutomationRuleDigest(expected)
		},
		func() (int64, [32]byte, error) {
			actual, err := w.store.CreateHistoricalMarketingAutomationRule(ctx, value)
			if err != nil {
				return 0, [32]byte{}, err
			}
			digest, err := HistoricalMarketingAutomationRuleDigest(actual)
			return actual.ID, digest, err
		},
		func(id int64) (int64, [32]byte, error) {
			actual, err := w.store.GetHistoricalMarketingAutomationRule(ctx, id)
			if err != nil {
				return 0, [32]byte{}, err
			}
			digest, err := HistoricalMarketingAutomationRuleDigest(actual)
			return actual.ID, digest, err
		})
}

func importMarketingConfigHistory(w *MarketingConfigHistoryWriter, ctx context.Context, kind, source string, key, payload [32]byte, inputID int64, expected func(int64) ([32]byte, error), create func() (int64, [32]byte, error), get func(int64) (int64, [32]byte, error)) (automation.MarketingConfigHistoryReceipt, error) {
	var empty automation.MarketingConfigHistoryReceipt
	if w == nil || ctx == nil || ctx.Err() != nil || nilMarketingConfigHistory(w.store) || nilMarketingConfigHistory(w.journal) {
		return empty, automation.ErrMarketingConfigHistoryUnavailable
	}
	if inputID != 0 || key == ([32]byte{}) || payload == ([32]byte{}) || expected == nil || create == nil || get == nil {
		return empty, automation.ErrMarketingConfigHistoryInvalid
	}
	if _, err := expected(1); err != nil {
		return empty, automation.ErrMarketingConfigHistoryInvalid
	}
	receipt, found, err := w.journal.LoadMarketingConfigHistory(ctx, kind, source)
	if err != nil {
		return empty, marketingConfigHistoryError(err)
	}
	if found {
		if !validMarketingConfigReceipt(receipt, kind, source, payload) {
			return empty, automation.ErrMarketingConfigHistoryConflict
		}
		expectedDigest, expectedErr := expected(receipt.TargetID)
		id, actualDigest, actualErr := get(receipt.TargetID)
		if expectedErr != nil || actualErr != nil {
			return empty, marketingConfigHistoryError(firstMarketingConfigError(expectedErr, actualErr))
		}
		if id != receipt.TargetID || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, automation.ErrMarketingConfigHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	id, actualDigest, err := create()
	if err != nil {
		return empty, marketingConfigHistoryError(err)
	}
	expectedDigest, expectedErr := expected(id)
	if expectedErr != nil || id < 1 || actualDigest != expectedDigest {
		return empty, automation.ErrMarketingConfigHistoryConflict
	}
	receipt = automation.MarketingConfigHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetDigest: actualDigest, TargetID: id}
	if err := w.journal.RecordMarketingConfigHistory(ctx, receipt); err != nil {
		return empty, marketingConfigHistoryError(err)
	}
	return receipt, nil
}

func firstMarketingConfigError(values ...error) error {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return automation.ErrMarketingConfigHistoryUnavailable
}

// These digests list the private fields explicitly; marshaling Port values
// directly would omit their json:"-" source and sealed-payload bindings.
func HistoricalMarketingAutomationConfigDigest(value automation.HistoricalMarketingAutomationConfig) ([32]byte, error) {
	value = normalizeMarketingConfig(value)
	if !validMarketingConfig(value, true) {
		return [32]byte{}, automation.ErrMarketingConfigHistoryInvalid
	}
	return marketingConfigHistoryDigest(marketingConfigHistoryConfigKind, struct {
		ID                                                      int64
		SourceKey, SourcePayload, SourceField, ConfigPayload    [32]byte
		SourceID                                                int64
		AutomationKey, AutomationName, TargetEvent, ChannelType string
		OriginalStatus                                          string
		DoNotStartAfterHour                                     int32
		CreatedAt, UpdatedAt                                    time.Time
	}{value.ID, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, value.ConfigPayloadDigest, value.SourceID, value.AutomationKey, value.AutomationName, value.TargetEvent, value.ChannelType, value.OriginalStatus, value.DoNotStartAfterHour, value.CreatedAt, value.UpdatedAt})
}

func HistoricalMarketingAutomationRuleDigest(value automation.HistoricalMarketingAutomationRule) ([32]byte, error) {
	value = normalizeMarketingRule(value)
	if !validMarketingRule(value, true) {
		return [32]byte{}, automation.ErrMarketingConfigHistoryInvalid
	}
	return marketingConfigHistoryDigest(marketingConfigHistoryRuleKind, struct {
		ID                                                              int64
		SourceKey, SourcePayload, SourceField, AnswerValue, RulePayload [32]byte
		SourceID, ConfigID, ConfigSourceID                              int64
		QuestionnaireSourceID, QuestionSourceID                         *int64
		RuleCode, RuleName, AnswerMatchType, SegmentHint, StageHint     string
		ScoreDelta, SortOrder                                           int32
		OriginalActive                                                  bool
		CreatedAt, UpdatedAt                                            time.Time
	}{value.ID, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, value.AnswerMatchValueDigest, value.RulePayloadDigest, value.SourceID, value.ConfigID, value.ConfigSourceID, value.QuestionnaireSourceID, value.QuestionSourceID, value.RuleCode, value.RuleName, value.AnswerMatchType, value.SegmentHint, value.StageHint, value.ScoreDelta, value.SortOrder, value.OriginalActive, value.CreatedAt, value.UpdatedAt})
}

func marketingConfigHistoryDigest(kind string, value any) ([32]byte, error) {
	encoded, err := json.Marshal(struct {
		Kind  string
		Value any
	}{kind, value})
	if err != nil {
		return [32]byte{}, automation.ErrMarketingConfigHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func normalizeMarketingConfig(value automation.HistoricalMarketingAutomationConfig) automation.HistoricalMarketingAutomationConfig {
	value.CreatedAt = marketingConfigHistoryTime(value.CreatedAt)
	value.UpdatedAt = marketingConfigHistoryTime(value.UpdatedAt)
	return value
}

func normalizeMarketingRule(value automation.HistoricalMarketingAutomationRule) automation.HistoricalMarketingAutomationRule {
	value.CreatedAt = marketingConfigHistoryTime(value.CreatedAt)
	value.UpdatedAt = marketingConfigHistoryTime(value.UpdatedAt)
	value.QuestionnaireSourceID = copyMarketingConfigInt64(value.QuestionnaireSourceID)
	value.QuestionSourceID = copyMarketingConfigInt64(value.QuestionSourceID)
	return value
}

func validMarketingConfig(value automation.HistoricalMarketingAutomationConfig, stored bool) bool {
	return validMarketingConfigIdentity(value.ID, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, stored) &&
		allMarketingConfigDigests(value.ConfigPayloadDigest) && validMarketingConfigText(value.AutomationKey, value.AutomationName, value.TargetEvent, value.ChannelType, value.OriginalStatus) &&
		validMarketingConfigTimes(value.CreatedAt, value.UpdatedAt, stored)
}

func validMarketingRule(value automation.HistoricalMarketingAutomationRule, stored bool) bool {
	return validMarketingConfigIdentity(value.ID, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, stored) && value.ConfigID > 0 &&
		allMarketingConfigDigests(value.AnswerMatchValueDigest, value.RulePayloadDigest) && validMarketingConfigText(value.RuleCode, value.RuleName, value.AnswerMatchType, value.SegmentHint, value.StageHint) &&
		validMarketingConfigTimes(value.CreatedAt, value.UpdatedAt, stored)
}

func validMarketingConfigIdentity(id int64, key, payload, field [32]byte, stored bool) bool {
	return (stored && id > 0 || !stored && id == 0) && allMarketingConfigDigests(key, payload, field)
}

func allMarketingConfigDigests(values ...[32]byte) bool {
	for _, value := range values {
		if value == ([32]byte{}) {
			return false
		}
	}
	return true
}

func validMarketingConfigSource(source string, key [32]byte) bool {
	return source != "" && strings.TrimSpace(source) == source && source == strings.ToLower(source) && source == hex.EncodeToString(key[:])
}

func validMarketingConfigText(values ...string) bool {
	for _, value := range values {
		if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
			return false
		}
	}
	return true
}

func validMarketingConfigTimes(created, updated time.Time, stored bool) bool {
	return validMarketingConfigTime(created, stored) && validMarketingConfigTime(updated, stored)
}

func validMarketingConfigTime(value time.Time, stored bool) bool {
	return !value.IsZero() && (!stored || value.Location() == time.UTC && value.Equal(value.UTC().Truncate(time.Microsecond)))
}

func marketingConfigHistoryTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}

func copyMarketingConfigInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func validMarketingConfigReceipt(value automation.MarketingConfigHistoryReceipt, kind, source string, payload [32]byte) bool {
	return value.Kind == kind && value.SourceIdentifier == source && value.PayloadDigest == payload && value.TargetID > 0 && value.TargetDigest != ([32]byte{})
}

func marketingConfigHistoryError(err error) error {
	if errors.Is(err, automation.ErrMarketingConfigHistoryInvalid) {
		return automation.ErrMarketingConfigHistoryInvalid
	}
	if errors.Is(err, automation.ErrMarketingConfigHistoryConflict) {
		return automation.ErrMarketingConfigHistoryConflict
	}
	return automation.ErrMarketingConfigHistoryUnavailable
}

func nilMarketingConfigHistory(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return (v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface) && v.IsNil()
}
