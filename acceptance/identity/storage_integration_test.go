package identity_test

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	identitydb "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store/generated"
)

var databaseURL = flag.String("database-url", "", "isolated PostgreSQL 16.14 identity database")

func TestCI01UnionIDLookupExcludesUnverifiedIdentity(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityStorage(t, pool)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	verifiedCustomerID, err := contactfixture.CreateCustomer(ctx, tx)
	if err != nil {
		t.Fatalf("create verified customer: %v", err)
	}
	unverifiedCustomerID, err := contactfixture.CreateCustomer(ctx, tx)
	if err != nil {
		t.Fatalf("create unverified customer: %v", err)
	}
	const unionID = "ci01-unionid-normalized"
	for _, record := range []struct {
		customerID  int64
		scope       string
		assurance   string
		fingerprint string
	}{
		{verifiedCustomerID, "wechat-open-platform:ci01-verified", "verified", "00112233445566778899aabbccddeeff"},
		{unverifiedCustomerID, "wechat-open-platform:ci01-declared", "declared", "ffeeddccbbaa99887766554433221100"},
	} {
		if _, err = tx.Exec(ctx, `
INSERT INTO identities (customer_id, kind, scope, normalized_value, normalizer_version, assurance, source, review_fingerprint, fingerprint_key_version, bound_at)
VALUES ($1::bigint, 'unionid', $2::text, $3::text, 1, $4::text, 'ci01-acceptance', decode($5::text, 'hex'), 1, now())`,
			record.customerID, record.scope, unionID, record.assurance, record.fingerprint); err != nil {
			t.Fatalf("insert %s identity: %v", record.assurance, err)
		}
	}

	rows, err := identitydb.New(tx).LookupMessageArchiveUnionIDCustomers(ctx, unionID)
	if err != nil {
		t.Fatalf("lookup unionid: %v", err)
	}
	if len(rows) != 1 || !rows[0].Valid || rows[0].Int64 != verifiedCustomerID {
		t.Fatalf("unionid lookup=%v, want verified customer %d only", rows, verifiedCustomerID)
	}
}

func TestIdentityStorageCatalogAndOwnership(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityStorage(t, pool)
	ctx := context.Background()
	var waterline int
	if err := pool.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&waterline); err != nil || waterline < 14 {
		t.Fatalf("migration waterline=%d err=%v, want at least 14", waterline, err)
	}

	tables := queryStrings(t, pool, `
SELECT table_name FROM information_schema.tables
WHERE table_schema = 'public' AND table_name IN ('identities', 'customer_merges', 'pending_events', 'identity_operation_receipts')
ORDER BY table_name`)
	if want := []string{"customer_merges", "identities", "identity_operation_receipts", "pending_events"}; !equalStrings(tables, want) {
		t.Fatalf("identity tables=%v, want %v", tables, want)
	}
	columns := queryStrings(t, pool, `
SELECT column_name FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = 'identities' ORDER BY column_name`)
	for _, required := range []string{"customer_id", "kind", "normalized_value", "review_fingerprint"} {
		if !containsSorted(columns, required) {
			t.Fatalf("identities missing column %q: %v", required, columns)
		}
	}
	for _, forbidden := range []string{"raw_identity", "raw_value", "value"} {
		if containsSorted(columns, forbidden) {
			t.Fatalf("identities has forbidden raw column %q", forbidden)
		}
	}
	indexes := queryStrings(t, pool, `
SELECT indexdef FROM pg_indexes
WHERE schemaname = 'public' AND tablename = 'identity_operation_receipts' ORDER BY indexname`)
	for _, index := range indexes {
		if containsInsensitive(index, " using gin ") || containsInsensitive(index, "(state") {
			t.Fatalf("receipt has prohibited queue/search index: %s", index)
		}
	}
}

func TestIdentityStorageConstraintsAndContactOwnedParents(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityStorage(t, pool)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	primaryID, err := contactfixture.CreateCustomer(ctx, tx)
	if err != nil {
		t.Fatalf("create primary Contact parent: %v", err)
	}
	mergedID, err := contactfixture.CreateCustomer(ctx, tx)
	if err != nil {
		t.Fatalf("create merged Contact parent: %v", err)
	}
	if primaryID < 1 || mergedID < 1 || primaryID == mergedID {
		t.Fatalf("Contact-owned parent IDs=(%d,%d)", primaryID, mergedID)
	}

	var identityID int64
	err = tx.QueryRow(ctx, `
INSERT INTO identities (customer_id, kind, scope, normalized_value, normalizer_version, assurance, source, review_fingerprint, fingerprint_key_version, bound_at)
VALUES ($1::bigint, 'unionid', 'wechat-open-platform:acceptance', 'normalized-only', 1, 'verified', 'acceptance', decode($2::text, 'hex'), 1, now())
RETURNING id`, primaryID, "00112233445566778899aabbccddeeff").Scan(&identityID)
	if err != nil || identityID < 1 {
		t.Fatalf("insert identity id=%d err=%v", identityID, err)
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO pending_events (kind, identity_ids, candidate_customer_ids, source, policy_version, event_type, payload, idempotency_key, occurred_at)
VALUES ('attribution', ARRAY[$1::bigint], ARRAY[$2::bigint], 'acceptance', 'identity_v1', 'event', '{}'::jsonb, 'event-key', now())`, identityID, mergedID); err != nil {
		t.Fatalf("insert attribution pending event: %v", err)
	}
	if _, err = tx.Exec(ctx, `SAVEPOINT invalid_merge_detail`); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `
INSERT INTO customer_merges (primary_customer_id, merged_customer_id, mode, policy_version, review_fingerprint, fingerprint_key_version, operated_by, detail)
VALUES ($1::bigint, $2::bigint, 'auto', 'merge_v1', decode($3::text, 'hex'), 1, 'acceptance', $4::jsonb)`, primaryID, mergedID, "11112222333344445555666677778888", `{"raw_value":"forbidden"}`)
	if err == nil {
		t.Fatal("customer_merges accepted a non-closed detail object")
	}
	assertPGCode(t, err, "23514")
	if _, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT invalid_merge_detail`); err != nil {
		t.Fatal(err)
	}
}

func TestIdentityReceiptCommitAndMutationConstraints(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityStorage(t, pool)
	ctx := context.Background()
	t.Run("in progress commit is rejected", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, `
INSERT INTO identity_operation_receipts (operation, idempotency_scope, key_digest, command_schema_version, payload_hmac, payload_hmac_key_version, result_schema_version)
VALUES ('bind', $1::text, decode($2::text, 'hex'), 1, decode($3::text, 'hex'), 1, 1)`, fmt.Sprintf("acceptance-%d", time.Now().UnixNano()), repeatHex("11"), repeatHex("22"))
		if err != nil {
			t.Fatal(err)
		}
		err = tx.Commit(ctx)
		if err == nil {
			t.Fatal("in_progress receipt committed")
		}
		assertPGCode(t, err, "23514")
	})
	t.Run("completed receipt is immutable", func(t *testing.T) {
		var receiptID int64
		err := pool.QueryRow(ctx, `
INSERT INTO identity_operation_receipts (operation, idempotency_scope, key_digest, command_schema_version, payload_hmac, payload_hmac_key_version, state, result_schema_version, result_status, completed_at)
VALUES ('bind', $1::text, decode($2::text, 'hex'), 1, decode($3::text, 'hex'), 1, 'completed', 1, 'bound', now()) RETURNING id`, fmt.Sprintf("acceptance-%d", time.Now().UnixNano()), repeatHex("33"), repeatHex("44")).Scan(&receiptID)
		if err != nil || receiptID < 1 {
			t.Fatalf("insert completed receipt id=%d err=%v", receiptID, err)
		}
		_, err = pool.Exec(ctx, `UPDATE identity_operation_receipts SET completed_at = now() WHERE id = $1::bigint`, receiptID)
		if err == nil {
			t.Fatal("completed receipt was mutable")
		}
		assertPGCode(t, err, "55000")
	})
}

func TestIdentityReceiptReservationCompletionTransactionBoundary(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityStorage(t, pool)
	ctx := context.Background()

	t.Run("same UoW reserves writes business fact and completes", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		scope := fmt.Sprintf("same-uow-%d", time.Now().UnixNano())
		var receiptID int64
		err = tx.QueryRow(ctx, `
INSERT INTO identity_operation_receipts (operation, idempotency_scope, key_digest, command_schema_version, payload_hmac, payload_hmac_key_version, result_schema_version)
VALUES ('bind', $1::text, decode($2::text, 'hex'), 1, decode($3::text, 'hex'), 1, 1)
RETURNING id`, scope, repeatHex("55"), repeatHex("66")).Scan(&receiptID)
		if err != nil || receiptID < 1 {
			t.Fatalf("reserve receipt id=%d err=%v", receiptID, err)
		}
		customerID, err := contactfixture.CreateCustomer(ctx, tx)
		if err != nil {
			t.Fatalf("write Contact-owned business fact: %v", err)
		}
		if _, err = tx.Exec(ctx, `
UPDATE identity_operation_receipts
SET state = 'completed', result_status = 'bound', result_customer_id = $1::bigint, completed_at = now()
WHERE id = $2::bigint`, customerID, receiptID); err != nil {
			t.Fatalf("complete receipt: %v", err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatalf("commit reservation, business fact, and completion: %v", err)
		}

		var state string
		if err = pool.QueryRow(ctx, `SELECT state FROM identity_operation_receipts WHERE id = $1::bigint`, receiptID).Scan(&state); err != nil || state != "completed" {
			t.Fatalf("receipt state=%q err=%v, want completed", state, err)
		}
	})

	t.Run("second connection cannot complete an uncommitted reservation and rollback removes it", func(t *testing.T) {
		first, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer first.Release()
		second, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer second.Release()

		scope := fmt.Sprintf("two-connection-%d", time.Now().UnixNano())
		firstTx, err := first.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = firstTx.Rollback(ctx) }()
		if _, err = firstTx.Exec(ctx, `
INSERT INTO identity_operation_receipts (operation, idempotency_scope, key_digest, command_schema_version, payload_hmac, payload_hmac_key_version, result_schema_version)
VALUES ('bind', $1::text, decode($2::text, 'hex'), 1, decode($3::text, 'hex'), 1, 1)`, scope, repeatHex("77"), repeatHex("88")); err != nil {
			t.Fatal(err)
		}
		secondTx, err := second.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = secondTx.Rollback(ctx) }()
		result, err := secondTx.Exec(ctx, `
UPDATE identity_operation_receipts
SET state = 'completed', result_status = 'bound', completed_at = now()
WHERE idempotency_scope = $1::text`, scope)
		if err != nil {
			t.Fatal(err)
		}
		if result.RowsAffected() != 0 {
			t.Fatalf("second connection completed %d uncommitted reservations", result.RowsAffected())
		}
		if err = secondTx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = firstTx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		var count int
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM identity_operation_receipts WHERE idempotency_scope = $1::text`, scope).Scan(&count); err != nil || count != 0 {
			t.Fatalf("rolled-back reservation count=%d err=%v, want 0", count, err)
		}
	})

	t.Run("in progress reservation cannot commit for a later transaction", func(t *testing.T) {
		scope := fmt.Sprintf("cross-uow-%d", time.Now().UnixNano())
		first, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer first.Release()
		reservationTx, err := first.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = reservationTx.Rollback(ctx) }()
		if _, err = reservationTx.Exec(ctx, `
INSERT INTO identity_operation_receipts (operation, idempotency_scope, key_digest, command_schema_version, payload_hmac, payload_hmac_key_version, result_schema_version)
VALUES ('bind', $1::text, decode($2::text, 'hex'), 1, decode($3::text, 'hex'), 1, 1)`, scope, repeatHex("99"), repeatHex("aa")); err != nil {
			t.Fatal(err)
		}
		err = reservationTx.Commit(ctx)
		if err == nil {
			t.Fatal("in_progress reservation committed for later completion")
		}
		assertPGCode(t, err, "23514")

		second, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer second.Release()
		result, err := second.Exec(ctx, `
UPDATE identity_operation_receipts
SET state = 'completed', result_status = 'bound', completed_at = now()
WHERE idempotency_scope = $1::text`, scope)
		if err != nil {
			t.Fatal(err)
		}
		if result.RowsAffected() != 0 {
			t.Fatalf("later transaction found %d committed reservations", result.RowsAffected())
		}
	})

	t.Run("illegal completed result and version remain rejected", func(t *testing.T) {
		_, err := pool.Exec(ctx, `
INSERT INTO identity_operation_receipts (operation, idempotency_scope, key_digest, command_schema_version, payload_hmac, payload_hmac_key_version, state, result_schema_version, result_status, completed_at)
VALUES ('bind', $1::text, decode($2::text, 'hex'), 1, decode($3::text, 'hex'), 1, 'completed', 1, 'attributed', now())`, fmt.Sprintf("illegal-status-%d", time.Now().UnixNano()), repeatHex("bb"), repeatHex("cc"))
		if err == nil {
			t.Fatal("bind receipt accepted an ingest-only completed result")
		}
		assertPGCode(t, err, "23514")

		_, err = pool.Exec(ctx, `
INSERT INTO identity_operation_receipts (operation, idempotency_scope, key_digest, command_schema_version, payload_hmac, payload_hmac_key_version, result_schema_version)
VALUES ('bind', $1::text, decode($2::text, 'hex'), 1, decode($3::text, 'hex'), 1, 0)`, fmt.Sprintf("illegal-version-%d", time.Now().UnixNano()), repeatHex("dd"), repeatHex("ee"))
		if err == nil {
			t.Fatal("receipt accepted a non-positive result schema version")
		}
		assertPGCode(t, err, "23514")
	})
}

func openIdentityPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if *databaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*databaseURL); err != nil {
		t.Fatalf("unsafe identity database URL: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v, want 160014", version, err)
	}
	return pool
}

func resetIdentityStorage(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `TRUNCATE identity_operation_receipts, pending_events, customer_merges, identities RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
}

func queryStrings(t *testing.T, pool *pgxpool.Pool, statement string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), statement)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		values = append(values, value)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(values)
	return values
}

func containsSorted(values []string, want string) bool {
	index := sort.SearchStrings(values, want)
	return index < len(values) && values[index] == want
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func repeatHex(pair string) string {
	value := ""
	for range 32 {
		value += pair
	}
	return value
}

func containsInsensitive(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		match := true
		for offset := range fragment {
			left, right := value[index+offset], fragment[offset]
			if left >= 'A' && left <= 'Z' {
				left += 'a' - 'A'
			}
			if right >= 'A' && right <= 'Z' {
				right += 'a' - 'A'
			}
			if left != right {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func assertPGCode(t *testing.T, err error, code string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code {
		t.Fatalf("PostgreSQL error=%v, want SQLSTATE %s", err, code)
	}
}
