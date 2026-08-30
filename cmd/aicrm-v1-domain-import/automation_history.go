package main

import (
	"context"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1automationhistory"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	automationapp "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/app"
	automationstore "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const automationHistoryImportVersion = "v1-automation-history-a1"

func newAutomationHistoryJournal(run string) (*v1domain.AutomationHistoryJournal, error) {
	journals := make([]*v1domain.Journal, 0, 4)
	for _, spec := range []struct{ source, target string }{
		{v1automationhistory.SOPTemplateTableID, "automation_v1_sop_history"},
		{v1automationhistory.AgentConfigTableID, "automation_v1_agent_config_history"},
		{v1automationhistory.PromptRegistryTableID, "automation_v1_prompt_history"},
		{v1automationhistory.AgentsTableID, "automation_v1_agent_history"},
	} {
		journal, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: automationHistoryImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: spec.source, TargetDomain: "automation", TargetTable: spec.target})
		if err != nil {
			return nil, err
		}
		journals = append(journals, journal)
	}
	return v1domain.NewAutomationHistoryJournal(journals[0], journals[1], journals[2], journals[3])
}

func importAutomationHistory(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string) (v1domain.AutomationHistoryImportResult, error) {
	journal, err := newAutomationHistoryJournal(run)
	if err != nil {
		return v1domain.AutomationHistoryImportResult{}, err
	}
	writer, err := automationapp.NewAutomationHistoryWriter(automationstore.NewRepository(nil), journal)
	if err != nil {
		return v1domain.AutomationHistoryImportResult{}, err
	}
	importer, err := v1domain.NewAutomationHistoryImporter(archive, uow, writer, journal)
	if err != nil {
		return v1domain.AutomationHistoryImportResult{}, err
	}
	return importer.Import(ctx, run)
}

func loadEditableAutomationAgents(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string) ([]v1automationhistory.EditableAgent, error) {
	journal, err := newAutomationHistoryJournal(run)
	if err != nil {
		return nil, err
	}
	writer, err := automationapp.NewAutomationHistoryWriter(automationstore.NewRepository(nil), journal)
	if err != nil {
		return nil, err
	}
	importer, err := v1domain.NewAutomationHistoryImporter(archive, uow, writer, journal)
	if err != nil {
		return nil, err
	}
	return importer.EditableAgents(ctx, run)
}
