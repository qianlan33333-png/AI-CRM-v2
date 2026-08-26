package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

type materialProviderStub struct {
	material orderport.WeChatShopOrderMaterial
	calls    int
}

func (stub *materialProviderStub) GetOrder(_ context.Context, orderID string) (orderport.WeChatShopOrderMaterial, error) {
	stub.calls++
	result := stub.material
	result.ProviderOrderID = orderID
	return result, nil
}

type materialStoreStub struct {
	material   orderport.WeChatShopOrderMaterial
	quarantine WeChatShopLegacyQuarantine
	changed    bool
}

func (store *materialStoreStub) UpsertWeChatShopOrderMaterial(_ context.Context, material orderport.WeChatShopOrderMaterial) (bool, error) {
	store.material = material
	return store.changed, nil
}

func (store *materialStoreStub) GetWeChatShopOrderMaterial(_ context.Context, orderID string) (orderport.WeChatShopOrderMaterial, bool, error) {
	return store.material, store.material.ProviderOrderID == orderID, nil
}

func (store *materialStoreStub) RecordWeChatShopLegacyQuarantine(_ context.Context, value WeChatShopLegacyQuarantine) (bool, error) {
	store.quarantine = value
	return store.changed, nil
}

type materialEventsStub struct{ rows []eventport.Event }

func (events *materialEventsStub) Append(_ context.Context, event eventport.Event) (eventport.EventID, error) {
	events.rows = append(events.rows, event)
	return eventport.EventID(len(events.rows)), nil
}

func TestWeChatShopMaterialServiceSyncsVerifiedProviderProjection(t *testing.T) {
	now := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	evidence := domainDigest("evidence", "provider")
	provider := &materialProviderStub{material: orderport.WeChatShopOrderMaterial{
		StatusCode: 20, DealRecorded: true, AmountMinor: 12900, Currency: "CNY",
		EvidenceDigest: evidence, Source: orderport.WeChatShopMaterialProvider,
		Readiness: orderport.WeChatShopMaterialReady, ProviderVerified: true, SyncedAt: now,
		Lines: []orderport.WeChatShopOrderLine{{Position: 1, ProductID: "product-1", SKUID: "sku-1", SKUCount: 1, RealPriceMinor: 12900, RemainingSKUCount: 1, AfterSaleEvidenceExact: true, Readiness: orderport.WeChatShopLineReady}},
	}}
	store := &materialStoreStub{changed: true}
	events := &materialEventsStub{}
	service, err := NewWeChatShopMaterialService(commerceRefundTestUOW{}, store, provider, events)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	material, err := service.SyncOrder(context.Background(), "shop-order-1")
	if err != nil || material.ProviderOrderID != "shop-order-1" || store.material.EvidenceDigest != evidence || provider.calls != 1 || len(events.rows) != 1 || events.rows[0].Type != weChatShopMaterialSyncedEvent {
		t.Fatalf("material=%+v err=%v calls=%d events=%+v", material, err, provider.calls, events.rows)
	}
	if strings.Contains(string(events.rows[0].Payload), "shop-order-1") {
		t.Fatalf("provider order id leaked in event payload: %s", events.rows[0].Payload)
	}
	read, err := service.GetOrderMaterial(context.Background(), "shop-order-1")
	if err != nil || read.EvidenceDigest != evidence || len(read.Lines) != 1 {
		t.Fatalf("GetOrderMaterial=%+v err=%v", read, err)
	}
	if _, err = service.GetOrderMaterial(context.Background(), "missing-order"); !errors.Is(err, orderport.ErrWeChatShopMaterialNotFound) {
		t.Fatalf("missing material err=%v", err)
	}
	store.changed = false
	if _, err = service.SyncOrder(context.Background(), "shop-order-1"); err != nil || len(events.rows) != 1 {
		t.Fatalf("idempotent replay err=%v events=%d", err, len(events.rows))
	}
}

func TestAdaptWeChatShopLegacyOrderImportsOnlyExactRawMaterial(t *testing.T) {
	now := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	raw := []byte(`{"order":{"order_id":"legacy-order-1","status":20,"order_detail":{"pay_info":{"pay_time":1700000000,"transaction_id":"transaction-1","openid":"sensitive-openid"},"price_info":{"order_price":12900},"delivery_info":{"address_info":{"tel_number":"13900000000"}},"product_infos":[{"product_id":"product-1","sku_id":"sku-1","sku_cnt":2,"on_aftersale_sku_cnt":0,"finish_aftersale_sku_cnt":0,"real_price":6450}]}}}`)
	row := WeChatShopLegacyOrderRow{SourceID: 19, OrderID: "legacy-order-1", TransactionID: "transaction-1", AmountMinor: 12900, Currency: "CNY", RawOrderJSON: raw, SyncedAt: now.Add(-2 * time.Hour)}
	result := AdaptWeChatShopLegacyOrder(row, now)
	if result.Material == nil || result.Quarantine != nil || result.Material.Source != orderport.WeChatShopMaterialLegacyRaw || result.Material.ProviderVerified || result.Material.Readiness != orderport.WeChatShopMaterialProviderSyncRequired || result.Material.SourceKeyDigest == ([32]byte{}) {
		t.Fatalf("result=%+v", result)
	}
	encoded, err := json.Marshal(result.Material)
	if err != nil || strings.Contains(string(encoded), "13900000000") || strings.Contains(string(encoded), "sensitive-openid") || strings.Contains(string(encoded), "transaction-1") {
		t.Fatalf("PII/raw transaction leaked: %s err=%v", encoded, err)
	}

	amountConflict := row
	amountConflict.AmountMinor++
	result = AdaptWeChatShopLegacyOrder(amountConflict, now)
	if result.Material != nil || result.Quarantine == nil || result.Quarantine.ReasonCode != "typed_amount_conflict" || result.Quarantine.PayloadDigest == ([32]byte{}) {
		t.Fatalf("amount conflict=%+v", result)
	}
	transactionConflict := row
	transactionConflict.TransactionID = "other-transaction"
	result = AdaptWeChatShopLegacyOrder(transactionConflict, now)
	if result.Quarantine == nil || result.Quarantine.ReasonCode != "typed_transaction_conflict" {
		t.Fatalf("transaction conflict=%+v", result)
	}
	ambiguous := row
	ambiguous.RawOrderJSON = []byte(`{"order_detail":{"product_infos":[{"product_id":"product-1","sku_id":"sku-1","sku_cnt":1}]}}`)
	result = AdaptWeChatShopLegacyOrder(ambiguous, now)
	if result.Quarantine == nil || result.Quarantine.ReasonCode != "raw_order_json_not_exact" {
		t.Fatalf("ambiguous result=%+v", result)
	}
}

func TestWeChatShopMaterialServicePersistsLegacyQuarantineWithoutRawPayload(t *testing.T) {
	now := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	store := &materialStoreStub{changed: true}
	events := &materialEventsStub{}
	service, err := NewWeChatShopMaterialService(commerceRefundTestUOW{}, store, &materialProviderStub{}, events)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	result, err := service.ImportLegacyOrder(context.Background(), WeChatShopLegacyOrderRow{SourceID: 5, OrderID: "legacy-order-5", AmountMinor: 100, Currency: "CNY", RawOrderJSON: []byte(`{"buyer_mobile":"13900000000"}`), SyncedAt: now.Add(-time.Hour)})
	if err != nil || result.Quarantine == nil || store.quarantine.ReasonCode != "raw_order_json_not_exact" || len(events.rows) != 1 || events.rows[0].Type != weChatShopLegacyQuarantinedEvent {
		t.Fatalf("result=%+v err=%v quarantine=%+v events=%+v", result, err, store.quarantine, events.rows)
	}
	if strings.Contains(string(events.rows[0].Payload), "13900000000") {
		t.Fatalf("raw payload leaked in quarantine event: %s", events.rows[0].Payload)
	}

	badService := *service
	badService.uow = materialFailingUOW{}
	if _, err = badService.ImportLegacyOrder(context.Background(), WeChatShopLegacyOrderRow{SourceID: 5, OrderID: "legacy-order-5", AmountMinor: 100, Currency: "CNY", RawOrderJSON: []byte(`{}`), SyncedAt: now.Add(-time.Hour)}); !errors.Is(err, orderport.ErrWeChatShopMaterialUnavailable) {
		t.Fatalf("transaction failure err=%v", err)
	}
}

type materialFailingUOW struct{}

func (materialFailingUOW) Within(context.Context, func(context.Context) error) error {
	return errors.New("transaction unavailable")
}
