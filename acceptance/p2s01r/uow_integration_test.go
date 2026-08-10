package p2s01r_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestUnitOfWorkPostgreSQL16CommitAndRollback(t *testing.T) {
	databaseURL := os.Getenv("ACCEPTANCE_FIXTURES_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ACCEPTANCE_FIXTURES_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fixture, err := acceptancefixtures.OpenPostgreSQL(ctx, databaseURL)
	if err != nil {
		t.Fatalf("OpenPostgreSQL() error = %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if cleanupErr := fixture.Cleanup(cleanupCtx); cleanupErr != nil {
			t.Errorf("Cleanup() error = %v", cleanupErr)
		}
	})
	if _, err = fixture.Pool().Exec(ctx, `CREATE TABLE acceptance_fixtures.uow_probe (id bigint PRIMARY KEY)`); err != nil {
		t.Fatalf("create UoW probe: %v", err)
	}

	uow := platformstore.NewUnitOfWork(fixture.Pool())
	if err = uow.Within(ctx, func(txCtx context.Context) error {
		tx, txErr := platformstore.TxFromContext(txCtx)
		if txErr != nil {
			return txErr
		}
		_, txErr = tx.Exec(txCtx, `INSERT INTO acceptance_fixtures.uow_probe (id) VALUES (1)`)
		return txErr
	}); err != nil {
		t.Fatalf("committed Within() error = %v", err)
	}

	sentinel := errors.New("force rollback")
	if err = uow.Within(ctx, func(txCtx context.Context) error {
		tx, txErr := platformstore.TxFromContext(txCtx)
		if txErr != nil {
			return txErr
		}
		if _, txErr = tx.Exec(txCtx, `INSERT INTO acceptance_fixtures.uow_probe (id) VALUES (2)`); txErr != nil {
			return txErr
		}
		return sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("rolled back Within() error = %v, want sentinel", err)
	}

	var ids []int64
	rows, err := fixture.Pool().Query(ctx, `SELECT id FROM acceptance_fixtures.uow_probe ORDER BY id`)
	if err != nil {
		t.Fatalf("query UoW probe: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if scanErr := rows.Scan(&id); scanErr != nil {
			t.Fatalf("scan UoW probe: %v", scanErr)
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		t.Fatalf("iterate UoW probe: %v", err)
	}
	if len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("persisted IDs = %v, want [1]", ids)
	}
}
