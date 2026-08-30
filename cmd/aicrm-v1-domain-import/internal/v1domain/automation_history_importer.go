package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1automationhistory"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

// AutomationHistoryWriter is the Automation-owned boundary for immutable V1
// history. Each method must use the caller transaction and must not create a
// current automation, publish work, or trigger an effect.
type AutomationHistoryWriter interface {
	ImportSOP(context.Context, string, automationport.HistoricalAutomationSOP) (automationport.AutomationHistoryReceipt, error)
	ImportConfig(context.Context, string, automationport.HistoricalAutomationConfig) (automationport.AutomationHistoryReceipt, error)
	ImportPrompt(context.Context, string, automationport.HistoricalAutomationPrompt) (automationport.AutomationHistoryReceipt, error)
	ImportAgent(context.Context, string, automationport.HistoricalAutomationAgent) (automationport.AutomationHistoryReceipt, error)
}

type AutomationHistoryImportResult struct {
	ImportedSOPs, ImportedConfigs, ImportedPrompts, ImportedAgents int
	Quarantined, Replayed                                          int
}

type AutomationHistoryImporter struct {
	archive ArchiveSource
	uow     UnitOfWork
	writer  AutomationHistoryWriter
	journal AutomationHistoryImportJournal
}

func NewAutomationHistoryImporter(archive ArchiveSource, uow UnitOfWork, writer AutomationHistoryWriter, journal AutomationHistoryImportJournal) (*AutomationHistoryImporter, error) {
	if archive == nil || uow == nil || writer == nil || journal == nil {
		return nil, ErrInvalidScope
	}
	return &AutomationHistoryImporter{archive: archive, uow: uow, writer: writer, journal: journal}, nil
}

func (importer *AutomationHistoryImporter) Import(ctx context.Context, archiveRunID string) (AutomationHistoryImportResult, error) {
	if importer == nil || ctx == nil || importer.archive == nil || importer.uow == nil || importer.writer == nil || importer.journal == nil ||
		importer.journal.ValidateAutomationHistoryImportScope(archiveRunID) != nil {
		return AutomationHistoryImportResult{}, ErrInvalidScope
	}
	sops, err := importer.readAutomationHistoryRows(ctx, archiveRunID, automationHistorySOPTable)
	if err != nil {
		return AutomationHistoryImportResult{}, err
	}
	configs, err := importer.readAutomationHistoryRows(ctx, archiveRunID, automationHistoryConfigTable)
	if err != nil {
		return AutomationHistoryImportResult{}, err
	}
	prompts, err := importer.readAutomationHistoryRows(ctx, archiveRunID, automationHistoryPromptTable)
	if err != nil {
		return AutomationHistoryImportResult{}, err
	}
	agents, err := importer.readAutomationHistoryRows(ctx, archiveRunID, automationHistoryAgentTable)
	if err != nil {
		return AutomationHistoryImportResult{}, err
	}

	history := v1automationhistory.AdaptHistory(automationHistoryPayloads(sops), automationHistoryPayloads(configs), automationHistoryPayloads(prompts), automationHistoryPayloads(agents))
	if len(history.SOPTemplates) != len(sops) || len(history.AgentConfigs) != len(configs) || len(history.PromptRegistries) != len(prompts) || len(history.Agents) != len(agents) {
		return AutomationHistoryImportResult{}, ErrConflict
	}
	result := AutomationHistoryImportResult{}
	for index, decision := range history.SOPTemplates {
		imported, replayed, err := importer.importSOP(ctx, sops[index], decision)
		if err != nil {
			return AutomationHistoryImportResult{}, err
		}
		if imported {
			result.ImportedSOPs++
		} else {
			result.Quarantined++
		}
		result.Replayed += boolCount(replayed)
	}
	for index, decision := range history.AgentConfigs {
		imported, replayed, err := importer.importConfig(ctx, configs[index], decision)
		if err != nil {
			return AutomationHistoryImportResult{}, err
		}
		if imported {
			result.ImportedConfigs++
		} else {
			result.Quarantined++
		}
		result.Replayed += boolCount(replayed)
	}
	for index, decision := range history.PromptRegistries {
		imported, replayed, err := importer.importPrompt(ctx, prompts[index], decision)
		if err != nil {
			return AutomationHistoryImportResult{}, err
		}
		if imported {
			result.ImportedPrompts++
		} else {
			result.Quarantined++
		}
		result.Replayed += boolCount(replayed)
	}
	for index, decision := range history.Agents {
		imported, replayed, err := importer.importAgent(ctx, agents[index], decision)
		if err != nil {
			return AutomationHistoryImportResult{}, err
		}
		if imported {
			result.ImportedAgents++
		} else {
			result.Quarantined++
		}
		result.Replayed += boolCount(replayed)
	}
	return result, nil
}

// EditableAgents decrypts the already sealed source rows only for the final
// one-time projection. It does not write, activate, publish, or execute them.
func (importer *AutomationHistoryImporter) EditableAgents(ctx context.Context, archiveRunID string) ([]v1automationhistory.EditableAgent, error) {
	if importer == nil || ctx == nil || importer.archive == nil || importer.journal == nil || importer.journal.ValidateAutomationHistoryImportScope(archiveRunID) != nil {
		return nil, ErrInvalidScope
	}
	configs, err := importer.readAutomationHistoryRows(ctx, archiveRunID, automationHistoryConfigTable)
	if err != nil {
		return nil, err
	}
	prompts, err := importer.readAutomationHistoryRows(ctx, archiveRunID, automationHistoryPromptTable)
	if err != nil {
		return nil, err
	}
	agents, err := importer.readAutomationHistoryRows(ctx, archiveRunID, automationHistoryAgentTable)
	if err != nil {
		return nil, err
	}
	return v1automationhistory.AdaptEditableAgents(automationHistoryPayloads(configs), automationHistoryPayloads(prompts), automationHistoryPayloads(agents))
}

func (importer *AutomationHistoryImporter) importSOP(ctx context.Context, row v1archive.ArchivedRow, decision v1automationhistory.Result[v1automationhistory.SOPTemplateFact]) (bool, bool, error) {
	if reason := automationHistoryDecisionReason(row, decision.Disposition, decision.Fact != nil, "invalid_automation_sop_history"); reason != "" {
		return importer.quarantineAutomationHistory(ctx, automationport.AutomationHistorySOP, row, reason)
	}
	value := automationHistorySOPValue(row, *decision.Fact)
	return importer.writeAutomationHistory(ctx, automationport.AutomationHistorySOP, row, func(tx context.Context) (automationport.AutomationHistoryReceipt, error) {
		return importer.writer.ImportSOP(tx, SourceIdentifier(row.SourceKeyHMAC), value)
	})
}

func (importer *AutomationHistoryImporter) importConfig(ctx context.Context, row v1archive.ArchivedRow, decision v1automationhistory.Result[v1automationhistory.AgentConfigFact]) (bool, bool, error) {
	if reason := automationHistoryDecisionReason(row, decision.Disposition, decision.Fact != nil, "invalid_automation_config_history"); reason != "" {
		return importer.quarantineAutomationHistory(ctx, automationport.AutomationHistoryConfig, row, reason)
	}
	value := automationHistoryConfigValue(row, *decision.Fact)
	return importer.writeAutomationHistory(ctx, automationport.AutomationHistoryConfig, row, func(tx context.Context) (automationport.AutomationHistoryReceipt, error) {
		return importer.writer.ImportConfig(tx, SourceIdentifier(row.SourceKeyHMAC), value)
	})
}

func (importer *AutomationHistoryImporter) importPrompt(ctx context.Context, row v1archive.ArchivedRow, decision v1automationhistory.Result[v1automationhistory.PromptRegistryFact]) (bool, bool, error) {
	if reason := automationHistoryDecisionReason(row, decision.Disposition, decision.Fact != nil, "invalid_automation_prompt_history"); reason != "" {
		return importer.quarantineAutomationHistory(ctx, automationport.AutomationHistoryPrompt, row, reason)
	}
	value := automationHistoryPromptValue(row, *decision.Fact)
	return importer.writeAutomationHistory(ctx, automationport.AutomationHistoryPrompt, row, func(tx context.Context) (automationport.AutomationHistoryReceipt, error) {
		return importer.writer.ImportPrompt(tx, SourceIdentifier(row.SourceKeyHMAC), value)
	})
}

func (importer *AutomationHistoryImporter) importAgent(ctx context.Context, row v1archive.ArchivedRow, decision v1automationhistory.Result[v1automationhistory.AgentFact]) (bool, bool, error) {
	if reason := automationHistoryDecisionReason(row, decision.Disposition, decision.Fact != nil, "invalid_automation_agent_history"); reason != "" {
		return importer.quarantineAutomationHistory(ctx, automationport.AutomationHistoryAgent, row, reason)
	}
	value := automationHistoryAgentValue(row, *decision.Fact)
	return importer.writeAutomationHistory(ctx, automationport.AutomationHistoryAgent, row, func(tx context.Context) (automationport.AutomationHistoryReceipt, error) {
		return importer.writer.ImportAgent(tx, SourceIdentifier(row.SourceKeyHMAC), value)
	})
}

func (importer *AutomationHistoryImporter) writeAutomationHistory(ctx context.Context, kind string, row v1archive.ArchivedRow, write func(context.Context) (automationport.AutomationHistoryReceipt, error)) (imported, replayed bool, err error) {
	err = importer.uow.Within(ctx, func(tx context.Context) error {
		// UnitOfWork may retry this callback. Only the committed attempt is
		// allowed to influence counters returned to the caller.
		imported, replayed = false, false
		receipt, writeErr := write(tx)
		if errors.Is(writeErr, automationport.ErrAutomationHistoryInvalid) {
			var terminalErr error
			replayed, terminalErr = recordAutomationHistoryQuarantine(tx, importer.journal, kind, row, "automation_history_target_invalid")
			return terminalErr
		}
		if writeErr != nil {
			return writeErr
		}
		if verifyErr := verifyAutomationHistoryReceipt(tx, importer.journal, kind, row, receipt); verifyErr != nil {
			return verifyErr
		}
		imported, replayed = true, receipt.Replayed
		return nil
	})
	return imported, replayed, err
}

func (importer *AutomationHistoryImporter) quarantineAutomationHistory(ctx context.Context, kind string, row v1archive.ArchivedRow, reason string) (bool, bool, error) {
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		// Do not retain a prior retry outcome.
		replayed = false
		var terminalErr error
		replayed, terminalErr = recordAutomationHistoryQuarantine(tx, importer.journal, kind, row, reason)
		return terminalErr
	})
	return false, replayed, err
}

func (importer *AutomationHistoryImporter) readAutomationHistoryRows(ctx context.Context, archiveRunID, tableID string) ([]v1archive.ArchivedRow, error) {
	rows := make([]v1archive.ArchivedRow, 0)
	seen := make(map[[sha256.Size]byte]struct{})
	expectedOrdinal := int64(1)
	err := importer.archive.EachTableRow(ctx, archiveRunID, tableID, func(row v1archive.ArchivedRow) error {
		if !validAutomationHistoryArchiveRow(row, tableID, expectedOrdinal) {
			return ErrConflict
		}
		expectedOrdinal++
		if _, duplicate := seen[row.SourceKeyHMAC]; duplicate {
			return ErrConflict
		}
		seen[row.SourceKeyHMAC] = struct{}{}
		rows = append(rows, row)
		return nil
	})
	return rows, err
}

func validAutomationHistoryArchiveRow(row v1archive.ArchivedRow, tableID string, ordinal int64) bool {
	return row.AdapterID == v1archive.DefaultAdapterID && row.TableID == tableID && row.SourceOrdinal == ordinal &&
		row.SourceKeyHMAC != ([sha256.Size]byte{}) && row.PayloadHMAC != ([sha256.Size]byte{}) && row.FieldHMAC != ([sha256.Size]byte{}) && json.Valid(row.Payload)
}

func automationHistoryPayloads(rows []v1archive.ArchivedRow) []json.RawMessage {
	payloads := make([]json.RawMessage, len(rows))
	for index := range rows {
		payloads[index] = rows[index].Payload
	}
	return payloads
}

func automationHistoryDecisionReason(row v1archive.ArchivedRow, disposition v1automationhistory.Disposition, hasFact bool, fallback string) string {
	if len(row.RedactedFields) != 0 {
		return "automation_history_business_field_redacted"
	}
	if disposition != v1automationhistory.DispositionCandidate || !hasFact {
		return fallback
	}
	return ""
}

func recordAutomationHistoryQuarantine(ctx context.Context, journal AutomationHistoryImportJournal, kind string, row v1archive.ArchivedRow, reason string) (bool, error) {
	if journal == nil || !validAutomationHistoryKind(kind) || reason == "" {
		return false, ErrInvalidScope
	}
	source := SourceIdentifier(row.SourceKeyHMAC)
	existing, found, err := journal.LoadAutomationHistoryTerminal(ctx, kind, source)
	if err != nil {
		return false, err
	}
	if found {
		if existing.SourceKeyDigest != row.SourceKeyHMAC || existing.PayloadDigest != row.PayloadHMAC || existing.Disposition != "quarantine" || existing.Reason != reason ||
			existing.TargetID != "" || existing.TargetDigest != ([sha256.Size]byte{}) || len(existing.Metadata) != 0 {
			return false, ErrConflict
		}
		return true, nil
	}
	return false, journal.RecordAutomationHistoryTerminal(ctx, kind, TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: "quarantine", Reason: reason})
}

func verifyAutomationHistoryReceipt(ctx context.Context, journal AutomationHistoryImportJournal, kind string, row v1archive.ArchivedRow, receipt automationport.AutomationHistoryReceipt) error {
	if journal == nil || receipt.Kind != kind || receipt.SourceIdentifier != SourceIdentifier(row.SourceKeyHMAC) || receipt.PayloadDigest != row.PayloadHMAC ||
		receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	terminal, found, err := journal.LoadAutomationHistoryTerminal(ctx, kind, receipt.SourceIdentifier)
	if err != nil {
		return err
	}
	if !found || terminal.SourceKeyDigest != row.SourceKeyHMAC || terminal.PayloadDigest != row.PayloadHMAC || terminal.Disposition != "import" || terminal.Reason != "" ||
		terminal.TargetID != strconv.FormatInt(receipt.TargetID, 10) || terminal.TargetDigest != receipt.TargetDigest || len(terminal.Metadata) != 0 {
		return ErrConflict
	}
	return nil
}

func automationHistoryIdentity(row v1archive.ArchivedRow, sourceID int64) automationport.HistoricalAutomationIdentity {
	return automationport.HistoricalAutomationIdentity{SourceID: sourceID, SourceKeyDigest: row.SourceKeyHMAC, SourcePayloadDigest: row.PayloadHMAC}
}

func automationHistorySOPValue(row v1archive.ArchivedRow, fact v1automationhistory.SOPTemplateFact) automationport.HistoricalAutomationSOP {
	return automationport.HistoricalAutomationSOP{HistoricalAutomationIdentity: automationHistoryIdentity(row, fact.SourceID), PoolKey: fact.PoolKey, DayIndex: fact.DayIndex,
		ContentMasked: fact.ContentMasked, ImagesDigest: [sha256.Size]byte(fact.ImagesDigest), OriginalEnabled: fact.OriginalEnabled,
		CreatedAt: automationHistoryTime(fact.CreatedAt), UpdatedAt: automationHistoryTime(fact.UpdatedAt)}
}

func automationHistoryConfigValue(row v1archive.ArchivedRow, fact v1automationhistory.AgentConfigFact) automationport.HistoricalAutomationConfig {
	return automationport.HistoricalAutomationConfig{HistoricalAutomationIdentity: automationHistoryIdentity(row, fact.SourceID), AgentCode: fact.AgentCode, DisplayName: fact.DisplayName,
		ScenarioCode: fact.ScenarioCode, OriginalEnabled: fact.OriginalEnabled, DraftVersion: fact.DraftVersion, PublishedVersion: fact.PublishedVersion,
		PublishedAt: fact.PublishedAt, LastModifiedAt: fact.LastModifiedAt, LastModifiedSource: fact.LastModifiedSource, SubmittedForPublish: fact.SubmittedForPublish,
		SubmittedAt: fact.SubmittedAt, CreatedAt: automationHistoryTime(fact.CreatedAt), UpdatedAt: automationHistoryTime(fact.UpdatedAt),
		ActorsDigest: automationHistoryActorsDigest("config", [2]string{"published_by", fact.PublishedBy}, [2]string{"last_modified_by", fact.LastModifiedBy}, [2]string{"submitted_by", fact.SubmittedBy}),
		ConfigDigest: [sha256.Size]byte(fact.ConfigDigest)}
}

func automationHistoryPromptValue(row v1archive.ArchivedRow, fact v1automationhistory.PromptRegistryFact) automationport.HistoricalAutomationPrompt {
	return automationport.HistoricalAutomationPrompt{HistoricalAutomationIdentity: automationHistoryIdentity(row, fact.SourceID), AgentCode: fact.AgentCode, DisplayName: fact.DisplayName,
		OriginalEnabled: fact.OriginalEnabled, Version: fact.Version, CreatedAt: automationHistoryTime(fact.CreatedAt), UpdatedAt: automationHistoryTime(fact.UpdatedAt), PromptDigest: [sha256.Size]byte(fact.PromptDigest)}
}

func automationHistoryAgentValue(row v1archive.ArchivedRow, fact v1automationhistory.AgentFact) automationport.HistoricalAutomationAgent {
	return automationport.HistoricalAutomationAgent{HistoricalAutomationIdentity: automationHistoryIdentity(row, fact.SourceID), ProgramSourceID: fact.ProgramSourceID,
		WorkflowSourceID: fact.WorkflowSourceID, NodeSourceID: fact.NodeSourceID, TaskSourceID: fact.TaskSourceID, AgentCode: fact.AgentCode, AgentName: fact.AgentName,
		OriginalType: fact.OriginalType, OriginalStatus: fact.OriginalStatus, SortOrder: fact.SortOrder, OriginalEnabled: fact.OriginalEnabled,
		CreatedAt: automationHistoryTime(fact.CreatedAt), UpdatedAt: automationHistoryTime(fact.UpdatedAt), ArchivedAt: fact.ArchivedAt,
		ActorsDigest:        automationHistoryActorsDigest("agent", [2]string{"created_by", fact.CreatedBySource}, [2]string{"updated_by", fact.UpdatedBySource}),
		ConfigurationDigest: [sha256.Size]byte(fact.ConfigurationDigest)}
}

func automationHistoryTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}

// automationHistoryActorsDigest has a distinct domain and ordered labels so
// raw V1 actor strings can never be recovered, confused across fact types, or
// concatenated ambiguously.
func automationHistoryActorsDigest(kind string, values ...[2]string) [sha256.Size]byte {
	hash := sha256.New()
	_, _ = hash.Write([]byte("v1-automation-history-actors-v1\x00"))
	automationHistoryDigestPart(hash, kind)
	for _, value := range values {
		automationHistoryDigestPart(hash, value[0])
		automationHistoryDigestPart(hash, value[1])
	}
	var result [sha256.Size]byte
	copy(result[:], hash.Sum(nil))
	return result
}

func automationHistoryDigestPart(hash interface{ Write([]byte) (int, error) }, value string) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(value)))
	_, _ = hash.Write(length[:])
	_, _ = hash.Write([]byte(value))
}
