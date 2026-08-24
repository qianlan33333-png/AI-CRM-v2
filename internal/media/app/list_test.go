package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

type imageListSnapshotKey struct{}

type imageListTestUOW struct {
	calls  int
	err    error
	marker string
}

type imageListTestStore struct {
	read       ImageListRead
	err        error
	calls      int
	writeCalls int
	exists     bool
	existsErr  error
	existsID   int64
	filter     ImageListFilter
	limit      int64
	offset     int64
	marker     string
}

func (uow *imageListTestUOW) Within(ctx context.Context, operation func(context.Context) error) error {
	uow.calls++
	if uow.err != nil {
		return uow.err
	}
	return operation(context.WithValue(ctx, imageListSnapshotKey{}, uow.marker))
}

func (store *imageListTestStore) ListImageRows(ctx context.Context, filter ImageListFilter, limit, offset int64) (ImageListRead, error) {
	store.calls++
	store.filter, store.limit, store.offset = filter, limit, offset
	store.marker, _ = ctx.Value(imageListSnapshotKey{}).(string)
	if store.err != nil {
		return ImageListRead{}, store.err
	}
	result := store.read
	result.Rows = append([]ImageListRow(nil), store.read.Rows...)
	return result, nil
}

func (store *imageListTestStore) Reserve(context.Context, Reservation) (Receipt, bool, error) {
	store.writeCalls++
	return Receipt{}, false, errors.New("unexpected reserve")
}

func (store *imageListTestStore) Create(context.Context, CreateInput) (mediaport.Image, error) {
	store.writeCalls++
	return mediaport.Image{}, errors.New("unexpected create")
}

func (store *imageListTestStore) Complete(context.Context, int64, json.RawMessage, time.Time) (Receipt, error) {
	store.writeCalls++
	return Receipt{}, errors.New("unexpected complete")
}

func (store *imageListTestStore) ImageExists(ctx context.Context, imageID int64) (bool, error) {
	store.existsID = imageID
	store.marker, _ = ctx.Value(imageListSnapshotKey{}).(string)
	return store.exists, store.existsErr
}

func TestLocalImageExistsUsesOnlyMetadataReader(t *testing.T) {
	store := &imageListTestStore{exists: true}
	uow := &imageListTestUOW{marker: "local-existence-uow"}
	exists, err := NewService(uow, store, nil).LocalImageExists(context.Background(), 42)
	if err != nil || !exists || uow.calls != 1 || store.existsID != 42 || store.marker != "local-existence-uow" || store.writeCalls != 0 {
		t.Fatalf("exists/uow/id/marker/writes/err=%t/%d/%d/%q/%d/%v", exists, uow.calls, store.existsID, store.marker, store.writeCalls, err)
	}
	store.existsErr = errors.New("database unavailable")
	if _, err = NewService(uow, store, nil).LocalImageExists(context.Background(), 42); !errors.Is(err, ErrListUnavailable) {
		t.Fatalf("dependency error=%v", err)
	}
}

func TestImageListRepositoryContractUsesOneCombinedSameSnapshotRead(t *testing.T) {
	created := time.Date(2026, 8, 17, 3, 4, 5, 123456789, time.FixedZone("CST", 8*60*60))
	updated := created.Add(2 * time.Hour)
	longTag := strings.Repeat("界", 64) + "尾"
	store := &imageListTestStore{read: ImageListRead{Total: 1, Rows: []ImageListRow{{
		ID: 42, Name: " Hero ", FileName: "hero.png", MimeType: "image/png", FileSize: 123,
		Enabled:     true,
		Description: "说明", Tags: " alpha,alpha," + longTag + "," + longTag, Category: "cover",
		Width: 640, Height: 480, CreatedAt: created, UpdatedAt: updated,
	}}}}
	uow := &imageListTestUOW{marker: "one-read-uow"}
	service := NewService(uow, store, nil)
	page, err := service.ListImages(context.Background(), mediaport.ImageListQuery{
		Limit: 0, Offset: -9, EnabledOnly: true, Search: "  %_  ", Category: " cover ",
		Tags: " alpha, alpha, beta ", TagGroups: []string{" alpha,beta ", "alpha,beta", "beta,alpha", ","},
		OnlyUnlabeled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if uow.calls != 1 || store.calls != 1 || store.writeCalls != 0 || store.marker != "one-read-uow" {
		t.Fatalf("uow=%d reads=%d writes=%d marker=%q", uow.calls, store.calls, store.writeCalls, store.marker)
	}
	if store.limit != 100 || store.offset != 0 || store.filter.Search != "%_" || store.filter.Category != "cover" || !store.filter.OnlyUnlabeled || !store.filter.EnabledOnly {
		t.Fatalf("limit=%d offset=%d filter=%#v", store.limit, store.offset, store.filter)
	}
	if want := []string{"alpha", "beta"}; !reflect.DeepEqual(store.filter.Tags, want) {
		t.Fatalf("tags=%q want=%q", store.filter.Tags, want)
	}
	if want := [][]string{{"alpha", "beta"}, {"beta", "alpha"}}; !reflect.DeepEqual(store.filter.TagGroups, want) {
		t.Fatalf("groups=%q want=%q", store.filter.TagGroups, want)
	}
	if page.Total != 1 || page.Limit != 100 || page.Offset != 0 || len(page.Items) != 1 {
		t.Fatalf("page=%#v", page)
	}
	item := page.Items[0]
	prefix := "/api/admin/image-library/42/variants/"
	if item.ID != 42 || item.Name != " Hero " || item.FileName != "hero.png" || item.MimeType != "image/png" || item.FileSize != 123 ||
		!item.Enabled || item.Description != "说明" || item.Category != "cover" || item.Width != 640 || item.Height != 480 {
		t.Fatalf("item=%#v", item)
	}
	if want := []string{"alpha", strings.Repeat("界", 64), strings.Repeat("界", 64)}; !reflect.DeepEqual(item.Tags, want) {
		t.Fatalf("item tags=%q want=%q", item.Tags, want)
	}
	if item.CreatedAt != created.UTC().Format(time.RFC3339Nano) || item.UpdatedAt != updated.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("created=%q updated=%q", item.CreatedAt, item.UpdatedAt)
	}
	if item.Thumb160URL != prefix+"thumb_160" || item.Thumb320URL != prefix+"thumb_320" || item.ThumbURL != item.Thumb320URL ||
		item.PreviewURL != prefix+"mobile_1080" || item.Mobile1080URL != item.PreviewURL ||
		item.Large1440URL != prefix+"large_1440" || item.OriginalURL != prefix+"original" {
		t.Fatalf("urls=%#v", item)
	}
}

func TestImageListEnabledOnlyIsPassedToTheRepository(t *testing.T) {
	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	read := ImageListRead{Total: 1, Rows: []ImageListRow{{
		ID: 1, FileName: "one.png", MimeType: "image/png", FileSize: 1, Width: 1, Height: 1,
		Enabled:   true,
		CreatedAt: now, UpdatedAt: now,
	}}}
	call := func(enabledOnly bool) (mediaport.ImageListPage, ImageListFilter) {
		store := &imageListTestStore{read: read}
		page, err := NewService(&imageListTestUOW{marker: "snapshot"}, store, nil).ListImages(
			context.Background(), mediaport.ImageListQuery{EnabledOnly: enabledOnly})
		if err != nil {
			t.Fatal(err)
		}
		if store.calls != 1 || store.writeCalls != 0 || len(page.Items) != 1 || !page.Items[0].Enabled {
			t.Fatalf("enabled_only=%v page=%#v reads=%d writes=%d", enabledOnly, page, store.calls, store.writeCalls)
		}
		return page, store.filter
	}
	_, trueFilter := call(true)
	_, falseFilter := call(false)
	if !trueFilter.EnabledOnly || falseFilter.EnabledOnly {
		t.Fatalf("true=%#v false=%#v", trueFilter, falseFilter)
	}
}

func TestImageListNormalizesTagsAndGroupsWithFrozenLongValueQuirk(t *testing.T) {
	prefix := strings.Repeat("界", 64)
	parts := []string{" Alpha ", "Alpha", prefix + "甲", prefix + "甲"}
	for index := 0; index < 60; index++ {
		parts = append(parts, "tag-"+string(rune('A'+index)))
	}
	tags := normalizeImageListTags(strings.Join(parts, ","))
	if len(tags) != 50 || tags[0] != "Alpha" || tags[1] != prefix || tags[2] != prefix || utf8.RuneCountInString(tags[1]) != 64 {
		t.Fatalf("len=%d first=%q second=%q third=%q", len(tags), tags[0], tags[1], tags[2])
	}
	groups := normalizeImageListTagGroups([]string{
		" Alpha,beta,Alpha ", "Alpha,beta", "beta,Alpha", ",", prefix + "甲," + prefix + "甲",
	})
	want := [][]string{{"Alpha", "beta"}, {"beta", "Alpha"}, {prefix, prefix}}
	if !reflect.DeepEqual(groups, want) {
		t.Fatalf("groups=%q want=%q", groups, want)
	}
}

func TestImageListClampPreservesFrozenLegacyPageSemantics(t *testing.T) {
	for _, test := range []struct {
		limit, offset int64
		wantLimit     int64
		wantOffset    int64
	}{
		{0, 0, 100, 0}, {-9, -3, 1, 0}, {1, 1, 1, 1}, {500, 9, 500, 9}, {501, 1 << 62, 500, 1 << 62},
	} {
		limit, offset := clampImageListPage(test.limit, test.offset)
		if limit != test.wantLimit || offset != test.wantOffset {
			t.Fatalf("input=%d/%d got=%d/%d want=%d/%d", test.limit, test.offset, limit, offset, test.wantLimit, test.wantOffset)
		}
	}
}

func TestImageListRepositoryFailuresAreSanitizedAndNeverWrite(t *testing.T) {
	raw := errors.New("pq: private-sql-marker-0356 actor=77")
	tests := []struct {
		name    string
		service *Service
		store   *imageListTestStore
	}{
		{"nil service", nil, nil},
		{"uow failure", NewService(&imageListTestUOW{err: raw}, &imageListTestStore{}, nil), nil},
		{"repository failure", NewService(&imageListTestUOW{}, &imageListTestStore{err: raw}, nil), nil},
		{"inconsistent snapshot", NewService(&imageListTestUOW{}, &imageListTestStore{read: ImageListRead{Total: 2}}, nil), nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			page, err := test.service.ListImages(context.Background(), mediaport.ImageListQuery{})
			if !errors.Is(err, ErrListUnavailable) || errors.Is(err, raw) {
				t.Fatalf("err=%v", err)
			}
			if page.Items == nil || len(page.Items) != 0 || page.Limit != 100 || page.Offset != 0 {
				t.Fatalf("page=%#v", page)
			}
			if test.service != nil {
				if store, ok := test.service.store.(*imageListTestStore); ok && store.writeCalls != 0 {
					t.Fatalf("write calls=%d", store.writeCalls)
				}
			}
		})
	}
}

func TestImageListRejectsMalformedRepositoryRows(t *testing.T) {
	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	valid := ImageListRow{ID: 1, FileName: "one.png", MimeType: "image/png", FileSize: 1, Width: 1, Height: 1, CreatedAt: now, UpdatedAt: now}
	tests := []struct {
		name string
		read ImageListRead
	}{
		{"negative total", ImageListRead{Total: -1}},
		{"page without total", ImageListRead{Total: 0, Rows: []ImageListRow{valid}}},
		{"missing page inside range", ImageListRead{Total: 2}},
		{"invalid row", ImageListRead{Total: 1, Rows: []ImageListRow{{ID: 1}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &imageListTestStore{read: test.read}
			page, err := NewService(&imageListTestUOW{marker: "snapshot"}, store, nil).ListImages(context.Background(), mediaport.ImageListQuery{})
			if !errors.Is(err, ErrListUnavailable) || page.Items == nil || len(page.Items) != 0 || store.calls != 1 || store.writeCalls != 0 {
				t.Fatalf("page=%#v err=%v reads=%d writes=%d", page, err, store.calls, store.writeCalls)
			}
		})
	}
}
