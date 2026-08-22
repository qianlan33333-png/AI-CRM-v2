package legacyaudiencemembers

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// TestSQLRepositoryPG16UsesMigratedSegmentSnapshotAndStablePagination runs
// against a database that has the repository migrations applied through
// 00056_ai_audience_local_lifecycle.sql. All fixture writes are rolled back.
func TestSQLRepositoryPG16UsesMigratedSegmentSnapshotAndStablePagination(t *testing.T) {
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

	var serverVersion int
	if err = connection.QueryRow(ctx, "SHOW server_version_num").Scan(&serverVersion); err != nil {
		t.Fatal(err)
	}
	if serverVersion/10000 != 16 {
		t.Fatalf("PostgreSQL major = %d, want 16", serverVersion/10000)
	}

	transaction, err := connection.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback(ctx) //nolint:errcheck -- rollback is fixture cleanup.

	assertMigratedAudienceMemberTables(t, ctx, transaction)
	customerIDs := insertAudienceMemberCustomers(t, ctx, transaction)
	segmentID := insertAudienceMemberSegment(t, ctx, transaction)
	if _, err = transaction.Exec(ctx, `
INSERT INTO public.ai_audience_package_metadata
  (segment_id, lifecycle, version, created_by, updated_by)
VALUES ($1, 'active', 1, 0, 0)`, segmentID); err != nil {
		t.Fatal(err)
	}

	newest := time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC)
	tied := newest.Add(-time.Hour)
	oldest := tied.Add(-time.Hour)
	enteredAt := map[int64]time.Time{
		customerIDs["Oldest"]:    oldest,
		customerIDs["Tie named"]: tied,
		customerIDs[""]:          tied,
		customerIDs["Newest"]:    newest,
	}
	for customerID, computedAt := range enteredAt {
		if _, err = transaction.Exec(ctx, `
INSERT INTO public.segment_members (segment_id, customer_id, computed_at)
VALUES ($1, $2, $3)`, segmentID, customerID, computedAt); err != nil {
			t.Fatal(err)
		}
	}

	repository, err := NewSQLRepository(pg16ReadProvider{transaction: transaction})
	if err != nil {
		t.Fatal(err)
	}
	exists, err := repository.PackageExists(ctx, segmentID)
	if err != nil || !exists {
		t.Fatalf("PackageExists() = %v, %v", exists, err)
	}

	highTieID, lowTieID := customerIDs[""], customerIDs["Tie named"]
	if highTieID < lowTieID {
		highTieID, lowTieID = lowTieID, highTieID
	}
	page, err := repository.ListMembers(ctx, segmentID, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := MemberPage{
		Total: 4,
		Items: []MemberRecord{
			{CustomerID: highTieID, Nickname: customerNameByID(customerIDs, highTieID), EnteredAt: tied},
			{CustomerID: lowTieID, Nickname: customerNameByID(customerIDs, lowTieID), EnteredAt: tied},
		},
	}
	if !reflect.DeepEqual(page, want) {
		t.Fatalf("page = %#v, want %#v", page, want)
	}

	withoutMetadata := insertAudienceMemberSegment(t, ctx, transaction)
	exists, err = repository.PackageExists(ctx, withoutMetadata)
	if err != nil || exists {
		t.Fatalf("metadata-less PackageExists() = %v, %v, want false", exists, err)
	}
}

func assertMigratedAudienceMemberTables(t *testing.T, ctx context.Context, transaction pgx.Tx) {
	t.Helper()
	var segments, members, customers, metadata string
	if err := transaction.QueryRow(ctx, `
SELECT
  COALESCE(to_regclass('public.segments')::text, ''),
  COALESCE(to_regclass('public.segment_members')::text, ''),
  COALESCE(to_regclass('public.customers')::text, ''),
  COALESCE(to_regclass('public.ai_audience_package_metadata')::text, '')`).Scan(
		&segments, &members, &customers, &metadata,
	); err != nil {
		t.Fatal(err)
	}
	if segments == "" || members == "" || customers == "" || metadata == "" {
		t.Fatalf("required migrated tables missing: segments=%q members=%q customers=%q metadata=%q", segments, members, customers, metadata)
	}
}

func insertAudienceMemberCustomers(t *testing.T, ctx context.Context, transaction pgx.Tx) map[string]int64 {
	t.Helper()
	ids := make(map[string]int64, 4)
	for _, name := range []string{"Oldest", "Tie named", "", "Newest"} {
		var id int64
		if err := transaction.QueryRow(ctx, `
INSERT INTO public.customers (name)
VALUES ($1)
RETURNING id`, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[name] = id
	}
	return ids
}

func insertAudienceMemberSegment(t *testing.T, ctx context.Context, transaction pgx.Tx) int64 {
	t.Helper()
	var segmentID int64
	name := fmt.Sprintf("audience-members-pg16-%d", time.Now().UnixNano())
	if err := transaction.QueryRow(ctx, `
INSERT INTO public.segments
  (name, definition, refresh_mode, member_count, refresh_status)
VALUES ($1, '{}'::jsonb, 'manual', 0, 'idle')
RETURNING id`, name).Scan(&segmentID); err != nil {
		t.Fatal(err)
	}
	return segmentID
}

func customerNameByID(ids map[string]int64, customerID int64) string {
	for name, id := range ids {
		if id == customerID {
			return name
		}
	}
	return ""
}

type pg16ReadProvider struct {
	transaction pgx.Tx
}

func (provider pg16ReadProvider) Reader(context.Context) (SQLReader, error) {
	return pg16Reader{transaction: provider.transaction}, nil
}

type pg16Reader struct {
	transaction pgx.Tx
}

func (reader pg16Reader) QueryRow(ctx context.Context, query string, arguments ...any) SQLRow {
	return reader.transaction.QueryRow(ctx, query, arguments...)
}

func (reader pg16Reader) Query(ctx context.Context, query string, arguments ...any) (SQLRows, error) {
	return reader.transaction.Query(ctx, query, arguments...)
}
