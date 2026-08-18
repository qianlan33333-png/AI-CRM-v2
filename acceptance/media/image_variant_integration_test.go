package media_acceptance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

// This exercises the PostgreSQL projection after the real upload chain has
// persisted each supported format. HTTP routing and authorization live in
// cmd/aicrm tests; this acceptance test proves the Media read itself makes no
// durable write, event, receipt, or provider effect.
func TestImageVariant0366PostgreSQLReadIsSideEffectFree(t *testing.T) {
	pool, ctx := openPool(t)
	service := realService(pool)
	created := make(map[string]mediaport.Image)
	for index, mime := range []string{"image/png", "image/jpeg", "image/gif"} {
		command := imageVariantUploadCommand(t, 7201+int64(index), unique("variant-"+mime), mime)
		value, err := service.Upload(ctx, command)
		if err != nil {
			t.Fatalf("upload %s: %v", mime, err)
		}
		created[mime] = value
	}

	before := imageFacetsFactSnapshot(t, pool, ctx)
	for _, test := range []struct {
		mime, key string
		maximum   int
		exact     bool
	}{
		{mime: "image/png", key: "thumb_160", maximum: 160},
		{mime: "image/jpeg", key: "thumb_320", maximum: 320},
		{mime: "image/gif", key: "original", maximum: 400, exact: true},
	} {
		t.Run(test.mime+"/"+test.key, func(t *testing.T) {
			imageValue := created[test.mime]
			variant, err := service.GetImageVariant(context.Background(), imageValue.ID, test.key)
			if err != nil {
				t.Fatal(err)
			}
			if variant.MediaType != test.mime || !mediaapp.ValidImageVariantETag(variant.ETag) {
				t.Fatalf("variant=%#v", variant)
			}
			configuration, _, err := image.DecodeConfig(bytes.NewReader(variant.Content))
			if err != nil || max(configuration.Width, configuration.Height) != test.maximum {
				t.Fatalf("config=%#v err=%v", configuration, err)
			}
			if test.exact && !bytes.Equal(variant.Content, imageVariantUploadContent(t, test.mime)) {
				t.Fatal("original GIF bytes changed")
			}
			digest := sha256.Sum256(variant.Content)
			if variant.ETag != `"`+hex.EncodeToString(digest[:])+`"` {
				t.Fatalf("etag=%q", variant.ETag)
			}
		})
	}
	after := imageFacetsFactSnapshot(t, pool, ctx)
	if before != after {
		t.Fatalf("read changed durable facts before=%#v after=%#v", before, after)
	}
}

func imageVariantUploadCommand(t *testing.T, actor int64, key, mime string) mediaport.UploadCommand {
	t.Helper()
	content := imageVariantUploadContent(t, mime)
	filename := "fixture.png"
	if mime == "image/jpeg" {
		filename = "fixture.jpg"
	} else if mime == "image/gif" {
		filename = "fixture.gif"
	}
	return mediaport.UploadCommand{
		Actor: actor, IdempotencyKey: key, FileName: filename, DeclaredType: mime, Content: content,
		Name: "image-variant-fixture", Description: "", Tags: "", Category: "",
	}
}

func imageVariantUploadContent(t *testing.T, mime string) []byte {
	t.Helper()
	if mime == "image/gif" {
		palette := color.Palette{color.RGBA{0, 0, 0, 255}, color.RGBA{255, 0, 0, 255}}
		value := image.NewPaletted(image.Rect(0, 0, 400, 200), palette)
		for y := 0; y < 200; y++ {
			for x := 0; x < 400; x++ {
				value.SetColorIndex(x, y, uint8((x+y)%2))
			}
		}
		var output bytes.Buffer
		if err := gif.Encode(&output, value, nil); err != nil {
			t.Fatal(err)
		}
		return output.Bytes()
	}
	value := image.NewRGBA(image.Rect(0, 0, 400, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 400; x++ {
			value.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x + y), A: 255})
		}
	}
	var output bytes.Buffer
	var err error
	if mime == "image/png" {
		err = png.Encode(&output, value)
	} else {
		err = jpeg.Encode(&output, value, &jpeg.Options{Quality: 85})
	}
	if err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
