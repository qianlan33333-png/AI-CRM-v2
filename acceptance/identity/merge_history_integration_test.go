package identity_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identitystore "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestCustomerMergeHistoryReadsWholeConnectedComponentWithoutPrivateAuditFields(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	primary := createBindCustomer(t, pool)
	middle := createBindCustomer(t, pool)
	leaf := createBindCustomer(t, pool)
	unrelatedPrimary := createBindCustomer(t, pool)
	unrelatedMerged := createBindCustomer(t, pool)

	first := insertMergeHistoryAudit(t, pool, primary, middle, "auto", "verified_unionid_unique_wecom_v1", time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC))
	second := insertMergeHistoryAudit(t, pool, middle, leaf, "manual", "verified_phone_review_v1", time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC))
	_ = insertMergeHistoryAudit(t, pool, unrelatedPrimary, unrelatedMerged, "auto", "unrelated_v1", time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC))

	service := identityapp.NewCustomerMergeHistoryService(platformstore.NewUnitOfWork(pool), identitystore.NewRepository())
	page, err := service.ListCustomerMergeHistory(ctx, contactport.CustomerID(primary), "", 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].MergeAuditID != second || page.NextCursor == "" {
		t.Fatalf("first page=%#v err=%v", page, err)
	}
	next, err := service.ListCustomerMergeHistory(ctx, contactport.CustomerID(primary), page.NextCursor, 1)
	if err != nil || len(next.Items) != 1 || next.Items[0].MergeAuditID != first || next.NextCursor != "" {
		t.Fatalf("next page=%#v err=%v", next, err)
	}
	fromLeaf, err := service.ListCustomerMergeHistory(ctx, contactport.CustomerID(leaf), "", 50)
	if err != nil || len(fromLeaf.Items) != 2 || fromLeaf.Items[0].MergeAuditID != second || fromLeaf.Items[1].MergeAuditID != first {
		t.Fatalf("leaf page=%#v err=%v", fromLeaf, err)
	}
	for _, item := range fromLeaf.Items {
		if item.PrimaryCustomerID == contactport.CustomerID(unrelatedPrimary) || item.MergedCustomerID == contactport.CustomerID(unrelatedMerged) {
			t.Fatalf("unrelated audit leaked: %#v", item)
		}
	}
}

func insertMergeHistoryAudit(t *testing.T, pool *pgxpool.Pool, primary, merged int64, mode, policy string, mergedAt time.Time) int64 {
	t.Helper()
	detail := `{"policy_version":"` + policy + `","mode":"` + mode + `","fingerprint_version":1,"fingerprint":"hmac-sha256-v1:AAAAAAAAAAAAAAAAAAAAAA"}`
	var id int64
	err := pool.QueryRow(context.Background(), `
		INSERT INTO customer_merges (
			primary_customer_id, merged_customer_id, mode, policy_version,
			review_fingerprint, fingerprint_key_version, operated_by, detail, merged_at
		) VALUES ($1::bigint,$2::bigint,$3::text,$4::text,decode('00000000000000000000000000000000','hex'),1,'acceptance:merge-history',$5::jsonb,$6::timestamptz)
		RETURNING id`, primary, merged, mode, policy, detail, mergedAt).Scan(&id)
	if err != nil || id <= 0 {
		t.Fatalf("insert merge audit id=%d err=%v", id, err)
	}
	return id
}
