package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

const (
	financeOrderKind  = "orders"
	financeRefundKind = "refunds"
)

// financeTerminalJournal is deliberately private so the production constructor
// still accepts the migration-owned Journal for each target table.
type financeTerminalJournal interface {
	LoadTerminal(context.Context, string) (TerminalReceipt, bool, error)
	Record(context.Context, TerminalReceipt) error
}

// FinanceJournal adapts the two migration-owned receipt journals to the
// Order-owned historical import journal. It persists the field digest as
// immutable receipt metadata, so replay also detects crosswalk drift.
type FinanceJournal struct {
	orders  financeTerminalJournal
	refunds financeTerminalJournal
}

var _ orderport.HistoricalImportJournal = (*FinanceJournal)(nil)

func NewFinanceJournal(orders, refunds *Journal) (*FinanceJournal, error) {
	return newFinanceJournal(orders, refunds)
}

func newFinanceJournal(orders, refunds financeTerminalJournal) (*FinanceJournal, error) {
	if orders == nil || refunds == nil {
		return nil, ErrInvalidScope
	}
	return &FinanceJournal{orders: orders, refunds: refunds}, nil
}

func (journal *FinanceJournal) LoadTerminal(ctx context.Context, kind string, sourceKey [sha256.Size]byte) (TerminalReceipt, bool, error) {
	selected, err := journal.selectJournal(kind)
	if err != nil {
		return TerminalReceipt{}, false, err
	}
	return selected.LoadTerminal(ctx, SourceIdentifier(sourceKey))
}

func (journal *FinanceJournal) Record(ctx context.Context, kind string, receipt TerminalReceipt) error {
	selected, err := journal.selectJournal(kind)
	if err != nil {
		return err
	}
	return selected.Record(ctx, receipt)
}

func (journal *FinanceJournal) FindHistoricalOrderReceipt(ctx context.Context, kind string, sourceKey [sha256.Size]byte) (orderport.HistoricalImportReceipt, bool, error) {
	terminal, found, err := journal.LoadTerminal(ctx, kind, sourceKey)
	if err != nil || !found {
		return orderport.HistoricalImportReceipt{}, found, err
	}
	if terminal.Disposition != "import" || terminal.SourceKeyDigest != sourceKey {
		return orderport.HistoricalImportReceipt{}, false, orderport.ErrHistoricalConflict
	}
	targetID, err := strconv.ParseInt(terminal.TargetID, 10, 64)
	if err != nil || targetID < 1 {
		return orderport.HistoricalImportReceipt{}, false, orderport.ErrHistoricalConflict
	}
	fieldDigest, err := receiptFieldDigest(terminal.Metadata)
	if err != nil {
		return orderport.HistoricalImportReceipt{}, false, orderport.ErrHistoricalConflict
	}
	return orderport.HistoricalImportReceipt{HistoricalFact: orderport.HistoricalFact{
		SourceKeyDigest: terminal.SourceKeyDigest,
		PayloadDigest:   terminal.PayloadDigest,
		FieldDigest:     fieldDigest,
	}, TargetID: targetID, TargetDigest: terminal.TargetDigest}, true, nil
}

func (journal *FinanceJournal) AppendHistoricalOrderReceipt(ctx context.Context, kind string, receipt orderport.HistoricalImportReceipt) error {
	if receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.SourceKeyDigest == ([sha256.Size]byte{}) || receipt.PayloadDigest == ([sha256.Size]byte{}) || receipt.FieldDigest == ([sha256.Size]byte{}) {
		return orderport.ErrHistoricalInput
	}
	return journal.Record(ctx, kind, TerminalReceipt{
		SourceKeyDigest: receipt.SourceKeyDigest,
		PayloadDigest:   receipt.PayloadDigest,
		Disposition:     "import",
		TargetID:        strconv.FormatInt(receipt.TargetID, 10),
		TargetDigest:    receipt.TargetDigest,
		Metadata:        fieldDigestMetadata(receipt.FieldDigest),
	})
}

func (journal *FinanceJournal) selectJournal(kind string) (financeTerminalJournal, error) {
	if journal == nil {
		return nil, ErrInvalidScope
	}
	switch kind {
	case financeOrderKind:
		if journal.orders != nil {
			return journal.orders, nil
		}
	case financeRefundKind:
		if journal.refunds != nil {
			return journal.refunds, nil
		}
	}
	return nil, ErrInvalidScope
}

func fieldDigestMetadata(digest [sha256.Size]byte) map[string]any {
	return map[string]any{"field_digest": hex.EncodeToString(digest[:])}
}

func receiptFieldDigest(metadata map[string]any) ([sha256.Size]byte, error) {
	value, found := metadata["field_digest"].(string)
	if !found || len(value) != hex.EncodedLen(sha256.Size) {
		return [sha256.Size]byte{}, ErrConflict
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return [sha256.Size]byte{}, ErrConflict
	}
	var digest [sha256.Size]byte
	copy(digest[:], decoded)
	if digest == ([sha256.Size]byte{}) {
		return [sha256.Size]byte{}, ErrConflict
	}
	return digest, nil
}
