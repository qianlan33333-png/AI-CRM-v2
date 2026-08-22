package coupon_acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	couponapp "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/app"
	couponport "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/port"
	couponstore "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/store"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productstore "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store"
)

var databaseURL = flag.String("database-url", "", "isolated PostgreSQL 16.14 J01 database")

func TestJ01LifecycleProductValidationAndSameStateIdempotency(t *testing.T) {
	pool, ctx := openPool(t)
	product := createProduct(t, ctx, pool, "CNY", 9900)
	service := realService(pool)
	command := validCommand(product.ID, time.Now().UTC())
	actor := command.Actor
	created, err := service.Create(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	command.ID = created.ID
	command.TotalIssueLimit = 200
	command.IdempotencyKey = uniqueKey("j01-update")
	updated, err := service.Update(ctx, command)
	if err != nil || updated.Status != "draft" || updated.TotalIssueLimit != 200 {
		t.Fatalf("update=%#v err=%v", updated, err)
	}
	publishKey := uniqueKey("coupon:publish")
	published, err := service.Publish(ctx, created.ID, actor, publishKey)
	if err != nil || published.Status != "published" {
		t.Fatalf("publish=%#v err=%v", published, err)
	}
	again, err := service.Publish(ctx, created.ID, actor, publishKey)
	if err != nil || again.ID != created.ID {
		t.Fatalf("publish no-op=%#v err=%v", again, err)
	}
	stopKey := uniqueKey("coupon:stop")
	stopped, err := service.Stop(ctx, created.ID, actor, stopKey)
	if err != nil || stopped.Status != "stopped" {
		t.Fatalf("stop=%#v err=%v", stopped, err)
	}
	again, err = service.Stop(ctx, created.ID, actor, stopKey)
	if err != nil || again.Status != "stopped" {
		t.Fatalf("stop no-op=%#v err=%v", again, err)
	}
	var coupons, receipts, events int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM coupons WHERE id=$1::bigint),(SELECT count(*) FROM coupon_operation_receipts WHERE actor_scope=$2 AND state='completed'),(SELECT count(*) FROM event_log WHERE payload->>'coupon_id'=($1::bigint)::text AND payload->>'actor'=$3 AND event_type LIKE 'coupon.%')`, int64(created.ID), fmt.Sprintf("admin:%d", actor), fmt.Sprint(actor)).Scan(&coupons, &receipts, &events); err != nil || coupons != 1 || receipts != 4 || events != 4 {
		t.Fatalf("facts=%d/%d/%d err=%v", coupons, receipts, events, err)
	}
}

func TestJ01PublishFailsClosedForMissingDuplicateExtraTypeCurrencyAndPrice(t *testing.T) {
	pool, ctx := openPool(t)
	service := realService(pool)
	now := time.Now().UTC()
	cny := createProduct(t, ctx, pool, "CNY", 1000)
	usd := createProduct(t, ctx, pool, "USD", 5000)
	cases := []struct {
		name     string
		refs     []string
		discount int64
	}{{"missing", []string{"standard_product:9223372036854775806"}, 1}, {"duplicate", []string{fmt.Sprintf("standard_product:%d", cny.ID), fmt.Sprintf("standard_product:%d", cny.ID)}, 1}, {"extra", []string{fmt.Sprintf("standard_product:%d:extra", cny.ID)}, 1}, {"type", []string{fmt.Sprintf("service_period:%d", cny.ID)}, 1}, {"currency", []string{fmt.Sprintf("standard_product:%d", usd.ID)}, 1}, {"price", []string{fmt.Sprintf("standard_product:%d", cny.ID)}, 1000}}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := validCommand(cny.ID, now)
			cmd.TargetRefs = tc.refs
			cmd.DiscountAmountTotal = tc.discount
			cmd.IdempotencyKey = fmt.Sprintf("j01-invalid-%s-%d", tc.name, i)
			if tc.name == "missing" {
				cmd.Name = fmt.Sprintf("j01-invalid-missing-%d", time.Now().UnixNano())
			}
			created, err := service.Create(ctx, cmd)
			if tc.name == "duplicate" || tc.name == "extra" || tc.name == "type" {
				if !errors.Is(err, couponapp.ErrInvalidTarget) {
					t.Fatalf("create err=%v", err)
				}
				return
			}
			// The product foreign key may reject a missing target before the
			// publish-time product revalidation. Both boundaries must leave no
			// business fact, receipt, or event behind.
			if tc.name == "missing" && errors.Is(err, couponapp.ErrUnavailable) {
				var coupons, receipts, events int
				if e := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM coupons WHERE name=$1),(SELECT count(*) FROM coupon_operation_receipts WHERE actor_scope=$2),(SELECT count(*) FROM event_log WHERE payload->>'actor'=$3 AND event_type='coupon.created')`, cmd.Name, fmt.Sprintf("admin:%d", cmd.Actor), fmt.Sprint(cmd.Actor)).Scan(&coupons, &receipts, &events); e != nil || coupons != 0 || receipts != 0 || events != 0 {
					t.Fatalf("create rollback=%d/%d/%d err=%v", coupons, receipts, events, e)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.Publish(ctx, created.ID, created.UpdatedBy, uniqueKey("coupon:publish-invalid"))
			if !errors.Is(err, couponapp.ErrInvalidTarget) {
				t.Fatalf("publish err=%v", err)
			}
			var status string
			var receipts, events int
			if e := pool.QueryRow(ctx, `SELECT status,(SELECT count(*) FROM coupon_operation_receipts WHERE operation='publish' AND result_snapshot->>'id'=($1::bigint)::text),(SELECT count(*) FROM event_log WHERE event_type='coupon.published' AND payload->>'coupon_id'=($1::bigint)::text) FROM coupons WHERE id=$1::bigint`, int64(created.ID)).Scan(&status, &receipts, &events); e != nil || status != "draft" || receipts != 0 || events != 0 {
				t.Fatalf("rollback=%s/%d/%d err=%v", status, receipts, events, e)
			}
		})
	}
}

func TestJ01ClaimedRulesAndTargetsFreezeWhileQuantityOnlyIncreases(t *testing.T) {
	pool, ctx := openPool(t)
	product := createProduct(t, ctx, pool, "CNY", 9900)
	service := realService(pool)
	cmd := validCommand(product.ID, time.Date(2026, 8, 15, 12, 0, 0, 123, time.UTC))
	cmd.IdempotencyKey = uniqueKey("j01-freeze-create")
	created, err := service.Create(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE coupons SET issued_count=1,first_claim_at=now() WHERE id=$1`, created.ID); err != nil {
		t.Fatal(err)
	}
	cmd.ID = created.ID
	cmd.TotalIssueLimit++
	cmd.IdempotencyKey = uniqueKey("j01-freeze-increase")
	if _, err = service.Update(ctx, cmd); err != nil {
		t.Fatalf("quantity increase err=%v", err)
	}
	cmd.DiscountAmountTotal++
	cmd.IdempotencyKey = uniqueKey("j01-freeze-rule-change")
	if _, err = service.Update(ctx, cmd); !errors.Is(err, couponapp.ErrRulesFrozen) {
		t.Fatalf("rule change err=%v", err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM coupon_targets WHERE coupon_id=$1`, created.ID); err == nil {
		t.Fatal("claimed target delete succeeded")
	}
}

type failingStore struct {
	couponapp.Store
	reserve, afterWrite, complete bool
}

func (s failingStore) Reserve(ctx context.Context, x couponapp.Reservation) (couponapp.Receipt, bool, error) {
	if s.reserve {
		return couponapp.Receipt{}, false, errors.New("reserve")
	}
	return s.Store.Reserve(ctx, x)
}
func (s failingStore) Create(ctx context.Context, c couponport.UpsertCommand, ids []int64, now time.Time) (couponport.Coupon, error) {
	v, e := s.Store.Create(ctx, c, ids, now)
	if e == nil && s.afterWrite {
		return couponport.Coupon{}, errors.New("write")
	}
	return v, e
}
func (s failingStore) Complete(ctx context.Context, id int64, b json.RawMessage, now time.Time) (couponapp.Receipt, error) {
	if s.complete {
		return couponapp.Receipt{}, errors.New("complete")
	}
	return s.Store.Complete(ctx, id, b, now)
}

type failingEvents struct{}

func (failingEvents) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	return 0, errors.New("event")
}
func TestJ01FourFailurePointsRollbackBusinessReceiptAndEvent(t *testing.T) {
	pool, ctx := openPool(t)
	product := createProduct(t, ctx, pool, "CNY", 9900)
	base := couponstore.NewRepository()
	for i, tc := range []struct {
		name   string
		store  couponapp.Store
		events eventport.Appender
	}{{"reserve", failingStore{Store: base, reserve: true}, eventstore.NewAppender()}, {"business", failingStore{Store: base, afterWrite: true}, eventstore.NewAppender()}, {"event", failingStore{Store: base}, failingEvents{}}, {"complete", failingStore{Store: base, complete: true}, eventstore.NewAppender()}} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := validCommand(product.ID, time.Now().UTC())
			cmd.Name = fmt.Sprintf("rollback-%d-%d", i, time.Now().UnixNano())
			cmd.IdempotencyKey = "j01-failure-" + tc.name + fmt.Sprint(time.Now().UnixNano())
			_, err := couponapp.NewService(platformstore.NewUnitOfWork(pool), tc.store, productstore.NewCatalogRepository(), tc.events).Create(ctx, cmd)
			if err == nil {
				t.Fatal("mutation succeeded")
			}
			var coupons, receipts, events int
			actorScope := fmt.Sprintf("admin:%d", cmd.Actor)
			actor := fmt.Sprint(cmd.Actor)
			if e := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM coupons WHERE name=$1),(SELECT count(*) FROM coupon_operation_receipts WHERE actor_scope=$2 AND state='in_progress'),(SELECT count(*) FROM event_log WHERE event_type='coupon.created' AND payload->>'actor'=$3 AND payload->>'coupon_id' IN (SELECT id::text FROM coupons WHERE name=$1))`, cmd.Name, actorScope, actor).Scan(&coupons, &receipts, &events); e != nil || coupons != 0 || receipts != 0 || events != 0 {
				t.Fatalf("facts=%d/%d/%d err=%v", coupons, receipts, events, e)
			}
		})
	}
}

func TestJ01ConcurrentPublishAndStopConverge(t *testing.T) {
	pool, ctx := openPool(t)
	product := createProduct(t, ctx, pool, "CNY", 9900)
	service := realService(pool)
	cmd := validCommand(product.ID, time.Now().UTC())
	cmd.IdempotencyKey = uniqueKey("j01-concurrent-create")
	created, err := service.Create(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	run := func(name string, fn func() (couponport.Coupon, error)) {
		t.Helper()
		const n = 8
		start := make(chan struct{})
		errs := make(chan error, n)
		var wg sync.WaitGroup
		for range n {
			wg.Add(1)
			go func() { defer wg.Done(); <-start; _, e := fn(); errs <- e }()
		}
		close(start)
		wg.Wait()
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatalf("%s err=%v", name, e)
			}
		}
	}
	publishKey := uniqueKey("coupon:publish-concurrent")
	run("publish", func() (couponport.Coupon, error) {
		return service.Publish(ctx, created.ID, cmd.Actor, publishKey)
	})
	stopKey := uniqueKey("coupon:stop-concurrent")
	run("stop", func() (couponport.Coupon, error) {
		return service.Stop(ctx, created.ID, cmd.Actor, stopKey)
	})
	var published, stopped int
	if err = pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE event_type='coupon.published'),count(*) FILTER(WHERE event_type='coupon.stopped') FROM event_log WHERE payload->>'coupon_id'=($1::bigint)::text AND payload->>'actor'=$2`, int64(created.ID), fmt.Sprint(cmd.Actor)).Scan(&published, &stopped); err != nil || published != 1 || stopped != 1 {
		t.Fatalf("events=%d/%d err=%v", published, stopped, err)
	}
}

func TestJ01S200KPlansUseIndexesAndCounter(t *testing.T) {
	pool, ctx := openPool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO coupons(name,discount_amount_total,total_issue_limit,per_user_issue_limit,claim_starts_at,claim_ends_at,validity_mode,use_starts_at,use_ends_at,instructions,created_by,updated_by,created_at,updated_at) SELECT '性能券'||g,1,100,1,now()-interval '1 day',now()+interval '1 day','fixed_range',now(),now()+interval '30 day','',99,99,now(),now() FROM generate_series(1,200000)g`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ANALYZE coupons`); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{`EXPLAIN (FORMAT JSON,COSTS OFF) SELECT id FROM coupons WHERE status='draft' ORDER BY id LIMIT 100`, `EXPLAIN (FORMAT JSON,COSTS OFF) SELECT id FROM coupons WHERE id=100000`, `EXPLAIN (FORMAT JSON,COSTS OFF) SELECT total_coupons FROM coupon_catalog_counters WHERE singleton=TRUE`} {
		plan := explain(t, ctx, tx, query)
		if strings.Contains(plan, `"Node Type": "Seq Scan"`) && strings.Contains(plan, `"Relation Name": "coupons"`) {
			t.Fatalf("illegal plan %s", plan)
		}
	}
}

func validCommand(productID productport.ID, now time.Time) couponport.UpsertCommand {
	useStart, useEnd := now.Add(time.Hour), now.Add(31*24*time.Hour)
	return couponport.UpsertCommand{Coupon: couponport.Coupon{Name: "J01优惠券", DiscountAmountTotal: 100, TotalIssueLimit: 100, PerUserIssueLimit: 1, ClaimStartsAt: now.Add(time.Hour), ClaimEndsAt: now.Add(24 * time.Hour), ValidityMode: couponport.ValidityFixedRange, UseStartsAt: &useStart, UseEndsAt: &useEnd, Instructions: "规则", TargetRefs: []string{fmt.Sprintf("standard_product:%d", productID)}}, Actor: time.Now().UnixNano(), IdempotencyKey: uniqueKey("j01-create-technical-receipt")}
}

func uniqueKey(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
func createProduct(t *testing.T, ctx context.Context, pool *pgxpool.Pool, currency string, price int64) productport.Product {
	t.Helper()
	code := fmt.Sprintf("j01-product-%d", time.Now().UnixNano())
	p, e := productapp.NewService(platformstore.NewUnitOfWork(pool), productstore.NewCatalogRepository(), eventstore.NewAppender()).Create(ctx, productport.CreateCommand{ProductCode: code, Name: "J01商品", Currency: currency, PriceMinor: price, StockQuantity: 0, Images: []string{}, LegacyAdminProjection: productapp.DefaultLegacyAdminProjection(), Actor: 91, IdempotencyKey: "j01-product-key-" + code})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func realService(pool *pgxpool.Pool) *couponapp.Service {
	return couponapp.NewService(platformstore.NewUnitOfWork(pool), couponstore.NewRepository(), productstore.NewCatalogRepository(), eventstore.NewAppender())
}
func openPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *databaseURL == "" {
		t.Skip("database-url missing")
	}
	if e := acceptancefixtures.ValidateDatabaseURL(*databaseURL); e != nil {
		if j01Err := acceptancefixtures.ValidateDatabaseURLForDatabase(*databaseURL, acceptancefixtures.J01CouponDatabaseName); j01Err != nil {
			t.Fatal(e)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	pool, e := pgxpool.New(ctx, *databaseURL)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	var version string
	if e = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); e != nil || version != "160014" {
		t.Fatalf("postgres=%s err=%v", version, e)
	}
	return pool, ctx
}
func explain(t *testing.T, ctx context.Context, tx pgx.Tx, query string) string {
	t.Helper()
	rows, e := tx.Query(ctx, query)
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var line string
		if e = rows.Scan(&line); e != nil {
			t.Fatal(e)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
