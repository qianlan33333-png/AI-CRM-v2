package store

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
)

func TestImageListReadFromGeneratedRowsPreservesFailClosedValidation(t *testing.T) {
	now := time.Date(2026, 8, 18, 1, 2, 3, 0, time.UTC)
	valid := mediadb.ListMediaImagePageRow{
		Total:       2,
		ID:          pgtype.Int8{Int64: 42, Valid: true},
		Name:        pgtype.Text{String: "cover", Valid: true},
		FileName:    pgtype.Text{String: "cover.png", Valid: true},
		MimeType:    pgtype.Text{String: "image/png", Valid: true},
		FileSize:    pgtype.Int4{Int32: 123, Valid: true},
		Enabled:     pgtype.Bool{Bool: true, Valid: true},
		Description: pgtype.Text{String: "description", Valid: true},
		Tags:        pgtype.Text{String: "hero,cover", Valid: true},
		Category:    pgtype.Text{String: "cover", Valid: true},
		Width:       pgtype.Int4{Int32: 640, Valid: true},
		Height:      pgtype.Int4{Int32: 480, Valid: true},
		CreatedAt:   pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:   pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
	}
	second := valid
	second.ID.Int64 = 41
	second.FileName.String = "second.png"

	read, err := imageListReadFromGeneratedRows([]mediadb.ListMediaImagePageRow{valid, second})
	if err != nil {
		t.Fatal(err)
	}
	want := mediaapp.ImageListRead{Total: 2, Rows: []mediaapp.ImageListRow{
		{
			ID: 42, Name: "cover", FileName: "cover.png", MimeType: "image/png", FileSize: 123, Enabled: true,
			Description: "description", Tags: "hero,cover", Category: "cover", Width: 640, Height: 480,
			CreatedAt: now, UpdatedAt: now.Add(time.Minute),
		},
		{
			ID: 41, Name: "cover", FileName: "second.png", MimeType: "image/png", FileSize: 123, Enabled: true,
			Description: "description", Tags: "hero,cover", Category: "cover", Width: 640, Height: 480,
			CreatedAt: now, UpdatedAt: now.Add(time.Minute),
		},
	}}
	if !reflect.DeepEqual(read, want) {
		t.Fatalf("read=%#v want=%#v", read, want)
	}

	totalOnly, err := imageListReadFromGeneratedRows([]mediadb.ListMediaImagePageRow{{Total: 7}})
	if err != nil || totalOnly.Total != 7 || totalOnly.Rows == nil || len(totalOnly.Rows) != 0 {
		t.Fatalf("total-only read=%#v err=%v", totalOnly, err)
	}

	partialEmpty := mediadb.ListMediaImagePageRow{Total: 1, Name: pgtype.Text{String: "unexpected", Valid: true}}
	missingField := valid
	missingField.MimeType.Valid = false
	inconsistent := second
	inconsistent.Total = 3
	for _, test := range []struct {
		name string
		rows []mediadb.ListMediaImagePageRow
	}{
		{name: "no rows", rows: nil},
		{name: "negative total", rows: []mediadb.ListMediaImagePageRow{{Total: -1}}},
		{name: "partial empty page row", rows: []mediadb.ListMediaImagePageRow{partialEmpty}},
		{name: "required item field missing", rows: []mediadb.ListMediaImagePageRow{missingField}},
		{name: "inconsistent total", rows: []mediadb.ListMediaImagePageRow{valid, inconsistent}},
	} {
		t.Run(test.name, func(t *testing.T) {
			read, err := imageListReadFromGeneratedRows(test.rows)
			if !errors.Is(err, errInvalidImageListRepository) || read.Rows != nil || read.Total != 0 {
				t.Fatalf("read=%#v err=%v", read, err)
			}
		})
	}
}
