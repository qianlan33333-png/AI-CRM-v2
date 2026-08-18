package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"reflect"
	"testing"
)

type variantMemoryStore struct {
	memoryStore
	row   ImageVariantRow
	err   error
	reads int
}

func (store *variantMemoryStore) ReadImageVariant(_ context.Context, id int64) (ImageVariantRow, error) {
	store.reads++
	if store.err != nil {
		return ImageVariantRow{}, store.err
	}
	if id != store.row.ID {
		return ImageVariantRow{}, ErrImageVariantNotFound
	}
	return store.row, nil
}

func TestImageVariantAllKeysResizeAndETagDeterministically(t *testing.T) {
	content := imageVariantFixture(t, "image/png", 400, 200)
	store := newVariantMemoryStore("cover.png", "image/png", 400, 200, content)
	state := &memoryState{receipts: map[string]Receipt{}}
	service := NewService(memoryUOW{state}, store, memoryEvents{state})

	for _, test := range []struct {
		key  string
		max  int
		same bool
	}{
		{key: "thumb_160", max: 160},
		{key: "thumb_320", max: 320},
		{key: "mobile_1080", max: 400, same: true},
		{key: "large_1440", max: 400, same: true},
		{key: "original", max: 400, same: true},
	} {
		t.Run(test.key, func(t *testing.T) {
			first, err := service.GetImageVariant(context.Background(), 7, test.key)
			if err != nil {
				t.Fatal(err)
			}
			second, err := service.GetImageVariant(context.Background(), 7, test.key)
			if err != nil || !reflect.DeepEqual(first, second) {
				t.Fatalf("first=%#v second=%#v err=%v", first, second, err)
			}
			if first.MediaType != "image/png" || !ValidImageVariantETag(first.ETag) {
				t.Fatalf("variant=%#v", first)
			}
			config, format, err := image.DecodeConfig(bytes.NewReader(first.Content))
			if err != nil || format != "png" || max(config.Width, config.Height) != test.max {
				t.Fatalf("config=%#v format=%q err=%v", config, format, err)
			}
			if test.same && !bytes.Equal(first.Content, content) {
				t.Fatal("unexpected upscaling/re-encoding")
			}
			digest := sha256.Sum256(first.Content)
			if first.ETag != `"`+fmtSHA256(digest)+`"` {
				t.Fatalf("etag=%q", first.ETag)
			}
		})
	}
	if store.reads != 10 {
		t.Fatalf("reads=%d", store.reads)
	}
}

func TestImageVariantJPEGNoUpscaleAndGIFRules(t *testing.T) {
	for _, test := range []struct {
		name, filename, mime, key string
		width, height             int
		wantErr                   error
	}{
		{name: "jpeg resize", filename: "cover.jpg", mime: "image/jpeg", key: "thumb_160", width: 320, height: 240},
		{name: "jpeg no upscale", filename: "cover.jpg", mime: "image/jpeg", key: "large_1440", width: 320, height: 240},
		{name: "gif no resize", filename: "cover.gif", mime: "image/gif", key: "thumb_320", width: 200, height: 100},
		{name: "gif oversized unavailable", filename: "cover.gif", mime: "image/gif", key: "thumb_160", width: 200, height: 100, wantErr: ErrImageVariantUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			content := imageVariantFixture(t, test.mime, test.width, test.height)
			store := newVariantMemoryStore(test.filename, test.mime, test.width, test.height, content)
			state := &memoryState{receipts: map[string]Receipt{}}
			variant, err := NewService(memoryUOW{state}, store, memoryEvents{state}).GetImageVariant(context.Background(), 7, test.key)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("variant=%#v err=%v want=%v", variant, err, test.wantErr)
			}
			if test.wantErr != nil {
				return
			}
			if test.mime == "image/jpeg" && test.key == "thumb_160" {
				config, format, configErr := image.DecodeConfig(bytes.NewReader(variant.Content))
				if configErr != nil || format != "jpeg" || max(config.Width, config.Height) != 160 {
					t.Fatalf("config=%#v format=%q err=%v", config, format, configErr)
				}
			} else if !bytes.Equal(variant.Content, content) {
				t.Fatal("no-upscale/gif output changed")
			}
		})
	}
}

func TestImageVariantFailsClosedBeforeReturningBytes(t *testing.T) {
	content := imageVariantFixture(t, "image/png", 2, 2)
	for _, test := range []struct {
		name   string
		mutate func(*ImageVariantRow)
		err    error
	}{
		{name: "checksum mismatch", mutate: func(row *ImageVariantRow) { row.BlobChecksum[0] ^= 1 }, err: ErrImageVariantUnavailable},
		{name: "file size mismatch", mutate: func(row *ImageVariantRow) { row.FileSize++ }, err: ErrImageVariantUnavailable},
		{name: "mime mismatch", mutate: func(row *ImageVariantRow) { row.MimeType = "image/jpeg" }, err: ErrImageVariantUnavailable},
		{name: "dimension mismatch", mutate: func(row *ImageVariantRow) { row.Width++ }, err: ErrImageVariantUnavailable},
		{name: "invalid key", mutate: func(*ImageVariantRow) {}, err: ErrInvalidImageVariant},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newVariantMemoryStore("cover.png", "image/png", 2, 2, content)
			test.mutate(&store.row)
			state := &memoryState{receipts: map[string]Receipt{}}
			key := "original"
			if test.name == "invalid key" {
				key = "bad"
			}
			variant, err := NewService(memoryUOW{state}, store, memoryEvents{state}).GetImageVariant(context.Background(), 7, key)
			if !errors.Is(err, test.err) || len(variant.Content) != 0 {
				t.Fatalf("variant=%#v err=%v want=%v", variant, err, test.err)
			}
		})
	}

	store := newVariantMemoryStore("cover.png", "image/png", 2, 2, content)
	store.err = ErrImageVariantNotFound
	state := &memoryState{receipts: map[string]Receipt{}}
	if _, err := NewService(memoryUOW{state}, store, memoryEvents{state}).GetImageVariant(context.Background(), 7, "original"); !errors.Is(err, ErrImageVariantNotFound) {
		t.Fatalf("missing err=%v", err)
	}
	store.err = errors.New("database unavailable")
	if _, err := NewService(memoryUOW{state}, store, memoryEvents{state}).GetImageVariant(context.Background(), 7, "original"); !errors.Is(err, ErrImageVariantUnavailable) {
		t.Fatalf("database err=%v", err)
	}
}

func TestValidImageVariantETagRequiresLowercaseSHA256(t *testing.T) {
	if !ValidImageVariantETag(`"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`) ||
		ValidImageVariantETag(`"0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef"`) ||
		ValidImageVariantETag(`"short"`) {
		t.Fatal("ETag validation drifted")
	}
}

func newVariantMemoryStore(filename, mime string, width, height int, content []byte) *variantMemoryStore {
	digest := sha256.Sum256(content)
	return &variantMemoryStore{memoryStore: memoryStore{state: &memoryState{receipts: map[string]Receipt{}}}, row: ImageVariantRow{
		ID: 7, FileName: filename, MimeType: mime, FileSize: int32(len(content)), Width: int32(width), Height: int32(height),
		ImageChecksum: append([]byte(nil), digest[:]...), BlobChecksum: append([]byte(nil), digest[:]...), Content: append([]byte(nil), content...),
	}}
}

func imageVariantFixture(t *testing.T, mime string, width, height int) []byte {
	t.Helper()
	if mime == "image/gif" {
		palette := color.Palette{color.RGBA{0, 0, 0, 255}, color.RGBA{255, 64, 32, 255}}
		value := image.NewPaletted(image.Rect(0, 0, width, height), palette)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				value.SetColorIndex(x, y, uint8((x+y)%2))
			}
		}
		var output bytes.Buffer
		if err := gif.Encode(&output, value, nil); err != nil {
			t.Fatal(err)
		}
		return output.Bytes()
	}
	value := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			value.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x + y), A: 255})
		}
	}
	var output bytes.Buffer
	var err error
	switch mime {
	case "image/png":
		err = png.Encode(&output, value)
	case "image/jpeg":
		err = jpeg.Encode(&output, value, &jpeg.Options{Quality: 85})
	default:
		t.Fatalf("unsupported fixture type %q", mime)
	}
	if err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func fmtSHA256(value [sha256.Size]byte) string {
	const hexadecimal = "0123456789abcdef"
	encoded := make([]byte, sha256.Size*2)
	for index, value := range value {
		encoded[index*2], encoded[index*2+1] = hexadecimal[value>>4], hexadecimal[value&15]
	}
	return string(encoded)
}
