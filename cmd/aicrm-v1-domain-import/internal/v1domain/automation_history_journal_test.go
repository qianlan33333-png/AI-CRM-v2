package v1domain

import (
	"crypto/sha256"
	"errors"
	"testing"

	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestNewAutomationHistoryJournalBindsKindsToExactSourceTables(t *testing.T) {
	sop := automationHistoryScopedJournal(t, automationHistorySOPTable, automationHistorySOPTarget)
	config := automationHistoryScopedJournal(t, automationHistoryConfigTable, automationHistoryConfigTarget)
	prompt := automationHistoryScopedJournal(t, automationHistoryPromptTable, automationHistoryPromptTarget)
	agent := automationHistoryScopedJournal(t, automationHistoryAgentTable, automationHistoryAgentTarget)
	journal, err := NewAutomationHistoryJournal(sop, config, prompt, agent)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.ValidateAutomationHistoryImportScope("archive-run"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAutomationHistoryJournal(sop, automationHistoryScopedJournal(t, automationHistorySOPTable, automationHistoryConfigTarget), prompt, agent); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("wrong source mapping error=%v", err)
	}
	wrongRun := automationHistoryScopedJournal(t, automationHistoryAgentTable, automationHistoryAgentTarget)
	wrongRun.scope.ArchiveRunID = "other-run"
	if _, err := NewAutomationHistoryJournal(sop, config, prompt, wrongRun); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("mixed run error=%v", err)
	}
}

func TestAutomationHistoryReceiptTerminalRoundTripRejectsDrift(t *testing.T) {
	sourceKey := digestByte(7)
	digest := digestByte(8)
	receipt := automationport.AutomationHistoryReceipt{Kind: automationport.AutomationHistoryPrompt, SourceIdentifier: SourceIdentifier(sourceKey), PayloadDigest: digest, TargetID: 9, TargetDigest: digestByte(10)}
	terminal, err := automationHistoryTerminalFromReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := automationHistoryReceiptFromTerminal(receipt.Kind, receipt.SourceIdentifier, terminal)
	if err != nil || replayed != receipt {
		t.Fatalf("round trip receipt=%+v err=%v", replayed, err)
	}
	terminal.SourceKeyDigest = digestByte(11)
	if _, err := automationHistoryReceiptFromTerminal(receipt.Kind, receipt.SourceIdentifier, terminal); !errors.Is(err, ErrConflict) {
		t.Fatalf("source drift error=%v", err)
	}
	if _, err := automationHistoryTerminalFromReceipt(automationport.AutomationHistoryReceipt{Kind: automationport.AutomationHistorySOP, SourceIdentifier: receipt.SourceIdentifier, PayloadDigest: digest, TargetID: 1, TargetDigest: digestByte(1), Replayed: true}); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("replayed record error=%v", err)
	}
}

func automationHistoryScopedJournal(t *testing.T, table, target string) *Journal {
	t.Helper()
	journal, err := NewJournal(Scope{ImportVersion: automationHistoryImportVersion, ArchiveRunID: "archive-run", AdapterID: v1archive.DefaultAdapterID, TableID: table, TargetDomain: automationHistoryDomain, TargetTable: target})
	if err != nil {
		t.Fatal(err)
	}
	return journal
}

func TestAutomationHistoryKindsAreClosed(t *testing.T) {
	if validAutomationHistoryKind("current_agent") {
		t.Fatal("current automation kind must not be accepted")
	}
	if _, _, ok := automationHistoryScope("current_agent"); ok {
		t.Fatal("current automation scope must not be accepted")
	}
	if len(automationHistoryTables) != 4 || len(automationHistoryKinds()) != 4 {
		t.Fatal("expected exactly four history tables")
	}
	if digest := automationHistoryActorsDigest("config", [2]string{"actor", "value"}); digest == ([sha256.Size]byte{}) {
		t.Fatal("actor digest must be nonzero")
	}
}
