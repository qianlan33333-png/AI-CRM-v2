// Command aicrm-dm01 starts one operator-controlled DM01 local import mode.
package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	identitystore "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func main() {
	if err := run(os.Args[1:], appconfig.LoadDM01RuntimeEnvironment()); err != nil {
		fmt.Fprintln(os.Stderr, "aicrm-dm01:", err)
		os.Exit(2)
	}
}

func run(args []string, runtime appconfig.DM01Runtime) error {
	flags := flag.NewFlagSet("aicrm-dm01", flag.ContinueOnError)
	mode := flags.String("mode", "", "preflight|full|incremental|reconcile")
	manifestPath := flags.String("source-manifest", "", "manifest path")
	manifestDigest := flags.String("manifest-sha256", "", "manifest sha256")
	parentRunID := flags.Int64("parent-run-id", 0, "required only for reconcile")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !migration.RunMode(*mode).Valid() || *manifestPath == "" || *manifestDigest == "" {
		return fmt.Errorf("mode, source-manifest and manifest-sha256 are required")
	}
	if (migration.RunMode(*mode) == migration.ModeReconcile) != (*parentRunID > 0) {
		return fmt.Errorf("parent-run-id is required only for reconcile")
	}
	source, target, hmacKey, archiveKey := runtime.SourceDatabaseURL, runtime.TargetDatabaseURL, runtime.SourceHMACKey, runtime.ArchiveKey
	if source == "" || target == "" || hmacKey == "" || archiveKey == "" {
		return fmt.Errorf("DM01 runtime configuration is incomplete")
	}
	if source == target {
		return fmt.Errorf("source and target database URLs must differ")
	}
	if sha256.Sum256([]byte(hmacKey)) == sha256.Sum256([]byte(archiveKey)) {
		return fmt.Errorf("source HMAC and archive keys must differ")
	}
	if len(hmacKey) < 32 || len(archiveKey) != 32 {
		return fmt.Errorf("DM01 cryptographic key lengths are invalid")
	}
	manifest, digest, err := migration.LoadManifest(*manifestPath, *manifestDigest)
	if err != nil {
		return err
	}
	ctx := context.Background()
	sourceReader, err := contactstore.OpenDM01SourceReader(ctx, source)
	if err != nil {
		return fmt.Errorf("open source database: %w", err)
	}
	defer sourceReader.Close()
	targetPool, err := pgxpool.New(ctx, target)
	if err != nil {
		return fmt.Errorf("open target database: %w", err)
	}
	defer targetPool.Close()
	if err = targetPool.Ping(ctx); err != nil {
		return fmt.Errorf("ping target database: %w", err)
	}
	sourceIdentity, err := sourceReader.DatabaseIdentity(ctx)
	if err != nil {
		return fmt.Errorf("identify source database: %w", err)
	}
	targetIdentity, err := contactstore.DM01TargetDatabaseIdentity(ctx, targetPool)
	if err != nil {
		return fmt.Errorf("identify target database: %w", err)
	}
	if sourceIdentity == targetIdentity {
		return fmt.Errorf("source and target resolve to the same database")
	}
	uow := platformstore.NewUnitOfWork(targetPool)
	contacts := contactstore.HistoricalImportRepository{}
	executor := migration.NewExecutor(sourceReader, uow, contacts, contacts, identitystore.NewRepository())
	_, err = executor.Execute(ctx, migration.ExecuteCommand{Manifest: manifest, ManifestDigest: digest[:], Mode: migration.RunMode(*mode), ParentRunID: *parentRunID, HMACKey: []byte(hmacKey), ArchiveKey: []byte(archiveKey)})
	return err
}
