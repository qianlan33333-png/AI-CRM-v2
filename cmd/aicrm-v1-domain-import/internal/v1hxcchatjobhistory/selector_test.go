package v1hxcchatjobhistory

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestSelectorPairsEveryChatJobWithGenericArchiveTerminalInSourceOrder(t *testing.T) {
	key := []byte(strings.Repeat("k", sha256.Size))
	at := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	firstValue := chatJobFixture(at, at)
	firstValue["id"] = int64(-1)
	secondValue := chatJobFixture(at, at)
	secondValue["id"] = int64(0)
	first := chatJobRow(t, key, firstValue)
	second := chatJobRow(t, key, secondValue)
	second.SourceOrdinal = 2

	archive := selectorArchive{rows: []v1archive.ArchivedRow{first, second}}
	terminals := selectorTerminals{rows: []ArchiveTerminalReceipt{archiveTerminal(first), archiveTerminal(second)}}
	selector, err := NewSelector(archive, terminals)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := selector.Select(context.Background(), SelectionOptions{ArchiveRunID: "archive-run", SourceHMACKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Total() != 2 || len(selection.Jobs) != 2 || selection.Jobs[0].SourceOrdinal != 1 || selection.Jobs[0].Fact.SourceID != -1 ||
		selection.Jobs[1].SourceOrdinal != 2 || selection.Jobs[1].Fact.SourceID != 0 {
		t.Fatalf("selection=%#v", selection)
	}
}

func TestSelectorFailsClosedOnArchivedRowOrGenericTerminalDrift(t *testing.T) {
	key := []byte(strings.Repeat("k", sha256.Size))
	at := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	row := chatJobRow(t, key, chatJobFixture(at, at))
	baseRows := []v1archive.ArchivedRow{row}
	baseTerminals := []ArchiveTerminalReceipt{archiveTerminal(row)}
	for _, test := range []struct {
		name            string
		mutateRows      func([]v1archive.ArchivedRow) []v1archive.ArchivedRow
		mutateTerminals func([]ArchiveTerminalReceipt) []ArchiveTerminalReceipt
	}{
		{name: "ordinal gap", mutateRows: func(rows []v1archive.ArchivedRow) []v1archive.ArchivedRow { rows[0].SourceOrdinal = 2; return rows }},
		{name: "duplicate source key", mutateRows: func(rows []v1archive.ArchivedRow) []v1archive.ArchivedRow {
			duplicate := rows[0]
			duplicate.SourceOrdinal = 2
			return append(rows, duplicate)
		}},
		{name: "wrong source adapter", mutateRows: func(rows []v1archive.ArchivedRow) []v1archive.ArchivedRow { rows[0].AdapterID = "other"; return rows }},
		{name: "wrong source table", mutateRows: func(rows []v1archive.ArchivedRow) []v1archive.ArchivedRow {
			rows[0].TableID = "public/other"
			return rows
		}},
		{name: "source HMAC", mutateRows: func(rows []v1archive.ArchivedRow) []v1archive.ArchivedRow { rows[0].SourceKeyHMAC[0]++; return rows }},
		{name: "payload HMAC", mutateRows: func(rows []v1archive.ArchivedRow) []v1archive.ArchivedRow { rows[0].PayloadHMAC[0]++; return rows }},
		{name: "field HMAC", mutateRows: func(rows []v1archive.ArchivedRow) []v1archive.ArchivedRow { rows[0].FieldHMAC[0]++; return rows }},
		{name: "missing terminal", mutateTerminals: func([]ArchiveTerminalReceipt) []ArchiveTerminalReceipt { return nil }},
		{name: "extra terminal", mutateTerminals: func(rows []ArchiveTerminalReceipt) []ArchiveTerminalReceipt {
			extra := rows[0]
			extra.SourceKeyDigest[0]++
			return append(rows, extra)
		}},
		{name: "duplicate terminal", mutateTerminals: func(rows []ArchiveTerminalReceipt) []ArchiveTerminalReceipt { return append(rows, rows[0]) }},
		{name: "payload terminal mismatch", mutateTerminals: func(rows []ArchiveTerminalReceipt) []ArchiveTerminalReceipt { rows[0].PayloadDigest[0]++; return rows }},
		{name: "field terminal mismatch", mutateTerminals: func(rows []ArchiveTerminalReceipt) []ArchiveTerminalReceipt { rows[0].FieldDigest[0]++; return rows }},
		{name: "wrong run", mutateTerminals: func(rows []ArchiveTerminalReceipt) []ArchiveTerminalReceipt {
			rows[0].ArchiveRunID = "other"
			return rows
		}},
		{name: "wrong adapter", mutateTerminals: func(rows []ArchiveTerminalReceipt) []ArchiveTerminalReceipt { rows[0].AdapterID = "other"; return rows }},
		{name: "wrong table", mutateTerminals: func(rows []ArchiveTerminalReceipt) []ArchiveTerminalReceipt {
			rows[0].TableID = "public/other"
			return rows
		}},
		{name: "non archive terminal", mutateTerminals: func(rows []ArchiveTerminalReceipt) []ArchiveTerminalReceipt {
			rows[0].Disposition = "import"
			return rows
		}},
		{name: "terminal operation", mutateTerminals: func(rows []ArchiveTerminalReceipt) []ArchiveTerminalReceipt {
			rows[0].Operation = "archive"
			return rows
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := append([]v1archive.ArchivedRow(nil), baseRows...)
			terminals := append([]ArchiveTerminalReceipt(nil), baseTerminals...)
			if test.mutateRows != nil {
				rows = test.mutateRows(rows)
			}
			if test.mutateTerminals != nil {
				terminals = test.mutateTerminals(terminals)
			}
			selector, err := NewSelector(selectorArchive{rows: rows}, selectorTerminals{rows: terminals})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = selector.Select(context.Background(), SelectionOptions{ArchiveRunID: "archive-run", SourceHMACKey: key}); !errors.Is(err, ErrSealedDrift) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestSelectorRejectsInvalidInputs(t *testing.T) {
	if _, err := NewSelector(nil, selectorTerminals{}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("nil archive error=%v", err)
	}
	selector, err := NewSelector(selectorArchive{}, selectorTerminals{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = selector.Select(context.Background(), SelectionOptions{ArchiveRunID: " archive-run", SourceHMACKey: []byte(strings.Repeat("k", sha256.Size))}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("invalid selection options error=%v", err)
	}
	if _, err = selector.Select(context.Background(), SelectionOptions{ArchiveRunID: "archive-run", SourceHMACKey: []byte(strings.Repeat("k", sha256.Size-1))}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("short source HMAC key accepted: err=%v", err)
	}
}

type selectorArchive struct {
	rows []v1archive.ArchivedRow
}

func (source selectorArchive) EachTableRow(_ context.Context, run, table string, emit func(v1archive.ArchivedRow) error) error {
	if run != "archive-run" || table != ChatJobsTableID {
		return errors.New("unexpected archive scope")
	}
	for _, row := range source.rows {
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}

type selectorTerminals struct {
	rows []ArchiveTerminalReceipt
}

func (source selectorTerminals) EachArchiveTerminal(_ context.Context, run, table string, emit func(ArchiveTerminalReceipt) error) error {
	if run != "archive-run" || table != ChatJobsTableID {
		return errors.New("unexpected terminal scope")
	}
	for _, row := range source.rows {
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}

func archiveTerminal(row v1archive.ArchivedRow) ArchiveTerminalReceipt {
	return ArchiveTerminalReceipt{
		ArchiveRunID: "archive-run", AdapterID: v1archive.DefaultAdapterID, TableID: ChatJobsTableID,
		SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, FieldDigest: row.FieldHMAC, Disposition: "archive",
	}
}
