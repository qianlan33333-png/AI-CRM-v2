package v1domain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	v1deferredidentityhistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1deferredidentityhistory"
	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestDeferredIdentityHistoryImporterWritesAllSelectedFactsAndReplays(t *testing.T) {
	fixture := newDeferredIdentityHistoryFixture(t)
	journal := newDeferredIdentityHistoryJournalFake()
	writer := newDeferredIdentityHistoryWriterFake(journal)
	importer, err := NewDeferredIdentityHistoryImporter(fixture.archive, fixture.dm01, deferredIdentityHistoryUOW{journal: journal}, writer, journal, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(context.Background())
	if err != nil || result != (DeferredIdentityHistoryImportResult{ImportedPeople: 1385, ImportedConflicts: 5, ImportedMissingRoots: 2}) {
		t.Fatalf("first import=%+v err=%v", result, err)
	}
	if writer.writes[DeferredPersonHistoryKind] != 1385 || writer.writes[DeferredConflictHistoryKind] != 5 || writer.writes[MissingRootIdentityKind] != 2 || len(journal.terminals) != 1392 || !writer.calledInTx {
		t.Fatalf("writes=%v terminals=%d caller-tx=%t", writer.writes, len(journal.terminals), writer.calledInTx)
	}
	if writer.person.SourceID != 684 || writer.conflict.SourceID != 2 || writer.missing.DM01RunID != 2 || writer.missing.DM01SourceHMACKeyVersion != "7" || writer.missing.QuarantineReason != v1deferredidentityhistory.MissingCustomerRootReason || writer.missing.SourceKeyDigest == ([32]byte{}) {
		t.Fatalf("historical evidence mapping lost: person=%+v conflict=%+v missing=%+v", writer.person, writer.conflict, writer.missing)
	}

	replayed, err := importer.Import(context.Background())
	if err != nil || replayed != (DeferredIdentityHistoryImportResult{ReplayedPeople: 1385, ReplayedConflicts: 5, ReplayedMissingRoots: 2}) {
		t.Fatalf("replay=%+v err=%v", replayed, err)
	}
	if writer.writes[DeferredPersonHistoryKind] != 1385 || writer.writes[DeferredConflictHistoryKind] != 5 || writer.writes[MissingRootIdentityKind] != 2 {
		t.Fatalf("replay created target rows: %v", writer.writes)
	}
}

func TestDeferredIdentityHistoryImporterDoesNotWriteBeforeFullSelectionAndRollsBackReceiptDrift(t *testing.T) {
	t.Run("selection failure writes nothing", func(t *testing.T) {
		fixture := newDeferredIdentityHistoryFixture(t)
		fixture.archive.rows[v1deferredidentityhistory.ExternalContactIdentityMapID][4].FieldHMAC = [32]byte{}
		journal := newDeferredIdentityHistoryJournalFake()
		writer := newDeferredIdentityHistoryWriterFake(journal)
		importer, err := NewDeferredIdentityHistoryImporter(fixture.archive, fixture.dm01, deferredIdentityHistoryUOW{journal: journal}, writer, journal, fixture.options)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = importer.Import(context.Background()); !errors.Is(err, v1deferredidentityhistory.ErrInvalidDeferredIdentitySelection) || len(writer.writes) != 0 || len(journal.terminals) != 0 {
			t.Fatalf("err=%v writes=%v terminals=%d", err, writer.writes, len(journal.terminals))
		}
	})
	t.Run("receipt drift rolls back caller transaction", func(t *testing.T) {
		fixture := newDeferredIdentityHistoryFixture(t)
		journal := newDeferredIdentityHistoryJournalFake()
		writer := newDeferredIdentityHistoryWriterFake(journal)
		writer.badReceipt = true
		importer, err := NewDeferredIdentityHistoryImporter(fixture.archive, fixture.dm01, deferredIdentityHistoryUOW{journal: journal}, writer, journal, fixture.options)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = importer.Import(context.Background()); !errors.Is(err, ErrConflict) || len(journal.terminals) != 0 {
			t.Fatalf("err=%v terminals=%d", err, len(journal.terminals))
		}
	})
}

type deferredIdentityHistoryFixture struct {
	archive *deferredIdentityHistoryArchive
	dm01    *deferredIdentityHistoryDM01
	options v1deferredidentityhistory.DeferredIdentitySelectionOptions
}

func newDeferredIdentityHistoryFixture(t *testing.T) *deferredIdentityHistoryFixture {
	t.Helper()
	archiveKey := bytes.Repeat([]byte{7}, 32)
	dm01Key := bytes.Repeat([]byte{4}, 64)
	archive := &deferredIdentityHistoryArchive{rows: map[string][]v1archive.ArchivedRow{}}
	dm01 := &deferredIdentityHistoryDM01{
		run:         v1deferredidentityhistory.DM01Run{ID: 2, Mode: "full", State: "imported", HMACKeyVersion: 7},
		checkpoints: map[string]v1deferredidentityhistory.DM01Checkpoint{},
		receipts:    map[string][]v1deferredidentityhistory.DM01TerminalReceipt{},
		quarantines: map[string][]v1deferredidentityhistory.DM01Quarantine{},
	}
	stamp := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	people := make([]v1archive.ArchivedRow, 0, 1385)
	for ordinal := 1; ordinal <= 1385; ordinal++ {
		id := int64(ordinal - 701)
		people = append(people, deferredHistoryArchiveRow(t, archiveKey, v1deferredidentityhistory.PeopleTableID, int64(ordinal), id, map[string]any{"id": id, "mobile": "", "third_party_user_id": "", "created_at": stamp, "updated_at": stamp}))
	}
	archive.rows[v1deferredidentityhistory.PeopleTableID] = people
	dm01.receipts[v1deferredidentityhistory.DM01PeopleSourceTable], dm01.quarantines[v1deferredidentityhistory.DM01PeopleSourceTable] = deferredHistoryQuarantinedEvidence(t, dm01Key, v1deferredidentityhistory.DM01PeopleSourceTable, people, v1deferredidentityhistory.TargetSchemaDeferredReason)
	dm01.checkpoints[v1deferredidentityhistory.DM01PeopleSourceTable] = deferredHistoryCheckpoint(v1deferredidentityhistory.DM01PeopleSourceTable, dm01.receipts[v1deferredidentityhistory.DM01PeopleSourceTable])

	conflicts := make([]v1archive.ArchivedRow, 0, 5)
	for ordinal := 1; ordinal <= 5; ordinal++ {
		id := int64(ordinal - 3)
		conflicts = append(conflicts, deferredHistoryArchiveRow(t, archiveKey, v1deferredidentityhistory.IdentityConflictsTableID, int64(ordinal), id, map[string]any{
			"id": id, "conflict_type": "", "unionid": "", "candidate_unionid": "", "external_userid": "", "openid": "", "mobile": "", "source_type": "", "source_key": "", "payload_json": nil, "source_payload_json": nil,
			"status": "", "resolution_status": "", "resolution_note": "", "created_at": stamp, "updated_at": stamp, "resolved_at": nil,
		}))
	}
	archive.rows[v1deferredidentityhistory.IdentityConflictsTableID] = conflicts
	dm01.receipts[v1deferredidentityhistory.DM01IdentityConflictsSourceTable], dm01.quarantines[v1deferredidentityhistory.DM01IdentityConflictsSourceTable] = deferredHistoryQuarantinedEvidence(t, dm01Key, v1deferredidentityhistory.DM01IdentityConflictsSourceTable, conflicts, v1deferredidentityhistory.TargetSchemaDeferredReason)
	dm01.checkpoints[v1deferredidentityhistory.DM01IdentityConflictsSourceTable] = deferredHistoryCheckpoint(v1deferredidentityhistory.DM01IdentityConflictsSourceTable, dm01.receipts[v1deferredidentityhistory.DM01IdentityConflictsSourceTable])

	maps := make([]v1archive.ArchivedRow, 0, 5)
	for index, id := range []int64{-2, -1, 0, 1, 2} {
		maps = append(maps, deferredHistoryArchiveRow(t, archiveKey, v1deferredidentityhistory.ExternalContactIdentityMapID, int64(index+1), id, map[string]any{
			"id": id, "corp_id": "", "external_userid": "", "unionid": "", "openid": "", "follow_user_userid": "", "name": "", "type": nil, "avatar": "", "gender": nil,
			"status": "", "raw_profile": nil, "first_seen_at": stamp, "last_seen_at": stamp, "created_at": stamp, "updated_at": stamp,
		}))
	}
	archive.rows[v1deferredidentityhistory.ExternalContactIdentityMapID] = maps
	mapReceipts := make([]v1deferredidentityhistory.DM01TerminalReceipt, 0, 4)
	mapQuarantines := make([]v1deferredidentityhistory.DM01Quarantine, 0, 3)
	for index, item := range []struct {
		row         v1archive.ArchivedRow
		disposition string
		reason      string
	}{
		{maps[0], "quarantined", v1deferredidentityhistory.MissingCustomerRootReason},
		{maps[1], "quarantined", v1deferredidentityhistory.MissingCustomerRootReason},
		{maps[2], "imported", ""},
		{maps[3], "quarantined", "scoped_identity_customer_conflict"},
	} {
		id := deferredHistorySourceID(t, item.row, v1deferredidentityhistory.DM01IdentityMapSourceTable)
		key := deferredHistoryDM01Key(t, dm01Key, v1deferredidentityhistory.DM01IdentityMapSourceTable, id)
		payload, fields := deferredHistoryDM01Digests(v1deferredidentityhistory.DM01IdentityMapSourceTable, id)
		receipt := v1deferredidentityhistory.DM01TerminalReceipt{SourceTable: v1deferredidentityhistory.DM01IdentityMapSourceTable, SourceOrdinal: int64(index + 1), SourceKeyHMAC: key, PayloadHMAC: payload, FieldDigest: fields, Disposition: item.disposition}
		mapReceipts = append(mapReceipts, receipt)
		if item.disposition == "quarantined" {
			mapQuarantines = append(mapQuarantines, v1deferredidentityhistory.DM01Quarantine{SourceTable: v1deferredidentityhistory.DM01IdentityMapSourceTable, SourceKeyHMAC: key, PayloadHMAC: payload, FieldDigest: fields, ReasonCode: item.reason})
		}
	}
	dm01.receipts[v1deferredidentityhistory.DM01IdentityMapSourceTable], dm01.quarantines[v1deferredidentityhistory.DM01IdentityMapSourceTable] = mapReceipts, mapQuarantines
	dm01.checkpoints[v1deferredidentityhistory.DM01IdentityMapSourceTable] = deferredHistoryCheckpoint(v1deferredidentityhistory.DM01IdentityMapSourceTable, mapReceipts)
	return &deferredIdentityHistoryFixture{archive: archive, dm01: dm01, options: v1deferredidentityhistory.DeferredIdentitySelectionOptions{ArchiveRunID: "archive-run", DM01RunID: 2, ArchiveHMACKey: archiveKey, DM01HMACKey: dm01Key, DM01HMACKeyVersion: 7}}
}

func deferredHistoryArchiveRow(t *testing.T, key []byte, table string, ordinal, id int64, value map[string]any) v1archive.ArchivedRow {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	payload, roots, err := v1archive.RedactPayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	sourceTable := table[len("public/"):]
	source, err := v1archive.SourceKeyHMAC(key, sourceTable, []byte("["+strconv.FormatInt(id, 10)+"]"))
	if err != nil {
		t.Fatal(err)
	}
	payloadHMAC, err := v1archive.PayloadHMAC(key, sourceTable, payload)
	if err != nil {
		t.Fatal(err)
	}
	fieldHMAC, err := v1archive.FieldHMAC(key, sourceTable, roots)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal, SourceKeyHMAC: source, PayloadHMAC: payloadHMAC, FieldHMAC: fieldHMAC, Payload: payload, RedactedFields: roots}
}

func deferredHistoryQuarantinedEvidence(t *testing.T, key []byte, table string, rows []v1archive.ArchivedRow, reason string) ([]v1deferredidentityhistory.DM01TerminalReceipt, []v1deferredidentityhistory.DM01Quarantine) {
	t.Helper()
	receipts := make([]v1deferredidentityhistory.DM01TerminalReceipt, 0, len(rows))
	quarantines := make([]v1deferredidentityhistory.DM01Quarantine, 0, len(rows))
	for index, row := range rows {
		id := deferredHistorySourceID(t, row, table)
		digest := deferredHistoryDM01Key(t, key, table, id)
		payload, fields := deferredHistoryDM01Digests(table, id)
		receipt := v1deferredidentityhistory.DM01TerminalReceipt{SourceTable: table, SourceOrdinal: int64(index + 1), SourceKeyHMAC: digest, PayloadHMAC: payload, FieldDigest: fields, Disposition: "quarantined"}
		receipts = append(receipts, receipt)
		quarantines = append(quarantines, v1deferredidentityhistory.DM01Quarantine{SourceTable: table, SourceKeyHMAC: digest, PayloadHMAC: payload, FieldDigest: fields, ReasonCode: reason})
	}
	return receipts, quarantines
}

func deferredHistorySourceID(t *testing.T, row v1archive.ArchivedRow, table string) int64 {
	t.Helper()
	archiveKey := bytes.Repeat([]byte{7}, 32)
	switch table {
	case v1deferredidentityhistory.DM01PeopleSourceTable:
		value, err := v1deferredidentityhistory.AdaptPerson(row, archiveKey)
		if err != nil {
			t.Fatal(err)
		}
		return value.SourceID
	case v1deferredidentityhistory.DM01IdentityConflictsSourceTable:
		value, err := v1deferredidentityhistory.AdaptConflict(row, archiveKey)
		if err != nil {
			t.Fatal(err)
		}
		return value.SourceID
	case v1deferredidentityhistory.DM01IdentityMapSourceTable:
		value, err := v1deferredidentityhistory.AdaptMissingRootIdentity(row, archiveKey)
		if err != nil {
			t.Fatal(err)
		}
		return value.SourceID
	default:
		t.Fatal("unexpected source table")
		return 0
	}
}

func deferredHistoryDM01Key(t *testing.T, key []byte, table string, id int64) [32]byte {
	t.Helper()
	value, err := contactmigration.SourceKeyHMAC(key, table, strconv.FormatInt(id, 10))
	if err != nil {
		t.Fatal(err)
	}
	var result [32]byte
	copy(result[:], value)
	return result
}

func deferredHistoryDM01Digests(table string, id int64) ([32]byte, [32]byte) {
	return sha256.Sum256([]byte("dm01-payload/" + table + "/" + strconv.FormatInt(id, 10))), sha256.Sum256([]byte("dm01-fields/" + table + "/" + strconv.FormatInt(id, 10)))
}

func deferredHistoryCheckpoint(table string, receipts []v1deferredidentityhistory.DM01TerminalReceipt) v1deferredidentityhistory.DM01Checkpoint {
	last := receipts[len(receipts)-1]
	return v1deferredidentityhistory.DM01Checkpoint{SourceTable: table, FinalSourceKeyHMAC: last.SourceKeyHMAC, PayloadHMAC: last.PayloadHMAC, FieldDigest: last.FieldDigest, Watermark: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC), UpperSourceKeyHMAC: last.SourceKeyHMAC}
}

type deferredIdentityHistoryArchive struct {
	rows map[string][]v1archive.ArchivedRow
}

func (archive *deferredIdentityHistoryArchive) EachTableRow(_ context.Context, runID, table string, callback func(v1archive.ArchivedRow) error) error {
	if runID != "archive-run" || callback == nil {
		return errors.New("archive")
	}
	for _, row := range archive.rows[table] {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type deferredIdentityHistoryDM01 struct {
	run         v1deferredidentityhistory.DM01Run
	checkpoints map[string]v1deferredidentityhistory.DM01Checkpoint
	receipts    map[string][]v1deferredidentityhistory.DM01TerminalReceipt
	quarantines map[string][]v1deferredidentityhistory.DM01Quarantine
}

func (source *deferredIdentityHistoryDM01) ReadDM01Run(_ context.Context, runID int64) (v1deferredidentityhistory.DM01Run, error) {
	if runID != source.run.ID {
		return v1deferredidentityhistory.DM01Run{}, errors.New("run")
	}
	return source.run, nil
}
func (source *deferredIdentityHistoryDM01) ReadDM01Checkpoint(_ context.Context, runID int64, table string) (v1deferredidentityhistory.DM01Checkpoint, error) {
	if runID != source.run.ID {
		return v1deferredidentityhistory.DM01Checkpoint{}, errors.New("run")
	}
	value, found := source.checkpoints[table]
	if !found {
		return v1deferredidentityhistory.DM01Checkpoint{}, errors.New("checkpoint")
	}
	return value, nil
}
func (source *deferredIdentityHistoryDM01) EachDM01TerminalReceipt(_ context.Context, runID int64, table string, callback func(v1deferredidentityhistory.DM01TerminalReceipt) error) error {
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
func (source *deferredIdentityHistoryDM01) EachDM01Quarantine(_ context.Context, runID int64, table string, callback func(v1deferredidentityhistory.DM01Quarantine) error) error {
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

type deferredIdentityHistoryTxKey struct{}

type deferredIdentityHistoryUOW struct {
	journal *deferredIdentityHistoryJournalFake
}

func (uow deferredIdentityHistoryUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	if ctx == nil || callback == nil || uow.journal == nil {
		return ErrInvalidScope
	}
	before := uow.journal.clone()
	err := callback(context.WithValue(ctx, deferredIdentityHistoryTxKey{}, true))
	if err != nil {
		uow.journal.terminals = before
	}
	return err
}

type deferredIdentityHistoryJournalFake struct{ terminals map[string]TerminalReceipt }

func newDeferredIdentityHistoryJournalFake() *deferredIdentityHistoryJournalFake {
	return &deferredIdentityHistoryJournalFake{terminals: map[string]TerminalReceipt{}}
}
func (journal *deferredIdentityHistoryJournalFake) clone() map[string]TerminalReceipt {
	result := make(map[string]TerminalReceipt, len(journal.terminals))
	for key, value := range journal.terminals {
		result[key] = value
	}
	return result
}
func deferredIdentityHistoryJournalKey(kind, source string) string { return kind + "/" + source }
func (journal *deferredIdentityHistoryJournalFake) ValidateDeferredIdentityHistoryImportScope(run string) error {
	if journal == nil || run != "archive-run" {
		return ErrInvalidScope
	}
	return nil
}
func (journal *deferredIdentityHistoryJournalFake) LoadDeferredIdentityHistoryTerminal(_ context.Context, kind, source string) (TerminalReceipt, bool, error) {
	value, found := journal.terminals[deferredIdentityHistoryJournalKey(kind, source)]
	return value, found, nil
}
func (journal *deferredIdentityHistoryJournalFake) RecordDeferredIdentityHistoryTerminal(_ context.Context, kind string, receipt TerminalReceipt) error {
	key := deferredIdentityHistoryJournalKey(kind, SourceIdentifier(receipt.SourceKeyDigest))
	if _, found := journal.terminals[key]; found {
		return ErrConflict
	}
	journal.terminals[key] = receipt
	return nil
}
func (journal *deferredIdentityHistoryJournalFake) LoadDeferredIdentityHistory(ctx context.Context, kind, source string) (contactport.DeferredIdentityHistoryReceipt, bool, error) {
	if ctx.Value(deferredIdentityHistoryTxKey{}) != true {
		return contactport.DeferredIdentityHistoryReceipt{}, false, ErrConflict
	}
	terminal, found, err := journal.LoadDeferredIdentityHistoryTerminal(ctx, kind, source)
	if err != nil || !found {
		return contactport.DeferredIdentityHistoryReceipt{}, found, err
	}
	receipt, err := deferredIdentityHistoryReceipt(kind, source, terminal)
	return receipt, err == nil, err
}
func (journal *deferredIdentityHistoryJournalFake) RecordDeferredIdentityHistory(ctx context.Context, receipt contactport.DeferredIdentityHistoryReceipt) error {
	if ctx.Value(deferredIdentityHistoryTxKey{}) != true {
		return ErrConflict
	}
	terminal, err := deferredIdentityHistoryTerminal(receipt)
	if err != nil {
		return err
	}
	return journal.RecordDeferredIdentityHistoryTerminal(ctx, receipt.Kind, terminal)
}

type deferredIdentityHistoryWriterFake struct {
	journal                *deferredIdentityHistoryJournalFake
	next                   int64
	writes                 map[string]int
	calledInTx, badReceipt bool
	person                 contactport.HistoricalDeferredPerson
	conflict               contactport.HistoricalDeferredIdentityConflict
	missing                contactport.HistoricalMissingRootIdentity
}

func newDeferredIdentityHistoryWriterFake(journal *deferredIdentityHistoryJournalFake) *deferredIdentityHistoryWriterFake {
	return &deferredIdentityHistoryWriterFake{journal: journal, next: 100, writes: map[string]int{}}
}
func (writer *deferredIdentityHistoryWriterFake) ImportHistoricalDeferredPerson(ctx context.Context, source string, value contactport.HistoricalDeferredPerson) (contactport.DeferredIdentityHistoryReceipt, error) {
	writer.person = value
	return writer.write(ctx, DeferredPersonHistoryKind, source, value.SourcePayloadDigest)
}
func (writer *deferredIdentityHistoryWriterFake) ImportHistoricalDeferredIdentityConflict(ctx context.Context, source string, value contactport.HistoricalDeferredIdentityConflict) (contactport.DeferredIdentityHistoryReceipt, error) {
	writer.conflict = value
	return writer.write(ctx, DeferredConflictHistoryKind, source, value.SourcePayloadDigest)
}
func (writer *deferredIdentityHistoryWriterFake) ImportHistoricalMissingRootIdentity(ctx context.Context, source string, value contactport.HistoricalMissingRootIdentity) (contactport.DeferredIdentityHistoryReceipt, error) {
	writer.missing = value
	return writer.write(ctx, MissingRootIdentityKind, source, value.SourcePayloadDigest)
}
func (writer *deferredIdentityHistoryWriterFake) write(ctx context.Context, kind, source string, payload [32]byte) (contactport.DeferredIdentityHistoryReceipt, error) {
	if ctx.Value(deferredIdentityHistoryTxKey{}) != true {
		return contactport.DeferredIdentityHistoryReceipt{}, errors.New("writer outside transaction")
	}
	writer.calledInTx = true
	if prior, found, err := writer.journal.LoadDeferredIdentityHistory(ctx, kind, source); err != nil || found {
		if err != nil {
			return contactport.DeferredIdentityHistoryReceipt{}, err
		}
		prior.Replayed = true
		return prior, nil
	}
	writer.next++
	writer.writes[kind]++
	receipt := contactport.DeferredIdentityHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetID: writer.next, TargetDigest: sha256.Sum256([]byte(kind + "/" + source))}
	if err := writer.journal.RecordDeferredIdentityHistory(ctx, receipt); err != nil {
		return contactport.DeferredIdentityHistoryReceipt{}, err
	}
	if writer.badReceipt {
		receipt.TargetID++
	}
	return receipt, nil
}
