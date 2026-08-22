package store

import (
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

type valueRow struct {
	values []any
	err    error
}

func (row valueRow) Scan(destinations ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(destinations) != len(row.values) {
		return errors.New("destination count mismatch")
	}
	for index, destination := range destinations {
		target := reflect.ValueOf(destination)
		if target.Kind() != reflect.Pointer || target.IsNil() {
			return errors.New("invalid destination")
		}
		value := row.values[index]
		if value == nil {
			target.Elem().Set(reflect.Zero(target.Elem().Type()))
			continue
		}
		source := reflect.ValueOf(value)
		if source.Type().AssignableTo(target.Elem().Type()) {
			target.Elem().Set(source)
			continue
		}
		if source.Type().ConvertibleTo(target.Elem().Type()) {
			target.Elem().Set(source.Convert(target.Elem().Type()))
			continue
		}
		return errors.New("incompatible destination")
	}
	return nil
}

func TestListQueryUsesOnlyClosedStableOrders(t *testing.T) {
	tests := []struct {
		sort radarport.Sort
		want string
	}{
		{sort: radarport.SortUpdatedDesc, want: "ORDER BY updated_at DESC, id DESC"},
		{sort: radarport.SortCreatedDesc, want: "ORDER BY created_at DESC, id DESC"},
		{sort: radarport.SortNameAsc, want: "ORDER BY name ASC, id ASC"},
	}
	for _, test := range tests {
		query := listQuery(test.sort)
		if !strings.Contains(query, test.want) || !strings.Contains(query, "LIMIT $2 OFFSET $3") || strings.Contains(query, string(test.sort)) {
			t.Errorf("sort=%q query=%q", test.sort, query)
		}
	}
	if query := listQuery("untrusted SQL material"); !strings.Contains(query, "ORDER BY updated_at DESC, id DESC") || strings.Contains(query, "untrusted") {
		t.Fatalf("untrusted sort reached SQL: %q", query)
	}
}

func TestScanLinkClosesNullableReferences(t *testing.T) {
	created := time.Date(2026, 8, 22, 1, 2, 3, 0, time.UTC)
	updated := created.Add(time.Minute)
	link, err := scanLink(valueRow{values: []any{
		int64(12),
		"rd_AAAAAAAAAAAAAAAAAAAAAA",
		"Guide",
		"Read guide",
		"https://example.com/guide",
		sql.NullInt64{Int64: 9, Valid: true},
		sql.NullInt64{},
		"enabled",
		int64(3),
		int64(41),
		int64(42),
		created,
		updated,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if link.LinkID != 12 || link.CoverImageID == nil || *link.CoverImageID != 9 || link.AttachmentID != nil || link.Status != radarport.StatusEnabled || link.Version != 3 || !link.UpdatedAt.Equal(updated) {
		t.Fatalf("link=%+v", link)
	}
}

func TestScanIdempotencyUsesClosedTypedResultColumns(t *testing.T) {
	created := time.Date(2026, 8, 22, 1, 2, 3, 0, time.UTC)
	resultUpdated := created.Add(500 * time.Millisecond)
	completed := created.Add(time.Second)
	keyDigest := make([]byte, 32)
	payloadDigest := make([]byte, 32)
	for index := range keyDigest {
		keyDigest[index] = byte(index)
		payloadDigest[index] = byte(31 - index)
	}
	record, err := scanIdempotency(valueRow{values: []any{
		int64(7),
		int64(41),
		keyDigest,
		"update",
		payloadDigest,
		"completed",
		sql.NullInt64{Int64: 12, Valid: true},
		sql.NullString{String: "rd_AAAAAAAAAAAAAAAAAAAAAA", Valid: true},
		sql.NullString{String: "Guide", Valid: true},
		sql.NullString{String: "Read guide", Valid: true},
		sql.NullString{String: "https://example.com/guide", Valid: true},
		sql.NullInt64{Int64: 9, Valid: true},
		sql.NullInt64{},
		sql.NullString{String: "enabled", Valid: true},
		sql.NullInt64{Int64: 3, Valid: true},
		sql.NullInt64{Int64: 41, Valid: true},
		sql.NullInt64{Int64: 42, Valid: true},
		sql.NullTime{Time: created, Valid: true},
		sql.NullTime{Time: resultUpdated, Valid: true},
		created,
		sql.NullTime{Time: completed, Valid: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if record.RecordID != 7 || record.State != radarport.IdempotencyCompleted || record.CompletedAt == nil || !record.CompletedAt.Equal(completed) || record.Result == nil {
		t.Fatalf("record=%+v", record)
	}
	if record.Result.LinkID != 12 || record.Result.CoverImageID == nil || *record.Result.CoverImageID != 9 || record.Result.AttachmentID != nil || record.Result.Status != radarport.StatusEnabled || record.Result.Version != 3 {
		t.Fatalf("result=%+v", record.Result)
	}

	reserved, err := scanIdempotency(valueRow{values: []any{
		int64(8), int64(41), keyDigest, "create", payloadDigest, "reserved",
		sql.NullInt64{}, sql.NullString{}, sql.NullString{}, sql.NullString{}, sql.NullString{},
		sql.NullInt64{}, sql.NullInt64{}, sql.NullString{}, sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{},
		sql.NullTime{}, sql.NullTime{}, created, sql.NullTime{},
	}})
	if err != nil || reserved.State != radarport.IdempotencyReserved || reserved.Result != nil || reserved.CompletedAt != nil {
		t.Fatalf("reserved=%+v err=%v", reserved, err)
	}

	if _, err = scanIdempotency(valueRow{values: []any{
		int64(7), int64(41), []byte{1}, "update", payloadDigest, "reserved",
		sql.NullInt64{}, sql.NullString{}, sql.NullString{}, sql.NullString{}, sql.NullString{},
		sql.NullInt64{}, sql.NullInt64{}, sql.NullString{}, sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{},
		sql.NullTime{}, sql.NullTime{}, created, sql.NullTime{},
	}}); !errors.Is(err, radarport.ErrUnavailable) {
		t.Fatalf("short digest err=%v", err)
	}
}

func TestConstructorRejectsNilResolver(t *testing.T) {
	if _, err := NewPostgresRepositoryWithTxResolver(nil); !errors.Is(err, radarport.ErrUnavailable) {
		t.Fatalf("err=%v", err)
	}
}
