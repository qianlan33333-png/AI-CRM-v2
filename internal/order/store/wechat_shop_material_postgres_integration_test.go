package store

import (
	"context"
	"crypto/sha256"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// This test becomes active after the serialized 00095 migration is applied.
// Package A cannot own that migration while 00094 is being integrated.
func TestWeChatShopMaterialPostgreSQLConstraintsAndPIIBoundary(t *testing.T) {
	databaseURL := os.Getenv("P4_WECHAT_SHOP_MATERIAL_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("P4_WECHAT_SHOP_MATERIAL_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
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
}
