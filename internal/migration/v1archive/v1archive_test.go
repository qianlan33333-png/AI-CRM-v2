package v1archive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func TestArchiveRecordIsEncryptedRedactedAndDeterministicByDigest(t *testing.T) {
	config := testConfig()
	run := testRun()
	table := testManifest().Tables[0]
	payload := []byte(`{"id":7,"nested":{"access_token":"must-not-persist"},"name":"浅蓝"}`)
	record, err := ArchiveRecord(config, run, table, 1, []byte(`[7]`), payload)
	if err != nil {
		t.Fatalf("archive record: %v", err)
	}
	if string(record.Ciphertext) == string(payload) || containsBytes(record.Ciphertext, []byte("must-not-persist")) {
		t.Fatal("credential plaintext reached archive ciphertext")
	}
	plain, err := DecryptRecord(config.ArchiveKey, record)
	if err != nil {
		t.Fatalf("decrypt record: %v", err)
	}
	want := `{"id":7,"name":"浅蓝","nested":{"access_token":"[REDACTED]"}}`
	if string(plain) != want {
		t.Fatalf("decrypted payload = %s, want %s", plain, want)
	}
	if len(record.RedactedPaths) != 1 || record.RedactedPaths[0] != "nested.access_token" {
		t.Fatalf("redacted paths = %#v", record.RedactedPaths)
	}
	again, err := ArchiveRecord(config, run, table, 1, []byte(`[7]`), payload)
	if err != nil {
		t.Fatal(err)
	}
	if record.SourceKeyHMAC != again.SourceKeyHMAC || record.PayloadHMAC != again.PayloadHMAC || record.FieldHMAC != again.FieldHMAC {
		t.Fatal("archive identity digest is not deterministic")
	}
	if string(record.Nonce) == string(again.Nonce) || string(record.Ciphertext) == string(again.Ciphertext) {
		t.Fatal("AES-GCM nonce/ciphertext unexpectedly repeated")
	}
	other, err := SourceKeyHMAC(config.SourceHMACKey, "other_table", []byte(`[7]`))
	if err != nil {
		t.Fatal(err)
	}
	if other == record.SourceKeyHMAC {
		t.Fatal("source key HMAC is not table-bound")
	}
}

func TestExecuteArchivesOnlyAndReconcilesIdempotently(t *testing.T) {
	manifest := testManifest()
	source := &memorySource{snapshot: &memorySnapshot{identity: manifest.Source, manifest: manifest, rows: map[string][]memoryRow{
		"contacts":     {{key: []byte(`[1]`), payload: []byte(`{"id":1,"name":"one"}`)}, {key: []byte(`[2]`), payload: []byte(`{"id":2,"api_key":"secret"}`)}},
		"runtime_jobs": {},
	}}}
	target := newMemoryTarget(SourceIdentity{SystemID: "target-system", Database: "aicrm", Role: "writer"})
	run := testRun()
	config := testConfig()
	result, err := Execute(context.Background(), config, ModeFull, run, source, target)
	if err != nil {
		t.Fatalf("full archive: %v", err)
	}
	if result.Summary.TotalCount() != 2 || len(target.records) != 2 {
		t.Fatalf("archive result=%#v records=%d", result.Summary, len(target.records))
	}
	// A second full execution with the same signed snapshot only reuses the
	// existing records; the memory target deliberately treats matching digests
	// as an idempotent receipt, not as an overwrite.
	if _, err = Execute(context.Background(), config, ModeFull, run, source, target); err != nil {
		t.Fatalf("idempotent full archive: %v", err)
	}
	if _, err = Execute(context.Background(), config, ModeReconcile, run, source, target); err != nil {
		t.Fatalf("reconcile archive: %v", err)
	}
}

func TestMemoryWriterRejectsDigestDrift(t *testing.T) {
	config := testConfig()
	run := testRun()
	table := testManifest().Tables[0]
	first, err := ArchiveRecord(config, run, table, 1, []byte(`[1]`), []byte(`{"id":1,"name":"one"}`))
	if err != nil {
		t.Fatal(err)
	}
	changed, err := ArchiveRecord(config, run, table, 1, []byte(`[1]`), []byte(`{"id":1,"name":"changed"}`))
	if err != nil {
		t.Fatal(err)
	}
	target := newMemoryTarget(SourceIdentity{SystemID: "target", Database: "aicrm", Role: "writer"})
	if err = target.WriteBatch(context.Background(), []Record{first}); err != nil {
		t.Fatal(err)
	}
	if err = target.WriteBatch(context.Background(), []Record{first}); err != nil {
		t.Fatalf("matching duplicate rejected: %v", err)
	}
	if err = target.WriteBatch(context.Background(), []Record{changed}); !errors.Is(err, ErrPayloadConflict) {
		t.Fatalf("changed duplicate error = %v, want ErrPayloadConflict", err)
	}
}

func TestExecuteRejectsSameDatabaseBeforeWrites(t *testing.T) {
	manifest := testManifest()
	source := &memorySource{snapshot: &memorySnapshot{identity: manifest.Source, manifest: manifest}}
	target := newMemoryTarget(manifest.Source)
	_, err := Execute(context.Background(), testConfig(), ModeFull, testRun(), source, target)
	if !errors.Is(err, ErrSameDatabase) {
		t.Fatalf("same database error = %v", err)
	}
	if target.ensured {
		t.Fatal("same database reached target writer")
	}
}

func testConfig() Config {
	return Config{SourceHMACKey: []byte(strings.Repeat("h", 32)), ArchiveKey: []byte(strings.Repeat("a", 32)), ArchiveKeyVersion: 1, BatchSize: 2}
}

func testRun() Run {
	dump := sha256.Sum256([]byte("dump"))
	schema, _ := testManifest().Digest()
	return Run{ID: "v1-full-20260827", AdapterID: DefaultAdapterID, Source: testManifest().Source, SourceDumpDigest: dump, RepositorySHA: "0123456789abcdef0123456789abcdef01234567", SnapshotDigest: dump, SchemaDigest: schema, PolicyDigest: ArchivePolicyDigest(), ArchiveKeyVersion: 1}
}

func testManifest() Manifest {
	return Manifest{Source: SourceIdentity{SystemID: "legacy-system", Database: "openclaw_wecom", Role: "archive_reader"}, Tables: []Table{
		{Name: "contacts", Columns: []Column{{Ordinal: 1, Name: "id", Type: "bigint", NotNull: true}, {Ordinal: 2, Name: "name", Type: "text", NotNull: false}}, PrimaryKey: []string{"id"}, RowCount: 2},
		{Name: "runtime_jobs", Columns: []Column{{Ordinal: 1, Name: "id", Type: "bigint", NotNull: true}}, PrimaryKey: []string{"id"}, RowCount: 0},
	}}
}

type memoryRow struct{ key, payload []byte }

type memorySource struct{ snapshot *memorySnapshot }

func (source *memorySource) WithSnapshot(_ context.Context, callback func(Snapshot) error) error {
	return callback(source.snapshot)
}
func (*memorySource) Close() {}

type memorySnapshot struct {
	identity SourceIdentity
	manifest Manifest
	rows     map[string][]memoryRow
}

func (snapshot *memorySnapshot) Identity() SourceIdentity { return snapshot.identity }
func (snapshot *memorySnapshot) Manifest(context.Context) (Manifest, error) {
	return snapshot.manifest, nil
}
func (snapshot *memorySnapshot) EachRow(_ context.Context, table Table, callback func([]byte, []byte) error) error {
	for _, row := range snapshot.rows[table.Name] {
		if err := callback(row.key, row.payload); err != nil {
			return err
		}
	}
	return nil
}

type memoryTarget struct {
	identity SourceIdentity
	run      Run
	ensured  bool
	records  map[string]Record
	summary  Summary
}

func newMemoryTarget(identity SourceIdentity) *memoryTarget {
	return &memoryTarget{identity: identity, records: make(map[string]Record)}
}
func (target *memoryTarget) Identity(context.Context) (SourceIdentity, error) {
	return target.identity, nil
}
func (target *memoryTarget) EnsureRun(_ context.Context, run Run, _ Manifest) error {
	target.ensured, target.run = true, run
	return nil
}
func (target *memoryTarget) WriteBatch(_ context.Context, records []Record) error {
	for _, record := range records {
		key := record.Table + ":" + hex.EncodeToString(record.SourceKeyHMAC[:])
		if existing, found := target.records[key]; found {
			if existing.PayloadHMAC != record.PayloadHMAC || existing.FieldHMAC != record.FieldHMAC || existing.SchemaDigest != record.SchemaDigest || existing.Ordinal != record.Ordinal {
				return ErrPayloadConflict
			}
			continue
		}
		target.records[key] = record
	}
	return nil
}
func (target *memoryTarget) Complete(_ context.Context, summary Summary) error {
	target.summary = summary
	return nil
}
func (target *memoryTarget) Run(_ context.Context, runID string) (Run, bool, error) {
	return target.run, target.ensured && target.run.ID == runID, nil
}
func (target *memoryTarget) Summary(_ context.Context, runID string) (Summary, error) {
	if target.summary.RunID != runID {
		return Summary{}, ErrRunConflict
	}
	return target.summary, nil
}
func (target *memoryTarget) Reconcile(_ context.Context, summary Summary) error {
	return RequireReconciled(summary, target.summary)
}

func containsBytes(haystack, needle []byte) bool {
	for index := 0; index+len(needle) <= len(haystack); index++ {
		if string(haystack[index:index+len(needle)]) == string(needle) {
			return true
		}
	}
	return false
}

func TestTableManifestAllowsKeylessFrozenArchive(t *testing.T) {
	table := Table{Name: "no_key", Columns: []Column{{Ordinal: 1, Name: "id", Type: "bigint"}}}
	if err := table.Validate(); err != nil {
		t.Fatalf("keyless table error = %v", err)
	}
}
