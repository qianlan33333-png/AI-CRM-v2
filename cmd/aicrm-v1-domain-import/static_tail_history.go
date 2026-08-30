package main

import (
	"context"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediastore "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	cycleapp "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/app"
	cyclestore "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productstore "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store"
)

const staticTailHistoryImportVersion = "v1-static-tail-history-a1"

type staticTailHistoryResult struct {
	History v1domain.StaticTailHistoryImportResult `json:"history"`
}

func newStaticTailHistoryJournal(run string) (*v1domain.StaticTailHistoryJournal, error) {
	var journals [5]*v1domain.Journal
	for index, mapping := range [][3]string{
		{"public/group_invite_library", "media", "media_v1_group_invite_history"},
		{"public/wechat_pay_product_page_slices", "product", "product_v1_page_slice_history"},
		{"public/operation_cycle_strategies", "operationcycle", "operation_cycle_v1_strategy_history"},
		{"public/operation_cycle_strategy_versions", "operationcycle", "operation_cycle_v1_version_history"},
		{"public/operation_cycle_strategy_version_documents", "operationcycle", "operation_cycle_v1_document_history"},
	} {
		journal, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: staticTailHistoryImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: mapping[0], TargetDomain: mapping[1], TargetTable: mapping[2]})
		if err != nil {
			return nil, err
		}
		journals[index] = journal
	}
	return v1domain.NewStaticTailHistoryJournal(journals[0], journals[1], journals[2], journals[3], journals[4])
}

func importStaticTailHistory(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string) (staticTailHistoryResult, error) {
	journal, err := newStaticTailHistoryJournal(run)
	if err != nil {
		return staticTailHistoryResult{}, err
	}
	media, err := mediaapp.NewStaticMediaHistoryWriter(mediastore.NewStaticMediaHistoryStore(), journal)
	if err != nil {
		return staticTailHistoryResult{}, err
	}
	product, err := productapp.NewStaticProductHistoryWriter(productstore.NewStaticProductHistoryStore(), journal)
	if err != nil {
		return staticTailHistoryResult{}, err
	}
	cycle, err := cycleapp.NewStaticCycleHistoryWriter(cyclestore.NewStaticCycleHistoryStore(), journal)
	if err != nil {
		return staticTailHistoryResult{}, err
	}
	importer, err := v1domain.NewStaticTailHistoryImporter(archive, uow, media, product, cycle, journal)
	if err != nil {
		return staticTailHistoryResult{}, err
	}
	history, err := importer.Import(ctx, run)
	if err != nil {
		return staticTailHistoryResult{}, err
	}
	return staticTailHistoryResult{History: history}, nil
}
