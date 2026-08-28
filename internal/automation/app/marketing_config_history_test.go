package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	automation "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
)

func TestMarketingConfigHistoryWriterReplayConflictAndTimeNormalization(t *testing.T) {
	at := time.Date(2026, 8, 28, 9, 2, 3, 123456789, time.FixedZone("+8", 8*3600))
	store, journal := &marketingConfigHistoryFakeStore{}, &marketingConfigHistoryFakeJournal{receipts: map[string]automation.MarketingConfigHistoryReceipt{}}
	writer, err := NewMarketingConfigHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	config := marketingConfigFact(at)
	source := hex.EncodeToString(config.SourceKeyDigest[:])
	receipt, err := writer.ImportHistoricalMarketingAutomationConfig(context.Background(), source, config)
	if err != nil || receipt.Kind != marketingConfigHistoryConfigKind || receipt.TargetID != 1 || receipt.Replayed {
		t.Fatalf("config receipt=%+v err=%v", receipt, err)
	}
	if store.config.CreatedAt.Location() != time.UTC || store.config.CreatedAt.Nanosecond() != 123456000 || !store.config.UpdatedAt.Before(store.config.CreatedAt) {
		t.Fatal("config source time fidelity changed")
	}
	if replay, err := writer.ImportHistoricalMarketingAutomationConfig(context.Background(), source, config); err != nil || !replay.Replayed {
		t.Fatalf("config replay=%+v err=%v", replay, err)
	}
	config.ConfigPayloadDigest = marketingConfigDigest(91)
	if _, err := writer.ImportHistoricalMarketingAutomationConfig(context.Background(), source, config); !errors.Is(err, automation.ErrMarketingConfigHistoryConflict) {
		t.Fatalf("config private drift err=%v", err)
	}

	rule := marketingRuleFact(at)
	ruleSource := hex.EncodeToString(rule.SourceKeyDigest[:])
	if receipt, err := writer.ImportHistoricalMarketingAutomationRule(context.Background(), ruleSource, rule); err != nil || receipt.Kind != marketingConfigHistoryRuleKind || receipt.TargetID != 2 {
		t.Fatalf("rule receipt=%+v err=%v", receipt, err)
	}
	if store.rule.QuestionnaireSourceID == nil || *store.rule.QuestionnaireSourceID != -3 || store.rule.QuestionSourceID != nil || store.rule.CreatedAt.Location() != time.UTC || store.rule.CreatedAt.Nanosecond() != 123456000 {
		t.Fatal("rule nullable or time fidelity changed")
	}
	if _, err := writer.ImportHistoricalMarketingAutomationRule(context.Background(), "bad", rule); !errors.Is(err, automation.ErrMarketingConfigHistoryInvalid) {
		t.Fatalf("bad source err=%v", err)
	}
}

func TestMarketingConfigHistoryDigestsBindPrivateFacts(t *testing.T) {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	config := marketingConfigFact(at)
	config.ID = 1
	before, err := HistoricalMarketingAutomationConfigDigest(config)
	if err != nil {
		t.Fatal(err)
	}
	config.SourceFieldDigest = marketingConfigDigest(99)
	after, err := HistoricalMarketingAutomationConfigDigest(config)
	if err != nil || before == after {
		t.Fatalf("config private source-field drift omitted err=%v", err)
	}
	rule := marketingRuleFact(at)
	rule.ID = 2
	before, err = HistoricalMarketingAutomationRuleDigest(rule)
	if err != nil {
		t.Fatal(err)
	}
	rule.RulePayloadDigest = marketingConfigDigest(98)
	after, err = HistoricalMarketingAutomationRuleDigest(rule)
	if err != nil || before == after {
		t.Fatalf("rule private payload drift omitted err=%v", err)
	}
}

func TestMarketingConfigHistoryWriterRejectsUnavailableAndTargetDrift(t *testing.T) {
	if _, err := NewMarketingConfigHistoryWriter((*marketingConfigHistoryFakeStore)(nil), &marketingConfigHistoryFakeJournal{}); !errors.Is(err, automation.ErrMarketingConfigHistoryUnavailable) {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 28, 1, 2, 3, 0, time.UTC)
	store := &marketingConfigHistoryFakeStore{}
	journal := &marketingConfigHistoryFakeJournal{receipts: map[string]automation.MarketingConfigHistoryReceipt{}}
	writer, err := NewMarketingConfigHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	config := marketingConfigFact(at)
	source := hex.EncodeToString(config.SourceKeyDigest[:])
	if _, err := writer.ImportHistoricalMarketingAutomationConfig(context.Background(), source, config); err != nil {
		t.Fatal(err)
	}
	store.config.AutomationName = "drift"
	if _, err := writer.ImportHistoricalMarketingAutomationConfig(context.Background(), source, config); !errors.Is(err, automation.ErrMarketingConfigHistoryConflict) {
		t.Fatalf("target drift err=%v", err)
	}
}

type marketingConfigHistoryFakeStore struct {
	config automation.HistoricalMarketingAutomationConfig
	rule   automation.HistoricalMarketingAutomationRule
}

func (s *marketingConfigHistoryFakeStore) CreateHistoricalMarketingAutomationConfig(_ context.Context, value automation.HistoricalMarketingAutomationConfig) (automation.HistoricalMarketingAutomationConfig, error) {
	value.ID = 1
	s.config = value
	return value, nil
}
func (s *marketingConfigHistoryFakeStore) GetHistoricalMarketingAutomationConfig(_ context.Context, id int64) (automation.HistoricalMarketingAutomationConfig, error) {
	if s.config.ID != id {
		return automation.HistoricalMarketingAutomationConfig{}, automation.ErrMarketingConfigHistoryUnavailable
	}
	return s.config, nil
}
func (s *marketingConfigHistoryFakeStore) CreateHistoricalMarketingAutomationRule(_ context.Context, value automation.HistoricalMarketingAutomationRule) (automation.HistoricalMarketingAutomationRule, error) {
	value.ID = 2
	s.rule = value
	return value, nil
}
func (s *marketingConfigHistoryFakeStore) GetHistoricalMarketingAutomationRule(_ context.Context, id int64) (automation.HistoricalMarketingAutomationRule, error) {
	if s.rule.ID != id {
		return automation.HistoricalMarketingAutomationRule{}, automation.ErrMarketingConfigHistoryUnavailable
	}
	return s.rule, nil
}

type marketingConfigHistoryFakeJournal struct {
	receipts map[string]automation.MarketingConfigHistoryReceipt
}

func (j *marketingConfigHistoryFakeJournal) LoadMarketingConfigHistory(_ context.Context, kind, source string) (automation.MarketingConfigHistoryReceipt, bool, error) {
	receipt, found := j.receipts[kind+"/"+source]
	return receipt, found, nil
}
func (j *marketingConfigHistoryFakeJournal) RecordMarketingConfigHistory(_ context.Context, value automation.MarketingConfigHistoryReceipt) error {
	if j.receipts == nil {
		j.receipts = map[string]automation.MarketingConfigHistoryReceipt{}
	}
	j.receipts[value.Kind+"/"+value.SourceIdentifier] = value
	return nil
}

func marketingConfigDigest(value byte) [32]byte { return sha256.Sum256([]byte{value}) }
func marketingConfigFact(at time.Time) automation.HistoricalMarketingAutomationConfig {
	return automation.HistoricalMarketingAutomationConfig{SourceKeyDigest: marketingConfigDigest(1), SourcePayloadDigest: marketingConfigDigest(2), SourceFieldDigest: marketingConfigDigest(3), SourceID: -7, AutomationKey: "", AutomationName: "历史营销", TargetEvent: "", ChannelType: "", OriginalStatus: "enabled", DoNotStartAfterHour: -1, CreatedAt: at, UpdatedAt: at.Add(-time.Second), ConfigPayloadDigest: marketingConfigDigest(4)}
}
func marketingRuleFact(at time.Time) automation.HistoricalMarketingAutomationRule {
	questionnaire := int64(-3)
	return automation.HistoricalMarketingAutomationRule{SourceKeyDigest: marketingConfigDigest(5), SourcePayloadDigest: marketingConfigDigest(6), SourceFieldDigest: marketingConfigDigest(7), SourceID: 0, ConfigID: 1, ConfigSourceID: -7, QuestionnaireSourceID: &questionnaire, RuleCode: "", RuleName: "规则", AnswerMatchType: "", ScoreDelta: -2, SegmentHint: "", StageHint: "", OriginalActive: false, SortOrder: -4, CreatedAt: at, UpdatedAt: at.Add(-time.Second), AnswerMatchValueDigest: marketingConfigDigest(8), RulePayloadDigest: marketingConfigDigest(9)}
}
