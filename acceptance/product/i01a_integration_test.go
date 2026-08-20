package product_acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	orderstore "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productstore "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store"
)

var i01aDatabaseURL = flag.String("database-url", "", "isolated PostgreSQL 16.14 I01A database")

func TestI01AMigrationHistoryFixture(t *testing.T) {
	pool, ctx := openPool(t)
	marker := uniqueCode("migration-history")
	err := platformstore.NewUnitOfWork(pool).Within(ctx, func(tx context.Context) error {
		_, appendErr := eventstore.NewAppender().Append(tx, eventport.Event{
			Type: "i01a.product.migration_fixture", Payload: json.RawMessage(fmt.Sprintf(`{"marker":%q}`, marker)),
			OccurredAt: time.Now().UTC(), IdempotencyKey: marker,
		})
		return appendErr
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestI01ACreateReplayProjectionAndEventShareOneUoW(t *testing.T) {
	pool, ctx := openPool(t)
	service := realService(pool)
	code := uniqueCode("normal")
	replayRef := "i01a-normal-idempotency-" + code
	command := productport.CreateCommand{
		ProductCode: code, Name: "普通商品", Description: "I01A", PriceMinor: 9900, Currency: "cny",
		StockQuantity: 0, Images: []string{"https://img.example/native.png"}, Actor: 81, IdempotencyKey: replayRef,
		LegacyAdminProjection: json.RawMessage(`{"schema_version":1,"status":"active","enabled":true,"slices":[{"image_id":71}],"wecom_tagging":{"tag_ids":[1,2]}}`),
	}
	created, err := service.Create(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	command.LegacyAdminProjection = json.RawMessage(`{"wecom_tagging":{"tag_ids":[1,2]},"slices":[{"image_id":71}],"enabled":true,"status":"active","schema_version":1}`)
	replayed, err := service.Create(ctx, command)
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("canonical replay product/error=%+v/%v, want id=%d", replayed, err, created.ID)
	}
	page, err := service.ListLegacy(ctx, 1, 0)
	if err != nil || page.Total < 1 || len(page.Items) != 1 {
		t.Fatalf("legacy page=%+v error=%v", page, err)
	}
	loaded, err := service.Get(ctx, created.ID)
	if err != nil || loaded.ProductCode != code || loaded.StockQuantity != 0 || len(loaded.Images) != 1 || !jsonEquivalent(loaded.LegacyAdminProjection, created.LegacyAdminProjection) {
		t.Fatalf("loaded=%+v error=%v", loaded, err)
	}
	eventDigest := sha256.Sum256([]byte("admin:81\x00" + replayRef))
	eventKey := "product.create:" + hex.EncodeToString(eventDigest[:])
	var products, receipts, events int
	var receiptState, eventType, actor, storedEventKey string
	err = pool.QueryRow(ctx, `SELECT
      (SELECT count(*) FROM products WHERE product_code=$1),
      (SELECT count(*) FROM product_operation_receipts WHERE actor_scope='admin:81'),
      (SELECT state FROM product_operation_receipts WHERE actor_scope='admin:81' ORDER BY id DESC LIMIT 1),
	  (SELECT count(*) FROM event_log WHERE idempotency_key=$2),
	  (SELECT event_type FROM event_log WHERE idempotency_key=$2),
	  (SELECT payload->>'actor' FROM event_log WHERE idempotency_key=$2),
	  (SELECT idempotency_key FROM event_log WHERE idempotency_key=$2)`,
		code, eventKey).Scan(&products, &receipts, &receiptState, &events, &eventType, &actor, &storedEventKey)
	if err != nil || products != 1 || receipts != 1 || receiptState != "completed" || events != 1 || eventType != eventport.EvProductCreated || actor != "81" || storedEventKey != eventKey {
		t.Fatalf("facts products/receipts/state/events/type/actor/key/error=%d/%d/%s/%d/%s/%s/%s/%v", products, receipts, receiptState, events, eventType, actor, storedEventKey, err)
	}
	command.LegacyAdminProjection = json.RawMessage(`{"schema_version":1,"status":"active","enabled":true,"slices":[{"image_id":72}]}`)
	if _, err = service.Create(ctx, command); !errors.Is(err, productapp.ErrConflict) {
		t.Fatalf("changed projection replay error=%v, want conflict", err)
	}
}

func TestI01AEventConflictRollsBackProductAndReceipt(t *testing.T) {
	pool, ctx := openPool(t)
	service := realService(pool)
	code := uniqueCode("rollback")
	key := "i01a-rollback-idempotency-" + code
	actor := int64(82)
	digest := sha256.Sum256([]byte(fmt.Sprintf("admin:%d\x00%s", actor, key)))
	eventKey := "product.create:" + hex.EncodeToString(digest[:])
	err := platformstore.NewUnitOfWork(pool).Within(ctx, func(tx context.Context) error {
		_, appendErr := eventstore.NewAppender().Append(tx, eventport.Event{
			Type: "i01a.conflict.fixture", Payload: json.RawMessage(`{"fixture":true}`),
			OccurredAt: time.Now().UTC(), IdempotencyKey: eventKey,
		})
		return appendErr
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Create(ctx, productport.CreateCommand{
		ProductCode: code, Name: "必须回滚", PriceMinor: 1, Currency: "CNY", StockQuantity: 0, Images: []string{},
		LegacyAdminProjection: productapp.DefaultLegacyAdminProjection(), Actor: actor, IdempotencyKey: key,
	})
	if !errors.Is(err, productapp.ErrUnavailable) {
		t.Fatalf("Create() error=%v, want unavailable classification for event conflict", err)
	}
	var products, receipts int
	if err = pool.QueryRow(ctx, `SELECT
      (SELECT count(*) FROM products WHERE product_code=$1),
      (SELECT count(*) FROM product_operation_receipts WHERE actor_scope=$2)`, code, fmt.Sprintf("admin:%d", actor)).Scan(&products, &receipts); err != nil {
		t.Fatal(err)
	}
	if products != 0 || receipts != 0 {
		t.Fatalf("rolled-back products/receipts=%d/%d", products, receipts)
	}
}

func TestI01BProductCASAndLocalEntitlementLifecycleUseOneUoW(t *testing.T) {
	pool, ctx := openPool(t)
	service := realService(pool)
	entitlements := realEntitlementService(pool)
	code := uniqueCode("local-entitlement")
	actor := int64(9301)
	created, err := service.Create(ctx, productport.CreateCommand{
		ProductCode: code, Name: "本地权益产品", Description: "I01B", PriceMinor: 1200, Currency: "CNY", StockQuantity: 4,
		Images: []string{}, LegacyAdminProjection: productapp.DefaultLegacyAdminProjection(), Actor: actor,
		IdempotencyKey: "i01b-create-" + code,
	})
	if err != nil || created.Version != 1 {
		t.Fatalf("create product=%+v err=%v", created, err)
	}

	var customerID, orderID int64
	if err = pool.QueryRow(ctx, `INSERT INTO customers (name) VALUES ('I01B 本地客户') RETURNING id`).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO order_list_projections (
		provider,provider_label,merchant_order_no,customer_id,product_id,product_code,product_name_snapshot,
		amount_minor,currency,status,status_label,detail_url,created_at,updated_at
	) VALUES ('wechat','微信支付',$1,$2,$3,$4,'本地权益产品',1200,'CNY','paid','已支付',$5,now(),now()) RETURNING id`,
		"i01b-order-"+code, customerID, int64(created.ID), code, "/orders/i01b-"+code).Scan(&orderID); err != nil {
		t.Fatal(err)
	}

	updateKey := "i01b-update-" + code
	updated, err := service.Update(ctx, productport.UpdateCommand{
		ID: created.ID, ExpectedVersion: created.Version, Name: "已更新本地权益产品", Description: "I01B CAS", PriceMinor: 1500,
		Currency: "CNY", StockQuantity: 7, Actor: actor, IdempotencyKey: updateKey,
	})
	if err != nil || updated.Version != created.Version+1 || updated.ProductCode != created.ProductCode || updated.CreatedBy != created.CreatedBy || !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("update product=%+v err=%v", updated, err)
	}
	replayedUpdate, err := service.Update(ctx, productport.UpdateCommand{
		ID: created.ID, ExpectedVersion: created.Version, Name: "已更新本地权益产品", Description: "I01B CAS", PriceMinor: 1500,
		Currency: "CNY", StockQuantity: 7, Actor: actor, IdempotencyKey: updateKey,
	})
	if err != nil || replayedUpdate.ID != updated.ID || replayedUpdate.Version != updated.Version {
		t.Fatalf("update replay=%+v err=%v", replayedUpdate, err)
	}
	if _, err = service.Update(ctx, productport.UpdateCommand{
		ID: created.ID, ExpectedVersion: created.Version, Name: "stale", Description: "I01B", PriceMinor: 1,
		Currency: "CNY", StockQuantity: 1, Actor: actor, IdempotencyKey: "i01b-stale-" + code,
	}); !errors.Is(err, productapp.ErrConflict) {
		t.Fatalf("stale product CAS error=%v", err)
	}

	grantKey := "i01b-grant-" + code
	grantCommand := productport.GrantLocalEntitlementCommand{ProductID: created.ID, OrderID: orderID, Actor: actor, IdempotencyKey: grantKey}
	granted, err := entitlements.Grant(ctx, grantCommand)
	if err != nil || granted.ProductID != created.ID || granted.OrderID != orderID || granted.CustomerID != customerID || granted.State != "active" || granted.Version != 1 {
		t.Fatalf("grant=%+v err=%v", granted, err)
	}
	replayedGrant, err := entitlements.Grant(ctx, grantCommand)
	if err != nil || replayedGrant.ID != granted.ID || replayedGrant.Version != granted.Version {
		t.Fatalf("grant replay=%+v err=%v", replayedGrant, err)
	}
	listed, err := entitlements.List(ctx, created.ID, 10)
	if err != nil || len(listed) != 1 || listed[0].ID != granted.ID {
		t.Fatalf("list=%+v err=%v", listed, err)
	}
	loaded, err := entitlements.Get(ctx, granted.ID)
	if err != nil || !sameLocalEntitlement(loaded, granted) {
		t.Fatalf("get=%+v err=%v want=%+v", loaded, err, granted)
	}

	revokeKey := "i01b-revoke-" + code
	revokeCommand := productport.RevokeLocalEntitlementCommand{ID: granted.ID, ExpectedVersion: granted.Version, Actor: actor, IdempotencyKey: revokeKey}
	revoked, err := entitlements.Revoke(ctx, revokeCommand)
	if err != nil || revoked.State != "revoked" || revoked.Version != granted.Version+1 || revoked.RevokedAt == nil || revoked.RevokedAt.Before(revoked.GrantedAt) {
		t.Fatalf("revoke=%+v err=%v", revoked, err)
	}
	replayedRevoke, err := entitlements.Revoke(ctx, revokeCommand)
	if err != nil || replayedRevoke.ID != revoked.ID || replayedRevoke.Version != revoked.Version || replayedRevoke.State != "revoked" {
		t.Fatalf("revoke replay=%+v err=%v", replayedRevoke, err)
	}
	readback, err := entitlements.Get(ctx, granted.ID)
	if err != nil || !sameLocalEntitlement(readback, revoked) {
		t.Fatalf("revoke readback=%+v err=%v want=%+v", readback, err, revoked)
	}

	updateDigest := sha256.Sum256([]byte(updateKey))
	grantDigest := sha256.Sum256([]byte(grantKey))
	revokeDigest := sha256.Sum256([]byte(revokeKey))
	actorScope := fmt.Sprintf("admin:%d", actor)
	updateEventDigest := sha256.Sum256([]byte(actorScope + "\x00" + updateKey))
	grantEventDigest := sha256.Sum256([]byte(actorScope + "\x00" + grantKey))
	revokeEventDigest := sha256.Sum256([]byte(actorScope + "\x00" + revokeKey))
	var updateReceipts, grantReceipts, revokeReceipts, updateEvents, grantEvents, revokeEvents int
	var grantState, revokeState string
	err = pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM product_operation_receipts WHERE operation='update' AND actor_scope=$1 AND key_digest=$2 AND state='completed'),
		(SELECT count(*) FROM entitlement_operation_receipts WHERE operation='grant' AND actor_scope=$1 AND key_digest=$3 AND state='completed'),
		(SELECT count(*) FROM entitlement_operation_receipts WHERE operation='revoke' AND actor_scope=$1 AND key_digest=$4 AND state='completed'),
		(SELECT count(*) FROM event_log WHERE event_type=$5 AND idempotency_key=$6),
		(SELECT count(*) FROM event_log WHERE event_type=$7 AND idempotency_key=$8 AND customer_id=$9),
		(SELECT count(*) FROM event_log WHERE event_type=$10 AND idempotency_key=$11 AND customer_id=$9),
		(SELECT state FROM entitlement_operation_receipts WHERE operation='grant' AND actor_scope=$1 AND key_digest=$3),
		(SELECT state FROM entitlement_operation_receipts WHERE operation='revoke' AND actor_scope=$1 AND key_digest=$4)`,
		actorScope, updateDigest[:], grantDigest[:], revokeDigest[:], eventport.EvProductUpdated,
		"product.update:"+hex.EncodeToString(updateEventDigest[:]), eventport.EvProductEntitlementGranted,
		"product.entitlement.grant:"+hex.EncodeToString(grantEventDigest[:]), customerID,
		eventport.EvProductEntitlementRevoked, "product.entitlement.revoke:"+hex.EncodeToString(revokeEventDigest[:]),
	).Scan(&updateReceipts, &grantReceipts, &revokeReceipts, &updateEvents, &grantEvents, &revokeEvents, &grantState, &revokeState)
	if err != nil || updateReceipts != 1 || grantReceipts != 1 || revokeReceipts != 1 || updateEvents != 1 || grantEvents != 1 || revokeEvents != 1 || grantState != "completed" || revokeState != "completed" {
		t.Fatalf("uow facts update/grant/revoke receipts=%d/%d/%d events=%d/%d/%d states=%s/%s err=%v", updateReceipts, grantReceipts, revokeReceipts, updateEvents, grantEvents, revokeEvents, grantState, revokeState, err)
	}
}

type failingProductStore struct {
	productapp.Store
	failReserve, failAfterCreate, failComplete bool
}

func (store failingProductStore) Reserve(ctx context.Context, reservation productapp.Reservation) (productapp.Receipt, bool, error) {
	if store.failReserve {
		return productapp.Receipt{}, false, errors.New("injected receipt reservation failure")
	}
	return store.Store.Reserve(ctx, reservation)
}

func (store failingProductStore) Create(ctx context.Context, command productport.CreateCommand, now time.Time) (productport.Product, error) {
	product, err := store.Store.Create(ctx, command, now)
	if err == nil && store.failAfterCreate {
		return productport.Product{}, errors.New("injected business write failure")
	}
	return product, err
}

func (store failingProductStore) Complete(ctx context.Context, id int64, snapshot json.RawMessage, now time.Time) (productapp.Receipt, error) {
	if store.failComplete {
		return productapp.Receipt{}, errors.New("injected receipt completion failure")
	}
	return store.Store.Complete(ctx, id, snapshot, now)
}

type failingProductEvents struct{ err error }

func (events failingProductEvents) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	return 0, events.err
}

func TestI01AFourFailurePointsRollbackAllProductFacts(t *testing.T) {
	pool, ctx := openPool(t)
	baseStore := productstore.NewCatalogRepository()
	for index, testCase := range []struct {
		name   string
		store  productapp.Store
		events eventport.Appender
	}{
		{"receipt reservation", failingProductStore{Store: baseStore, failReserve: true}, eventstore.NewAppender()},
		{"business write", failingProductStore{Store: baseStore, failAfterCreate: true}, eventstore.NewAppender()},
		{"event append", failingProductStore{Store: baseStore}, failingProductEvents{err: errors.New("injected event append failure")}},
		{"receipt completion", failingProductStore{Store: baseStore, failComplete: true}, eventstore.NewAppender()},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			code := uniqueCode(fmt.Sprintf("failure-%d", index))
			actor := time.Now().UnixNano()
			replayRef := "i01a-failure-idempotency-" + code
			service := productapp.NewService(platformstore.NewUnitOfWork(pool), testCase.store, testCase.events)
			_, err := service.Create(ctx, productport.CreateCommand{
				ProductCode: code, Name: "必须回滚", PriceMinor: 1, Currency: "CNY", StockQuantity: 0,
				Images: []string{}, LegacyAdminProjection: productapp.DefaultLegacyAdminProjection(),
				Actor: actor, IdempotencyKey: replayRef,
			})
			if err == nil {
				t.Fatal("Create() unexpectedly succeeded")
			}
			assertProductFactCounts(t, pool, ctx, code, actor, replayRef, 0, 0, 0)
		})
	}
}

func TestI01AConcurrentReplayAndProductCodeConflict(t *testing.T) {
	pool, ctx := openPool(t)
	service := realService(pool)
	code := uniqueCode("concurrent")
	actor := time.Now().UnixNano()
	key := "i01a-concurrent-idempotency-" + code
	command := productport.CreateCommand{
		ProductCode: code, Name: "并发商品", PriceMinor: 8800, Currency: "CNY", StockQuantity: 0,
		Images: []string{"https://img.example/concurrent.png"}, Actor: actor, IdempotencyKey: key,
		LegacyAdminProjection: json.RawMessage(`{"schema_version":1,"status":"active","enabled":true,"completion_target":null,"slices":[{"image_id":1},{"image_id":2}]}`),
	}
	const callers = 8
	start := make(chan struct{})
	results := make(chan productport.Product, callers)
	errorsFound := make(chan error, callers)
	var group sync.WaitGroup
	for range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			product, err := service.Create(ctx, command)
			if err != nil {
				errorsFound <- err
				return
			}
			results <- product
		}()
	}
	close(start)
	group.Wait()
	close(results)
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent Create() error=%v", err)
	}
	var productID productport.ID
	var resultCount int
	for product := range results {
		resultCount++
		if productID == 0 {
			productID = product.ID
		}
		if product.ID != productID {
			t.Fatalf("concurrent product id=%d want=%d", product.ID, productID)
		}
	}
	if resultCount != callers || productID == 0 {
		t.Fatalf("concurrent results=%d product_id=%d", resultCount, productID)
	}
	assertProductFactCounts(t, pool, ctx, code, actor, key, 1, 1, 1)

	changed := command
	changed.LegacyAdminProjection = json.RawMessage(`{"schema_version":1,"status":"active","enabled":true,"completion_target":{},"slices":[{"image_id":1},{"image_id":2}]}`)
	if _, err := service.Create(ctx, changed); !errors.Is(err, productapp.ErrConflict) {
		t.Fatalf("null-to-object replay error=%v, want conflict", err)
	}
	changed = command
	changed.LegacyAdminProjection = json.RawMessage(`{"schema_version":1,"status":"active","enabled":true,"completion_target":null,"slices":[{"image_id":2},{"image_id":1}]}`)
	if _, err := service.Create(ctx, changed); !errors.Is(err, productapp.ErrConflict) {
		t.Fatalf("array-order replay error=%v, want conflict", err)
	}

	duplicateKey := key + "-different"
	duplicate := command
	duplicate.IdempotencyKey = duplicateKey
	if _, err := service.Create(ctx, duplicate); !errors.Is(err, productapp.ErrConflict) {
		t.Fatalf("duplicate product_code error=%v, want conflict", err)
	}
	assertProductFactCounts(t, pool, ctx, code, actor, key, 1, 1, 1)
	assertProductFactCounts(t, pool, ctx, code, actor, duplicateKey, 1, 0, 0)
	var counter int64
	if err := pool.QueryRow(ctx, `SELECT total_products FROM product_catalog_counters WHERE singleton=TRUE`).Scan(&counter); err != nil || counter < 1 {
		t.Fatalf("catalog counter=%d error=%v", counter, err)
	}
}

func TestI01AS200KPlansUseProductIndexes(t *testing.T) {
	pool, ctx := openPool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `INSERT INTO products
      (product_code,name,description,price_minor,currency,stock_quantity,created_by,created_at,updated_at,legacy_admin_projection)
      SELECT 'i01a-perf-'||g,'性能商品','',g,'CNY',0,99,now(),now(),'{"schema_version":1}'::jsonb
        FROM generate_series(1,200000) AS g`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ANALYZE products`); err != nil {
		t.Fatal(err)
	}
	listPlan := explain(t, ctx, tx, `EXPLAIN (FORMAT JSON, COSTS OFF)
      SELECT id,product_code FROM products ORDER BY id LIMIT 100 OFFSET 100000`)
	if strings.Contains(listPlan, `"Node Type": "Seq Scan"`) || !strings.Contains(listPlan, `"Index Name": "products_pkey"`) {
		t.Fatalf("illegal list S plan:\n%s", listPlan)
	}
	getPlan := explain(t, ctx, tx, `EXPLAIN (FORMAT JSON, COSTS OFF) SELECT id,product_code FROM products WHERE id=100000`)
	if strings.Contains(getPlan, `"Node Type": "Seq Scan"`) || !strings.Contains(getPlan, `"Index Name": "products_pkey"`) {
		t.Fatalf("illegal get S plan:\n%s", getPlan)
	}
	countPlan := explain(t, ctx, tx, `EXPLAIN (FORMAT JSON, COSTS OFF) SELECT total_products FROM product_catalog_counters WHERE singleton=TRUE`)
	if strings.Contains(countPlan, `"Relation Name": "products"`) {
		t.Fatalf("illegal exact-total S plan:\n%s", countPlan)
	}
}

func TestI01AStorageCatalogAndSingleInstanceShape(t *testing.T) {
	pool, ctx := openPool(t)
	var waterline, constraints, invalidConstraints, indexes, invalidIndexes, eventForeignKeys int
	err := pool.QueryRow(ctx, `SELECT
      (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
	  (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('products'::regclass,'product_images'::regclass,'product_catalog_counters'::regclass,'product_operation_receipts'::regclass)),
	  (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('products'::regclass,'product_images'::regclass,'product_catalog_counters'::regclass,'product_operation_receipts'::regclass) AND NOT convalidated),
	  (SELECT count(*) FROM pg_index WHERE indrelid IN ('products'::regclass,'product_images'::regclass,'product_catalog_counters'::regclass,'product_operation_receipts'::regclass)),
	  (SELECT count(*) FROM pg_index WHERE indrelid IN ('products'::regclass,'product_images'::regclass,'product_catalog_counters'::regclass,'product_operation_receipts'::regclass) AND (NOT indisvalid OR NOT indisready OR NOT indislive)),
	  (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('products'::regclass,'product_images'::regclass,'product_catalog_counters'::regclass,'product_operation_receipts'::regclass) AND confrelid='event_log'::regclass)`).Scan(
		&waterline, &constraints, &invalidConstraints, &indexes, &invalidIndexes, &eventForeignKeys,
	)
	if err != nil || waterline != 29 || constraints < 18 || invalidConstraints != 0 || indexes < 6 || invalidIndexes != 0 || eventForeignKeys != 0 {
		t.Fatalf("catalog waterline/constraints/invalid/indexes/invalid/event-fk/error=%d/%d/%d/%d/%d/%d/%v",
			waterline, constraints, invalidConstraints, indexes, invalidIndexes, eventForeignKeys, err)
	}
}

func realService(pool *pgxpool.Pool) *productapp.Service {
	return productapp.NewService(platformstore.NewUnitOfWork(pool), productstore.NewCatalogRepository(), eventstore.NewAppender())
}

func realEntitlementService(pool *pgxpool.Pool) *productapp.EntitlementService {
	return productapp.NewEntitlementService(
		platformstore.NewUnitOfWork(pool), productstore.NewCatalogRepository(), orderstore.NewRepository(), eventstore.NewAppender(),
	)
}

func sameLocalEntitlement(left, right productport.LocalEntitlement) bool {
	if left.ID != right.ID || left.ProductID != right.ProductID || left.OrderID != right.OrderID || left.CustomerID != right.CustomerID || left.State != right.State || left.Version != right.Version || !left.GrantedAt.Equal(right.GrantedAt) {
		return false
	}
	if left.RevokedAt == nil || right.RevokedAt == nil {
		return left.RevokedAt == nil && right.RevokedAt == nil
	}
	return left.RevokedAt.Equal(*right.RevokedAt)
}

func assertProductFactCounts(t *testing.T, pool *pgxpool.Pool, ctx context.Context, code string, actor int64, key string, wantProducts, wantReceipts, wantEvents int) {
	t.Helper()
	digest := sha256.Sum256([]byte(fmt.Sprintf("admin:%d\x00%s", actor, key)))
	eventKey := "product.create:" + hex.EncodeToString(digest[:])
	keyDigest := sha256.Sum256([]byte(key))
	var products, receipts, events int
	err := pool.QueryRow(ctx, `SELECT
      (SELECT count(*) FROM products WHERE product_code=$1),
      (SELECT count(*) FROM product_operation_receipts WHERE actor_scope=$2 AND key_digest=$3),
      (SELECT count(*) FROM event_log WHERE idempotency_key=$4)`,
		code, fmt.Sprintf("admin:%d", actor), keyDigest[:], eventKey).Scan(&products, &receipts, &events)
	if err != nil || products != wantProducts || receipts != wantReceipts || events != wantEvents {
		t.Fatalf("facts products/receipts/events/error=%d/%d/%d/%v want=%d/%d/%d", products, receipts, events, err, wantProducts, wantReceipts, wantEvents)
	}
}

func openPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *i01aDatabaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*i01aDatabaseURL); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, *i01aDatabaseURL)
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

func explain(t *testing.T, ctx context.Context, tx pgx.Tx, sql string) string {
	t.Helper()
	rows, err := tx.Query(ctx, sql)
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

func uniqueCode(prefix string) string {
	return fmt.Sprintf("i01a-%s-%d", prefix, time.Now().UnixNano())
}

func jsonEquivalent(a, b []byte) bool {
	var left, right any
	return json.Unmarshal(a, &left) == nil && json.Unmarshal(b, &right) == nil && fmt.Sprintf("%#v", left) == fmt.Sprintf("%#v", right)
}
