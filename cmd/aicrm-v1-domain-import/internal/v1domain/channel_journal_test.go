package v1domain

import (
	"errors"
	"testing"
)

func TestChannelReceiptRequiresExactTerminal(t *testing.T) {
	row := TerminalReceipt{SourceKeyDigest: [32]byte{1}, PayloadDigest: [32]byte{2},
		Disposition: "import", TargetID: "7", TargetDigest: [32]byte{3}}
	source := SourceIdentifier(row.SourceKeyDigest)
	got, err := channelReceiptFromTerminal(source, row)
	if err != nil || got.TargetID != 7 || got.SourceIdentifier != source || got.TargetDigest != row.TargetDigest {
		t.Fatalf("receipt/error = %+v/%v", got, err)
	}
	for _, mutate := range []func(*TerminalReceipt){
		func(r *TerminalReceipt) { r.Disposition = "archive" },
		func(r *TerminalReceipt) { r.TargetID = "007" },
		func(r *TerminalReceipt) { r.SourceKeyDigest = [32]byte{9} },
		func(r *TerminalReceipt) { r.TargetDigest = [32]byte{} },
		func(r *TerminalReceipt) { r.Reason = "unexpected" },
		func(r *TerminalReceipt) { r.Metadata = map[string]any{"unexpected": true} },
	} {
		bad := row
		mutate(&bad)
		if _, err := channelReceiptFromTerminal(source, bad); !errors.Is(err, ErrConflict) {
			t.Fatalf("invalid terminal accepted: %v", err)
		}
	}
}
