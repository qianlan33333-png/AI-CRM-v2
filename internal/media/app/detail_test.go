package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"
)

type detailMemoryUOW struct{ calls int }

func (uow *detailMemoryUOW) Within(ctx context.Context, fn func(context.Context) error) error {
	uow.calls++
	return fn(ctx)
}

type detailMemoryStore struct {
	memoryStore
	row   ImageDetailRow
	err   error
	reads int
}

func (store *detailMemoryStore) ReadImageDetail(_ context.Context, id int64) (ImageDetailRow, error) {
	store.reads++
	if store.err != nil {
		return ImageDetailRow{}, store.err
	}
	if id != store.row.ID {
		return ImageDetailRow{}, ErrImageDetailNotFound
	}
	return store.row, nil
}

func TestGetImageDetailValidatesOneUOWAndProjectsCurrentRow(t *testing.T) {
	content := imageVariantFixture(t, "image/png", 2, 1)
	createdAt := time.Date(2026, 8, 19, 1, 2, 3, 456789123, time.FixedZone("legacy", 8*60*60))
	updatedAt := createdAt.Add(2 * time.Hour)
	store := newDetailMemoryStore(content)
	store.row.Name, store.row.Description, store.row.Tags, store.row.Category = "封面", "说明", " hero,hero, 中文 ", "cover"
	store.row.Width, store.row.Height = 2, 1
	store.row.CreatedAt, store.row.UpdatedAt = createdAt, updatedAt
	uow := &detailMemoryUOW{}
	detail, err := NewService(uow, store, nil).GetImageDetail(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if uow.calls != 1 || store.reads != 1 {
		t.Fatalf("uow=%d reads=%d", uow.calls, store.reads)
	}
	if detail.ID != 7 || detail.Name != "封面" || detail.FileName != "cover.png" || detail.MimeType != "image/png" ||
		detail.FileSize != int32(len(content)) || detail.Description != "说明" || detail.Category != "cover" ||
		detail.Width != 2 || detail.Height != 1 || !detail.CreatedAt.Equal(createdAt.UTC()) || !detail.UpdatedAt.Equal(updatedAt.UTC()) ||
		len(detail.Tags) != 2 || detail.Tags[0] != "hero" || detail.Tags[1] != "中文" || string(detail.Content) != string(content) {
		t.Fatalf("detail=%#v", detail)
	}
	content[0] ^= 1
	if detail.Content[0] == content[0] {
		t.Fatal("detail content aliases store content")
	}
}

func TestGetImageDetailFailsClosedForMetadataAndBlobCorruption(t *testing.T) {
	content := imageVariantFixture(t, "image/png", 2, 2)
	for _, test := range []struct {
		name   string
		mutate func(*ImageDetailRow)
	}{
		{name: "image checksum", mutate: func(row *ImageDetailRow) { row.ImageChecksum[0] ^= 1 }},
		{name: "blob checksum", mutate: func(row *ImageDetailRow) { row.BlobChecksum[0] ^= 1 }},
		{name: "file size", mutate: func(row *ImageDetailRow) { row.FileSize++ }},
		{name: "mime", mutate: func(row *ImageDetailRow) { row.MimeType = "image/jpeg" }},
		{name: "dimensions", mutate: func(row *ImageDetailRow) { row.Width++ }},
		{name: "decode", mutate: func(row *ImageDetailRow) {
			row.Content[0] ^= 1
			digest := sha256.Sum256(row.Content)
			row.ImageChecksum, row.BlobChecksum = digest[:], digest[:]
		}},
		{name: "filename utf8", mutate: func(row *ImageDetailRow) { row.FileName = string([]byte{0xff}) }},
		{name: "mime utf8", mutate: func(row *ImageDetailRow) { row.MimeType = string([]byte{0xff}) }},
		{name: "oversized metadata", mutate: func(row *ImageDetailRow) { row.Description = string(make([]byte, 10_001)) }},
		{name: "missing timestamp", mutate: func(row *ImageDetailRow) { row.CreatedAt = time.Time{} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newDetailMemoryStore(content)
			test.mutate(&store.row)
			detail, err := NewService(&detailMemoryUOW{}, store, nil).GetImageDetail(context.Background(), 7)
			if !errors.Is(err, ErrImageDetailUnavailable) || len(detail.Content) != 0 {
				t.Fatalf("detail=%#v err=%v", detail, err)
			}
		})
	}
}

func TestGetImageDetailUsesUnicodeCodePointMetadataLimits(t *testing.T) {
	for _, test := range []struct {
		name   string
		limit  int
		assign func(*ImageDetailRow, string)
	}{
		{name: "name", limit: 200, assign: func(row *ImageDetailRow, value string) { row.Name = value }},
		{name: "description", limit: 10_000, assign: func(row *ImageDetailRow, value string) { row.Description = value }},
		{name: "tags", limit: 10_000, assign: func(row *ImageDetailRow, value string) { row.Tags = value }},
		{name: "category", limit: 200, assign: func(row *ImageDetailRow, value string) { row.Category = value }},
	} {
		t.Run(test.name, func(t *testing.T) {
			atLimit := newDetailMemoryStore(imageVariantFixture(t, "image/png", 2, 2))
			test.assign(&atLimit.row, strings.Repeat("中", test.limit))
			if _, err := NewService(&detailMemoryUOW{}, atLimit, nil).GetImageDetail(context.Background(), 7); err != nil {
				t.Fatalf("%s at %d code points error = %v", test.name, test.limit, err)
			}

			overLimit := newDetailMemoryStore(imageVariantFixture(t, "image/png", 2, 2))
			test.assign(&overLimit.row, strings.Repeat("中", test.limit+1))
			if _, err := NewService(&detailMemoryUOW{}, overLimit, nil).GetImageDetail(context.Background(), 7); !errors.Is(err, ErrImageDetailUnavailable) {
				t.Fatalf("%s over %d code points error = %v, want ErrImageDetailUnavailable", test.name, test.limit, err)
			}
		})
	}
}

func TestGetImageDetailMapsInputMissingAndStoreFailures(t *testing.T) {
	store := newDetailMemoryStore(imageVariantFixture(t, "image/png", 1, 1))
	service := NewService(&detailMemoryUOW{}, store, nil)
	if _, err := service.GetImageDetail(context.Background(), 0); !errors.Is(err, ErrInvalidImageDetail) || store.reads != 0 {
		t.Fatalf("err=%v reads=%d", err, store.reads)
	}
	store.err = ErrImageDetailNotFound
	if _, err := service.GetImageDetail(context.Background(), 7); !errors.Is(err, ErrImageDetailNotFound) {
		t.Fatalf("missing err=%v", err)
	}
	store.err = errors.New("database contains secret.png")
	if _, err := service.GetImageDetail(context.Background(), 7); !errors.Is(err, ErrImageDetailUnavailable) {
		t.Fatalf("unavailable err=%v", err)
	}
}

func newDetailMemoryStore(content []byte) *detailMemoryStore {
	digest := sha256.Sum256(content)
	return &detailMemoryStore{memoryStore: memoryStore{state: &memoryState{receipts: map[string]Receipt{}}}, row: ImageDetailRow{
		ID: 7, Name: "cover", FileName: "cover.png", MimeType: "image/png", FileSize: int32(len(content)),
		Description: "", Tags: "", Category: "", Width: 2, Height: 2, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		ImageChecksum: append([]byte(nil), digest[:]...), BlobChecksum: append([]byte(nil), digest[:]...), Content: append([]byte(nil), content...),
	}}
}
