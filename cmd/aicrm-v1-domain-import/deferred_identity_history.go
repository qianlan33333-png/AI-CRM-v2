package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	deferredhistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1deferredidentityhistory"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const deferredIdentityHistoryDomain = "deferred-identity-history"

func runDeferredIdentityHistory(ctx context.Context, pool *pgxpool.Pool, archive *v1archive.PostgresArchiveReader, mode, run string, dm01Run int64, archiveKey, dm01Key []byte) error {
	unit := gapUnitOfWork{uow: platformstore.NewUnitOfWork(pool)}
	reader := v1domain.DeferredIdentityDM01Reader{UOW: unit}
	dm01, err := reader.ReadDM01Run(ctx, dm01Run)
	if err != nil {
		return err
	}
	options := deferredhistory.DeferredIdentitySelectionOptions{ArchiveRunID: run, DM01RunID: dm01Run, ArchiveHMACKey: archiveKey, DM01HMACKey: dm01Key, DM01HMACKeyVersion: dm01.HMACKeyVersion}
	var journals [3]*v1domain.Journal
	for i, scope := range [][2]string{
		{deferredhistory.PeopleTableID, v1domain.DeferredPersonHistoryTarget},
		{deferredhistory.IdentityConflictsTableID, v1domain.DeferredConflictHistoryTarget},
		{deferredhistory.ExternalContactIdentityMapID, v1domain.MissingRootIdentityTarget},
	} {
		journals[i], err = v1domain.NewJournal(v1domain.Scope{ImportVersion: v1domain.DeferredIdentityHistoryImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: scope[0], TargetDomain: "contact", TargetTable: scope[1]})
		if err != nil {
			return err
		}
	}
	journal, err := v1domain.NewDeferredIdentityHistoryJournal(journals[0], journals[1], journals[2])
	if err != nil {
		return err
	}
	writer, err := contactapp.NewDeferredIdentityHistoryWriter(contactstore.NewDeferredIdentityHistoryStore(), journal)
	if err != nil {
		return err
	}
	importer, err := v1domain.NewDeferredIdentityHistoryImporter(archive, reader, unit, writer, journal, options)
	if err != nil {
		return err
	}
	if mode == "reconcile" {
		value, err := v1domain.ReconcileDeferredIdentityHistory(ctx, pool, importer)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"deferred_identity_history_reconciliation": value})
	}
	value, err := importer.Import(ctx)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"deferred_identity_history": value})
}
