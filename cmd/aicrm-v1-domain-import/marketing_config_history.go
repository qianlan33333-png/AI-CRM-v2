package main

import (
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1marketingconfighistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

// newMarketingConfigHistoryJournal routes the config and rule source tables to
// their immutable Automation-owned receipt streams. No automation is started.
func newMarketingConfigHistoryJournal(run string) (v1domain.MarketingConfigHistoryImportJournal, error) {
	config, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: v1domain.MarketingConfigHistoryImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: v1marketingconfighistory.ConfigTableID, TargetDomain: "automation", TargetTable: v1domain.MarketingConfigHistoryConfigTarget})
	if err != nil {
		return nil, err
	}
	rule, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: v1domain.MarketingConfigHistoryImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: v1marketingconfighistory.RulesTableID, TargetDomain: "automation", TargetTable: v1domain.MarketingConfigHistoryRuleTarget})
	if err != nil {
		return nil, err
	}
	return v1domain.NewMarketingConfigHistoryJournal(config, rule)
}
