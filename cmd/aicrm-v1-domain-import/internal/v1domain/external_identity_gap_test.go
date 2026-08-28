package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1externalidentitygap"
	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const externalIdentityGapFixtureRows = 23936

func TestNewExternalIdentityGapImportJournalPinsOnlyStaticScope(t *testing.T) {
	t.Parallel()
	if _, err := NewExternalIdentityGapImportJournal(nil); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("nil journal must fail without panic: %v", err)
	}
	journal := &Journal{scope: Scope{
		ImportVersion: externalIdentityGapImportVersion,
		ArchiveRunID:  "archive-run",
		AdapterID:     v1archive.DefaultAdapterID,
		TableID:       v1externalidentitygap.TableID,
		TargetDomain:  externalIdentityGapTargetDomain,
		TargetTable:   externalIdentityGapTargetTable,
	}, tx: func(context.Context) (pgx.Tx, error) { return nil, nil }}
	if _, err := NewExternalIdentityGapImportJournal(journal); err != nil {
		t.Fatalf("static scope should construct before runtime keys: %v", err)
	}
	journal.scope.TargetTable = "wrong"
	if _, err := NewExternalIdentityGapImportJournal(journal); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("wrong target scope must fail: %v", err)
	}
}

func TestExternalIdentityGapImporterImportsUnboundAndVerifiedRootsThenReplays(t *testing.T) {
	t.Parallel()
	fixture := newExternalIdentityGapFixture(t)
	selection, err := SelectExternalIdentityGap(context.Background(), fixture.archive, fixture.dm01, fixture.options)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if selection.ArchiveRows != externalIdentityGapFixtureRows || selection.DM01TerminalRows != externalIdentityGapFixtureRows-63 || len(selection.OnlyArchive) != 63 || selection.SummaryDigest == ([sha256.Size]byte{}) {
		t.Fatalf("unexpected selection: %+v", selection)
	}
	if selection.OnlyArchive[0].Fact.UnionID != nil || selection.OnlyArchive[61].Fact.UnionID == nil || selection.OnlyArchive[62].Fact.UnionID == nil {
		t.Fatal("expected 61 unbound and two verified-root candidates")
	}

	importer, err := NewExternalIdentityGapImporter(fixture.archive, fixture.uow, fixture.target, fixture.roots, fixture.dm01, fixture.journal)
	if err != nil {
		t.Fatalf("new importer: %v", err)
	}
	first, err := importer.Import(context.Background(), fixture.options)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if first.Selected != 63 || first.Imported != 63 || first.Replayed != 0 || len(fixture.target.rows) != 63 || len(fixture.journal.rows) != 63 || fixture.target.effects != 0 || fixture.roots.calls != 2 {
		t.Fatalf("unexpected first import: result=%+v target=%d receipts=%d effects=%d roots=%d", first, len(fixture.target.rows), len(fixture.journal.rows), fixture.target.effects, fixture.roots.calls)
	}
	for _, fact := range fixture.target.rows {
		if fact.Assurance != "declared" || fact.Source != "v1.archive_identity_gap" || fact.CustomerID == nil && fact.BoundAt != nil || fact.CustomerID != nil && fact.BoundAt == nil {
			t.Fatalf("target is not a non-actionable archive identity: %+v", fact)
		}
	}

	second, err := importer.Import(context.Background(), fixture.options)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if second.Selected != 63 || second.Imported != 0 || second.Replayed != 63 || len(fixture.target.rows) != 63 || len(fixture.journal.rows) != 63 {
		t.Fatalf("unexpected replay: result=%+v target=%d receipts=%d", second, len(fixture.target.rows), len(fixture.journal.rows))
	}
	if err := importer.Verify(context.Background(), fixture.options); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestExternalIdentityGapImporterFailsClosedAndRollsBack(t *testing.T) {
	t.Parallel()
	fixture := newExternalIdentityGapFixture(t)
	selection, err := SelectExternalIdentityGap(context.Background(), fixture.archive, fixture.dm01, fixture.options)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	t.Run("DM01 collection drift", func(t *testing.T) {
		drifted := append([]DM01ExternalIdentityReceipt(nil), fixture.dm01.rows...)
		drifted = append(drifted, DM01ExternalIdentityReceipt{SourceOrdinal: int64(len(drifted) + 1), SourceKeyHMAC: [sha256.Size]byte{9}, Disposition: "imported"})
		if _, err := SelectExternalIdentityGap(context.Background(), fixture.archive, externalIdentityGapReceiptSource{rows: drifted}, fixture.options); !errors.Is(err, ErrConflict) {
			t.Fatalf("collection drift must fail closed: %v", err)
		}
	})

	t.Run("source envelope drift", func(t *testing.T) {
		drifted := newExternalIdentityGapFixture(t)
		drifted.archive.rows[0].FieldHMAC[0]++
		blocked, err := NewExternalIdentityGapImporter(drifted.archive, drifted.uow, drifted.target, drifted.roots, drifted.dm01, drifted.journal)
		if err != nil {
			t.Fatalf("new importer: %v", err)
		}
		if _, err := blocked.Import(context.Background(), drifted.options); !errors.Is(err, ErrConflict) {
			t.Fatalf("source HMAC drift must fail closed: %v", err)
		}
		if len(drifted.target.rows) != 0 || len(drifted.journal.rows) != 0 {
			t.Fatalf("source drift must not write: target=%d receipts=%d", len(drifted.target.rows), len(drifted.journal.rows))
		}
	})

	t.Run("missing verified root", func(t *testing.T) {
		withoutRoots := *fixture.roots
		withoutRoots.rows = map[[sha256.Size]byte]contactport.CustomerID{}
		blocked, err := NewExternalIdentityGapImporter(fixture.archive, fixture.uow, fixture.target, &withoutRoots, fixture.dm01, fixture.journal)
		if err != nil {
			t.Fatalf("new importer: %v", err)
		}
		if _, err := blocked.importSelection(context.Background(), selection, fixture.options); !errors.Is(err, ErrConflict) {
			t.Fatalf("missing root must fail closed: %v", err)
		}
		if len(fixture.target.rows) != 61 || len(fixture.journal.rows) != 61 {
			t.Fatalf("only completed preceding rows may persist: target=%d receipts=%d", len(fixture.target.rows), len(fixture.journal.rows))
		}
	})

	t.Run("writer journal rollback", func(t *testing.T) {
		fixture := newExternalIdentityGapFixture(t)
		fixture.journal.recordErr = errors.New("receipt unavailable")
		selection, err := SelectExternalIdentityGap(context.Background(), fixture.archive, fixture.dm01, fixture.options)
		if err != nil {
			t.Fatalf("select: %v", err)
		}
		importer, err := NewExternalIdentityGapImporter(fixture.archive, fixture.uow, fixture.target, fixture.roots, fixture.dm01, fixture.journal)
		if err != nil {
			t.Fatalf("new importer: %v", err)
		}
		if _, err := importer.importSelection(context.Background(), selection, fixture.options); err == nil {
			t.Fatal("record failure must rollback target")
		}
		if len(fixture.target.rows) != 0 || len(fixture.journal.rows) != 0 || fixture.target.effects != 0 {
			t.Fatalf("failed transaction persisted a target or effect: targets=%d receipts=%d effects=%d", len(fixture.target.rows), len(fixture.journal.rows), fixture.target.effects)
		}
	})

	t.Run("target drift", func(t *testing.T) {
		fixture := newExternalIdentityGapFixture(t)
		selection, err := SelectExternalIdentityGap(context.Background(), fixture.archive, fixture.dm01, fixture.options)
		if err != nil {
			t.Fatalf("select: %v", err)
		}
		importer, err := NewExternalIdentityGapImporter(fixture.archive, fixture.uow, fixture.target, fixture.roots, fixture.dm01, fixture.journal)
		if err != nil {
			t.Fatalf("new importer: %v", err)
		}
		if _, err := importer.importSelection(context.Background(), selection, fixture.options); err != nil {
			t.Fatalf("import: %v", err)
		}
		for id, row := range fixture.target.rows {
			row.ExternalUserID = "drifted"
			fixture.target.rows[id] = row
			break
		}
		if err := importer.Verify(context.Background(), fixture.options); !errors.Is(err, ErrConflict) {
			t.Fatalf("target drift must fail closed: %v", err)
		}
	})

	t.Run("root receipt binding drift", func(t *testing.T) {
		fixture := newExternalIdentityGapFixture(t)
		selection, err := SelectExternalIdentityGap(context.Background(), fixture.archive, fixture.dm01, fixture.options)
		if err != nil {
			t.Fatalf("select: %v", err)
		}
		importer, err := NewExternalIdentityGapImporter(fixture.archive, fixture.uow, fixture.target, fixture.roots, fixture.dm01, fixture.journal)
		if err != nil {
			t.Fatalf("new importer: %v", err)
		}
		if _, err := importer.importSelection(context.Background(), selection, fixture.options); err != nil {
			t.Fatalf("import: %v", err)
		}
		for source, receipt := range fixture.journal.rows {
			if receipt.Metadata["root_route"] == "verified_root" {
				receipt.Metadata["root_source_hmac"] = strings.Repeat("0", sha256.Size*2)
				fixture.journal.rows[source] = receipt
				break
			}
		}
		if err := importer.Verify(context.Background(), fixture.options); !errors.Is(err, ErrConflict) {
			t.Fatalf("root receipt binding drift must fail closed: %v", err)
		}
	})
}

type externalIdentityGapFixture struct {
	archive *externalIdentityGapArchive
	dm01    externalIdentityGapReceiptSource
	uow     *externalIdentityGapUOW
	target  *externalIdentityGapTarget
	roots   *externalIdentityGapRoots
	journal *externalIdentityGapJournalFake
	options ExternalIdentityGapImportOptions
}

func newExternalIdentityGapFixture(t *testing.T) externalIdentityGapFixture {
	t.Helper()
	options := ExternalIdentityGapImportOptions{
		ArchiveRunID:      "archive-run",
		DM01RunID:         2,
		SourceHMACKey:     []byte("archive-source-key-32-bytes-long!!"),
		DM01SourceHMACKey: []byte("dm01-source-key-32-bytes-long!!!!!"),
		TargetHMACKey:     []byte("target-hmac-key-is-at-least-32-bytes"),
		KeyVersion:        1,
	}
	archive := &externalIdentityGapArchive{rows: make([]v1archive.ArchivedRow, 0, externalIdentityGapFixtureRows)}
	receipts := make([]DM01ExternalIdentityReceipt, 0, externalIdentityGapFixtureRows-63)
	roots := &externalIdentityGapRoots{rows: make(map[[sha256.Size]byte]contactport.CustomerID)}
	for id := int64(1); id <= externalIdentityGapFixtureRows; id++ {
		unionID := ""
		if id > externalIdentityGapFixtureRows-2 {
			unionID = "union-" + strconv.FormatInt(id, 10)
			digest, err := contactmigration.SourceKeyHMAC(options.DM01SourceHMACKey, dm01CustomerIdentitySourceTable, unionID)
			if err != nil || len(digest) != sha256.Size {
				t.Fatalf("root digest: %v", err)
			}
			var key [sha256.Size]byte
			copy(key[:], digest)
			roots.rows[key] = contactport.CustomerID(id)
		}
		archive.rows = append(archive.rows, externalIdentityGapArchiveRow(t, options.SourceHMACKey, id, id, unionID))
		if id <= externalIdentityGapFixtureRows-63 {
			digest, err := contactmigration.SourceKeyHMAC(options.DM01SourceHMACKey, "wecom_external_contact_identity_map", strconv.FormatInt(id, 10))
			if err != nil || len(digest) != sha256.Size {
				t.Fatalf("DM01 source digest: %v", err)
			}
			var key [sha256.Size]byte
			copy(key[:], digest)
			receipts = append(receipts, DM01ExternalIdentityReceipt{SourceOrdinal: id, SourceKeyHMAC: key, Disposition: "imported"})
		}
	}
	target := &externalIdentityGapTarget{rows: make(map[int64]identityport.ArchiveIdentityFact), nextID: 1}
	journal := &externalIdentityGapJournalFake{rows: make(map[string]TerminalReceipt), options: options}
	uow := &externalIdentityGapUOW{target: target, journal: journal}
	return externalIdentityGapFixture{archive: archive, dm01: externalIdentityGapReceiptSource{rows: receipts}, uow: uow, target: target, roots: roots, journal: journal, options: options}
}

func externalIdentityGapArchiveRow(t *testing.T, key []byte, ordinal, id int64, unionID string) v1archive.ArchivedRow {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"id":              id,
		"corp_id":         "corp-a",
		"external_userid": "external-" + strconv.FormatInt(id, 10),
		"unionid":         unionID,
		"updated_at":      "2026-08-28T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical, fields, err := v1archive.RedactPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	source, err := v1archive.SourceKeyHMAC(key, "wecom_external_contact_identity_map", []byte("["+strconv.FormatInt(id, 10)+"]"))
	if err != nil {
		t.Fatal(err)
	}
	payloadHMAC, err := v1archive.PayloadHMAC(key, "wecom_external_contact_identity_map", canonical)
	if err != nil {
		t.Fatal(err)
	}
	fieldHMAC, err := v1archive.FieldHMAC(key, "wecom_external_contact_identity_map", fields)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: dm01ExternalIdentityArchiveTableID, SourceOrdinal: ordinal, SourceKeyHMAC: source, PayloadHMAC: payloadHMAC, FieldHMAC: fieldHMAC, Payload: canonical, RedactedFields: fields}
}

type externalIdentityGapArchive struct{ rows []v1archive.ArchivedRow }

func (archive *externalIdentityGapArchive) EachTableRow(_ context.Context, run, table string, callback func(v1archive.ArchivedRow) error) error {
	if run != "archive-run" || table != v1externalidentitygap.TableID {
		return ErrInvalidScope
	}
	for _, row := range archive.rows {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type externalIdentityGapReceiptSource struct{ rows []DM01ExternalIdentityReceipt }

func (source externalIdentityGapReceiptSource) EachDM01ExternalIdentityReceipt(_ context.Context, run int64, callback func(DM01ExternalIdentityReceipt) error) error {
	if run != 2 {
		return ErrInvalidScope
	}
	for _, row := range source.rows {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type externalIdentityGapTxKey struct{}

type externalIdentityGapUOW struct {
	target  *externalIdentityGapTarget
	journal *externalIdentityGapJournalFake
}

func (uow *externalIdentityGapUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	targetRows, nextID := cloneExternalIdentityGapTargets(uow.target.rows), uow.target.nextID
	journalRows := cloneExternalIdentityGapReceipts(uow.journal.rows)
	err := callback(context.WithValue(ctx, externalIdentityGapTxKey{}, true))
	if err != nil {
		uow.target.rows, uow.target.nextID, uow.journal.rows = targetRows, nextID, journalRows
	}
	return err
}

type externalIdentityGapTarget struct {
	rows    map[int64]identityport.ArchiveIdentityFact
	nextID  int64
	effects int
}

func (target *externalIdentityGapTarget) ImportArchiveWeComIdentity(ctx context.Context, input identityport.ArchiveIdentityInput) (identityport.ArchiveIdentityFact, error) {
	if ctx.Value(externalIdentityGapTxKey{}) != true {
		return identityport.ArchiveIdentityFact{}, ErrConflict
	}
	if input.CustomerID != nil && *input.CustomerID < 1 {
		return identityport.ArchiveIdentityFact{}, ErrConflict
	}
	id := target.nextID
	target.nextID++
	createdAt := time.Date(2026, 8, 28, 12, 0, 0, 123456000, time.UTC)
	fact := identityport.ArchiveIdentityFact{ID: id, CustomerID: copyExternalIdentityGapCustomer(input.CustomerID), Scope: input.Scope, ExternalUserID: input.ExternalUserID, HMACKeyVersion: input.HMACKeyVersion, Assurance: "declared", Source: "v1.archive_identity_gap", NormalizerVersion: 1, CreatedAt: createdAt}
	copy(fact.ReviewFingerprint[:], input.SourceKeyHMAC[:16])
	if input.CustomerID != nil {
		boundAt := createdAt
		fact.BoundAt = &boundAt
	}
	target.rows[id] = fact
	return cloneExternalIdentityGapFact(fact), nil
}

func (target *externalIdentityGapTarget) ReadArchiveWeComIdentity(ctx context.Context, id int64) (identityport.ArchiveIdentityFact, error) {
	if ctx.Value(externalIdentityGapTxKey{}) != true {
		return identityport.ArchiveIdentityFact{}, ErrConflict
	}
	fact, found := target.rows[id]
	if !found {
		return identityport.ArchiveIdentityFact{}, ErrConflict
	}
	return cloneExternalIdentityGapFact(fact), nil
}

type externalIdentityGapRoots struct {
	rows  map[[sha256.Size]byte]contactport.CustomerID
	calls int
	err   error
}

func (roots *externalIdentityGapRoots) LockVerifiedDM01CustomerRoot(ctx context.Context, runID int64, key [sha256.Size]byte) (contactport.CustomerID, bool, error) {
	if ctx.Value(externalIdentityGapTxKey{}) != true || runID != 2 {
		return 0, false, ErrConflict
	}
	roots.calls++
	if roots.err != nil {
		return 0, false, roots.err
	}
	id, found := roots.rows[key]
	return id, found, nil
}

type externalIdentityGapJournalFake struct {
	rows      map[string]TerminalReceipt
	options   ExternalIdentityGapImportOptions
	recordErr error
}

func (journal *externalIdentityGapJournalFake) LoadTerminal(ctx context.Context, source string) (TerminalReceipt, bool, error) {
	if ctx.Value(externalIdentityGapTxKey{}) != true {
		return TerminalReceipt{}, false, ErrConflict
	}
	receipt, found := journal.rows[source]
	return cloneExternalIdentityGapReceipt(receipt), found, nil
}

func (journal *externalIdentityGapJournalFake) Record(ctx context.Context, receipt TerminalReceipt) error {
	if ctx.Value(externalIdentityGapTxKey{}) != true {
		return ErrConflict
	}
	if journal.recordErr != nil {
		return journal.recordErr
	}
	source := SourceIdentifier(receipt.SourceKeyDigest)
	if _, found := journal.rows[source]; found {
		return ErrConflict
	}
	journal.rows[source] = cloneExternalIdentityGapReceipt(receipt)
	return nil
}

func (journal *externalIdentityGapJournalFake) ValidateExternalIdentityGapScope(options ExternalIdentityGapImportOptions) error {
	if options.ArchiveRunID != journal.options.ArchiveRunID || options.DM01RunID != journal.options.DM01RunID || options.KeyVersion != journal.options.KeyVersion || string(options.SourceHMACKey) != string(journal.options.SourceHMACKey) || string(options.DM01SourceHMACKey) != string(journal.options.DM01SourceHMACKey) || string(options.TargetHMACKey) != string(journal.options.TargetHMACKey) {
		return ErrInvalidScope
	}
	return nil
}

func cloneExternalIdentityGapTargets(values map[int64]identityport.ArchiveIdentityFact) map[int64]identityport.ArchiveIdentityFact {
	copy := make(map[int64]identityport.ArchiveIdentityFact, len(values))
	for id, value := range values {
		copy[id] = cloneExternalIdentityGapFact(value)
	}
	return copy
}

func cloneExternalIdentityGapFact(value identityport.ArchiveIdentityFact) identityport.ArchiveIdentityFact {
	value.CustomerID = copyExternalIdentityGapCustomer(value.CustomerID)
	if value.BoundAt != nil {
		boundAt := *value.BoundAt
		value.BoundAt = &boundAt
	}
	return value
}

func copyExternalIdentityGapCustomer(value *contactport.CustomerID) *contactport.CustomerID {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneExternalIdentityGapReceipts(values map[string]TerminalReceipt) map[string]TerminalReceipt {
	copy := make(map[string]TerminalReceipt, len(values))
	for source, value := range values {
		copy[source] = cloneExternalIdentityGapReceipt(value)
	}
	return copy
}

func cloneExternalIdentityGapReceipt(value TerminalReceipt) TerminalReceipt {
	if value.Metadata == nil {
		return value
	}
	metadata := make(map[string]any, len(value.Metadata))
	for key, item := range value.Metadata {
		metadata[key] = item
	}
	value.Metadata = metadata
	return value
}
