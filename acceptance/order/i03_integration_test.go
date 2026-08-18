package order_acceptance

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	orderstore "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

var i03DatabaseURL = flag.String("database-url", "", "isolated PostgreSQL 16.14 I03 database")

type snapshotCustomers struct{}

func (snapshotCustomers) ReadCustomer(context.Context, contactport.CustomerID) (contactport.CustomerProjection, error) {
	return contactport.CustomerProjection{}, contactport.ErrCustomerReadNotFound
}

type snapshotProducts struct{}

func (snapshotProducts) ReadProduct(context.Context, productport.ID) (productport.Product, error) {
	return productport.Product{}, productport.ErrProductReadNotFound
}

func TestI03OrderStoreNormalBoundaryAndExactTotal(t *testing.T) {
	pool, ctx := openOrderPool(t)
	prefix := fmt.Sprintf("i03-%d", time.Now().UnixNano())
	now := time.Now().UTC().Truncate(time.Microsecond)
	for index, provider := range []string{"wechat", "alipay", "wechat_shop"} {
		_, err := pool.Exec(ctx, `INSERT INTO order_list_projections
      (provider,provider_label,merchant_order_no,platform_transaction_no,payer_name_snapshot,mobile_snapshot,identity_kind,identity_value,product_code,product_name_snapshot,amount_minor,currency,status,status_label,detail_url,created_at,updated_at)
      VALUES ($1,$2,$3,$4,$5,$6,'external_userid',$7,$8,$9,$10,'CNY','paid','已支付',$11,$12,$12)`,
			provider, provider, fmt.Sprintf("%s-M-%d", prefix, index), fmt.Sprintf("%s-T-%d", prefix, index), "快照客户", fmt.Sprintf("1380000000%d", index), fmt.Sprintf("wmid-%d", index), fmt.Sprintf("%s-SKU-%d", prefix, index), "快照商品", int64(1990+index), fmt.Sprintf("/api/admin/orders/%d", index+1), now.Add(time.Duration(index)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
	}
	service := orderapp.NewService(platformstore.NewUnitOfWork(pool), orderstore.NewRepository(), snapshotCustomers{}, snapshotProducts{})
	page, err := service.List(ctx, orderport.Filter{Provider: "alipay", OrderNo: prefix, Status: "paid", Limit: 1})
	if err != nil || len(page.Items) != 1 || page.Total != 1 || page.HasMore || page.Items[0].AmountYuan != "19.91" || page.Items[0].ExternalUserID != "wmid-1" {
		t.Fatalf("page=%+v error=%v", page, err)
	}
	all, err := service.List(ctx, orderport.Filter{OrderNo: prefix, Limit: 2})
	if err != nil || len(all.Items) != 2 || all.Total != 3 || !all.HasMore {
		t.Fatalf("all=%+v error=%v", all, err)
	}
	var counter, actual int64
	if err = pool.QueryRow(ctx, `SELECT (SELECT total_orders FROM order_list_projection_counters WHERE singleton=TRUE),(SELECT count(*) FROM order_list_projections)`).Scan(&counter, &actual); err != nil || counter != actual {
		t.Fatalf("counter/actual/error=%d/%d/%v", counter, actual, err)
	}
}

func TestI03S200KPlansUseOrderIndexes(t *testing.T) {
	pool, ctx := openOrderPool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO order_list_projections
      (provider,provider_label,merchant_order_no,platform_transaction_no,payer_name_snapshot,mobile_snapshot,product_code,product_name_snapshot,amount_minor,currency,status,status_label,detail_url,created_at,updated_at)
      SELECT CASE WHEN g%3=0 THEN 'wechat' WHEN g%3=1 THEN 'alipay' ELSE 'wechat_shop' END,'provider','i03-perf-order-'||g,'tx-'||g,'payer','139'||lpad(g::text,8,'0'),'i03-sku-'||g,'product',g,'CNY',CASE WHEN g%2=0 THEN 'paid' ELSE 'pending' END,'status','/api/admin/orders/'||g,now()-g*interval '1 microsecond',now()
        FROM generate_series(1,200000) AS g`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ANALYZE order_list_projections`); err != nil {
		t.Fatal(err)
	}
	plans := map[string]string{
		"list":          `EXPLAIN (FORMAT JSON, COSTS OFF) SELECT id FROM order_list_projections WHERE provider='wechat' AND status='paid' ORDER BY created_at DESC,id DESC LIMIT 100`,
		"merchant":      `EXPLAIN (FORMAT JSON, COSTS OFF) SELECT id FROM order_list_projections WHERE merchant_order_no ILIKE '%199999%' ORDER BY created_at DESC,id DESC LIMIT 100`,
		"product count": `EXPLAIN (FORMAT JSON, COSTS OFF) SELECT count(*) FROM order_list_projections WHERE product_code='i03-sku-199999'`,
		"exact total":   `EXPLAIN (FORMAT JSON, COSTS OFF) SELECT total_orders FROM order_list_projection_counters WHERE singleton=TRUE`,
	}
	for name, query := range plans {
		plan := explainOrder(t, ctx, tx, query)
		if strings.Contains(plan, `"Node Type": "Seq Scan"`) && strings.Contains(plan, `"Relation Name": "order_list_projections"`) {
			t.Fatalf("illegal %s S plan:\n%s", name, plan)
		}
	}
}

func TestI03StorageCatalogAndNoCrossDomainFK(t *testing.T) {
	pool, ctx := openOrderPool(t)
	var constraints, invalidConstraints, indexes, invalidIndexes, crossDomainFK int
	err := pool.QueryRow(ctx, `SELECT
      (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('order_list_projections'::regclass,'order_list_projection_counters'::regclass)),
      (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('order_list_projections'::regclass,'order_list_projection_counters'::regclass) AND NOT convalidated),
      (SELECT count(*) FROM pg_index WHERE indrelid IN ('order_list_projections'::regclass,'order_list_projection_counters'::regclass)),
      (SELECT count(*) FROM pg_index WHERE indrelid IN ('order_list_projections'::regclass,'order_list_projection_counters'::regclass) AND (NOT indisvalid OR NOT indisready OR NOT indislive)),
	      (SELECT count(*) FROM pg_constraint WHERE conrelid='order_list_projections'::regclass AND contype='f')`).Scan(&constraints, &invalidConstraints, &indexes, &invalidIndexes, &crossDomainFK)
	if err != nil || constraints < 18 || invalidConstraints != 0 || indexes < 9 || invalidIndexes != 0 || crossDomainFK != 0 {
		t.Fatalf("catalog constraints/invalid/indexes/invalid/fk/error=%d/%d/%d/%d/%d/%v", constraints, invalidConstraints, indexes, invalidIndexes, crossDomainFK, err)
	}
}

func openOrderPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *i03DatabaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*i03DatabaseURL); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, *i03DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q error=%v", version, err)
	}
	return pool, ctx
}

func explainOrder(t *testing.T, ctx context.Context, tx pgx.Tx, query string) string {
	t.Helper()
	rows, err := tx.Query(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var line string
		if err = rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return strings.Join(lines, "\n")
}
