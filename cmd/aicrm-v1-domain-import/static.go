package main

import (
	"context"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	media "github.com/qianlan33333-png/AI-CRM-v2/internal/media"
	mediastore "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	product "github.com/qianlan33333-png/AI-CRM-v2/internal/product"
	productstore "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store"
)

func newStaticJournal(runID, tableID, domain, targetTable string) (*v1domain.Journal, error) {
	return v1domain.NewJournal(v1domain.Scope{ImportVersion: staticImportVersion, ArchiveRunID: runID,
		AdapterID: v1archive.DefaultAdapterID, TableID: tableID, TargetDomain: domain, TargetTable: targetTable})
}

func importStatic(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, runID string, actor, dm01RunID int64, dm01Key []byte) (map[string]any, error) {
	result := map[string]any{}
	groups, err := newStaticJournal(runID, "public/wecom_corp_tag_groups", "contact", "tag_groups")
	if err != nil {
		return nil, err
	}
	tags, err := newStaticJournal(runID, "public/wecom_corp_tags", "contact", "tags")
	if err != nil {
		return nil, err
	}
	bindings, err := newStaticJournal(runID, "public/contact_tags", "contact", "customer_tags")
	if err != nil {
		return nil, err
	}
	contactJournal, err := v1domain.NewContactTagJournal(groups, tags, bindings)
	if err != nil {
		return nil, err
	}
	verifier, err := v1domain.NewDM01CustomerTagVerifier(uow, contactstore.HistoricalImportRepository{}, dm01Key, dm01RunID)
	if err != nil {
		return nil, err
	}
	if err = verifier.PreflightContactTagBindings(ctx, archive, runID); err != nil {
		return nil, err
	}
	contacts, err := v1domain.NewContactTagImporter(archive, uow, contactstore.NewHistoricalTagImportRepository(), contactJournal, verifier)
	if err != nil {
		return nil, err
	}
	if result["contact"], err = contacts.Import(ctx, runID); err != nil {
		return nil, err
	}
	productJournal, err := newStaticJournal(runID, "public/wechat_pay_products", "product", "products")
	if err != nil {
		return nil, err
	}
	productWriter, err := product.NewHistoricalStaticProductWriter(productstore.NewHistoricalStaticProductStore(), productJournal)
	if err != nil {
		return nil, err
	}
	products, err := v1domain.NewProductImporter(archive, uow, productWriter, productJournal, actor)
	if err != nil {
		return nil, err
	}
	if result["product"], err = products.Import(ctx, runID); err != nil {
		return nil, err
	}
	for _, item := range []struct {
		kind   media.HistoricalStaticKind
		target string
	}{
		{media.HistoricalImage, "media_images"},
		{media.HistoricalAttachment, "media_attachments"},
	} {
		journal, err := newStaticJournal(runID, "public/"+string(item.kind), "media", item.target)
		if err != nil {
			return nil, err
		}
		writer, err := media.NewHistoricalStaticWriter(mediastore.NewHistoricalStaticStore(), journal)
		if err != nil {
			return nil, err
		}
		importer, err := v1domain.NewMediaStaticImporter(archive, uow, writer, journal, item.kind, actor)
		if err != nil {
			return nil, err
		}
		if result[string(item.kind)], err = importer.Import(ctx, runID); err != nil {
			return nil, err
		}
	}
	return result, nil
}
