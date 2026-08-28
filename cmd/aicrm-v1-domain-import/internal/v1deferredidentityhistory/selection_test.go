package v1deferredidentityhistory

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestImportedIdentityTerminalKeepsDistinctSourceAndTargetFieldDigests(t *testing.T) {
	for _, mutation := range []string{"none", "source-key", "payload", "empty-source-field", "empty-target-field"} {
		t.Run(mutation, func(t *testing.T) {
			fixture := newDeferredIdentitySelectionFixture(t)
			table := DM01IdentityMapSourceTable
			receipts := fixture.dm01.receipts[table]
			last := &receipts[len(receipts)-1]
			last.Disposition = "imported"
			last.FieldDigest[0] ^= 1 // DM01 withFieldDigest(target), not tracker source fields.
			fixture.dm01.quarantines[table] = fixture.dm01.quarantines[table][:2]
			checkpoint := fixture.dm01.checkpoints[table]
			switch mutation {
			case "source-key":
				checkpoint.FinalSourceKeyHMAC[0] ^= 1
			case "payload":
				checkpoint.PayloadHMAC[0] ^= 1
			case "empty-source-field":
				checkpoint.FieldDigest = [32]byte{}
			case "empty-target-field":
				last.FieldDigest = [32]byte{}
			}
			fixture.dm01.checkpoints[table] = checkpoint
			selection, err := SelectDeferredIdentityEvidence(context.Background(), fixture.archive, fixture.dm01, fixture.options)
			if mutation == "none" {
				if err != nil || selection.Count() != 1392 || selection.MapImportedRows != 2 {
					t.Fatalf("legitimate target digest rejected: %v", err)
				}
			} else if !errors.Is(err, ErrInvalidDeferredIdentitySelection) {
				t.Fatalf("invalid checkpoint accepted: %v", err)
			}
		})
	}
}

func TestSelectDeferredIdentityEvidencePreservesOnlyFrozenScope(t *testing.T) {
	fixture := newDeferredIdentitySelectionFixture(t)
	selection, err := SelectDeferredIdentityEvidence(context.Background(), fixture.archive, fixture.dm01, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Count() != expectedPeopleRows+expectedIdentityConflictRows+expectedMissingCustomerRootRows || len(selection.People) != expectedPeopleRows || len(selection.IdentityConflicts) != expectedIdentityConflictRows || len(selection.MissingCustomerRootMaps) != expectedMissingCustomerRootRows {
		t.Fatalf("selected rows = people:%d conflicts:%d maps:%d total:%d", len(selection.People), len(selection.IdentityConflicts), len(selection.MissingCustomerRootMaps), selection.Count())
	}
	if selection.MapImportedRows != 1 || selection.MapArchiveOnlyRows != 1 || selection.MapNonTargetQuarantinedRows != 1 {
		t.Fatalf("map exclusion counts = imported:%d archive-only:%d non-target-quarantine:%d", selection.MapImportedRows, selection.MapArchiveOnlyRows, selection.MapNonTargetQuarantinedRows)
	}
	for _, selected := range selection.People {
		if selected.Lineage.ReasonCode != TargetSchemaDeferredReason || selected.Lineage.KeyVersion != fixture.options.DM01HMACKeyVersion || selected.Lineage.PayloadHMAC == ([32]byte{}) || selected.Lineage.FieldDigest == ([32]byte{}) || selected.Fact.Source.SourceKeyDigest != OpaqueDigest(selected.ArchivedRow.SourceKeyHMAC) {
			t.Fatalf("unexpected people lineage: %#v", selected.Lineage)
		}
	}
	for _, selected := range selection.MissingCustomerRootMaps {
		want, err := contactmigration.SourceKeyHMAC(fixture.options.DM01HMACKey, DM01IdentityMapSourceTable, strconv.FormatInt(selected.Fact.SourceID, 10))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(selected.Lineage.SourceKeyHMAC[:], want) || selected.Lineage.SourceKeyHMAC == selected.ArchivedRow.SourceKeyHMAC || selected.Lineage.ReasonCode != MissingCustomerRootReason {
			t.Fatalf("map selection used archive [id] key or lost DM01 reason")
		}
	}
}

func TestSelectDeferredIdentityEvidenceFailsClosedOnRunCheckpointAndReceiptDrift(t *testing.T) {
	for name, change := range map[string]func(*deferredIdentitySelectionFixture){
		"run must be full": func(fixture *deferredIdentitySelectionFixture) {
			fixture.dm01.run.Mode = "incremental"
		},
		"run must be imported": func(fixture *deferredIdentitySelectionFixture) {
			fixture.dm01.run.State = "reconciled"
		},
		"run key version must agree": func(fixture *deferredIdentitySelectionFixture) {
			fixture.dm01.run.HMACKeyVersion++
		},
		"checkpoint must be completed": func(fixture *deferredIdentitySelectionFixture) {
			checkpoint := fixture.dm01.checkpoints[DM01PeopleSourceTable]
			checkpoint.UpperBoundEmpty = true
			fixture.dm01.checkpoints[DM01PeopleSourceTable] = checkpoint
		},
		"checkpoint source table must agree": func(fixture *deferredIdentitySelectionFixture) {
			checkpoint := fixture.dm01.checkpoints[DM01PeopleSourceTable]
			checkpoint.SourceTable = DM01IdentityConflictsSourceTable
			fixture.dm01.checkpoints[DM01PeopleSourceTable] = checkpoint
		},
		"checkpoint terminal must agree": func(fixture *deferredIdentitySelectionFixture) {
			checkpoint := fixture.dm01.checkpoints[DM01IdentityConflictsSourceTable]
			checkpoint.FieldDigest[0]++
			fixture.dm01.checkpoints[DM01IdentityConflictsSourceTable] = checkpoint
		},
		"receipt and quarantine payload must agree": func(fixture *deferredIdentitySelectionFixture) {
			fixture.dm01.receipts[DM01PeopleSourceTable][0].PayloadHMAC[0]++
		},
		"people reason must remain frozen": func(fixture *deferredIdentitySelectionFixture) {
			fixture.dm01.quarantines[DM01PeopleSourceTable][0].ReasonCode = MissingCustomerRootReason
		},
		"map source HMAC cannot use archive key domain": func(fixture *deferredIdentitySelectionFixture) {
			row := fixture.archive.rows[ExternalContactIdentityMapID][0]
			fixture.dm01.receipts[DM01IdentityMapSourceTable][0].SourceKeyHMAC = row.SourceKeyHMAC
			fixture.dm01.quarantines[DM01IdentityMapSourceTable][0].SourceKeyHMAC = row.SourceKeyHMAC
		},
		"map target reason must remain missing customer root": func(fixture *deferredIdentitySelectionFixture) {
			fixture.dm01.quarantines[DM01IdentityMapSourceTable][0].ReasonCode = TargetSchemaDeferredReason
		},
		"short DM01 key is rejected": func(fixture *deferredIdentitySelectionFixture) {
			fixture.options.DM01HMACKey = bytes.Repeat([]byte{3}, 31)
		},
		"archive ordinal must remain contiguous": func(fixture *deferredIdentitySelectionFixture) {
			fixture.archive.rows[PeopleTableID][1].SourceOrdinal = 9
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newDeferredIdentitySelectionFixture(t)
			change(fixture)
			_, err := SelectDeferredIdentityEvidence(context.Background(), fixture.archive, fixture.dm01, fixture.options)
			if !errors.Is(err, ErrInvalidDeferredIdentitySelection) {
				t.Fatalf("error = %v", err)
			}
			var selectionErr *SelectionError
			if !errors.As(err, &selectionErr) || selectionErr.Stage == "" {
				t.Fatalf("safe selection error = %v", err)
			}
		})
	}
}

type deferredIdentitySelectionFixture struct {
	archive *deferredIdentityArchive
	dm01    *deferredIdentityDM01
	options DeferredIdentitySelectionOptions
}

func newDeferredIdentitySelectionFixture(t *testing.T) *deferredIdentitySelectionFixture {
	t.Helper()
	archiveKey := bytes.Repeat([]byte{9}, 32)
	dm01Key := bytes.Repeat([]byte{3}, 64) // DM01 accepts the deployed 64-byte HMAC key.
	archive := &deferredIdentityArchive{rows: map[string][]v1archive.ArchivedRow{}}
	dm01 := &deferredIdentityDM01{
		run:         DM01Run{ID: 2, Mode: "full", State: "imported", HMACKeyVersion: 7},
		checkpoints: map[string]DM01Checkpoint{},
		receipts:    map[string][]DM01TerminalReceipt{},
		quarantines: map[string][]DM01Quarantine{},
	}
	stamp := time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
	people := make([]v1archive.ArchivedRow, 0, expectedPeopleRows)
	for index := 0; index < expectedPeopleRows; index++ {
		id := int64(index - 700)
		row := deferredIdentityRow(t, archiveKey, PeopleTableID, id, map[string]any{"id": id, "mobile": "", "third_party_user_id": "", "created_at": stamp, "updated_at": stamp})
		row.SourceOrdinal = int64(index + 1)
		people = append(people, row)
	}
	archive.rows[PeopleTableID] = people
	dm01.receipts[DM01PeopleSourceTable], dm01.quarantines[DM01PeopleSourceTable] = deferredQuarantinedEvidence(t, dm01Key, DM01PeopleSourceTable, people, TargetSchemaDeferredReason)
	dm01.checkpoints[DM01PeopleSourceTable] = deferredCheckpoint(DM01PeopleSourceTable, dm01.receipts[DM01PeopleSourceTable])

	conflicts := make([]v1archive.ArchivedRow, 0, expectedIdentityConflictRows)
	for index := 0; index < expectedIdentityConflictRows; index++ {
		id := int64(index - 2)
		row := deferredIdentityRow(t, archiveKey, IdentityConflictsTableID, id, map[string]any{
			"id": id, "conflict_type": "", "unionid": "", "candidate_unionid": "", "external_userid": "", "openid": "", "mobile": "", "source_type": "", "source_key": "", "payload_json": nil, "source_payload_json": nil,
			"status": "", "resolution_status": "", "resolution_note": "", "created_at": stamp, "updated_at": stamp, "resolved_at": nil,
		})
		row.SourceOrdinal = int64(index + 1)
		conflicts = append(conflicts, row)
	}
	archive.rows[IdentityConflictsTableID] = conflicts
	dm01.receipts[DM01IdentityConflictsSourceTable], dm01.quarantines[DM01IdentityConflictsSourceTable] = deferredQuarantinedEvidence(t, dm01Key, DM01IdentityConflictsSourceTable, conflicts, TargetSchemaDeferredReason)
	dm01.checkpoints[DM01IdentityConflictsSourceTable] = deferredCheckpoint(DM01IdentityConflictsSourceTable, dm01.receipts[DM01IdentityConflictsSourceTable])

	maps := make([]v1archive.ArchivedRow, 0, 5)
	for index, id := range []int64{-2, -1, 0, 1, 2} {
		row := deferredIdentityRow(t, archiveKey, ExternalContactIdentityMapID, id, map[string]any{
			"id": id, "corp_id": "", "external_userid": "", "unionid": "", "openid": "", "follow_user_userid": "", "name": "", "type": nil, "avatar": "", "gender": nil,
			"status": "", "raw_profile": nil, "first_seen_at": stamp, "last_seen_at": stamp, "created_at": stamp, "updated_at": stamp,
		})
		row.SourceOrdinal = int64(index + 1)
		maps = append(maps, row)
	}
	archive.rows[ExternalContactIdentityMapID] = maps
	mapReceipts := make([]DM01TerminalReceipt, 0, 4)
	mapQuarantines := make([]DM01Quarantine, 0, 3)
	for index, item := range []struct {
		row         v1archive.ArchivedRow
		disposition string
		reason      string
	}{
		{maps[0], "quarantined", MissingCustomerRootReason},
		{maps[1], "quarantined", MissingCustomerRootReason},
		{maps[2], "imported", ""},
		{maps[3], "quarantined", "scoped_identity_customer_conflict"},
	} {
		key := deferredDM01Key(t, dm01Key, DM01IdentityMapSourceTable, mapSourceID(t, archiveKey, item.row))
		receipt := DM01TerminalReceipt{SourceTable: DM01IdentityMapSourceTable, SourceOrdinal: int64(index + 1), SourceKeyHMAC: key, PayloadHMAC: item.row.PayloadHMAC, FieldDigest: item.row.FieldHMAC, Disposition: item.disposition}
		mapReceipts = append(mapReceipts, receipt)
		if item.disposition == "quarantined" {
			mapQuarantines = append(mapQuarantines, DM01Quarantine{SourceTable: DM01IdentityMapSourceTable, SourceKeyHMAC: key, PayloadHMAC: item.row.PayloadHMAC, FieldDigest: item.row.FieldHMAC, ReasonCode: item.reason})
		}
	}
	dm01.receipts[DM01IdentityMapSourceTable], dm01.quarantines[DM01IdentityMapSourceTable] = mapReceipts, mapQuarantines
	dm01.checkpoints[DM01IdentityMapSourceTable] = deferredCheckpoint(DM01IdentityMapSourceTable, mapReceipts)

	return &deferredIdentitySelectionFixture{archive: archive, dm01: dm01, options: DeferredIdentitySelectionOptions{ArchiveRunID: "full-archive", DM01RunID: 2, ArchiveHMACKey: archiveKey, DM01HMACKey: dm01Key, DM01HMACKeyVersion: 7}}
}

func deferredQuarantinedEvidence(t *testing.T, key []byte, sourceTable string, rows []v1archive.ArchivedRow, reason string) ([]DM01TerminalReceipt, []DM01Quarantine) {
	t.Helper()
	receipts := make([]DM01TerminalReceipt, 0, len(rows))
	quarantines := make([]DM01Quarantine, 0, len(rows))
	for index, row := range rows {
		id := deferredSourceID(t, row, sourceTable)
		digest := deferredDM01Key(t, key, sourceTable, id)
		receipt := DM01TerminalReceipt{SourceTable: sourceTable, SourceOrdinal: int64(index + 1), SourceKeyHMAC: digest, PayloadHMAC: row.PayloadHMAC, FieldDigest: row.FieldHMAC, Disposition: "quarantined"}
		receipts = append(receipts, receipt)
		quarantines = append(quarantines, DM01Quarantine{SourceTable: sourceTable, SourceKeyHMAC: digest, PayloadHMAC: row.PayloadHMAC, FieldDigest: row.FieldHMAC, ReasonCode: reason})
	}
	return receipts, quarantines
}

func deferredCheckpoint(table string, receipts []DM01TerminalReceipt) DM01Checkpoint {
	last := receipts[len(receipts)-1]
	return DM01Checkpoint{SourceTable: table, FinalSourceKeyHMAC: last.SourceKeyHMAC, PayloadHMAC: last.PayloadHMAC, FieldDigest: last.FieldDigest, Watermark: time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC), UpperSourceKeyHMAC: last.SourceKeyHMAC}
}

func deferredSourceID(t *testing.T, row v1archive.ArchivedRow, table string) int64 {
	t.Helper()
	switch table {
	case DM01PeopleSourceTable:
		fact, err := AdaptPerson(row, bytes.Repeat([]byte{9}, 32))
		if err != nil {
			t.Fatal(err)
		}
		return fact.SourceID
	case DM01IdentityConflictsSourceTable:
		fact, err := AdaptConflict(row, bytes.Repeat([]byte{9}, 32))
		if err != nil {
			t.Fatal(err)
		}
		return fact.SourceID
	default:
		t.Fatal("unexpected table")
		return 0
	}
}

func mapSourceID(t *testing.T, key []byte, row v1archive.ArchivedRow) int64 {
	t.Helper()
	fact, err := AdaptMissingRootIdentity(row, key)
	if err != nil {
		t.Fatal(err)
	}
	return fact.SourceID
}

func deferredDM01Key(t *testing.T, key []byte, table string, id int64) [32]byte {
	t.Helper()
	value, err := contactmigration.SourceKeyHMAC(key, table, strconv.FormatInt(id, 10))
	if err != nil {
		t.Fatal(err)
	}
	var result [32]byte
	copy(result[:], value)
	return result
}

type deferredIdentityArchive struct {
	rows map[string][]v1archive.ArchivedRow
}

func (archive *deferredIdentityArchive) EachTableRow(_ context.Context, runID, table string, callback func(v1archive.ArchivedRow) error) error {
	if runID != "full-archive" || callback == nil {
		return errors.New("archive scope")
	}
	for _, row := range archive.rows[table] {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type deferredIdentityDM01 struct {
	run         DM01Run
	checkpoints map[string]DM01Checkpoint
	receipts    map[string][]DM01TerminalReceipt
	quarantines map[string][]DM01Quarantine
}

func (source *deferredIdentityDM01) ReadDM01Run(_ context.Context, runID int64) (DM01Run, error) {
	if runID != source.run.ID {
		return DM01Run{}, errors.New("run")
	}
	return source.run, nil
}

func (source *deferredIdentityDM01) ReadDM01Checkpoint(_ context.Context, runID int64, table string) (DM01Checkpoint, error) {
	if runID != source.run.ID {
		return DM01Checkpoint{}, errors.New("run")
	}
	checkpoint, found := source.checkpoints[table]
	if !found {
		return DM01Checkpoint{}, errors.New("checkpoint")
	}
	return checkpoint, nil
}

func (source *deferredIdentityDM01) EachDM01TerminalReceipt(_ context.Context, runID int64, table string, callback func(DM01TerminalReceipt) error) error {
	if runID != source.run.ID || callback == nil {
		return errors.New("receipt")
	}
	for _, value := range source.receipts[table] {
		if err := callback(value); err != nil {
			return err
		}
	}
	return nil
}

func (source *deferredIdentityDM01) EachDM01Quarantine(_ context.Context, runID int64, table string, callback func(DM01Quarantine) error) error {
	if runID != source.run.ID || callback == nil {
		return errors.New("quarantine")
	}
	for _, value := range source.quarantines[table] {
		if err := callback(value); err != nil {
			return err
		}
	}
	return nil
}
