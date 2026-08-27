package media

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

	"github.com/qianlan33333-png/AI-CRM-v2/internal/media/domain"
)

func historicalImageFixture(t *testing.T) (V1ImageLibraryRow, HistoricalStaticOrigin) {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewNRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.FixedZone("source", 8*3600))
	return V1ImageLibraryRow{ID: 11, Name: " 原图 ", FileName: "original.png", MimeType: "image/png", DataBase64: base64.StdEncoding.EncodeToString(buffer.Bytes()),
			FileSize: int64(buffer.Len()), Description: " 说明 ", Category: " 分类 ", Tags: []string{"标签"}, Enabled: true,
			SourceURL: "https://not-fetched.invalid/original", ThumbMediaID: "discard-provider-id", ThumbMediaExpiresAt: &now, CreatedAt: now, UpdatedAt: now.Add(time.Hour)},
		HistoricalStaticOrigin{SourceIdentifier: "v1:image_library:11", SourceID: 11, PayloadDigest: sha256.Sum256([]byte("complete archived image row"))}
}

func historicalAttachmentFixture() (V1AttachmentLibraryRow, HistoricalStaticOrigin) {
	content := []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF\n")
	now := time.Date(2026, 8, 27, 2, 0, 0, 0, time.UTC)
	return V1AttachmentLibraryRow{ID: 12, Name: " PDF ", FileName: "document.pdf", MimeType: "application/pdf", DataBase64: base64.StdEncoding.EncodeToString(content),
			FileSize: int64(len(content)), Description: " 说明 ", Tags: []string{"手册"}, Enabled: true, MediaID: "discard-provider-id", MediaExpiresAt: &now, CreatedAt: now, UpdatedAt: now},
		HistoricalStaticOrigin{SourceIdentifier: "v1:attachment_library:12", SourceID: 12, PayloadDigest: sha256.Sum256([]byte("complete archived attachment row"))}
}

func TestHistoricalStaticAdaptersForceDisabledAndRetainVerifiedBytes(t *testing.T) {
	source, origin := historicalImageFixture(t)
	definition, err := AdaptV1ImageLibrary(source, origin, 7)
	if err != nil {
		t.Fatal(err)
	}
	if definition.Image.Enabled || definition.Image.ID != 0 || definition.Image.Name != "原图" || definition.Image.Tags != "标签" || definition.Image.Width != 2 || definition.Image.Height != 3 ||
		definition.Actor != 7 || definition.Origin != origin || !definition.ProviderMaterialDropped || definition.Checksum != sha256.Sum256(definition.Content) ||
		definition.Image.CreatedAt.Location() != time.UTC || !definition.Image.CreatedAt.Equal(source.CreatedAt) || base64.StdEncoding.EncodeToString(definition.Content) != source.DataBase64 {
		t.Fatalf("wrong image projection: %+v", definition)
	}
	attachment, attachmentOrigin := historicalAttachmentFixture()
	attachment.MimeType = "application/pdf; charset=binary"
	pdf, err := AdaptV1AttachmentLibrary(attachment, attachmentOrigin, 7)
	if err != nil {
		t.Fatal(err)
	}
	if pdf.Attachment.Enabled || pdf.Attachment.ID != 0 || pdf.Attachment.Version != 1 || pdf.Attachment.Name != "PDF" || pdf.Attachment.MimeType != "application/pdf" ||
		pdf.Attachment.CreatedBy != 7 || pdf.Attachment.UpdatedBy != 7 || !pdf.ProviderMaterialDropped || pdf.Checksum != sha256.Sum256(pdf.Content) || pdf.Origin != attachmentOrigin {
		t.Fatalf("wrong attachment projection: %+v", pdf)
	}
	attachment.Tags[0] = "changed"
	if pdf.Attachment.Tags[0] != "手册" {
		t.Fatal("adapter retained mutable source tags")
	}
}

func TestHistoricalImageRejectsUnverifiableSources(t *testing.T) {
	cases := map[string]func(*V1ImageLibraryRow){
		"url-only":       func(s *V1ImageLibraryRow) { s.DataBase64 = "" },
		"data-url":       func(s *V1ImageLibraryRow) { s.DataBase64 = "data:image/png;base64," + s.DataBase64 },
		"base64-newline": func(s *V1ImageLibraryRow) { s.DataBase64 += "\n" },
		"size-mismatch":  func(s *V1ImageLibraryRow) { s.FileSize++ },
		"oversize":       func(s *V1ImageLibraryRow) { s.FileSize = domain.MaxImageBytes + 1 },
		"truncated": func(s *V1ImageLibraryRow) {
			b, _ := base64.StdEncoding.DecodeString(s.DataBase64)
			b = b[:len(b)/2]
			s.DataBase64 = base64.StdEncoding.EncodeToString(b)
			s.FileSize = int64(len(b))
		},
		"mime-mismatch":  func(s *V1ImageLibraryRow) { s.MimeType = "image/jpeg" },
		"unsupported":    func(s *V1ImageLibraryRow) { s.MimeType = "image/svg+xml" },
		"filename":       func(s *V1ImageLibraryRow) { s.FileName = "../image.png" },
		"source-id":      func(s *V1ImageLibraryRow) { s.ID++ },
		"timestamps":     func(s *V1ImageLibraryRow) { s.UpdatedAt = s.CreatedAt.Add(-time.Second) },
		"name":           func(s *V1ImageLibraryRow) { s.Name = strings.Repeat("图", 201) },
		"duplicate-tags": func(s *V1ImageLibraryRow) { s.Tags = []string{"a", "a"} },
		"ambiguous-tags": func(s *V1ImageLibraryRow) { s.Tags = []string{"a,b"} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			source, origin := historicalImageFixture(t)
			mutate(&source)
			if _, err := AdaptV1ImageLibrary(source, origin, 7); !errors.Is(err, ErrHistoricalStaticInvalid) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestHistoricalAttachmentRejectsUnsupportedOrTruncatedSources(t *testing.T) {
	cases := map[string]func(*V1AttachmentLibraryRow){
		"no-bytes":   func(s *V1AttachmentLibraryRow) { s.DataBase64 = "" },
		"wrong-size": func(s *V1AttachmentLibraryRow) { s.FileSize-- },
		"office":     func(s *V1AttachmentLibraryRow) { s.MimeType = "application/msword" },
		"not-pdf": func(s *V1AttachmentLibraryRow) {
			b := []byte("not a PDF %%EOF")
			s.DataBase64 = base64.StdEncoding.EncodeToString(b)
			s.FileSize = int64(len(b))
		},
		"missing-eof": func(s *V1AttachmentLibraryRow) {
			b := []byte("%PDF-1.7\ntruncated")
			s.DataBase64 = base64.StdEncoding.EncodeToString(b)
			s.FileSize = int64(len(b))
		},
		"bad-name":        func(s *V1AttachmentLibraryRow) { s.Name = " " },
		"unsafe-filename": func(s *V1AttachmentLibraryRow) { s.FileName = "bad\n.pdf" },
		"invalid-tags":    func(s *V1AttachmentLibraryRow) { s.Tags = []string{""} },
		"source-id":       func(s *V1AttachmentLibraryRow) { s.ID++ },
		"timestamps":      func(s *V1AttachmentLibraryRow) { s.CreatedAt = time.Time{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			source, origin := historicalAttachmentFixture()
			mutate(&source)
			if _, err := AdaptV1AttachmentLibrary(source, origin, 7); !errors.Is(err, ErrHistoricalStaticInvalid) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

type historicalStaticFake struct {
	receipts                      map[string]HistoricalStaticReceipt
	images, attachments, records  int
	loadErr, insertErr, recordErr error
	zeroID                        bool
}

func (fake *historicalStaticFake) InsertHistoricalImage(context.Context, HistoricalImageDefinition) (int64, error) {
	fake.images++
	if fake.zeroID {
		return 0, fake.insertErr
	}
	return 101, fake.insertErr
}

func (fake *historicalStaticFake) InsertHistoricalAttachment(context.Context, HistoricalAttachmentDefinition) (int64, error) {
	fake.attachments++
	if fake.zeroID {
		return 0, fake.insertErr
	}
	return 102, fake.insertErr
}

func (fake *historicalStaticFake) LoadHistoricalStatic(_ context.Context, key string) (HistoricalStaticReceipt, bool, error) {
	receipt, found := fake.receipts[key]
	return receipt, found, fake.loadErr
}

func (fake *historicalStaticFake) RecordHistoricalStatic(_ context.Context, receipt HistoricalStaticReceipt) error {
	fake.records++
	if fake.recordErr != nil {
		return fake.recordErr
	}
	if fake.receipts == nil {
		fake.receipts = map[string]HistoricalStaticReceipt{}
	}
	fake.receipts[receipt.Origin.SourceIdentifier] = receipt
	return nil
}

func TestHistoricalStaticWriterReplaysAndRejectsDrift(t *testing.T) {
	source, origin := historicalImageFixture(t)
	definition, err := AdaptV1ImageLibrary(source, origin, 7)
	if err != nil {
		t.Fatal(err)
	}
	fake := &historicalStaticFake{}
	writer, err := NewHistoricalStaticWriter(fake, fake)
	if err != nil {
		t.Fatal(err)
	}
	first, err := writer.ImportImage(context.Background(), definition)
	if err != nil || first.Replayed || first.TargetID != 101 || first.Kind != HistoricalImage || first.DefinitionDigest == [32]byte{} {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	replay, err := writer.ImportImage(context.Background(), definition)
	if err != nil || !replay.Replayed || fake.images != 1 || fake.records != 1 {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	for name, mutate := range map[string]func(*HistoricalImageDefinition){
		"payload":           func(d *HistoricalImageDefinition) { d.Origin.PayloadDigest[0]++ },
		"source-id":         func(d *HistoricalImageDefinition) { d.Origin.SourceID++ },
		"metadata":          func(d *HistoricalImageDefinition) { d.Image.Name = "changed" },
		"actor":             func(d *HistoricalImageDefinition) { d.Actor++ },
		"provider-dropping": func(d *HistoricalImageDefinition) { d.ProviderMaterialDropped = false },
	} {
		t.Run(name, func(t *testing.T) {
			changed := definition
			mutate(&changed)
			if _, err := writer.ImportImage(context.Background(), changed); !errors.Is(err, ErrHistoricalStaticConflict) {
				t.Fatalf("got %v", err)
			}
		})
	}
	attachment, attachmentOrigin := historicalAttachmentFixture()
	pdf, err := AdaptV1AttachmentLibrary(attachment, attachmentOrigin, 7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = writer.ImportAttachment(context.Background(), pdf); err != nil {
		t.Fatal(err)
	}
	if r, err := writer.ImportAttachment(context.Background(), pdf); err != nil || !r.Replayed || fake.attachments != 1 || fake.records != 2 {
		t.Fatalf("receipt=%+v err=%v", r, err)
	}
	pdf.Origin = origin // Reusing an image source key must not become an attachment.
	if _, err := writer.ImportAttachment(context.Background(), pdf); !errors.Is(err, ErrHistoricalStaticConflict) {
		t.Fatalf("kind collision: %v", err)
	}
	if fake.images != 1 || fake.attachments != 1 {
		t.Fatal("replay/drift created another target")
	}
}

func TestHistoricalStaticWriterRejectsUnsafeDefinitionsBeforePersistence(t *testing.T) {
	source, origin := historicalImageFixture(t)
	for name, mutate := range map[string]func(*HistoricalImageDefinition){
		"enabled":  func(d *HistoricalImageDefinition) { d.Image.Enabled = true },
		"id":       func(d *HistoricalImageDefinition) { d.Image.ID = 1 },
		"checksum": func(d *HistoricalImageDefinition) { d.Checksum[0]++ },
		"content":  func(d *HistoricalImageDefinition) { d.Content[0]++ },
		"size":     func(d *HistoricalImageDefinition) { d.Image.FileSize++ },
		"actor":    func(d *HistoricalImageDefinition) { d.Actor = 0 },
		"origin":   func(d *HistoricalImageDefinition) { d.Origin.PayloadDigest = [32]byte{} },
	} {
		t.Run(name, func(t *testing.T) {
			definition, err := AdaptV1ImageLibrary(source, origin, 7)
			if err != nil {
				t.Fatal(err)
			}
			mutate(&definition)
			fake := &historicalStaticFake{}
			writer, _ := NewHistoricalStaticWriter(fake, fake)
			if _, err := writer.ImportImage(context.Background(), definition); !errors.Is(err, ErrHistoricalStaticInvalid) {
				t.Fatalf("got %v", err)
			}
			if fake.images != 0 || fake.records != 0 {
				t.Fatal("unsafe definition reached persistence")
			}
		})
	}
}

func TestHistoricalStaticWriterPropagatesTransactionFailures(t *testing.T) {
	source, origin := historicalImageFixture(t)
	definition, err := AdaptV1ImageLibrary(source, origin, 7)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("transaction failure")
	for _, stage := range []string{"load", "insert", "record", "zero-id"} {
		t.Run(stage, func(t *testing.T) {
			fake := &historicalStaticFake{}
			want := failure
			switch stage {
			case "load":
				fake.loadErr = failure
			case "insert":
				fake.insertErr = failure
			case "record":
				fake.recordErr = failure
			case "zero-id":
				fake.zeroID = true
				want = ErrHistoricalStaticInvalid
			}
			writer, _ := NewHistoricalStaticWriter(fake, fake)
			if receipt, err := writer.ImportImage(context.Background(), definition); !errors.Is(err, want) || receipt.TargetID != 0 {
				t.Fatalf("receipt=%+v err=%v", receipt, err)
			}
			if len(fake.receipts) != 0 || stage != "record" && fake.records != 0 {
				t.Fatal("failure recorded a completed receipt")
			}
		})
	}
	var fake *historicalStaticFake
	if _, err := NewHistoricalStaticWriter(fake, fake); !errors.Is(err, ErrHistoricalStaticInvalid) {
		t.Fatal(err)
	}
	var writer *HistoricalStaticWriter
	if _, err := writer.ImportImage(context.Background(), definition); !errors.Is(err, ErrHistoricalStaticInvalid) {
		t.Fatal(err)
	}
	valid := &historicalStaticFake{}
	writer, _ = NewHistoricalStaticWriter(valid, valid)
	if _, err := writer.ImportImage(nil, definition); !errors.Is(err, ErrHistoricalStaticInvalid) {
		t.Fatal(err)
	}
}
