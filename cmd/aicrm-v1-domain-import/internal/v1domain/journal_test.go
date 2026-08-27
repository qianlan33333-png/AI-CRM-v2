package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestSourceIdentifierRoundTrip(t *testing.T) {
	digest := sha256.Sum256([]byte("source"))
	encoded := SourceIdentifier(digest)
	decoded, err := ParseSourceIdentifier(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != digest {
		t.Fatalf("decoded digest = %x, want %x", decoded, digest)
	}
}

type journalTestRow func(...any) error

func (row journalTestRow) Scan(values ...any) error { return row(values...) }

type journalTestTx struct {
	pgx.Tx
	execs []string
	rows  []journalTestRow
}

func (tx *journalTestTx) Exec(_ context.Context, query string, _ ...any) (pgconn.CommandTag, error) {
	tx.execs = append(tx.execs, query)
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (tx *journalTestTx) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	row := tx.rows[0]
	tx.rows = tx.rows[1:]
	return row
}

func testJournal(tx *journalTestTx) *Journal {
	return &Journal{scope: Scope{ImportVersion: "v1-domain-a1", ArchiveRunID: "archive-run", AdapterID: "adapter", TableID: "public/questionnaires", TargetDomain: "survey", TargetTable: "questionnaires"},
		tx: func(context.Context) (pgx.Tx, error) { return tx, nil }}
}

func TestLoadTerminalLocksMissingSourceBeforeTargetWrite(t *testing.T) {
	tx := &journalTestTx{rows: []journalTestRow{func(...any) error { return pgx.ErrNoRows }}}
	_, found, err := testJournal(tx).LoadTerminal(context.Background(), SourceIdentifier(sha256.Sum256([]byte("source"))))
	if err != nil || found || len(tx.execs) != 1 || !strings.Contains(tx.execs[0], "pg_advisory_xact_lock") {
		t.Fatalf("load: found=%v err=%v execs=%v", found, err, tx.execs)
	}
}

func TestRecordReplaysWithoutInsertAndUsesJSONBMetadataEquality(t *testing.T) {
	payload := sha256.Sum256([]byte("payload"))
	for _, matches := range []bool{true, false} {
		tx := &journalTestTx{rows: []journalTestRow{
			func(values ...any) error {
				*values[0].(*[]byte) = payload[:]
				*values[1].(*string) = "quarantine"
				*values[2].(*string) = "unresolved"
				*values[7].(*[]byte) = []byte(`{"source_id": "9007199254740993", "count": 1}`)
				*values[8].(*bool) = true
				return nil
			},
			func(values ...any) error {
				*values[0].(*[]byte) = payload[:]
				*values[1].(*string) = "quarantine"
				*values[2].(*string) = "unresolved"
				*values[5].(*bool) = matches
				return nil
			},
		}}
		err := testJournal(tx).Record(context.Background(), TerminalReceipt{
			SourceKeyDigest: sha256.Sum256([]byte("source")), PayloadDigest: payload,
			Disposition: "quarantine", Reason: "unresolved", Metadata: map[string]any{"source_id": "9007199254740993", "count": 1},
		})
		if matches && err != nil || !matches && !errors.Is(err, ErrConflict) {
			t.Fatalf("metadata matches=%v, err=%v", matches, err)
		}
		if len(tx.execs) != 1 || !strings.Contains(tx.execs[0], "pg_advisory_xact_lock") {
			t.Fatalf("replay must not INSERT into a sealed journal: %v", tx.execs)
		}
	}
}

func TestNilReceiptMetadataIsJSONObject(t *testing.T) {
	encoded, err := marshalReceiptMetadata(nil)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err = json.Unmarshal(encoded, &value); err != nil {
		t.Fatal(err)
	}
	if _, ok := value.(map[string]any); !ok {
		t.Fatalf("metadata = %s, want JSON object", encoded)
	}
}

func TestNewJournalRejectsUnsafeScope(t *testing.T) {
	for _, scope := range []Scope{
		{},
		{ImportVersion: "v1", ArchiveRunID: "run", AdapterID: "adapter", TableID: "public/campaigns", TargetDomain: "campaign", TargetTable: "cloud-campaigns"},
		{ImportVersion: "../v1", ArchiveRunID: "run", AdapterID: "adapter", TableID: "public/campaigns", TargetDomain: "campaign", TargetTable: "cloud_campaigns"},
	} {
		if _, err := NewJournal(scope); err == nil {
			t.Fatalf("scope %#v unexpectedly accepted", scope)
		}
	}
}
