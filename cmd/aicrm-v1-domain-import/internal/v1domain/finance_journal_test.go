package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
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

func TestNewFinanceJournalBindsExactPairAndArchiveRun(t *testing.T) {
	orders := financeScopedJournal(financeOrdersTableID, "order_list_projections", "v1-finance-a1", "archive-run")
	refunds := financeScopedJournal(financeRefundsTableID, "order_historical_refunds", "v1-finance-a1", "archive-run")
	journal, err := NewFinanceJournal(orders, refunds)
	if err != nil || journal.validateArchiveRun("archive-run") != nil || !errors.Is(journal.validateArchiveRun("other-run"), ErrConflict) {
		t.Fatalf("journal/errors = %#v/%v", journal, err)
	}
	if _, err = NewFinanceJournal(nil, refunds); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("nil orders error = %v", err)
	}
	badTarget := financeScopedJournal(financeRefundsTableID, "order_list_projections", "v1-finance-a1", "archive-run")
	if _, err = NewFinanceJournal(orders, badTarget); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("target error = %v", err)
	}
	mismatchedRun := financeScopedJournal(financeRefundsTableID, "order_historical_refunds", "v1-finance-a1", "another-run")
	if _, err = NewFinanceJournal(orders, mismatchedRun); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("run error = %v", err)
	}
	mismatchedVersion := financeScopedJournal(financeRefundsTableID, "order_historical_refunds", "v1-finance-a2", "archive-run")
	if _, err = NewFinanceJournal(orders, mismatchedVersion); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("version error = %v", err)
	}
	wrongAdapter := financeScopedJournal(financeRefundsTableID, "order_historical_refunds", "v1-finance-a1", "archive-run")
	wrongAdapter.scope.AdapterID = "another_adapter"
	if _, err = NewFinanceJournal(orders, wrongAdapter); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("adapter error = %v", err)
	}
}

type financeTerminalFake struct {
	values                     map[string]TerminalReceipt
	recordErrOnce              error
	removeReceiptOnRecordError bool
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
	if journal.recordErrOnce != nil {
		err := journal.recordErrOnce
		journal.recordErrOnce = nil
		if journal.removeReceiptOnRecordError {
			delete(journal.values, key)
		}
		return err
	}
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

func financeScopedJournal(tableID, targetTable, version, run string) *Journal {
	return &Journal{scope: Scope{ImportVersion: version, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: tableID, TargetDomain: "order", TargetTable: targetTable}, tx: func(context.Context) (pgx.Tx, error) {
		return nil, nil
	}}
}
