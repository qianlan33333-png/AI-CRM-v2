package main

import (
	"context"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const channelImportVersion = "v1-channel-a1"

func importChannel(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string, actor, dm01Run int64, key []byte) (v1domain.ChannelImportResult, error) {
	resolver, err := newChannelCustomerResolver(ctx, uow, dm01Run, key)
	if err != nil {
		return v1domain.ChannelImportResult{}, err
	}
	journals := map[string]*v1domain.Journal{}
	for _, table := range []string{
		"automation_channel", "automation_channel_assignee", "automation_channel_contact",
		"automation_channel_entry_effect_log", "automation_channel_entry_runtime", "automation_channel_qrcode_asset",
		"automation_channel_scene_alias", "channel_welcome_effect_dependency", "channel_welcome_effect_graph",
	} {
		target := "channels"
		if table == "automation_channel_contact" {
			target = "channel_historical_contacts"
		}
		if table == "automation_channel_assignee" {
			target = "channel_historical_assignees"
		}
		journal, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: channelImportVersion, ArchiveRunID: run,
			AdapterID: v1archive.DefaultAdapterID, TableID: "public/" + table, TargetDomain: "contact", TargetTable: target})
		if err != nil {
			return v1domain.ChannelImportResult{}, err
		}
		journals["public/"+table] = journal
	}
	writer, err := contactapp.NewHistoricalChannelWriter(contactstore.NewHistoricalChannelStore(), journals["public/automation_channel"])
	if err != nil {
		return v1domain.ChannelImportResult{}, err
	}
	relationJournal, err := v1domain.NewChannelRelationsJournal(journals["public/automation_channel_contact"], journals["public/automation_channel_assignee"])
	if err != nil {
		return v1domain.ChannelImportResult{}, err
	}
	relations, err := contactapp.NewHistoricalChannelRelationsWriter(contactstore.NewHistoricalChannelRelationsStore(), relationJournal)
	if err != nil {
		return v1domain.ChannelImportResult{}, err
	}
	importer, err := v1domain.NewChannelImporter(archive, uow, writer, relations, resolver, journals, actor)
	if err != nil {
		return v1domain.ChannelImportResult{}, err
	}
	return importer.Import(ctx, run)
}
