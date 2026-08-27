package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

func TestFinanceProductReferenceRequiresExactDisabledTarget(t *testing.T) {
	metadata := []byte(`{"target_product_code":"P-1","target_product_name":"历史商品","price_minor":"990","currency":"CNY","created_by":"1"}`)
	product := productport.Product{ID: 8, ProductCode: "P-1", Name: "历史商品", PriceMinor: 990, Currency: "CNY", CreatedBy: 1, Version: 1, LocalLifecycle: productport.LocalProductDisabled}
	if !financeProductMatches(product, "P-1", metadata) {
		t.Fatal("matching disabled product rejected")
	}
	for name, mutate := range map[string]func(*productport.Product){
		"id":       func(p *productport.Product) { p.ID = 0 },
		"code":     func(p *productport.Product) { p.ProductCode = "other" },
		"name":     func(p *productport.Product) { p.Name = "changed" },
		"amount":   func(p *productport.Product) { p.PriceMinor++ },
		"currency": func(p *productport.Product) { p.Currency = "USD" },
		"actor":    func(p *productport.Product) { p.CreatedBy++ },
		"version":  func(p *productport.Product) { p.Version++ },
		"stock":    func(p *productport.Product) { p.StockQuantity++ },
		"enabled":  func(p *productport.Product) { p.LocalLifecycle = productport.LocalProductEnabled },
	} {
		t.Run(name, func(t *testing.T) {
			changed := product
			mutate(&changed)
			if financeProductMatches(changed, "P-1", metadata) {
				t.Fatal("drift accepted")
			}
		})
	}
}

type financeContactTarget struct {
	contactport.HistoricalImportTarget
	lineage   contactport.HistoricalImportLineage
	receipt   contactport.HistoricalImportRowReceipt
	found     bool
	validated int64
}

func (s *financeContactTarget) LockHistoricalImportSource(context.Context, contactport.HistoricalImportSource, []byte) error {
	return nil
}
func (s *financeContactTarget) LockHistoricalImportLineage(context.Context, contactport.HistoricalImportSource, []byte) (contactport.HistoricalImportLineage, bool, error) {
	return s.lineage, s.found, nil
}
func (s *financeContactTarget) FindHistoricalImportRowReceipt(context.Context, int64, contactport.HistoricalImportSource, []byte) (contactport.HistoricalImportRowReceipt, bool, error) {
	return s.receipt, s.found, nil
}
func (s *financeContactTarget) LockHistoricalImportCustomerTarget(context.Context, int64) (contactport.HistoricalImportCustomerFact, error) {
	return contactport.HistoricalImportCustomerFact{}, nil
}
func (s *financeContactTarget) ValidateHistoricalImportCustomerRoot(_ context.Context, id int64) error {
	s.validated = id
	return nil
}

func TestFinanceCustomerReferenceUsesVerifiedRunNotLegacyID(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	store := &financeContactTarget{found: true, lineage: contactport.HistoricalImportLineage{TargetID: 91, LastRunID: 2, PayloadHMAC: key, FieldDigest: key}, receipt: contactport.HistoricalImportRowReceipt{Disposition: contactport.HistoricalImportImported, PayloadHMAC: key, FieldDigest: key}}
	r := &financeReferenceResolver{dm01Key: key, dm01Run: 2, contacts: store}
	id, err := r.customer(context.Background(), "source-unionid")
	if err != nil || id == nil || *id != 91 || store.validated != 91 {
		t.Fatalf("id=%v err=%v", id, err)
	}
	store.found = false
	if id, err = r.customer(context.Background(), "unmapped"); err != nil || id != nil {
		t.Fatal("missing relation must remain unlinked")
	}
	store.found = true
	store.lineage.LastRunID = 3
	if _, err = r.customer(context.Background(), "source-unionid"); !errors.Is(err, v1domain.ErrConflict) {
		t.Fatal("wrong import run accepted")
	}
	store.lineage.LastRunID = 2
	store.receipt.FieldDigest = bytes.Repeat([]byte{2}, 32)
	if _, err = r.customer(context.Background(), "source-unionid"); !errors.Is(err, v1domain.ErrConflict) {
		t.Fatal("receipt drift accepted")
	}
}
