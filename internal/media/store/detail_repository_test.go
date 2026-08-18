package store

import (
	"bytes"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
)

func TestImageDetailRowFromGeneratedCopiesEveryReadColumn(t *testing.T) {
	createdAt := time.Date(2026, 8, 19, 1, 2, 3, 0, time.UTC)
	generated := mediadb.GetMediaImageDetailRow{
		ID: 7, Name: "cover", FileName: "cover.png", MimeType: "image/png", FileSize: 3, Description: "desc", Tags: "a,b", Category: "cat",
		Width: 1, Height: 2, CreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: createdAt.Add(time.Second), Valid: true},
		ImageChecksum: []byte{1, 2, 3}, BlobChecksum: []byte{1, 2, 3}, Content: []byte{4, 5, 6},
	}
	row := imageDetailRowFromGenerated(generated)
	if row.ID != generated.ID || row.Name != generated.Name || row.FileName != generated.FileName || row.MimeType != generated.MimeType ||
		row.FileSize != generated.FileSize || row.Description != generated.Description || row.Tags != generated.Tags || row.Category != generated.Category ||
		row.Width != generated.Width || row.Height != generated.Height || !row.CreatedAt.Equal(createdAt) || !row.UpdatedAt.Equal(createdAt.Add(time.Second)) ||
		!bytes.Equal(row.ImageChecksum, generated.ImageChecksum) || !bytes.Equal(row.BlobChecksum, generated.BlobChecksum) || !bytes.Equal(row.Content, generated.Content) {
		t.Fatalf("row=%#v", row)
	}
	generated.ImageChecksum[0], generated.BlobChecksum[0], generated.Content[0] = 9, 9, 9
	if row.ImageChecksum[0] != 1 || row.BlobChecksum[0] != 1 || row.Content[0] != 4 {
		t.Fatalf("row aliases SQL values: %#v", row)
	}
}
