package store

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	automation "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	automationdb "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var marketingConfigHistoryPostgresDSN = flag.String("marketing-config-history-postgres-dsn", "", "isolated PostgreSQL DSN for schema 126 rollback verification")

func TestMarketingConfigHistoryReaderMappingAndStrictDependencies(t *testing.T) {
	ctx := context.Background()
	if _, err := NewMarketingConfigHistoryStore().GetHistoricalMarketingAutomationConfig(ctx, 1); !errors.Is(err, automation.ErrMarketingConfigHistoryUnavailable) {
		t.Fatal(err)
	}
	var pool *pgxpool.Pool
	for _, reader := range []*MarketingConfigHistoryReader{nil, NewMarketingConfigHistoryReader(nil), NewMarketingConfigHistoryReader(pool)} {
		if _, _, err := reader.ListHistoricalMarketingAutomationConfig(ctx, automation.MarketingConfigHistoryQuery{Limit: 1}); !errors.Is(err, automation.ErrMarketingConfigHistoryUnavailable) {
			t.Fatal(err)
		}
	}
	for _, query := range []automation.MarketingConfigHistoryQuery{{}, {Limit: 101}, {Limit: 1, Offset: -1}} {
		if _, _, err := NewMarketingConfigHistoryReader(nil).ListHistoricalMarketingAutomationRule(ctx, query); !errors.Is(err, automation.ErrMarketingConfigHistoryInvalid) {
			t.Fatal(err)
		}
	}
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	config, err := marketingConfigValue(automationdb.AutomationV1MarketingConfigHistory{ID: 1, SourceKeyDigest: marketingConfigBytes(1), SourcePayloadDigest: marketingConfigBytes(2), SourceFieldDigest: marketingConfigBytes(3), SourceID: -7, AutomationName: "历史营销", OriginalStatus: "enabled", CreatedAt: marketingConfigStoredTime(at), UpdatedAt: marketingConfigStoredTime(at.Add(-time.Second)), ConfigPayloadDigest: marketingConfigBytes(4)})
	if err != nil || config.SourceID != -7 || config.DoNotStartAfterHour != 0 || config.AutomationName != "历史营销" || !config.UpdatedAt.Before(config.CreatedAt) {
		t.Fatalf("config=%+v err=%v", config, err)
	}
	questionnaire := int64(-3)
	rule, err := marketingRuleValue(automationdb.AutomationV1MarketingRuleHistory{ID: 2, SourceKeyDigest: marketingConfigBytes(5), SourcePayloadDigest: marketingConfigBytes(6), SourceFieldDigest: marketingConfigBytes(7), SourceID: 0, ConfigID: 1, ConfigSourceID: -7, QuestionnaireSourceID: pgtype.Int8{Int64: questionnaire, Valid: true}, RuleName: "规则", CreatedAt: marketingConfigStoredTime(at), UpdatedAt: marketingConfigStoredTime(at.Add(-time.Second)), AnswerMatchValueDigest: marketingConfigBytes(8), RulePayloadDigest: marketingConfigBytes(9)})
	if err != nil || rule.SourceID != 0 || rule.QuestionnaireSourceID == nil || *rule.QuestionnaireSourceID != questionnaire || rule.QuestionSourceID != nil || rule.ScoreDelta != 0 || !rule.UpdatedAt.Before(rule.CreatedAt) {
		t.Fatalf("rule=%+v err=%v", rule, err)
	}
}

func TestMarketingConfigHistoryPostgresRoundTripRollback(t *testing.T) {
	if *marketingConfigHistoryPostgresDSN == "" {
		t.Skip("set -marketing-config-history-postgres-dsn for isolated schema 126 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *marketingConfigHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	queries := automationdb.New(pool)
	beforeConfigs, err := queries.CountHistoricalMarketingAutomationConfig(ctx)
	if err != nil {
		t.Fatal("marketing-config-history stage=count-configs")
	}
	beforeRules, err := queries.CountHistoricalMarketingAutomationRule(ctx)
	if err != nil {
		t.Fatal("marketing-config-history stage=count-rules")
	}
	rollback := errors.New("marketing config history rollback")
	var configID, ruleID int64
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		store := NewMarketingConfigHistoryStore()
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return fmt.Errorf("marketing-config-history stage=tx: %w", err)
		}
		poolReader, txReader := NewMarketingConfigHistoryReader(pool), NewMarketingConfigHistoryReader(tx)
		at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
		config, err := store.CreateHistoricalMarketingAutomationConfig(txCtx, marketingConfigStoreConfig(at))
		if err != nil {
			return fmt.Errorf("marketing-config-history stage=config-create: %w", err)
		}
		configID = config.ID
		if loaded, err := poolReader.GetHistoricalMarketingAutomationConfig(txCtx, config.ID); err != nil || !reflect.DeepEqual(loaded, config) {
			return fmt.Errorf("marketing-config-history stage=config-caller-tx: %w", err)
		}
		if loaded, err := txReader.GetHistoricalMarketingAutomationConfig(context.Background(), config.ID); err != nil || !reflect.DeepEqual(loaded, config) {
			return fmt.Errorf("marketing-config-history stage=config-bare-tx: %w", err)
		}
		ruleInput := marketingConfigStoreRule(at)
		ruleInput.ConfigID = config.ID
		rule, err := store.CreateHistoricalMarketingAutomationRule(txCtx, ruleInput)
		if err != nil {
			return fmt.Errorf("marketing-config-history stage=rule-create: %w", err)
		}
		ruleID = rule.ID
		if loaded, err := txReader.GetHistoricalMarketingAutomationRule(context.Background(), rule.ID); err != nil || !reflect.DeepEqual(loaded, rule) {
			return fmt.Errorf("marketing-config-history stage=rule-bare-tx: %w", err)
		}
		if values, total, err := poolReader.ListHistoricalMarketingAutomationRule(txCtx, automation.MarketingConfigHistoryQuery{Limit: 1, Offset: int32(beforeRules + 1)}); err != nil || total != beforeRules+1 || len(values) != 0 {
			return fmt.Errorf("marketing-config-history stage=rule-page: %w", err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	afterConfigs, configErr := queries.CountHistoricalMarketingAutomationConfig(ctx)
	afterRules, ruleErr := queries.CountHistoricalMarketingAutomationRule(ctx)
	if configErr != nil || ruleErr != nil || afterConfigs != beforeConfigs || afterRules != beforeRules {
		t.Fatal("marketing-config-history rollback retained rows")
	}
	if _, err := queries.GetHistoricalMarketingAutomationConfig(ctx, configID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("config row remains after rollback")
	}
	if _, err := queries.GetHistoricalMarketingAutomationRule(ctx, ruleID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("rule row remains after rollback")
	}
}

func marketingConfigBytes(value byte) []byte {
	result := make([]byte, 32)
	result[0] = value
	return result
}
func marketingConfigArray(value byte) [32]byte { var result [32]byte; result[0] = value; return result }
func marketingConfigStoredTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func marketingConfigStoreConfig(at time.Time) automation.HistoricalMarketingAutomationConfig {
	return automation.HistoricalMarketingAutomationConfig{SourceKeyDigest: marketingConfigArray(11), SourcePayloadDigest: marketingConfigArray(12), SourceFieldDigest: marketingConfigArray(13), SourceID: -7, AutomationKey: "", AutomationName: "历史营销", TargetEvent: "", ChannelType: "", OriginalStatus: "enabled", DoNotStartAfterHour: -1, CreatedAt: at, UpdatedAt: at.Add(-time.Second), ConfigPayloadDigest: marketingConfigArray(14)}
}
func marketingConfigStoreRule(at time.Time) automation.HistoricalMarketingAutomationRule {
	questionnaire := int64(-3)
	return automation.HistoricalMarketingAutomationRule{SourceKeyDigest: marketingConfigArray(15), SourcePayloadDigest: marketingConfigArray(16), SourceFieldDigest: marketingConfigArray(17), SourceID: 0, ConfigID: 1, ConfigSourceID: -7, QuestionnaireSourceID: &questionnaire, RuleCode: "", RuleName: "规则", AnswerMatchType: "", ScoreDelta: -2, SegmentHint: "", StageHint: "", OriginalActive: false, SortOrder: -4, CreatedAt: at, UpdatedAt: at.Add(-time.Second), AnswerMatchValueDigest: marketingConfigArray(18), RulePayloadDigest: marketingConfigArray(19)}
}
