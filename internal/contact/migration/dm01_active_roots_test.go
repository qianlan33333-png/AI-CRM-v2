package migration

import (
	"errors"
	"testing"
)

func TestActiveRootProcessorRequiresMappedCustomerFor314(t *testing.T) {
	p := ActiveRootProcessor{Receipts: &receiptStore{rows: map[string]RowReceipt{}}}
	if err := p.RequireCustomerRoot(0); !errors.Is(err, ErrMissingCustomerRoot) {
		t.Fatalf("missing root = %v", err)
	}
	if err := p.RequireCustomerRoot(3); err != nil {
		t.Fatalf("mapped root = %v", err)
	}
}
