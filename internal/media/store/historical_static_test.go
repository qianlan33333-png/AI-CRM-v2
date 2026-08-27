package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	media "github.com/qianlan33333-png/AI-CRM-v2/internal/media"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
)

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
			query := &historicalStaticQueriesFake{imageID: 99, attachmentID: 99}
			store := historicalStaticStoreFake(query)
			var id int64
			var err error
			if kind == "image" {
				id, err = store.InsertHistoricalImage(context.Background(), definition)
				params := query.imageParams
				if query.imageCalls != 1 || !bytes.Equal(params.Checksum, definition.Checksum[:]) || !bytes.Equal(params.Content, definition.Content) || params.CreatedBy != definition.Actor ||
					params.FileSize != definition.Image.FileSize || params.Width != definition.Image.Width || params.Height != definition.Image.Height || !params.CreatedAt.Time.Equal(definition.Image.CreatedAt) || !params.UpdatedAt.Time.Equal(definition.Image.UpdatedAt) {
					t.Fatalf("wrong generated image params: %#v", params)
				}
			} else {
				id, err = store.InsertHistoricalAttachment(context.Background(), attachment)
				params := query.attachmentParams
				if query.attachmentCalls != 1 || !bytes.Equal(params.Checksum, attachment.Checksum[:]) || !bytes.Equal(params.Content, attachment.Content) || params.Actor != attachment.Attachment.CreatedBy ||
					params.FileSize != int32(attachment.Attachment.FileSize) || string(params.Tags) != "[]" || !params.CreatedAt.Time.Equal(attachment.Attachment.CreatedAt) || !params.UpdatedAt.Time.Equal(attachment.Attachment.UpdatedAt) {
					t.Fatalf("wrong generated attachment params: %#v", params)
				}
			}
			if err != nil || id != 99 {
				t.Fatalf("id=%d err=%v", id, err)
			}
		})
	}
}

func TestHistoricalStaticStoreRejectsMutationBeforeOpeningTransaction(t *testing.T) {
	definition, attachment := historicalStaticStoreFixtures(t)
	store := &HistoricalStaticStore{tx: func(context.Context) (pgx.Tx, error) {
		t.Fatal("invalid definition reached transaction")
		return nil, nil
	}, newQueries: func(pgx.Tx) historicalStaticQueries { return &historicalStaticQueriesFake{} }}
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
				query := &historicalStaticQueriesFake{imageID: 3, attachmentID: 3}
				want := failure
				var txErr error
				switch stage {
				case "tx":
					txErr = failure
				case "query":
					query.imageErr, query.attachmentErr = failure, failure
				case "conflict":
					query.imageErr, query.attachmentErr = &pgconn.PgError{Code: "23505"}, &pgconn.PgError{Code: "23505"}
					want = media.ErrHistoricalStaticConflict
				case "invalid-id":
					query.imageID, query.attachmentID = 0, 0
					want = media.ErrHistoricalStaticInvalid
				}
				store := historicalStaticStoreFake(query)
				store.tx = func(context.Context) (pgx.Tx, error) { return nil, txErr }
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

func historicalStaticStoreFake(query *historicalStaticQueriesFake) *HistoricalStaticStore {
	return &HistoricalStaticStore{
		tx: func(context.Context) (pgx.Tx, error) { return nil, nil },
		newQueries: func(pgx.Tx) historicalStaticQueries {
			return query
		},
	}
}

type historicalStaticQueriesFake struct {
	imageCalls, attachmentCalls int
	imageParams                 mediadb.InsertHistoricalStaticImageParams
	attachmentParams            mediadb.InsertHistoricalStaticAttachmentParams
	imageID, attachmentID       int64
	imageErr, attachmentErr     error
}

func (fake *historicalStaticQueriesFake) InsertHistoricalStaticImage(_ context.Context, params mediadb.InsertHistoricalStaticImageParams) (int64, error) {
	fake.imageCalls++
	fake.imageParams = params
	return fake.imageID, fake.imageErr
}

func (fake *historicalStaticQueriesFake) InsertHistoricalStaticAttachment(_ context.Context, params mediadb.InsertHistoricalStaticAttachmentParams) (int64, error) {
	fake.attachmentCalls++
	fake.attachmentParams = params
	return fake.attachmentID, fake.attachmentErr
}
