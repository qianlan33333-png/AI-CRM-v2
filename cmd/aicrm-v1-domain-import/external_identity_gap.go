package main

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	identitystore "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

// Reconciliation supplies one outer transaction; ordinary import opens its own.
// This adapter is private to this migration and does not alter normal UoW rules.
type gapUnitOfWork struct{ uow *platformstore.UnitOfWork }

func (u gapUnitOfWork) Within(ctx context.Context, apply func(context.Context) error) error {
	if _, err := platformstore.TxFromContext(ctx); err == nil {
		return apply(ctx)
	}
	return u.uow.Within(ctx, apply)
}

type gapDM01ReceiptReader struct{ uow gapUnitOfWork }

func (reader gapDM01ReceiptReader) EachDM01ExternalIdentityReceipt(ctx context.Context, runID int64, emit func(v1domain.DM01ExternalIdentityReceipt) error) error {
	if runID < 1 || emit == nil {
		return v1domain.ErrInvalidScope
	}
	return reader.uow.Within(ctx, func(tx context.Context) error {
		repo := contactstore.HistoricalImportRepository{}
		mode, state, err := repo.ReadHistoricalImportRunSnapshot(tx, runID)
		if err != nil || mode != "full" || state != "imported" {
			return fmt.Errorf("external identity gap requires a completed full DM01 run")
		}
		var ordinal int64
		return repo.StreamReconcileReceipts(tx, runID, contactmigration.ReconcileExternalIdentity, func(receipt contactmigration.ReconcileReceipt) error {
			if len(receipt.SourceFact.SourceKeyHMAC) != sha256.Size {
				return v1domain.ErrConflict
			}
			ordinal++
			var key [sha256.Size]byte
			copy(key[:], receipt.SourceFact.SourceKeyHMAC)
			return emit(v1domain.DM01ExternalIdentityReceipt{SourceOrdinal: ordinal, SourceKeyHMAC: key, Disposition: receipt.Disposition})
		})
	})
}

func newExternalIdentityGapImporter(archive v1domain.ArchiveSource, uow *platformstore.UnitOfWork, run string) (*v1domain.ExternalIdentityGapImporter, error) {
	journal, err := v1domain.NewJournal(v1domain.Scope{
		ImportVersion: "v1-external-identity-gap-a1", ArchiveRunID: run,
		AdapterID: v1archive.DefaultAdapterID, TableID: "public/wecom_external_contact_identity_map",
		TargetDomain: "identity", TargetTable: "identities",
	})
	if err != nil {
		return nil, err
	}
	gapJournal, err := v1domain.NewExternalIdentityGapImportJournal(journal)
	if err != nil {
		return nil, err
	}
	unit := gapUnitOfWork{uow: uow}
	return v1domain.NewExternalIdentityGapImporter(archive, unit, identitystore.NewArchiveIdentityRepository(), contactstore.HistoricalImportRepository{}, gapDM01ReceiptReader{uow: unit}, gapJournal)
}
