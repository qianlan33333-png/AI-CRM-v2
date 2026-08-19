package contact_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestCustomerProfileTags0301PostgreSQLProjectionIsLocalAndReadOnly(t *testing.T) {
	pool, ctx := c01OpenPool(t)
	marker := fmt.Sprintf("customer-profile-tags-0301-%d", time.Now().UnixNano())
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	customerID, err := contactfixture.CreateCustomer(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	emptyCustomerID, err := contactfixture.CreateCustomer(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	firstTagID, err := contactfixture.CreateTag(ctx, tx, marker+"-first")
	if err != nil {
		t.Fatal(err)
	}
	secondTagID, err := contactfixture.CreateTag(ctx, tx, marker+"-second")
	if err != nil {
		t.Fatal(err)
	}
	if err := contactfixture.AttachTag(ctx, tx, customerID, firstTagID, "acceptance:0301"); err != nil {
		t.Fatal(err)
	}
	if err := contactfixture.AttachTag(ctx, tx, customerID, secondTagID, "acceptance:0301"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	before := customerProfileTagsFacts(t, ctx, pool, customerID, firstTagID, secondTagID)
	service := contactapp.NewCustomerDetailService(platformstore.NewUnitOfWork(pool), contactstore.NewCustomerDetailRepository())
	projection, err := service.Get(ctx, contactapp.CustomerDetailInput{ID: contactport.CustomerID(customerID)})
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Tags) != 2 || projection.Tags[0].Name != marker+"-first" || projection.Tags[1].Name != marker+"-second" {
		t.Fatalf("local tag projection=%+v", projection.Tags)
	}
	if projection.Tags[0].ID != firstTagID || projection.Tags[1].ID != secondTagID {
		t.Fatalf("tag projection IDs=%+v", projection.Tags)
	}
	empty, err := service.Get(ctx, contactapp.CustomerDetailInput{ID: contactport.CustomerID(emptyCustomerID)})
	if err != nil || len(empty.Tags) != 0 {
		t.Fatalf("authoritative empty projection=%+v err=%v", empty, err)
	}
	after := customerProfileTagsFacts(t, ctx, pool, customerID, firstTagID, secondTagID)
	if after != before {
		t.Fatalf("customer-profile-tags read wrote facts: before=%+v after=%+v", before, after)
	}
}

type customerProfileTagsProjectionFacts struct {
	Tags, Associations int
	Digest             string
}

func customerProfileTagsFacts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, customerID, firstTagID, secondTagID int64) customerProfileTagsProjectionFacts {
	t.Helper()
	var facts customerProfileTagsProjectionFacts
	if err := pool.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM tags WHERE id IN ($1, $2)),
  (SELECT count(*) FROM customer_tags WHERE customer_id = $3 AND tag_id IN ($1, $2)),
  (SELECT md5(COALESCE(string_agg(t.id::text || ':' || t.name || ':' || ct.tagged_by, ',' ORDER BY t.id), ''))
   FROM tags AS t JOIN customer_tags AS ct ON ct.tag_id = t.id
   WHERE ct.customer_id = $3 AND t.id IN ($1, $2))`, firstTagID, secondTagID, customerID).Scan(&facts.Tags, &facts.Associations, &facts.Digest); err != nil {
		t.Fatal(err)
	}
	return facts
}
