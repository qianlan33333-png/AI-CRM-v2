package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1marketingconfighistory"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	MarketingConfigHistoryImportVersion = "v1-marketing-config-history-a1"
	MarketingConfigHistoryDomain        = "marketing-config-history"
	MarketingConfigHistoryConfigKind    = "marketing_config"
	MarketingConfigHistoryRuleKind      = "marketing_rule"
	MarketingConfigHistoryConfigTarget  = "automation_v1_marketing_config_history"
	MarketingConfigHistoryRuleTarget    = "automation_v1_marketing_rule_history"
)

type MarketingConfigHistoryWriter interface {
	ImportHistoricalMarketingAutomationConfig(context.Context, string, automationport.HistoricalMarketingAutomationConfig) (automationport.MarketingConfigHistoryReceipt, error)
	ImportHistoricalMarketingAutomationRule(context.Context, string, automationport.HistoricalMarketingAutomationRule) (automationport.MarketingConfigHistoryReceipt, error)
}

type MarketingConfigHistoryImportJournal interface {
	automationport.MarketingConfigHistoryJournal
	LoadMarketingConfigHistoryTerminal(context.Context, string, string) (TerminalReceipt, bool, error)
	RecordMarketingConfigHistoryTerminal(context.Context, string, TerminalReceipt) error
}

type MarketingConfigHistoryImportResult struct{ ImportedConfigs, ImportedRules, QuarantinedConfigs, QuarantinedRules, Replayed int }

func (r MarketingConfigHistoryImportResult) terminalCount() int {
	return r.ImportedConfigs + r.ImportedRules + r.QuarantinedConfigs + r.QuarantinedRules
}

type MarketingConfigHistoryImporter struct {
	archive ArchiveSource
	uow     UnitOfWork
	writer  MarketingConfigHistoryWriter
	journal MarketingConfigHistoryImportJournal
	configs int
	rules   int
}

func NewMarketingConfigHistoryImporter(archive ArchiveSource, uow UnitOfWork, writer MarketingConfigHistoryWriter, journal MarketingConfigHistoryImportJournal) (*MarketingConfigHistoryImporter, error) {
	return newMarketingConfigHistoryImporter(archive, uow, writer, journal, 1, 3)
}

type marketingConfigHistoryJournal struct{ journals map[string]*Journal }

var _ MarketingConfigHistoryImportJournal = (*marketingConfigHistoryJournal)(nil)

func NewMarketingConfigHistoryJournal(config, rule *Journal) (*marketingConfigHistoryJournal, error) {
	if config == nil || rule == nil || !validMarketingConfigHistoryJournal(config, MarketingConfigHistoryConfigKind) || !validMarketingConfigHistoryJournal(rule, MarketingConfigHistoryRuleKind) || config.scope.ArchiveRunID != rule.scope.ArchiveRunID {
		return nil, ErrInvalidScope
	}
	return &marketingConfigHistoryJournal{journals: map[string]*Journal{MarketingConfigHistoryConfigKind: config, MarketingConfigHistoryRuleKind: rule}}, nil
}

func validMarketingConfigHistoryJournal(journal *Journal, kind string) bool {
	if journal == nil || journal.tx == nil || !journal.scope.valid() || journal.scope.ImportVersion != MarketingConfigHistoryImportVersion || journal.scope.AdapterID != v1archive.DefaultAdapterID || journal.scope.TargetDomain != "automation" {
		return false
	}
	switch kind {
	case MarketingConfigHistoryConfigKind:
		return journal.scope.TableID == v1marketingconfighistory.ConfigTableID && journal.scope.TargetTable == MarketingConfigHistoryConfigTarget
	case MarketingConfigHistoryRuleKind:
		return journal.scope.TableID == v1marketingconfighistory.RulesTableID && journal.scope.TargetTable == MarketingConfigHistoryRuleTarget
	default:
		return false
	}
}

func (j *marketingConfigHistoryJournal) selected(kind string) (*Journal, error) {
	if j == nil || j.journals == nil || j.journals[kind] == nil || (kind != MarketingConfigHistoryConfigKind && kind != MarketingConfigHistoryRuleKind) {
		return nil, ErrInvalidScope
	}
	return j.journals[kind], nil
}

func (j *marketingConfigHistoryJournal) LoadMarketingConfigHistory(ctx context.Context, kind, source string) (automationport.MarketingConfigHistoryReceipt, bool, error) {
	terminal, found, err := j.LoadMarketingConfigHistoryTerminal(ctx, kind, source)
	if err != nil || !found {
		return automationport.MarketingConfigHistoryReceipt{}, found, err
	}
	return marketingConfigHistoryReceipt(kind, source, terminal)
}

func (j *marketingConfigHistoryJournal) RecordMarketingConfigHistory(ctx context.Context, receipt automationport.MarketingConfigHistoryReceipt) error {
	terminal, err := marketingConfigHistoryTerminal(receipt)
	if err != nil {
		return err
	}
	return j.RecordMarketingConfigHistoryTerminal(ctx, receipt.Kind, terminal)
}

func (j *marketingConfigHistoryJournal) LoadMarketingConfigHistoryTerminal(ctx context.Context, kind, source string) (TerminalReceipt, bool, error) {
	selected, err := j.selected(kind)
	if err != nil {
		return TerminalReceipt{}, false, err
	}
	return selected.LoadTerminal(ctx, source)
}

func (j *marketingConfigHistoryJournal) RecordMarketingConfigHistoryTerminal(ctx context.Context, kind string, terminal TerminalReceipt) error {
	selected, err := j.selected(kind)
	if err != nil {
		return err
	}
	return selected.Record(ctx, terminal)
}

func newMarketingConfigHistoryImporter(archive ArchiveSource, uow UnitOfWork, writer MarketingConfigHistoryWriter, journal MarketingConfigHistoryImportJournal, configs, rules int) (*MarketingConfigHistoryImporter, error) {
	if archive == nil || uow == nil || writer == nil || journal == nil || configs < 0 || rules < 0 {
		return nil, ErrInvalidScope
	}
	return &MarketingConfigHistoryImporter{archive: archive, uow: uow, writer: writer, journal: journal, configs: configs, rules: rules}, nil
}

func (i *MarketingConfigHistoryImporter) Import(ctx context.Context, run string) (MarketingConfigHistoryImportResult, error) {
	if i == nil || ctx == nil || run == "" || i.archive == nil || i.uow == nil || i.writer == nil || i.journal == nil {
		return MarketingConfigHistoryImportResult{}, ErrInvalidScope
	}
	configs, err := i.readRows(ctx, run, v1marketingconfighistory.ConfigTableID, i.configs)
	if err != nil {
		return MarketingConfigHistoryImportResult{}, err
	}
	rules, err := i.readRows(ctx, run, v1marketingconfighistory.RulesTableID, i.rules)
	if err != nil {
		return MarketingConfigHistoryImportResult{}, err
	}
	history := v1marketingconfighistory.AdaptHistory(marketingConfigPayloads(configs), marketingConfigPayloads(rules))
	result := MarketingConfigHistoryImportResult{}
	parents := make(map[int64]int64, len(configs))
	for index := range configs {
		if err = i.importConfig(ctx, configs[index], history.Configs[index], parents, &result); err != nil {
			return MarketingConfigHistoryImportResult{}, err
		}
	}
	for index := range rules {
		if err = i.importRule(ctx, rules[index], history.Rules[index], parents, &result); err != nil {
			return MarketingConfigHistoryImportResult{}, err
		}
	}
	if result.terminalCount() != i.configs+i.rules {
		return MarketingConfigHistoryImportResult{}, ErrConflict
	}
	return result, nil
}

func (i *MarketingConfigHistoryImporter) readRows(ctx context.Context, run, table string, expected int) ([]v1archive.ArchivedRow, error) {
	rows := make([]v1archive.ArchivedRow, 0, expected)
	seen := map[[sha256.Size]byte]struct{}{}
	err := i.archive.EachTableRow(ctx, run, table, func(row v1archive.ArchivedRow) error {
		if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != int64(len(rows)+1) || row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) {
			return ErrConflict
		}
		if _, duplicate := seen[row.SourceKeyHMAC]; duplicate {
			return ErrConflict
		}
		seen[row.SourceKeyHMAC] = struct{}{}
		rows = append(rows, row)
		return nil
	})
	if err != nil || len(rows) != expected {
		if err != nil {
			return nil, err
		}
		return nil, ErrConflict
	}
	return rows, nil
}

func marketingConfigPayloads(rows []v1archive.ArchivedRow) []json.RawMessage {
	values := make([]json.RawMessage, len(rows))
	for index := range rows {
		values[index] = append(json.RawMessage(nil), rows[index].Payload...)
	}
	return values
}

func (i *MarketingConfigHistoryImporter) importConfig(ctx context.Context, row v1archive.ArchivedRow, decision v1marketingconfighistory.Result[v1marketingconfighistory.ConfigFact], parents map[int64]int64, result *MarketingConfigHistoryImportResult) error {
	imported, replayed := false, false
	err := i.uow.Within(ctx, func(tx context.Context) error {
		imported, replayed = false, false
		if len(row.RedactedFields) != 0 || decision.Disposition != v1marketingconfighistory.DispositionCandidate || decision.Fact == nil {
			reason := decision.Reason
			if len(row.RedactedFields) != 0 {
				reason = "marketing_config_source_redacted"
			}
			if reason == "" {
				reason = "invalid_marketing_config"
			}
			var recordErr error
			replayed, recordErr = recordMarketingConfigHistoryQuarantine(tx, i.journal, MarketingConfigHistoryConfigKind, row, reason)
			return recordErr
		}
		value := marketingConfigHistoryConfigValue(row, *decision.Fact)
		receipt, writeErr := i.writer.ImportHistoricalMarketingAutomationConfig(tx, SourceIdentifier(row.SourceKeyHMAC), value)
		if errors.Is(writeErr, automationport.ErrMarketingConfigHistoryInvalid) {
			var recordErr error
			replayed, recordErr = recordMarketingConfigHistoryQuarantine(tx, i.journal, MarketingConfigHistoryConfigKind, row, "marketing_config_target_invalid")
			return recordErr
		}
		if writeErr != nil {
			return writeErr
		}
		if verifyErr := verifyMarketingConfigHistoryReceipt(tx, i.journal, MarketingConfigHistoryConfigKind, row, receipt); verifyErr != nil {
			return verifyErr
		}
		parents[decision.Fact.SourceID] = receipt.TargetID
		imported, replayed = true, receipt.Replayed
		return nil
	})
	if err != nil {
		return err
	}
	if imported {
		result.ImportedConfigs++
	} else {
		result.QuarantinedConfigs++
	}
	if replayed {
		result.Replayed++
	}
	return nil
}

func (i *MarketingConfigHistoryImporter) importRule(ctx context.Context, row v1archive.ArchivedRow, decision v1marketingconfighistory.Result[v1marketingconfighistory.RuleFact], parents map[int64]int64, result *MarketingConfigHistoryImportResult) error {
	imported, replayed := false, false
	err := i.uow.Within(ctx, func(tx context.Context) error {
		imported, replayed = false, false
		if len(row.RedactedFields) != 0 || decision.Disposition != v1marketingconfighistory.DispositionCandidate || decision.Fact == nil {
			reason := decision.Reason
			if len(row.RedactedFields) != 0 {
				reason = "marketing_rule_source_redacted"
			}
			if reason == "" {
				reason = "invalid_marketing_rule"
			}
			var recordErr error
			replayed, recordErr = recordMarketingConfigHistoryQuarantine(tx, i.journal, MarketingConfigHistoryRuleKind, row, reason)
			return recordErr
		}
		configID, found := parents[decision.Fact.ConfigSourceID]
		if !found || configID < 1 {
			var recordErr error
			replayed, recordErr = recordMarketingConfigHistoryQuarantine(tx, i.journal, MarketingConfigHistoryRuleKind, row, "marketing_rule_config_not_imported")
			return recordErr
		}
		value := marketingConfigHistoryRuleValue(row, *decision.Fact, configID)
		receipt, writeErr := i.writer.ImportHistoricalMarketingAutomationRule(tx, SourceIdentifier(row.SourceKeyHMAC), value)
		if errors.Is(writeErr, automationport.ErrMarketingConfigHistoryInvalid) {
			var recordErr error
			replayed, recordErr = recordMarketingConfigHistoryQuarantine(tx, i.journal, MarketingConfigHistoryRuleKind, row, "marketing_rule_target_invalid")
			return recordErr
		}
		if writeErr != nil {
			return writeErr
		}
		if verifyErr := verifyMarketingConfigHistoryReceipt(tx, i.journal, MarketingConfigHistoryRuleKind, row, receipt); verifyErr != nil {
			return verifyErr
		}
		imported, replayed = true, receipt.Replayed
		return nil
	})
	if err != nil {
		return err
	}
	if imported {
		result.ImportedRules++
	} else {
		result.QuarantinedRules++
	}
	if replayed {
		result.Replayed++
	}
	return nil
}

func marketingConfigHistoryConfigValue(row v1archive.ArchivedRow, fact v1marketingconfighistory.ConfigFact) automationport.HistoricalMarketingAutomationConfig {
	return automationport.HistoricalMarketingAutomationConfig{SourceKeyDigest: row.SourceKeyHMAC, SourcePayloadDigest: row.PayloadHMAC, SourceFieldDigest: row.FieldHMAC, SourceID: fact.SourceID, AutomationKey: fact.AutomationKey, AutomationName: fact.AutomationName, TargetEvent: fact.TargetEvent, ChannelType: fact.ChannelType, OriginalStatus: fact.OriginalStatus, DoNotStartAfterHour: fact.DoNotStartAfterHour, CreatedAt: fact.CreatedAt.UTC().Truncate(time.Microsecond), UpdatedAt: fact.UpdatedAt.UTC().Truncate(time.Microsecond), ConfigPayloadDigest: [sha256.Size]byte(fact.ConfigPayloadDigest)}
}

func marketingConfigHistoryRuleValue(row v1archive.ArchivedRow, fact v1marketingconfighistory.RuleFact, configID int64) automationport.HistoricalMarketingAutomationRule {
	return automationport.HistoricalMarketingAutomationRule{SourceKeyDigest: row.SourceKeyHMAC, SourcePayloadDigest: row.PayloadHMAC, SourceFieldDigest: row.FieldHMAC, SourceID: fact.SourceID, ConfigID: configID, ConfigSourceID: fact.ConfigSourceID, QuestionnaireSourceID: copyMarketingConfigID(fact.QuestionnaireSourceID), QuestionSourceID: copyMarketingConfigID(fact.QuestionSourceID), RuleCode: fact.RuleCode, RuleName: fact.RuleName, AnswerMatchType: fact.AnswerMatchType, ScoreDelta: fact.ScoreDelta, SegmentHint: fact.SegmentHint, StageHint: fact.StageHint, OriginalActive: fact.OriginalActive, SortOrder: fact.SortOrder, CreatedAt: fact.CreatedAt.UTC().Truncate(time.Microsecond), UpdatedAt: fact.UpdatedAt.UTC().Truncate(time.Microsecond), AnswerMatchValueDigest: [sha256.Size]byte(fact.AnswerMatchValueDigest), RulePayloadDigest: [sha256.Size]byte(fact.RulePayloadDigest)}
}

func copyMarketingConfigID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func recordMarketingConfigHistoryQuarantine(ctx context.Context, journal MarketingConfigHistoryImportJournal, kind string, row v1archive.ArchivedRow, reason string) (bool, error) {
	if reason == "" {
		return false, ErrInvalidScope
	}
	source := SourceIdentifier(row.SourceKeyHMAC)
	existing, found, err := journal.LoadMarketingConfigHistoryTerminal(ctx, kind, source)
	if err != nil {
		return false, err
	}
	if found {
		if existing.SourceKeyDigest != row.SourceKeyHMAC || existing.PayloadDigest != row.PayloadHMAC || existing.Disposition != "quarantine" || existing.Reason != reason || existing.TargetID != "" || existing.TargetDigest != ([sha256.Size]byte{}) || len(existing.Metadata) != 0 {
			return false, ErrConflict
		}
		return true, nil
	}
	return false, journal.RecordMarketingConfigHistoryTerminal(ctx, kind, TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: "quarantine", Reason: reason})
}

func verifyMarketingConfigHistoryReceipt(ctx context.Context, journal MarketingConfigHistoryImportJournal, kind string, row v1archive.ArchivedRow, receipt automationport.MarketingConfigHistoryReceipt) error {
	if receipt.Kind != kind || receipt.SourceIdentifier != SourceIdentifier(row.SourceKeyHMAC) || receipt.PayloadDigest != row.PayloadHMAC || receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	stored, found, err := journal.LoadMarketingConfigHistory(ctx, kind, receipt.SourceIdentifier)
	if err != nil || !found || stored.Kind != receipt.Kind || stored.SourceIdentifier != receipt.SourceIdentifier || stored.PayloadDigest != receipt.PayloadDigest || stored.TargetID != receipt.TargetID || stored.TargetDigest != receipt.TargetDigest {
		return ErrConflict
	}
	return nil
}

func marketingConfigHistoryReceipt(kind, source string, terminal TerminalReceipt) (automationport.MarketingConfigHistoryReceipt, bool, error) {
	key, err := ParseSourceIdentifier(source)
	id, idErr := strconv.ParseInt(terminal.TargetID, 10, 64)
	if err != nil || idErr != nil || (kind != MarketingConfigHistoryConfigKind && kind != MarketingConfigHistoryRuleKind) || key == ([sha256.Size]byte{}) || terminal.SourceKeyDigest != key || terminal.PayloadDigest == ([sha256.Size]byte{}) || terminal.TargetDigest == ([sha256.Size]byte{}) || terminal.Disposition != "import" || terminal.Reason != "" || len(terminal.Metadata) != 0 || id < 1 || strconv.FormatInt(id, 10) != terminal.TargetID {
		return automationport.MarketingConfigHistoryReceipt{}, false, ErrConflict
	}
	return automationport.MarketingConfigHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetID: id, TargetDigest: terminal.TargetDigest}, true, nil
}

func marketingConfigHistoryTerminal(receipt automationport.MarketingConfigHistoryReceipt) (TerminalReceipt, error) {
	key, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || (receipt.Kind != MarketingConfigHistoryConfigKind && receipt.Kind != MarketingConfigHistoryRuleKind) || key == ([sha256.Size]byte{}) || receipt.SourceIdentifier != SourceIdentifier(key) || receipt.PayloadDigest == ([sha256.Size]byte{}) || receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.TargetID < 1 || receipt.Replayed {
		return TerminalReceipt{}, ErrInvalidScope
	}
	return TerminalReceipt{SourceKeyDigest: key, PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest}, nil
}
