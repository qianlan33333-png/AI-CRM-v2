package store

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var archiveIdentityRootPostgresDSN = flag.String("archive-identity-root-postgres-dsn", "", "isolated PostgreSQL DSN for DM01 root lock rollback verification")

func TestLockVerifiedDM01CustomerRootRequiresCallerTransactionAndCanonicalInput(t *testing.T) {
	repository := HistoricalImportRepository{}
	key := [32]byte{1}
	for _, input := range []struct {
		run int64
		key [32]byte
	}{
		{0, key},
		{1, [32]byte{}},
	} {
		if _, found, err := repository.LockVerifiedDM01CustomerRoot(context.Background(), input.run, input.key); found || !errors.Is(err, ErrInvalidHistoricalImport) {
			t.Fatal("invalid_root_lock_input_accepted")
		}
	}
	if _, found, err := repository.LockVerifiedDM01CustomerRoot(context.Background(), 1, key); found || !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatal("root_lock_without_transaction_accepted")
	}
}

type archiveIdentityRootCounts = contactdb.CountArchiveIdentityRootFixtureRowsRow

func archiveIdentityRootCount(ctx context.Context, queryer contactdb.DBTX) (archiveIdentityRootCounts, error) {
	return contactdb.New(queryer).CountArchiveIdentityRootFixtureRows(ctx)
}

func archiveIdentityRootFixture(ctx context.Context, tx pgx.Tx, seed byte, deleted, matchingPayload bool) (int64, [32]byte, int64, error) {
	key := [32]byte{seed}
	payload := bytes.Repeat([]byte{seed + 1}, 32)
	receiptPayload := append([]byte(nil), payload...)
	if !matchingPayload {
		receiptPayload[0]++
	}
	row, err := contactdb.New(tx).CreateArchiveIdentityRootFixture(ctx, contactdb.CreateArchiveIdentityRootFixtureParams{
		Manifest:      bytes.Repeat([]byte{seed + 3}, 32),
		RepositorySha: strings.Repeat(fmt.Sprintf("%02x", seed), 20),
		Watermark:     pgtype.Timestamptz{Time: time.Date(2026, 8, 28, 0, 0, int(seed), 0, time.UTC), Valid: true},
		Deleted:       deleted, SourceKey: key[:], Payload: payload, ReceiptPayload: receiptPayload,
		FieldDigest: bytes.Repeat([]byte{seed + 2}, 32),
	})
	return row.RunID, key, row.CustomerID, err
}

func TestLockVerifiedDM01CustomerRootPostgresRollback(t *testing.T) {
	if *archiveIdentityRootPostgresDSN == "" {
		t.Skip("set -archive-identity-root-postgres-dsn for isolated DM01 root lock verification")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *archiveIdentityRootPostgresDSN)
	if err != nil {
		t.Fatal("open_isolated_postgres")
	}
	defer pool.Close()
	before, err := archiveIdentityRootCount(ctx, pool)
	if err != nil {
		t.Fatal("count_before")
	}
	rollback := errors.New("archive identity root lock rollback")
	repository, uow := HistoricalImportRepository{}, platformstore.NewUnitOfWork(pool)
	err = uow.Within(ctx, func(txCtx context.Context) error {
		tx, txErr := platformstore.TxFromContext(txCtx)
		if txErr != nil {
			return txErr
		}
		runID, key, customerID, fixtureErr := archiveIdentityRootFixture(txCtx, tx, 1, false, true)
		if fixtureErr != nil {
			return fixtureErr
		}
		got, found, lockErr := repository.LockVerifiedDM01CustomerRoot(txCtx, runID, key)
		if lockErr != nil || !found || int64(got) != customerID {
			return errors.New("verified_root_not_locked")
		}
		for _, test := range []struct {
			name string
			run  int64
			key  [32]byte
		}{
			{"wrong_run", runID + 1, key},
			{"missing", runID, [32]byte{99}},
		} {
			if got, found, checkErr := repository.LockVerifiedDM01CustomerRoot(txCtx, test.run, test.key); checkErr != nil || found || got != 0 {
				return errors.New(test.name + "_accepted")
			}
		}
		deletedRun, deletedKey, _, fixtureErr := archiveIdentityRootFixture(txCtx, tx, 2, true, true)
		if fixtureErr != nil {
			return fixtureErr
		}
		if got, found, checkErr := repository.LockVerifiedDM01CustomerRoot(txCtx, deletedRun, deletedKey); checkErr != nil || found || got != 0 {
			return errors.New("deleted_root_accepted")
		}
		driftRun, driftKey, _, fixtureErr := archiveIdentityRootFixture(txCtx, tx, 3, false, false)
		if fixtureErr != nil {
			return fixtureErr
		}
		if got, found, checkErr := repository.LockVerifiedDM01CustomerRoot(txCtx, driftRun, driftKey); checkErr != nil || found || got != 0 {
			return errors.New("payload_drift_accepted")
		}
		afterLocks, countErr := archiveIdentityRootCount(txCtx, tx)
		if countErr != nil || afterLocks != (archiveIdentityRootCounts{Mappings: before.Mappings + 3, Receipts: before.Receipts + 3}) {
			return errors.New("root_lock_wrote_data")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal("rollback_verification_failed")
	}
	after, err := archiveIdentityRootCount(ctx, pool)
	if err != nil || after != before {
		t.Fatal("rollback_changed_dm01_or_customer")
	}
}
