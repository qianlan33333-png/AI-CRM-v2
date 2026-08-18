package store

import (
	"bytes"
	"testing"

	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
)

func TestImageVariantRowFromGeneratedCopiesBlobAndChecksums(t *testing.T) {
	generated := mediadb.GetMediaImageVariantRow{
		ID: 7, FileName: "cover.png", MimeType: "image/png", FileSize: 3, Width: 1, Height: 1,
		ImageChecksum: []byte{1, 2, 3}, BlobChecksum: []byte{1, 2, 3}, Content: []byte{4, 5, 6},
	}
	row := imageVariantRowFromGenerated(generated)
	if row.ID != 7 || row.FileName != "cover.png" || row.MimeType != "image/png" || row.FileSize != 3 || row.Width != 1 || row.Height != 1 ||
		!bytes.Equal(row.ImageChecksum, generated.ImageChecksum) || !bytes.Equal(row.BlobChecksum, generated.BlobChecksum) || !bytes.Equal(row.Content, generated.Content) {
		t.Fatalf("row=%#v", row)
	}
	generated.ImageChecksum[0], generated.BlobChecksum[0], generated.Content[0] = 9, 9, 9
	if row.ImageChecksum[0] != 1 || row.BlobChecksum[0] != 1 || row.Content[0] != 4 {
		t.Fatalf("returned row aliases sqlc values: %#v", row)
	}
}
