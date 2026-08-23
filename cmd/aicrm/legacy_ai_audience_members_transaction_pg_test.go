package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	identitystore "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/legacyaudiencemembers"
	segmentdb "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store/generated"
)

type blockingTrustedWeComReader struct {
	reader  identityport.TrustedWeComIdentityReader
	entered chan struct{}
	release chan struct{}
}

func (reader *blockingTrustedWeComReader) ListPrimaryWeComExternalUserIDs(
	ctx context.Context,
	customerIDs []contactport.CustomerID,
) ([]identityport.TrustedWeComExternalIdentity, error) {
	close(reader.entered)
	select {
	case <-reader.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return reader.reader.ListPrimaryWeComExternalUserIDs(ctx, customerIDs)
}

func TestAIAudienceMembersPG16RefreshCannotSplitMemberAndIdentitySnapshot(t *testing.T) {
	dsn := os.Getenv("CI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CI_TEST_DATABASE_URL is required for the isolated PG16 test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var version string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v, want 160014", version, err)
	}

	segmentID, customerID, externalUserID := insertAudienceMembersTransactionFixture(t, ctx, pool)
	defer deleteAudienceMembersTransactionFixture(t, pool, segmentID, customerID)

	identityGate := &blockingTrustedWeComReader{
		reader: identitystore.NewRepository(), entered: make(chan struct{}), release: make(chan struct{}),
	}
	service, err := legacyaudiencemembers.NewService(
		legacyaudiencemembers.NewSQLRepository(),
		legacyaudiencemembers.NewSQLRepository(),
		legacyAIAudienceMembersIdentityReader{reader: identityGate},
	)
	if err != nil {
		t.Fatal(err)
	}
	application := legacyAIAudienceMembersApplication{
		uow: platformstore.NewUnitOfWork(pool), application: service,
	}
	type readResult struct {
		response legacyaudiencemembers.ListResponse
		err      error
	}
	readDone := make(chan readResult, 1)
	go func() {
		response, readErr := application.ListMembers(ctx, legacyaudiencemembers.ListInput{
			PackageID: segmentID, Limit: 1,
		})
		readDone <- readResult{response: response, err: readErr}
	}()

	select {
	case <-identityGate.entered:
	case <-ctx.Done():
		t.Fatal("read did not reach the identity projection")
	}
	refreshTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = refreshTx.Exec(ctx, `SET LOCAL lock_timeout = '250ms'`); err != nil {
		t.Fatal(err)
	}
	_, lockErr := segmentdb.New(refreshTx).LockSegmentDefinitionForRefresh(ctx, segmentID)
	_ = refreshTx.Rollback(ctx)
	var pgErr *pgconn.PgError
	if !errors.As(lockErr, &pgErr) || pgErr.Code != "55P03" {
		t.Fatalf("concurrent refresh lock error=%v, want PostgreSQL 55P03", lockErr)
	}

	close(identityGate.release)
	var result readResult
	select {
	case result = <-readDone:
	case <-ctx.Done():
		t.Fatal("transactional member read did not complete")
	}
	if result.err != nil || len(result.response.Items) != 1 ||
		result.response.Items[0].CustomerID != customerID || result.response.Items[0].ExternalUserID != externalUserID {
		t.Fatalf("response=%#v err=%v", result.response, result.err)
	}

	refreshTx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer refreshTx.Rollback(ctx) //nolint:errcheck -- fixture read lock cleanup.
	if _, err = segmentdb.New(refreshTx).LockSegmentDefinitionForRefresh(ctx, segmentID); err != nil {
		t.Fatalf("refresh lock after snapshot commit: %v", err)
	}
}

func insertAudienceMembersTransactionFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (int64, int64, string) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck -- fixture failure cleanup.
	var customerID, segmentID int64
	externalUserID := fmt.Sprintf("wm_snapshot_%d", time.Now().UnixNano())
	if err = tx.QueryRow(ctx, `INSERT INTO customers(name) VALUES($1) RETURNING id`,
		fmt.Sprintf("audience-member-tx-%d", time.Now().UnixNano())).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `
INSERT INTO segments(name, definition, refresh_mode, member_count, refresh_status)
VALUES($1, '{}'::jsonb, 'manual', 1, 'idle') RETURNING id`,
		fmt.Sprintf("audience-member-segment-%d", time.Now().UnixNano())).Scan(&segmentID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO ai_audience_package_metadata(segment_id, lifecycle, version, created_by, updated_by)
VALUES($1, 'active', 1, 0, 0)`, segmentID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO segment_members(segment_id, customer_id, computed_at) VALUES($1, $2, now())`,
		segmentID, customerID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO identities(customer_id, kind, scope, normalized_value, normalizer_version, assurance, source,
  review_fingerprint, fingerprint_key_version, bound_at)
VALUES($1, 'wecom_external_userid', 'corp', $2, 1, 'verified', 'audience-member-tx-test',
  decode('00112233445566778899aabbccddeeff', 'hex'), 1, now())`, customerID, externalUserID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return segmentID, customerID, externalUserID
}

func deleteAudienceMembersTransactionFixture(t *testing.T, pool *pgxpool.Pool, segmentID, customerID int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `DELETE FROM segments WHERE id=$1`, segmentID); err != nil {
		t.Errorf("cleanup audience member segment fixture: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `DELETE FROM identities WHERE customer_id=$1`, customerID); err != nil {
		t.Errorf("cleanup audience member identity fixture: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `DELETE FROM customers WHERE id=$1`, customerID); err != nil {
		t.Errorf("cleanup audience member customer fixture: %v", err)
	}
}

var _ identityport.TrustedWeComIdentityReader = (*blockingTrustedWeComReader)(nil)
