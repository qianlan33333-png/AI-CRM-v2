package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var dm01ExternalIdentityArchiveDiffEnabled = flag.Bool("dm01-external-identity-archive-diff", false, "compare a reconciled V2 archive identity table with one DM01 run using read-only transactions")
var dm01ExternalIdentityArchiveDiffArchiveRun = flag.String("dm01-external-identity-archive-diff-archive-run", "", "reconciled V2 archive run for the DM01 identity difference check")
var dm01ExternalIdentityArchiveDiffRun = flag.Int64("dm01-external-identity-archive-diff-run", 0, "full imported DM01 run for the archive identity difference check")

func TestDM01ExternalIdentityArchiveDiffReadOnly(t *testing.T) {
	if !*dm01ExternalIdentityArchiveDiffEnabled {
		t.Skip("supply -dm01-external-identity-archive-diff with archive and DM01 run flags for the read-only difference check")
	}
	if *dm01ExternalIdentityArchiveDiffArchiveRun == "" || *dm01ExternalIdentityArchiveDiffRun < 1 {
		t.Fatal("archive run and positive DM01 run are required")
	}
	archiveEnvironment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	dm01Environment := appconfig.LoadDM01RuntimeEnvironment()
	if archiveEnvironment.SourceDatabaseURL != "" || dm01Environment.SourceDatabaseURL != "" {
		t.Fatal("read-only difference check refuses configured V1 source URLs")
	}
	if archiveEnvironment.TargetDatabaseURL == "" || dm01Environment.TargetDatabaseURL == "" || len(archiveEnvironment.ArchiveKey) != sha256.Size || len(archiveEnvironment.SourceHMACKey) < sha256.Size || len(dm01Environment.SourceHMACKey) < sha256.Size {
		t.Fatal("V2 target and frozen archive/DM01 keys are required")
	}

	ctx := context.Background()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, archiveEnvironment.TargetDatabaseURL, []byte(archiveEnvironment.ArchiveKey))
	if err != nil {
		t.Fatal("cannot open reconciled V2 archive")
	}
	defer archive.Close()
	pool, err := pgxpool.New(ctx, dm01Environment.TargetDatabaseURL)
	if err != nil {
		t.Fatal("cannot open V2 DM01 target")
	}
	defer pool.Close()

	receipts := dm01ExternalIdentityReceiptReader{uow: platformstore.NewUnitOfWork(dm01ReadOnlyBeginner{pool: pool})}
	result, err := v1domain.DiffDM01ExternalIdentityArchive(ctx, archive, receipts, *dm01ExternalIdentityArchiveDiffArchiveRun, *dm01ExternalIdentityArchiveDiffRun, []byte(archiveEnvironment.SourceHMACKey), []byte(dm01Environment.SourceHMACKey))
	if err != nil {
		stage := "unknown"
		var failure *v1domain.DM01ExternalIdentityArchiveDiffError
		if errors.As(err, &failure) {
			stage = failure.Stage
		}
		t.Fatalf("read-only DM01/archive difference check failed: stage=%s", stage)
	}
	if result.ArchiveRows != 23936 || result.DM01TerminalRows != 23873 || result.Intersection+result.OnlyArchive != result.ArchiveRows || result.Intersection+result.OnlyDM01 != result.DM01TerminalRows {
		t.Fatal("DM01/archive difference counts are not conserved")
	}
	t.Logf("dm01 external identity archive diff: archive_rows=%d dm01_terminal_rows=%d intersection=%d only_archive=%d only_dm01=%d summary_sha256=%x", result.ArchiveRows, result.DM01TerminalRows, result.Intersection, result.OnlyArchive, result.OnlyDM01, result.SummaryDigest)
	shape, err := v1domain.AggregateDM01ExternalIdentityArchiveOnly(ctx, archive, receipts, *dm01ExternalIdentityArchiveDiffArchiveRun, *dm01ExternalIdentityArchiveDiffRun, []byte(archiveEnvironment.SourceHMACKey), []byte(dm01Environment.SourceHMACKey))
	if err != nil || shape.ArchiveRows != result.ArchiveRows || shape.DM01TerminalRows != result.DM01TerminalRows || shape.OnlyArchive != result.OnlyArchive {
		t.Fatal("read-only archive-only shape check failed")
	}
	t.Logf("dm01 archive-only shape: only_archive=%d canonical_source_shape=%d required_field_redacted=%d empty_external=%d empty_union=%d empty_corp=%d invalid_updated_at=%d summary_sha256=%x", shape.OnlyArchive, shape.CanonicalSourceShape, shape.RequiredFieldRedacted, shape.ExternalUserIDMissingOrBlank, shape.UnionIDMissingOrBlank, shape.CorpIDMissingOrBlank, shape.UpdatedAtMissingOrInvalid, shape.SummaryDigest)
	roots, err := v1domain.AggregateDM01ExternalIdentityArchiveOnlyRootCandidates(ctx, archive, receipts, receipts, *dm01ExternalIdentityArchiveDiffArchiveRun, *dm01ExternalIdentityArchiveDiffRun, []byte(archiveEnvironment.SourceHMACKey), []byte(dm01Environment.SourceHMACKey))
	if err != nil || roots.OnlyArchive != shape.OnlyArchive || roots.UnionIDNotVerifiable+roots.SourceShapeNotEligible+roots.CustomerRootVerified+roots.CustomerRootMissing != roots.OnlyArchive {
		t.Fatal("read-only archive-only root candidate check failed")
	}
	t.Logf("dm01 archive-only root candidates: only_archive=%d union_not_verifiable=%d source_shape_not_eligible=%d customer_root_verified=%d customer_root_missing=%d summary_sha256=%x", roots.OnlyArchive, roots.UnionIDNotVerifiable, roots.SourceShapeNotEligible, roots.CustomerRootVerified, roots.CustomerRootMissing, roots.SummaryDigest)
}

type dm01ReadOnlyBeginner struct{ pool *pgxpool.Pool }

func (beginner dm01ReadOnlyBeginner) BeginTx(ctx context.Context, _ pgx.TxOptions) (pgx.Tx, error) {
	if beginner.pool == nil {
		return nil, errors.New("DM01 read-only transaction pool is required")
	}
	return beginner.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
}

type dm01ExternalIdentityReceiptReader struct {
	uow  *platformstore.UnitOfWork
	repo contactstore.HistoricalImportRepository
}

func (reader dm01ExternalIdentityReceiptReader) VerifyDM01CustomerIdentitySnapshot(ctx context.Context, runID int64, key [32]byte) (bool, error) {
	if reader.uow == nil {
		return false, errors.New("DM01 read-only root verifier is invalid")
	}
	var verified bool
	err := reader.uow.Within(ctx, func(tx context.Context) error {
		var err error
		verified, err = reader.repo.VerifyDM01CustomerIdentitySnapshot(tx, runID, key)
		return err
	})
	return verified, err
}

func (reader dm01ExternalIdentityReceiptReader) EachDM01ExternalIdentityReceipt(ctx context.Context, runID int64, emit func(v1domain.DM01ExternalIdentityReceipt) error) error {
	if reader.uow == nil || runID < 1 || emit == nil {
		return errors.New("DM01 read-only receipt reader is invalid")
	}
	return reader.uow.Within(ctx, func(tx context.Context) error {
		mode, state, err := reader.repo.ReadHistoricalImportRunSnapshot(tx, runID)
		if err != nil {
			return &v1domain.DM01ExternalIdentityArchiveDiffError{Stage: "receipt_run_snapshot"}
		}
		if mode != "full" || state != "imported" {
			return &v1domain.DM01ExternalIdentityArchiveDiffError{Stage: "receipt_run_state"}
		}
		var ordinal int64
		err = reader.repo.StreamReconcileReceipts(tx, runID, contactmigration.ReconcileExternalIdentity, func(receipt contactmigration.ReconcileReceipt) error {
			if len(receipt.SourceFact.SourceKeyHMAC) != sha256.Size {
				return &v1domain.DM01ExternalIdentityArchiveDiffError{Stage: "receipt_adapter"}
			}
			ordinal++
			var sourceKey [sha256.Size]byte
			copy(sourceKey[:], receipt.SourceFact.SourceKeyHMAC)
			return emit(v1domain.DM01ExternalIdentityReceipt{SourceOrdinal: ordinal, SourceKeyHMAC: sourceKey, Disposition: receipt.Disposition})
		})
		if err == nil {
			return nil
		}
		var failure *v1domain.DM01ExternalIdentityArchiveDiffError
		if errors.As(err, &failure) {
			return failure
		}
		return &v1domain.DM01ExternalIdentityArchiveDiffError{Stage: "receipt_stream_repository"}
	})
}
