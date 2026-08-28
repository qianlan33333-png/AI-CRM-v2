package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"
	"testing"

	automationapp "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/app"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
)

type automationHistoryReaderFake struct {
	sop    automationport.HistoricalAutomationSOP
	config automationport.HistoricalAutomationConfig
	prompt automationport.HistoricalAutomationPrompt
	agent  automationport.HistoricalAutomationAgent
}

func (reader automationHistoryReaderFake) GetHistoricalAutomationSOP(_ context.Context, id int64) (automationport.HistoricalAutomationSOP, error) {
	if id != reader.sop.ID {
		return automationport.HistoricalAutomationSOP{}, automationport.ErrAutomationHistoryUnavailable
	}
	return reader.sop, nil
}

func (reader automationHistoryReaderFake) ListHistoricalAutomationSOPs(_ context.Context, _ automationport.AutomationHistoryQuery) ([]automationport.HistoricalAutomationSOP, int64, error) {
	return []automationport.HistoricalAutomationSOP{reader.sop}, 1, nil
}

func (reader automationHistoryReaderFake) GetHistoricalAutomationConfig(_ context.Context, id int64) (automationport.HistoricalAutomationConfig, error) {
	if id != reader.config.ID {
		return automationport.HistoricalAutomationConfig{}, automationport.ErrAutomationHistoryUnavailable
	}
	return reader.config, nil
}

func (reader automationHistoryReaderFake) ListHistoricalAutomationConfigs(_ context.Context, _ automationport.AutomationHistoryQuery) ([]automationport.HistoricalAutomationConfig, int64, error) {
	return []automationport.HistoricalAutomationConfig{reader.config}, 1, nil
}

func (reader automationHistoryReaderFake) GetHistoricalAutomationPrompt(_ context.Context, id int64) (automationport.HistoricalAutomationPrompt, error) {
	if id != reader.prompt.ID {
		return automationport.HistoricalAutomationPrompt{}, automationport.ErrAutomationHistoryUnavailable
	}
	return reader.prompt, nil
}

func (reader automationHistoryReaderFake) ListHistoricalAutomationPrompts(_ context.Context, _ automationport.AutomationHistoryQuery) ([]automationport.HistoricalAutomationPrompt, int64, error) {
	return []automationport.HistoricalAutomationPrompt{reader.prompt}, 1, nil
}

func (reader automationHistoryReaderFake) GetHistoricalAutomationAgent(_ context.Context, id int64) (automationport.HistoricalAutomationAgent, error) {
	if id != reader.agent.ID {
		return automationport.HistoricalAutomationAgent{}, automationport.ErrAutomationHistoryUnavailable
	}
	return reader.agent, nil
}

func (reader automationHistoryReaderFake) ListHistoricalAutomationAgents(_ context.Context, _ automationport.AutomationHistoryQuery) ([]automationport.HistoricalAutomationAgent, int64, error) {
	return []automationport.HistoricalAutomationAgent{reader.agent}, 1, nil
}

func TestVerifyAutomationHistoryRowChecksAllTypedTargets(t *testing.T) {
	reader := automationHistoryTestReader()
	tests := []struct {
		table, target string
		id            int64
		digest        [sha256.Size]byte
	}{
		{automationHistorySOPTable, automationHistorySOPTarget, reader.sop.ID, mustAutomationHistorySOPDigest(t, reader.sop)},
		{automationHistoryConfigTable, automationHistoryConfigTarget, reader.config.ID, mustAutomationHistoryConfigDigest(t, reader.config)},
		{automationHistoryPromptTable, automationHistoryPromptTarget, reader.prompt.ID, mustAutomationHistoryPromptDigest(t, reader.prompt)},
		{automationHistoryAgentTable, automationHistoryAgentTarget, reader.agent.ID, mustAutomationHistoryAgentDigest(t, reader.agent)},
	}
	for _, test := range tests {
		t.Run(test.target, func(t *testing.T) {
			row := automationHistoryReconciliationRow(test.table, test.target, test.id, test.digest)
			proof, err := verifyAutomationHistoryRow(context.Background(), reader, row)
			if err != nil || proof == "" {
				t.Fatalf("proof=%q err=%v", proof, err)
			}
		})
	}
}

func TestVerifyAutomationHistoryRowFailsClosedOnTargetOrPayloadDrift(t *testing.T) {
	reader := automationHistoryTestReader()
	digest := mustAutomationHistoryPromptDigest(t, reader.prompt)
	row := automationHistoryReconciliationRow(automationHistoryPromptTable, automationHistoryPromptTarget, reader.prompt.ID, digest)
	driftedPayload := digestByte(99)
	row.PayloadDigest = driftedPayload[:]
	if _, err := verifyAutomationHistoryRow(context.Background(), reader, row); err == nil {
		t.Fatal("payload drift must fail")
	}
	row = automationHistoryReconciliationRow(automationHistoryPromptTable, automationHistoryAgentTarget, reader.prompt.ID, digest)
	if _, err := verifyAutomationHistoryRow(context.Background(), reader, row); err == nil {
		t.Fatal("wrong target table must fail")
	}
}

func automationHistoryTestReader() automationHistoryReaderFake {
	identity := func(id, sourceID int64, key, payload byte) automationport.HistoricalAutomationIdentity {
		return automationport.HistoricalAutomationIdentity{ID: id, SourceID: sourceID, SourceKeyDigest: digestByte(key), SourcePayloadDigest: digestByte(payload)}
	}
	return automationHistoryReaderFake{
		sop:    automationport.HistoricalAutomationSOP{HistoricalAutomationIdentity: identity(1, 11, 21, 31), PoolKey: "pool", DayIndex: -1, ContentMasked: "text", ImagesDigest: digestByte(41), CreatedAt: automationHistoryTestTime(), UpdatedAt: automationHistoryTestTime()},
		config: automationport.HistoricalAutomationConfig{HistoricalAutomationIdentity: identity(2, 12, 22, 32), AgentCode: "code", DisplayName: "name", ScenarioCode: "scenario", DraftVersion: -1, PublishedVersion: 2, PublishedAt: "", LastModifiedAt: "", LastModifiedSource: "", SubmittedAt: "", CreatedAt: automationHistoryTestTime(), UpdatedAt: automationHistoryTestTime(), ActorsDigest: digestByte(42), ConfigDigest: digestByte(43)},
		prompt: automationport.HistoricalAutomationPrompt{HistoricalAutomationIdentity: identity(3, 13, 23, 33), AgentCode: "code", DisplayName: "name", Version: -2, CreatedAt: automationHistoryTestTime(), UpdatedAt: automationHistoryTestTime(), PromptDigest: digestByte(44)},
		agent:  automationport.HistoricalAutomationAgent{HistoricalAutomationIdentity: identity(4, 14, 24, 34), ProgramSourceID: 1, WorkflowSourceID: 2, NodeSourceID: 3, TaskSourceID: 4, AgentCode: "code", AgentName: "name", OriginalType: "type", OriginalStatus: "disabled", SortOrder: -3, CreatedAt: automationHistoryTestTime(), UpdatedAt: automationHistoryTestTime(), ArchivedAt: "", ActorsDigest: digestByte(45), ConfigurationDigest: digestByte(46)},
	}
}

func automationHistoryReconciliationRow(table, target string, id int64, digest [sha256.Size]byte) reconciliationRow {
	reader := automationHistoryTestReader()
	var identity automationport.HistoricalAutomationIdentity
	switch table {
	case automationHistorySOPTable:
		identity = reader.sop.HistoricalAutomationIdentity
	case automationHistoryConfigTable:
		identity = reader.config.HistoricalAutomationIdentity
	case automationHistoryPromptTable:
		identity = reader.prompt.HistoricalAutomationIdentity
	case automationHistoryAgentTable:
		identity = reader.agent.HistoricalAutomationIdentity
	}
	domain, targetTable, targetID := automationHistoryDomain, target, strconv.FormatInt(id, 10)
	return reconciliationRow{TableID: table, SourceKeyDigest: identity.SourceKeyDigest[:], PayloadDigest: identity.SourcePayloadDigest[:], TargetDomain: &domain, TargetTable: &targetTable, TargetID: &targetID, TargetDigest: digest[:]}
}

func mustAutomationHistorySOPDigest(t *testing.T, value automationport.HistoricalAutomationSOP) [sha256.Size]byte {
	t.Helper()
	digest, err := automationapp.HistoricalAutomationSOPDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func mustAutomationHistoryConfigDigest(t *testing.T, value automationport.HistoricalAutomationConfig) [sha256.Size]byte {
	t.Helper()
	digest, err := automationapp.HistoricalAutomationConfigDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func mustAutomationHistoryPromptDigest(t *testing.T, value automationport.HistoricalAutomationPrompt) [sha256.Size]byte {
	t.Helper()
	digest, err := automationapp.HistoricalAutomationPromptDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func mustAutomationHistoryAgentDigest(t *testing.T, value automationport.HistoricalAutomationAgent) [sha256.Size]byte {
	t.Helper()
	digest, err := automationapp.HistoricalAutomationAgentDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
