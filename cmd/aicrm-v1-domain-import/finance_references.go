package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1finance"
	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productstore "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store"
)

// Only immutable migration provenance may supply V2 foreign keys. An absent
// source relation remains NULL; a conflicting existing relation aborts import.
type financeReferenceResolver struct {
	uow        *platformstore.UnitOfWork
	archiveRun string
	dm01Run    int64
	dm01Key    []byte
	contacts   contactport.HistoricalImportTarget
	products   productport.Reader
}

func newFinanceReferenceResolver(ctx context.Context, uow *platformstore.UnitOfWork, archiveRun string, dm01Run int64, key []byte) (*financeReferenceResolver, error) {
	if ctx == nil || uow == nil || archiveRun == "" || dm01Run < 1 || len(key) < sha256.Size {
		return nil, v1domain.ErrInvalidScope
	}
	r := &financeReferenceResolver{uow: uow, archiveRun: archiveRun, dm01Run: dm01Run,
		dm01Key: append([]byte(nil), key...), contacts: contactstore.HistoricalImportRepository{}, products: productstore.NewCatalogRepository()}
	err := uow.Within(ctx, func(ctx context.Context) error {
		return v1domain.VerifyFinanceReferencePrerequisites(ctx, archiveRun, dm01Run)
	})
	return r, err
}

func (r *financeReferenceResolver) ResolveHistoricalOrderReferences(ctx context.Context, fact v1finance.OrderFact) (customerID, productID *int64, err error) {
	if r == nil || r.uow == nil {
		return nil, nil, v1domain.ErrInvalidScope
	}
	err = r.uow.Within(ctx, func(ctx context.Context) error {
		customerID, productID = nil, nil
		var e error
		if fact.UnionID != "" {
			customerID, e = r.customer(ctx, fact.UnionID)
			if e != nil {
				return e
			}
		}
		productID, e = r.product(ctx, fact.Product.Value)
		return e
	})
	if err != nil {
		return nil, nil, err
	}
	return customerID, productID, nil
}

func (r *financeReferenceResolver) customer(ctx context.Context, unionID string) (*int64, error) {
	key, err := contactmigration.SourceKeyHMAC(r.dm01Key, "crm_user_identity", unionID)
	if err != nil {
		return nil, nil
	}
	source := contactport.HistoricalImportCustomerIdentity
	if err = r.contacts.LockHistoricalImportSource(ctx, source, key); err != nil {
		return nil, err
	}
	lineage, found, err := r.contacts.LockHistoricalImportLineage(ctx, source, key)
	if err != nil || !found {
		return nil, err
	}
	if lineage.TargetID < 1 || lineage.LastRunID != r.dm01Run || len(lineage.PayloadHMAC) != sha256.Size || len(lineage.FieldDigest) != sha256.Size {
		return nil, v1domain.ErrConflict
	}
	receipt, found, err := r.contacts.FindHistoricalImportRowReceipt(ctx, r.dm01Run, source, key)
	if err != nil {
		return nil, err
	}
	if !found || receipt.Disposition != contactport.HistoricalImportImported || !hmac.Equal(receipt.PayloadHMAC, lineage.PayloadHMAC) || !hmac.Equal(receipt.FieldDigest, lineage.FieldDigest) {
		return nil, v1domain.ErrConflict
	}
	if _, err = r.contacts.LockHistoricalImportCustomerTarget(ctx, lineage.TargetID); err != nil {
		return nil, err
	}
	if err = r.contacts.ValidateHistoricalImportCustomerRoot(ctx, lineage.TargetID); err != nil {
		return nil, err
	}
	return &lineage.TargetID, nil
}

func (r *financeReferenceResolver) product(ctx context.Context, code string) (*int64, error) {
	idText, payload, digest, metadata, found, err := v1domain.LoadFinanceProductReference(ctx, r.archiveRun, code)
	if err != nil || !found {
		return nil, err
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id < 1 || strconv.FormatInt(id, 10) != idText || len(payload) != sha256.Size {
		return nil, v1domain.ErrConflict
	}
	want := sha256.Sum256([]byte("product\x00products\x00" + idText + "\x00" + hex.EncodeToString(payload)))
	if !hmac.Equal(want[:], digest) {
		return nil, v1domain.ErrConflict
	}
	product, err := r.products.ReadProduct(ctx, productport.ID(id))
	if err != nil {
		return nil, err
	}
	if !financeProductMatches(product, code, metadata) {
		return nil, v1domain.ErrConflict
	}
	return &id, nil
}

func financeProductMatches(product productport.Product, code string, metadata []byte) bool {
	var expected struct {
		Code     string `json:"target_product_code"`
		Name     string `json:"target_product_name"`
		Price    string `json:"price_minor"`
		Currency string `json:"currency"`
		Actor    string `json:"created_by"`
	}
	return json.Unmarshal(metadata, &expected) == nil && product.ID > 0 && product.ProductCode == code && code == expected.Code &&
		product.Name == expected.Name && strconv.FormatInt(product.PriceMinor, 10) == expected.Price &&
		product.Currency == "CNY" && product.Currency == expected.Currency && strconv.FormatInt(product.CreatedBy, 10) == expected.Actor &&
		product.LocalLifecycle == productport.LocalProductDisabled && product.Version == 1 && product.StockQuantity == 0
}
