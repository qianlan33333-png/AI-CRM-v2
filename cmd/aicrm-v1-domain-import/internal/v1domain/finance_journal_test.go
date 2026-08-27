package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

func TestFinanceJournalStoresFieldDigestAndRejectsSealedQuarantine(t *testing.T) {
	orders, refunds := newFinanceTerminalFake(), newFinanceTerminalFake()
	journal, err := newFinanceJournal(orders, refunds)
	if err != nil {
		t.Fatal(err)
	}
	receipt := orderport.HistoricalImportReceipt{HistoricalFact: orderport.HistoricalFact{
		SourceKeyDigest: financeDigest(1), PayloadDigest: financeDigest(2), FieldDigest: financeDigest(3),
	}, TargetID: 41, TargetDigest: financeDigest(4)}
	if err := journal.AppendHistoricalOrderReceipt(context.Background(), financeOrderKind, receipt); err != nil {
		t.Fatal(err)
	}
	stored, found, err := journal.FindHistoricalOrderReceipt(context.Background(), financeOrderKind, receipt.SourceKeyDigest)
	if err != nil || !found || stored != receipt {
		t.Fatalf("stored/found/error = %#v/%v/%v", stored, found, err)
	}
	terminal := orders.values[SourceIdentifier(receipt.SourceKeyDigest)]
	if value, ok := terminal.Metadata["field_digest"].(string); !ok || value == "" {
		t.Fatalf("terminal metadata = %#v", terminal.Metadata)
	}
	if err := journal.Record(context.Background(), financeRefundKind, TerminalReceipt{
		SourceKeyDigest: financeDigest(9), PayloadDigest: financeDigest(10), Disposition: "quarantine", Reason: "refund_parent_order_unavailable", Metadata: fieldDigestMetadata(financeDigest(11)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := journal.FindHistoricalOrderReceipt(context.Background(), financeRefundKind, financeDigest(9)); !errors.Is(err, orderport.ErrHistoricalConflict) {
		t.Fatalf("sealed quarantine error = %v", err)
	}
}

type financeTerminalFake struct {
	values map[string]TerminalReceipt
}

func newFinanceTerminalFake() *financeTerminalFake {
	return &financeTerminalFake{values: map[string]TerminalReceipt{}}
}

func (journal *financeTerminalFake) LoadTerminal(_ context.Context, source string) (TerminalReceipt, bool, error) {
	receipt, found := journal.values[source]
	return receipt, found, nil
}

func (journal *financeTerminalFake) Record(_ context.Context, receipt TerminalReceipt) error {
	key := SourceIdentifier(receipt.SourceKeyDigest)
	if found, exists := journal.values[key]; exists {
		if !sameFinanceTerminal(found, receipt) {
			return ErrConflict
		}
		return nil
	}
	journal.values[key] = receipt
	return nil
}

func financeDigest(first byte) (digest [sha256.Size]byte) {
	digest[0] = first
	return digest
}
