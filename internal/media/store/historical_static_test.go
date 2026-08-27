package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	media "github.com/qianlan33333-png/AI-CRM-v2/internal/media"
)

type historicalStaticTx struct {
	pgx.Tx
	query string
	args  []any
	id    int64
	err   error
}

func (tx *historicalStaticTx) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	tx.query, tx.args = query, args
	return tx
}

func (tx *historicalStaticTx) Scan(dest ...any) error {
	if tx.err != nil {
		return tx.err
	}
	*dest[0].(*int64) = tx.id
	return nil
}

func historicalStaticStoreFixtures(t *testing.T) (media.HistoricalImageDefinition, media.HistoricalAttachmentDefinition) {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	origin := media.HistoricalStaticOrigin{SourceIdentifier: "image_library:5", SourceID: 5, PayloadDigest: sha256.Sum256([]byte("source"))}
	definition, err := media.AdaptV1ImageLibrary(media.V1ImageLibraryRow{ID: 5, Name: "image", FileName: "image.png", MimeType: "image/png",
		FileSize: int64(buffer.Len()), DataBase64: base64.StdEncoding.EncodeToString(buffer.Bytes()), Enabled: true, CreatedAt: now, UpdatedAt: now}, origin, 7)
	if err != nil {
		t.Fatal(err)
	}
	pdf := []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF\n")
	origin.SourceIdentifier = "attachment_library:5"
	attachment, err := media.AdaptV1AttachmentLibrary(media.V1AttachmentLibraryRow{ID: 5, Name: "PDF", FileName: "document.pdf", MimeType: "application/pdf",
		FileSize: int64(len(pdf)), DataBase64: base64.StdEncoding.EncodeToString(pdf), Enabled: true, CreatedAt: now, UpdatedAt: now}, origin, 7)
	if err != nil {
		t.Fatal(err)
	}
	return definition, attachment
}

func TestHistoricalStaticStoreAtomicallyInsertsDisabledMetadataAndBlob(t *testing.T) {
	definition, attachment := historicalStaticStoreFixtures(t)
	for _, kind := range []string{"image", "attachment"} {
		t.Run(kind, func(t *testing.T) {
			tx := &historicalStaticTx{id: 99}
			store := &HistoricalStaticStore{tx: func(context.Context) (pgx.Tx, error) { return tx, nil }}
			var id int64
			var err error
			fragments := []string{"WITH inserted AS", "FALSE"}
			if kind == "image" {
				fragments = append(fragments, "INSERT INTO public.media_images", "INSERT INTO public.media_image_blobs")
				id, err = store.InsertHistoricalImage(context.Background(), definition)
				if len(tx.args) != 14 || !bytes.Equal(tx.args[6].([]byte), definition.Checksum[:]) || !bytes.Equal(tx.args[13].([]byte), definition.Content) || tx.args[10] != int64(7) {
					t.Fatalf("wrong image args: %v", tx.args)
				}
			} else {
				fragments = append(fragments, "INSERT INTO public.media_attachments", "INSERT INTO public.media_attachment_blobs")
				id, err = store.InsertHistoricalAttachment(context.Background(), attachment)
				if len(tx.args) != 11 || !bytes.Equal(tx.args[4].([]byte), attachment.Checksum[:]) || !bytes.Equal(tx.args[10].([]byte), attachment.Content) || tx.args[7] != int64(7) || string(tx.args[6].([]byte)) != "[]" {
					t.Fatalf("wrong attachment args: %v", tx.args)
				}
				if !strings.Contains(tx.query, "FALSE,1") {
					t.Fatal("historical attachment must start at disabled version 1")
				}
			}
			if err != nil || id != 99 {
				t.Fatalf("id=%d err=%v", id, err)
			}
			for _, fragment := range fragments {
				if !strings.Contains(tx.query, fragment) {
					t.Fatalf("missing %q: %s", fragment, tx.query)
				}
			}
			for _, forbidden := range []string{"UPDATE ", "ON CONFLICT", "receipt", "event", "river", "provider", "variant", "media_id", "expires"} {
				if strings.Contains(strings.ToLower(tx.query), strings.ToLower(forbidden)) {
					t.Fatalf("historical insert contains %q", forbidden)
				}
			}
		})
	}
}

func TestHistoricalStaticStoreRejectsMutationBeforeOpeningTransaction(t *testing.T) {
	definition, attachment := historicalStaticStoreFixtures(t)
	store := &HistoricalStaticStore{tx: func(context.Context) (pgx.Tx, error) {
		t.Fatal("invalid definition reached transaction")
		return nil, nil
	}}
	definition.Image.Enabled = true
	if _, err := store.InsertHistoricalImage(context.Background(), definition); !errors.Is(err, media.ErrHistoricalStaticInvalid) {
		t.Fatal(err)
	}
	definition.Image.Enabled = false
	definition.Checksum[0]++
	if _, err := store.InsertHistoricalImage(context.Background(), definition); !errors.Is(err, media.ErrHistoricalStaticInvalid) {
		t.Fatal(err)
	}
	attachment.Attachment.Enabled = true
	if _, err := store.InsertHistoricalAttachment(context.Background(), attachment); !errors.Is(err, media.ErrHistoricalStaticInvalid) {
		t.Fatal(err)
	}
	attachment.Attachment.Enabled = false
	attachment.Content[0]++
	if _, err := store.InsertHistoricalAttachment(context.Background(), attachment); !errors.Is(err, media.ErrHistoricalStaticInvalid) {
		t.Fatal(err)
	}
}

func TestHistoricalStaticStoreNeedsCallerTransactionAndReturnsErrors(t *testing.T) {
	definition, attachment := historicalStaticStoreFixtures(t)
	store := NewHistoricalStaticStore()
	if _, err := store.InsertHistoricalImage(context.Background(), definition); err == nil {
		t.Fatal("insert without UnitOfWork succeeded")
	}
	if _, err := store.InsertHistoricalAttachment(context.Background(), attachment); err == nil {
		t.Fatal("insert without UnitOfWork succeeded")
	}
	failure := errors.New("database failure")
	for _, kind := range []string{"image", "attachment"} {
		for _, stage := range []string{"tx", "query", "conflict", "invalid-id"} {
			t.Run(kind+"/"+stage, func(t *testing.T) {
				tx := &historicalStaticTx{id: 3}
				want := failure
				var txErr error
				switch stage {
				case "tx":
					txErr = failure
				case "query":
					tx.err = failure
				case "conflict":
					tx.err = &pgconn.PgError{Code: "23505"}
					want = media.ErrHistoricalStaticConflict
				case "invalid-id":
					tx.id = 0
					want = media.ErrHistoricalStaticInvalid
				}
				store := &HistoricalStaticStore{tx: func(context.Context) (pgx.Tx, error) { return tx, txErr }}
				var id int64
				var err error
				if kind == "image" {
					id, err = store.InsertHistoricalImage(context.Background(), definition)
				} else {
					id, err = store.InsertHistoricalAttachment(context.Background(), attachment)
				}
				if !errors.Is(err, want) || id != 0 {
					t.Fatalf("id=%d err=%v want=%v", id, err, want)
				}
			})
		}
	}
}
