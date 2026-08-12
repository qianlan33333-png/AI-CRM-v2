package contact_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestContactMergeLineageCommitsTagUnionSoftDeleteAndReplay(t *testing.T) {
	pool := openContactLineagePool(t)
	repository := contactstore.NewMergePortRepository()
	uow := platformstore.NewUnitOfWork(pool)

	if _, err := repository.CreateForIdentity(context.Background(), contactport.CreateForIdentityCommand{
		Actor: "acceptance",
	}); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("CreateForIdentity outside UoW error=%v", err)
	}
	if err := repository.MergeCustomers(context.Background(), contactport.MergeCustomersCommand{
		PrimaryID: 1, MergedID: 2, Actor: "acceptance", Reason: "outside transaction",
	}); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("MergeCustomers outside UoW error=%v", err)
	}

	primaryID := createLineageCustomer(t, uow, repository, "merge-primary")
	mergedID := createLineageCustomer(t, uow, repository, "merge-source")
	primaryTag := insertLineageTag(t, pool, "primary")
	sharedTag := insertLineageTag(t, pool, "shared")
	mergedTag := insertLineageTag(t, pool, "merged")
	if _, err := pool.Exec(context.Background(), `
INSERT INTO customer_tags (customer_id, tag_id, tagged_by)
VALUES ($1, $3, 'primary'), ($1, $4, 'primary'), ($2, $4, 'source'), ($2, $5, 'source')`,
		primaryID, mergedID, primaryTag, sharedTag, mergedTag); err != nil {
		t.Fatalf("seed customer tags: %v", err)
	}

	command := contactport.MergeCustomersCommand{
		PrimaryID: primaryID, MergedID: mergedID, Actor: "acceptance", Reason: "verified identity merge",
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if err := uow.Within(context.Background(), func(txCtx context.Context) error {
			return repository.MergeCustomers(txCtx, command)
		}); err != nil {
			t.Fatalf("MergeCustomers attempt=%d: %v", attempt, err)
		}
	}
	assertLineageMergeFacts(t, pool, primaryID, mergedID, []int64{primaryTag, sharedTag, mergedTag})

	for name, invalid := range map[string]contactport.MergeCustomersCommand{
		"self":    {PrimaryID: primaryID, MergedID: primaryID, Actor: "acceptance", Reason: "invalid"},
		"reverse": {PrimaryID: mergedID, MergedID: primaryID, Actor: "acceptance", Reason: "invalid"},
		"missing": {PrimaryID: primaryID, MergedID: 9223372036854775000, Actor: "acceptance", Reason: "invalid"},
	} {
		t.Run(name, func(t *testing.T) {
			err := uow.Within(context.Background(), func(txCtx context.Context) error {
				return repository.MergeCustomers(txCtx, invalid)
			})
			if err == nil {
				t.Fatal("invalid merge unexpectedly succeeded")
			}
		})
	}
}

func TestContactMergeLineageRollsBackCreateTagsDeleteAndLineage(t *testing.T) {
	pool := openContactLineagePool(t)
	repository := contactstore.NewMergePortRepository()
	uow := platformstore.NewUnitOfWork(pool)
	rollbackMarker := errors.New("downstream identity step failed")

	rolledBackName := fmt.Sprintf("rollback-create-%d", time.Now().UnixNano())
	var rolledBackID contactport.CustomerID
	err := uow.Within(context.Background(), func(txCtx context.Context) error {
		var createErr error
		rolledBackID, createErr = repository.CreateForIdentity(txCtx, contactport.CreateForIdentityCommand{
			Name: rolledBackName, Actor: "acceptance",
		})
		if createErr != nil {
			return createErr
		}
		return rollbackMarker
	})
	if !errors.Is(err, rollbackMarker) || rolledBackID <= 0 {
		t.Fatalf("rolled-back create id=%d error=%v", rolledBackID, err)
	}
	var createdCount int
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM customers WHERE id=$1`, rolledBackID).Scan(&createdCount); err != nil || createdCount != 0 {
		t.Fatalf("rolled-back customer count=%d err=%v", createdCount, err)
	}

	primaryID := createLineageCustomer(t, uow, repository, "rollback-primary")
	mergedID := createLineageCustomer(t, uow, repository, "rollback-source")
	mergedTag := insertLineageTag(t, pool, "rollback-source")
	if _, err = pool.Exec(context.Background(), `
INSERT INTO customer_tags (customer_id, tag_id, tagged_by) VALUES ($1, $2, 'source')`,
		mergedID, mergedTag); err != nil {
		t.Fatal(err)
	}
	err = uow.Within(context.Background(), func(txCtx context.Context) error {
		if mergeErr := repository.MergeCustomers(txCtx, contactport.MergeCustomersCommand{
			PrimaryID: primaryID, MergedID: mergedID, Actor: "acceptance", Reason: "must rollback",
		}); mergeErr != nil {
			return mergeErr
		}
		return rollbackMarker
	})
	if !errors.Is(err, rollbackMarker) {
		t.Fatalf("rollback error=%v", err)
	}
	var deleted bool
	var lineageCount, copiedTagCount int
	if err = pool.QueryRow(context.Background(), `SELECT is_deleted FROM customers WHERE id=$1`, mergedID).Scan(&deleted); err != nil || deleted {
		t.Fatalf("rolled-back customer deleted=%t err=%v", deleted, err)
	}
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM customer_merge_lineage WHERE merged_customer_id=$1`, mergedID).Scan(&lineageCount); err != nil || lineageCount != 0 {
		t.Fatalf("rolled-back lineage count=%d err=%v", lineageCount, err)
	}
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM customer_tags WHERE customer_id=$1 AND tag_id=$2`, primaryID, mergedTag).Scan(&copiedTagCount); err != nil || copiedTagCount != 0 {
		t.Fatalf("rolled-back copied tag count=%d err=%v", copiedTagCount, err)
	}
}

func TestContactMergeLineageRetryableDatabaseErrorRerunsWholeUoW(t *testing.T) {
	pool := openContactLineagePool(t)
	repository := contactstore.NewMergePortRepository()
	uow := platformstore.NewUnitOfWork(pool)
	primaryID := createLineageCustomer(t, uow, repository, "retry-primary")
	mergedID := createLineageCustomer(t, uow, repository, "retry-source")

	const setup = `
CREATE SCHEMA IF NOT EXISTS acceptance_fixtures;
DROP TRIGGER IF EXISTS acceptance_p3c07a_retry ON customers;
DROP FUNCTION IF EXISTS acceptance_fixtures.raise_first_p3c07a_merge_retry();
DROP SEQUENCE IF EXISTS acceptance_fixtures.p3c07a_merge_retry_attempt;
CREATE SEQUENCE acceptance_fixtures.p3c07a_merge_retry_attempt;
CREATE FUNCTION acceptance_fixtures.raise_first_p3c07a_merge_retry()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF nextval('acceptance_fixtures.p3c07a_merge_retry_attempt') = 1 THEN
    RAISE EXCEPTION 'forced acceptance retry' USING ERRCODE = '40001';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER acceptance_p3c07a_retry
BEFORE UPDATE OF is_deleted ON customers
FOR EACH ROW
WHEN (NEW.is_deleted IS TRUE AND OLD.is_deleted IS FALSE)
EXECUTE FUNCTION acceptance_fixtures.raise_first_p3c07a_merge_retry();`
	if _, err := pool.Exec(context.Background(), setup); err != nil {
		t.Fatalf("install retry trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `
DROP TRIGGER IF EXISTS acceptance_p3c07a_retry ON customers;
DROP FUNCTION IF EXISTS acceptance_fixtures.raise_first_p3c07a_merge_retry();
DROP SEQUENCE IF EXISTS acceptance_fixtures.p3c07a_merge_retry_attempt;`)
	})

	attempts := 0
	err := uow.Within(context.Background(), func(txCtx context.Context) error {
		attempts++
		return repository.MergeCustomers(txCtx, contactport.MergeCustomersCommand{
			PrimaryID: primaryID, MergedID: mergedID, Actor: "acceptance", Reason: "retry complete callback",
		})
	})
	if err != nil {
		t.Fatalf("retrying MergeCustomers: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("callback attempts=%d want 2", attempts)
	}
	assertLineageMergeFacts(t, pool, primaryID, mergedID, nil)
}

func TestSameDirectionMergeReplaySurvivesLaterLineageMerge(t *testing.T) {
	pool := openContactLineagePool(t)
	repository := contactstore.NewMergePortRepository()
	uow := platformstore.NewUnitOfWork(pool)
	finalRootID := createLineageCustomer(t, uow, repository, "replay-final-root")
	primaryID := createLineageCustomer(t, uow, repository, "replay-intermediate-root")
	mergedID := createLineageCustomer(t, uow, repository, "replay-source")
	original := contactport.MergeCustomersCommand{
		PrimaryID: primaryID, MergedID: mergedID, Actor: "acceptance", Reason: "original merge",
	}
	for _, command := range []contactport.MergeCustomersCommand{
		original,
		{PrimaryID: finalRootID, MergedID: primaryID, Actor: "acceptance", Reason: "later merge"},
		original,
	} {
		if err := uow.Within(context.Background(), func(txCtx context.Context) error {
			return repository.MergeCustomers(txCtx, command)
		}); err != nil {
			t.Fatalf("merge replay across later lineage command=%+v: %v", command, err)
		}
	}
	var directParent int64
	if err := pool.QueryRow(context.Background(), `
SELECT primary_customer_id FROM customer_merge_lineage WHERE merged_customer_id=$1`, mergedID).Scan(&directParent); err != nil {
		t.Fatal(err)
	}
	if directParent != int64(primaryID) {
		t.Fatalf("direct lineage parent=%d want original primary=%d", directParent, primaryID)
	}
}

func openContactLineagePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if *databaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*databaseURL); err != nil {
		t.Fatalf("unsafe test database URL: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(*databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 8
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL server_version_num=%q err=%v, want 160014", version, err)
	}
	return pool
}

func createLineageCustomer(
	t *testing.T,
	uow *platformstore.UnitOfWork,
	repository *contactstore.MergePortRepository,
	label string,
) contactport.CustomerID {
	t.Helper()
	var id contactport.CustomerID
	err := uow.Within(context.Background(), func(txCtx context.Context) error {
		var createErr error
		id, createErr = repository.CreateForIdentity(txCtx, contactport.CreateForIdentityCommand{
			Name: fmt.Sprintf("%s-%d", label, time.Now().UnixNano()), Actor: "acceptance",
		})
		return createErr
	})
	if err != nil || id <= 0 {
		t.Fatalf("create identity customer id=%d err=%v", id, err)
	}
	return id
}

func insertLineageTag(t *testing.T, pool *pgxpool.Pool, label string) int64 {
	t.Helper()
	var tagID int64
	name := fmt.Sprintf("merge-%s-%d", label, time.Now().UnixNano())
	if err := pool.QueryRow(context.Background(), `INSERT INTO tags (name) VALUES ($1) RETURNING id`, name).Scan(&tagID); err != nil {
		t.Fatal(err)
	}
	return tagID
}

func assertLineageMergeFacts(
	t *testing.T,
	pool *pgxpool.Pool,
	primaryID, mergedID contactport.CustomerID,
	wantPrimaryTags []int64,
) {
	t.Helper()
	var primaryDeleted, mergedDeleted bool
	var parentID int64
	if err := pool.QueryRow(context.Background(), `SELECT is_deleted FROM customers WHERE id=$1`, primaryID).Scan(&primaryDeleted); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT is_deleted FROM customers WHERE id=$1`, mergedID).Scan(&mergedDeleted); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT primary_customer_id FROM customer_merge_lineage WHERE merged_customer_id=$1`, mergedID).Scan(&parentID); err != nil {
		t.Fatal(err)
	}
	if primaryDeleted || !mergedDeleted || parentID != int64(primaryID) {
		t.Fatalf("merge facts primary_deleted=%t merged_deleted=%t parent=%d", primaryDeleted, mergedDeleted, parentID)
	}
	for _, tagID := range wantPrimaryTags {
		var tagCount int
		if err := pool.QueryRow(context.Background(), `
SELECT count(*) FROM customer_tags WHERE customer_id=$1 AND tag_id=$2`, primaryID, tagID).Scan(&tagCount); err != nil {
			t.Fatal(err)
		}
		if tagCount != 1 {
			t.Fatalf("primary customer=%d tag=%d count=%d want 1", primaryID, tagID, tagCount)
		}
	}
}
