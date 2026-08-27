package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var _ contactport.HistoricalChannelJournal = (*Journal)(nil)

func (journal *Journal) validChannelDefinitionScope() bool {
	return journal != nil && journal.tx != nil && journal.scope.valid() &&
		journal.scope.AdapterID == v1archive.DefaultAdapterID && journal.scope.TableID == "public/automation_channel" &&
		journal.scope.TargetDomain == "contact" && journal.scope.TargetTable == "channels"
}

func (journal *Journal) LoadHistoricalChannel(ctx context.Context, source string) (contactport.HistoricalChannelReceipt, bool, error) {
	if !journal.validChannelDefinitionScope() {
		return contactport.HistoricalChannelReceipt{}, false, ErrInvalidScope
	}
	terminal, found, err := journal.LoadTerminal(ctx, source)
	if err != nil || !found {
		return contactport.HistoricalChannelReceipt{}, found, err
	}
	receipt, err := channelReceiptFromTerminal(source, terminal)
	return receipt, err == nil, err
}

func (journal *Journal) RecordHistoricalChannel(ctx context.Context, receipt contactport.HistoricalChannelReceipt) error {
	if !journal.validChannelDefinitionScope() {
		return ErrInvalidScope
	}
	source, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || source == [32]byte{} || receipt.PayloadDigest == [32]byte{} || receipt.TargetDigest == [32]byte{} || receipt.TargetID < 1 || receipt.Replayed {
		return ErrInvalidScope
	}
	return journal.Record(ctx, TerminalReceipt{SourceKeyDigest: source, PayloadDigest: receipt.PayloadDigest,
		Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest})
}

func channelReceiptFromTerminal(source string, terminal TerminalReceipt) (contactport.HistoricalChannelReceipt, error) {
	key, err := ParseSourceIdentifier(source)
	id, idErr := positiveID(terminal.TargetID)
	if err != nil || idErr != nil || strconv.FormatInt(id, 10) != terminal.TargetID || key == [sha256.Size]byte{} || key != terminal.SourceKeyDigest ||
		terminal.Disposition != "import" || terminal.Reason != "" || len(terminal.Metadata) != 0 ||
		terminal.PayloadDigest == [sha256.Size]byte{} || terminal.TargetDigest == [sha256.Size]byte{} {
		return contactport.HistoricalChannelReceipt{}, ErrConflict
	}
	return contactport.HistoricalChannelReceipt{SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest,
		TargetID: id, TargetDigest: terminal.TargetDigest}, nil
}
