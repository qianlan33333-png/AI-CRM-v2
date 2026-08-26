package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	orderprovider "github.com/qianlan33333-png/AI-CRM-v2/internal/order/provider"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	weChatShopMaterialSyncedEvent    = "order.wechat_shop.material_synced"
	weChatShopLegacyImportedEvent    = "order.wechat_shop.legacy_material_imported"
	weChatShopLegacyQuarantinedEvent = "order.wechat_shop.legacy_material_quarantined"
	maxLegacyWeChatShopRawBytes      = 256 << 10
)

type WeChatShopLegacyOrderRow struct {
	SourceID      int64
	OrderID       string
	TransactionID string
	AmountMinor   int64
	Currency      string
	RawOrderJSON  []byte
	SyncedAt      time.Time
}

type WeChatShopLegacyQuarantine struct {
	SourceTable     string
	SourceKeyDigest [32]byte
	PayloadDigest   [32]byte
	ReasonCode      string
	RecordedAt      time.Time
}

type WeChatShopLegacyImport struct {
	Material   *orderport.WeChatShopOrderMaterial
	Quarantine *WeChatShopLegacyQuarantine
}

type WeChatShopMaterialStore interface {
	UpsertWeChatShopOrderMaterial(context.Context, orderport.WeChatShopOrderMaterial) (bool, error)
	GetWeChatShopOrderMaterial(context.Context, string) (orderport.WeChatShopOrderMaterial, bool, error)
	RecordWeChatShopLegacyQuarantine(context.Context, WeChatShopLegacyQuarantine) (bool, error)
}

type WeChatShopMaterialService struct {
	uow      platformport.UnitOfWork
	store    WeChatShopMaterialStore
	provider orderport.WeChatShopOrderMaterialProvider
	events   eventport.Appender
	now      func() time.Time
}

var _ orderport.WeChatShopOrderMaterialApplication = (*WeChatShopMaterialService)(nil)

func NewWeChatShopMaterialService(uow platformport.UnitOfWork, store WeChatShopMaterialStore, provider orderport.WeChatShopOrderMaterialProvider, events eventport.Appender) (*WeChatShopMaterialService, error) {
	if uow == nil || store == nil || provider == nil || events == nil {
		return nil, orderport.ErrWeChatShopMaterialUnavailable
	}
	return &WeChatShopMaterialService{uow: uow, store: store, provider: provider, events: events, now: time.Now}, nil
}

func (service *WeChatShopMaterialService) SyncOrder(ctx context.Context, providerOrderID string) (orderport.WeChatShopOrderMaterial, error) {
	if !service.ready() || ctx == nil || ctx.Err() != nil || !validReference(providerOrderID) {
		return orderport.WeChatShopOrderMaterial{}, orderport.ErrWeChatShopMaterialInvalid
	}
	material, err := service.provider.GetOrder(ctx, providerOrderID)
	if err != nil {
		return orderport.WeChatShopOrderMaterial{}, errors.Join(orderport.ErrWeChatShopMaterialUnavailable, err)
	}
	if !validWeChatShopMaterial(material) || material.Source != orderport.WeChatShopMaterialProvider || !material.ProviderVerified || material.ProviderOrderID != providerOrderID {
		return orderport.WeChatShopOrderMaterial{}, orderport.ErrWeChatShopMaterialUnavailable
	}
	err = service.uow.Within(ctx, func(tx context.Context) error {
		changed, storeErr := service.store.UpsertWeChatShopOrderMaterial(tx, material)
		if storeErr != nil {
			return storeErr
		}
		if !changed {
			stored, found, getErr := service.store.GetWeChatShopOrderMaterial(tx, providerOrderID)
			if getErr != nil || !found || !validWeChatShopMaterial(stored) {
				return errors.Join(orderport.ErrWeChatShopMaterialUnavailable, getErr)
			}
			material = stored
			return nil
		}
		return appendWeChatShopMaterialEvent(tx, service.events, weChatShopMaterialSyncedEvent, material, material.EvidenceDigest, service.now().UTC())
	})
	if err != nil {
		return orderport.WeChatShopOrderMaterial{}, errors.Join(orderport.ErrWeChatShopMaterialUnavailable, err)
	}
	return material, nil
}

func (service *WeChatShopMaterialService) GetOrderMaterial(ctx context.Context, providerOrderID string) (orderport.WeChatShopOrderMaterial, error) {
	if !service.ready() || ctx == nil || ctx.Err() != nil || !validReference(providerOrderID) {
		return orderport.WeChatShopOrderMaterial{}, orderport.ErrWeChatShopMaterialInvalid
	}
	var material orderport.WeChatShopOrderMaterial
	var found bool
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var storeErr error
		material, found, storeErr = service.store.GetWeChatShopOrderMaterial(tx, providerOrderID)
		return storeErr
	})
	if err != nil {
		return orderport.WeChatShopOrderMaterial{}, errors.Join(orderport.ErrWeChatShopMaterialUnavailable, err)
	}
	if !found {
		return orderport.WeChatShopOrderMaterial{}, orderport.ErrWeChatShopMaterialNotFound
	}
	if material.ProviderOrderID != providerOrderID || !validWeChatShopMaterial(material) {
		return orderport.WeChatShopOrderMaterial{}, orderport.ErrWeChatShopMaterialUnavailable
	}
	return material, nil
}

func (service *WeChatShopMaterialService) ImportLegacyOrder(ctx context.Context, row WeChatShopLegacyOrderRow) (WeChatShopLegacyImport, error) {
	if !service.ready() || ctx == nil || ctx.Err() != nil {
		return WeChatShopLegacyImport{}, orderport.ErrWeChatShopMaterialInvalid
	}
	result := AdaptWeChatShopLegacyOrder(row, service.now().UTC())
	if result.Material == nil && result.Quarantine == nil {
		return WeChatShopLegacyImport{}, orderport.ErrWeChatShopMaterialInvalid
	}
	err := service.uow.Within(ctx, func(tx context.Context) error {
		if result.Quarantine != nil {
			changed, storeErr := service.store.RecordWeChatShopLegacyQuarantine(tx, *result.Quarantine)
			if storeErr != nil || !changed {
				return storeErr
			}
			return appendWeChatShopLegacyQuarantineEvent(tx, service.events, *result.Quarantine)
		}
		changed, storeErr := service.store.UpsertWeChatShopOrderMaterial(tx, *result.Material)
		if storeErr != nil {
			return storeErr
		}
		if !changed {
			stored, found, getErr := service.store.GetWeChatShopOrderMaterial(tx, result.Material.ProviderOrderID)
			if getErr != nil || !found || !validWeChatShopMaterial(stored) {
				return errors.Join(orderport.ErrWeChatShopMaterialUnavailable, getErr)
			}
			result.Material = &stored
			return nil
		}
		return appendWeChatShopMaterialEvent(tx, service.events, weChatShopLegacyImportedEvent, *result.Material, result.Material.SourceKeyDigest, service.now().UTC())
	})
	if err != nil {
		return WeChatShopLegacyImport{}, errors.Join(orderport.ErrWeChatShopMaterialUnavailable, err)
	}
	return result, nil
}

func AdaptWeChatShopLegacyOrder(row WeChatShopLegacyOrderRow, at time.Time) WeChatShopLegacyImport {
	sourceKey := domainDigest("order/wechat-shop/legacy-source/v1", "wechat_shop_orders", fmt.Sprint(row.SourceID), row.OrderID)
	payload := domainDigest("order/wechat-shop/legacy-payload/v1", digestHexValue(sha256.Sum256(row.RawOrderJSON)))
	quarantine := func(reason string) WeChatShopLegacyImport {
		return WeChatShopLegacyImport{Quarantine: &WeChatShopLegacyQuarantine{SourceTable: "wechat_shop_orders", SourceKeyDigest: sourceKey, PayloadDigest: payload, ReasonCode: reason, RecordedAt: at.UTC()}}
	}
	if row.SourceID < 1 || !validReference(row.OrderID) || row.Currency != "CNY" || row.AmountMinor < 0 || row.SyncedAt.IsZero() || at.IsZero() {
		return quarantine("invalid_source_row")
	}
	if len(row.RawOrderJSON) == 0 || len(row.RawOrderJSON) > maxLegacyWeChatShopRawBytes || !json.Valid(row.RawOrderJSON) {
		return quarantine("raw_order_json_invalid")
	}
	material, err := orderprovider.NormalizeWeChatShopOrderMaterial(row.RawOrderJSON, row.OrderID, orderport.WeChatShopMaterialLegacyRaw, row.SyncedAt.UTC())
	if err != nil {
		return quarantine("raw_order_json_not_exact")
	}
	if material.AmountMinor != row.AmountMinor {
		return quarantine("typed_amount_conflict")
	}
	if row.TransactionID != "" {
		if !validReference(row.TransactionID) || !sameDigest(material.TransactionDigest, domainDigest("wechat-shop/transaction/v1", row.TransactionID)) {
			return quarantine("typed_transaction_conflict")
		}
	}
	material.SourceKeyDigest = sourceKey
	material.Readiness = orderport.WeChatShopMaterialProviderSyncRequired
	material.ProviderVerified = false
	return WeChatShopLegacyImport{Material: &material}
}

func (service *WeChatShopMaterialService) ready() bool {
	return service != nil && service.uow != nil && service.store != nil && service.provider != nil && service.events != nil && service.now != nil
}

func validWeChatShopMaterial(material orderport.WeChatShopOrderMaterial) bool {
	if !validReference(material.ProviderOrderID) || material.StatusCode < 0 || material.AmountMinor < 0 || material.Currency != "CNY" || material.EvidenceDigest == ([32]byte{}) || material.SyncedAt.IsZero() || len(material.Lines) == 0 {
		return false
	}
	switch material.Source {
	case orderport.WeChatShopMaterialProvider:
		if !material.ProviderVerified || material.SourceKeyDigest != ([32]byte{}) {
			return false
		}
	case orderport.WeChatShopMaterialLegacyRaw:
		if material.ProviderVerified || material.SourceKeyDigest == ([32]byte{}) || material.Readiness != orderport.WeChatShopMaterialProviderSyncRequired {
			return false
		}
	default:
		return false
	}
	seen := make(map[string]struct{}, len(material.Lines))
	readyLine, missingAfterSale := false, false
	for index, line := range material.Lines {
		if line.Position != index+1 || !validReference(line.ProductID) || !validReference(line.SKUID) || line.SKUCount < 1 || line.RealPriceMinor < 0 {
			return false
		}
		key := line.ProductID + "\x00" + line.SKUID
		if _, exists := seen[key]; exists {
			return false
		}
		seen[key] = struct{}{}
		if line.AfterSaleEvidenceExact {
			if line.OnAfterSaleSKUCount < 0 || line.FinishAfterSaleSKUCount < 0 || line.RemainingSKUCount != line.SKUCount-line.OnAfterSaleSKUCount-line.FinishAfterSaleSKUCount || line.RemainingSKUCount < 0 {
				return false
			}
			if (line.RemainingSKUCount > 0 && line.Readiness != orderport.WeChatShopLineReady) || (line.RemainingSKUCount == 0 && line.Readiness != orderport.WeChatShopLineNoRemainingCount) {
				return false
			}
			readyLine = readyLine || line.RemainingSKUCount > 0
		} else if line.OnAfterSaleSKUCount != 0 || line.FinishAfterSaleSKUCount != 0 || line.RemainingSKUCount != 0 || line.Readiness != orderport.WeChatShopLineAfterSaleEvidenceMiss {
			return false
		} else {
			missingAfterSale = true
		}
	}
	if material.Source == orderport.WeChatShopMaterialLegacyRaw {
		return true
	}
	expected := orderport.WeChatShopMaterialReady
	if !material.DealRecorded {
		expected = orderport.WeChatShopMaterialOrderNotPaid
	} else if missingAfterSale {
		expected = orderport.WeChatShopMaterialAfterSaleEvidenceMiss
	} else if !readyLine {
		expected = orderport.WeChatShopMaterialNoRefundableLine
	}
	return material.Readiness == expected
}

func appendWeChatShopMaterialEvent(ctx context.Context, events eventport.Appender, eventType string, material orderport.WeChatShopOrderMaterial, digest [32]byte, at time.Time) error {
	orderDigest := domainDigest("order/wechat-shop/order-ref/v1", material.ProviderOrderID)
	payload, _ := json.Marshal(map[string]any{"provider": "wechat_shop", "provider_order_digest": hex.EncodeToString(orderDigest[:]), "source": material.Source, "readiness": material.Readiness, "line_count": len(material.Lines), "evidence_digest": hex.EncodeToString(material.EvidenceDigest[:])})
	_, err := events.Append(ctx, eventport.Event{Type: eventType, Payload: payload, OccurredAt: at.UTC(), IdempotencyKey: eventType + ":" + hex.EncodeToString(digest[:])})
	return err
}

func appendWeChatShopLegacyQuarantineEvent(ctx context.Context, events eventport.Appender, value WeChatShopLegacyQuarantine) error {
	payload, _ := json.Marshal(map[string]any{"source_table": value.SourceTable, "source_key_digest": hex.EncodeToString(value.SourceKeyDigest[:]), "payload_digest": hex.EncodeToString(value.PayloadDigest[:]), "reason_code": value.ReasonCode})
	_, err := events.Append(ctx, eventport.Event{Type: weChatShopLegacyQuarantinedEvent, Payload: payload, OccurredAt: value.RecordedAt.UTC(), IdempotencyKey: weChatShopLegacyQuarantinedEvent + ":" + hex.EncodeToString(value.SourceKeyDigest[:]) + ":" + hex.EncodeToString(value.PayloadDigest[:])})
	return err
}

func digestHexValue(value [32]byte) string { return hex.EncodeToString(value[:]) }
