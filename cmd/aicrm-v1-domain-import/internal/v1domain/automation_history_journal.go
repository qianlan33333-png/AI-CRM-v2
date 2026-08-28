package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"

	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	automationHistoryImportVersion = "v1-automation-history-a1"
	automationHistoryDomain        = "automation"

	automationHistorySOPTable    = "public/automation_sop_template"
	automationHistoryConfigTable = "public/automation_agent_config"
	automationHistoryPromptTable = "public/automation_agent_prompt_registry"
	automationHistoryAgentTable  = "public/automation_agents"

	automationHistorySOPTarget    = "automation_v1_sop_history"
	automationHistoryConfigTarget = "automation_v1_agent_config_history"
	automationHistoryPromptTarget = "automation_v1_prompt_history"
	automationHistoryAgentTarget  = "automation_v1_agent_history"
)

var automationHistoryTables = [...]string{
	automationHistorySOPTable,
	automationHistoryConfigTable,
	automationHistoryPromptTable,
	automationHistoryAgentTable,
}

// AutomationHistoryImportJournal keeps the owner-facing receipt Port tied to
// the migration-owned terminal receipt in the same caller transaction.
type AutomationHistoryImportJournal interface {
	automationport.AutomationHistoryJournal
	LoadAutomationHistoryTerminal(context.Context, string, string) (TerminalReceipt, bool, error)
	RecordAutomationHistoryTerminal(context.Context, string, TerminalReceipt) error
	ValidateAutomationHistoryImportScope(string) error
}

// AutomationHistoryJournal routes each owner kind to its exact V1 source
// table. It has no runtime automation capability.
type AutomationHistoryJournal struct{ journals map[string]*Journal }

var _ AutomationHistoryImportJournal = (*AutomationHistoryJournal)(nil)

func NewAutomationHistoryJournal(sop, config, prompt, agent *Journal) (*AutomationHistoryJournal, error) {
	journals := map[string]*Journal{
		automationport.AutomationHistorySOP:    sop,
		automationport.AutomationHistoryConfig: config,
		automationport.AutomationHistoryPrompt: prompt,
		automationport.AutomationHistoryAgent:  agent,
	}
	if !validAutomationHistoryJournals(journals) {
		return nil, ErrInvalidScope
	}
	return &AutomationHistoryJournal{journals: journals}, nil
}

func (journal *AutomationHistoryJournal) ValidateAutomationHistoryImportScope(archiveRunID string) error {
	if journal == nil || archiveRunID == "" || !validAutomationHistoryJournals(journal.journals) {
		return ErrInvalidScope
	}
	for _, kind := range automationHistoryKinds() {
		if journal.journals[kind].scope.ArchiveRunID != archiveRunID {
			return ErrInvalidScope
		}
	}
	return nil
}

func (journal *AutomationHistoryJournal) LoadAutomationHistory(ctx context.Context, kind, sourceIdentifier string) (automationport.AutomationHistoryReceipt, bool, error) {
	terminal, found, err := journal.LoadAutomationHistoryTerminal(ctx, kind, sourceIdentifier)
	if err != nil || !found {
		return automationport.AutomationHistoryReceipt{}, found, err
	}
	receipt, err := automationHistoryReceiptFromTerminal(kind, sourceIdentifier, terminal)
	if err != nil {
		return automationport.AutomationHistoryReceipt{}, false, err
	}
	return receipt, true, nil
}

func (journal *AutomationHistoryJournal) RecordAutomationHistory(ctx context.Context, receipt automationport.AutomationHistoryReceipt) error {
	terminal, err := automationHistoryTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return journal.RecordAutomationHistoryTerminal(ctx, receipt.Kind, terminal)
}

func (journal *AutomationHistoryJournal) LoadAutomationHistoryTerminal(ctx context.Context, kind, sourceIdentifier string) (TerminalReceipt, bool, error) {
	selected, err := journal.selectAutomationHistoryJournal(kind)
	if err != nil {
		return TerminalReceipt{}, false, err
	}
	key, err := ParseSourceIdentifier(sourceIdentifier)
	if err != nil || key == ([sha256.Size]byte{}) || sourceIdentifier != SourceIdentifier(key) {
		return TerminalReceipt{}, false, ErrInvalidScope
	}
	return selected.LoadTerminal(ctx, sourceIdentifier)
}

func (journal *AutomationHistoryJournal) RecordAutomationHistoryTerminal(ctx context.Context, kind string, receipt TerminalReceipt) error {
	selected, err := journal.selectAutomationHistoryJournal(kind)
	if err != nil {
		return err
	}
	return selected.Record(ctx, receipt)
}

func (journal *AutomationHistoryJournal) selectAutomationHistoryJournal(kind string) (*Journal, error) {
	if journal == nil || !validAutomationHistoryKind(kind) || !validAutomationHistoryJournals(journal.journals) {
		return nil, ErrInvalidScope
	}
	return journal.journals[kind], nil
}

func automationHistoryReceiptFromTerminal(kind, sourceIdentifier string, terminal TerminalReceipt) (automationport.AutomationHistoryReceipt, error) {
	key, err := ParseSourceIdentifier(sourceIdentifier)
	id, idErr := positiveID(terminal.TargetID)
	if err != nil || idErr != nil || !validAutomationHistoryKind(kind) || key == ([sha256.Size]byte{}) ||
		terminal.SourceKeyDigest != key || terminal.PayloadDigest == ([sha256.Size]byte{}) || terminal.TargetDigest == ([sha256.Size]byte{}) ||
		terminal.Disposition != "import" || terminal.Reason != "" || len(terminal.Metadata) != 0 || strconv.FormatInt(id, 10) != terminal.TargetID {
		return automationport.AutomationHistoryReceipt{}, ErrConflict
	}
	return automationport.AutomationHistoryReceipt{Kind: kind, SourceIdentifier: sourceIdentifier, PayloadDigest: terminal.PayloadDigest, TargetID: id, TargetDigest: terminal.TargetDigest}, nil
}

func automationHistoryTerminalFromReceipt(receipt automationport.AutomationHistoryReceipt) (TerminalReceipt, error) {
	key, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || !validAutomationHistoryKind(receipt.Kind) || key == ([sha256.Size]byte{}) || receipt.SourceIdentifier != SourceIdentifier(key) ||
		receipt.PayloadDigest == ([sha256.Size]byte{}) || receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.TargetID < 1 || receipt.Replayed {
		return TerminalReceipt{}, ErrInvalidScope
	}
	return TerminalReceipt{SourceKeyDigest: key, PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest}, nil
}

func automationHistoryKinds() []string {
	return []string{
		automationport.AutomationHistorySOP,
		automationport.AutomationHistoryConfig,
		automationport.AutomationHistoryPrompt,
		automationport.AutomationHistoryAgent,
	}
}

func validAutomationHistoryKind(kind string) bool {
	for _, candidate := range automationHistoryKinds() {
		if kind == candidate {
			return true
		}
	}
	return false
}

func automationHistoryScope(kind string) (tableID, target string, ok bool) {
	switch kind {
	case automationport.AutomationHistorySOP:
		return automationHistorySOPTable, automationHistorySOPTarget, true
	case automationport.AutomationHistoryConfig:
		return automationHistoryConfigTable, automationHistoryConfigTarget, true
	case automationport.AutomationHistoryPrompt:
		return automationHistoryPromptTable, automationHistoryPromptTarget, true
	case automationport.AutomationHistoryAgent:
		return automationHistoryAgentTable, automationHistoryAgentTarget, true
	default:
		return "", "", false
	}
}

func validAutomationHistoryJournals(journals map[string]*Journal) bool {
	if len(journals) != len(automationHistoryTables) {
		return false
	}
	var version, run string
	for index, kind := range automationHistoryKinds() {
		journal := journals[kind]
		table, target, ok := automationHistoryScope(kind)
		if !ok || journal == nil || journal.tx == nil || !journal.scope.valid() ||
			journal.scope.ImportVersion != automationHistoryImportVersion || journal.scope.AdapterID != v1archive.DefaultAdapterID ||
			journal.scope.TableID != table || journal.scope.TargetDomain != automationHistoryDomain || journal.scope.TargetTable != target {
			return false
		}
		if index == 0 {
			version, run = journal.scope.ImportVersion, journal.scope.ArchiveRunID
		} else if journal.scope.ImportVersion != version || journal.scope.ArchiveRunID != run {
			return false
		}
	}
	return true
}
