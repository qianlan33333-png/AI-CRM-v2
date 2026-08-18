package store

import (
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
)

func TestImageMetadataGeneratedRowsPreserveAllMutableAndImmutableFacts(t *testing.T) {
	created := time.Date(2026, 8, 19, 1, 2, 3, 0, time.UTC)
	updated := created.Add(time.Second)
	row := mediadb.LockMediaImageMetadataRow{
		ID: 44, Name: "before", FileName: "cover.png", MimeType: "image/png", FileSize: 123, Enabled: true,
		Description: "description", Tags: "hero,首页", Category: "cover", Width: 640, Height: 480,
		CreatedAt: pgtype.Timestamptz{Time: created, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: updated, Valid: true},
	}
	want := mediaapp.ImageMetadata{ID: 44, Name: "before", FileName: "cover.png", MimeType: "image/png", FileSize: 123, Enabled: true,
		Description: "description", Tags: "hero,首页", Category: "cover", Width: 640, Height: 480, CreatedAt: created, UpdatedAt: updated}
	if got := imageMetadataFromLockRow(row); !reflect.DeepEqual(got, want) {
		t.Fatalf("lock row=%#v want=%#v", got, want)
	}
	updatedRow := mediadb.UpdateMediaImageMetadataRow{
		ID: 44, Name: "after", FileName: "cover.png", MimeType: "image/png", FileSize: 123, Enabled: false,
		Description: "updated", Tags: "hero,首页,新品", Category: "banner", Width: 640, Height: 480,
		CreatedAt: pgtype.Timestamptz{Time: created, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: updated.Add(time.Second), Valid: true},
	}
	got := imageMetadataFromUpdateRow(updatedRow)
	if got.ID != 44 || got.Name != "after" || got.FileName != "cover.png" || got.MimeType != "image/png" || got.FileSize != 123 || got.Enabled ||
		got.Description != "updated" || got.Tags != "hero,首页,新品" || got.Category != "banner" || got.Width != 640 || got.Height != 480 || !got.CreatedAt.Equal(created) || !got.UpdatedAt.Equal(updated.Add(time.Second)) {
		t.Fatalf("update row=%#v", got)
	}
}
