package product_acceptance

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	contactfixture "github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	orderfixture "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store/acceptancefixture"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productstore "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store"
)

func TestD01ServicePeriodLifecycleUsesProductCASReceiptsAndRetainsReferences(t *testing.T) {
	pool, ctx := openPool(t)
	service := productapp.NewServicePeriodService(
		platformstore.NewUnitOfWork(pool),
		productstore.NewCatalogRepository(),
		eventstore.NewAppender(),
	)
	ordinary := realService(pool)
	entitlements := realEntitlementService(pool)
	actor := int64(9701)
	code := uniqueCode("d01-service-period")
	createKey := "d01-service-period-create-" + code
	createCommand := productport.CreateServicePeriodProductCommand{
		ProductCode:    code,
		Name:           "D01 周期商品",
		Description:    "local lifecycle only",
		PriceMinor:     16800,
		Currency:       "cny",
		StockQuantity:  12,
		Actor:          actor,
		IdempotencyKey: createKey,
	}
	ordinaryBefore, err := ordinary.ListLegacy(ctx, 1, 0)
	if err != nil {
		t.Fatal(err)
	}

	created, err := service.CreateServicePeriodProduct(ctx, createCommand)
	if err != nil || created.Version != 1 || created.Lifecycle != productport.ServicePeriodDraft || created.Enabled || created.Archived || created.Currency != "CNY" {
		t.Fatalf("create=%+v err=%v", created, err)
	}
	replayedCreate, err := service.CreateServicePeriodProduct(ctx, createCommand)
	if err != nil || replayedCreate != created {
		t.Fatalf("create replay=%+v err=%v want=%+v", replayedCreate, err, created)
	}
	createCommand.PriceMinor++
	if _, err = service.CreateServicePeriodProduct(ctx, createCommand); !errors.Is(err, productapp.ErrConflict) {
		t.Fatalf("changed create replay error=%v", err)
	}

	if _, err = service.UpdateServicePeriodProduct(ctx, productport.UpdateServicePeriodProductCommand{
		ID:              created.ServiceProductID,
		ExpectedVersion: created.Version + 1,
		Name:            "stale",
		Description:     "stale",
		PriceMinor:      1,
		Currency:        "CNY",
		StockQuantity:   1,
		Actor:           actor,
		IdempotencyKey:  "d01-service-period-stale-" + code,
	}); !errors.Is(err, productapp.ErrConflict) {
		t.Fatalf("stale update error=%v", err)
	}

	updated, err := service.UpdateServicePeriodProduct(ctx, productport.UpdateServicePeriodProductCommand{
		ID:              created.ServiceProductID,
		ExpectedVersion: created.Version,
		Name:            "D01 周期商品已更新",
		Description:     "still local lifecycle only",
		PriceMinor:      18800,
		Currency:        "cny",
		StockQuantity:   15,
		Actor:           actor,
		IdempotencyKey:  "d01-service-period-update-" + code,
	})
	if err != nil || updated.Version != 2 || updated.Name != "D01 周期商品已更新" || updated.PriceMinor != 18800 || updated.Lifecycle != productport.ServicePeriodDraft {
		t.Fatalf("update=%+v err=%v", updated, err)
	}

	enabled, err := service.SetServicePeriodProductEnabled(ctx, productport.SetServicePeriodProductEnabledCommand{
		ID:              updated.ServiceProductID,
		ExpectedVersion: updated.Version,
		Enabled:         true,
		Actor:           actor,
		IdempotencyKey:  "d01-service-period-enable-" + code,
	})
	if err != nil || enabled.Version != 3 || enabled.Lifecycle != productport.ServicePeriodEnabled || !enabled.Enabled || enabled.Archived {
		t.Fatalf("enable=%+v err=%v", enabled, err)
	}

	disabled, err := service.SetServicePeriodProductEnabled(ctx, productport.SetServicePeriodProductEnabledCommand{
		ID:              enabled.ServiceProductID,
		ExpectedVersion: enabled.Version,
		Enabled:         false,
		Actor:           actor,
		IdempotencyKey:  "d01-service-period-disable-" + code,
	})
	if err != nil || disabled.Version != 4 || disabled.Lifecycle != productport.ServicePeriodDisabled || disabled.Enabled || disabled.Archived {
		t.Fatalf("disable=%+v err=%v", disabled, err)
	}

	copied, err := service.CopyServicePeriodProduct(ctx, productport.CopyServicePeriodProductCommand{
		ID:              disabled.ServiceProductID,
		ExpectedVersion: disabled.Version,
		Actor:           actor,
		IdempotencyKey:  "d01-service-period-copy-" + code,
	})
	if err != nil || copied.ServiceProductID == disabled.ServiceProductID || copied.Version != 1 || copied.Lifecycle != productport.ServicePeriodDraft || copied.Enabled || copied.Archived || copied.ProductCode == disabled.ProductCode {
		t.Fatalf("copy=%+v err=%v", copied, err)
	}
	if copied.PriceMinor != disabled.PriceMinor || copied.Currency != disabled.Currency || copied.StockQuantity != disabled.StockQuantity {
		t.Fatalf("copy lost stable Product fields source=%+v copy=%+v", disabled, copied)
	}

	customerID, err := contactfixture.CreateCustomerWithDetails(ctx, pool, "D01 本地引用客户", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var orderID int64
	merchantOrderNo := "d01-service-period-order-" + code
	// order_list_projections requires a provider enum and an eligible local paid
	// status before the existing Local Entitlement application will grant. This
	// row is a direct local fixture only: no payment or Provider client is called,
	// and the test makes no claim that an external payment occurred.
	orderID, err = orderfixture.CreatePaidProjection(ctx, pool, orderfixture.PaidProjection{
		ProviderLabel: "本地引用夹具", MerchantOrderNo: merchantOrderNo, CustomerID: customerID,
		ProductID: int64(disabled.ServiceProductID), ProductCode: disabled.ProductCode, ProductName: disabled.Name,
		AmountMinor: disabled.PriceMinor, Currency: disabled.Currency, StatusLabel: "本地既存引用", DetailURL: "/orders/" + merchantOrderNo,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = entitlements.Grant(ctx, productport.GrantLocalEntitlementCommand{
		ProductID:      disabled.ServiceProductID,
		OrderID:        orderID,
		Actor:          actor,
		IdempotencyKey: "d01-service-period-entitlement-" + code,
	}); !errors.Is(err, productapp.ErrEntitlementNotFound) {
		t.Fatalf("generic local entitlement must reject service-period product: %v", err)
	}

	archived, err := service.ArchiveServicePeriodProduct(ctx, productport.ArchiveServicePeriodProductCommand{
		ID:              disabled.ServiceProductID,
		ExpectedVersion: disabled.Version,
		Actor:           actor,
		IdempotencyKey:  "d01-service-period-archive-" + code,
	})
	if err != nil || archived.Version != 5 || archived.Lifecycle != productport.ServicePeriodArchived || !archived.Archived || archived.Enabled {
		t.Fatalf("archive=%+v err=%v", archived, err)
	}

	loadedArchived, err := service.GetServicePeriodProduct(ctx, archived.ServiceProductID)
	if err != nil || loadedArchived != archived {
		t.Fatalf("archived readback=%+v err=%v want=%+v", loadedArchived, err, archived)
	}
	ordinaryAfter, err := ordinary.ListLegacy(ctx, 1, 0)
	if err != nil || ordinaryAfter.Total != ordinaryBefore.Total {
		t.Fatalf("ordinary catalog total before/after service-period lifecycle=%d/%d err=%v", ordinaryBefore.Total, ordinaryAfter.Total, err)
	}
	if _, err = ordinary.Get(ctx, archived.ServiceProductID); !errors.Is(err, productapp.ErrNotFound) {
		t.Fatalf("ordinary catalog read must reject service-period product: %v", err)
	}
	if _, err = ordinary.Update(ctx, productport.UpdateCommand{
		ID:              archived.ServiceProductID,
		ExpectedVersion: archived.Version,
		Name:            archived.Name,
		Description:     archived.Description,
		PriceMinor:      archived.PriceMinor,
		Currency:        archived.Currency,
		StockQuantity:   archived.StockQuantity,
		Actor:           actor,
		IdempotencyKey:  "d01-service-period-ordinary-update-" + code,
	}); !errors.Is(err, productapp.ErrNotFound) {
		t.Fatalf("ordinary catalog update must reject service-period product: %v", err)
	}

	actorScope := fmt.Sprintf("admin:%d", actor)
	var productRows, entitlementRows, orderRows, completedReceipts, servicePeriodEvents, invalidEventTypes int
	var archivedStatus string
	var archivedEnabled bool
	err = pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM products WHERE id IN ($1,$2)),
		(SELECT count(*) FROM product_local_entitlements WHERE product_id=$1 AND order_id=$3),
		(SELECT count(*) FROM order_list_projections WHERE id=$4 AND product_id=$1),
		(SELECT count(*) FROM product_operation_receipts WHERE actor_scope=$5 AND state='completed'),
		(SELECT count(*) FROM event_log WHERE payload->>'kind'='service_period' AND payload->>'actor'=$6),
		(SELECT count(*) FROM event_log WHERE payload->>'kind'='service_period' AND event_type NOT IN ($7,$8)),
		(SELECT legacy_admin_projection->>'status' FROM products WHERE id=$1),
		(SELECT (legacy_admin_projection->>'enabled')::boolean FROM products WHERE id=$1)`,
		int64(archived.ServiceProductID),
		int64(copied.ServiceProductID),
		orderID,
		orderID,
		actorScope,
		fmt.Sprint(actor),
		eventport.EvProductCreated,
		eventport.EvProductUpdated,
	).Scan(&productRows, &entitlementRows, &orderRows, &completedReceipts, &servicePeriodEvents, &invalidEventTypes, &archivedStatus, &archivedEnabled)
	if err != nil {
		t.Fatal(err)
	}
	if productRows != 2 || entitlementRows != 0 || orderRows != 1 {
		t.Fatalf("retained rows products/entitlements/orders=%d/%d/%d", productRows, entitlementRows, orderRows)
	}
	if completedReceipts != 6 || servicePeriodEvents != 6 || invalidEventTypes != 0 {
		t.Fatalf("receipts/events/invalid_event_types=%d/%d/%d", completedReceipts, servicePeriodEvents, invalidEventTypes)
	}
	if archivedStatus != productapp.ServicePeriodProjectionArchivedStatus || archivedEnabled {
		t.Fatalf("archive projection status/enabled=%q/%v", archivedStatus, archivedEnabled)
	}

	page, err := service.ListServicePeriodProducts(ctx, 1, 0)
	if err != nil || !page.OK || page.Limit != 1 || len(page.Items) != 1 || page.Total < 2 {
		t.Fatalf("bounded page=%+v err=%v", page, err)
	}
	if _, err = service.ListServicePeriodProducts(ctx, productapp.MaximumLimit+1, 0); !errors.Is(err, productapp.ErrInvalidCursor) {
		t.Fatalf("over-limit read error=%v", err)
	}
}

func TestD01Migration50CompatibilityUsesExistingProductAndLocalEntitlementFacts(t *testing.T) {
	pool, ctx := openPool(t)
	var versionColumns, localEntitlementTables, servicePeriodTables int
	var receiptConstraint string
	err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='products' AND column_name='version' AND data_type='bigint'),
		(SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='product_local_entitlements'),
		(SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name LIKE 'service_period_product%'),
		COALESCE((SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='product_operation_receipts'::regclass AND contype='c' AND pg_get_constraintdef(oid) LIKE '%operation%'), '')`).Scan(
		&versionColumns,
		&localEntitlementTables,
		&servicePeriodTables,
		&receiptConstraint,
	)
	if err != nil {
		t.Fatal(err)
	}
	if versionColumns != 1 || localEntitlementTables != 1 || servicePeriodTables != 0 {
		t.Fatalf("migration-50 facts version/local/service-period-tables=%d/%d/%d", versionColumns, localEntitlementTables, servicePeriodTables)
	}
	if receiptConstraint == "" || !strings.Contains(receiptConstraint, "create") || !strings.Contains(receiptConstraint, "update") {
		t.Fatalf("product receipt operation constraint=%q", receiptConstraint)
	}
}
