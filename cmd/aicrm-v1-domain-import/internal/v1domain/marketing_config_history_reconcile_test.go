package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	automationapp "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/app"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
)

type marketingConfigHistoryReconcileReader struct {
	config automationport.HistoricalMarketingAutomationConfig
	rule   automationport.HistoricalMarketingAutomationRule
	err    error
}

func (reader *marketingConfigHistoryReconcileReader) GetHistoricalMarketingAutomationConfig(_ context.Context, id int64) (automationport.HistoricalMarketingAutomationConfig, error) {
	if reader == nil || reader.err != nil || reader.config.ID != id {
		return automationport.HistoricalMarketingAutomationConfig{}, errors.New("historical config unavailable")
	}
	return reader.config, nil
}

func (reader *marketingConfigHistoryReconcileReader) ListHistoricalMarketingAutomationConfig(context.Context, automationport.MarketingConfigHistoryQuery) ([]automationport.HistoricalMarketingAutomationConfig, int64, error) {
	if reader == nil || reader.err != nil {
		return nil, 0, errors.New("historical config unavailable")
	}
	return []automationport.HistoricalMarketingAutomationConfig{reader.config}, 1, nil
}

func (reader *marketingConfigHistoryReconcileReader) GetHistoricalMarketingAutomationRule(_ context.Context, id int64) (automationport.HistoricalMarketingAutomationRule, error) {
	if reader == nil || reader.err != nil || reader.rule.ID != id {
		return automationport.HistoricalMarketingAutomationRule{}, errors.New("historical rule unavailable")
	}
	return reader.rule, nil
}

func (reader *marketingConfigHistoryReconcileReader) ListHistoricalMarketingAutomationRule(context.Context, automationport.MarketingConfigHistoryQuery) ([]automationport.HistoricalMarketingAutomationRule, int64, error) {
	if reader == nil || reader.err != nil {
		return nil, 0, errors.New("historical rule unavailable")
	}
	return []automationport.HistoricalMarketingAutomationRule{reader.rule}, 1, nil
}

func TestVerifyMarketingConfigHistoryRowBindsCompleteTargets(t *testing.T) {
	reader := marketingConfigHistoryReconcileFixture(t)
	tests := []struct {
		name, table, target string
		id                  int64
		digest              [sha256.Size]byte
		privateDrift        func(*marketingConfigHistoryReconcileReader)
	}{
		{"config", "public/marketing_automation_configs", "automation_v1_marketing_config_history", reader.config.ID, mustMarketingConfigHistoryDigest(t, reader.config), func(reader *marketingConfigHistoryReconcileReader) { reader.config.ConfigPayloadDigest[0]++ }},
		{"rule", "public/marketing_automation_question_rules", "automation_v1_marketing_rule_history", reader.rule.ID, mustMarketingConfigHistoryRuleDigest(t, reader.rule), func(reader *marketingConfigHistoryReconcileReader) { reader.rule.AnswerMatchValueDigest[0]++ }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			row := marketingConfigHistoryReconciliationRow(test.table, test.target, test.id, test.digest, reader)
			proof, err := verifyMarketingConfigHistoryRow(context.Background(), reader, row)
			if err != nil || proof != "history_only:"+hex.EncodeToString(row.TargetDigest) {
				t.Fatalf("proof=%q err=%v", proof, err)
			}
			for name, mutate := range map[string]func(*marketingConfigHistoryReconcileReader, *reconciliationRow){
				"source key hmac": func(_ *marketingConfigHistoryReconcileReader, row *reconciliationRow) { row.SourceKeyDigest[0]++ },
				"payload hmac":    func(_ *marketingConfigHistoryReconcileReader, row *reconciliationRow) { row.PayloadDigest[0]++ },
				"field hmac":      func(_ *marketingConfigHistoryReconcileReader, row *reconciliationRow) { row.FieldDigest[0]++ },
				"target digest":   func(_ *marketingConfigHistoryReconcileReader, row *reconciliationRow) { row.TargetDigest[0]++ },
				"source table": func(_ *marketingConfigHistoryReconcileReader, row *reconciliationRow) {
					row.TableID = "public/marketing_automation_configs_extra"
				},
				"target domain": func(_ *marketingConfigHistoryReconcileReader, row *reconciliationRow) {
					value := "campaign"
					row.TargetDomain = &value
				},
				"target table": func(_ *marketingConfigHistoryReconcileReader, row *reconciliationRow) {
					value := "automation_configs"
					row.TargetTable = &value
				},
				"non canonical id": func(_ *marketingConfigHistoryReconcileReader, row *reconciliationRow) {
					value := "0" + strconv.FormatInt(test.id, 10)
					row.TargetID = &value
				},
				"actual target id": func(reader *marketingConfigHistoryReconcileReader, _ *reconciliationRow) {
					if test.name == "config" {
						reader.config.ID++
					} else {
						reader.rule.ID++
					}
				},
				"private field digest": func(reader *marketingConfigHistoryReconcileReader, _ *reconciliationRow) { test.privateDrift(reader) },
				"reader error": func(reader *marketingConfigHistoryReconcileReader, _ *reconciliationRow) {
					reader.err = errors.New("unavailable")
				},
			} {
				t.Run(name, func(t *testing.T) {
					candidate := marketingConfigHistoryReconcileFixture(t)
					candidateRow := marketingConfigHistoryReconciliationRow(test.table, test.target, test.id, test.digest, candidate)
					mutate(candidate, &candidateRow)
					if _, err := verifyMarketingConfigHistoryRow(context.Background(), candidate, candidateRow); !errors.Is(err, ErrConflict) {
						t.Fatalf("err=%v", err)
					}
				})
			}
		})
	}

	reader = marketingConfigHistoryReconcileFixture(t)
	row := marketingConfigHistoryReconciliationRow("public/marketing_automation_configs", "automation_v1_marketing_config_history", reader.config.ID, mustMarketingConfigHistoryDigest(t, reader.config), reader)
	var typedNil *marketingConfigHistoryReconcileReader
	if _, err := verifyMarketingConfigHistoryRow(context.Background(), typedNil, row); !errors.Is(err, ErrConflict) {
		t.Fatalf("typed nil err=%v", err)
	}
}

func TestReconcileMarketingConfigHistoryRejectsWrongVersionBeforeDatabase(t *testing.T) {
	var pool *pgxpool.Pool
	if _, err := ReconcileMarketingConfigHistory(context.Background(), pool, "v1-marketing-config-history-a2", "archive"); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("err=%v", err)
	}
}

func marketingConfigHistoryReconcileFixture(t *testing.T) *marketingConfigHistoryReconcileReader {
	t.Helper()
	at := time.Date(2026, 8, 28, 8, 9, 10, 123456000, time.UTC)
	return &marketingConfigHistoryReconcileReader{
		config: automationport.HistoricalMarketingAutomationConfig{
			ID: 71, SourceID: 17, SourceKeyDigest: marketingConfigHistoryDigest(1), SourcePayloadDigest: marketingConfigHistoryDigest(2), SourceFieldDigest: marketingConfigHistoryDigest(3), ConfigPayloadDigest: marketingConfigHistoryDigest(4),
			AutomationKey: "legacy-key", AutomationName: "", TargetEvent: "answer", ChannelType: "qrcode", OriginalStatus: "disabled", DoNotStartAfterHour: -1, CreatedAt: at, UpdatedAt: at,
		},
		rule: automationport.HistoricalMarketingAutomationRule{
			ID: 72, SourceID: 18, ConfigID: 71, ConfigSourceID: 17, SourceKeyDigest: marketingConfigHistoryDigest(5), SourcePayloadDigest: marketingConfigHistoryDigest(6), SourceFieldDigest: marketingConfigHistoryDigest(7),
			AnswerMatchValueDigest: marketingConfigHistoryDigest(8), RulePayloadDigest: marketingConfigHistoryDigest(9), RuleCode: "", RuleName: "legacy", AnswerMatchType: "exact", ScoreDelta: -3, SegmentHint: "", StageHint: "", OriginalActive: false, SortOrder: -1, CreatedAt: at, UpdatedAt: at,
		},
	}
}

func marketingConfigHistoryReconciliationRow(table, target string, id int64, digest [sha256.Size]byte, reader *marketingConfigHistoryReconcileReader) reconciliationRow {
	var key, payload, field [sha256.Size]byte
	if table == "public/marketing_automation_configs" {
		key, payload, field = reader.config.SourceKeyDigest, reader.config.SourcePayloadDigest, reader.config.SourceFieldDigest
	} else {
		key, payload, field = reader.rule.SourceKeyDigest, reader.rule.SourcePayloadDigest, reader.rule.SourceFieldDigest
	}
	domain, targetID := "automation", strconv.FormatInt(id, 10)
	return reconciliationRow{TableID: table, SourceKeyDigest: key[:], PayloadDigest: payload[:], FieldDigest: field[:], TargetDomain: &domain, TargetTable: &target, TargetID: &targetID, TargetDigest: digest[:]}
}

func mustMarketingConfigHistoryDigest(t *testing.T, value automationport.HistoricalMarketingAutomationConfig) [sha256.Size]byte {
	t.Helper()
	digest, err := automationapp.HistoricalMarketingAutomationConfigDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func mustMarketingConfigHistoryRuleDigest(t *testing.T, value automationport.HistoricalMarketingAutomationRule) [sha256.Size]byte {
	t.Helper()
	digest, err := automationapp.HistoricalMarketingAutomationRuleDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func marketingConfigHistoryDigest(first byte) [sha256.Size]byte {
	var value [sha256.Size]byte
	value[0] = first
	return value
}
