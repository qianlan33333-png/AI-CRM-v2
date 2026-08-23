package contact_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestP4UserOpsCustomerReferenceShareBlocksConcurrentSoftDelete(t *testing.T) {
	pool, ctx := userOpsReferenceOpenPool(t)
	var customerID int64
	if err := pool.QueryRow(ctx, `INSERT INTO customers (name) VALUES ($1) RETURNING id`, fmt.Sprintf("userops-reference-lock-%d", time.Now().UnixNano())).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM customers WHERE id=$1`, customerID)
	})

	locked := make(chan struct{})
	release := make(chan struct{})
	completed := make(chan error, 1)
	go func() {
		completed <- platformstore.NewUnitOfWork(pool).Within(ctx, func(tx context.Context) error {
			references, err := contactstore.NewCustomerQueryRepository().ReadActiveCustomerReferences(tx, []contactport.CustomerID{contactport.CustomerID(customerID)})
			if err != nil || len(references) != 1 || references[0].ID != contactport.CustomerID(customerID) {
				return fmt.Errorf("locked reference read = %#v, %w", references, err)
			}
			close(locked)
			<-release
			return nil
		})
	}()
	select {
	case <-locked:
	case <-time.After(5 * time.Second):
		t.Fatal("customer reference transaction did not acquire SHARE")
	}

	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = blocker.Exec(ctx, `SET LOCAL statement_timeout = '200ms'`); err != nil {
		t.Fatal(err)
	}
	_, err = blocker.Exec(ctx, `UPDATE customers SET is_deleted=TRUE WHERE id=$1`, customerID)
	_ = blocker.Rollback(ctx)
	var databaseErr *pgconn.PgError
	if !errors.As(err, &databaseErr) || databaseErr.Code != "57014" {
		t.Fatalf("concurrent soft-delete error=%v, want statement timeout while SHARE is held", err)
	}

	close(release)
	select {
	case err = <-completed:
		if err != nil {
			t.Fatalf("reference transaction = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("customer reference transaction did not commit")
	}
	if _, err = pool.Exec(ctx, `UPDATE customers SET is_deleted=TRUE WHERE id=$1`, customerID); err != nil {
		t.Fatalf("soft-delete after reference transaction commit = %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE customers SET is_deleted=FALSE WHERE id=$1`, customerID); err != nil {
		t.Fatalf("restore after soft-delete = %v", err)
	}
}

func userOpsReferenceOpenPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *databaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*databaseURL); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		t.Fatalf("postgres=%q err=%v", version, err)
	}
	return pool, ctx
}
