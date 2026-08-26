package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	orderdb "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store/generated"
)

type WeChatShopMaterialRepository struct{}

var _ orderapp.WeChatShopMaterialStore = (*WeChatShopMaterialRepository)(nil)

func NewWeChatShopMaterialRepository() *WeChatShopMaterialRepository {
	return &WeChatShopMaterialRepository{}
}

func (repository *WeChatShopMaterialRepository) UpsertWeChatShopOrderMaterial(ctx context.Context, material orderport.WeChatShopOrderMaterial) (bool, error) {
	queries, err := transactionQueries(ctx)
	if repository == nil || err != nil {
		return false, weChatShopMaterialUnavailable(err)
	}
	row, err := queries.UpsertWeChatShopOrderMaterial(ctx, orderdb.UpsertWeChatShopOrderMaterialParams{
		ProviderOrderID: material.ProviderOrderID, StatusCode: material.StatusCode,
		DealRecorded: material.DealRecorded, AmountMinor: material.AmountMinor,
		Currency: material.Currency, TransactionDigest: optionalDigest(material.TransactionDigest),
		EvidenceDigest: material.EvidenceDigest[:], Source: string(material.Source),
		SourceKeyDigest: optionalDigest(material.SourceKeyDigest), Readiness: string(material.Readiness),
		ProviderVerified: material.ProviderVerified, ProviderCreatedAt: optionalTimestamp(material.CreatedAt),
		ProviderPaidAt: optionalTimestamp(material.PaidAt), ProviderUpdatedAt: optionalTimestamp(material.UpdatedAt),
		SyncedAt: optionalTimestamp(material.SyncedAt),
	})
	if err != nil || row.ID < 1 || row.Version < 1 {
		return false, weChatShopMaterialUnavailable(err)
	}
	if !row.Changed {
		if err = completeWeChatShopMaterialSyncRequest(ctx, queries, material); err != nil {
			return false, err
		}
		return false, nil
	}
	if err = queries.DeleteWeChatShopOrderMaterialLines(ctx, row.ID); err != nil {
		return false, weChatShopMaterialUnavailable(err)
	}
	for _, line := range material.Lines {
		onAfterSale, finishAfterSale, remaining := pgtype.Int8{}, pgtype.Int8{}, pgtype.Int8{}
		if line.AfterSaleEvidenceExact {
			onAfterSale = pgtype.Int8{Int64: line.OnAfterSaleSKUCount, Valid: true}
			finishAfterSale = pgtype.Int8{Int64: line.FinishAfterSaleSKUCount, Valid: true}
			remaining = pgtype.Int8{Int64: line.RemainingSKUCount, Valid: true}
		}
		err = queries.InsertWeChatShopOrderMaterialLine(ctx, orderdb.InsertWeChatShopOrderMaterialLineParams{
			MaterialID: row.ID, Position: int32(line.Position), ProductID: line.ProductID,
			SkuID: line.SKUID, SkuCount: line.SKUCount, OnAftersaleSkuCount: onAfterSale,
			FinishAftersaleSkuCount: finishAfterSale, RealPriceMinor: line.RealPriceMinor,
			RemainingSkuCount: remaining, AftersaleEvidenceExact: line.AfterSaleEvidenceExact,
			Readiness: string(line.Readiness),
		})
		if err != nil {
			return false, weChatShopMaterialUnavailable(err)
		}
	}
	if err = completeWeChatShopMaterialSyncRequest(ctx, queries, material); err != nil {
		return false, err
	}
	return true, nil
}

func completeWeChatShopMaterialSyncRequest(ctx context.Context, queries *orderdb.Queries, material orderport.WeChatShopOrderMaterial) error {
	if material.Source != orderport.WeChatShopMaterialProvider || !material.ProviderVerified {
		return nil
	}
	exists, err := queries.WeChatShopMaterialSyncRequestTableExists(ctx)
	if err != nil {
		return weChatShopMaterialUnavailable(err)
	}
	if !exists {
		return nil
	}
	_, err = queries.CompleteWeChatShopMaterialSyncRequest(ctx, orderdb.CompleteWeChatShopMaterialSyncRequestParams{
		EvidenceDigest: material.EvidenceDigest[:], CompletedAt: optionalTimestamp(material.SyncedAt),
		ProviderOrderID: material.ProviderOrderID,
	})
	if err != nil {
		return weChatShopMaterialUnavailable(err)
	}
	return nil
}

func (repository *WeChatShopMaterialRepository) GetWeChatShopOrderMaterial(ctx context.Context, providerOrderID string) (orderport.WeChatShopOrderMaterial, bool, error) {
	queries, err := transactionQueries(ctx)
	if repository == nil || err != nil {
		return orderport.WeChatShopOrderMaterial{}, false, weChatShopMaterialUnavailable(err)
	}
	row, err := queries.GetWeChatShopOrderMaterial(ctx, providerOrderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return orderport.WeChatShopOrderMaterial{}, false, nil
	}
	if err != nil {
		return orderport.WeChatShopOrderMaterial{}, false, weChatShopMaterialUnavailable(err)
	}
	lines, err := queries.ListWeChatShopOrderMaterialLines(ctx, row.ID)
	if err != nil {
		return orderport.WeChatShopOrderMaterial{}, false, weChatShopMaterialUnavailable(err)
	}
	material, err := mapWeChatShopOrderMaterial(row, lines)
	if err != nil {
		return orderport.WeChatShopOrderMaterial{}, false, weChatShopMaterialUnavailable(err)
	}
	return material, true, nil
}

func (repository *WeChatShopMaterialRepository) RecordWeChatShopLegacyQuarantine(ctx context.Context, value orderapp.WeChatShopLegacyQuarantine) (bool, error) {
	queries, err := transactionQueries(ctx)
	if repository == nil || err != nil {
		return false, weChatShopMaterialUnavailable(err)
	}
	params := orderdb.RecordWeChatShopLegacyQuarantineParams{
		SourceTable: value.SourceTable, SourceKeyDigest: value.SourceKeyDigest[:],
		PayloadDigest: value.PayloadDigest[:], ReasonCode: value.ReasonCode,
		RecordedAt: optionalTimestamp(value.RecordedAt),
	}
	if _, err = queries.RecordWeChatShopLegacyQuarantine(ctx, params); err == nil {
		return true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, weChatShopMaterialUnavailable(err)
	}
	existing, err := queries.GetWeChatShopLegacyQuarantine(ctx, orderdb.GetWeChatShopLegacyQuarantineParams{
		SourceTable: value.SourceTable, SourceKeyDigest: value.SourceKeyDigest[:], PayloadDigest: value.PayloadDigest[:],
	})
	if err != nil {
		return false, weChatShopMaterialUnavailable(err)
	}
	if existing.ReasonCode != value.ReasonCode {
		return false, orderport.ErrWeChatShopMaterialConflict
	}
	return false, nil
}

func mapWeChatShopOrderMaterial(row orderdb.OrderWechatShopMaterial, rows []orderdb.OrderWechatShopMaterialLine) (orderport.WeChatShopOrderMaterial, error) {
	evidence, valid := requiredDigest(row.EvidenceDigest)
	if !valid || row.ID < 1 || row.Version < 1 || !row.SyncedAt.Valid || !row.CreatedAt.Valid || !row.UpdatedAt.Valid || len(rows) == 0 {
		return orderport.WeChatShopOrderMaterial{}, orderport.ErrWeChatShopMaterialUnavailable
	}
	transaction, valid := nullableDigest(row.TransactionDigest)
	if !valid {
		return orderport.WeChatShopOrderMaterial{}, orderport.ErrWeChatShopMaterialUnavailable
	}
	sourceKey, valid := nullableDigest(row.SourceKeyDigest)
	if !valid {
		return orderport.WeChatShopOrderMaterial{}, orderport.ErrWeChatShopMaterialUnavailable
	}
	material := orderport.WeChatShopOrderMaterial{
		ProviderOrderID: row.ProviderOrderID, StatusCode: row.StatusCode,
		DealRecorded: row.DealRecorded, AmountMinor: row.AmountMinor, Currency: row.Currency,
		TransactionDigest: transaction, EvidenceDigest: evidence, SourceKeyDigest: sourceKey,
		Source: orderport.WeChatShopMaterialSource(row.Source), Readiness: orderport.WeChatShopMaterialReadiness(row.Readiness),
		ProviderVerified: row.ProviderVerified, CreatedAt: timeValueOrZero(row.ProviderCreatedAt),
		PaidAt: timeValueOrZero(row.ProviderPaidAt), UpdatedAt: timeValueOrZero(row.ProviderUpdatedAt),
		SyncedAt: row.SyncedAt.Time.UTC(),
	}
	material.Lines = make([]orderport.WeChatShopOrderLine, len(rows))
	for index, row := range rows {
		if row.MaterialID < 1 || row.Position < 1 || int(row.Position) != index+1 {
			return orderport.WeChatShopOrderMaterial{}, orderport.ErrWeChatShopMaterialUnavailable
		}
		line := orderport.WeChatShopOrderLine{
			Position: int(row.Position), ProductID: row.ProductID, SKUID: row.SkuID,
			SKUCount: row.SkuCount, RealPriceMinor: row.RealPriceMinor,
			AfterSaleEvidenceExact: row.AftersaleEvidenceExact,
			Readiness:              orderport.WeChatShopLineReadiness(row.Readiness),
		}
		if row.AftersaleEvidenceExact {
			if !row.OnAftersaleSkuCount.Valid || !row.FinishAftersaleSkuCount.Valid || !row.RemainingSkuCount.Valid {
				return orderport.WeChatShopOrderMaterial{}, orderport.ErrWeChatShopMaterialUnavailable
			}
			line.OnAfterSaleSKUCount = row.OnAftersaleSkuCount.Int64
			line.FinishAfterSaleSKUCount = row.FinishAftersaleSkuCount.Int64
			line.RemainingSKUCount = row.RemainingSkuCount.Int64
		} else if row.OnAftersaleSkuCount.Valid || row.FinishAftersaleSkuCount.Valid || row.RemainingSkuCount.Valid {
			return orderport.WeChatShopOrderMaterial{}, orderport.ErrWeChatShopMaterialUnavailable
		}
		material.Lines[index] = line
	}
	return material, nil
}

func optionalDigest(value [32]byte) []byte {
	if value == ([32]byte{}) {
		return nil
	}
	return value[:]
}

func requiredDigest(value []byte) ([32]byte, bool) {
	if len(value) != 32 {
		return [32]byte{}, false
	}
	var result [32]byte
	copy(result[:], value)
	return result, true
}

func nullableDigest(value []byte) ([32]byte, bool) {
	if value == nil {
		return [32]byte{}, true
	}
	return requiredDigest(value)
}

func optionalTimestamp(value time.Time) pgtype.Timestamptz {
	if value.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func timeValueOrZero(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

func weChatShopMaterialUnavailable(err error) error {
	if err == nil {
		err = errors.New("invalid persisted wechat shop material")
	}
	return fmt.Errorf("%w: %v", orderport.ErrWeChatShopMaterialUnavailable, err)
}
