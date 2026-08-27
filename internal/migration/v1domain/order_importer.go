package v1domain

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

type WeChatShopImportResult struct {
	Imported    int
	Quarantined int
	Replayed    int
}

type WeChatShopImporter struct {
	archive ArchiveSource
	uow     UnitOfWork
	store   orderapp.WeChatShopMaterialStore
	journal *Journal
	now     func() time.Time
}

func NewWeChatShopImporter(archive ArchiveSource, uow UnitOfWork, store orderapp.WeChatShopMaterialStore, journal *Journal) (*WeChatShopImporter, error) {
	if archive == nil || uow == nil || store == nil || journal == nil {
		return nil, ErrInvalidScope
	}
	return &WeChatShopImporter{archive: archive, uow: uow, store: store, journal: journal, now: time.Now}, nil
}

type weChatShopJSON struct {
	ID            int64           `json:"id"`
	OrderID       string          `json:"order_id"`
	TransactionID string          `json:"transaction_id"`
	AmountTotal   int64           `json:"amount_total"`
	Currency      string          `json:"currency"`
	RawOrderJSON  json.RawMessage `json:"raw_order_json"`
	SyncedAt      time.Time       `json:"synced_at"`
}

// Import writes only PII-free legacy_raw material. It calls neither the Shop
// Provider nor the normal event appender and therefore cannot create payment,
// refund, entitlement, EER, or River work.
func (importer *WeChatShopImporter) Import(ctx context.Context, archiveRunID string) (WeChatShopImportResult, error) {
	if importer == nil || importer.now == nil || archiveRunID == "" {
		return WeChatShopImportResult{}, ErrInvalidScope
	}
	result := WeChatShopImportResult{}
	err := importer.archive.EachTableRow(ctx, archiveRunID, "public/wechat_shop_orders", func(row v1archive.ArchivedRow) error {
		var source weChatShopJSON
		if err := json.Unmarshal(row.Payload, &source); err != nil {
			return fmt.Errorf("decode archived WeChat Shop row %d: %w", row.SourceOrdinal, err)
		}
		adapted := orderapp.AdaptWeChatShopLegacyOrder(orderapp.WeChatShopLegacyOrderRow{
			SourceID: source.ID, OrderID: source.OrderID, TransactionID: source.TransactionID,
			AmountMinor: source.AmountTotal, Currency: source.Currency, RawOrderJSON: source.RawOrderJSON,
			SyncedAt: source.SyncedAt,
		}, importer.now().UTC())
		if adapted.Material == nil && adapted.Quarantine == nil {
			return ErrConflict
		}
		replayed := false
		err := importer.uow.Within(ctx, func(tx context.Context) error {
			if adapted.Quarantine != nil {
				changed, err := importer.store.RecordWeChatShopLegacyQuarantine(tx, *adapted.Quarantine)
				if err != nil {
					return err
				}
				replayed = !changed
				return importer.journal.Record(tx, TerminalReceipt{
					SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC,
					Disposition: "quarantine", Reason: adapted.Quarantine.ReasonCode,
				})
			}
			changed, err := importer.store.UpsertWeChatShopOrderMaterial(tx, *adapted.Material)
			if err != nil {
				return err
			}
			replayed = !changed
			if !changed {
				stored, found, err := importer.store.GetWeChatShopOrderMaterial(tx, adapted.Material.ProviderOrderID)
				if err != nil || !found || !sameLegacyMaterial(stored, *adapted.Material) {
					return ErrConflict
				}
			}
			return importer.journal.Record(tx, TerminalReceipt{
				SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC,
				Disposition: "import", TargetID: adapted.Material.ProviderOrderID,
				TargetDigest: adapted.Material.EvidenceDigest,
			})
		})
		if err != nil {
			return err
		}
		if adapted.Material != nil {
			result.Imported++
		} else {
			result.Quarantined++
		}
		if replayed {
			result.Replayed++
		}
		return nil
	})
	return result, err
}

func sameLegacyMaterial(left, right orderport.WeChatShopOrderMaterial) bool {
	return left.ProviderOrderID == right.ProviderOrderID && left.EvidenceDigest == right.EvidenceDigest &&
		left.SourceKeyDigest == right.SourceKeyDigest && left.Source == orderport.WeChatShopMaterialLegacyRaw &&
		!left.ProviderVerified && left.Readiness == orderport.WeChatShopMaterialProviderSyncRequired
}
