package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	cycleport "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/port"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

func TestStaticTailHistoryJournalRoutesFiveExactScopes(t *testing.T) {
	values := map[string]*staticTailHistoryTerminalFake{}
	journals := map[string]staticTailHistoryTerminalJournal{}
	for _, scope := range staticTailHistoryScopes {
		values[scope.kind] = &staticTailHistoryTerminalFake{values: map[string]TerminalReceipt{}}
		journals[scope.kind] = values[scope.kind]
	}
	journal, err := newStaticTailHistoryJournal(journals)
	if err != nil {
		t.Fatal(err)
	}
	media := mediaport.StaticMediaHistoryReceipt{Kind: staticTailGroupInviteKind, SourceIdentifier: SourceIdentifier(staticTailHistoryDigest(1)), PayloadDigest: staticTailHistoryDigest(11), TargetDigest: staticTailHistoryDigest(21), TargetID: 31}
	if err = journal.RecordStaticMediaHistory(context.Background(), media); err != nil {
		t.Fatal(err)
	}
	if got, found, err := journal.LoadStaticMediaHistory(context.Background(), media.Kind, media.SourceIdentifier); err != nil || !found || got != media {
		t.Fatalf("media=%#v found=%t err=%v", got, found, err)
	}
	product := productport.StaticProductHistoryReceipt{Kind: staticTailPageSliceKind, SourceIdentifier: SourceIdentifier(staticTailHistoryDigest(2)), PayloadDigest: staticTailHistoryDigest(12), TargetDigest: staticTailHistoryDigest(22), TargetID: 32}
	if err = journal.RecordStaticProductHistory(context.Background(), product); err != nil {
		t.Fatal(err)
	}
	if got, found, err := journal.LoadStaticProductHistory(context.Background(), product.Kind, product.SourceIdentifier); err != nil || !found || got != product {
		t.Fatalf("product=%#v found=%t err=%v", got, found, err)
	}
	for index, kind := range []string{staticTailCycleStrategyKind, staticTailCycleVersionKind, staticTailCycleDocumentKind} {
		receipt := cycleport.StaticCycleHistoryReceipt{Kind: kind, SourceIdentifier: SourceIdentifier(staticTailHistoryDigest(byte(index + 3))), PayloadDigest: staticTailHistoryDigest(byte(index + 13)), TargetDigest: staticTailHistoryDigest(byte(index + 23)), TargetID: int64(index + 33)}
		if err = journal.RecordStaticCycleHistory(context.Background(), receipt); err != nil {
			t.Fatalf("record %s: %v", kind, err)
		}
		if got, found, loadErr := journal.LoadStaticCycleHistory(context.Background(), kind, receipt.SourceIdentifier); loadErr != nil || !found || got != receipt {
			t.Fatalf("load %s: %#v found=%t err=%v", kind, got, found, loadErr)
		}
	}
	for kind, value := range values {
		if len(value.values) != 1 {
			t.Fatalf("kind=%s values=%d", kind, len(value.values))
		}
	}
}

func TestStaticTailHistoryJournalRejectsWrongKindAndScope(t *testing.T) {
	values := map[string]staticTailHistoryTerminalJournal{}
	for _, scope := range staticTailHistoryScopes {
		values[scope.kind] = &staticTailHistoryTerminalFake{values: map[string]TerminalReceipt{}}
	}
	journal, err := newStaticTailHistoryJournal(values)
	if err != nil {
		t.Fatal(err)
	}
	receipt := mediaport.StaticMediaHistoryReceipt{Kind: staticTailGroupInviteKind, SourceIdentifier: SourceIdentifier(staticTailHistoryDigest(1)), PayloadDigest: staticTailHistoryDigest(11), TargetDigest: staticTailHistoryDigest(21), TargetID: 31}
	if err := journal.RecordStaticMediaHistory(context.Background(), mediaport.StaticMediaHistoryReceipt{Kind: "wrong", SourceIdentifier: receipt.SourceIdentifier, PayloadDigest: receipt.PayloadDigest, TargetDigest: receipt.TargetDigest, TargetID: receipt.TargetID}); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("wrong kind error=%v", err)
	}
	if err := journal.RecordStaticMediaHistory(context.Background(), mediaport.StaticMediaHistoryReceipt{Kind: receipt.Kind, SourceIdentifier: receipt.SourceIdentifier, PayloadDigest: receipt.PayloadDigest, TargetDigest: receipt.TargetDigest, TargetID: receipt.TargetID, Replayed: true}); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("replay record error=%v", err)
	}
	if _, _, err := journal.LoadTerminal(context.Background(), "public/not_static_tail", receipt.SourceIdentifier); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("unknown table error=%v", err)
	}

	groupInvites := staticTailHistoryScopedJournal(staticTailGroupInviteKind, "run")
	pageSlices := staticTailHistoryScopedJournal(staticTailPageSliceKind, "run")
	strategies := staticTailHistoryScopedJournal(staticTailCycleStrategyKind, "run")
	versions := staticTailHistoryScopedJournal(staticTailCycleVersionKind, "run")
	documents := staticTailHistoryScopedJournal(staticTailCycleDocumentKind, "run")
	if _, err := NewStaticTailHistoryJournal(groupInvites, pageSlices, strategies, versions, documents); err != nil {
		t.Fatal(err)
	}
	documents.scope.ArchiveRunID = "other"
	if _, err := NewStaticTailHistoryJournal(groupInvites, pageSlices, strategies, versions, documents); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("mixed run error=%v", err)
	}
}

type staticTailHistoryTerminalFake struct{ values map[string]TerminalReceipt }

func (journal *staticTailHistoryTerminalFake) LoadTerminal(_ context.Context, source string) (TerminalReceipt, bool, error) {
	value, found := journal.values[source]
	return value, found, nil
}

func (journal *staticTailHistoryTerminalFake) Record(_ context.Context, receipt TerminalReceipt) error {
	source := SourceIdentifier(receipt.SourceKeyDigest)
	if existing, found := journal.values[source]; found && !reflect.DeepEqual(existing, receipt) {
		return ErrConflict
	}
	journal.values[source] = receipt
	return nil
}

func staticTailHistoryDigest(first byte) (digest [sha256.Size]byte) {
	digest[0] = first
	return digest
}

func staticTailHistoryScopedJournal(kind, archiveRun string) *Journal {
	scope, ok := staticTailHistoryScopeForKind(kind)
	if !ok {
		panic("unknown static tail kind")
	}
	return &Journal{scope: Scope{ImportVersion: staticTailHistoryImportVersion, ArchiveRunID: archiveRun, AdapterID: v1archive.DefaultAdapterID, TableID: scope.table, TargetDomain: scope.domain, TargetTable: scope.target}, tx: func(context.Context) (pgx.Tx, error) {
		return nil, nil
	}}
}
