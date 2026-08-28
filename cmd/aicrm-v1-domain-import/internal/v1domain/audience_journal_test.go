package v1domain

import (
	"context"
	"errors"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

func audienceTestJournals(t *testing.T) map[string]*Journal {
	t.Helper()
	result := map[string]*Journal{}
	for _, scope := range audienceHistoryScopes {
		j, err := NewJournal(Scope{ImportVersion: "v1-audience-history-a1", ArchiveRunID: "test-archive",
			AdapterID: v1archive.DefaultAdapterID, TableID: scope.source, TargetDomain: "segment", TargetTable: scope.target})
		if err != nil {
			t.Fatal(err)
		}
		result[scope.source] = j
	}
	return result
}

func TestAudienceHistoryJournalScopes(t *testing.T) {
	for _, scope := range audienceHistoryScopes {
		t.Run(scope.kind, func(t *testing.T) {
			journals := audienceTestJournals(t)
			j, err := NewAudienceHistoryJournal(journals)
			if err != nil {
				t.Fatal(err)
			}
			got, err := j.scope(scope.kind)
			if err != nil || got != journals[scope.source] {
				t.Fatal("wrong source journal")
			}
			delete(journals, scope.source)
			if _, err = NewAudienceHistoryJournal(journals); !errors.Is(err, ErrInvalidScope) {
				t.Fatal("incomplete scopes accepted")
			}
			if _, err = j.scope(scope.kind); err != nil {
				t.Fatal("caller map mutation affected journal")
			}
		})
	}
	for _, mutate := range []func(*Scope){
		func(s *Scope) { s.ImportVersion = "different" }, func(s *Scope) { s.ArchiveRunID = "different" },
		func(s *Scope) { s.AdapterID = "different" }, func(s *Scope) { s.TableID = "public/segments" },
		func(s *Scope) { s.TargetDomain = "contact" }, func(s *Scope) { s.TargetTable = "segments" },
	} {
		journals := audienceTestJournals(t)
		mutate(&journals[audienceHistoryScopes[0].source].scope)
		if _, err := NewAudienceHistoryJournal(journals); !errors.Is(err, ErrInvalidScope) {
			t.Fatal("crossed scope accepted")
		}
	}
	j, _ := NewAudienceHistoryJournal(audienceTestJournals(t))
	if _, _, err := j.LoadAudienceHistory(context.Background(), "other", "source"); !errors.Is(err, ErrInvalidScope) {
		t.Fatal("unknown kind accepted")
	}
}

func TestAudienceHistoryReceiptTerminal(t *testing.T) {
	terminal := TerminalReceipt{SourceKeyDigest: [32]byte{1}, PayloadDigest: [32]byte{2},
		Disposition: "import", TargetID: "7", TargetDigest: [32]byte{3}}
	source := SourceIdentifier(terminal.SourceKeyDigest)
	got, err := audienceHistoryReceiptFromTerminal(source, terminal)
	if err != nil || got.SourceIdentifier != source || got.TargetID != 7 || got.Replayed || got.TargetDigest != terminal.TargetDigest {
		t.Fatal("valid terminal rejected")
	}
	for _, mutate := range []func(*TerminalReceipt){
		func(r *TerminalReceipt) { r.Disposition = "archive" }, func(r *TerminalReceipt) { r.TargetID = "007" },
		func(r *TerminalReceipt) { r.SourceKeyDigest = [32]byte{9} }, func(r *TerminalReceipt) { r.PayloadDigest = [32]byte{} },
		func(r *TerminalReceipt) { r.TargetDigest = [32]byte{} }, func(r *TerminalReceipt) { r.Reason = "unexpected" },
		func(r *TerminalReceipt) { r.Metadata = map[string]any{"unexpected": true} },
	} {
		bad := terminal
		mutate(&bad)
		if _, err := audienceHistoryReceiptFromTerminal(source, bad); !errors.Is(err, ErrConflict) {
			t.Fatal("invalid terminal accepted")
		}
	}
	j, _ := NewAudienceHistoryJournal(audienceTestJournals(t))
	for _, invalid := range []segmentport.AudienceHistoryReceipt{
		{}, {SourceIdentifier: source, PayloadDigest: [32]byte{2}, TargetID: 7, TargetDigest: [32]byte{3}, Replayed: true},
	} {
		if err := j.RecordAudienceHistory(context.Background(), "groups", invalid); !errors.Is(err, ErrInvalidScope) {
			t.Fatal("invalid receipt reached transaction")
		}
	}
}
