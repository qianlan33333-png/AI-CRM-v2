package v1contactreferencehistory

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestSelectorPairsCompleteArchiveTerminalsInSourceOrder(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	stamp := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	bindingOne := contactReferenceRow(t, key, ExternalContactBindingsTableID, 1, bindingPayload("external-a", 9, stamp))
	bindingTwo := contactReferenceRow(t, key, ExternalContactBindingsTableID, 2, bindingPayload("external-b", -3, stamp))
	directory := contactReferenceRow(t, key, AdminWeComDirectoryMembersTableID, 1, directoryPayload(stamp))
	archive := selectorArchive{rows: map[string][]v1archive.ArchivedRow{
		ExternalContactBindingsTableID:    {bindingOne, bindingTwo},
		AdminWeComDirectoryMembersTableID: {directory},
	}}
	terminals := selectorTerminals{rows: map[string][]ArchiveTerminalReceipt{
		ExternalContactBindingsTableID:    {archiveTerminal(bindingOne), archiveTerminal(bindingTwo)},
		AdminWeComDirectoryMembersTableID: {archiveTerminal(directory)},
	}}
	selector, err := NewSelector(archive, terminals)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := selector.Select(context.Background(), SelectionOptions{ArchiveRunID: "run", SourceHMACKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Total() != 3 || len(selection.Bindings) != 2 || selection.Bindings[0].SourceOrdinal != 1 || selection.Bindings[0].Fact.ExternalUserID != "external-a" || selection.Bindings[1].SourceOrdinal != 2 || selection.Bindings[1].Fact.PersonID != -3 || len(selection.DirectoryMembers) != 1 || selection.DirectoryMembers[0].SourceOrdinal != 1 || selection.DirectoryMembers[0].Fact.ID != 1 {
		t.Fatalf("selection = %#v", selection)
	}
}

func TestSelectorFailsClosedOnArchiveOrReceiptDrift(t *testing.T) {
	key := bytes.Repeat([]byte{8}, 32)
	stamp := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	binding := contactReferenceRow(t, key, ExternalContactBindingsTableID, 1, bindingPayload("external", 1, stamp))
	directory := contactReferenceRow(t, key, AdminWeComDirectoryMembersTableID, 1, directoryPayload(stamp))
	baseRows := map[string][]v1archive.ArchivedRow{ExternalContactBindingsTableID: {binding}, AdminWeComDirectoryMembersTableID: {directory}}
	baseReceipts := map[string][]ArchiveTerminalReceipt{ExternalContactBindingsTableID: {archiveTerminal(binding)}, AdminWeComDirectoryMembersTableID: {archiveTerminal(directory)}}
	cases := map[string]struct {
		mutateRows     func(map[string][]v1archive.ArchivedRow)
		mutateReceipts func(map[string][]ArchiveTerminalReceipt)
	}{
		"non-continuous ordinal": {mutateRows: func(rows map[string][]v1archive.ArchivedRow) {
			rows[ExternalContactBindingsTableID][0].SourceOrdinal = 2
		}},
		"duplicate archive key": {mutateRows: func(rows map[string][]v1archive.ArchivedRow) {
			rows[ExternalContactBindingsTableID] = append(rows[ExternalContactBindingsTableID], rows[ExternalContactBindingsTableID][0])
		}},
		"missing receipt": {mutateReceipts: func(rows map[string][]ArchiveTerminalReceipt) { rows[ExternalContactBindingsTableID] = nil }},
		"extra receipt": {mutateReceipts: func(rows map[string][]ArchiveTerminalReceipt) {
			extra := archiveTerminal(binding)
			extra.SourceKeyDigest[0]++
			rows[ExternalContactBindingsTableID] = append(rows[ExternalContactBindingsTableID], extra)
		}},
		"payload drift": {mutateReceipts: func(rows map[string][]ArchiveTerminalReceipt) {
			rows[ExternalContactBindingsTableID][0].PayloadDigest[0]++
		}},
		"field drift": {mutateReceipts: func(rows map[string][]ArchiveTerminalReceipt) {
			rows[ExternalContactBindingsTableID][0].FieldDigest[0]++
		}},
		"wrong run": {mutateReceipts: func(rows map[string][]ArchiveTerminalReceipt) {
			rows[ExternalContactBindingsTableID][0].ArchiveRunID = "other"
		}},
		"wrong table": {mutateReceipts: func(rows map[string][]ArchiveTerminalReceipt) {
			rows[ExternalContactBindingsTableID][0].TableID = AdminWeComDirectoryMembersTableID
		}},
		"wrong disposition": {mutateReceipts: func(rows map[string][]ArchiveTerminalReceipt) {
			rows[ExternalContactBindingsTableID][0].Disposition = "quarantine"
		}},
		"operation present": {mutateReceipts: func(rows map[string][]ArchiveTerminalReceipt) {
			rows[ExternalContactBindingsTableID][0].Operation = "archive"
		}},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			rows, receipts := cloneSelectorRows(baseRows), cloneSelectorReceipts(baseReceipts)
			if test.mutateRows != nil {
				test.mutateRows(rows)
			}
			if test.mutateReceipts != nil {
				test.mutateReceipts(receipts)
			}
			selector, err := NewSelector(selectorArchive{rows: rows}, selectorTerminals{rows: receipts})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = selector.Select(context.Background(), SelectionOptions{ArchiveRunID: "run", SourceHMACKey: key}); err != ErrSealedDrift {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestSelectorRejectsInvalidInputs(t *testing.T) {
	if _, err := NewSelector(nil, selectorTerminals{}); err != ErrInvalidSelection {
		t.Fatalf("nil archive error = %v", err)
	}
	selector, err := NewSelector(selectorArchive{}, selectorTerminals{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = selector.Select(context.Background(), SelectionOptions{ArchiveRunID: " run", SourceHMACKey: bytes.Repeat([]byte{1}, 32)}); err != ErrInvalidSelection {
		t.Fatalf("invalid run error = %v", err)
	}
}

func bindingPayload(external string, person int64, stamp time.Time) map[string]any {
	return map[string]any{
		"external_userid": external, "person_id": person, "first_bound_by_userid": "", "first_owner_userid": "", "last_owner_userid": "", "created_at": stamp, "updated_at": stamp,
	}
}

type selectorArchive struct {
	rows map[string][]v1archive.ArchivedRow
}

func (source selectorArchive) EachTableRow(_ context.Context, _ string, table string, emit func(v1archive.ArchivedRow) error) error {
	for _, row := range source.rows[table] {
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}

type selectorTerminals struct {
	rows map[string][]ArchiveTerminalReceipt
}

func (source selectorTerminals) EachArchiveTerminal(_ context.Context, _ string, table string, emit func(ArchiveTerminalReceipt) error) error {
	for _, receipt := range source.rows[table] {
		if err := emit(receipt); err != nil {
			return err
		}
	}
	return nil
}

func archiveTerminal(row v1archive.ArchivedRow) ArchiveTerminalReceipt {
	return ArchiveTerminalReceipt{
		ArchiveRunID: "run", AdapterID: v1archive.DefaultAdapterID, TableID: row.TableID,
		SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, FieldDigest: row.FieldHMAC,
		Disposition: "archive",
	}
}

func cloneSelectorRows(input map[string][]v1archive.ArchivedRow) map[string][]v1archive.ArchivedRow {
	output := make(map[string][]v1archive.ArchivedRow, len(input))
	for table, rows := range input {
		output[table] = append([]v1archive.ArchivedRow(nil), rows...)
	}
	return output
}

func cloneSelectorReceipts(input map[string][]ArchiveTerminalReceipt) map[string][]ArchiveTerminalReceipt {
	output := make(map[string][]ArchiveTerminalReceipt, len(input))
	for table, rows := range input {
		output[table] = append([]ArchiveTerminalReceipt(nil), rows...)
	}
	return output
}
