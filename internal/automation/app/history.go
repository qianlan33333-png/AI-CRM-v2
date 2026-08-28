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

	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
)

// AutomationHistoryWriter writes frozen V1 facts in the caller transaction.
// It never creates a current automation, rule, publish, event, LLM, or Provider action.
type AutomationHistoryWriter struct {
	store   automationport.AutomationHistoryStore
	journal automationport.AutomationHistoryJournal
}

func NewAutomationHistoryWriter(store automationport.AutomationHistoryStore, journal automationport.AutomationHistoryJournal) (*AutomationHistoryWriter, error) {
	if nilAutomationHistoryDependency(store) || nilAutomationHistoryDependency(journal) {
		return nil, automationport.ErrAutomationHistoryUnavailable
	}
	return &AutomationHistoryWriter{store: store, journal: journal}, nil
}

func (writer *AutomationHistoryWriter) ImportSOP(ctx context.Context, source string, fact automationport.HistoricalAutomationSOP) (automationport.AutomationHistoryReceipt, error) {
	fact = normalizeAutomationSOP(fact)
	if !validAutomationSOP(fact, false) || !validAutomationSource(source, fact.HistoricalAutomationIdentity) {
		return automationport.AutomationHistoryReceipt{}, automationport.ErrAutomationHistoryInvalid
	}
	return writer.importHistory(ctx, automationport.AutomationHistorySOP, source, fact.SourcePayloadDigest, func(id int64) ([32]byte, error) {
		expected := fact
		expected.ID = id
		return HistoricalAutomationSOPDigest(expected)
	}, func(id int64) (int64, [32]byte, error) {
		var actual automationport.HistoricalAutomationSOP
		var err error
		if id == 0 {
			actual, err = writer.store.CreateHistoricalAutomationSOP(ctx, fact)
		} else {
			actual, err = writer.store.GetHistoricalAutomationSOP(ctx, id)
		}
		if err != nil {
			return 0, [32]byte{}, err
		}
		digest, digestErr := HistoricalAutomationSOPDigest(actual)
		return actual.ID, digest, digestErr
	})
}

func (writer *AutomationHistoryWriter) ImportConfig(ctx context.Context, source string, fact automationport.HistoricalAutomationConfig) (automationport.AutomationHistoryReceipt, error) {
	fact = normalizeAutomationConfig(fact)
	if !validAutomationConfig(fact, false) || !validAutomationSource(source, fact.HistoricalAutomationIdentity) {
		return automationport.AutomationHistoryReceipt{}, automationport.ErrAutomationHistoryInvalid
	}
	return writer.importHistory(ctx, automationport.AutomationHistoryConfig, source, fact.SourcePayloadDigest, func(id int64) ([32]byte, error) {
		expected := fact
		expected.ID = id
		return HistoricalAutomationConfigDigest(expected)
	}, func(id int64) (int64, [32]byte, error) {
		var actual automationport.HistoricalAutomationConfig
		var err error
		if id == 0 {
			actual, err = writer.store.CreateHistoricalAutomationConfig(ctx, fact)
		} else {
			actual, err = writer.store.GetHistoricalAutomationConfig(ctx, id)
		}
		if err != nil {
			return 0, [32]byte{}, err
		}
		digest, digestErr := HistoricalAutomationConfigDigest(actual)
		return actual.ID, digest, digestErr
	})
}

func (writer *AutomationHistoryWriter) ImportPrompt(ctx context.Context, source string, fact automationport.HistoricalAutomationPrompt) (automationport.AutomationHistoryReceipt, error) {
	fact = normalizeAutomationPrompt(fact)
	if !validAutomationPrompt(fact, false) || !validAutomationSource(source, fact.HistoricalAutomationIdentity) {
		return automationport.AutomationHistoryReceipt{}, automationport.ErrAutomationHistoryInvalid
	}
	return writer.importHistory(ctx, automationport.AutomationHistoryPrompt, source, fact.SourcePayloadDigest, func(id int64) ([32]byte, error) {
		expected := fact
		expected.ID = id
		return HistoricalAutomationPromptDigest(expected)
	}, func(id int64) (int64, [32]byte, error) {
		var actual automationport.HistoricalAutomationPrompt
		var err error
		if id == 0 {
			actual, err = writer.store.CreateHistoricalAutomationPrompt(ctx, fact)
		} else {
			actual, err = writer.store.GetHistoricalAutomationPrompt(ctx, id)
		}
		if err != nil {
			return 0, [32]byte{}, err
		}
		digest, digestErr := HistoricalAutomationPromptDigest(actual)
		return actual.ID, digest, digestErr
	})
}

func (writer *AutomationHistoryWriter) ImportAgent(ctx context.Context, source string, fact automationport.HistoricalAutomationAgent) (automationport.AutomationHistoryReceipt, error) {
	fact = normalizeAutomationAgent(fact)
	if !validAutomationAgent(fact, false) || !validAutomationSource(source, fact.HistoricalAutomationIdentity) {
		return automationport.AutomationHistoryReceipt{}, automationport.ErrAutomationHistoryInvalid
	}
	return writer.importHistory(ctx, automationport.AutomationHistoryAgent, source, fact.SourcePayloadDigest, func(id int64) ([32]byte, error) {
		expected := fact
		expected.ID = id
		return HistoricalAutomationAgentDigest(expected)
	}, func(id int64) (int64, [32]byte, error) {
		var actual automationport.HistoricalAutomationAgent
		var err error
		if id == 0 {
			actual, err = writer.store.CreateHistoricalAutomationAgent(ctx, fact)
		} else {
			actual, err = writer.store.GetHistoricalAutomationAgent(ctx, id)
		}
		if err != nil {
			return 0, [32]byte{}, err
		}
		digest, digestErr := HistoricalAutomationAgentDigest(actual)
		return actual.ID, digest, digestErr
	})
}

func (writer *AutomationHistoryWriter) importHistory(ctx context.Context, kind, source string, payload [32]byte, expected func(int64) ([32]byte, error), access func(int64) (int64, [32]byte, error)) (automationport.AutomationHistoryReceipt, error) {
	var empty automationport.AutomationHistoryReceipt
	if writer == nil || nilAutomationHistoryDependency(writer.store) || nilAutomationHistoryDependency(writer.journal) || ctx == nil {
		return empty, automationport.ErrAutomationHistoryUnavailable
	}
	if err := ctx.Err(); err != nil {
		return empty, automationport.ErrAutomationHistoryUnavailable
	}
	if payload == [32]byte{} || expected == nil || access == nil {
		return empty, automationport.ErrAutomationHistoryInvalid
	}
	receipt, found, err := writer.journal.LoadAutomationHistory(ctx, kind, source)
	if err != nil {
		return empty, automationHistoryWriteError(err)
	}
	if found {
		expectedDigest, expectedErr := expected(receipt.TargetID)
		if expectedErr != nil {
			return empty, automationport.ErrAutomationHistoryConflict
		}
		if receipt.Kind != kind || receipt.SourceIdentifier != source || receipt.PayloadDigest != payload || receipt.TargetID < 1 || receipt.TargetDigest != expectedDigest {
			return empty, automationport.ErrAutomationHistoryConflict
		}
		id, digest, readErr := access(receipt.TargetID)
		if readErr != nil {
			return empty, automationHistoryWriteError(readErr)
		}
		if id != receipt.TargetID || digest != receipt.TargetDigest {
			return empty, automationport.ErrAutomationHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	id, digest, err := access(0)
	if err != nil {
		return empty, automationHistoryWriteError(err)
	}
	expectedDigest, expectedErr := expected(id)
	if expectedErr != nil || id < 1 || digest != expectedDigest {
		return empty, automationport.ErrAutomationHistoryConflict
	}
	receipt = automationport.AutomationHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetID: id, TargetDigest: digest}
	if err = writer.journal.RecordAutomationHistory(ctx, receipt); err != nil {
		return empty, automationHistoryWriteError(err)
	}
	return receipt, nil
}

// Target digests bind every typed historical field and the generated V2 history ID.
func HistoricalAutomationSOPDigest(fact automationport.HistoricalAutomationSOP) ([32]byte, error) {
	fact = normalizeAutomationSOP(fact)
	if !validAutomationSOP(fact, true) {
		return [32]byte{}, automationport.ErrAutomationHistoryInvalid
	}
	return automationHistoryDigest(automationport.AutomationHistorySOP, fact)
}

func HistoricalAutomationConfigDigest(fact automationport.HistoricalAutomationConfig) ([32]byte, error) {
	fact = normalizeAutomationConfig(fact)
	if !validAutomationConfig(fact, true) {
		return [32]byte{}, automationport.ErrAutomationHistoryInvalid
	}
	return automationHistoryDigest(automationport.AutomationHistoryConfig, fact)
}

func HistoricalAutomationPromptDigest(fact automationport.HistoricalAutomationPrompt) ([32]byte, error) {
	fact = normalizeAutomationPrompt(fact)
	if !validAutomationPrompt(fact, true) {
		return [32]byte{}, automationport.ErrAutomationHistoryInvalid
	}
	return automationHistoryDigest(automationport.AutomationHistoryPrompt, fact)
}

func HistoricalAutomationAgentDigest(fact automationport.HistoricalAutomationAgent) ([32]byte, error) {
	fact = normalizeAutomationAgent(fact)
	if !validAutomationAgent(fact, true) {
		return [32]byte{}, automationport.ErrAutomationHistoryInvalid
	}
	return automationHistoryDigest(automationport.AutomationHistoryAgent, fact)
}

func automationHistoryDigest(kind string, fact any) ([32]byte, error) {
	encoded, err := json.Marshal(fact)
	if err != nil {
		return [32]byte{}, automationport.ErrAutomationHistoryInvalid
	}
	return sha256.Sum256(append([]byte("automation_history\x00"+kind+"\x00"), encoded...)), nil
}

func normalizeAutomationSOP(fact automationport.HistoricalAutomationSOP) automationport.HistoricalAutomationSOP {
	fact.CreatedAt, fact.UpdatedAt = automationHistoryMicro(fact.CreatedAt), automationHistoryMicro(fact.UpdatedAt)
	return fact
}

func normalizeAutomationConfig(fact automationport.HistoricalAutomationConfig) automationport.HistoricalAutomationConfig {
	fact.CreatedAt, fact.UpdatedAt = automationHistoryMicro(fact.CreatedAt), automationHistoryMicro(fact.UpdatedAt)
	return fact
}

func normalizeAutomationPrompt(fact automationport.HistoricalAutomationPrompt) automationport.HistoricalAutomationPrompt {
	fact.CreatedAt, fact.UpdatedAt = automationHistoryMicro(fact.CreatedAt), automationHistoryMicro(fact.UpdatedAt)
	return fact
}

func normalizeAutomationAgent(fact automationport.HistoricalAutomationAgent) automationport.HistoricalAutomationAgent {
	fact.CreatedAt, fact.UpdatedAt = automationHistoryMicro(fact.CreatedAt), automationHistoryMicro(fact.UpdatedAt)
	return fact
}

func validAutomationSOP(fact automationport.HistoricalAutomationSOP, stored bool) bool {
	return validAutomationIdentity(fact.HistoricalAutomationIdentity, stored) && fact.ImagesDigest != [32]byte{} &&
		automationHistoryText(fact.PoolKey, fact.ContentMasked) && automationHistoryTimes(fact.CreatedAt, fact.UpdatedAt)
}

func validAutomationConfig(fact automationport.HistoricalAutomationConfig, stored bool) bool {
	return validAutomationIdentity(fact.HistoricalAutomationIdentity, stored) && fact.ActorsDigest != [32]byte{} && fact.ConfigDigest != [32]byte{} &&
		automationHistoryText(fact.AgentCode, fact.DisplayName, fact.ScenarioCode, fact.PublishedAt, fact.LastModifiedAt, fact.LastModifiedSource, fact.SubmittedAt) && automationHistoryTimes(fact.CreatedAt, fact.UpdatedAt)
}

func validAutomationPrompt(fact automationport.HistoricalAutomationPrompt, stored bool) bool {
	return validAutomationIdentity(fact.HistoricalAutomationIdentity, stored) && fact.PromptDigest != [32]byte{} &&
		automationHistoryText(fact.AgentCode, fact.DisplayName) && automationHistoryTimes(fact.CreatedAt, fact.UpdatedAt)
}

func validAutomationAgent(fact automationport.HistoricalAutomationAgent, stored bool) bool {
	return validAutomationIdentity(fact.HistoricalAutomationIdentity, stored) && fact.ActorsDigest != [32]byte{} && fact.ConfigurationDigest != [32]byte{} &&
		automationHistoryText(fact.AgentCode, fact.AgentName, fact.OriginalType, fact.OriginalStatus, fact.ArchivedAt) && automationHistoryTimes(fact.CreatedAt, fact.UpdatedAt)
}

func validAutomationIdentity(identity automationport.HistoricalAutomationIdentity, stored bool) bool {
	if (stored && identity.ID < 1) || (!stored && identity.ID != 0) {
		return false
	}
	return identity.SourceID > 0 && identity.SourceKeyDigest != [32]byte{} && identity.SourcePayloadDigest != [32]byte{}
}

func validAutomationSource(source string, identity automationport.HistoricalAutomationIdentity) bool {
	if source == "" || strings.TrimSpace(source) != source || !automationHistoryText(source) {
		return false
	}
	decoded, err := hex.DecodeString(source)
	return err == nil && len(decoded) == len(identity.SourceKeyDigest) && source == strings.ToLower(source) && source == hex.EncodeToString(identity.SourceKeyDigest[:])
}

func automationHistoryMicro(value time.Time) time.Time { return value.UTC().Truncate(time.Microsecond) }

func automationHistoryTimes(created, updated time.Time) bool {
	return automationHistoryTime(created) && automationHistoryTime(updated)
}

func automationHistoryTime(value time.Time) bool {
	return !value.IsZero() && value.Year() >= 1 && value.Year() <= 9999
}

func automationHistoryText(values ...string) bool {
	for _, value := range values {
		if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
			return false
		}
	}
	return true
}

func automationHistoryWriteError(err error) error {
	switch {
	case errors.Is(err, automationport.ErrAutomationHistoryInvalid):
		return automationport.ErrAutomationHistoryInvalid
	case errors.Is(err, automationport.ErrAutomationHistoryConflict):
		return automationport.ErrAutomationHistoryConflict
	default:
		return automationport.ErrAutomationHistoryUnavailable
	}
}

func nilAutomationHistoryDependency(value any) bool {
	if value == nil {
		return true
	}
	reflectValue := reflect.ValueOf(value)
	return reflectValue.Kind() == reflect.Ptr && reflectValue.IsNil()
}
