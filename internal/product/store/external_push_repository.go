package store

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

var _ productapp.CommerceExternalPushStore = (*CatalogRepository)(nil)

func (r *CatalogRepository) ReadCommerceExternalPushConfiguration(ctx context.Context, id productport.ID, kind productport.ExternalPushProductKind) (productport.ExternalPushConfiguration, error) {
	if r == nil || id < 1 || !validExternalPushKind(kind) {
		return productport.ExternalPushConfiguration{}, productapp.ErrNotFound
	}
	if _, err := r.externalPushProduct(ctx, id, kind, false); err != nil {
		return productport.ExternalPushConfiguration{}, err
	}
	q, err := queries(ctx)
	if err != nil {
		return productport.ExternalPushConfiguration{}, unavailable(err)
	}
	row, err := q.ReadProductExternalPushConfiguration(ctx, productdb.ReadProductExternalPushConfigurationParams{ProductID: int64(id), ProductKind: string(kind)})
	if err != nil {
		return productport.ExternalPushConfiguration{}, unavailable(err)
	}
	value, err := externalPushConfiguration(row.ProductID, row.ProductKind, row.Enabled, row.ConfigurationReference, row.UpdatedAt.Time)
	if err != nil || value.ProductKind != kind {
		return productport.ExternalPushConfiguration{}, productapp.ErrUnavailable
	}
	return value, nil
}

func (r *CatalogRepository) LockCommerceExternalPushConfiguration(ctx context.Context, id productport.ID, kind productport.ExternalPushProductKind) (productport.ExternalPushConfiguration, error) {
	if r == nil || id < 1 || !validExternalPushKind(kind) {
		return productport.ExternalPushConfiguration{}, productapp.ErrNotFound
	}
	if _, err := r.externalPushProduct(ctx, id, kind, true); err != nil {
		return productport.ExternalPushConfiguration{}, err
	}
	q, err := queries(ctx)
	if err != nil {
		return productport.ExternalPushConfiguration{}, unavailable(err)
	}
	row, err := q.LockProductExternalPushConfiguration(ctx, productdb.LockProductExternalPushConfigurationParams{ProductID: int64(id), ProductKind: string(kind)})
	if err != nil {
		return productport.ExternalPushConfiguration{}, unavailable(err)
	}
	value, err := externalPushConfiguration(row.ProductID, row.ProductKind, row.Enabled, row.ConfigurationReference, row.UpdatedAt.Time)
	if err != nil || value.ProductKind != kind {
		return productport.ExternalPushConfiguration{}, productapp.ErrUnavailable
	}
	return value, nil
}

func (r *CatalogRepository) SaveCommerceExternalPushConfiguration(ctx context.Context, value productport.ExternalPushConfiguration, now time.Time) (productport.ExternalPushConfiguration, error) {
	if r == nil || value.ProductID < 1 || !validExternalPushKind(value.ProductKind) || now.IsZero() {
		return productport.ExternalPushConfiguration{}, productapp.ErrUnavailable
	}
	if _, err := r.externalPushProduct(ctx, value.ProductID, value.ProductKind, true); err != nil {
		return productport.ExternalPushConfiguration{}, err
	}
	q, err := queries(ctx)
	if err != nil {
		return productport.ExternalPushConfiguration{}, unavailable(err)
	}
	row, err := q.SaveProductExternalPushConfiguration(ctx, productdb.SaveProductExternalPushConfigurationParams{
		ProductID: int64(value.ProductID), ProductKind: string(value.ProductKind), Enabled: value.Enabled,
		ConfigurationReference: value.ConfigurationReference, UpdatedAt: stamp(now),
	})
	if err != nil {
		return productport.ExternalPushConfiguration{}, unavailable(err)
	}
	return externalPushConfiguration(row.ProductID, row.ProductKind, row.Enabled, row.ConfigurationReference, row.UpdatedAt.Time)
}

func (r *CatalogRepository) ReserveCommerceExternalPush(ctx context.Context, value productapp.Reservation) (productapp.Receipt, bool, error) {
	return r.Reserve(ctx, value)
}

func (r *CatalogRepository) CompleteCommerceExternalPush(ctx context.Context, id int64, snapshot json.RawMessage, now time.Time) (productapp.Receipt, error) {
	return r.Complete(ctx, id, snapshot, now)
}

func (r *CatalogRepository) CreateCommerceExternalPushTest(ctx context.Context, value productport.ExternalPushTest, configurationDigest [32]byte, receiptID int64) (productport.ExternalPushTest, error) {
	if r == nil || value.ProductID < 1 || !validExternalPushKind(value.ProductKind) || receiptID < 1 ||
		(value.State != "accepted" && value.State != "queued") || value.ProviderAccepted || value.DeliveryProven ||
		value.RealExternalCallExecuted || value.AutoRetryAllowed || value.CreatedAt.IsZero() {
		return productport.ExternalPushTest{}, productapp.ErrUnavailable
	}
	effectID, err := productExternalPushEffectID(value.EffectID)
	if err != nil {
		return productport.ExternalPushTest{}, productapp.ErrUnavailable
	}
	q, err := queries(ctx)
	if err != nil {
		return productport.ExternalPushTest{}, unavailable(err)
	}
	row, err := q.CreateProductExternalPushTestBinding(ctx, productdb.CreateProductExternalPushTestBindingParams{
		ProductID: int64(value.ProductID), ProductKind: string(value.ProductKind), OperationReceiptID: receiptID,
		ConfigurationDigest: configurationDigest[:], State: value.State, CreatedAt: stamp(value.CreatedAt), ExternalEffectID: effectID,
	})
	if err != nil {
		return productport.ExternalPushTest{}, unavailable(err)
	}
	return productport.ExternalPushTest{
		ProductID: productport.ID(row.ProductID), ProductKind: productport.ExternalPushProductKind(row.ProductKind),
		EffectID: "eer_" + strconv.FormatInt(row.ExternalEffectID, 10), State: row.State,
		ProviderAccepted: row.ProviderAccepted, DeliveryProven: row.DeliveryProven,
		RealExternalCallExecuted: row.RealExternalCallExecuted, AutoRetryAllowed: row.AutoRetryAllowed,
		CreatedAt: row.CreatedAt.Time,
	}, nil
}

func (r *CatalogRepository) externalPushProduct(ctx context.Context, id productport.ID, kind productport.ExternalPushProductKind, lock bool) (productport.Product, error) {
	if kind == productport.ExternalPushWeChatPay {
		if lock {
			return r.GetForUpdate(ctx, id)
		}
		return r.Get(ctx, id)
	}
	if lock {
		return r.GetServicePeriodProductForUpdate(ctx, id)
	}
	return r.GetServicePeriodProduct(ctx, id)
}

func externalPushConfiguration(productID int64, kind string, enabled bool, reference string, updatedAt time.Time) (productport.ExternalPushConfiguration, error) {
	value := productport.ExternalPushConfiguration{
		ProductID: productport.ID(productID), ProductKind: productport.ExternalPushProductKind(kind), Enabled: enabled,
		ConfigurationReference: reference, UpdatedAt: updatedAt,
	}
	if productID < 1 || !validExternalPushKind(value.ProductKind) || updatedAt.IsZero() ||
		(enabled && reference == "") || (!enabled && reference != "") {
		return productport.ExternalPushConfiguration{}, productapp.ErrUnavailable
	}
	return value, nil
}

func validExternalPushKind(kind productport.ExternalPushProductKind) bool {
	return kind == productport.ExternalPushWeChatPay || kind == productport.ExternalPushServicePeriod
}

func productExternalPushEffectID(value string) (int64, error) {
	const prefix = "eer_"
	if !strings.HasPrefix(value, prefix) || len(value) == len(prefix) || value[len(prefix)] == '0' {
		return 0, productapp.ErrUnavailable
	}
	for _, r := range value[len(prefix):] {
		if r < '0' || r > '9' {
			return 0, productapp.ErrUnavailable
		}
	}
	id, err := strconv.ParseInt(value[len(prefix):], 10, 64)
	if err != nil || id < 1 {
		return 0, productapp.ErrUnavailable
	}
	return id, nil
}
