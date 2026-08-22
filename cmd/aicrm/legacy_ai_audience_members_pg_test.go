package main

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/legacyaudiencemembers"
)

func TestAIAudienceMembersIdentityReaderPG16OmitsAmbiguousAndUnverifiedValues(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("CI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CI_TEST_DATABASE_URL is required for the isolated migrated PG16 test")
	}
	connection, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)

	transaction, err := connection.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback(ctx) //nolint:errcheck -- fixture cleanup.

	uniqueCustomer := insertAIAudienceMembersIdentityCustomer(t, ctx, transaction, "unique")
	ambiguousCustomer := insertAIAudienceMembersIdentityCustomer(t, ctx, transaction, "ambiguous")
	declaredCustomer := insertAIAudienceMembersIdentityCustomer(t, ctx, transaction, "declared")
	insertAIAudienceMembersIdentity(t, ctx, transaction, uniqueCustomer, "corp-a", "wm_unique", "verified")
	insertAIAudienceMembersIdentity(t, ctx, transaction, uniqueCustomer, "corp-b", "wm_unique", "verified")
	insertAIAudienceMembersIdentity(t, ctx, transaction, ambiguousCustomer, "corp-a", "wm_first", "verified")
	insertAIAudienceMembersIdentity(t, ctx, transaction, ambiguousCustomer, "corp-b", "wm_second", "verified")
	insertAIAudienceMembersIdentity(t, ctx, transaction, declaredCustomer, "corp-a", "wm_declared", "declared")

	reader := legacyAIAudienceMembersIdentityReader{queryer: transaction}
	items, err := reader.ListPrimaryExternalUserIDs(ctx, []int64{declaredCustomer, uniqueCustomer, ambiguousCustomer})
	if err != nil {
		t.Fatal(err)
	}
	want := []legacyaudiencemembers.TrustedExternalIdentity{{CustomerID: uniqueCustomer, ExternalUserID: "wm_unique"}}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("items=%#v want=%#v", items, want)
	}
}

func insertAIAudienceMembersIdentityCustomer(t *testing.T, ctx context.Context, transaction pgx.Tx, name string) int64 {
	t.Helper()
	var customerID int64
	if err := transaction.QueryRow(ctx, `INSERT INTO public.customers (name) VALUES ($1) RETURNING id`, name).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	return customerID
}

func insertAIAudienceMembersIdentity(
	t *testing.T,
	ctx context.Context,
	transaction pgx.Tx,
	customerID int64,
	scope string,
	value string,
	assurance string,
) {
	t.Helper()
	if _, err := transaction.Exec(ctx, `
INSERT INTO public.identities
  (customer_id, kind, scope, normalized_value, normalizer_version, assurance, source,
   review_fingerprint, fingerprint_key_version, bound_at)
VALUES ($1, 'wecom_external_userid', $2, $3, 1, $4, 'audience-members-pg-test',
        decode('00112233445566778899aabbccddeeff', 'hex'), 1, now())`, customerID, scope, value, assurance); err != nil {
		t.Fatal(err)
	}
}
