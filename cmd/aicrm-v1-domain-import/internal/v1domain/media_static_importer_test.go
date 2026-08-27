package v1domain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	media "github.com/qianlan33333-png/AI-CRM-v2/internal/media"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

// The fake implements only the journal's existing queries, with one source row.
// Store and journal both require the context supplied by the same UnitOfWork.
type mediaStaticTxKey struct{}
type mediaStaticTestDB struct {
	pgx.Tx
	stored                                      []any
	images                                      []media.HistoricalImageDefinition
	attachments                                 []media.HistoricalAttachmentDefinition
	storeErr, recordErr                         error
	storeCalls, recordCalls, commits, rollbacks int
	lockKeys                                    []string
}

func (db *mediaStaticTestDB) Within(ctx context.Context, callback func(context.Context) error) error {
	stored, images, attachments := db.stored, len(db.images), len(db.attachments)
	err := callback(context.WithValue(ctx, mediaStaticTxKey{}, db))
	if err != nil {
		db.stored, db.images, db.attachments = stored, db.images[:images], db.attachments[:attachments]
		db.rollbacks++
		return err
	}
	db.commits++
	return nil
}

func (db *mediaStaticTestDB) InsertHistoricalImage(ctx context.Context, value media.HistoricalImageDefinition) (int64, error) {
	if ctx.Value(mediaStaticTxKey{}) != db {
		return 0, errors.New("missing transaction")
	}
	db.storeCalls++
	db.images = append(db.images, value)
	return 901, db.storeErr
}

func (db *mediaStaticTestDB) InsertHistoricalAttachment(ctx context.Context, value media.HistoricalAttachmentDefinition) (int64, error) {
	if ctx.Value(mediaStaticTxKey{}) != db {
		return 0, errors.New("missing transaction")
	}
	db.storeCalls++
	db.attachments = append(db.attachments, value)
	return 902, db.storeErr
}

func (db *mediaStaticTestDB) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	if ctx.Value(mediaStaticTxKey{}) != db {
		return pgconn.CommandTag{}, errors.New("missing transaction")
	}
	if strings.Contains(query, "pg_advisory_xact_lock") {
		db.lockKeys = append(db.lockKeys, args[0].(string))
	} else {
		db.recordCalls++
		if db.recordErr != nil {
			return pgconn.CommandTag{}, db.recordErr
		}
		db.stored = args
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (db *mediaStaticTestDB) QueryRow(ctx context.Context, _ string, args ...any) pgx.Row {
	return journalTestRow(func(values ...any) error {
		if ctx.Value(mediaStaticTxKey{}) != db {
			return errors.New("missing transaction")
		}
		if db.stored == nil || !reflect.DeepEqual(args[:5], db.stored[:5]) {
			return pgx.ErrNoRows
		}
		*values[0].(*[]byte), *values[1].(*string), *values[2].(*string) = db.stored[5].([]byte), db.stored[6].(string), db.stored[7].(string)
		putString := func(index, storedIndex int) {
			if db.stored[storedIndex] != nil {
				value := db.stored[storedIndex].(string)
				*values[index].(**string) = &value
			}
		}
		if len(values) == 9 {
			putString(3, 8)
			putString(4, 9)
			putString(5, 10)
			if db.stored[11] != nil {
				*values[6].(*[]byte) = db.stored[11].([]byte)
			}
			*values[7].(*[]byte), *values[8].(*bool) = db.stored[12].([]byte), true
		} else {
			putString(3, 10)
			if db.stored[11] != nil {
				*values[4].(*[]byte) = db.stored[11].([]byte)
			}
			var actual, expected any
			if err := json.Unmarshal(db.stored[12].([]byte), &actual); err != nil {
				return err
			}
			if err := json.Unmarshal(args[5].([]byte), &expected); err != nil {
				return err
			}
			*values[5].(*bool) = reflect.DeepEqual(actual, expected)
		}
		return nil
	})
}

type mediaStaticArchive struct {
	row   v1archive.ArchivedRow
	calls int
}

func (archive *mediaStaticArchive) EachTableRow(_ context.Context, run, table string, callback func(v1archive.ArchivedRow) error) error {
	archive.calls++
	if run != "archive-run" || (table != "public/image_library" && table != "public/attachment_library") {
		return ErrInvalidScope
	}
	return callback(archive.row)
}

func mediaStaticFixture(t *testing.T, kind media.HistoricalStaticKind) (v1archive.ArchivedRow, []byte) {
	t.Helper()
	content, mime, file := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\n"), "application/pdf", "static.pdf"
	if kind == media.HistoricalImage {
		var buffer bytes.Buffer
		if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
			t.Fatal(err)
		}
		content, mime, file = buffer.Bytes(), "image/png", "static.png"
	}
	when := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	source := mediaStaticJSON{ID: 41, Name: "archived static", FileName: file, MimeType: mime, FileSize: int64(len(content)), DataBase64: base64.StdEncoding.EncodeToString(content),
		Tags: []string{"archived"}, SourceURL: "https://invalid.example/do-not-fetch", ThumbMediaID: "old-thumb-id", ThumbMediaExpiresAt: &when,
		MediaID: "old-provider-id", MediaExpiresAt: &when, Enabled: true, CreatedAt: when, UpdatedAt: when}
	payload, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: "public/" + string(kind), SourceOrdinal: 1,
		SourceKeyHMAC: sha256.Sum256([]byte("source-key")), PayloadHMAC: sha256.Sum256(payload), Payload: payload}, content
}

func mediaStaticImporterFixture(t *testing.T, kind media.HistoricalStaticKind, row v1archive.ArchivedRow) (*MediaStaticImporter, *mediaStaticTestDB) {
	t.Helper()
	db := &mediaStaticTestDB{}
	target := "media_images"
	if kind == media.HistoricalAttachment {
		target = "media_attachments"
	}
	journal := &Journal{scope: Scope{ImportVersion: "v1-static-a1", ArchiveRunID: "archive-run", AdapterID: v1archive.DefaultAdapterID,
		TableID: "public/" + string(kind), TargetDomain: "media", TargetTable: target}, tx: func(ctx context.Context) (pgx.Tx, error) {
		if ctx.Value(mediaStaticTxKey{}) != db {
			return nil, errors.New("missing transaction")
		}
		return db, nil
	}}
	writer, err := media.NewHistoricalStaticWriter(db, journal)
	if err != nil {
		t.Fatal(err)
	}
	importer, err := NewMediaStaticImporter(&mediaStaticArchive{row: row}, db, writer, journal, kind, 7)
	if err != nil {
		t.Fatal(err)
	}
	return importer, db
}

func TestMediaStaticImporterPreservesBytesDisabledAndReplays(t *testing.T) {
	for _, kind := range []media.HistoricalStaticKind{media.HistoricalImage, media.HistoricalAttachment} {
		t.Run(string(kind), func(t *testing.T) {
			row, content := mediaStaticFixture(t, kind)
			original := append([]byte(nil), row.Payload...)
			importer, db := mediaStaticImporterFixture(t, kind, row)
			for attempt := 0; attempt < 2; attempt++ {
				result, err := importer.Import(context.Background(), "archive-run")
				if err != nil || result != (StaticImportResult{Imported: 1, Replayed: attempt}) {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			}
			if db.storeCalls != 1 || db.recordCalls != 1 || db.commits != 2 || !bytes.Equal(row.Payload, original) {
				t.Fatal("replay wrote targets/receipts or changed archive")
			}
			if kind == media.HistoricalImage {
				value := db.images[0]
				if value.Image.Enabled || value.Image.ID != 0 || value.Actor != 7 || !bytes.Equal(value.Content, content) || value.Checksum != sha256.Sum256(content) || !value.ProviderMaterialDropped {
					t.Fatalf("unsafe image: %+v", value)
				}
			} else {
				value := db.attachments[0]
				if value.Attachment.Enabled || value.Attachment.ID != 0 || value.Attachment.CreatedBy != 7 || value.Attachment.Version != 1 || !bytes.Equal(value.Content, content) || value.Checksum != sha256.Sum256(content) || !value.ProviderMaterialDropped {
					t.Fatalf("unsafe PDF: %+v", value)
				}
			}
			if db.stored[10] == "41" || strings.Contains(string(db.stored[12].([]byte)), "old-") || strings.Contains(string(db.stored[12].([]byte)), "https:") {
				t.Fatal("source ID/provider material copied to target receipt")
			}
			wantLock := strings.Join([]string{"v1-static-a1", "archive-run", v1archive.DefaultAdapterID, row.TableID, SourceIdentifier(row.SourceKeyHMAC)}, "/")
			for _, key := range db.lockKeys {
				if key != wantLock {
					t.Fatalf("wrong scoped lock: %s", key)
				}
			}
		})
	}
}

func TestMediaStaticImporterQuarantinesOnlyInvalidSourceAndReplays(t *testing.T) {
	for _, input := range []string{"invalid-json", "invalid-base64", "missing-bytes", "wrong-size", "wrong-mime", "redacted-bytes"} {
		t.Run(input, func(t *testing.T) {
			row, _ := mediaStaticFixture(t, media.HistoricalImage)
			var source mediaStaticJSON
			if err := json.Unmarshal(row.Payload, &source); err != nil {
				t.Fatal(err)
			}
			switch input {
			case "invalid-base64":
				source.DataBase64 = "%%%"
			case "missing-bytes":
				source.DataBase64 = ""
			case "wrong-size":
				source.FileSize++
			case "wrong-mime":
				source.MimeType = "application/pdf"
			case "redacted-bytes":
				row.RedactedFields = []string{"data_base64"}
			}
			row.Payload, _ = json.Marshal(source)
			if input == "invalid-json" {
				row.Payload = []byte("{")
			}
			row.PayloadHMAC = sha256.Sum256(row.Payload)
			importer, db := mediaStaticImporterFixture(t, media.HistoricalImage, row)
			for attempt := 0; attempt < 2; attempt++ {
				result, err := importer.Import(context.Background(), "archive-run")
				if err != nil || result != (StaticImportResult{Quarantined: 1, Replayed: attempt}) {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			}
			if db.storeCalls != 0 || db.recordCalls != 1 || db.stored[6] != "quarantine" || db.stored[10] != nil {
				t.Fatal("invalid source wrote formal target")
			}
		})
	}
}

func TestMediaStaticImporterWriterFailuresAlwaysRollback(t *testing.T) {
	for _, failure := range []string{"owner-invalid-after-write", "store-error", "receipt-error"} {
		t.Run(failure, func(t *testing.T) {
			row, _ := mediaStaticFixture(t, media.HistoricalImage)
			importer, db := mediaStaticImporterFixture(t, media.HistoricalImage, row)
			want := errors.New("write failed")
			switch failure {
			case "owner-invalid-after-write":
				want = media.ErrHistoricalStaticInvalid
				db.storeErr = want
			case "store-error":
				db.storeErr = want
			case "receipt-error":
				db.recordErr = want
			}
			result, err := importer.Import(context.Background(), "archive-run")
			if !errors.Is(err, want) || result != (StaticImportResult{}) || db.rollbacks != 1 || db.commits != 0 || len(db.images) != 0 || db.stored != nil {
				t.Fatalf("not rolled back: result=%+v err=%v db=%+v", result, err, db)
			}
			if failure != "receipt-error" && db.recordCalls != 0 {
				t.Fatal("writer failure wrongly recorded as quarantine")
			}
		})
	}
}

func TestMediaStaticImporterQuarantinesUnsupportedOrIncompleteAttachment(t *testing.T) {
	for _, content := range [][]byte{[]byte("not a PDF"), []byte("%PDF-1.7\ntruncated before EOF")} {
		row, _ := mediaStaticFixture(t, media.HistoricalAttachment)
		var source mediaStaticJSON
		if err := json.Unmarshal(row.Payload, &source); err != nil {
			t.Fatal(err)
		}
		source.DataBase64, source.FileSize = base64.StdEncoding.EncodeToString(content), int64(len(content))
		row.Payload, _ = json.Marshal(source)
		row.PayloadHMAC = sha256.Sum256(row.Payload)
		importer, db := mediaStaticImporterFixture(t, media.HistoricalAttachment, row)
		result, err := importer.Import(context.Background(), "archive-run")
		if err != nil || result != (StaticImportResult{Quarantined: 1}) || db.storeCalls != 0 {
			t.Fatalf("incomplete/unsupported PDF accepted: result=%+v err=%v", result, err)
		}
	}
}

func TestMediaStaticImporterRejectsScopeAndUnauthenticatedRows(t *testing.T) {
	for _, failure := range []string{"table", "adapter", "source-hmac", "payload-hmac", "ordinal", "run", "mutated-scope", "nil-context"} {
		t.Run(failure, func(t *testing.T) {
			row, _ := mediaStaticFixture(t, media.HistoricalImage)
			switch failure {
			case "table":
				row.TableID = "public/image_library_variants"
			case "adapter":
				row.AdapterID = "different"
			case "source-hmac":
				row.SourceKeyHMAC = [32]byte{}
			case "payload-hmac":
				row.PayloadHMAC = [32]byte{}
			case "ordinal":
				row.SourceOrdinal = 0
			}
			importer, db := mediaStaticImporterFixture(t, media.HistoricalImage, row)
			run, ctx := "archive-run", context.Background()
			if failure == "run" {
				run = "different"
			}
			if failure == "mutated-scope" {
				importer.journal.scope.TargetTable = "media_attachments"
			}
			if failure == "nil-context" {
				ctx = nil
			}
			result, err := importer.Import(ctx, run)
			if err == nil || result != (StaticImportResult{}) || db.commits != 0 || db.rollbacks != 0 || db.storeCalls != 0 || db.recordCalls != 0 {
				t.Fatalf("unsafe row accepted: result=%+v err=%v", result, err)
			}
		})
	}
}

func TestMediaStaticImporterConflictingQuarantineReplayRollsBack(t *testing.T) {
	row, _ := mediaStaticFixture(t, media.HistoricalImage)
	row.Payload = []byte("{")
	importer, db := mediaStaticImporterFixture(t, media.HistoricalImage, row)
	if _, err := importer.Import(context.Background(), "archive-run"); err != nil {
		t.Fatal(err)
	}
	importer.archive.(*mediaStaticArchive).row.PayloadHMAC = sha256.Sum256([]byte("different immutable payload"))
	result, err := importer.Import(context.Background(), "archive-run")
	if !errors.Is(err, ErrConflict) || result != (StaticImportResult{}) || db.recordCalls != 1 || db.rollbacks != 1 {
		t.Fatalf("quarantine overwritten: result=%+v err=%v", result, err)
	}
}
