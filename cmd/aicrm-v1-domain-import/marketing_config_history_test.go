package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1marketingconfighistory"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

type marketingHistoryArchive struct {
	rows map[string][]v1archive.ArchivedRow
}

func (a marketingHistoryArchive) EachTableRow(_ context.Context, _ string, table string, callback func(v1archive.ArchivedRow) error) error {
	for _, row := range a.rows[table] {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type marketingHistoryTx struct{}
type marketingHistoryRuntime struct {
	terminals map[string]v1domain.TerminalReceipt
	configs   map[int64]automationport.HistoricalMarketingAutomationConfig
	rules     map[int64]automationport.HistoricalMarketingAutomationRule
	writes    int
}

func newMarketingHistoryRuntime() *marketingHistoryRuntime {
	return &marketingHistoryRuntime{terminals: map[string]v1domain.TerminalReceipt{}, configs: map[int64]automationport.HistoricalMarketingAutomationConfig{}, rules: map[int64]automationport.HistoricalMarketingAutomationRule{}}
}
func (r *marketingHistoryRuntime) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(context.WithValue(ctx, marketingHistoryTx{}, true))
}
func marketingTerminalKey(kind, source string) string { return kind + "\x00" + source }
func (r *marketingHistoryRuntime) LoadMarketingConfigHistoryTerminal(ctx context.Context, kind, source string) (v1domain.TerminalReceipt, bool, error) {
	if ctx.Value(marketingHistoryTx{}) != true {
		return v1domain.TerminalReceipt{}, false, v1domain.ErrInvalidScope
	}
	v, ok := r.terminals[marketingTerminalKey(kind, source)]
	return v, ok, nil
}
func (r *marketingHistoryRuntime) RecordMarketingConfigHistoryTerminal(ctx context.Context, kind string, value v1domain.TerminalReceipt) error {
	if ctx.Value(marketingHistoryTx{}) != true {
		return v1domain.ErrInvalidScope
	}
	key := marketingTerminalKey(kind, v1domain.SourceIdentifier(value.SourceKeyDigest))
	if old, found := r.terminals[key]; found && !reflect.DeepEqual(old, value) {
		return v1domain.ErrConflict
	}
	r.terminals[key] = value
	return nil
}
func (r *marketingHistoryRuntime) LoadMarketingConfigHistory(ctx context.Context, kind, source string) (automationport.MarketingConfigHistoryReceipt, bool, error) {
	terminal, found, err := r.LoadMarketingConfigHistoryTerminal(ctx, kind, source)
	if err != nil || !found {
		return automationport.MarketingConfigHistoryReceipt{}, found, err
	}
	id, e := strconv.ParseInt(terminal.TargetID, 10, 64)
	if e != nil || terminal.Disposition != "import" {
		return automationport.MarketingConfigHistoryReceipt{}, false, v1domain.ErrConflict
	}
	return automationport.MarketingConfigHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetID: id, TargetDigest: terminal.TargetDigest}, true, nil
}
func (r *marketingHistoryRuntime) RecordMarketingConfigHistory(context.Context, automationport.MarketingConfigHistoryReceipt) error {
	return v1domain.ErrInvalidScope
}
func (r *marketingHistoryRuntime) ImportHistoricalMarketingAutomationConfig(ctx context.Context, source string, value automationport.HistoricalMarketingAutomationConfig) (automationport.MarketingConfigHistoryReceipt, error) {
	if ctx.Value(marketingHistoryTx{}) != true {
		return automationport.MarketingConfigHistoryReceipt{}, automationport.ErrMarketingConfigHistoryUnavailable
	}
	return r.writeConfig(ctx, source, value)
}
func (r *marketingHistoryRuntime) ImportHistoricalMarketingAutomationRule(ctx context.Context, source string, value automationport.HistoricalMarketingAutomationRule) (automationport.MarketingConfigHistoryReceipt, error) {
	if ctx.Value(marketingHistoryTx{}) != true {
		return automationport.MarketingConfigHistoryReceipt{}, automationport.ErrMarketingConfigHistoryUnavailable
	}
	return r.writeRule(ctx, source, value)
}
func (r *marketingHistoryRuntime) writeConfig(ctx context.Context, source string, value automationport.HistoricalMarketingAutomationConfig) (automationport.MarketingConfigHistoryReceipt, error) {
	if terminal, found, _ := r.LoadMarketingConfigHistoryTerminal(ctx, v1domain.MarketingConfigHistoryConfigKind, source); found {
		id, _ := strconv.ParseInt(terminal.TargetID, 10, 64)
		value.ID = id
		if !reflect.DeepEqual(r.configs[id], value) {
			return automationport.MarketingConfigHistoryReceipt{}, automationport.ErrMarketingConfigHistoryConflict
		}
		return automationport.MarketingConfigHistoryReceipt{Kind: v1domain.MarketingConfigHistoryConfigKind, SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetID: id, TargetDigest: terminal.TargetDigest, Replayed: true}, nil
	}
	r.writes++
	id := int64(101)
	value.ID = id
	r.configs[id] = value
	return r.recordMarketingImport(ctx, v1domain.MarketingConfigHistoryConfigKind, source, value.SourceKeyDigest, value.SourcePayloadDigest, id)
}
func (r *marketingHistoryRuntime) writeRule(ctx context.Context, source string, value automationport.HistoricalMarketingAutomationRule) (automationport.MarketingConfigHistoryReceipt, error) {
	if terminal, found, _ := r.LoadMarketingConfigHistoryTerminal(ctx, v1domain.MarketingConfigHistoryRuleKind, source); found {
		id, _ := strconv.ParseInt(terminal.TargetID, 10, 64)
		value.ID = id
		if !reflect.DeepEqual(r.rules[id], value) {
			return automationport.MarketingConfigHistoryReceipt{}, automationport.ErrMarketingConfigHistoryConflict
		}
		return automationport.MarketingConfigHistoryReceipt{Kind: v1domain.MarketingConfigHistoryRuleKind, SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetID: id, TargetDigest: terminal.TargetDigest, Replayed: true}, nil
	}
	r.writes++
	id := int64(200 + len(r.rules))
	value.ID = id
	r.rules[id] = value
	return r.recordMarketingImport(ctx, v1domain.MarketingConfigHistoryRuleKind, source, value.SourceKeyDigest, value.SourcePayloadDigest, id)
}
func (r *marketingHistoryRuntime) recordMarketingImport(ctx context.Context, kind, source string, key, payload [sha256.Size]byte, id int64) (automationport.MarketingConfigHistoryReceipt, error) {
	digest := sha256.Sum256([]byte(kind + "/" + source))
	receipt := automationport.MarketingConfigHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetID: id, TargetDigest: digest}
	return receipt, r.RecordMarketingConfigHistoryTerminal(ctx, kind, v1domain.TerminalReceipt{SourceKeyDigest: key, PayloadDigest: payload, Disposition: "import", TargetID: fmt.Sprint(id), TargetDigest: digest})
}

func TestMarketingConfigHistoryImporterMapsParentAndPrivateFacts(t *testing.T) {
	config := marketingConfigRow(t, 1)
	rules := []v1archive.ArchivedRow{marketingRuleRow(t, 1, 1, nil, nil), marketingRuleRow(t, 2, 1, ptrMarketingInt64(-7), nil), marketingRuleRow(t, 3, 1, nil, ptrMarketingInt64(0))}
	archive := marketingHistoryArchive{rows: map[string][]v1archive.ArchivedRow{v1marketingconfighistory.ConfigTableID: {config}, v1marketingconfighistory.RulesTableID: rules}}
	runtime := newMarketingHistoryRuntime()
	importer, err := v1domain.NewMarketingConfigHistoryImporter(archive, runtime, runtime, runtime)
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(context.Background(), "run")
	if err != nil || result != (v1domain.MarketingConfigHistoryImportResult{ImportedConfigs: 1, ImportedRules: 3}) || runtime.writes != 4 {
		t.Fatal("marketing_config_import_failed")
	}
	rule := runtime.rules[201]
	if rule.ConfigID != 101 || rule.ConfigSourceID != 1 || rule.QuestionnaireSourceID == nil || *rule.QuestionnaireSourceID != -7 || rule.QuestionSourceID != nil || rule.AnswerMatchValueDigest == ([32]byte{}) || rule.RulePayloadDigest == ([32]byte{}) || rule.CreatedAt != time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC) {
		t.Fatal("marketing_config_rule_changed")
	}
	encoded, _ := json.Marshal(rule)
	if containsAny(string(encoded), "answer-private", "payload-private") {
		t.Fatal("marketing_config_private_field_serialized")
	}
	replayed, err := importer.Import(context.Background(), "run")
	if err != nil || replayed != (v1domain.MarketingConfigHistoryImportResult{ImportedConfigs: 1, ImportedRules: 3, Replayed: 4}) || runtime.writes != 4 {
		t.Fatal("marketing_config_replay_failed")
	}
}

func TestMarketingConfigHistoryImporterQuarantinesRuleWithoutImportedParent(t *testing.T) {
	config := marketingConfigRow(t, 1)
	config.RedactedFields = []string{"automation_name"}
	rule := marketingRuleRow(t, 1, 1, nil, nil)
	archive := marketingHistoryArchive{rows: map[string][]v1archive.ArchivedRow{v1marketingconfighistory.ConfigTableID: {config}, v1marketingconfighistory.RulesTableID: {rule, marketingRuleRow(t, 2, 1, nil, nil), marketingRuleRow(t, 3, 1, nil, nil)}}}
	runtime := newMarketingHistoryRuntime()
	importer, err := v1domain.NewMarketingConfigHistoryImporter(archive, runtime, runtime, runtime)
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(context.Background(), "run")
	if err != nil || result.ImportedConfigs != 0 || result.QuarantinedConfigs != 1 || result.QuarantinedRules != 3 || runtime.writes != 0 {
		t.Fatal("marketing_config_parent_failure_not_closed")
	}
}

func marketingConfigRow(t *testing.T, id int64) v1archive.ArchivedRow {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"id": id, "automation_key": "key", "automation_name": "name", "target_event": "event", "channel_type": "channel", "status": "enabled", "do_not_start_after_hour": int32(-1), "config_payload_json": map[string]any{"secret": "config-private"}, "created_at": "2026-08-28T09:02:03.123456+08:00", "updated_at": "2026-08-27T09:02:03.123456+08:00"})
	if err != nil {
		t.Fatal(err)
	}
	return marketingHistoryRow(v1marketingconfighistory.ConfigTableID, id, payload)
}
func marketingRuleRow(t *testing.T, id, configID int64, questionnaire, question *int64) v1archive.ArchivedRow {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"id": id, "automation_config_id": configID, "questionnaire_id": questionnaire, "question_id": question, "rule_code": "rule", "rule_name": "rule-name", "answer_match_type": "equals", "answer_match_value_json": map[string]any{"secret": "answer-private"}, "score_delta": int32(-8), "segment_hint": "", "stage_hint": "", "is_active": false, "sort_order": int32(-9), "rule_payload_json": map[string]any{"secret": "payload-private"}, "created_at": "2026-08-28T09:02:03.123456+08:00", "updated_at": "2026-08-27T09:02:03.123456+08:00"})
	if err != nil {
		t.Fatal(err)
	}
	return marketingHistoryRow(v1marketingconfighistory.RulesTableID, id, payload)
}
func marketingHistoryRow(table string, ordinal int64, payload []byte) v1archive.ArchivedRow {
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal, SourceKeyHMAC: sha256.Sum256([]byte(fmt.Sprintf("%s/key/%d", table, ordinal))), PayloadHMAC: sha256.Sum256(payload), FieldHMAC: sha256.Sum256([]byte(fmt.Sprintf("%s/field/%d", table, ordinal))), Payload: payload}
}
func ptrMarketingInt64(value int64) *int64 { return &value }
