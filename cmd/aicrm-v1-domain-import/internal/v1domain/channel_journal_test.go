package v1domain

import (
	"context"
	"errors"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
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

func TestChannelRelationJournalScopesCannotCross(t *testing.T) {
	makeJournal := func(table, target string) *Journal {
		j, err := NewJournal(Scope{ImportVersion: "v1-channel-a1", ArchiveRunID: "archive-run", AdapterID: v1archive.DefaultAdapterID, TableID: "public/" + table, TargetDomain: "contact", TargetTable: target})
		if err != nil {
			t.Fatal(err)
		}
		return j
	}
	contacts := makeJournal("automation_channel_contact", "channel_historical_contacts")
	assignees := makeJournal("automation_channel_assignee", "channel_historical_assignees")
	j, err := NewChannelRelationsJournal(contacts, assignees)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := j.scope("contacts"); err != nil || got != contacts {
		t.Fatal("contacts scope")
	}
	if got, err := j.scope("assignees"); err != nil || got != assignees {
		t.Fatal("assignees scope")
	}
	if _, _, err := j.LoadHistoricalChannelRelation(context.Background(), "other", "anything"); !errors.Is(err, ErrInvalidScope) {
		t.Fatal("unknown scope allowed")
	}
	if _, err := NewChannelRelationsJournal(assignees, contacts); !errors.Is(err, ErrInvalidScope) {
		t.Fatal("crossed scope allowed")
	}
	assignees.scope.ArchiveRunID = "other"
	if _, err := NewChannelRelationsJournal(contacts, assignees); !errors.Is(err, ErrInvalidScope) {
		t.Fatal("crossed run allowed")
	}
}
