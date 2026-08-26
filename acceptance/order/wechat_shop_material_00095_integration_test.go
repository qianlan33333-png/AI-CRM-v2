package order_acceptance

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	orderstore "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var weChatShopMaterialDatabaseURL = flag.String("wechat-shop-material-database-url", "", "PostgreSQL 16.14 acceptance database URL")

// This test becomes active after the serialized 00095 migration is applied.
// Package A cannot own that migration while 00094 is being integrated.
func TestWeChatShopMaterialPostgreSQLConstraintsAndPIIBoundary(t *testing.T) {
	if *weChatShopMaterialDatabaseURL == "" {
		t.Skip("-wechat-shop-material-database-url is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *weChatShopMaterialDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var version string
	if err = pool.QueryRow(ctx, "SHOW server_version_num").Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v", version, err)
	}
	rows, err := pool.Query(ctx, `SELECT table_name,column_name FROM information_schema.columns WHERE table_schema='public' AND table_name IN ('order_wechat_shop_materials','order_wechat_shop_material_lines','order_wechat_shop_material_quarantines') ORDER BY table_name,ordinal_position`)
	if err != nil {
		t.Fatal(err)
	}
	tables := map[string]bool{}
	for rows.Next() {
		var table, column string
		if err = rows.Scan(&table, &column); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		tables[table] = true
		for _, forbidden := range []string{"phone", "mobile", "openid", "unionid", "address", "raw_order", "access_token", "appsecret"} {
			if strings.Contains(column, forbidden) {
				rows.Close()
				t.Fatalf("PII/secret column %s.%s", table, column)
			}
		}
	}
	rows.Close()
	if len(tables) != 3 {
		t.Fatalf("material tables=%v", tables)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err = tx.Exec(ctx, `TRUNCATE order_wechat_shop_material_lines, order_wechat_shop_materials, order_wechat_shop_material_quarantines RESTART IDENTITY`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	evidence, transaction := sha256.Sum256([]byte("evidence")), sha256.Sum256([]byte("transaction"))
	var materialID int64
	err = tx.QueryRow(ctx, `INSERT INTO order_wechat_shop_materials (provider_order_id,status_code,deal_recorded,amount_minor,currency,transaction_digest,evidence_digest,source,source_key_digest,readiness,provider_verified,synced_at,created_at,updated_at) VALUES ($1,20,true,12900,'CNY',$2,$3,'provider',NULL,'ready',true,$4,$4,$4) RETURNING id`, "shop-order-pg", transaction[:], evidence[:], now).Scan(&materialID)
	if err != nil || materialID < 1 {
		t.Fatalf("insert material id=%d err=%v", materialID, err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO order_wechat_shop_material_lines (material_id,position,product_id,sku_id,sku_count,on_aftersale_sku_count,finish_aftersale_sku_count,real_price_minor,remaining_sku_count,aftersale_evidence_exact,readiness,created_at) VALUES ($1,1,'product-1','sku-1',2,1,0,6450,1,true,'ready',$2)`, materialID, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO order_wechat_shop_material_lines (material_id,position,product_id,sku_id,sku_count,on_aftersale_sku_count,finish_aftersale_sku_count,real_price_minor,remaining_sku_count,aftersale_evidence_exact,readiness,created_at) VALUES ($1,2,'product-2','sku-2',1,NULL,NULL,6450,NULL,false,'aftersale_evidence_missing',$2)`, materialID, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "SAVEPOINT invalid_material_line"); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO order_wechat_shop_material_lines (material_id,position,product_id,sku_id,sku_count,on_aftersale_sku_count,finish_aftersale_sku_count,real_price_minor,remaining_sku_count,aftersale_evidence_exact,readiness,created_at) VALUES ($1,3,'product-3','sku-3',1,1,1,100,-1,true,'ready',$2)`, materialID, now)
	if err == nil {
		t.Fatal("invalid aftersale counts were accepted")
	}
	if _, err = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT invalid_material_line"); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "SAVEPOINT invalid_legacy_material"); err != nil {
		t.Fatal(err)
	}
	legacyKey := sha256.Sum256([]byte("legacy-key"))
	_, err = tx.Exec(ctx, `INSERT INTO order_wechat_shop_materials (provider_order_id,status_code,deal_recorded,amount_minor,currency,evidence_digest,source,source_key_digest,readiness,provider_verified,synced_at,created_at,updated_at) VALUES ('legacy-invalid',20,true,100,'CNY',$1,'legacy_raw',$2,'provider_sync_required',true,$3,$3,$3)`, evidence[:], legacyKey[:], now)
	if err == nil {
		t.Fatal("legacy material was accepted as Provider-verified")
	}
	if _, err = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT invalid_legacy_material"); err != nil {
		t.Fatal(err)
	}
	payload := sha256.Sum256([]byte("legacy-payload"))
	for range 2 {
		if _, err = tx.Exec(ctx, `INSERT INTO order_wechat_shop_material_quarantines (source_table,source_key_digest,payload_digest,reason_code,recorded_at) VALUES ('wechat_shop_orders',$1,$2,'raw_order_json_not_exact',$3) ON CONFLICT (source_table,source_key_digest,payload_digest) DO NOTHING`, legacyKey[:], payload[:], now); err != nil {
			t.Fatal(err)
		}
	}
	var quarantineCount int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM order_wechat_shop_material_quarantines WHERE source_table='wechat_shop_orders' AND source_key_digest=$1 AND payload_digest=$2`, legacyKey[:], payload[:]).Scan(&quarantineCount); err != nil || quarantineCount != 1 {
		t.Fatalf("quarantine count=%d err=%v", quarantineCount, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	repository := orderstore.NewWeChatShopMaterialRepository()
	uow := platformstore.NewUnitOfWork(pool)
	provider := orderport.WeChatShopOrderMaterial{
		ProviderOrderID: "shop-order-repository", StatusCode: 20, DealRecorded: true,
		AmountMinor: 12900, Currency: "CNY", TransactionDigest: transaction,
		EvidenceDigest: sha256.Sum256([]byte("repository-provider-evidence")),
		Source:         orderport.WeChatShopMaterialProvider, Readiness: orderport.WeChatShopMaterialAfterSaleEvidenceMiss,
		ProviderVerified: true, CreatedAt: now.Add(-2 * time.Hour), PaidAt: now.Add(-time.Hour),
		UpdatedAt: now.Add(-30 * time.Minute), SyncedAt: now,
		Lines: []orderport.WeChatShopOrderLine{
			{Position: 1, ProductID: "product-1", SKUID: "sku-1", SKUCount: 2, OnAfterSaleSKUCount: 1, RealPriceMinor: 6450, RemainingSKUCount: 1, AfterSaleEvidenceExact: true, Readiness: orderport.WeChatShopLineReady},
			{Position: 2, ProductID: "product-2", SKUID: "sku-2", SKUCount: 1, RealPriceMinor: 6450, Readiness: orderport.WeChatShopLineAfterSaleEvidenceMiss},
		},
	}
	assertMaterialUpsertAndRead(t, ctx, uow, repository, provider, true)
	assertMaterialUpsertAndRead(t, ctx, uow, repository, provider, false)

	legacyOrder := provider
	legacyOrder.ProviderOrderID = "shop-order-replace-legacy"
	legacyOrder.TransactionDigest = [32]byte{}
	legacyOrder.EvidenceDigest = sha256.Sum256([]byte("legacy-evidence"))
	legacyOrder.SourceKeyDigest = sha256.Sum256([]byte("legacy-source"))
	legacyOrder.Source = orderport.WeChatShopMaterialLegacyRaw
	legacyOrder.Readiness = orderport.WeChatShopMaterialProviderSyncRequired
	legacyOrder.ProviderVerified = false
	legacyOrder.SyncedAt = now.Add(-time.Hour)
	assertMaterialUpsertAndRead(t, ctx, uow, repository, legacyOrder, true)

	verifiedOrder := provider
	verifiedOrder.ProviderOrderID = legacyOrder.ProviderOrderID
	verifiedOrder.EvidenceDigest = sha256.Sum256([]byte("verified-evidence"))
	verifiedOrder.SyncedAt = now
	assertMaterialUpsertAndRead(t, ctx, uow, repository, verifiedOrder, true)
	legacyOrder.EvidenceDigest = sha256.Sum256([]byte("late-legacy-evidence"))
	legacyOrder.SyncedAt = now.Add(time.Hour)
	assertMaterialUpsertAndRead(t, ctx, uow, repository, legacyOrder, false)

	quarantine := orderapp.WeChatShopLegacyQuarantine{
		SourceTable: "wechat_shop_orders", SourceKeyDigest: legacyKey,
		PayloadDigest: sha256.Sum256([]byte("repository-quarantine")),
		ReasonCode:    "raw_order_json_not_exact", RecordedAt: now,
	}
	var changed bool
	if err = uow.Within(ctx, func(txContext context.Context) error {
		changed, err = repository.RecordWeChatShopLegacyQuarantine(txContext, quarantine)
		return err
	}); err != nil || !changed {
		t.Fatalf("record quarantine changed=%v err=%v", changed, err)
	}
	if err = uow.Within(ctx, func(txContext context.Context) error {
		changed, err = repository.RecordWeChatShopLegacyQuarantine(txContext, quarantine)
		return err
	}); err != nil || changed {
		t.Fatalf("replay quarantine changed=%v err=%v", changed, err)
	}
	quarantine.ReasonCode = "invalid_source_row"
	err = uow.Within(ctx, func(txContext context.Context) error {
		_, recordErr := repository.RecordWeChatShopLegacyQuarantine(txContext, quarantine)
		return recordErr
	})
	if !errors.Is(err, orderport.ErrWeChatShopMaterialConflict) {
		t.Fatalf("conflicting quarantine error=%v", err)
	}
}

func assertMaterialUpsertAndRead(t *testing.T, ctx context.Context, uow *platformstore.UnitOfWork, repository *orderstore.WeChatShopMaterialRepository, material orderport.WeChatShopOrderMaterial, wantChanged bool) {
	t.Helper()
	var changed bool
	var stored orderport.WeChatShopOrderMaterial
	err := uow.Within(ctx, func(txContext context.Context) error {
		var storeErr error
		changed, storeErr = repository.UpsertWeChatShopOrderMaterial(txContext, material)
		if storeErr != nil {
			return storeErr
		}
		stored, _, storeErr = repository.GetWeChatShopOrderMaterial(txContext, material.ProviderOrderID)
		return storeErr
	})
	if err != nil || changed != wantChanged {
		t.Fatalf("upsert %s changed=%v want=%v err=%v", material.ProviderOrderID, changed, wantChanged, err)
	}
	if stored.ProviderOrderID != material.ProviderOrderID || stored.Source != material.Source && wantChanged || len(stored.Lines) != len(material.Lines) {
		t.Fatalf("stored material=%#v input=%#v changed=%v", stored, material, wantChanged)
	}
	if wantChanged && stored.Lines[1].AfterSaleEvidenceExact {
		t.Fatalf("missing aftersale evidence was not preserved as NULL: %#v", stored.Lines[1])
	}
}
