package identity_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	identitystore "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestIdentityResolveReturnsOnlyFoundNotFoundOrConflict(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	service := identityapp.NewResolveService(platformstore.NewUnitOfWork(pool), identitystore.NewRepository())

	floatingRef := identityport.IDRef{Kind: identityport.KindPhone, Scope: "phone:e164", Value: " +86 (138) 0013-8000 "}
	result, err := service.Resolve(ctx, floatingRef)
	if err != nil || result != (identityport.ResolveResult{Status: identityport.ResolveNotFound}) {
		t.Fatalf("missing Resolve()=%+v err=%v", result, err)
	}

	insertResolveIdentity(t, pool, "phone", "phone:e164", "+8613800138000")
	result, err = service.Resolve(ctx, floatingRef)
	if err != nil || result != (identityport.ResolveResult{Status: identityport.ResolveNotFound}) {
		t.Fatalf("floating Resolve()=%+v err=%v", result, err)
	}

	foundCustomerID := createResolveCustomer(t, pool)
	foundIdentityID := insertResolveIdentity(t, pool, "unionid", "wechat-open-platform:resolve", "found-unionid")
	bindResolveIdentity(t, pool, foundIdentityID, foundCustomerID)
	result, err = service.Resolve(ctx, identityport.IDRef{Kind: identityport.KindUnionID, Scope: "wechat-open-platform:resolve", Value: " found-unionid "})
	wantFound := identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: contactport.CustomerID(foundCustomerID)}
	if err != nil || result != wantFound {
		t.Fatalf("found Resolve()=%+v err=%v, want %+v", result, err, wantFound)
	}

	conflictCustomerID := createResolveCustomer(t, pool)
	conflictIdentityID := insertResolveIdentity(t, pool, "ext", "ext:resolve", "conflicted")
	bindResolveIdentity(t, pool, conflictIdentityID, conflictCustomerID)
	softDeleteResolveCustomer(t, pool, conflictCustomerID)
	result, err = service.Resolve(ctx, identityport.IDRef{Kind: identityport.KindExtension, Scope: "ext:resolve", Value: "conflicted"})
	if err != nil || result != (identityport.ResolveResult{Status: identityport.ResolveConflict}) {
		t.Fatalf("conflict Resolve()=%+v err=%v", result, err)
	}

	var identities int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM identities`).Scan(&identities); err != nil || identities != 3 {
		t.Fatalf("Resolve implicitly mutated identities count=%d err=%v, want 3", identities, err)
	}
}

func TestIdentityResolveRejectsInvalidValueWithoutImplicitCreation(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	service := identityapp.NewResolveService(platformstore.NewUnitOfWork(pool), identitystore.NewRepository())
	_, err := service.Resolve(context.Background(), identityport.IDRef{Kind: identityport.KindPhone, Scope: "phone:e164", Value: "13800138000"})
	if err == nil {
		t.Fatal("Resolve accepted a non-E.164 phone")
	}
	var identities int
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM identities`).Scan(&identities); err != nil || identities != 0 {
		t.Fatalf("invalid Resolve mutated identities count=%d err=%v", identities, err)
	}
}

func createResolveCustomer(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	customerID, err := contactfixture.CreateCustomer(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return customerID
}

func softDeleteResolveCustomer(t *testing.T, pool *pgxpool.Pool, customerID int64) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = contactfixture.SoftDeleteCustomer(ctx, tx, customerID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func insertResolveIdentity(t *testing.T, pool *pgxpool.Pool, kind, scope, normalizedValue string) int64 {
	t.Helper()
	var identityID int64
	err := pool.QueryRow(context.Background(), `
INSERT INTO identities (kind, scope, normalized_value, normalizer_version, assurance, source, review_fingerprint, fingerprint_key_version)
VALUES ($1::text, $2::text, $3::text, 1, 'declared', 'acceptance.identity.resolve', decode('00000000000000000000000000000000', 'hex'), 1)
RETURNING id`, kind, scope, normalizedValue).Scan(&identityID)
	if err != nil || identityID <= 0 {
		t.Fatalf("insert resolve identity id=%d err=%v", identityID, err)
	}
	return identityID
}

func bindResolveIdentity(t *testing.T, pool *pgxpool.Pool, identityID, customerID int64) {
	t.Helper()
	commandTag, err := pool.Exec(context.Background(), `
UPDATE identities
SET customer_id = $1::bigint, bound_at = now()
WHERE id = $2::bigint`, customerID, identityID)
	if err != nil || commandTag.RowsAffected() != 1 {
		t.Fatalf("bind resolve identity rows=%d err=%v", commandTag.RowsAffected(), err)
	}
}
